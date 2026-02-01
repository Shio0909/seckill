#!/usr/bin/env python3
"""
模型评估与分析脚本

包含:
- 离线评估指标计算
- 分组评估（按类别、城市等）
- A/B 测试分析
- 模型对比

用法:
    python evaluate.py --model_path models/ctr_model.txt --data_path ../data/processed/train.csv
"""

import os
import sys
import json
import argparse
import numpy as np
import pandas as pd
from pathlib import Path
from typing import Dict, List, Tuple
from collections import defaultdict

import lightgbm as lgb
from sklearn.metrics import (
    roc_auc_score, log_loss, precision_score, recall_score, f1_score,
    precision_recall_curve, roc_curve, average_precision_score,
    ndcg_score, confusion_matrix, classification_report
)

PROJECT_DIR = Path(__file__).parent.parent.parent
DATA_DIR = PROJECT_DIR / "data" / "processed"
MODEL_DIR = Path(__file__).parent / "models"


class RankingMetrics:
    """排序相关指标"""
    
    @staticmethod
    def hit_rate_at_k(y_true: np.ndarray, y_pred: np.ndarray, k: int = 10) -> float:
        """
        Hit Rate@K: 用户的正样本是否出现在 Top K 推荐中
        """
        top_k_idx = np.argsort(y_pred)[-k:]
        return float(np.any(y_true[top_k_idx] == 1))
    
    @staticmethod
    def mrr(y_true: np.ndarray, y_pred: np.ndarray) -> float:
        """
        Mean Reciprocal Rank: 第一个正样本的排名倒数
        """
        sorted_idx = np.argsort(y_pred)[::-1]
        sorted_labels = y_true[sorted_idx]
        
        for i, label in enumerate(sorted_labels):
            if label == 1:
                return 1.0 / (i + 1)
        return 0.0
    
    @staticmethod
    def precision_at_k(y_true: np.ndarray, y_pred: np.ndarray, k: int = 10) -> float:
        """
        Precision@K: Top K 推荐中正样本的比例
        """
        top_k_idx = np.argsort(y_pred)[-k:]
        return np.mean(y_true[top_k_idx])
    
    @staticmethod
    def recall_at_k(y_true: np.ndarray, y_pred: np.ndarray, k: int = 10) -> float:
        """
        Recall@K: Top K 推荐覆盖的正样本比例
        """
        top_k_idx = np.argsort(y_pred)[-k:]
        total_positive = np.sum(y_true)
        if total_positive == 0:
            return 0.0
        return np.sum(y_true[top_k_idx]) / total_positive
    
    @staticmethod
    def ndcg_at_k(y_true: np.ndarray, y_pred: np.ndarray, k: int = 10) -> float:
        """
        NDCG@K: 归一化折损累积增益
        """
        try:
            return ndcg_score([y_true], [y_pred], k=k)
        except:
            return 0.0
    
    @staticmethod
    def map_score(y_true: np.ndarray, y_pred: np.ndarray) -> float:
        """
        Mean Average Precision
        """
        return average_precision_score(y_true, y_pred)


class ModelEvaluator:
    """模型评估器"""
    
    def __init__(self, model_path: str):
        self.model = lgb.Booster(model_file=model_path)
        self.meta = self._load_meta(model_path)
    
    def _load_meta(self, model_path: str) -> Dict:
        """加载模型元信息"""
        meta_path = Path(model_path).with_suffix('.meta.json')
        if meta_path.exists():
            with open(meta_path, 'r') as f:
                return json.load(f)
        return {}
    
    def predict(self, X: pd.DataFrame) -> np.ndarray:
        """预测"""
        return self.model.predict(X)
    
    def evaluate_overall(self, X: pd.DataFrame, y: pd.Series) -> Dict:
        """整体评估"""
        y_pred = self.predict(X)
        y_true = y.values
        
        metrics = {
            # 分类指标
            'auc': roc_auc_score(y_true, y_pred),
            'logloss': log_loss(y_true, y_pred),
            'ap': average_precision_score(y_true, y_pred),
            
            # 排序指标
            'ndcg@10': RankingMetrics.ndcg_at_k(y_true, y_pred, k=10),
            'ndcg@20': RankingMetrics.ndcg_at_k(y_true, y_pred, k=20),
            'precision@10': RankingMetrics.precision_at_k(y_true, y_pred, k=10),
            'recall@10': RankingMetrics.recall_at_k(y_true, y_pred, k=10),
            'map': RankingMetrics.map_score(y_true, y_pred),
        }
        
        # 二分类指标（阈值 0.5）
        y_pred_binary = (y_pred >= 0.5).astype(int)
        metrics['precision'] = precision_score(y_true, y_pred_binary)
        metrics['recall'] = recall_score(y_true, y_pred_binary)
        metrics['f1'] = f1_score(y_true, y_pred_binary)
        
        return metrics
    
    def evaluate_by_group(
        self, 
        X: pd.DataFrame, 
        y: pd.Series, 
        group_col: str,
        original_df: pd.DataFrame = None
    ) -> Dict[str, Dict]:
        """按分组评估"""
        if original_df is None:
            original_df = X
        
        if group_col not in original_df.columns:
            print(f"⚠️ 列 {group_col} 不存在")
            return {}
        
        y_pred = self.predict(X)
        y_true = y.values
        
        results = {}
        for group_value in original_df[group_col].unique():
            mask = original_df[group_col] == group_value
            if mask.sum() < 10:  # 样本太少跳过
                continue
            
            group_true = y_true[mask]
            group_pred = y_pred[mask]
            
            try:
                auc = roc_auc_score(group_true, group_pred)
            except:
                auc = 0.0
            
            results[str(group_value)] = {
                'auc': auc,
                'count': int(mask.sum()),
                'positive_rate': float(group_true.mean()),
            }
        
        return results
    
    def evaluate_by_user(
        self,
        X: pd.DataFrame,
        y: pd.Series,
        user_ids: pd.Series,
    ) -> Dict:
        """按用户评估（计算用户粒度的排序指标）"""
        y_pred = self.predict(X)
        y_true = y.values
        
        user_metrics = defaultdict(list)
        
        for user_id in user_ids.unique():
            mask = user_ids == user_id
            if mask.sum() < 2:  # 每个用户至少 2 条样本
                continue
            
            user_true = y_true[mask]
            user_pred = y_pred[mask]
            
            if user_true.sum() == 0:  # 没有正样本
                continue
            
            user_metrics['hit@10'].append(
                RankingMetrics.hit_rate_at_k(user_true, user_pred, k=10)
            )
            user_metrics['mrr'].append(
                RankingMetrics.mrr(user_true, user_pred)
            )
            user_metrics['ndcg@10'].append(
                RankingMetrics.ndcg_at_k(user_true, user_pred, k=10)
            )
        
        # 计算平均值
        return {
            metric: float(np.mean(values)) 
            for metric, values in user_metrics.items()
        }


class ABTestAnalyzer:
    """A/B 测试分析器"""
    
    @staticmethod
    def compare_models(
        model_a: ModelEvaluator,
        model_b: ModelEvaluator,
        X: pd.DataFrame,
        y: pd.Series,
    ) -> Dict:
        """对比两个模型"""
        metrics_a = model_a.evaluate_overall(X, y)
        metrics_b = model_b.evaluate_overall(X, y)
        
        comparison = {}
        for metric in metrics_a.keys():
            val_a = metrics_a[metric]
            val_b = metrics_b[metric]
            diff = val_b - val_a
            diff_pct = (diff / val_a * 100) if val_a != 0 else 0
            
            comparison[metric] = {
                'model_a': val_a,
                'model_b': val_b,
                'diff': diff,
                'diff_pct': diff_pct,
                'winner': 'B' if diff > 0 else 'A' if diff < 0 else 'Tie'
            }
        
        return comparison
    
    @staticmethod
    def statistical_significance(
        y_true: np.ndarray,
        y_pred_a: np.ndarray,
        y_pred_b: np.ndarray,
        n_bootstrap: int = 1000,
    ) -> Dict:
        """
        Bootstrap 方法计算统计显著性
        """
        n_samples = len(y_true)
        auc_diffs = []
        
        for _ in range(n_bootstrap):
            idx = np.random.choice(n_samples, n_samples, replace=True)
            try:
                auc_a = roc_auc_score(y_true[idx], y_pred_a[idx])
                auc_b = roc_auc_score(y_true[idx], y_pred_b[idx])
                auc_diffs.append(auc_b - auc_a)
            except:
                continue
        
        auc_diffs = np.array(auc_diffs)
        
        return {
            'mean_diff': float(np.mean(auc_diffs)),
            'std_diff': float(np.std(auc_diffs)),
            'ci_95_lower': float(np.percentile(auc_diffs, 2.5)),
            'ci_95_upper': float(np.percentile(auc_diffs, 97.5)),
            'significant': not (np.percentile(auc_diffs, 2.5) <= 0 <= np.percentile(auc_diffs, 97.5)),
        }


def print_metrics(metrics: Dict, title: str = "评估结果"):
    """打印评估指标"""
    print(f"\n📊 {title}")
    print("-" * 40)
    
    for metric, value in metrics.items():
        if isinstance(value, float):
            print(f"   {metric:<20s}: {value:.4f}")
        else:
            print(f"   {metric:<20s}: {value}")


def print_group_metrics(group_metrics: Dict, group_name: str):
    """打印分组评估结果"""
    print(f"\n📊 按 {group_name} 分组评估")
    print("-" * 60)
    print(f"   {'Group':<15s} {'AUC':>10s} {'Count':>10s} {'Pos Rate':>10s}")
    print("-" * 60)
    
    sorted_groups = sorted(group_metrics.items(), key=lambda x: x[1]['auc'], reverse=True)
    
    for group, metrics in sorted_groups[:20]:  # 只显示前 20 个
        print(f"   {str(group):<15s} {metrics['auc']:>10.4f} {metrics['count']:>10d} {metrics['positive_rate']:>10.2%}")


def parse_args():
    parser = argparse.ArgumentParser(description='Model Evaluation')
    parser.add_argument('--model_path', type=str, default=str(MODEL_DIR / 'ctr_model.txt'),
                        help='模型路径')
    parser.add_argument('--data_path', type=str, default=str(DATA_DIR / 'train.csv'),
                        help='测试数据路径')
    parser.add_argument('--output_path', type=str, default=str(MODEL_DIR / 'evaluation_report.json'),
                        help='评估报告输出路径')
    return parser.parse_args()


def main():
    args = parse_args()
    
    print("=" * 60)
    print("📊 EventHub 推荐系统 - 模型评估")
    print("=" * 60)
    
    # 加载模型
    print(f"\n📂 加载模型: {args.model_path}")
    evaluator = ModelEvaluator(args.model_path)
    
    # 加载数据
    print(f"📂 加载数据: {args.data_path}")
    df = pd.read_csv(args.data_path)
    
    # 准备特征
    feature_names = evaluator.meta.get('feature_names', [])
    if not feature_names:
        print("⚠️ 无法获取特征名，使用默认特征")
        feature_names = ['gender', 'age', 'city_id', 'category_id', 'event_city_id', 
                         'price', 'hot_score', 'city_match']
    
    available_features = [f for f in feature_names if f in df.columns]
    X = df[available_features]
    y = df['label']
    
    print(f"   样本数: {len(df):,}")
    print(f"   特征数: {len(available_features)}")
    
    # 整体评估
    overall_metrics = evaluator.evaluate_overall(X, y)
    print_metrics(overall_metrics, "整体评估")
    
    # 分组评估
    report = {'overall': overall_metrics, 'by_group': {}}
    
    for group_col in ['category_id', 'city_id', 'gender']:
        if group_col in df.columns:
            group_metrics = evaluator.evaluate_by_group(X, y, group_col, df)
            if group_metrics:
                print_group_metrics(group_metrics, group_col)
                report['by_group'][group_col] = group_metrics
    
    # 用户粒度评估
    if 'user_id' in df.columns:
        print("\n📊 用户粒度评估")
        print("-" * 40)
        user_metrics = evaluator.evaluate_by_user(X, y, df['user_id'])
        for metric, value in user_metrics.items():
            print(f"   {metric:<20s}: {value:.4f}")
        report['user_level'] = user_metrics
    
    # 保存报告
    with open(args.output_path, 'w') as f:
        json.dump(report, f, indent=2)
    print(f"\n📄 评估报告已保存: {args.output_path}")
    
    print("\n" + "=" * 60)
    print("✅ 评估完成!")
    print("=" * 60)


if __name__ == '__main__':
    main()

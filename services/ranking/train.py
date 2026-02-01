#!/usr/bin/env python3
"""
LightGBM CTR 排序模型训练脚本

用于训练推荐系统的排序模型，预测用户点击/购买活动的概率

用法:
    python train.py
    python train.py --data_path ../data/processed/train.csv
    python train.py --epochs 100 --lr 0.05
"""

import os
import sys
import json
import pickle
import argparse
import numpy as np
import pandas as pd
from pathlib import Path
from datetime import datetime
from typing import Dict, List, Tuple, Optional

import lightgbm as lgb
from sklearn.model_selection import train_test_split
from sklearn.metrics import roc_auc_score, log_loss, precision_recall_curve, average_precision_score

# 项目路径
PROJECT_DIR = Path(__file__).parent.parent.parent
DATA_DIR = PROJECT_DIR / "data" / "processed"
MODEL_DIR = Path(__file__).parent / "models"


class FeatureConfig:
    """特征配置"""
    
    # 用户特征
    USER_FEATURES = [
        'gender',           # 性别
        'age',              # 年龄
        'city_id',          # 用户所在城市
    ]
    
    # 活动特征
    EVENT_FEATURES = [
        'category_id',      # 活动类别
        'event_city_id',    # 活动城市
        'price',            # 票价
        'hot_score',        # 热度分数
    ]
    
    # 交叉特征
    CROSS_FEATURES = [
        'city_match',       # 用户城市与活动城市是否匹配
    ]
    
    # 类别型特征（用于 LightGBM 的 categorical_feature）
    CATEGORICAL_FEATURES = [
        'gender',
        'city_id', 
        'category_id',
        'event_city_id',
    ]
    
    @classmethod
    def get_all_features(cls) -> List[str]:
        """获取所有特征列"""
        return cls.USER_FEATURES + cls.EVENT_FEATURES + cls.CROSS_FEATURES


class DataLoader:
    """数据加载器"""
    
    def __init__(self, data_path: str):
        self.data_path = Path(data_path)
    
    def load(self) -> pd.DataFrame:
        """加载训练数据"""
        print(f"📂 加载数据: {self.data_path}")
        
        if not self.data_path.exists():
            raise FileNotFoundError(f"数据文件不存在: {self.data_path}")
        
        df = pd.read_csv(self.data_path)
        print(f"   样本数: {len(df):,}")
        print(f"   正样本: {len(df[df['label'] == 1]):,} ({len(df[df['label'] == 1]) / len(df) * 100:.2f}%)")
        print(f"   负样本: {len(df[df['label'] == 0]):,} ({len(df[df['label'] == 0]) / len(df) * 100:.2f}%)")
        
        return df
    
    def preprocess(self, df: pd.DataFrame) -> pd.DataFrame:
        """数据预处理"""
        print("\n🔧 数据预处理...")
        
        # 处理缺失值
        for col in FeatureConfig.get_all_features():
            if col in df.columns:
                if df[col].dtype in ['float64', 'float32']:
                    df[col] = df[col].fillna(df[col].median())
                else:
                    df[col] = df[col].fillna(-1)
        
        # 添加衍生特征
        df = self._add_derived_features(df)
        
        return df
    
    def _add_derived_features(self, df: pd.DataFrame) -> pd.DataFrame:
        """添加衍生特征"""
        
        # 年龄分段
        if 'age' in df.columns:
            df['age_group'] = pd.cut(
                df['age'], 
                bins=[0, 18, 25, 35, 45, 55, 100],
                labels=[0, 1, 2, 3, 4, 5]
            ).astype(int)
        
        # 价格分段
        if 'price' in df.columns:
            df['price_level'] = pd.qcut(
                df['price'], 
                q=5, 
                labels=[0, 1, 2, 3, 4],
                duplicates='drop'
            ).astype(int)
        
        # 热度分段
        if 'hot_score' in df.columns:
            df['hot_level'] = pd.qcut(
                df['hot_score'], 
                q=5, 
                labels=[0, 1, 2, 3, 4],
                duplicates='drop'
            ).astype(int)
        
        return df


class LGBMRanker:
    """LightGBM 排序模型"""
    
    def __init__(self, params: Optional[Dict] = None):
        self.params = params or self._default_params()
        self.model = None
        self.feature_names = None
        self.feature_importance = None
    
    def _default_params(self) -> Dict:
        """默认超参数"""
        return {
            'objective': 'binary',
            'metric': ['auc', 'binary_logloss'],
            'boosting_type': 'gbdt',
            'num_leaves': 31,
            'max_depth': 6,
            'learning_rate': 0.05,
            'feature_fraction': 0.8,
            'bagging_fraction': 0.8,
            'bagging_freq': 5,
            'min_child_samples': 20,
            'lambda_l1': 0.1,
            'lambda_l2': 0.1,
            'verbose': -1,
            'seed': 42,
        }
    
    def train(
        self, 
        X_train: pd.DataFrame, 
        y_train: pd.Series,
        X_valid: pd.DataFrame,
        y_valid: pd.Series,
        num_boost_round: int = 500,
        early_stopping_rounds: int = 50,
        categorical_features: List[str] = None,
    ) -> Dict:
        """训练模型"""
        print("\n🚀 开始训练 LightGBM 模型...")
        
        self.feature_names = list(X_train.columns)
        
        # 构建数据集
        train_data = lgb.Dataset(
            X_train, 
            label=y_train,
            categorical_feature=categorical_features,
            free_raw_data=False,
        )
        valid_data = lgb.Dataset(
            X_valid, 
            label=y_valid,
            reference=train_data,
            free_raw_data=False,
        )
        
        # 训练回调
        callbacks = [
            lgb.early_stopping(stopping_rounds=early_stopping_rounds),
            lgb.log_evaluation(period=50),
        ]
        
        # 训练
        self.model = lgb.train(
            self.params,
            train_data,
            num_boost_round=num_boost_round,
            valid_sets=[train_data, valid_data],
            valid_names=['train', 'valid'],
            callbacks=callbacks,
        )
        
        # 特征重要性
        self.feature_importance = dict(zip(
            self.feature_names,
            self.model.feature_importance(importance_type='gain')
        ))
        
        # 评估
        train_pred = self.model.predict(X_train)
        valid_pred = self.model.predict(X_valid)
        
        metrics = {
            'train_auc': roc_auc_score(y_train, train_pred),
            'valid_auc': roc_auc_score(y_valid, valid_pred),
            'train_logloss': log_loss(y_train, train_pred),
            'valid_logloss': log_loss(y_valid, valid_pred),
            'best_iteration': self.model.best_iteration,
        }
        
        print(f"\n📊 训练结果:")
        print(f"   Train AUC: {metrics['train_auc']:.4f}")
        print(f"   Valid AUC: {metrics['valid_auc']:.4f}")
        print(f"   Train LogLoss: {metrics['train_logloss']:.4f}")
        print(f"   Valid LogLoss: {metrics['valid_logloss']:.4f}")
        print(f"   Best Iteration: {metrics['best_iteration']}")
        
        return metrics
    
    def predict(self, X: pd.DataFrame) -> np.ndarray:
        """预测"""
        if self.model is None:
            raise ValueError("模型未训练")
        return self.model.predict(X)
    
    def save(self, model_path: str):
        """保存模型"""
        model_path = Path(model_path)
        model_path.parent.mkdir(parents=True, exist_ok=True)
        
        # 保存 LightGBM 模型
        self.model.save_model(str(model_path))
        
        # 保存元信息
        meta = {
            'feature_names': self.feature_names,
            'feature_importance': self.feature_importance,
            'params': self.params,
            'created_at': datetime.now().isoformat(),
        }
        meta_path = model_path.with_suffix('.meta.json')
        with open(meta_path, 'w') as f:
            json.dump(meta, f, indent=2)
        
        print(f"💾 模型已保存: {model_path}")
    
    def load(self, model_path: str):
        """加载模型"""
        model_path = Path(model_path)
        
        self.model = lgb.Booster(model_file=str(model_path))
        
        meta_path = model_path.with_suffix('.meta.json')
        if meta_path.exists():
            with open(meta_path, 'r') as f:
                meta = json.load(f)
            self.feature_names = meta.get('feature_names')
            self.feature_importance = meta.get('feature_importance')
            self.params = meta.get('params')
        
        print(f"📂 模型已加载: {model_path}")
    
    def print_feature_importance(self, top_k: int = 20):
        """打印特征重要性"""
        if self.feature_importance is None:
            return
        
        sorted_importance = sorted(
            self.feature_importance.items(), 
            key=lambda x: x[1], 
            reverse=True
        )[:top_k]
        
        print(f"\n📈 Top {top_k} 特征重要性:")
        for i, (name, score) in enumerate(sorted_importance, 1):
            print(f"   {i:2d}. {name:<20s} {score:,.0f}")


class ModelEvaluator:
    """模型评估器"""
    
    @staticmethod
    def evaluate(y_true: np.ndarray, y_pred: np.ndarray) -> Dict:
        """综合评估"""
        metrics = {}
        
        # AUC
        metrics['auc'] = roc_auc_score(y_true, y_pred)
        
        # Log Loss
        metrics['logloss'] = log_loss(y_true, y_pred)
        
        # Average Precision (PR-AUC)
        metrics['ap'] = average_precision_score(y_true, y_pred)
        
        # 按阈值计算 Precision/Recall
        for threshold in [0.3, 0.5, 0.7]:
            y_pred_binary = (y_pred >= threshold).astype(int)
            tp = np.sum((y_pred_binary == 1) & (y_true == 1))
            fp = np.sum((y_pred_binary == 1) & (y_true == 0))
            fn = np.sum((y_pred_binary == 0) & (y_true == 1))
            
            precision = tp / (tp + fp) if (tp + fp) > 0 else 0
            recall = tp / (tp + fn) if (tp + fn) > 0 else 0
            
            metrics[f'precision@{threshold}'] = precision
            metrics[f'recall@{threshold}'] = recall
        
        return metrics
    
    @staticmethod
    def print_metrics(metrics: Dict):
        """打印评估指标"""
        print("\n📊 模型评估指标:")
        print(f"   AUC:      {metrics['auc']:.4f}")
        print(f"   LogLoss:  {metrics['logloss']:.4f}")
        print(f"   AP:       {metrics['ap']:.4f}")
        print(f"   Precision@0.5: {metrics['precision@0.5']:.4f}")
        print(f"   Recall@0.5:    {metrics['recall@0.5']:.4f}")


def parse_args():
    """解析命令行参数"""
    parser = argparse.ArgumentParser(description='LightGBM CTR Model Training')
    parser.add_argument('--data_path', type=str, default=str(DATA_DIR / 'train.csv'),
                        help='训练数据路径')
    parser.add_argument('--model_dir', type=str, default=str(MODEL_DIR),
                        help='模型保存目录')
    parser.add_argument('--num_boost_round', type=int, default=500,
                        help='最大迭代次数')
    parser.add_argument('--early_stopping', type=int, default=50,
                        help='早停轮数')
    parser.add_argument('--lr', type=float, default=0.05,
                        help='学习率')
    parser.add_argument('--test_size', type=float, default=0.2,
                        help='测试集比例')
    return parser.parse_args()


def main():
    """主函数"""
    args = parse_args()
    
    print("=" * 60)
    print("🎯 EventHub 推荐系统 - LightGBM CTR 模型训练")
    print("=" * 60)
    
    # 1. 加载数据
    loader = DataLoader(args.data_path)
    df = loader.load()
    df = loader.preprocess(df)
    
    # 2. 准备特征和标签
    feature_columns = [col for col in FeatureConfig.get_all_features() if col in df.columns]
    
    # 添加衍生特征
    derived_features = ['age_group', 'price_level', 'hot_level']
    for f in derived_features:
        if f in df.columns:
            feature_columns.append(f)
    
    print(f"\n📋 使用的特征 ({len(feature_columns)} 个):")
    for i, col in enumerate(feature_columns, 1):
        print(f"   {i:2d}. {col}")
    
    X = df[feature_columns]
    y = df['label']
    
    # 3. 划分数据集
    X_train, X_test, y_train, y_test = train_test_split(
        X, y, test_size=args.test_size, random_state=42, stratify=y
    )
    X_train, X_valid, y_train, y_valid = train_test_split(
        X_train, y_train, test_size=0.1, random_state=42, stratify=y_train
    )
    
    print(f"\n📊 数据集划分:")
    print(f"   训练集: {len(X_train):,}")
    print(f"   验证集: {len(X_valid):,}")
    print(f"   测试集: {len(X_test):,}")
    
    # 4. 训练模型
    params = {
        'objective': 'binary',
        'metric': ['auc', 'binary_logloss'],
        'boosting_type': 'gbdt',
        'num_leaves': 31,
        'max_depth': 6,
        'learning_rate': args.lr,
        'feature_fraction': 0.8,
        'bagging_fraction': 0.8,
        'bagging_freq': 5,
        'min_child_samples': 20,
        'lambda_l1': 0.1,
        'lambda_l2': 0.1,
        'verbose': -1,
        'seed': 42,
    }
    
    model = LGBMRanker(params)
    
    # 获取类别特征
    cat_features = [f for f in FeatureConfig.CATEGORICAL_FEATURES if f in feature_columns]
    
    train_metrics = model.train(
        X_train, y_train,
        X_valid, y_valid,
        num_boost_round=args.num_boost_round,
        early_stopping_rounds=args.early_stopping,
        categorical_features=cat_features,
    )
    
    # 5. 测试集评估
    print("\n" + "=" * 60)
    print("📊 测试集评估")
    print("=" * 60)
    
    y_pred = model.predict(X_test)
    test_metrics = ModelEvaluator.evaluate(y_test.values, y_pred)
    ModelEvaluator.print_metrics(test_metrics)
    
    # 6. 特征重要性
    model.print_feature_importance(top_k=15)
    
    # 7. 保存模型
    model_path = Path(args.model_dir) / 'ctr_model.txt'
    model.save(model_path)
    
    # 8. 保存评估报告
    report = {
        'train_metrics': train_metrics,
        'test_metrics': test_metrics,
        'feature_importance': model.feature_importance,
        'params': params,
        'data_path': args.data_path,
        'created_at': datetime.now().isoformat(),
    }
    
    report_path = Path(args.model_dir) / 'training_report.json'
    with open(report_path, 'w') as f:
        json.dump(report, f, indent=2)
    
    print(f"\n📄 训练报告: {report_path}")
    
    print("\n" + "=" * 60)
    print("✅ 训练完成!")
    print("=" * 60)


if __name__ == '__main__':
    main()

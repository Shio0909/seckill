"""
LightGBM排序模型
"""
import os
import pickle
from typing import Dict, List, Optional

import numpy as np
from loguru import logger


class LightGBMRanker:
    """
    基于LightGBM的CTR预估排序模型
    
    使用场景:
    1. 对召回结果进行精排
    2. 预测点击率
    """

    def __init__(self, config):
        self.config = config
        self.model = None
        self.model_path = config.ranking.model_path
        
        self._load_model()
        
        logger.info("LightGBMRanker 初始化完成")

    def _load_model(self):
        """加载模型"""
        if os.path.exists(self.model_path):
            try:
                with open(self.model_path, "rb") as f:
                    self.model = pickle.load(f)
                logger.info(f"模型加载成功: {self.model_path}")
            except Exception as e:
                logger.warning(f"模型加载失败: {e}")
                self.model = None
        else:
            logger.warning(f"模型文件不存在: {self.model_path},使用默认排序")
            self.model = None

    def rank(
        self,
        candidates: List[Dict],
        features: np.ndarray,
    ) -> List[Dict]:
        """
        对候选进行排序
        
        Args:
            candidates: 候选列表 [{"event_id": xx, "score": xx, ...}, ...]
            features: 特征矩阵 [N, feature_dim]
            
        Returns:
            排序后的候选列表
        """
        if not candidates:
            return []
        
        if self.model is None:
            # 模型未加载,使用召回分数排序
            logger.debug("模型未加载,使用召回分数排序")
            return sorted(candidates, key=lambda x: x["score"], reverse=True)
        
        try:
            # 预测CTR
            predictions = self.model.predict(features)
            
            # 合并分数
            for i, cand in enumerate(candidates):
                recall_score = cand["score"]
                ctr_score = predictions[i]
                
                # 最终分数 = 召回分数 * 0.3 + CTR分数 * 0.7
                final_score = recall_score * 0.3 + ctr_score * 0.7
                
                cand["score"] = final_score
                cand["debug_info"] = {
                    "recall_score": recall_score,
                    "ctr_score": float(ctr_score),
                }
            
            # 按最终分数排序
            ranked = sorted(candidates, key=lambda x: x["score"], reverse=True)
            
            return ranked
            
        except Exception as e:
            logger.error(f"排序失败: {e}")
            return sorted(candidates, key=lambda x: x["score"], reverse=True)

    def train(
        self,
        X_train: np.ndarray,
        y_train: np.ndarray,
        X_val: np.ndarray = None,
        y_val: np.ndarray = None,
    ):
        """
        训练模型(离线脚本调用)
        
        Args:
            X_train: 训练特征 [N, feature_dim]
            y_train: 训练标签 [N,] (0/1)
            X_val: 验证特征
            y_val: 验证标签
        """
        import lightgbm as lgb
        
        # LightGBM参数
        params = {
            "objective": "binary",
            "metric": "auc",
            "boosting_type": "gbdt",
            "num_leaves": 31,
            "learning_rate": 0.05,
            "feature_fraction": 0.8,
            "bagging_fraction": 0.8,
            "bagging_freq": 5,
            "verbose": -1,
            "n_jobs": -1,
        }
        
        train_data = lgb.Dataset(X_train, label=y_train)
        
        valid_sets = [train_data]
        if X_val is not None:
            val_data = lgb.Dataset(X_val, label=y_val, reference=train_data)
            valid_sets.append(val_data)
        
        # 训练
        self.model = lgb.train(
            params,
            train_data,
            num_boost_round=200,
            valid_sets=valid_sets,
            callbacks=[
                lgb.early_stopping(stopping_rounds=20),
                lgb.log_evaluation(period=20),
            ],
        )
        
        # 保存模型
        os.makedirs(os.path.dirname(self.model_path), exist_ok=True)
        with open(self.model_path, "wb") as f:
            pickle.dump(self.model, f)
        
        logger.info(f"模型训练完成,保存至: {self.model_path}")
        
        # 打印特征重要性
        importance = self.model.feature_importance()
        logger.info(f"特征重要性Top10: {sorted(enumerate(importance), key=lambda x: x[1], reverse=True)[:10]}")

    def evaluate(self, X_test: np.ndarray, y_test: np.ndarray) -> Dict:
        """
        评估模型
        
        Returns:
            {"auc": xx, "logloss": xx}
        """
        from sklearn.metrics import roc_auc_score, log_loss
        
        predictions = self.model.predict(X_test)
        
        auc = roc_auc_score(y_test, predictions)
        logloss = log_loss(y_test, predictions)
        
        logger.info(f"模型评估: AUC={auc:.4f}, LogLoss={logloss:.4f}")
        
        return {"auc": auc, "logloss": logloss}

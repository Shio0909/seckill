"""
LightGBM模型训练脚本
"""
import os
import pickle
from datetime import datetime

import lightgbm as lgb
import numpy as np
import pandas as pd
from loguru import logger
from sklearn.model_selection import train_test_split
from sklearn.metrics import roc_auc_score, log_loss


def load_data():
    """加载数据"""
    processed_dir = "data/processed"
    
    train_data = pd.read_csv(os.path.join(processed_dir, "train_data.csv"))
    events = pd.read_csv(os.path.join(processed_dir, "events.csv"))
    users = pd.read_csv(os.path.join(processed_dir, "users.csv"))
    behaviors = pd.read_csv(os.path.join(processed_dir, "behaviors.csv"))
    
    return train_data, events, users, behaviors


def compute_user_features(train_data, behaviors, users):
    """计算用户特征"""
    # 用户行为统计
    user_behavior_stats = behaviors.groupby("user_id").agg({
        "event_id": "count",
        "behavior_type": lambda x: (x == "view").sum(),
    }).rename(columns={
        "event_id": "behavior_count",
        "behavior_type": "view_count",
    })
    
    # 点击数
    click_counts = behaviors[behaviors["behavior_type"] == "click"].groupby("user_id").size()
    user_behavior_stats["click_count"] = click_counts
    user_behavior_stats["click_count"] = user_behavior_stats["click_count"].fillna(0)
    
    # 购买数
    order_counts = behaviors[behaviors["behavior_type"] == "order"].groupby("user_id").size()
    user_behavior_stats["order_count"] = order_counts
    user_behavior_stats["order_count"] = user_behavior_stats["order_count"].fillna(0)
    
    # 偏好类别
    prefer_category = behaviors.groupby("user_id")["category_id"].agg(
        lambda x: x.mode().iloc[0] if len(x.mode()) > 0 else 0
    )
    user_behavior_stats["prefer_category"] = prefer_category
    
    # 合并用户基础信息
    user_features = users.set_index("user_id").join(user_behavior_stats, how="left")
    user_features = user_features.fillna(0)
    
    return user_features


def compute_event_features(events, behaviors):
    """计算活动特征"""
    # 活动统计
    event_stats = behaviors.groupby("event_id").agg({
        "user_id": "nunique",
        "behavior_type": lambda x: (x == "order").sum(),
    }).rename(columns={
        "user_id": "total_viewers",
        "behavior_type": "total_orders",
    })
    
    # 热度分
    view_counts = behaviors[behaviors["behavior_type"] == "view"].groupby("event_id").size()
    event_stats["view_count"] = view_counts
    event_stats["view_count"] = event_stats["view_count"].fillna(0)
    
    # 热度分 = views + clicks*3 + orders*10
    click_counts = behaviors[behaviors["behavior_type"] == "click"].groupby("event_id").size()
    event_stats["click_count"] = click_counts.fillna(0)
    
    event_stats["hot_score"] = (
        event_stats["view_count"] + 
        event_stats["click_count"] * 3 + 
        event_stats["total_orders"] * 10
    )
    
    # 合并活动基础信息
    event_features = events.set_index("event_id").join(event_stats, how="left")
    event_features = event_features.fillna(0)
    
    return event_features


def build_features(train_data, user_features, event_features):
    """构建特征矩阵"""
    features = []
    
    for _, row in train_data.iterrows():
        user_id = row["user_id"]
        event_id = row["event_id"]
        
        # 用户特征
        if user_id in user_features.index:
            uf = user_features.loc[user_id]
            user_feat = [
                uf.get("age_group", 0),
                uf.get("gender", 0),
                uf.get("city_id", 0),
                uf.get("behavior_count", 0),
                uf.get("view_count", 0),
                uf.get("click_count", 0),
                uf.get("order_count", 0),
                uf.get("prefer_category", 0),
            ]
        else:
            user_feat = [0] * 8
        
        # 活动特征
        if event_id in event_features.index:
            ef = event_features.loc[event_id]
            event_feat = [
                ef.get("category_id", 0),
                ef.get("price", 0),
                ef.get("city_id", 0),
                ef.get("hot_score", 0),
                ef.get("total_viewers", 0),
                ef.get("total_orders", 0),
            ]
        else:
            event_feat = [0] * 6
        
        # 交叉特征
        user_city = user_features.loc[user_id]["city_id"] if user_id in user_features.index else -1
        event_city = event_features.loc[event_id]["city_id"] if event_id in event_features.index else -1
        city_match = 1 if user_city == event_city else 0
        
        user_prefer = user_features.loc[user_id]["prefer_category"] if user_id in user_features.index else -1
        event_category = event_features.loc[event_id]["category_id"] if event_id in event_features.index else -1
        category_match = 1 if user_prefer == event_category else 0
        
        cross_feat = [city_match, category_match]
        
        # 合并所有特征
        feature_vector = user_feat + event_feat + cross_feat
        features.append(feature_vector)
    
    return np.array(features, dtype=np.float32)


def train_model(X_train, y_train, X_val, y_val):
    """训练LightGBM模型"""
    # 创建数据集
    train_data = lgb.Dataset(X_train, label=y_train)
    val_data = lgb.Dataset(X_val, label=y_val, reference=train_data)
    
    # 参数
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
        "seed": 42,
    }
    
    # 训练
    logger.info("开始训练模型...")
    model = lgb.train(
        params,
        train_data,
        num_boost_round=300,
        valid_sets=[train_data, val_data],
        valid_names=["train", "valid"],
        callbacks=[
            lgb.early_stopping(stopping_rounds=30),
            lgb.log_evaluation(period=50),
        ],
    )
    
    return model


def evaluate_model(model, X_test, y_test):
    """评估模型"""
    predictions = model.predict(X_test)
    
    auc = roc_auc_score(y_test, predictions)
    logloss_val = log_loss(y_test, predictions)
    
    logger.info(f"测试集评估: AUC={auc:.4f}, LogLoss={logloss_val:.4f}")
    
    return {"auc": auc, "logloss": logloss_val}


def save_model(model, path):
    """保存模型"""
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "wb") as f:
        pickle.dump(model, f)
    logger.info(f"模型已保存: {path}")


def main():
    """主函数"""
    logger.info("=" * 50)
    logger.info("LightGBM CTR模型训练")
    logger.info("=" * 50)
    
    # 加载数据
    logger.info("加载数据...")
    train_data, events, users, behaviors = load_data()
    
    # 计算特征
    logger.info("计算用户特征...")
    user_features = compute_user_features(train_data, behaviors, users)
    
    logger.info("计算活动特征...")
    event_features = compute_event_features(events, behaviors)
    
    # 构建特征矩阵
    logger.info("构建特征矩阵...")
    X = build_features(train_data, user_features, event_features)
    y = train_data["label"].values
    
    logger.info(f"特征维度: {X.shape}")
    logger.info(f"正样本比例: {y.mean():.4f}")
    
    # 划分数据集
    X_train, X_test, y_train, y_test = train_test_split(
        X, y, test_size=0.2, random_state=42, stratify=y
    )
    X_train, X_val, y_train, y_val = train_test_split(
        X_train, y_train, test_size=0.1, random_state=42, stratify=y_train
    )
    
    logger.info(f"训练集: {len(X_train)}, 验证集: {len(X_val)}, 测试集: {len(X_test)}")
    
    # 训练模型
    model = train_model(X_train, y_train, X_val, y_val)
    
    # 评估模型
    metrics = evaluate_model(model, X_test, y_test)
    
    # 保存模型
    model_path = "src/model/models/lgb_ctr_v1.pkl"
    save_model(model, model_path)
    
    # 打印特征重要性
    logger.info("\n特征重要性 (Top 10):")
    feature_names = [
        "user_age", "user_gender", "user_city", "user_behavior_cnt",
        "user_view_cnt", "user_click_cnt", "user_order_cnt", "user_prefer_cat",
        "event_category", "event_price", "event_city", "event_hot_score",
        "event_viewers", "event_orders",
        "city_match", "category_match",
    ]
    importance = model.feature_importance()
    for idx in np.argsort(importance)[-10:][::-1]:
        name = feature_names[idx] if idx < len(feature_names) else f"feature_{idx}"
        logger.info(f"  {name}: {importance[idx]}")
    
    logger.info("\n训练完成!")
    logger.info(f"模型路径: {model_path}")
    logger.info(f"AUC: {metrics['auc']:.4f}")


if __name__ == "__main__":
    main()

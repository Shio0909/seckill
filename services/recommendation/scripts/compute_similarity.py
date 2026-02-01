"""
计算ItemCF相似度矩阵并写入Redis
"""
import os
import math
from collections import defaultdict

import pandas as pd
import redis
from loguru import logger


def load_behaviors():
    """加载用户行为数据"""
    processed_dir = "data/processed"
    behaviors = pd.read_csv(os.path.join(processed_dir, "behaviors.csv"))
    return behaviors


def build_user_item_matrix(behaviors: pd.DataFrame):
    """构建用户-物品交互矩阵"""
    # 只考虑有明确正向行为的
    positive_behaviors = behaviors[behaviors["behavior_type"].isin(["click", "order"])]
    
    user_items = defaultdict(set)
    item_users = defaultdict(set)
    
    for _, row in positive_behaviors.iterrows():
        user_id = row["user_id"]
        event_id = row["event_id"]
        
        user_items[user_id].add(event_id)
        item_users[event_id].add(user_id)
    
    logger.info(f"用户数: {len(user_items)}, 物品数: {len(item_users)}")
    
    return user_items, item_users


def compute_item_similarity(item_users: dict, top_k: int = 100):
    """
    计算物品相似度
    
    使用余弦相似度 (带IUF惩罚活跃用户)
    """
    items = list(item_users.keys())
    n_items = len(items)
    
    logger.info(f"计算 {n_items} 个物品的相似度...")
    
    # 计算用户活跃度(用于IUF)
    user_activity = defaultdict(int)
    for item_id, users in item_users.items():
        for user_id in users:
            user_activity[user_id] += 1
    
    # 计算物品相似度
    similarity = defaultdict(dict)
    
    for i, item_a in enumerate(items):
        if i % 500 == 0:
            logger.info(f"  进度: {i}/{n_items}")
        
        users_a = item_users[item_a]
        
        scores = []
        for item_b in items:
            if item_a == item_b:
                continue
            
            users_b = item_users[item_b]
            
            # 交集用户
            common_users = users_a & users_b
            if not common_users:
                continue
            
            # 带IUF的余弦相似度
            # sim(a, b) = sum(1/log(1+N_u)) / sqrt(|users_a| * |users_b|)
            numerator = sum(1 / math.log(1 + user_activity[u]) for u in common_users)
            denominator = math.sqrt(len(users_a) * len(users_b))
            
            sim = numerator / denominator
            scores.append((item_b, sim))
        
        # 取Top K
        scores.sort(key=lambda x: x[1], reverse=True)
        similarity[item_a] = {item_id: score for item_id, score in scores[:top_k]}
    
    return similarity


def save_to_redis(similarity: dict, redis_config: dict):
    """保存相似度到Redis"""
    r = redis.Redis(
        host=redis_config.get("host", "localhost"),
        port=redis_config.get("port", 6379),
        db=redis_config.get("db", 0),
        password=redis_config.get("password") or None,
        decode_responses=True,
    )
    
    pipe = r.pipeline()
    
    for item_a, similar_items in similarity.items():
        key = f"cf:item:{item_a}:similar"
        
        # 删除旧数据
        pipe.delete(key)
        
        # 写入新数据
        for item_b, score in similar_items.items():
            pipe.zadd(key, {str(item_b): score})
    
    pipe.execute()
    
    logger.info(f"相似度数据已写入Redis: {len(similarity)} 个物品")


def save_user_history(behaviors: pd.DataFrame, redis_config: dict):
    """保存用户历史到Redis"""
    r = redis.Redis(
        host=redis_config.get("host", "localhost"),
        port=redis_config.get("port", 6379),
        db=redis_config.get("db", 0),
        password=redis_config.get("password") or None,
        decode_responses=True,
    )
    
    # 按用户分组,获取历史行为
    user_history = behaviors.sort_values("timestamp").groupby("user_id")["event_id"].apply(list)
    
    pipe = r.pipeline()
    
    for user_id, history in user_history.items():
        key = f"user:{user_id}:history"
        
        pipe.delete(key)
        for event_id in history[-200:]:  # 只保留最近200条
            pipe.rpush(key, str(event_id))
    
    pipe.execute()
    
    logger.info(f"用户历史已写入Redis: {len(user_history)} 个用户")


def compute_hot_list(behaviors: pd.DataFrame, events: pd.DataFrame, redis_config: dict):
    """计算热门榜单"""
    r = redis.Redis(
        host=redis_config.get("host", "localhost"),
        port=redis_config.get("port", 6379),
        db=redis_config.get("db", 0),
        password=redis_config.get("password") or None,
        decode_responses=True,
    )
    
    # 计算热度分
    # hot_score = views + clicks * 3 + orders * 10
    view_counts = behaviors[behaviors["behavior_type"] == "view"].groupby("event_id").size()
    click_counts = behaviors[behaviors["behavior_type"] == "click"].groupby("event_id").size()
    order_counts = behaviors[behaviors["behavior_type"] == "order"].groupby("event_id").size()
    
    all_events = set(behaviors["event_id"].unique())
    hot_scores = {}
    
    for event_id in all_events:
        views = view_counts.get(event_id, 0)
        clicks = click_counts.get(event_id, 0)
        orders = order_counts.get(event_id, 0)
        
        hot_scores[event_id] = views + clicks * 3 + orders * 10
    
    # 写入全站热门
    pipe = r.pipeline()
    pipe.delete("hot:all")
    for event_id, score in hot_scores.items():
        pipe.zadd("hot:all", {str(event_id): score})
    
    # 写入城市热门
    event_city = dict(zip(events["event_id"], events["city"]))
    city_scores = defaultdict(dict)
    
    for event_id, score in hot_scores.items():
        city = event_city.get(event_id, "")
        if city:
            city_scores[city][event_id] = score
    
    for city, scores in city_scores.items():
        key = f"hot:city:{city}"
        pipe.delete(key)
        for event_id, score in scores.items():
            pipe.zadd(key, {str(event_id): score})
    
    pipe.execute()
    
    logger.info(f"热门榜单已写入Redis: 全站{len(hot_scores)}个, {len(city_scores)}个城市")


def save_event_features(events: pd.DataFrame, behaviors: pd.DataFrame, redis_config: dict):
    """保存物品特征到Redis"""
    r = redis.Redis(
        host=redis_config.get("host", "localhost"),
        port=redis_config.get("port", 6379),
        db=redis_config.get("db", 0),
        password=redis_config.get("password") or None,
        decode_responses=True,
    )
    
    # 统计热度
    view_counts = behaviors[behaviors["behavior_type"] == "view"].groupby("event_id").size()
    order_counts = behaviors[behaviors["behavior_type"] == "order"].groupby("event_id").size()
    
    pipe = r.pipeline()
    
    for _, event in events.iterrows():
        event_id = event["event_id"]
        key = f"event:feature:{event_id}"
        
        features = {
            "category": event["category_id"],
            "price": event["price"],
            "city_id": event["city_id"],
            "venue_id": hash(event["venue"]) % 1000,  # 简化
            "time_slot": 0,  # 简化
            "weekday": 0,  # 简化
            "hot_score": view_counts.get(event_id, 0) + order_counts.get(event_id, 0) * 10,
            "total_views": view_counts.get(event_id, 0),
            "total_orders": order_counts.get(event_id, 0),
            "days_to_start": 30,  # 简化
        }
        
        pipe.hset(key, mapping={k: str(v) for k, v in features.items()})
    
    pipe.execute()
    
    logger.info(f"物品特征已写入Redis: {len(events)} 个活动")


def main():
    """主函数"""
    logger.info("=" * 50)
    logger.info("计算相似度矩阵并同步到Redis")
    logger.info("=" * 50)
    
    # Redis配置
    redis_config = {
        "host": os.getenv("REDIS_HOST", "localhost"),
        "port": int(os.getenv("REDIS_PORT", 6379)),
        "db": 0,
        "password": os.getenv("REDIS_PASSWORD", ""),
    }
    
    # 加载数据
    logger.info("加载数据...")
    behaviors = load_behaviors()
    events = pd.read_csv("data/processed/events.csv")
    
    # 构建用户-物品矩阵
    logger.info("构建用户-物品矩阵...")
    user_items, item_users = build_user_item_matrix(behaviors)
    
    # 计算相似度
    logger.info("计算物品相似度...")
    similarity = compute_item_similarity(item_users, top_k=100)
    
    # 保存到Redis
    logger.info("保存相似度到Redis...")
    save_to_redis(similarity, redis_config)
    
    # 保存用户历史
    logger.info("保存用户历史到Redis...")
    save_user_history(behaviors, redis_config)
    
    # 计算热门榜单
    logger.info("计算热门榜单...")
    compute_hot_list(behaviors, events, redis_config)
    
    # 保存物品特征
    logger.info("保存物品特征...")
    save_event_features(events, behaviors, redis_config)
    
    logger.info("\n同步完成!")


if __name__ == "__main__":
    main()

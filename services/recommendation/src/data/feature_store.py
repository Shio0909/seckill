"""
特征存储
"""
import time
from typing import Dict, List

import redis
from loguru import logger


class FeatureStore:
    """
    特征存储
    
    负责:
    1. 存储用户/物品特征
    2. 记录用户行为
    3. 实时特征更新
    """

    def __init__(self, config):
        self.config = config
        self.redis_client = redis.Redis(
            host=config.redis.host,
            port=config.redis.port,
            db=config.redis.db,
            password=config.redis.password or None,
            decode_responses=True,
        )
        
        logger.info("FeatureStore 初始化完成")

    def record_behavior(
        self,
        user_id: int,
        event_id: int,
        behavior_type: str,
        timestamp: int = None,
    ):
        """
        记录用户行为
        
        Args:
            user_id: 用户ID
            event_id: 活动ID
            behavior_type: 行为类型 (view/click/order/favorite)
            timestamp: 时间戳
        """
        timestamp = timestamp or int(time.time())
        
        pipe = self.redis_client.pipeline()
        
        # 1. 记录用户历史
        history_key = f"user:{user_id}:history"
        pipe.lpush(history_key, str(event_id))
        pipe.ltrim(history_key, 0, 199)  # 保留最近200条
        
        # 2. 更新物品热度
        if behavior_type == "view":
            pipe.zincrby("hot:all", 1, str(event_id))
        elif behavior_type == "click":
            pipe.zincrby("hot:all", 3, str(event_id))
        elif behavior_type == "order":
            pipe.zincrby("hot:all", 10, str(event_id))
        
        # 3. 记录物品被哪些用户浏览(用于ItemCF)
        viewers_key = f"item:{event_id}:viewers"
        pipe.zadd(viewers_key, {str(user_id): timestamp})
        
        # 4. 更新用户行为计数
        profile_key = f"user:profile:{user_id}"
        pipe.hincrby(profile_key, "behavior_count", 1)
        pipe.hincrby(profile_key, f"{behavior_type}_count", 1)
        
        pipe.execute()
        
        logger.debug(f"记录行为: user={user_id}, event={event_id}, type={behavior_type}")

    def get_user_history(self, user_id: int, count: int = 100) -> List[int]:
        """获取用户历史"""
        key = f"user:{user_id}:history"
        history = self.redis_client.lrange(key, 0, count - 1)
        return [int(x) for x in history]

    def get_item_viewers(self, event_id: int, count: int = 1000) -> List[int]:
        """获取浏览过该物品的用户"""
        key = f"item:{event_id}:viewers"
        viewers = self.redis_client.zrevrange(key, 0, count - 1)
        return [int(x) for x in viewers]

    def save_event_feature(self, event_id: int, features: Dict):
        """
        保存物品特征
        
        Args:
            event_id: 活动ID
            features: 特征字典
        """
        key = f"event:feature:{event_id}"
        data = {k: str(v) for k, v in features.items()}
        self.redis_client.hset(key, mapping=data)

    def get_event_feature(self, event_id: int) -> Dict:
        """获取物品特征"""
        key = f"event:feature:{event_id}"
        return self.redis_client.hgetall(key)

    def batch_save_event_features(self, features_map: Dict[int, Dict]):
        """
        批量保存物品特征
        
        Args:
            features_map: {event_id: {feature_name: value}}
        """
        pipe = self.redis_client.pipeline()
        
        for event_id, features in features_map.items():
            key = f"event:feature:{event_id}"
            data = {k: str(v) for k, v in features.items()}
            pipe.hset(key, mapping=data)
        
        pipe.execute()
        
        logger.info(f"批量保存物品特征: {len(features_map)} 个")

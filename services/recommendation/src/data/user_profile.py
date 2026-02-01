"""
用户画像存储
"""
from typing import Dict, Optional

import redis
from loguru import logger


class UserProfileStore:
    """
    用户画像存储
    
    数据结构(Redis Hash):
    user:profile:{user_id} -> {
        "age_group": 3,
        "gender": 1,
        "city_id": 1,
        "behavior_count": 100,
        "view_count": 80,
        "click_count": 30,
        "order_count": 5,
        "avg_price": 200.0,
        "prefer_category": 1,
        "active_days": 30,
    }
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
        self.key_prefix = "user:profile:"
        
        logger.info("UserProfileStore 初始化完成")

    def get(self, user_id: int) -> Optional[Dict]:
        """
        获取用户画像
        
        Args:
            user_id: 用户ID
            
        Returns:
            用户画像字典,不存在返回None
        """
        key = f"{self.key_prefix}{user_id}"
        data = self.redis_client.hgetall(key)
        
        if not data:
            return None
        
        # 类型转换
        profile = {
            "user_id": user_id,
            "age_group": int(data.get("age_group", 0)),
            "gender": int(data.get("gender", 0)),
            "city_id": int(data.get("city_id", 0)),
            "behavior_count": int(data.get("behavior_count", 0)),
            "view_count": int(data.get("view_count", 0)),
            "click_count": int(data.get("click_count", 0)),
            "order_count": int(data.get("order_count", 0)),
            "avg_price": float(data.get("avg_price", 0)),
            "prefer_category": int(data.get("prefer_category", 0)),
            "active_days": int(data.get("active_days", 0)),
        }
        
        return profile

    def set(self, user_id: int, profile: Dict):
        """
        设置用户画像
        
        Args:
            user_id: 用户ID
            profile: 用户画像字典
        """
        key = f"{self.key_prefix}{user_id}"
        
        # 转换为字符串
        data = {k: str(v) for k, v in profile.items()}
        
        self.redis_client.hset(key, mapping=data)
        # 设置过期时间: 7天
        self.redis_client.expire(key, 7 * 24 * 3600)

    def refresh(self, user_id: int):
        """
        刷新用户画像(重新计算)
        
        实际应该从数据库统计用户行为
        """
        # 这里简化,实际应该:
        # 1. 从MySQL查询用户信息
        # 2. 从行为日志统计行为数据
        # 3. 计算用户画像特征
        # 4. 写入Redis
        
        logger.info(f"刷新用户画像: user_id={user_id}")
        
        # 模拟实现
        profile = self.get(user_id)
        if profile:
            # 重新计算
            self.set(user_id, profile)

    def incr_behavior(self, user_id: int, behavior_type: str):
        """
        增加行为计数
        
        Args:
            user_id: 用户ID
            behavior_type: 行为类型 (view/click/order)
        """
        key = f"{self.key_prefix}{user_id}"
        
        field_map = {
            "view": "view_count",
            "click": "click_count",
            "order": "order_count",
        }
        
        field = field_map.get(behavior_type)
        if field:
            self.redis_client.hincrby(key, field, 1)
            self.redis_client.hincrby(key, "behavior_count", 1)

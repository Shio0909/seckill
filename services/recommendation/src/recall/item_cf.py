"""
ItemCF协同过滤召回
"""
from typing import Dict, List, Optional

import redis
from loguru import logger

from .base_recall import BaseRecall


class ItemCFRecall(BaseRecall):
    """
    基于物品的协同过滤召回
    
    原理: 用户喜欢A物品,推荐与A相似的物品B
    
    数据结构(Redis):
    - cf:item:{item_id}:similar -> ZSet, 存储相似物品及分数
    - user:{user_id}:history -> List, 存储用户历史行为
    """

    def __init__(self, config):
        super().__init__(config)
        self.redis_client = redis.Redis(
            host=config.redis.host,
            port=config.redis.port,
            db=config.redis.db,
            password=config.redis.password or None,
            decode_responses=True,
        )
        self.similarity_key_prefix = "cf:item:"
        self.history_key_prefix = "user:"
        
        logger.info("ItemCFRecall 初始化完成")

    def recall(
        self,
        user_id: int,
        user_profile: Optional[Dict] = None,
        count: int = 100,
        **kwargs,
    ) -> List[Dict]:
        """
        ItemCF召回
        
        1. 获取用户历史行为物品
        2. 对每个历史物品,获取相似物品
        3. 去重合并,按分数排序
        """
        # 获取用户历史
        history = self._get_user_history(user_id)
        
        if not history:
            logger.debug(f"用户 {user_id} 无历史行为,ItemCF召回为空")
            return []
        
        # 收集相似物品
        similar_items = {}
        
        for item_id in history[:20]:  # 只取最近20个
            similar = self._get_similar_items_from_redis(item_id, 50)
            for event_id, score in similar:
                if event_id not in history:  # 排除已看过的
                    if event_id in similar_items:
                        similar_items[event_id] = max(similar_items[event_id], score)
                    else:
                        similar_items[event_id] = score
        
        # 排序
        sorted_items = sorted(
            similar_items.items(),
            key=lambda x: x[1],
            reverse=True,
        )[:count]
        
        # 构造返回结果
        result = [
            {
                "event_id": int(item_id),
                "score": float(score),
                "recall_source": "item_cf",
            }
            for item_id, score in sorted_items
        ]
        
        logger.debug(f"ItemCF召回 user_id={user_id}, count={len(result)}")
        
        return result

    def get_similar_items(self, item_id: int, count: int) -> List[Dict]:
        """获取相似物品"""
        similar = self._get_similar_items_from_redis(item_id, count)
        
        return [
            {
                "event_id": int(event_id),
                "score": float(score),
                "recall_source": "similar",
            }
            for event_id, score in similar
        ]

    def _get_user_history(self, user_id: int) -> List[int]:
        """获取用户历史行为"""
        key = f"{self.history_key_prefix}{user_id}:history"
        history = self.redis_client.lrange(key, 0, 100)
        return [int(x) for x in history]

    def _get_similar_items_from_redis(self, item_id: int, count: int) -> List[tuple]:
        """从Redis获取相似物品"""
        key = f"{self.similarity_key_prefix}{item_id}:similar"
        # ZREVRANGE返回分数从高到低的成员
        result = self.redis_client.zrevrange(key, 0, count - 1, withscores=True)
        return result

    def compute_similarity_offline(self, user_item_matrix: Dict):
        """
        离线计算物品相似度(由离线脚本调用)
        
        使用余弦相似度/Jaccard相似度
        结果写入Redis
        """
        from collections import defaultdict
        import math
        
        # 1. 构建倒排表: item -> [users]
        item_users = defaultdict(set)
        for user_id, items in user_item_matrix.items():
            for item_id in items:
                item_users[item_id].add(user_id)
        
        # 2. 计算物品相似度
        items = list(item_users.keys())
        
        for i, item_a in enumerate(items):
            similar_scores = []
            users_a = item_users[item_a]
            
            for item_b in items:
                if item_a == item_b:
                    continue
                    
                users_b = item_users[item_b]
                
                # Jaccard相似度
                intersection = len(users_a & users_b)
                if intersection == 0:
                    continue
                    
                union = len(users_a | users_b)
                similarity = intersection / union
                
                similar_scores.append((item_b, similarity))
            
            # 取Top100相似物品写入Redis
            similar_scores.sort(key=lambda x: x[1], reverse=True)
            top_similar = similar_scores[:100]
            
            if top_similar:
                key = f"{self.similarity_key_prefix}{item_a}:similar"
                # 写入ZSet
                pipe = self.redis_client.pipeline()
                pipe.delete(key)
                for item_id, score in top_similar:
                    pipe.zadd(key, {str(item_id): score})
                pipe.execute()
        
        logger.info(f"物品相似度计算完成,共 {len(items)} 个物品")

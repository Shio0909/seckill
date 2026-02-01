"""
热门榜单召回
"""
from typing import Dict, List, Optional

import redis
from loguru import logger

from .base_recall import BaseRecall


class HotRecall(BaseRecall):
    """
    热门榜单召回
    
    用于冷启动和兜底策略
    
    数据结构(Redis):
    - hot:all -> ZSet, 全站热门
    - hot:city:{city} -> ZSet, 城市热门
    - hot:category:{category} -> ZSet, 分类热门
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
        
        logger.info("HotRecall 初始化完成")

    def recall(
        self,
        user_id: int,
        user_profile: Optional[Dict] = None,
        count: int = 50,
        city: str = "",
        **kwargs,
    ) -> List[Dict]:
        """
        热门召回
        
        优先返回城市热门,不足时补充全站热门
        """
        result = []
        
        # 1. 城市热门
        if city:
            city_hot = self._get_hot_list(f"hot:city:{city}", count)
            result.extend(city_hot)
        
        # 2. 全站热门补充
        if len(result) < count:
            all_hot = self._get_hot_list("hot:all", count - len(result))
            
            # 去重
            existing_ids = {item["event_id"] for item in result}
            for item in all_hot:
                if item["event_id"] not in existing_ids:
                    result.append(item)
        
        logger.debug(f"HotRecall: user_id={user_id}, city={city}, count={len(result)}")
        
        return result[:count]

    def _get_hot_list(self, key: str, count: int) -> List[Dict]:
        """从Redis获取热门列表"""
        items = self.redis_client.zrevrange(key, 0, count - 1, withscores=True)
        
        return [
            {
                "event_id": int(event_id),
                "score": float(score),
                "recall_source": "hot",
            }
            for event_id, score in items
        ]

    def update_hot_score(self, event_id: int, score_delta: float, city: str = ""):
        """
        更新热度分数(实时更新)
        
        Args:
            event_id: 活动ID
            score_delta: 分数增量
            city: 城市
        """
        pipe = self.redis_client.pipeline()
        
        # 更新全站热门
        pipe.zincrby("hot:all", score_delta, str(event_id))
        
        # 更新城市热门
        if city:
            pipe.zincrby(f"hot:city:{city}", score_delta, str(event_id))
        
        pipe.execute()

    def rebuild_hot_list_offline(self, event_scores: Dict[int, float], city_map: Dict[int, str]):
        """
        离线重建热门列表
        
        Args:
            event_scores: {event_id: hot_score}
            city_map: {event_id: city}
        """
        from collections import defaultdict
        
        # 按城市分组
        city_scores = defaultdict(dict)
        
        for event_id, score in event_scores.items():
            city = city_map.get(event_id, "")
            if city:
                city_scores[city][event_id] = score
        
        pipe = self.redis_client.pipeline()
        
        # 写入全站热门
        pipe.delete("hot:all")
        for event_id, score in event_scores.items():
            pipe.zadd("hot:all", {str(event_id): score})
        
        # 写入城市热门
        for city, scores in city_scores.items():
            key = f"hot:city:{city}"
            pipe.delete(key)
            for event_id, score in scores.items():
                pipe.zadd(key, {str(event_id): score})
        
        pipe.execute()
        
        logger.info(f"热门列表重建完成: {len(event_scores)} 个活动, {len(city_scores)} 个城市")

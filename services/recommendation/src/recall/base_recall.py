"""
召回基类
"""
from abc import ABC, abstractmethod
from typing import Dict, List, Optional


class BaseRecall(ABC):
    """召回器基类"""

    def __init__(self, config):
        self.config = config
        self.name = self.__class__.__name__

    @abstractmethod
    def recall(
        self,
        user_id: int,
        user_profile: Optional[Dict] = None,
        count: int = 100,
        **kwargs,
    ) -> List[Dict]:
        """
        执行召回
        
        Args:
            user_id: 用户ID
            user_profile: 用户画像
            count: 召回数量
            
        Returns:
            召回结果列表 [{"event_id": xx, "score": xx, "recall_source": xx}, ...]
        """
        pass

    def get_similar_items(self, item_id: int, count: int) -> List[Dict]:
        """
        获取相似物品(可选实现)
        """
        return []

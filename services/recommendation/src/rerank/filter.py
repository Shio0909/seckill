"""
业务规则过滤
"""
import time
from typing import Dict, List

from loguru import logger


class BusinessFilter:
    """
    业务规则过滤器
    
    负责:
    1. 过滤下架/无效活动
    2. 过滤库存不足
    3. 过滤过期活动
    4. 运营干预(置顶/降权)
    """

    def __init__(self):
        # 黑名单(可从配置中心加载)
        self.blacklist = set()
        
        # 置顶列表
        self.pinned = {}  # event_id -> priority
        
        # 降权列表
        self.demoted = set()

    def filter(self, items: List[Dict]) -> List[Dict]:
        """
        应用业务规则过滤
        
        Args:
            items: 候选列表
            
        Returns:
            过滤后的列表
        """
        result = []
        
        for item in items:
            event_id = item["event_id"]
            
            # 1. 黑名单过滤
            if event_id in self.blacklist:
                continue
            
            # 2. 降权处理
            if event_id in self.demoted:
                item["score"] *= 0.5
            
            result.append(item)
        
        # 3. 置顶处理
        result = self._apply_pinned(result)
        
        return result

    def _apply_pinned(self, items: List[Dict]) -> List[Dict]:
        """应用置顶规则"""
        if not self.pinned:
            return items
        
        pinned_items = []
        normal_items = []
        
        for item in items:
            if item["event_id"] in self.pinned:
                item["_pin_priority"] = self.pinned[item["event_id"]]
                pinned_items.append(item)
            else:
                normal_items.append(item)
        
        # 置顶物品按优先级排序
        pinned_items.sort(key=lambda x: x.get("_pin_priority", 0), reverse=True)
        
        # 清理临时字段
        for item in pinned_items:
            item.pop("_pin_priority", None)
        
        return pinned_items + normal_items

    def add_to_blacklist(self, event_id: int):
        """添加到黑名单"""
        self.blacklist.add(event_id)
        logger.info(f"添加黑名单: {event_id}")

    def remove_from_blacklist(self, event_id: int):
        """从黑名单移除"""
        self.blacklist.discard(event_id)
        logger.info(f"移除黑名单: {event_id}")

    def pin(self, event_id: int, priority: int = 1):
        """置顶活动"""
        self.pinned[event_id] = priority
        logger.info(f"置顶活动: {event_id}, priority={priority}")

    def unpin(self, event_id: int):
        """取消置顶"""
        self.pinned.pop(event_id, None)
        logger.info(f"取消置顶: {event_id}")

    def demote(self, event_id: int):
        """降权活动"""
        self.demoted.add(event_id)
        logger.info(f"降权活动: {event_id}")

    def undemote(self, event_id: int):
        """取消降权"""
        self.demoted.discard(event_id)
        logger.info(f"取消降权: {event_id}")

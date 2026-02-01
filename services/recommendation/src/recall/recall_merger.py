"""
召回合并器
"""
from typing import Dict, List, Optional

from loguru import logger

from .base_recall import BaseRecall


class RecallMerger:
    """
    多路召回合并器
    
    支持:
    1. 注册多个召回器
    2. 并行执行召回
    3. 结果去重合并
    4. 按权重排序
    """

    def __init__(self, config):
        self.config = config
        self.recallers: Dict[str, BaseRecall] = {}
        self.recall_config = {
            "item_cf": {"weight": 0.30, "count": 100},
            "user_cf": {"weight": 0.20, "count": 100},
            "hot": {"weight": 0.15, "count": 50},
            "vector": {"weight": 0.25, "count": 100},
            "lbs": {"weight": 0.05, "count": 50},
            "tag": {"weight": 0.05, "count": 100},
        }
        
        logger.info("RecallMerger 初始化完成")

    def register(self, name: str, recaller: BaseRecall):
        """注册召回器"""
        self.recallers[name] = recaller
        logger.info(f"注册召回器: {name}")

    def get_recaller(self, name: str) -> Optional[BaseRecall]:
        """获取召回器"""
        return self.recallers.get(name)

    def recall(
        self,
        user_id: int,
        user_profile: Optional[Dict] = None,
        exclude_ids: List[int] = None,
        city: str = "",
    ) -> List[Dict]:
        """
        执行多路召回并合并
        
        Args:
            user_id: 用户ID
            user_profile: 用户画像
            exclude_ids: 排除的ID列表
            city: 城市
            
        Returns:
            合并后的召回结果
        """
        exclude_ids = set(exclude_ids or [])
        
        # 根据用户状态决定召回策略
        strategy = self._get_recall_strategy(user_profile)
        
        # 收集各路召回结果
        all_results = {}  # event_id -> {"score": xx, "sources": [...]}
        
        for recall_name in strategy:
            recaller = self.recallers.get(recall_name)
            if recaller is None:
                continue
            
            recall_cfg = self.recall_config.get(recall_name, {"weight": 0.1, "count": 50})
            
            try:
                items = recaller.recall(
                    user_id=user_id,
                    user_profile=user_profile,
                    count=recall_cfg["count"],
                    city=city,
                )
                
                weight = recall_cfg["weight"]
                
                for item in items:
                    event_id = item["event_id"]
                    
                    if event_id in exclude_ids:
                        continue
                    
                    weighted_score = item["score"] * weight
                    
                    if event_id in all_results:
                        # 多路召回到同一物品,分数累加
                        all_results[event_id]["score"] += weighted_score
                        all_results[event_id]["sources"].append(recall_name)
                    else:
                        all_results[event_id] = {
                            "event_id": event_id,
                            "score": weighted_score,
                            "sources": [recall_name],
                        }
                        
            except Exception as e:
                logger.error(f"召回器 {recall_name} 执行失败: {e}")
                continue
        
        # 按分数排序
        sorted_results = sorted(
            all_results.values(),
            key=lambda x: x["score"],
            reverse=True,
        )
        
        # 构造返回结果
        result = [
            {
                "event_id": item["event_id"],
                "score": item["score"],
                "recall_source": ",".join(item["sources"]),
            }
            for item in sorted_results
        ]
        
        logger.info(f"多路召回完成: user_id={user_id}, "
                   f"策略={strategy}, 结果数={len(result)}")
        
        return result

    def _get_recall_strategy(self, user_profile: Optional[Dict]) -> List[str]:
        """
        根据用户状态决定召回策略
        
        冷启动用户: 热门 + 地理位置
        普通用户: 协同过滤 + 向量 + 热门
        """
        if user_profile is None:
            return ["hot"]
        
        behavior_count = user_profile.get("behavior_count", 0)
        
        if behavior_count < 5:
            # 冷启动: 热门为主
            return ["hot", "tag"]
        elif behavior_count < 20:
            # 新用户: 热门 + 协同过滤
            return ["hot", "item_cf"]
        else:
            # 老用户: 全量召回
            return ["item_cf", "hot", "vector"]

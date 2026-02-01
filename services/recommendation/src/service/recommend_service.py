"""
推荐业务服务
"""
import random
from typing import Dict, List, Optional, Tuple

from loguru import logger

from recall.recall_merger import RecallMerger
from recall.item_cf import ItemCFRecall
from recall.hot_recall import HotRecall
from recall.vector_recall import VectorRecall
from ranking.lgb_ranker import LightGBMRanker
from ranking.feature_builder import FeatureBuilder
from rerank.diversifier import Diversifier
from rerank.filter import BusinessFilter
from data.user_profile import UserProfileStore
from data.feature_store import FeatureStore


class RecommendService:
    """推荐服务核心逻辑"""

    def __init__(self, config):
        self.config = config
        
        # 初始化各组件
        self._init_components()
        
        logger.info("RecommendService 初始化完成")

    def _init_components(self):
        """初始化各组件"""
        # 数据存储
        self.user_profile = UserProfileStore(self.config)
        self.feature_store = FeatureStore(self.config)
        
        # 召回层
        self.recall_merger = RecallMerger(self.config)
        self.recall_merger.register("item_cf", ItemCFRecall(self.config))
        self.recall_merger.register("hot", HotRecall(self.config))
        # self.recall_merger.register("vector", VectorRecall(self.config))
        
        # 排序层
        self.feature_builder = FeatureBuilder(self.config)
        self.ranker = LightGBMRanker(self.config)
        
        # 重排层
        self.diversifier = Diversifier()
        self.filter = BusinessFilter()

    def recommend(
        self,
        user_id: int,
        scene: str = "home",
        count: int = 10,
        city: str = "",
        device_type: str = "",
        exclude_ids: List[int] = None,
    ) -> Tuple[List[Dict], str]:
        """
        获取个性化推荐
        
        Args:
            user_id: 用户ID
            scene: 场景
            count: 返回数量
            city: 城市
            device_type: 设备类型
            exclude_ids: 排除的ID列表
            
        Returns:
            (推荐列表, AB分组)
        """
        exclude_ids = exclude_ids or []
        
        # 1. 获取用户画像
        user_profile = self.user_profile.get(user_id)
        
        # 2. AB测试分组
        ab_group = self._get_ab_group(user_id)
        
        # 3. 召回
        recall_result = self.recall_merger.recall(
            user_id=user_id,
            user_profile=user_profile,
            exclude_ids=exclude_ids,
            city=city,
        )
        
        logger.debug(f"召回数量: {len(recall_result)}")
        
        if not recall_result:
            # 降级: 返回热门
            return self._fallback_hot(count), ab_group
        
        # 4. 构建特征
        features = self.feature_builder.build(
            user_id=user_id,
            user_profile=user_profile,
            candidates=recall_result,
            context={"city": city, "device_type": device_type, "scene": scene},
        )
        
        # 5. 排序
        ranked_result = self.ranker.rank(recall_result, features)
        
        logger.debug(f"排序后数量: {len(ranked_result)}")
        
        # 6. 重排
        # 6.1 业务过滤
        filtered_result = self.filter.filter(ranked_result)
        
        # 6.2 多样性控制
        diversified_result = self.diversifier.diversify(filtered_result, lambda_param=0.5)
        
        # 7. 截断返回
        final_result = diversified_result[:count]
        
        return final_result, ab_group

    def get_similar(
        self,
        event_id: int,
        count: int = 10,
        user_id: Optional[int] = None,
    ) -> List[Dict]:
        """
        获取相似活动
        
        Args:
            event_id: 活动ID
            count: 返回数量
            user_id: 用户ID(可选,用于个性化)
            
        Returns:
            相似活动列表
        """
        # 使用ItemCF获取相似物品
        item_cf = self.recall_merger.get_recaller("item_cf")
        if item_cf:
            similar_items = item_cf.get_similar_items(event_id, count * 2)
        else:
            similar_items = []
        
        # 如果有用户ID,进行个性化排序
        if user_id and similar_items:
            user_profile = self.user_profile.get(user_id)
            features = self.feature_builder.build(
                user_id=user_id,
                user_profile=user_profile,
                candidates=similar_items,
                context={},
            )
            similar_items = self.ranker.rank(similar_items, features)
        
        return similar_items[:count]

    def record_behavior(
        self,
        user_id: int,
        event_id: int,
        behavior_type: str,
        timestamp: int,
    ):
        """记录用户行为"""
        self.feature_store.record_behavior(
            user_id=user_id,
            event_id=event_id,
            behavior_type=behavior_type,
            timestamp=timestamp,
        )

    def refresh_user_profile(self, user_id: int):
        """刷新用户画像"""
        self.user_profile.refresh(user_id)

    def _get_ab_group(self, user_id: int) -> str:
        """获取AB测试分组"""
        # 简单的哈希分组
        group_id = user_id % 10
        if group_id < 5:
            return "control"  # 对照组
        else:
            return "experiment"  # 实验组

    def _fallback_hot(self, count: int) -> List[Dict]:
        """降级: 返回热门列表"""
        hot_recall = self.recall_merger.get_recaller("hot")
        if hot_recall:
            return hot_recall.recall(user_id=0, count=count)
        return []

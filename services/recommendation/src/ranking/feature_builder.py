"""
特征工程
"""
from typing import Dict, List, Optional

import numpy as np
import redis
from loguru import logger


class FeatureBuilder:
    """
    特征构建器
    
    负责构建排序模型所需的特征:
    1. 用户特征
    2. 物品特征
    3. 交叉特征
    4. 上下文特征
    """

    # 特征定义
    FEATURE_COLUMNS = {
        # 用户特征 (10维)
        "user_features": [
            "user_age",           # 年龄段 (0-5)
            "user_gender",        # 性别 (0/1)
            "user_city_id",       # 城市ID
            "user_behavior_cnt",  # 历史行为数
            "user_view_cnt",      # 浏览数
            "user_click_cnt",     # 点击数
            "user_order_cnt",     # 订单数
            "user_avg_price",     # 平均消费
            "user_prefer_category",  # 偏好类别
            "user_active_days",   # 活跃天数
        ],
        
        # 物品特征 (10维)
        "item_features": [
            "event_category",     # 类别ID
            "event_price",        # 票价
            "event_city_id",      # 城市ID
            "event_venue_id",     # 场馆ID
            "event_time_slot",    # 时间段
            "event_weekday",      # 周末/工作日
            "event_hot_score",    # 热度分
            "event_total_views",  # 总浏览数
            "event_total_orders", # 总订单数
            "event_days_to_start",  # 距开始天数
        ],
        
        # 交叉特征 (6维)
        "cross_features": [
            "user_event_city_match",    # 城市匹配
            "user_category_preference", # 类别偏好匹配度
            "user_price_match",         # 价格区间匹配
            "user_event_sim_score",     # 用户-物品相似度
            "history_same_venue",       # 历史同场馆
            "history_same_category",    # 历史同类别
        ],
        
        # 上下文特征 (4维)
        "context_features": [
            "request_hour",       # 请求小时
            "request_weekday",    # 请求星期
            "device_type",        # 设备类型
            "scene_type",         # 场景类型
        ],
    }

    def __init__(self, config):
        self.config = config
        self.redis_client = redis.Redis(
            host=config.redis.host,
            port=config.redis.port,
            db=config.redis.db,
            password=config.redis.password or None,
            decode_responses=True,
        )
        
        # 计算总特征维度
        self.feature_dim = sum(
            len(cols) for cols in self.FEATURE_COLUMNS.values()
        )
        
        logger.info(f"FeatureBuilder 初始化完成, 特征维度: {self.feature_dim}")

    def build(
        self,
        user_id: int,
        user_profile: Optional[Dict],
        candidates: List[Dict],
        context: Dict,
    ) -> np.ndarray:
        """
        构建特征矩阵
        
        Args:
            user_id: 用户ID
            user_profile: 用户画像
            candidates: 候选列表
            context: 上下文信息
            
        Returns:
            特征矩阵 [N, feature_dim]
        """
        n = len(candidates)
        features = np.zeros((n, self.feature_dim), dtype=np.float32)
        
        # 1. 获取用户特征 (所有候选共享)
        user_features = self._get_user_features(user_id, user_profile)
        
        # 2. 获取上下文特征 (所有候选共享)
        context_features = self._get_context_features(context)
        
        # 3. 批量获取物品特征
        event_ids = [c["event_id"] for c in candidates]
        item_features_batch = self._get_item_features_batch(event_ids)
        
        # 4. 构建每个候选的特征
        for i, candidate in enumerate(candidates):
            event_id = candidate["event_id"]
            item_features = item_features_batch.get(event_id, self._get_default_item_features())
            
            # 交叉特征
            cross_features = self._get_cross_features(user_profile, item_features)
            
            # 拼接特征
            feature_vector = np.concatenate([
                user_features,
                item_features,
                cross_features,
                context_features,
            ])
            
            features[i] = feature_vector
        
        return features

    def _get_user_features(self, user_id: int, user_profile: Optional[Dict]) -> np.ndarray:
        """获取用户特征"""
        if user_profile is None:
            return np.zeros(10, dtype=np.float32)
        
        return np.array([
            user_profile.get("age_group", 0),
            user_profile.get("gender", 0),
            user_profile.get("city_id", 0),
            user_profile.get("behavior_count", 0),
            user_profile.get("view_count", 0),
            user_profile.get("click_count", 0),
            user_profile.get("order_count", 0),
            user_profile.get("avg_price", 0),
            user_profile.get("prefer_category", 0),
            user_profile.get("active_days", 0),
        ], dtype=np.float32)

    def _get_item_features_batch(self, event_ids: List[int]) -> Dict[int, np.ndarray]:
        """批量获取物品特征"""
        result = {}
        
        # 从Redis批量获取
        pipe = self.redis_client.pipeline()
        for event_id in event_ids:
            pipe.hgetall(f"event:feature:{event_id}")
        
        features_list = pipe.execute()
        
        for event_id, features in zip(event_ids, features_list):
            if features:
                result[event_id] = np.array([
                    float(features.get("category", 0)),
                    float(features.get("price", 0)),
                    float(features.get("city_id", 0)),
                    float(features.get("venue_id", 0)),
                    float(features.get("time_slot", 0)),
                    float(features.get("weekday", 0)),
                    float(features.get("hot_score", 0)),
                    float(features.get("total_views", 0)),
                    float(features.get("total_orders", 0)),
                    float(features.get("days_to_start", 0)),
                ], dtype=np.float32)
            else:
                result[event_id] = self._get_default_item_features()
        
        return result

    def _get_default_item_features(self) -> np.ndarray:
        """默认物品特征"""
        return np.zeros(10, dtype=np.float32)

    def _get_cross_features(
        self,
        user_profile: Optional[Dict],
        item_features: np.ndarray,
    ) -> np.ndarray:
        """获取交叉特征"""
        if user_profile is None:
            return np.zeros(6, dtype=np.float32)
        
        # 城市匹配
        user_city = user_profile.get("city_id", 0)
        item_city = item_features[2] if len(item_features) > 2 else 0
        city_match = 1.0 if user_city == item_city else 0.0
        
        # 类别偏好匹配
        user_prefer = user_profile.get("prefer_category", 0)
        item_category = item_features[0] if len(item_features) > 0 else 0
        category_match = 1.0 if user_prefer == item_category else 0.0
        
        # 价格匹配
        user_avg_price = user_profile.get("avg_price", 0)
        item_price = item_features[1] if len(item_features) > 1 else 0
        price_match = 1.0 - min(abs(user_avg_price - item_price) / max(user_avg_price, 1), 1.0)
        
        return np.array([
            city_match,
            category_match,
            price_match,
            0.0,  # user_event_sim_score (需要额外计算)
            0.0,  # history_same_venue
            0.0,  # history_same_category
        ], dtype=np.float32)

    def _get_context_features(self, context: Dict) -> np.ndarray:
        """获取上下文特征"""
        import datetime
        
        now = datetime.datetime.now()
        
        # 设备类型映射
        device_map = {"ios": 0, "android": 1, "web": 2, "mini": 3}
        device = context.get("device_type", "web")
        device_id = device_map.get(device.lower(), 2)
        
        # 场景类型映射
        scene_map = {"home": 0, "search": 1, "detail": 2, "cart": 3}
        scene = context.get("scene", "home")
        scene_id = scene_map.get(scene.lower(), 0)
        
        return np.array([
            now.hour,
            now.weekday(),
            device_id,
            scene_id,
        ], dtype=np.float32)

    def save_item_features(self, event_id: int, features: Dict):
        """
        保存物品特征到Redis(离线脚本调用)
        """
        key = f"event:feature:{event_id}"
        self.redis_client.hset(key, mapping={k: str(v) for k, v in features.items()})

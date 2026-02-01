"""
gRPC服务实现
"""
import time
import uuid
from typing import List

from loguru import logger

# 导入protobuf
import sys
import os
sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "..", "proto"))
import recommendation_pb2
import recommendation_pb2_grpc

from service.recommend_service import RecommendService


class RecommendationServicer(recommendation_pb2_grpc.RecommendationServiceServicer):
    """推荐服务gRPC实现"""

    def __init__(self, config):
        self.config = config
        self.recommend_service = RecommendService(config)
        self.version = "1.0.0"
        logger.info("RecommendationServicer 初始化完成")

    def GetRecommendations(self, request, context):
        """获取个性化推荐"""
        trace_id = str(uuid.uuid4())[:8]
        start_time = time.time()
        
        logger.info(f"[{trace_id}] 推荐请求: user_id={request.user_id}, "
                   f"scene={request.scene}, count={request.count}")
        
        try:
            # 调用推荐服务
            items, ab_group = self.recommend_service.recommend(
                user_id=request.user_id,
                scene=request.scene,
                count=request.count,
                city=request.city,
                device_type=request.device_type,
                exclude_ids=list(request.exclude_ids),
            )
            
            # 构造响应
            response = recommendation_pb2.RecommendResponse(
                code=0,
                message="success",
                trace_id=trace_id,
                ab_group=ab_group,
            )
            
            for item in items:
                rec_item = recommendation_pb2.RecommendItem(
                    event_id=item["event_id"],
                    score=item["score"],
                    recall_source=item.get("recall_source", ""),
                )
                # 添加调试信息
                for k, v in item.get("debug_info", {}).items():
                    rec_item.debug_info[k] = v
                response.items.append(rec_item)
            
            elapsed = (time.time() - start_time) * 1000
            logger.info(f"[{trace_id}] 推荐完成: count={len(items)}, elapsed={elapsed:.2f}ms")
            
            return response
            
        except Exception as e:
            logger.error(f"[{trace_id}] 推荐失败: {e}")
            return recommendation_pb2.RecommendResponse(
                code=-1,
                message=str(e),
                trace_id=trace_id,
            )

    def GetSimilarEvents(self, request, context):
        """获取相似活动"""
        trace_id = str(uuid.uuid4())[:8]
        
        logger.info(f"[{trace_id}] 相似推荐: event_id={request.event_id}")
        
        try:
            items = self.recommend_service.get_similar(
                event_id=request.event_id,
                count=request.count,
                user_id=request.user_id if request.user_id else None,
            )
            
            response = recommendation_pb2.SimilarResponse(
                code=0,
                message="success",
            )
            
            for item in items:
                rec_item = recommendation_pb2.RecommendItem(
                    event_id=item["event_id"],
                    score=item["score"],
                    recall_source="similar",
                )
                response.items.append(rec_item)
            
            return response
            
        except Exception as e:
            logger.error(f"[{trace_id}] 相似推荐失败: {e}")
            return recommendation_pb2.SimilarResponse(
                code=-1,
                message=str(e),
            )

    def RecordBehavior(self, request, context):
        """记录用户行为"""
        try:
            self.recommend_service.record_behavior(
                user_id=request.user_id,
                event_id=request.event_id,
                behavior_type=request.behavior_type,
                timestamp=request.timestamp,
            )
            
            return recommendation_pb2.BehaviorResponse(
                code=0,
                message="success",
            )
            
        except Exception as e:
            logger.error(f"记录行为失败: {e}")
            return recommendation_pb2.BehaviorResponse(
                code=-1,
                message=str(e),
            )

    def RefreshUserProfile(self, request, context):
        """刷新用户画像"""
        try:
            self.recommend_service.refresh_user_profile(request.user_id)
            
            return recommendation_pb2.RefreshResponse(
                code=0,
                message="success",
            )
            
        except Exception as e:
            logger.error(f"刷新画像失败: {e}")
            return recommendation_pb2.RefreshResponse(
                code=-1,
                message=str(e),
            )

    def HealthCheck(self, request, context):
        """健康检查"""
        return recommendation_pb2.HealthResponse(
            healthy=True,
            version=self.version,
        )

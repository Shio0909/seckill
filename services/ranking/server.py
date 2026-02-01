"""
排序服务 - Python实现
只负责CTR模型推理，不负责召回和重排
"""
import os
import time
import pickle
from concurrent import futures
from typing import List

import grpc
import numpy as np
from loguru import logger

# gRPC生成的代码
import ranking_pb2
import ranking_pb2_grpc


class RankingServicer(ranking_pb2_grpc.RankingServiceServicer):
    """排序服务实现"""
    
    def __init__(self, model_path: str = None):
        self.model = None
        self.model_version = "v1.0"
        
        if model_path is None:
            model_path = os.getenv("MODEL_PATH", "models/lgb_ctr_v1.pkl")
        
        self._load_model(model_path)
    
    def _load_model(self, path: str):
        """加载模型"""
        try:
            if os.path.exists(path):
                with open(path, "rb") as f:
                    self.model = pickle.load(f)
                logger.info(f"模型加载成功: {path}")
            else:
                logger.warning(f"模型文件不存在: {path}, 将使用召回分数")
        except Exception as e:
            logger.error(f"模型加载失败: {e}")
    
    def Rank(self, request: ranking_pb2.RankRequest, context) -> ranking_pb2.RankResponse:
        """执行排序"""
        start_time = time.time()
        
        items = list(request.items)
        
        if len(items) == 0:
            return ranking_pb2.RankResponse(items=[], latency_ms=0)
        
        # 提取特征矩阵
        features = []
        for item in items:
            if len(item.features) > 0:
                features.append(list(item.features))
            else:
                features.append([0.0] * 16)  # 默认特征维度
        
        features_array = np.array(features, dtype=np.float32)
        
        # 模型预测
        if self.model is not None:
            try:
                scores = self.model.predict(features_array)
            except Exception as e:
                logger.error(f"模型预测失败: {e}")
                scores = [item.recall_score for item in items]
        else:
            # 无模型时使用召回分数
            scores = [item.recall_score for item in items]
        
        # 填充排序分数
        result_items = []
        for i, item in enumerate(items):
            result_item = ranking_pb2.RankItem(
                event_id=item.event_id,
                recall_score=item.recall_score,
                recall_source=item.recall_source,
                features=item.features,
                rank_score=float(scores[i]),
            )
            result_items.append(result_item)
        
        # 按分数排序
        result_items.sort(key=lambda x: x.rank_score, reverse=True)
        
        latency = int((time.time() - start_time) * 1000)
        
        return ranking_pb2.RankResponse(
            items=result_items,
            latency_ms=latency,
        )
    
    def HealthCheck(self, request, context) -> ranking_pb2.HealthResponse:
        """健康检查"""
        status = "healthy" if self.model is not None else "degraded"
        return ranking_pb2.HealthResponse(
            status=status,
            model_version=self.model_version,
        )


def serve(port: int = 50052):
    """启动服务"""
    server = grpc.server(
        futures.ThreadPoolExecutor(max_workers=10),
        options=[
            ("grpc.max_send_message_length", 50 * 1024 * 1024),
            ("grpc.max_receive_message_length", 50 * 1024 * 1024),
        ],
    )
    
    ranking_pb2_grpc.add_RankingServiceServicer_to_server(
        RankingServicer(), server
    )
    
    server.add_insecure_port(f"[::]:{port}")
    server.start()
    
    logger.info(f"排序服务启动: port={port}")
    
    try:
        server.wait_for_termination()
    except KeyboardInterrupt:
        server.stop(0)
        logger.info("服务已停止")


if __name__ == "__main__":
    import sys
    
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 50052
    serve(port)

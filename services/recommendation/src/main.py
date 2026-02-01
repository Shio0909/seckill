"""
推荐系统服务入口
"""
import os
import sys
from concurrent import futures

import grpc
from loguru import logger

# 添加src到路径
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from config import get_config
from server.grpc_server import RecommendationServicer

# 导入生成的protobuf代码
sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "proto"))
import recommendation_pb2_grpc


def configure_logging():
    """配置日志"""
    logger.remove()
    logger.add(
        sys.stdout,
        format="<green>{time:YYYY-MM-DD HH:mm:ss}</green> | "
               "<level>{level: <8}</level> | "
               "<cyan>{name}</cyan>:<cyan>{function}</cyan>:<cyan>{line}</cyan> - "
               "<level>{message}</level>",
        level="INFO",
    )
    logger.add(
        "logs/recommendation.log",
        rotation="100 MB",
        retention="7 days",
        level="DEBUG",
    )


def serve():
    """启动gRPC服务"""
    config = get_config()
    
    # 创建gRPC服务器
    server = grpc.server(
        futures.ThreadPoolExecutor(max_workers=config.server.max_workers),
        options=[
            ("grpc.max_send_message_length", 50 * 1024 * 1024),
            ("grpc.max_receive_message_length", 50 * 1024 * 1024),
        ],
    )
    
    # 注册服务
    servicer = RecommendationServicer(config)
    recommendation_pb2_grpc.add_RecommendationServiceServicer_to_server(servicer, server)
    
    # 启动服务
    address = f"{config.server.host}:{config.server.port}"
    server.add_insecure_port(address)
    server.start()
    
    logger.info(f"🚀 推荐服务已启动: {address}")
    logger.info(f"📊 召回配置: ItemCF={config.recall.item_cf['weight']}, "
                f"UserCF={config.recall.user_cf['weight']}, "
                f"Vector={config.recall.vector['weight']}")
    logger.info(f"🎯 排序模型: {config.ranking.model_type}")
    
    try:
        server.wait_for_termination()
    except KeyboardInterrupt:
        logger.info("服务正在关闭...")
        server.stop(grace=5)
        logger.info("服务已关闭")


if __name__ == "__main__":
    configure_logging()
    serve()

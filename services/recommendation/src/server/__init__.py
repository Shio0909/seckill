"""gRPC服务模块"""
from .grpc_server import RecommendationServicer, serve

__all__ = [
    "RecommendationServicer",
    "serve",
]

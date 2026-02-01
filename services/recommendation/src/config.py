"""
推荐系统配置
"""
import os
from dataclasses import dataclass, field
from typing import Dict, List

import yaml
from dotenv import load_dotenv

load_dotenv()


@dataclass
class RedisConfig:
    host: str = "localhost"
    port: int = 6379
    db: int = 0
    password: str = ""


@dataclass
class MilvusConfig:
    host: str = "localhost"
    port: int = 19530
    collection: str = "event_embeddings"


@dataclass
class MySQLConfig:
    host: str = "localhost"
    port: int = 3306
    user: str = "root"
    password: str = ""
    database: str = "seckill"


@dataclass
class RecallConfig:
    """召回配置"""
    item_cf: Dict = field(default_factory=lambda: {"weight": 0.30, "count": 100})
    user_cf: Dict = field(default_factory=lambda: {"weight": 0.20, "count": 100})
    hot: Dict = field(default_factory=lambda: {"weight": 0.15, "count": 50})
    vector: Dict = field(default_factory=lambda: {"weight": 0.25, "count": 100})
    lbs: Dict = field(default_factory=lambda: {"weight": 0.05, "count": 50})
    tag: Dict = field(default_factory=lambda: {"weight": 0.05, "count": 100})


@dataclass
class RankingConfig:
    """排序配置"""
    model_type: str = "lightgbm"  # lightgbm / deepfm
    model_path: str = "models/lgb_ctr_v1.pkl"
    feature_dim: int = 64


@dataclass
class ServerConfig:
    """服务配置"""
    host: str = "0.0.0.0"
    port: int = 50052
    max_workers: int = 10


@dataclass
class Config:
    redis: RedisConfig = field(default_factory=RedisConfig)
    milvus: MilvusConfig = field(default_factory=MilvusConfig)
    mysql: MySQLConfig = field(default_factory=MySQLConfig)
    recall: RecallConfig = field(default_factory=RecallConfig)
    ranking: RankingConfig = field(default_factory=RankingConfig)
    server: ServerConfig = field(default_factory=ServerConfig)

    @classmethod
    def from_yaml(cls, path: str) -> "Config":
        """从YAML文件加载配置"""
        with open(path, "r", encoding="utf-8") as f:
            data = yaml.safe_load(f)
        return cls._from_dict(data)

    @classmethod
    def from_env(cls) -> "Config":
        """从环境变量加载配置"""
        return cls(
            redis=RedisConfig(
                host=os.getenv("REDIS_HOST", "localhost"),
                port=int(os.getenv("REDIS_PORT", 6379)),
                password=os.getenv("REDIS_PASSWORD", ""),
            ),
            milvus=MilvusConfig(
                host=os.getenv("MILVUS_HOST", "localhost"),
                port=int(os.getenv("MILVUS_PORT", 19530)),
            ),
            mysql=MySQLConfig(
                host=os.getenv("MYSQL_HOST", "localhost"),
                port=int(os.getenv("MYSQL_PORT", 3306)),
                user=os.getenv("MYSQL_USER", "root"),
                password=os.getenv("MYSQL_PASSWORD", ""),
                database=os.getenv("MYSQL_DATABASE", "seckill"),
            ),
            server=ServerConfig(
                host=os.getenv("GRPC_HOST", "0.0.0.0"),
                port=int(os.getenv("GRPC_PORT", 50052)),
            ),
        )

    @classmethod
    def _from_dict(cls, data: dict) -> "Config":
        """从字典创建配置"""
        return cls(
            redis=RedisConfig(**data.get("redis", {})),
            milvus=MilvusConfig(**data.get("milvus", {})),
            mysql=MySQLConfig(**data.get("mysql", {})),
            recall=RecallConfig(**data.get("recall", {})),
            ranking=RankingConfig(**data.get("ranking", {})),
            server=ServerConfig(**data.get("server", {})),
        )


# 全局配置实例
_config: Config = None


def get_config() -> Config:
    """获取配置单例"""
    global _config
    if _config is None:
        config_path = os.getenv("CONFIG_PATH", "config.yaml")
        if os.path.exists(config_path):
            _config = Config.from_yaml(config_path)
        else:
            _config = Config.from_env()
    return _config

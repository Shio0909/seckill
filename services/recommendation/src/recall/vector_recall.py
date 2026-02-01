"""
向量召回
"""
from typing import Dict, List, Optional

import numpy as np
from loguru import logger

from .base_recall import BaseRecall


class VectorRecall(BaseRecall):
    """
    基于向量的语义召回
    
    使用Milvus进行向量检索
    
    流程:
    1. 获取用户向量(平均历史物品向量 或 用户特征向量)
    2. 在Milvus中检索最近邻
    """

    def __init__(self, config):
        super().__init__(config)
        self.milvus_client = None
        self.collection_name = config.milvus.collection
        self.embedding_dim = 128
        
        self._connect_milvus()
        
        logger.info("VectorRecall 初始化完成")

    def _connect_milvus(self):
        """连接Milvus"""
        try:
            from pymilvus import connections, Collection
            
            connections.connect(
                alias="default",
                host=self.config.milvus.host,
                port=self.config.milvus.port,
            )
            
            # 检查collection是否存在
            from pymilvus import utility
            if utility.has_collection(self.collection_name):
                self.collection = Collection(self.collection_name)
                self.collection.load()
                logger.info(f"已连接Milvus,collection: {self.collection_name}")
            else:
                logger.warning(f"Milvus collection {self.collection_name} 不存在")
                self.collection = None
                
        except Exception as e:
            logger.warning(f"Milvus连接失败: {e}")
            self.collection = None

    def recall(
        self,
        user_id: int,
        user_profile: Optional[Dict] = None,
        count: int = 100,
        **kwargs,
    ) -> List[Dict]:
        """
        向量召回
        """
        if self.collection is None:
            logger.debug("Milvus未连接,跳过向量召回")
            return []
        
        # 1. 获取用户向量
        user_vector = self._get_user_vector(user_id, user_profile)
        
        if user_vector is None:
            return []
        
        # 2. 向量检索
        try:
            search_params = {
                "metric_type": "IP",  # 内积相似度
                "params": {"nprobe": 10},
            }
            
            results = self.collection.search(
                data=[user_vector.tolist()],
                anns_field="embedding",
                param=search_params,
                limit=count,
                output_fields=["event_id"],
            )
            
            # 3. 构造结果
            recall_result = []
            for hits in results:
                for hit in hits:
                    recall_result.append({
                        "event_id": hit.entity.get("event_id"),
                        "score": float(hit.score),
                        "recall_source": "vector",
                    })
            
            logger.debug(f"VectorRecall: user_id={user_id}, count={len(recall_result)}")
            
            return recall_result
            
        except Exception as e:
            logger.error(f"向量检索失败: {e}")
            return []

    def _get_user_vector(self, user_id: int, user_profile: Optional[Dict]) -> Optional[np.ndarray]:
        """
        获取用户向量
        
        策略:
        1. 从缓存获取用户向量
        2. 或使用用户历史物品向量的平均值
        """
        # 这里简化为随机向量,实际应从Redis或数据库获取
        # 实际实现应该是:
        # 1. 从Redis获取预计算的用户向量
        # 2. 或者实时计算(历史物品向量平均)
        
        try:
            # 模拟: 返回随机向量
            # 实际应该: 
            # user_vec = self.redis.get(f"user:vec:{user_id}")
            return np.random.randn(self.embedding_dim).astype(np.float32)
        except:
            return None

    def create_collection(self):
        """
        创建Milvus Collection(离线脚本调用)
        """
        from pymilvus import (
            connections, Collection, FieldSchema, 
            CollectionSchema, DataType, utility
        )
        
        connections.connect(
            alias="default",
            host=self.config.milvus.host,
            port=self.config.milvus.port,
        )
        
        # 定义Schema
        fields = [
            FieldSchema(name="id", dtype=DataType.INT64, is_primary=True, auto_id=True),
            FieldSchema(name="event_id", dtype=DataType.INT64),
            FieldSchema(name="embedding", dtype=DataType.FLOAT_VECTOR, dim=self.embedding_dim),
        ]
        
        schema = CollectionSchema(fields=fields, description="Event embeddings")
        
        # 创建Collection
        if utility.has_collection(self.collection_name):
            utility.drop_collection(self.collection_name)
        
        collection = Collection(name=self.collection_name, schema=schema)
        
        # 创建索引
        index_params = {
            "metric_type": "IP",
            "index_type": "IVF_FLAT",
            "params": {"nlist": 128},
        }
        collection.create_index(field_name="embedding", index_params=index_params)
        
        logger.info(f"Milvus collection {self.collection_name} 创建成功")
        
        return collection

    def insert_embeddings(self, event_ids: List[int], embeddings: np.ndarray):
        """
        插入物品向量(离线脚本调用)
        
        Args:
            event_ids: 活动ID列表
            embeddings: 向量矩阵 [N, dim]
        """
        if self.collection is None:
            self.collection = self.create_collection()
        
        data = [
            event_ids,
            embeddings.tolist(),
        ]
        
        self.collection.insert(data)
        self.collection.flush()
        
        logger.info(f"插入 {len(event_ids)} 个向量")

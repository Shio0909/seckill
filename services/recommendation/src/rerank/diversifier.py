"""
多样性控制 - MMR算法
"""
from typing import Dict, List

from loguru import logger


class Diversifier:
    """
    多样性控制器
    
    使用MMR(Maximal Marginal Relevance)算法
    平衡相关性和多样性
    """

    def __init__(self):
        pass

    def diversify(
        self,
        items: List[Dict],
        lambda_param: float = 0.5,
        max_count: int = 50,
    ) -> List[Dict]:
        """
        MMR多样性重排
        
        Args:
            items: 排序后的候选列表
            lambda_param: 权重参数 (0-1)
                - 1.0: 只考虑相关性
                - 0.0: 只考虑多样性
                - 0.5: 平衡
            max_count: 最大返回数量
            
        Returns:
            多样性重排后的列表
        """
        if len(items) <= 1:
            return items
        
        # 归一化分数
        max_score = max(item["score"] for item in items) or 1.0
        for item in items:
            item["_norm_score"] = item["score"] / max_score
        
        selected = []
        remaining = items.copy()
        
        # 选择第一个(最高分)
        first = max(remaining, key=lambda x: x["_norm_score"])
        selected.append(first)
        remaining.remove(first)
        
        # 迭代选择
        while remaining and len(selected) < max_count:
            best_item = None
            best_mmr = float("-inf")
            
            for item in remaining:
                # 相关性分数
                relevance = item["_norm_score"]
                
                # 多样性分数 = 与已选物品的最大相似度
                max_sim = max(self._similarity(item, sel) for sel in selected)
                diversity = 1 - max_sim
                
                # MMR分数
                mmr = lambda_param * relevance + (1 - lambda_param) * diversity
                
                if mmr > best_mmr:
                    best_mmr = mmr
                    best_item = item
            
            if best_item:
                selected.append(best_item)
                remaining.remove(best_item)
        
        # 清理临时字段
        for item in selected:
            item.pop("_norm_score", None)
        
        logger.debug(f"MMR多样性重排: 输入{len(items)}, 输出{len(selected)}")
        
        return selected

    def _similarity(self, item_a: Dict, item_b: Dict) -> float:
        """
        计算两个物品的相似度
        
        简化实现:基于召回来源的相似度
        实际可以基于类别/标签/向量等
        """
        source_a = set(item_a.get("recall_source", "").split(","))
        source_b = set(item_b.get("recall_source", "").split(","))
        
        if not source_a or not source_b:
            return 0.0
        
        # Jaccard相似度
        intersection = len(source_a & source_b)
        union = len(source_a | source_b)
        
        return intersection / union if union > 0 else 0.0

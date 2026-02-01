"""召回层模块"""
from .base_recall import BaseRecall
from .item_cf import ItemCFRecall
from .hot_recall import HotRecall
from .vector_recall import VectorRecall
from .recall_merger import RecallMerger

__all__ = [
    "BaseRecall",
    "ItemCFRecall",
    "HotRecall",
    "VectorRecall",
    "RecallMerger",
]

#!/usr/bin/env python3
"""
计算ItemCF相似度矩阵并导出
用于召回层的协同过滤召回

输出:
- data/processed/item_similarity.csv: 物品相似度矩阵
- Redis导入脚本
"""

import math
import pandas as pd
import numpy as np
from pathlib import Path
from collections import defaultdict
from typing import Dict, Set, List, Tuple


DATA_DIR = Path(__file__).parent.parent.parent / "data"
OUTPUT_DIR = DATA_DIR / "processed"


def load_behaviors() -> pd.DataFrame:
    """加载行为数据"""
    behavior_file = OUTPUT_DIR / "behaviors.csv"
    
    if not behavior_file.exists():
        print(f"❌ 行为数据不存在: {behavior_file}")
        print("   请先运行: python transform_movielens.py")
        exit(1)
    
    print("📂 加载行为数据...")
    df = pd.read_csv(behavior_file)
    print(f"   总行为数: {len(df):,}")
    
    return df


def build_user_item_matrix(behaviors: pd.DataFrame, 
                           min_rating: float = 3.0) -> Dict[int, Set[int]]:
    """构建用户-物品倒排表"""
    print(f"\n📊 构建用户-物品矩阵 (rating >= {min_rating})...")
    
    # 只用高评分的交互
    positive = behaviors[behaviors['rating'] >= min_rating]
    
    user_items: Dict[int, Set[int]] = defaultdict(set)
    for _, row in positive.iterrows():
        user_items[row['user_id']].add(row['event_id'])
    
    print(f"   用户数: {len(user_items):,}")
    print(f"   平均物品数: {sum(len(v) for v in user_items.values()) / len(user_items):.1f}")
    
    return user_items


def compute_item_similarity(user_items: Dict[int, Set[int]], 
                           top_k: int = 50) -> Dict[int, List[Tuple[int, float]]]:
    """
    计算物品相似度（基于余弦相似度）
    
    使用IUF (Inverse User Frequency) 权重，减少热门物品的影响
    sim(i,j) = sum_u(1/log(1+|N_u|)) / sqrt(|N_i| * |N_j|)
    """
    print(f"\n🔢 计算物品相似度 (Top-{top_k})...")
    
    # 物品共现次数
    item_cooccur: Dict[int, Dict[int, float]] = defaultdict(lambda: defaultdict(float))
    # 物品出现次数
    item_count: Dict[int, int] = defaultdict(int)
    
    total_users = len(user_items)
    processed = 0
    
    for user, items in user_items.items():
        # IUF权重：用户交互物品越多，权重越低
        weight = 1.0 / math.log(1 + len(items))
        
        for i in items:
            item_count[i] += 1
            for j in items:
                if i != j:
                    item_cooccur[i][j] += weight
        
        processed += 1
        if processed % 1000 == 0:
            print(f"   处理进度: {processed}/{total_users} ({processed*100//total_users}%)")
    
    # 计算归一化相似度并取Top-K
    print("   归一化相似度...")
    item_sim: Dict[int, List[Tuple[int, float]]] = {}
    
    for i, related in item_cooccur.items():
        sim_list = []
        for j, cooccur in related.items():
            # 余弦相似度归一化
            sim = cooccur / math.sqrt(item_count[i] * item_count[j])
            sim_list.append((j, sim))
        
        # 按相似度排序取Top-K
        sim_list.sort(key=lambda x: x[1], reverse=True)
        item_sim[i] = sim_list[:top_k]
    
    print(f"   物品数: {len(item_sim):,}")
    
    return item_sim


def export_to_csv(item_sim: Dict[int, List[Tuple[int, float]]]):
    """导出相似度矩阵到CSV"""
    print("\n💾 导出相似度矩阵...")
    
    rows = []
    for item_i, similar_items in item_sim.items():
        for item_j, score in similar_items:
            rows.append({
                'item_i': item_i,
                'item_j': item_j,
                'similarity': round(score, 6),
            })
    
    df = pd.DataFrame(rows)
    output_file = OUTPUT_DIR / "item_similarity.csv"
    df.to_csv(output_file, index=False)
    
    print(f"   ✅ {output_file}")
    print(f"   总记录: {len(df):,}")


def generate_redis_import_script(item_sim: Dict[int, List[Tuple[int, float]]]):
    """生成Redis导入脚本"""
    print("\n📝 生成Redis导入脚本...")
    
    script_lines = [
        "#!/bin/bash",
        "# 导入ItemCF相似度矩阵到Redis",
        "# 用法: ./import_cf_to_redis.sh",
        "",
        "REDIS_HOST=${REDIS_HOST:-localhost}",
        "REDIS_PORT=${REDIS_PORT:-6379}",
        "",
        "echo '导入ItemCF相似度矩阵...'",
        "",
    ]
    
    for item_i, similar_items in item_sim.items():
        if len(similar_items) == 0:
            continue
        
        # 构建ZADD命令
        members = " ".join([f"{score:.6f} {item_j}" for item_j, score in similar_items])
        key = f"cf:item:{item_i}:similar"
        script_lines.append(f"redis-cli -h $REDIS_HOST -p $REDIS_PORT ZADD {key} {members}")
    
    script_lines.append("")
    script_lines.append("echo '导入完成!'")
    
    output_file = OUTPUT_DIR / "import_cf_to_redis.sh"
    with open(output_file, 'w', encoding='utf-8', newline='\n') as f:
        f.write('\n'.join(script_lines))
    
    print(f"   ✅ {output_file}")


def generate_redis_import_python(item_sim: Dict[int, List[Tuple[int, float]]]):
    """生成Python Redis导入脚本"""
    print("\n📝 生成Python导入脚本...")
    
    script = '''#!/usr/bin/env python3
"""
导入ItemCF相似度矩阵到Redis
"""
import redis
import csv
from pathlib import Path

def main():
    # 连接Redis
    rdb = redis.Redis(host='localhost', port=6379, db=0, decode_responses=True)
    
    # 读取相似度矩阵
    csv_file = Path(__file__).parent / "item_similarity.csv"
    
    print(f"📂 读取: {csv_file}")
    
    current_item = None
    batch = {}
    imported = 0
    
    with open(csv_file, 'r') as f:
        reader = csv.DictReader(f)
        for row in reader:
            item_i = int(row['item_i'])
            item_j = int(row['item_j'])
            score = float(row['similarity'])
            
            if current_item != item_i:
                # 写入上一个物品的相似度
                if current_item is not None and batch:
                    key = f"cf:item:{current_item}:similar"
                    rdb.zadd(key, batch)
                    imported += 1
                    
                    if imported % 500 == 0:
                        print(f"   已导入: {imported}")
                
                current_item = item_i
                batch = {}
            
            batch[str(item_j)] = score
        
        # 写入最后一个
        if current_item is not None and batch:
            key = f"cf:item:{current_item}:similar"
            rdb.zadd(key, batch)
            imported += 1
    
    print(f"✅ 导入完成: {imported} 个物品的相似度矩阵")

if __name__ == "__main__":
    main()
'''
    
    output_file = OUTPUT_DIR / "import_cf_to_redis.py"
    with open(output_file, 'w', encoding='utf-8') as f:
        f.write(script)
    
    print(f"   ✅ {output_file}")


def analyze_similarity(item_sim: Dict[int, List[Tuple[int, float]]]):
    """分析相似度分布"""
    print("\n📊 相似度分析")
    print("-" * 40)
    
    all_scores = []
    for similar_items in item_sim.values():
        for _, score in similar_items:
            all_scores.append(score)
    
    if all_scores:
        arr = np.array(all_scores)
        print(f"   最大值: {arr.max():.4f}")
        print(f"   最小值: {arr.min():.4f}")
        print(f"   平均值: {arr.mean():.4f}")
        print(f"   中位数: {np.median(arr):.4f}")
        print(f"   标准差: {arr.std():.4f}")


def main():
    print("=" * 60)
    print("🔗 ItemCF 相似度矩阵计算")
    print("=" * 60)
    
    # 加载数据
    behaviors = load_behaviors()
    
    # 构建用户-物品矩阵
    user_items = build_user_item_matrix(behaviors, min_rating=3.0)
    
    # 计算相似度
    item_sim = compute_item_similarity(user_items, top_k=50)
    
    # 分析
    analyze_similarity(item_sim)
    
    # 导出
    export_to_csv(item_sim)
    generate_redis_import_python(item_sim)
    
    print("\n" + "=" * 60)
    print("✅ 完成!")
    print("=" * 60)


if __name__ == "__main__":
    main()

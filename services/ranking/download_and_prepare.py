"""
MovieLens 1M 数据下载与预处理脚本
适配 EventHub 票务平台推荐系统

用法:
    python download_and_prepare.py --output_dir ./data
"""

import os
import zipfile
import urllib.request
import argparse
from datetime import datetime
from pathlib import Path

import pandas as pd
import numpy as np


# MovieLens 1M 下载地址
MOVIELENS_URL = "https://files.grouplens.org/datasets/movielens/ml-1m.zip"

# 分类映射：电影类型 → 票务分类
GENRE_TO_CATEGORY = {
    "Musical": "演唱会",
    "Music": "演唱会",
    "Drama": "话剧",
    "Romance": "话剧",
    "Action": "体育赛事",
    "Adventure": "体育赛事",
    "War": "体育赛事",
    "Documentary": "展览",
    "History": "展览",
    "Comedy": "音乐节",
    "Animation": "儿童剧",
    "Children's": "儿童剧",
    "Fantasy": "音乐节",
    "Sci-Fi": "展览",
    "Horror": "话剧",
    "Thriller": "话剧",
    "Crime": "话剧",
    "Mystery": "话剧",
    "Western": "体育赛事",
    "Film-Noir": "话剧",
}

CATEGORIES = ["演唱会", "话剧", "体育赛事", "展览", "音乐节", "儿童剧"]

# 城市列表（随机分配）
CITIES = ["北京", "上海", "广州", "深圳", "杭州", "成都", "武汉", "南京", "重庆", "西安"]

# 场馆列表
VENUES = {
    "北京": ["国家体育场(鸟巢)", "工人体育场", "国家大剧院", "北展剧场"],
    "上海": ["上海体育场", "梅赛德斯奔驰文化中心", "上海大剧院", "东方艺术中心"],
    "广州": ["广州体育馆", "广州大剧院", "中山纪念堂"],
    "深圳": ["深圳湾体育中心", "深圳大剧院", "保利剧院"],
    "杭州": ["黄龙体育中心", "杭州大剧院"],
    "成都": ["成都露天音乐公园", "四川大剧院"],
    "武汉": ["武汉体育中心", "琴台大剧院"],
    "南京": ["南京奥体中心", "江苏大剧院"],
    "重庆": ["重庆奥体中心", "重庆大剧院"],
    "西安": ["西安奥体中心", "陕西大剧院"],
}


def download_movielens(output_dir: str) -> str:
    """下载 MovieLens 1M 数据集"""
    os.makedirs(output_dir, exist_ok=True)
    zip_path = os.path.join(output_dir, "ml-1m.zip")
    extract_dir = os.path.join(output_dir, "ml-1m")
    
    if os.path.exists(extract_dir):
        print(f"✅ 数据已存在: {extract_dir}")
        return extract_dir
    
    print(f"📥 正在下载 MovieLens 1M...")
    print(f"   URL: {MOVIELENS_URL}")
    
    try:
        urllib.request.urlretrieve(MOVIELENS_URL, zip_path)
        print(f"✅ 下载完成: {zip_path}")
    except Exception as e:
        print(f"❌ 下载失败: {e}")
        print("   请手动下载并解压到:", output_dir)
        raise
    
    print(f"📦 正在解压...")
    with zipfile.ZipFile(zip_path, 'r') as zip_ref:
        zip_ref.extractall(output_dir)
    print(f"✅ 解压完成: {extract_dir}")
    
    # 删除zip文件
    os.remove(zip_path)
    
    return extract_dir


def load_movielens(data_dir: str) -> tuple:
    """加载 MovieLens 原始数据"""
    print(f"\n📂 加载数据...")
    
    # 加载用户数据
    users = pd.read_csv(
        os.path.join(data_dir, "users.dat"),
        sep="::",
        names=["user_id", "gender", "age", "occupation", "zip_code"],
        engine="python",
        encoding="latin-1"
    )
    print(f"   用户数: {len(users)}")
    
    # 加载电影数据
    movies = pd.read_csv(
        os.path.join(data_dir, "movies.dat"),
        sep="::",
        names=["movie_id", "title", "genres"],
        engine="python",
        encoding="latin-1"
    )
    print(f"   电影数: {len(movies)}")
    
    # 加载评分数据
    ratings = pd.read_csv(
        os.path.join(data_dir, "ratings.dat"),
        sep="::",
        names=["user_id", "movie_id", "rating", "timestamp"],
        engine="python",
        encoding="latin-1"
    )
    print(f"   评分数: {len(ratings)}")
    
    return users, movies, ratings


def adapt_timestamp(df: pd.DataFrame) -> pd.DataFrame:
    """将时间戳适配到 2025-2026 年"""
    original_min = df['timestamp'].min()
    original_max = df['timestamp'].max()
    original_range = original_max - original_min
    
    # 目标时间范围: 2025-01-01 到 2026-02-01
    target_start = int(datetime(2025, 1, 1).timestamp())
    target_end = int(datetime(2026, 2, 1).timestamp())
    target_range = target_end - target_start
    
    df['timestamp'] = (
        (df['timestamp'] - original_min) / original_range * target_range + target_start
    ).astype(int)
    
    return df


def map_genre_to_category(genres: str) -> str:
    """将电影类型映射到票务分类"""
    genre_list = genres.split("|")
    for genre in genre_list:
        if genre in GENRE_TO_CATEGORY:
            return GENRE_TO_CATEGORY[genre]
    return "演唱会"  # 默认


def transform_to_events(movies: pd.DataFrame) -> pd.DataFrame:
    """将电影数据转换为活动数据"""
    np.random.seed(42)
    
    events = pd.DataFrame()
    events['event_id'] = movies['movie_id']
    events['title'] = movies['title'].apply(lambda x: x.rsplit('(', 1)[0].strip())
    events['category'] = movies['genres'].apply(map_genre_to_category)
    
    # 随机分配城市
    events['city'] = np.random.choice(CITIES, len(events))
    
    # 根据城市分配场馆
    events['venue'] = events['city'].apply(
        lambda c: np.random.choice(VENUES.get(c, ["默认场馆"]))
    )
    
    # 生成价格（根据分类）
    price_ranges = {
        "演唱会": (380, 1680),
        "话剧": (180, 880),
        "体育赛事": (280, 1280),
        "展览": (50, 200),
        "音乐节": (299, 999),
        "儿童剧": (100, 300),
    }
    
    def generate_price(category):
        min_p, max_p = price_ranges.get(category, (100, 500))
        return np.random.randint(min_p, max_p)
    
    events['min_price'] = events['category'].apply(generate_price)
    events['max_price'] = events['min_price'] * np.random.uniform(1.5, 3.0, len(events))
    events['max_price'] = events['max_price'].astype(int)
    
    # 生成活动时间（未来1年内）
    base_time = datetime(2026, 3, 1)
    events['start_time'] = [
        int((base_time + pd.Timedelta(days=np.random.randint(1, 365))).timestamp())
        for _ in range(len(events))
    ]
    
    # 状态：1=预售中, 2=售卖中
    events['status'] = np.random.choice([1, 2], len(events), p=[0.3, 0.7])
    
    # 销量和浏览量
    events['sales_count'] = np.random.exponential(1000, len(events)).astype(int)
    events['view_count'] = events['sales_count'] * np.random.uniform(5, 20, len(events))
    events['view_count'] = events['view_count'].astype(int)
    
    # 评分
    events['score'] = np.random.uniform(7.0, 9.8, len(events)).round(1)
    
    return events


def transform_to_users(users: pd.DataFrame) -> pd.DataFrame:
    """转换用户数据"""
    result = pd.DataFrame()
    result['user_id'] = users['user_id']
    result['gender'] = users['gender'].map({'M': 'male', 'F': 'female'})
    result['age'] = users['age']
    result['city'] = np.random.choice(CITIES, len(users))
    
    # 注册天数
    result['reg_days'] = np.random.randint(1, 365 * 3, len(users))
    
    return result


def transform_to_behaviors(ratings: pd.DataFrame, events: pd.DataFrame) -> pd.DataFrame:
    """将评分转换为行为数据"""
    # 适配时间戳
    ratings = adapt_timestamp(ratings.copy())
    
    behaviors = pd.DataFrame()
    behaviors['user_id'] = ratings['user_id']
    behaviors['event_id'] = ratings['movie_id']
    behaviors['timestamp'] = ratings['timestamp']
    
    # 评分 >= 4 视为点击/购买，< 4 视为仅浏览
    behaviors['rating'] = ratings['rating']
    behaviors['label'] = (ratings['rating'] >= 4).astype(int)
    
    # 事件类型
    behaviors['event_type'] = behaviors['rating'].apply(
        lambda r: 'purchase' if r >= 5 else ('click' if r >= 4 else 'view')
    )
    
    return behaviors


def generate_train_test_split(behaviors: pd.DataFrame, test_ratio: float = 0.2):
    """按时间划分训练集和测试集"""
    behaviors = behaviors.sort_values('timestamp')
    
    split_idx = int(len(behaviors) * (1 - test_ratio))
    train = behaviors.iloc[:split_idx]
    test = behaviors.iloc[split_idx:]
    
    return train, test


def compute_item_cf_similarity(behaviors: pd.DataFrame, top_k: int = 50) -> pd.DataFrame:
    """计算物品相似度矩阵（用于ItemCF召回）"""
    print("\n🔧 计算ItemCF相似度...")
    
    # 只使用正样本
    positive = behaviors[behaviors['label'] == 1]
    
    # 构建用户-物品交互矩阵
    user_items = positive.groupby('user_id')['event_id'].apply(set).to_dict()
    
    # 构建物品-用户倒排索引
    item_users = positive.groupby('event_id')['user_id'].apply(set).to_dict()
    
    # 计算物品共现次数
    from collections import defaultdict
    cooccur = defaultdict(lambda: defaultdict(int))
    item_count = defaultdict(int)
    
    for user_id, items in user_items.items():
        for item_i in items:
            item_count[item_i] += 1
            for item_j in items:
                if item_i != item_j:
                    cooccur[item_i][item_j] += 1
    
    # 计算相似度
    similarities = []
    for item_i, related in cooccur.items():
        for item_j, count in related.items():
            # 余弦相似度
            sim = count / np.sqrt(item_count[item_i] * item_count[item_j])
            similarities.append({
                'item_i': item_i,
                'item_j': item_j,
                'similarity': sim
            })
    
    sim_df = pd.DataFrame(similarities)
    
    # 每个物品保留 top_k 相似物品
    sim_df = sim_df.sort_values(['item_i', 'similarity'], ascending=[True, False])
    sim_df = sim_df.groupby('item_i').head(top_k)
    
    print(f"   相似度对数: {len(sim_df)}")
    
    return sim_df


def main():
    parser = argparse.ArgumentParser(description="下载并处理MovieLens数据集")
    parser.add_argument("--output_dir", type=str, default="./data", help="输出目录")
    parser.add_argument("--skip_download", action="store_true", help="跳过下载")
    args = parser.parse_args()
    
    output_dir = Path(args.output_dir)
    output_dir.mkdir(parents=True, exist_ok=True)
    
    # Step 1: 下载数据
    if not args.skip_download:
        data_dir = download_movielens(str(output_dir))
    else:
        data_dir = str(output_dir / "ml-1m")
    
    # Step 2: 加载原始数据
    users, movies, ratings = load_movielens(data_dir)
    
    # Step 3: 转换数据
    print("\n🔄 转换数据格式...")
    events = transform_to_events(movies)
    users_transformed = transform_to_users(users)
    behaviors = transform_to_behaviors(ratings, events)
    
    # Step 4: 划分数据集
    train, test = generate_train_test_split(behaviors)
    print(f"\n📊 数据集划分:")
    print(f"   训练集: {len(train)} 条")
    print(f"   测试集: {len(test)} 条")
    
    # Step 5: 计算ItemCF相似度
    item_sim = compute_item_cf_similarity(train)
    
    # Step 6: 保存处理后的数据
    processed_dir = output_dir / "processed"
    processed_dir.mkdir(exist_ok=True)
    
    events.to_csv(processed_dir / "events.csv", index=False)
    users_transformed.to_csv(processed_dir / "users.csv", index=False)
    train.to_csv(processed_dir / "train.csv", index=False)
    test.to_csv(processed_dir / "test.csv", index=False)
    item_sim.to_csv(processed_dir / "item_similarity.csv", index=False)
    
    print(f"\n✅ 数据处理完成!")
    print(f"   输出目录: {processed_dir}")
    print(f"   - events.csv: {len(events)} 条活动")
    print(f"   - users.csv: {len(users_transformed)} 条用户")
    print(f"   - train.csv: {len(train)} 条训练样本")
    print(f"   - test.csv: {len(test)} 条测试样本")
    print(f"   - item_similarity.csv: {len(item_sim)} 条相似度")
    
    # 打印数据统计
    print("\n📈 数据统计:")
    print(f"   分类分布:")
    print(events['category'].value_counts().to_string())
    print(f"\n   城市分布:")
    print(events['city'].value_counts().head(5).to_string())
    print(f"\n   正负样本比例:")
    print(f"   正样本(label=1): {(behaviors['label']==1).sum()} ({(behaviors['label']==1).mean()*100:.1f}%)")
    print(f"   负样本(label=0): {(behaviors['label']==0).sum()} ({(behaviors['label']==0).mean()*100:.1f}%)")


if __name__ == "__main__":
    main()

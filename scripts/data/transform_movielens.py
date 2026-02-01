#!/usr/bin/env python3
"""
MovieLens数据转换脚本
将MovieLens数据转换为票务平台格式，用于推荐系统训练

输出:
- data/processed/events.csv: 活动数据
- data/processed/users.csv: 用户数据
- data/processed/behaviors.csv: 行为数据
- data/processed/train.csv: 训练数据（带标签）
"""

import os
import sys
import random
import pandas as pd
import numpy as np
from pathlib import Path
from datetime import datetime, timedelta


DATA_DIR = Path(__file__).parent.parent.parent / "data"
OUTPUT_DIR = DATA_DIR / "processed"


# ===================== 配置 =====================

# 电影类型 -> 活动类别映射
GENRE_TO_CATEGORY = {
    'Action': 1,      # 演唱会（热门、刺激）
    'Adventure': 5,   # 音乐节（户外、探索）
    'Animation': 3,   # 展览（艺术）
    'Children\'s': 3, # 展览（儿童展）
    'Comedy': 4,      # 话剧（喜剧）
    'Crime': 4,       # 话剧（悬疑）
    'Documentary': 3, # 展览（博物馆）
    'Drama': 4,       # 话剧
    'Fantasy': 1,     # 演唱会（奇幻主题）
    'Film-Noir': 4,   # 话剧（黑色电影）
    'Horror': 4,      # 话剧
    'Musical': 1,     # 演唱会
    'Mystery': 4,     # 话剧
    'Romance': 4,     # 话剧
    'Sci-Fi': 3,      # 展览（科技展）
    'Thriller': 2,    # 体育赛事（紧张刺激）
    'War': 3,         # 展览（历史展）
    'Western': 2,     # 体育赛事
}

CATEGORY_NAMES = {
    1: '演唱会',
    2: '体育赛事',
    3: '展览',
    4: '话剧',
    5: '音乐节',
}

# 城市列表
CITIES = [
    (1, '上海'), (2, '北京'), (3, '广州'), (4, '深圳'),
    (5, '成都'), (6, '杭州'), (7, '南京'), (8, '武汉'),
    (9, '重庆'), (10, '沈阳'), (11, '青岛'), (12, '西安'),
]

# 各类别的场馆
VENUES = {
    1: ['体育馆', '大剧院', '文化中心', '演艺中心'],
    2: ['体育场', '体育中心', '竞技场', '足球场'],
    3: ['博物馆', '美术馆', '展览中心', '科技馆'],
    4: ['剧院', '话剧中心', '艺术中心', '大剧院'],
    5: ['公园', '露天广场', '音乐公园', '生态园'],
}

# 年龄编码（MovieLens的年龄分段）
AGE_MAPPING = {
    1: 18,    # Under 18
    18: 22,   # 18-24
    25: 30,   # 25-34
    35: 40,   # 35-44
    45: 50,   # 45-49
    50: 55,   # 50-55
    56: 60,   # 56+
}


# ===================== 加载数据 =====================

def load_movielens_1m() -> tuple:
    """加载MovieLens 1M数据"""
    ml_dir = DATA_DIR / "ml-1m"
    
    if not ml_dir.exists():
        print(f"❌ 数据不存在: {ml_dir}")
        print("   请先运行: python download_datasets.py -d movielens -v 1m")
        sys.exit(1)
    
    print("📂 加载 MovieLens 1M 数据...")
    
    # 用户数据
    users = pd.read_csv(
        ml_dir / "users.dat",
        sep='::',
        names=['user_id', 'gender', 'age', 'occupation', 'zipcode'],
        engine='python',
        encoding='latin-1'
    )
    print(f"   用户: {len(users):,} 条")
    
    # 电影数据
    movies = pd.read_csv(
        ml_dir / "movies.dat",
        sep='::',
        names=['movie_id', 'title', 'genres'],
        engine='python',
        encoding='latin-1'
    )
    print(f"   电影: {len(movies):,} 条")
    
    # 评分数据
    ratings = pd.read_csv(
        ml_dir / "ratings.dat",
        sep='::',
        names=['user_id', 'movie_id', 'rating', 'timestamp'],
        engine='python'
    )
    print(f"   评分: {len(ratings):,} 条")
    
    return users, movies, ratings


# ===================== 转换函数 =====================

def transform_movies_to_events(movies: pd.DataFrame) -> pd.DataFrame:
    """将电影转换为活动"""
    print("\n🎭 转换电影为活动...")
    
    events = []
    base_date = datetime(2024, 6, 1)
    
    for _, row in movies.iterrows():
        genres = row['genres'].split('|')
        first_genre = genres[0] if genres else 'Drama'
        category_id = GENRE_TO_CATEGORY.get(first_genre, 4)
        
        # 随机分配城市
        city_id, city_name = random.choice(CITIES)
        
        # 生成场馆
        venue_list = VENUES.get(category_id, ['场馆'])
        venue = f"{city_name}{random.choice(venue_list)}"
        
        # 生成价格
        base_price = random.randint(80, 500)
        high_price = base_price + random.randint(100, 800)
        
        # 生成活动时间（未来随机时间）
        event_time = base_date + timedelta(days=random.randint(1, 180))
        
        # 热度分数
        hot_score = random.randint(1000, 50000)
        
        events.append({
            'event_id': row['movie_id'],
            'name': row['title'],
            'category_id': category_id,
            'category_name': CATEGORY_NAMES[category_id],
            'city_id': city_id,
            'city': city_name,
            'venue': venue,
            'price': base_price,
            'high_price': high_price,
            'stock': random.randint(1000, 50000),
            'tags': row['genres'],
            'hot_score': hot_score,
            'event_time': event_time.strftime('%Y-%m-%d %H:%M:%S'),
        })
    
    df = pd.DataFrame(events)
    print(f"   生成 {len(df):,} 条活动数据")
    print(f"   类别分布: {df['category_name'].value_counts().to_dict()}")
    
    return df


def transform_users(users: pd.DataFrame) -> pd.DataFrame:
    """转换用户数据"""
    print("\n👥 转换用户数据...")
    
    transformed = []
    
    for _, row in users.iterrows():
        city_id, city_name = random.choice(CITIES)
        age = AGE_MAPPING.get(row['age'], 25)
        
        transformed.append({
            'user_id': row['user_id'],
            'gender': 1 if row['gender'] == 'M' else 0,
            'age': age,
            'city_id': city_id,
            'city': city_name,
            'occupation': row['occupation'],
        })
    
    df = pd.DataFrame(transformed)
    print(f"   生成 {len(df):,} 条用户数据")
    print(f"   性别分布: 男={len(df[df['gender']==1]):,}, 女={len(df[df['gender']==0]):,}")
    
    return df


def transform_ratings_to_behaviors(ratings: pd.DataFrame) -> pd.DataFrame:
    """将评分转换为行为数据"""
    print("\n📊 转换评分为行为数据...")
    
    behaviors = []
    
    for _, row in ratings.iterrows():
        rating = row['rating']
        
        # 评分 -> 行为类型
        if rating >= 4:
            behavior_type = 'purchase'
            weight = 1.0
        elif rating >= 3:
            behavior_type = 'click'
            weight = 0.5
        else:
            behavior_type = 'view'
            weight = 0.1
        
        behaviors.append({
            'user_id': row['user_id'],
            'event_id': row['movie_id'],
            'behavior_type': behavior_type,
            'weight': weight,
            'rating': rating,
            'timestamp': row['timestamp'],
        })
    
    df = pd.DataFrame(behaviors)
    print(f"   生成 {len(df):,} 条行为数据")
    print(f"   行为分布: {df['behavior_type'].value_counts().to_dict()}")
    
    return df


def generate_training_data(behaviors: pd.DataFrame, events: pd.DataFrame, 
                          users: pd.DataFrame, neg_ratio: int = 4) -> pd.DataFrame:
    """生成训练数据（正负样本）"""
    print(f"\n🏋️ 生成训练数据 (负样本比例 1:{neg_ratio})...")
    
    # 正样本：购买和点击
    positive = behaviors[behaviors['behavior_type'].isin(['purchase', 'click'])].copy()
    positive['label'] = 1
    
    # 构建用户-物品交互集合
    user_items = positive.groupby('user_id')['event_id'].apply(set).to_dict()
    all_events = set(events['event_id'].tolist())
    
    # 生成负样本
    print("   生成负样本...")
    negative_samples = []
    
    for user_id, pos_items in user_items.items():
        # 未交互的物品
        neg_candidates = list(all_events - pos_items)
        num_neg = min(len(pos_items) * neg_ratio, len(neg_candidates))
        
        if num_neg > 0:
            neg_items = random.sample(neg_candidates, num_neg)
            for event_id in neg_items:
                negative_samples.append({
                    'user_id': user_id,
                    'event_id': event_id,
                    'label': 0,
                })
    
    negative = pd.DataFrame(negative_samples)
    
    # 合并正负样本
    positive_subset = positive[['user_id', 'event_id', 'label']]
    train_data = pd.concat([positive_subset, negative], ignore_index=True)
    
    # 打乱顺序
    train_data = train_data.sample(frac=1, random_state=42).reset_index(drop=True)
    
    print(f"   正样本: {len(positive_subset):,}")
    print(f"   负样本: {len(negative):,}")
    print(f"   总样本: {len(train_data):,}")
    print(f"   正负比例: 1:{len(negative)//len(positive_subset)}")
    
    return train_data


def add_features(train_data: pd.DataFrame, events: pd.DataFrame, 
                users: pd.DataFrame) -> pd.DataFrame:
    """添加特征"""
    print("\n🔧 添加特征...")
    
    # 合并用户特征
    train_data = train_data.merge(users, on='user_id', how='left')
    
    # 合并活动特征
    event_features = events[['event_id', 'category_id', 'city_id', 'price', 
                            'high_price', 'hot_score']].copy()
    event_features.rename(columns={
        'city_id': 'event_city_id',
    }, inplace=True)
    train_data = train_data.merge(event_features, on='event_id', how='left')
    
    # 添加交叉特征
    train_data['city_match'] = (train_data['city_id'] == train_data['event_city_id']).astype(int)
    
    print(f"   特征列: {list(train_data.columns)}")
    
    return train_data


# ===================== 主函数 =====================

def main():
    print("=" * 60)
    print("🎬 MovieLens -> 票务平台 数据转换")
    print("=" * 60)
    
    # 创建输出目录
    OUTPUT_DIR.mkdir(parents=True, exist_ok=True)
    
    # 加载数据
    users, movies, ratings = load_movielens_1m()
    
    # 转换数据
    events = transform_movies_to_events(movies)
    users_transformed = transform_users(users)
    behaviors = transform_ratings_to_behaviors(ratings)
    
    # 生成训练数据
    train_data = generate_training_data(behaviors, events, users_transformed)
    train_data_with_features = add_features(train_data, events, users_transformed)
    
    # 保存数据
    print("\n💾 保存数据...")
    
    events.to_csv(OUTPUT_DIR / "events.csv", index=False)
    print(f"   ✅ {OUTPUT_DIR / 'events.csv'}")
    
    users_transformed.to_csv(OUTPUT_DIR / "users.csv", index=False)
    print(f"   ✅ {OUTPUT_DIR / 'users.csv'}")
    
    behaviors.to_csv(OUTPUT_DIR / "behaviors.csv", index=False)
    print(f"   ✅ {OUTPUT_DIR / 'behaviors.csv'}")
    
    train_data_with_features.to_csv(OUTPUT_DIR / "train.csv", index=False)
    print(f"   ✅ {OUTPUT_DIR / 'train.csv'}")
    
    # 统计信息
    print("\n" + "=" * 60)
    print("📊 数据统计")
    print("=" * 60)
    print(f"   活动数量: {len(events):,}")
    print(f"   用户数量: {len(users_transformed):,}")
    print(f"   行为数量: {len(behaviors):,}")
    print(f"   训练样本: {len(train_data_with_features):,}")
    print(f"   数据目录: {OUTPUT_DIR}")
    print("=" * 60)


if __name__ == "__main__":
    main()

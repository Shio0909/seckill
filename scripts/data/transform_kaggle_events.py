#!/usr/bin/env python3
"""
Kaggle Event Recommendation 数据集转换脚本

这是一个真实的活动推荐数据集，比MovieLens更贴合我们的场景！

数据来源: https://www.kaggle.com/c/event-recommendation-engine-challenge

需要先手动下载数据到 data/kaggle-events/ 目录

数据结构:
- users.csv: user_id, locale, birthyear, gender, joinedAt, location, timezone
- events.csv: event_id, user_id, start_time, city, state, country, lat, lng, c_1...c_100
- train.csv: user, event, invited, timestamp, interested, not_interested
- event_attendees.csv: event, yes, maybe, invited, no
"""

import os
import sys
import gzip
import pandas as pd
import numpy as np
from pathlib import Path
from datetime import datetime


DATA_DIR = Path(__file__).parent.parent.parent / "data"
KAGGLE_DIR = DATA_DIR / "kaggle-events"
OUTPUT_DIR = DATA_DIR / "processed-kaggle"


# 中国城市映射（根据时区和国家推断）
TIMEZONE_CITY_MAPPING = {
    'Asia/Shanghai': (1, '上海'),
    'Asia/Hong_Kong': (4, '深圳'),
    'Asia/Tokyo': (1, '上海'),  # 映射到最近
    'America/New_York': (2, '北京'),
    'America/Los_Angeles': (6, '杭州'),
    'Europe/London': (3, '广州'),
    'Europe/Paris': (5, '成都'),
}

DEFAULT_CITY = (1, '上海')


def read_csv_or_gzip(filepath: Path) -> pd.DataFrame:
    """读取CSV或GZ压缩的CSV"""
    if filepath.with_suffix('.csv.gz').exists():
        with gzip.open(filepath.with_suffix('.csv.gz'), 'rt') as f:
            return pd.read_csv(f)
    elif filepath.with_suffix('.csv').exists():
        return pd.read_csv(filepath.with_suffix('.csv'))
    elif filepath.exists():
        return pd.read_csv(filepath)
    else:
        raise FileNotFoundError(f"找不到文件: {filepath}")


def load_kaggle_data():
    """加载Kaggle数据"""
    print("📂 加载Kaggle Event Recommendation数据...")
    
    if not KAGGLE_DIR.exists():
        print(f"❌ 数据目录不存在: {KAGGLE_DIR}")
        print("   请先下载数据: https://www.kaggle.com/c/event-recommendation-engine-challenge/data")
        sys.exit(1)
    
    # 加载用户数据
    print("   加载 users...")
    users = read_csv_or_gzip(KAGGLE_DIR / "users")
    print(f"   用户: {len(users):,}")
    
    # 加载活动数据（可能很大）
    print("   加载 events...")
    events = read_csv_or_gzip(KAGGLE_DIR / "events")
    print(f"   活动: {len(events):,}")
    
    # 加载训练数据
    print("   加载 train...")
    train = read_csv_or_gzip(KAGGLE_DIR / "train")
    print(f"   训练样本: {len(train):,}")
    
    return users, events, train


def transform_kaggle_users(users: pd.DataFrame) -> pd.DataFrame:
    """转换Kaggle用户数据"""
    print("\n👥 转换用户数据...")
    
    transformed = []
    
    for _, row in users.iterrows():
        # 性别
        gender = 0  # 默认
        if pd.notna(row.get('gender')):
            if row['gender'] == 'male':
                gender = 1
            elif row['gender'] == 'female':
                gender = 0
        
        # 年龄（从出生年推算）
        age = 25  # 默认
        if pd.notna(row.get('birthyear')):
            try:
                age = 2024 - int(row['birthyear'])
                age = max(18, min(70, age))  # 限制范围
            except:
                pass
        
        # 城市（从时区推断）
        timezone = row.get('timezone', '')
        city_info = TIMEZONE_CITY_MAPPING.get(timezone, DEFAULT_CITY)
        city_id, city = city_info
        
        transformed.append({
            'user_id': row['user_id'],
            'gender': gender,
            'age': age,
            'city_id': city_id,
            'city': city,
            'locale': row.get('locale', 'zh_CN'),
        })
    
    df = pd.DataFrame(transformed)
    print(f"   生成 {len(df):,} 条用户数据")
    
    return df


def transform_kaggle_events(events: pd.DataFrame, sample_size: int = None) -> pd.DataFrame:
    """转换Kaggle活动数据"""
    print("\n🎭 转换活动数据...")
    
    # 采样（如果数据太大）
    if sample_size and len(events) > sample_size:
        print(f"   采样 {sample_size} 条...")
        events = events.sample(n=sample_size, random_state=42)
    
    # 类别映射（使用聚类特征）
    # Kaggle数据有 c_1 到 c_100 的特征列，可以用于推断类别
    def infer_category(row):
        # 简化处理：根据活动的某些特征分配类别
        # 实际应该用聚类或规则
        import random
        return random.randint(1, 5)
    
    # 中国城市列表
    cities = [
        (1, '上海'), (2, '北京'), (3, '广州'), (4, '深圳'),
        (5, '成都'), (6, '杭州'), (7, '南京'), (8, '武汉'),
    ]
    
    category_names = {
        1: '演唱会',
        2: '体育赛事',
        3: '展览',
        4: '话剧',
        5: '音乐节',
    }
    
    venues = {
        1: ['体育馆', '大剧院', '文化中心'],
        2: ['体育场', '体育中心', '竞技场'],
        3: ['博物馆', '美术馆', '展览中心'],
        4: ['剧院', '话剧中心', '艺术中心'],
        5: ['公园', '露天广场', '音乐公园'],
    }
    
    import random
    
    transformed = []
    for _, row in events.iterrows():
        category_id = infer_category(row)
        city_id, city_name = random.choice(cities)
        venue = f"{city_name}{random.choice(venues[category_id])}"
        
        # 价格
        base_price = random.randint(80, 500)
        high_price = base_price + random.randint(100, 800)
        
        # 热度
        hot_score = random.randint(1000, 50000)
        
        # 活动时间
        event_time = row.get('start_time', '')
        if pd.isna(event_time) or event_time == '':
            from datetime import datetime, timedelta
            event_time = (datetime.now() + timedelta(days=random.randint(1, 180))).strftime('%Y-%m-%d %H:%M:%S')
        
        transformed.append({
            'event_id': row['event_id'],
            'name': f"活动{row['event_id'][:8]}",  # 生成名称
            'category_id': category_id,
            'category_name': category_names[category_id],
            'city_id': city_id,
            'city': city_name,
            'venue': venue,
            'price': base_price,
            'high_price': high_price,
            'stock': random.randint(1000, 50000),
            'hot_score': hot_score,
            'event_time': event_time,
        })
    
    df = pd.DataFrame(transformed)
    print(f"   生成 {len(df):,} 条活动数据")
    
    return df


def transform_kaggle_train(train: pd.DataFrame) -> pd.DataFrame:
    """转换Kaggle训练数据为行为数据"""
    print("\n📊 转换训练数据...")
    
    behaviors = []
    
    for _, row in train.iterrows():
        # interested = 1 表示感兴趣（正样本）
        # not_interested = 1 表示不感兴趣（负样本）
        
        if row.get('interested', 0) == 1:
            behavior_type = 'click'
            weight = 0.8
            label = 1
        elif row.get('not_interested', 0) == 1:
            behavior_type = 'view'
            weight = 0.0
            label = 0
        else:
            # 没有明确反馈，跳过
            continue
        
        behaviors.append({
            'user_id': row['user'],
            'event_id': row['event'],
            'behavior_type': behavior_type,
            'weight': weight,
            'label': label,
            'timestamp': row.get('timestamp', 0),
        })
    
    df = pd.DataFrame(behaviors)
    print(f"   生成 {len(df):,} 条行为数据")
    print(f"   正样本: {len(df[df['label']==1]):,}")
    print(f"   负样本: {len(df[df['label']==0]):,}")
    
    return df


def main():
    print("=" * 60)
    print("🎭 Kaggle Event Recommendation 数据转换")
    print("=" * 60)
    
    # 创建输出目录
    OUTPUT_DIR.mkdir(parents=True, exist_ok=True)
    
    # 加载数据
    users, events, train = load_kaggle_data()
    
    # 转换数据（采样以减少处理时间）
    users_transformed = transform_kaggle_users(users)
    events_transformed = transform_kaggle_events(events, sample_size=10000)  # 采样1万条
    behaviors = transform_kaggle_train(train)
    
    # 保存数据
    print("\n💾 保存数据...")
    
    users_transformed.to_csv(OUTPUT_DIR / "users.csv", index=False)
    print(f"   ✅ {OUTPUT_DIR / 'users.csv'}")
    
    events_transformed.to_csv(OUTPUT_DIR / "events.csv", index=False)
    print(f"   ✅ {OUTPUT_DIR / 'events.csv'}")
    
    behaviors.to_csv(OUTPUT_DIR / "behaviors.csv", index=False)
    print(f"   ✅ {OUTPUT_DIR / 'behaviors.csv'}")
    
    print("\n" + "=" * 60)
    print("✅ 转换完成!")
    print(f"   输出目录: {OUTPUT_DIR}")
    print("=" * 60)


if __name__ == "__main__":
    main()

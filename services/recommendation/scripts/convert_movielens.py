"""
MovieLens数据转换为票务场景数据

将MovieLens的电影评分数据转换为演出票务的用户行为数据
"""
import os
import random
from datetime import datetime, timedelta

import pandas as pd
import numpy as np
from loguru import logger


# 类别映射: MovieLens genres -> 票务类别
CATEGORY_MAP = {
    "Action": "演唱会",
    "Adventure": "展览",
    "Animation": "儿童剧",
    "Children's": "儿童剧",
    "Comedy": "话剧",
    "Crime": "话剧",
    "Documentary": "展览",
    "Drama": "话剧",
    "Fantasy": "音乐会",
    "Film-Noir": "话剧",
    "Horror": "沉浸式体验",
    "Musical": "音乐剧",
    "Mystery": "沉浸式体验",
    "Romance": "话剧",
    "Sci-Fi": "沉浸式体验",
    "Thriller": "沉浸式体验",
    "War": "话剧",
    "Western": "话剧",
}

# 城市列表
CITIES = [
    "北京", "上海", "广州", "深圳", "杭州",
    "成都", "武汉", "南京", "西安", "重庆",
]

# 场馆映射
VENUES = {
    "北京": ["工人体育场", "国家大剧院", "鸟巢", "五棵松体育馆", "保利剧院"],
    "上海": ["梅赛德斯奔驰文化中心", "上海大剧院", "上海体育场", "东方艺术中心", "文化广场"],
    "广州": ["广州体育馆", "广州大剧院", "中山纪念堂", "白云国际会议中心", "广州塔"],
    "深圳": ["深圳大运中心", "深圳音乐厅", "深圳大剧院", "华夏艺术中心", "欢乐海岸"],
    "杭州": ["杭州大剧院", "黄龙体育中心", "浙江音乐厅", "杭州奥体中心", "西湖剧院"],
    "成都": ["成都城市音乐厅", "锦城艺术宫", "成都大魔方", "省体育馆", "欢乐谷"],
    "武汉": ["武汉琴台大剧院", "武汉体育中心", "洪山体育馆", "武汉剧院", "琴台音乐厅"],
    "南京": ["南京奥体中心", "南京大剧院", "江苏大剧院", "南京人民大会堂", "南京文化艺术中心"],
    "西安": ["陕西大剧院", "西安城市运动公园", "曲江国际会议中心", "西安音乐厅", "大唐芙蓉园"],
    "重庆": ["重庆大剧院", "重庆国际博览中心", "重庆奥体中心", "重庆文化宫", "解放碑大剧院"],
}


def load_movielens(data_dir: str):
    """加载MovieLens数据"""
    # 加载评分数据
    ratings = pd.read_csv(
        os.path.join(data_dir, "ratings.dat"),
        sep="::",
        names=["user_id", "movie_id", "rating", "timestamp"],
        engine="python",
        encoding="latin-1",
    )
    
    # 加载电影数据
    movies = pd.read_csv(
        os.path.join(data_dir, "movies.dat"),
        sep="::",
        names=["movie_id", "title", "genres"],
        engine="python",
        encoding="latin-1",
    )
    
    # 加载用户数据
    users = pd.read_csv(
        os.path.join(data_dir, "users.dat"),
        sep="::",
        names=["user_id", "gender", "age", "occupation", "zipcode"],
        engine="python",
        encoding="latin-1",
    )
    
    logger.info(f"加载数据: ratings={len(ratings)}, movies={len(movies)}, users={len(users)}")
    
    return ratings, movies, users


def convert_to_events(movies: pd.DataFrame) -> pd.DataFrame:
    """将电影转换为演出活动"""
    events = []
    
    for _, movie in movies.iterrows():
        movie_id = movie["movie_id"]
        title = movie["title"]
        genres = movie["genres"].split("|")
        
        # 选择主要类别
        main_genre = genres[0] if genres else "Drama"
        category = CATEGORY_MAP.get(main_genre, "话剧")
        
        # 随机分配城市
        city = random.choice(CITIES)
        
        # 随机分配场馆
        venue = random.choice(VENUES[city])
        
        # 生成价格 (根据类别)
        base_price = {
            "演唱会": random.randint(300, 1500),
            "音乐会": random.randint(200, 800),
            "音乐剧": random.randint(200, 600),
            "话剧": random.randint(100, 400),
            "展览": random.randint(50, 200),
            "儿童剧": random.randint(80, 300),
            "沉浸式体验": random.randint(200, 500),
        }.get(category, 200)
        
        # 生成活动时间
        days_offset = random.randint(-30, 60)
        event_time = datetime.now() + timedelta(days=days_offset)
        
        # 生成库存
        stock = random.randint(100, 5000)
        
        events.append({
            "event_id": movie_id,
            "title": title,
            "category": category,
            "category_id": list(set(CATEGORY_MAP.values())).index(category),
            "city": city,
            "city_id": CITIES.index(city),
            "venue": venue,
            "price": base_price,
            "event_time": event_time.strftime("%Y-%m-%d %H:%M:%S"),
            "stock": stock,
            "status": 1 if days_offset > -7 else 0,  # 过期的标记为下架
        })
    
    return pd.DataFrame(events)


def convert_to_users(users: pd.DataFrame) -> pd.DataFrame:
    """转换用户数据"""
    result = []
    
    # 年龄段映射
    age_map = {1: 0, 18: 1, 25: 2, 35: 3, 45: 4, 50: 5, 56: 6}
    
    for _, user in users.iterrows():
        user_id = user["user_id"]
        
        # 性别
        gender = 1 if user["gender"] == "M" else 0
        
        # 年龄段
        age_group = age_map.get(user["age"], 2)
        
        # 随机分配城市
        city = random.choice(CITIES)
        
        result.append({
            "user_id": user_id,
            "gender": gender,
            "age_group": age_group,
            "city": city,
            "city_id": CITIES.index(city),
        })
    
    return pd.DataFrame(result)


def convert_to_behaviors(ratings: pd.DataFrame, events: pd.DataFrame) -> pd.DataFrame:
    """转换评分为用户行为"""
    behaviors = []
    
    # 创建event_id到city的映射
    event_city = dict(zip(events["event_id"], events["city_id"]))
    event_category = dict(zip(events["event_id"], events["category_id"]))
    
    for _, rating in ratings.iterrows():
        user_id = rating["user_id"]
        event_id = rating["movie_id"]
        score = rating["rating"]
        timestamp = rating["timestamp"]
        
        # 根据评分生成行为类型
        # 评分 >= 4: 有购买行为
        # 评分 >= 3: 有点击行为
        # 所有: 有浏览行为
        
        # 浏览行为
        behaviors.append({
            "user_id": user_id,
            "event_id": event_id,
            "behavior_type": "view",
            "timestamp": timestamp,
            "city_id": event_city.get(event_id, 0),
            "category_id": event_category.get(event_id, 0),
        })
        
        if score >= 3:
            # 点击行为
            behaviors.append({
                "user_id": user_id,
                "event_id": event_id,
                "behavior_type": "click",
                "timestamp": timestamp + 60,
                "city_id": event_city.get(event_id, 0),
                "category_id": event_category.get(event_id, 0),
            })
        
        if score >= 4:
            # 购买行为
            behaviors.append({
                "user_id": user_id,
                "event_id": event_id,
                "behavior_type": "order",
                "timestamp": timestamp + 120,
                "city_id": event_city.get(event_id, 0),
                "category_id": event_category.get(event_id, 0),
            })
    
    return pd.DataFrame(behaviors)


def generate_training_data(behaviors: pd.DataFrame, events: pd.DataFrame, users: pd.DataFrame):
    """生成训练数据(正负样本)"""
    # 正样本: 有购买行为的
    positive = behaviors[behaviors["behavior_type"] == "order"][["user_id", "event_id"]].copy()
    positive["label"] = 1
    
    # 负样本: 随机采样(用户没有交互过的物品)
    all_events = set(events["event_id"].unique())
    user_events = behaviors.groupby("user_id")["event_id"].apply(set).to_dict()
    
    negative_samples = []
    for user_id, interacted in user_events.items():
        # 采样负样本
        non_interacted = list(all_events - interacted)
        if len(non_interacted) > 0:
            neg_count = min(len(interacted), 10)  # 负样本数量
            neg_events = random.sample(non_interacted, min(neg_count, len(non_interacted)))
            for event_id in neg_events:
                negative_samples.append({
                    "user_id": user_id,
                    "event_id": event_id,
                    "label": 0,
                })
    
    negative = pd.DataFrame(negative_samples)
    
    # 合并正负样本
    train_data = pd.concat([positive, negative], ignore_index=True)
    train_data = train_data.sample(frac=1).reset_index(drop=True)  # 打乱
    
    logger.info(f"训练数据: 正样本={len(positive)}, 负样本={len(negative)}")
    
    return train_data


def main():
    """主函数"""
    # 路径配置
    raw_dir = "data/raw/ml-1m"
    processed_dir = "data/processed"
    
    os.makedirs(processed_dir, exist_ok=True)
    
    # 加载数据
    logger.info("加载MovieLens数据...")
    ratings, movies, users = load_movielens(raw_dir)
    
    # 转换数据
    logger.info("转换活动数据...")
    events = convert_to_events(movies)
    events.to_csv(os.path.join(processed_dir, "events.csv"), index=False)
    logger.info(f"保存活动数据: {len(events)} 条")
    
    logger.info("转换用户数据...")
    users_converted = convert_to_users(users)
    users_converted.to_csv(os.path.join(processed_dir, "users.csv"), index=False)
    logger.info(f"保存用户数据: {len(users_converted)} 条")
    
    logger.info("转换行为数据...")
    behaviors = convert_to_behaviors(ratings, events)
    behaviors.to_csv(os.path.join(processed_dir, "behaviors.csv"), index=False)
    logger.info(f"保存行为数据: {len(behaviors)} 条")
    
    logger.info("生成训练数据...")
    train_data = generate_training_data(behaviors, events, users_converted)
    train_data.to_csv(os.path.join(processed_dir, "train_data.csv"), index=False)
    logger.info(f"保存训练数据: {len(train_data)} 条")
    
    # 统计信息
    logger.info("=" * 50)
    logger.info("数据转换完成!")
    logger.info(f"活动数量: {len(events)}")
    logger.info(f"用户数量: {len(users_converted)}")
    logger.info(f"行为数量: {len(behaviors)}")
    logger.info(f"训练样本: {len(train_data)}")
    logger.info(f"类别分布:")
    for cat, count in events["category"].value_counts().items():
        logger.info(f"  - {cat}: {count}")
    logger.info(f"城市分布:")
    for city, count in events["city"].value_counts().head(5).items():
        logger.info(f"  - {city}: {count}")


if __name__ == "__main__":
    main()

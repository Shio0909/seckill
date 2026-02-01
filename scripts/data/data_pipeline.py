#!/usr/bin/env python3
"""
完整数据处理流水线
一键完成：下载 -> 转换 -> 计算相似度 -> 导入数据库

用法:
    python data_pipeline.py
    python data_pipeline.py --skip-download   # 跳过下载
    python data_pipeline.py --import-mysql    # 导入MySQL
    python data_pipeline.py --import-redis    # 导入Redis
"""

import os
import sys
import argparse
import subprocess
from pathlib import Path


SCRIPT_DIR = Path(__file__).parent
PROJECT_DIR = SCRIPT_DIR.parent.parent
DATA_DIR = PROJECT_DIR / "data"


def run_script(script_name: str, *args) -> bool:
    """运行Python脚本"""
    script_path = SCRIPT_DIR / script_name
    cmd = [sys.executable, str(script_path)] + list(args)
    
    print(f"\n{'='*60}")
    print(f"🚀 运行: {script_name}")
    print('='*60)
    
    result = subprocess.run(cmd, cwd=str(SCRIPT_DIR))
    return result.returncode == 0


def check_processed_data() -> bool:
    """检查处理后的数据是否存在"""
    required_files = [
        "events.csv",
        "users.csv", 
        "behaviors.csv",
        "train.csv",
    ]
    
    processed_dir = DATA_DIR / "processed"
    
    for f in required_files:
        if not (processed_dir / f).exists():
            return False
    
    return True


def import_to_mysql():
    """导入数据到MySQL"""
    print("\n" + "="*60)
    print("📥 导入数据到MySQL...")
    print("="*60)
    
    try:
        import mysql.connector
        import pandas as pd
        
        # 读取配置
        # 这里使用默认配置，实际应该从config.yaml读取
        conn = mysql.connector.connect(
            host='localhost',
            port=3307,
            user='root',
            password='secret',
            database='seckill'
        )
        cursor = conn.cursor()
        
        # 读取活动数据
        events_df = pd.read_csv(DATA_DIR / "processed" / "events.csv")
        print(f"   读取 {len(events_df)} 条活动数据")
        
        # 导入活动数据
        imported = 0
        for _, event in events_df.iterrows():
            try:
                sql = """
                INSERT INTO products (id, name, category_id, city_id, city, venue, 
                                     price, high_price, stock, tags, hot_score, 
                                     created_at, updated_at)
                VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, NOW(), NOW())
                ON DUPLICATE KEY UPDATE 
                    name=VALUES(name), 
                    hot_score=VALUES(hot_score)
                """
                cursor.execute(sql, (
                    int(event['event_id']),
                    event['name'][:100],  # 截断名称
                    int(event['category_id']),
                    int(event['city_id']),
                    event['city'],
                    event['venue'],
                    float(event['price']),
                    float(event['high_price']),
                    int(event['stock']),
                    event['tags'][:200] if pd.notna(event['tags']) else '',
                    int(event['hot_score']),
                ))
                imported += 1
            except Exception as e:
                print(f"   ⚠️ 跳过 {event['event_id']}: {e}")
        
        conn.commit()
        print(f"   ✅ 导入 {imported} 条活动数据")
        
        # 读取用户数据
        users_df = pd.read_csv(DATA_DIR / "processed" / "users.csv")
        print(f"   读取 {len(users_df)} 条用户数据")
        
        # 导入用户数据
        imported = 0
        for _, user in users_df.iterrows():
            try:
                sql = """
                INSERT INTO users (id, username, password, phone, email, 
                                  city_id, city, gender, age, created_at, updated_at)
                VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, NOW(), NOW())
                ON DUPLICATE KEY UPDATE 
                    city_id=VALUES(city_id),
                    gender=VALUES(gender),
                    age=VALUES(age)
                """
                cursor.execute(sql, (
                    int(user['user_id']),
                    f"user_{user['user_id']}",
                    "hashed_password",  # 占位
                    f"138{user['user_id']:08d}",
                    f"user_{user['user_id']}@example.com",
                    int(user['city_id']),
                    user['city'],
                    int(user['gender']),
                    int(user['age']),
                ))
                imported += 1
            except Exception as e:
                if 'Duplicate' not in str(e):
                    print(f"   ⚠️ 跳过用户 {user['user_id']}: {e}")
        
        conn.commit()
        print(f"   ✅ 导入 {imported} 条用户数据")
        
        cursor.close()
        conn.close()
        
        return True
        
    except ImportError:
        print("   ❌ 需要安装: pip install mysql-connector-python pandas")
        return False
    except Exception as e:
        print(f"   ❌ MySQL导入失败: {e}")
        return False


def import_to_redis():
    """导入数据到Redis"""
    print("\n" + "="*60)
    print("📥 导入数据到Redis...")
    print("="*60)
    
    try:
        import redis
        import pandas as pd
        
        rdb = redis.Redis(host='localhost', port=6379, db=0, decode_responses=True)
        
        # 测试连接
        rdb.ping()
        
        # 导入热门活动到ZSet
        events_df = pd.read_csv(DATA_DIR / "processed" / "events.csv")
        
        hot_key = "hot:products"
        rdb.delete(hot_key)
        
        for _, event in events_df.iterrows():
            rdb.zadd(hot_key, {str(int(event['event_id'])): event['hot_score']})
        
        print(f"   ✅ 导入热门榜单: {len(events_df)} 条")
        
        # 按城市导入热门活动
        for city_id in events_df['city_id'].unique():
            city_events = events_df[events_df['city_id'] == city_id]
            city_key = f"hot:city:{int(city_id)}"
            rdb.delete(city_key)
            
            for _, event in city_events.iterrows():
                rdb.zadd(city_key, {str(int(event['event_id'])): event['hot_score']})
        
        print(f"   ✅ 导入城市热门榜单: {len(events_df['city_id'].unique())} 个城市")
        
        # 按类别导入热门活动
        for cat_id in events_df['category_id'].unique():
            cat_events = events_df[events_df['category_id'] == cat_id]
            cat_key = f"hot:category:{int(cat_id)}"
            rdb.delete(cat_key)
            
            for _, event in cat_events.iterrows():
                rdb.zadd(cat_key, {str(int(event['event_id'])): event['hot_score']})
        
        print(f"   ✅ 导入类别热门榜单: {len(events_df['category_id'].unique())} 个类别")
        
        # 导入ItemCF相似度矩阵
        sim_file = DATA_DIR / "processed" / "item_similarity.csv"
        if sim_file.exists():
            sim_df = pd.read_csv(sim_file)
            
            current_item = None
            batch = {}
            imported = 0
            
            for _, row in sim_df.iterrows():
                item_i = int(row['item_i'])
                
                if current_item != item_i:
                    if current_item is not None and batch:
                        key = f"cf:item:{current_item}:similar"
                        rdb.zadd(key, batch)
                        imported += 1
                    
                    current_item = item_i
                    batch = {}
                
                batch[str(int(row['item_j']))] = row['similarity']
            
            if current_item is not None and batch:
                key = f"cf:item:{current_item}:similar"
                rdb.zadd(key, batch)
                imported += 1
            
            print(f"   ✅ 导入ItemCF相似度: {imported} 个物品")
        
        return True
        
    except ImportError:
        print("   ❌ 需要安装: pip install redis pandas")
        return False
    except redis.ConnectionError:
        print("   ❌ Redis连接失败，请检查Redis是否运行")
        return False
    except Exception as e:
        print(f"   ❌ Redis导入失败: {e}")
        return False


def main():
    parser = argparse.ArgumentParser(description="数据处理流水线")
    parser.add_argument("--skip-download", action="store_true", help="跳过下载步骤")
    parser.add_argument("--import-mysql", action="store_true", help="导入MySQL")
    parser.add_argument("--import-redis", action="store_true", help="导入Redis")
    parser.add_argument("--import-all", action="store_true", help="导入所有数据库")
    
    args = parser.parse_args()
    
    print("="*60)
    print("🚀 推荐系统数据处理流水线")
    print("="*60)
    
    # 步骤1: 下载数据
    if not args.skip_download:
        if not run_script("download_datasets.py", "-d", "movielens", "-v", "1m"):
            print("❌ 下载失败")
            return 1
    
    # 步骤2: 转换数据
    if not check_processed_data():
        if not run_script("transform_movielens.py"):
            print("❌ 转换失败")
            return 1
    else:
        print("\n✅ 处理后的数据已存在，跳过转换")
    
    # 步骤3: 计算ItemCF
    sim_file = DATA_DIR / "processed" / "item_similarity.csv"
    if not sim_file.exists():
        if not run_script("compute_item_cf.py"):
            print("❌ 计算ItemCF失败")
            return 1
    else:
        print("\n✅ ItemCF相似度已存在，跳过计算")
    
    # 步骤4: 导入数据库
    if args.import_all or args.import_mysql:
        import_to_mysql()
    
    if args.import_all or args.import_redis:
        import_to_redis()
    
    # 完成
    print("\n" + "="*60)
    print("✅ 数据处理流水线完成!")
    print("="*60)
    print(f"\n数据目录: {DATA_DIR / 'processed'}")
    print("\n生成的文件:")
    print("  - events.csv      活动数据")
    print("  - users.csv       用户数据")
    print("  - behaviors.csv   行为数据")
    print("  - train.csv       训练数据")
    print("  - item_similarity.csv  ItemCF相似度")
    
    if not (args.import_mysql or args.import_redis or args.import_all):
        print("\n💡 提示: 使用 --import-all 导入数据到MySQL和Redis")
    
    return 0


if __name__ == "__main__":
    sys.exit(main())

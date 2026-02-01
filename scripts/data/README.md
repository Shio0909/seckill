# 数据处理脚本

这个目录包含推荐系统的数据处理脚本。

## 快速开始

```bash
# 1. 安装依赖
pip install -r requirements.txt

# 2. 一键运行流水线（下载 + 转换 + 计算相似度）
python data_pipeline.py

# 3. 导入到数据库
python data_pipeline.py --import-all
```

## 脚本说明

| 脚本 | 功能 |
|------|------|
| `download_datasets.py` | 下载MovieLens数据集 |
| `transform_movielens.py` | 将MovieLens转换为票务场景数据 |
| `transform_kaggle_events.py` | 转换Kaggle活动推荐数据集 |
| `compute_item_cf.py` | 计算ItemCF相似度矩阵 |
| `data_pipeline.py` | 完整数据处理流水线 |

## 数据集选择

### 推荐：MovieLens 1M（已集成自动下载）

- 6,040 用户
- 3,706 电影 → 转换为活动
- 1,000,000 评分 → 转换为行为

```bash
python download_datasets.py -d movielens -v 1m
```

### 最佳选择：Kaggle Event Recommendation（需手动下载）

- 3,900,000+ 用户
- 3,100,000+ 活动
- 真实的活动推荐场景

下载地址：https://www.kaggle.com/c/event-recommendation-engine-challenge/data

下载后放到 `data/kaggle-events/` 目录，然后运行：

```bash
python transform_kaggle_events.py
```

## 生成的数据

处理后的数据保存在 `data/processed/` 目录：

```
data/processed/
├── events.csv           # 活动数据
├── users.csv            # 用户数据  
├── behaviors.csv        # 行为数据（浏览/点击/购买）
├── train.csv            # 训练数据（带特征和标签）
├── item_similarity.csv  # ItemCF相似度矩阵
└── import_cf_to_redis.py # Redis导入脚本
```

## 数据字段说明

### events.csv

| 字段 | 类型 | 说明 |
|------|------|------|
| event_id | int | 活动ID |
| name | string | 活动名称 |
| category_id | int | 类别ID (1-5) |
| category_name | string | 类别名称 |
| city_id | int | 城市ID |
| city | string | 城市名称 |
| venue | string | 场馆 |
| price | float | 最低票价 |
| high_price | float | 最高票价 |
| stock | int | 库存 |
| tags | string | 标签 |
| hot_score | int | 热度分数 |
| event_time | datetime | 活动时间 |

### users.csv

| 字段 | 类型 | 说明 |
|------|------|------|
| user_id | int | 用户ID |
| gender | int | 性别 (0=女, 1=男) |
| age | int | 年龄 |
| city_id | int | 所在城市ID |
| city | string | 所在城市 |
| occupation | int | 职业编码 |

### behaviors.csv

| 字段 | 类型 | 说明 |
|------|------|------|
| user_id | int | 用户ID |
| event_id | int | 活动ID |
| behavior_type | string | 行为类型 (view/click/purchase) |
| weight | float | 权重 |
| rating | float | 原始评分 |
| timestamp | int | 时间戳 |

### train.csv（带特征）

| 字段 | 类型 | 说明 |
|------|------|------|
| user_id | int | 用户ID |
| event_id | int | 活动ID |
| label | int | 标签 (0/1) |
| gender | int | 用户性别 |
| age | int | 用户年龄 |
| city_id | int | 用户城市 |
| category_id | int | 活动类别 |
| event_city_id | int | 活动城市 |
| price | float | 票价 |
| hot_score | int | 热度 |
| city_match | int | 城市是否匹配 |

## 导入数据库

### 导入MySQL

```bash
python data_pipeline.py --import-mysql
```

会导入：
- 活动数据到 `products` 表
- 用户数据到 `users` 表

### 导入Redis

```bash
python data_pipeline.py --import-redis
```

会导入：
- 热门榜单到 `hot:products` (ZSet)
- 城市热门到 `hot:city:{city_id}` (ZSet)
- 类别热门到 `hot:category:{cat_id}` (ZSet)
- ItemCF相似度到 `cf:item:{item_id}:similar` (ZSet)

## 面试话术

> Q: 你的推荐系统用什么数据训练的？
>
> 主数据是 **MovieLens 1M**，这是学术界广泛使用的推荐系统基准数据集。为了适配票务场景，我做了领域适配：
> - 电影类型 → 活动类别（演唱会、体育、展览等）
> - 用户评分 → 用户行为（评分≥4=购买，3=点击，<3=浏览）
> - 添加了城市、场馆、价格等业务特征
>
> 补充使用 **Kaggle Event Recommendation** 数据集验证算法效果，这是真实的活动推荐场景。

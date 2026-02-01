# 智能推荐系统微服务

基于 Python 实现的票务推荐系统，采用经典的三层架构（召回→排序→重排），通过 gRPC 与 Go 主服务通信。

## 🎯 核心特性

- **多路召回**: ItemCF、UserCF、向量召回、热门召回、标签召回
- **精排模型**: LightGBM CTR 预估模型
- **多样性重排**: MMR 算法平衡相关性与多样性
- **冷启动策略**: 分级冷启动，新用户也能获得合理推荐
- **AB 测试**: 内置 AB 测试框架，支持多实验并行

## 📁 项目结构

```
services/recommendation/
├── config/                 # 配置文件
│   └── config.example.yaml
├── data/                   # 数据目录
│   ├── raw/               # 原始数据 (MovieLens)
│   └── processed/         # 处理后数据
├── proto/                  # gRPC 定义
│   └── recommendation.proto
├── scripts/                # 离线脚本
│   ├── convert_movielens.py   # 数据转换
│   ├── train_lgb.py           # 模型训练
│   └── compute_similarity.py  # 相似度计算
├── src/                    # 源代码
│   ├── recall/            # 召回层
│   ├── ranking/           # 排序层
│   ├── rerank/            # 重排层
│   ├── data/              # 数据层
│   ├── service/           # 业务服务
│   ├── server/            # gRPC服务
│   ├── config.py          # 配置管理
│   └── main.py            # 入口
├── Dockerfile
├── docker-compose.yml
├── requirements.txt
└── README.md
```

## 🚀 快速开始

### 1. 环境准备

```bash
# 创建虚拟环境
python -m venv .venv
source .venv/bin/activate  # Linux/Mac
# .venv\Scripts\activate   # Windows

# 安装依赖
pip install -r requirements.txt
```

### 2. 准备数据

```bash
# 下载 MovieLens 1M 数据集
# https://grouplens.org/datasets/movielens/1m/

# 解压到 data/raw/ml-1m/
mkdir -p data/raw
unzip ml-1m.zip -d data/raw/

# 转换数据格式
python scripts/convert_movielens.py
```

### 3. 训练模型

```bash
# 计算 ItemCF 相似度矩阵
python scripts/compute_similarity.py

# 训练 LightGBM 排序模型
python scripts/train_lgb.py
```

### 4. 启动服务

```bash
# 使用 Docker Compose (推荐)
docker-compose up -d

# 或直接启动
python -m src.main
```

### 5. 生成 gRPC 代码

```bash
# Python
python -m grpc_tools.protoc \
    -I./proto \
    --python_out=./proto \
    --grpc_python_out=./proto \
    ./proto/recommendation.proto

# Go (在项目根目录执行)
protoc --go_out=. --go-grpc_out=. proto/recommendation.proto
```

## 🔧 配置说明

复制配置模板并修改:

```bash
cp config/config.example.yaml config/config.yaml
```

主要配置项:

| 配置项 | 说明 | 默认值 |
|--------|------|--------|
| `grpc.port` | gRPC 服务端口 | 50052 |
| `redis.host` | Redis 地址 | localhost |
| `milvus.host` | Milvus 地址 | localhost |
| `recall.item_cf.weight` | ItemCF 召回权重 | 0.30 |
| `ranking.model_path` | 排序模型路径 | src/model/models/lgb_ctr_v1.pkl |
| `rerank.diversity.lambda` | MMR 多样性参数 | 0.7 |

## 📊 架构设计

### 三层架构

```
┌─────────────────────────────────────────────────────────────┐
│                      gRPC 请求                              │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                     召回层 (Recall)                          │
│  ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────┐│
│  │ ItemCF  │ │ UserCF  │ │ Vector  │ │   Hot   │ │   Tag   ││
│  │  30%    │ │  20%    │ │  25%    │ │  15%    │ │  10%    ││
│  └────┬────┘ └────┬────┘ └────┬────┘ └────┬────┘ └────┬────┘│
│       └──────────┬┴───────────┴───────────┴───────────┘     │
│                  ▼                                          │
│           RecallMerger (去重 + 冷启动策略)                    │
└─────────────────────────────────────────────────────────────┘
                              │ ~500 候选
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                     排序层 (Ranking)                         │
│  ┌─────────────────┐     ┌─────────────────┐                │
│  │  FeatureBuilder │────▶│  LightGBM CTR   │                │
│  │   30维特征      │     │   预估模型       │                │
│  └─────────────────┘     └─────────────────┘                │
└─────────────────────────────────────────────────────────────┘
                              │ TopK by score
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                     重排层 (Rerank)                          │
│  ┌─────────────────┐     ┌─────────────────┐                │
│  │ BusinessFilter  │────▶│  MMRDiversifier │                │
│  │   业务规则       │     │   多样性重排     │                │
│  └─────────────────┘     └─────────────────┘                │
└─────────────────────────────────────────────────────────────┘
                              │ 最终推荐
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                      gRPC 响应                              │
└─────────────────────────────────────────────────────────────┘
```

### 冷启动策略

| 行为数 | 策略 |
|--------|------|
| < 5 | 热门 + 标签召回 |
| 5-20 | 热门 + ItemCF |
| > 20 | 全链路召回 |

## 🧪 测试

```bash
# 单元测试
pytest tests/ -v

# 集成测试
pytest tests/integration/ -v

# 压力测试 (需要启动服务)
python -m grpc_tools.protoc # 先生成代码
python tests/benchmark.py
```

## 📈 监控

服务暴露 Prometheus 指标:

- `recommend_request_total`: 请求总数
- `recommend_request_latency_seconds`: 请求延迟
- `recall_items_total`: 召回物品数
- `ranking_score_histogram`: 排序分数分布

## 🔗 与 Go 服务集成

在 Go Gateway 中调用推荐服务:

```go
import "seckill/pkg/recommendation"

// 创建客户端
client, _ := recommendation.NewClient(&recommendation.Config{
    Address: "localhost:50052",
    Timeout: time.Second * 3,
})

// 获取推荐
result, _ := client.GetRecommendations(ctx, userID, "home", 20, "北京", 0, false)

// 记录行为
client.RecordBehavior(ctx, userID, eventID, "click")
```

## 📝 开发计划

- [x] 基础架构搭建
- [x] 召回层实现 (ItemCF, Hot, Vector)
- [x] 排序层实现 (LightGBM)
- [x] 重排层实现 (MMR, 业务规则)
- [ ] DeepFM 模型集成
- [ ] 实时特征更新
- [ ] AB 测试框架完善
- [ ] 向量召回优化 (Embedding 训练)

## 📚 参考资料

- [推荐系统实践](https://book.douban.com/subject/10769749/)
- [深度学习推荐系统](https://book.douban.com/subject/35013197/)
- [LightGBM Documentation](https://lightgbm.readthedocs.io/)
- [Milvus Documentation](https://milvus.io/docs)

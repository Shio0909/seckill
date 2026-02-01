# 排序服务 (Ranking Service)

精简版的Python微服务，**只负责CTR模型推理**。

## 架构定位

```
Go (召回+重排) ──gRPC──▶ Python (排序) ──gRPC──▶ Go (返回结果)
     │                       │
     │                       ├── LightGBM推理
     │                       └── (可选) DeepFM推理
     │
     ├── 召回: ItemCF/Hot/Vector
     ├── 特征读取: Redis批量查询
     └── 重排: MMR多样性+业务规则
```

## 为什么这样设计？

| 模块 | 语言 | 原因 |
|-----|------|-----|
| 召回 | **Go** | I/O密集(查Redis/Milvus)，goroutine并发能力强 |
| 排序 | **Python** | 计算密集(矩阵运算)，复用ML生态(LightGBM/PyTorch) |
| 重排 | **Go** | CPU密集(循环/判断)，Go执行速度快 |

## 接口定义

```protobuf
service RankingService {
    rpc Rank(RankRequest) returns (RankResponse);
}

message RankItem {
    int64 event_id = 1;
    double recall_score = 2;
    string recall_source = 3;
    repeated double features = 4;  // Go端构建的特征向量
    double rank_score = 5;         // Python返回的CTR分数
}
```

## 快速开始

```bash
# 1. 安装依赖
pip install -r requirements.txt

# 2. 生成gRPC代码
python -m grpc_tools.protoc -I. --python_out=. --grpc_python_out=. ranking.proto

# 3. 启动服务
python server.py
```

## Docker

```bash
docker build -t ranking-service .
docker run -p 50052:50052 -v ./models:/app/models ranking-service
```

## 模型文件

将训练好的模型放到 `models/` 目录：

```
models/
└── lgb_ctr_v1.pkl  # LightGBM模型
```

## 性能

- 单次请求: ~5ms (500个候选)
- QPS: ~200 (单进程)
- 可通过Gunicorn多进程扩展

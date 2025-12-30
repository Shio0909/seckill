# 🎫 DaMai-Go：高并发票务秒杀系统（Go 微服务版）

> 仿大麦网高并发票务系统的 Go 语言实现，采用 **gRPC 微服务架构**，支持百万级并发秒杀场景

[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Build Status](https://img.shields.io/badge/build-passing-brightgreen.svg)]()
[![Architecture](https://img.shields.io/badge/Architecture-Microservices-orange.svg)]()
[![gRPC](https://img.shields.io/badge/gRPC-Protocol%20Buffers-blue.svg)](https://grpc.io/)

---

## 📑 目录

- [项目简介](#-项目简介)
- [系统架构](#-系统架构)
- [项目现状分析](#-项目现状分析)
- [技术选型对比](#-技术选型对比)
- [接口规划](#-接口规划)
- [分阶段开发计划](#-分阶段开发计划)
- [微服务拆分方案](#-微服务拆分方案)
- [快速启动](#-快速启动)
- [项目结构](#-项目结构)

---

## 🎯 项目简介

本项目是一个**生产级高并发票务秒杀系统**，参考大麦网架构设计，使用 Go 语言从零实现。项目采用 **gRPC 微服务架构**，涵盖了后端开发中的核心技术栈：

- **微服务架构**：gRPC 服务间通信、Consul 服务发现、API Gateway 统一入口
- **高并发处理**：Redis Lua 脚本原子扣减、消息队列异步削峰、熔断器防雪崩
- **分布式组件**：分布式锁（Redis）、延迟队列（订单超时）、分布式 ID（雪花算法）
- **可观测性**：Prometheus 指标监控、结构化日志、链路追踪（规划中）
- **云原生部署**：Docker 容器化、Kubernetes 编排、服务健康检查

### 核心业务场景

```
用户 -> API Gateway -> gRPC 服务调用链
         (限流/鉴权)
              │
              ├─> User Service (用户认证)
              │
              ├─> Product Service (商品查询)
              │
              └─> Seckill Service (秒杀核心)
                      │
                      ├─> Redis 预扣库存 (Lua 原子操作)
                      │
                      └─> RabbitMQ 异步下单 -> Order Service
                              │
                              └─> 库存不足/已购买 -> 快速失败返回
```

### 🆕 Phase 2 & 3 架构升级

**Phase 2 已完成功能**：
- ✅ **服务拆分**：User、Product、Order、Seckill 四大微服务
- ✅ **gRPC 通信**：Protocol Buffers 定义接口，高性能 RPC 调用
- ✅ **服务发现**：Consul 注册中心，自动服务注册与健康检查
- ✅ **API Gateway**：HTTP 到 gRPC 协议转换，统一鉴权与路由
- ✅ **配置中心**：Consul KV 存储，支持动态配置
- ✅ **熔断器**：三状态熔断器（Closed/Open/Half-Open），防止级联故障
- ✅ **单元测试**：熔断器、Consul 客户端完整测试覆盖

**Phase 3 已完成功能**：
- ✅ **分布式锁**：基于 Redis + Lua 脚本，支持看门狗自动续期
- ✅ **延迟队列**：基于 Redis ZSET，支持订单超时自动取消
- ✅ **Prometheus 监控**：HTTP/业务/中间件全方位指标采集

---

## 🏗 系统架构

### 当前架构图（Phase 2 - 微服务架构）

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              客户端层                                        │
│                    PC / APP / 小程序 / H5                                    │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │ HTTP/HTTPS
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                          API Gateway (Port 8080)                             │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  • HTTP → gRPC 协议转换                                              │    │
│  │  • JWT 鉴权 & 路由转发                                               │    │
│  │  • 统一错误处理                                                      │    │
│  │  • 请求日志记录                                                      │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │ gRPC
                    ┌───────────────┼───────────────┐
                    ▼               ▼               ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                            微服务层 (gRPC)                                   │
│                                                                              │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐   │
│  │ User Service │  │Product Service│  │Order Service │  │Seckill Service│  │
│  │  Port 50051  │  │  Port 50052   │  │  Port 50053  │  │  Port 50054   │  │
│  ├──────────────┤  ├──────────────┤  ├──────────────┤  ├──────────────┤   │
│  │• 用户注册登录 │  │• 商品 CRUD    │  │• 订单创建    │  │• 库存预扣减   │   │
│  │• Token 验证  │  │• 库存预热     │  │• 订单查询    │  │• 防重复购买   │   │
│  │• 用户信息    │  │• 商品列表     │  │• 雪花 ID     │  │• MQ 异步下单  │   │
│  └──────────────┘  └──────────────┘  └──────────────┘  └──────────────┘   │
│         │                  │                  │                  │          │
│         └──────────────────┴──────────────────┴──────────────────┘          │
│                                    │                                         │
└────────────────────────────────────┼─────────────────────────────────────────┘
                                     │
        ┌────────────────────────────┼────────────────────────────┐
        ▼                            ▼                            ▼
┌──────────────────┐      ┌──────────────────┐      ┌──────────────────┐
│   服务治理层      │      │     消息层        │      │     存储层        │
│                  │      │                  │      │                  │
│ ┌──────────────┐ │      │ ┌──────────────┐ │      │ ┌──────────────┐ │
│ │   Consul     │ │      │ │  RabbitMQ    │ │      │ │    MySQL     │ │
│ │ Port 8500    │ │      │ │  Port 6672   │ │      │ │  Port 3307   │ │
│ ├──────────────┤ │      │ ├──────────────┤ │      │ ├──────────────┤ │
│ │• 服务注册    │ │      │ │• 秒杀队列    │ │      │ │• 用户表      │ │
│ │• 服务发现    │ │      │ │• 异步削峰    │ │      │ │• 商品表      │ │
│ │• 健康检查    │ │      │ │• 消息持久化  │ │      │ │• 订单表      │ │
│ │• KV 配置中心 │ │      │ └──────────────┘ │      │ └──────────────┘ │
│ └──────────────┘ │      │                  │      │ ┌──────────────┐ │
│                  │      │                  │      │ │    Redis     │ │
│ ┌──────────────┐ │      │                  │      │ │  Port 6379   │ │
│ │ 熔断器管理器  │ │      │                  │      │ ├──────────────┤ │
│ ├──────────────┤ │      │                  │      │ │• 库存缓存    │ │
│ │• Closed      │ │      │                  │      │ │• Lua 脚本    │ │
│ │• Open        │ │      │                  │      │ │• 购买记录    │ │
│ │• Half-Open   │ │      │                  │      │ └──────────────┘ │
│ └──────────────┘ │      │                  │      │                  │
└──────────────────┘      └──────────────────┘      └──────────────────┘
```

### 服务调用链示例

```
秒杀请求流程：
1. 客户端 → API Gateway (HTTP POST /api/seckill)
2. Gateway → Seckill Service (gRPC SeckillProduct)
3. Seckill Service → Redis (Lua 脚本扣减库存)
4. Seckill Service → RabbitMQ (发送订单消息)
5. Order Service ← RabbitMQ (消费消息创建订单)
6. Order Service → MySQL (持久化订单)
7. Gateway ← Seckill Service (返回秒杀结果)
8. 客户端 ← Gateway (HTTP 响应)
```

---

## 各系统

### 配置系统

┌─────────────────────────────────────────────────────────────────┐
│                        配置系统架构                              │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│   config.yaml  ──┐                                              │
│                  │                                              │
│   环境变量 ──────┼──→  Viper  ──→  Config结构体  ──→  全局访问   │
│                  │      ↑                                       │
│   默认值 ────────┘      │                                       │
│                         │                                       │
│                    fsnotify (文件监听 → 热更新)                  │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘





## 📊 项目现状分析

### ✅ 已实现功能

| 模块 | 功能点 | 实现位置 | 完成度 |
|------|--------|----------|--------|
| **Web 框架** | Gin HTTP 服务 | `cmd/main.go`, `internal/router/` | ✅ 100% |
| **用户认证** | JWT Token 鉴权 | `internal/middleware/auth.go` | ✅ 100% |
| **日志系统** | Zap 结构化日志 | `pkg/logger/`, `middleware/logger.go` | ✅ 100% |
| **数据库** | MySQL + GORM | `pkg/database/mysql.go` | ✅ 100% |
| **缓存** | Redis 连接池 | `pkg/redis/redis.go` | ✅ 100% |
| **秒杀核心** | Redis Lua 原子扣减 | `pkg/redis/scripts.go` | ✅ 100% |
| **防重复购买** | Redis Set 记录 | Lua 脚本内 `SISMEMBER` | ✅ 100% |
| **消息队列** | RabbitMQ 异步下单 | `pkg/rabbitmq/`, `service/consumer.go` | ✅ 100% |
| **分布式 ID** | 雪花算法 | `pkg/snowflake/` | ✅ 100% |
| **API 文档** | Swagger/OpenAPI | `docs/`, `/swagger/*` 路由 | ✅ 100% |
| **容器编排** | K8s 基础部署文件 | `deploy/k8s/` | ⚠️ 50% |
| **跨域处理** | CORS 中间件 | `internal/middleware/cors.go` | ✅ 100% |
| **API 网关** | 统一入口、路由、限流 | `services/gateway/` | ✅ 100% |
| **服务注册发现** | Consul | `pkg/consul/` | ✅ 100% |
| **配置中心** | Consul KV | `pkg/config/` | ✅ 100% |
| **熔断降级** | gobreaker | `pkg/breaker/` | ✅ 100% |
| **分布式锁** | Redis + Lua | `pkg/distlock/` | ✅ 100% |
| **延迟队列** | Redis ZSET | `pkg/delayqueue/` | ✅ 100% |
| **监控告警** | Prometheus | `pkg/metrics/` | ✅ 100% |
| **优雅停机** | 信号处理 | `cmd/main.go` | ✅ 100% |
| **接口幂等** | Redis | `internal/middleware/idempotent.go` | ✅ 100% |
| **链路追踪** | OpenTelemetry + Jaeger | `pkg/tracing/` | ✅ 100% |
| **消息队列抽象** | Broker 接口（RabbitMQ/Kafka） | `pkg/broker/` | ✅ 100% |
| **压测脚本** | K6 压测 | `scripts/k6/` | ✅ 100% |
| **CI/CD** | GitHub Actions | `.github/workflows/` | ✅ 100% |

### ❌ 待完善功能

| 模块 | 缺失功能 | 重要程度 | 简历加分 |
|------|----------|----------|----------|
| **分布式事务** | Saga/TCC/Seata | ⭐⭐⭐ | 高 |
| **数据库读写分离** | MySQL 主从 | ⭐⭐⭐ | 中 |
| **分库分表** | 应用层路由/ShardingSphere | ⭐⭐⭐ | 高 |
| **支付对接** | 支付宝/微信沙箱 | ⭐⭐⭐ | 中 |
| **ELK 日志** | Elasticsearch 日志收集 | ⭐⭐⭐ | 中 |

---

## 🔧 技术选型对比

### 1. 消息队列：Kafka vs RabbitMQ

| 对比维度 | RabbitMQ（当前） | Kafka（推荐升级） |
|----------|------------------|-------------------|
| **吞吐量** | 万级 QPS | 百万级 QPS |
| **延迟** | 微秒级（更低） | 毫秒级 |
| **消息模型** | 队列模型，灵活路由 | 日志模型，分区消费 |
| **持久化** | 支持但性能下降 | 天然持久化，高性能 |
| **消息回溯** | 不支持 | 支持（可重新消费） |
| **水平扩展** | 较复杂 | 原生支持 |
| **运维复杂度** | 简单 | 较复杂（需 ZK/KRaft） |
| **Go 客户端** | `rabbitmq/amqp091-go` | `segmentio/kafka-go`, `Shopify/sarama` |

**选型建议**：
- **短期**：保留 RabbitMQ，抽象消息层接口（`MessageBroker`）
- **中期**：引入 Kafka，支持双写过渡
- **理由**：秒杀场景峰值流量大，Kafka 的分区机制和高吞吐更适合；消息可回溯便于故障恢复和数据分析

**不选其他方案的原因**：
- RocketMQ：Java 生态为主，Go SDK 成熟度略低
- Pulsar：架构先进但运维复杂，社区规模小于 Kafka
- Redis Streams：轻量但大规模场景不如专业 MQ

---

### 2. 服务注册发现：Consul vs Nacos vs etcd

| 对比维度 | Consul | Nacos | etcd |
|----------|--------|-------|------|
| **语言** | Go | Java | Go |
| **功能** | 服务发现 + KV + 健康检查 | 服务发现 + 配置中心 | 分布式 KV |
| **一致性** | CP (Raft) | AP/CP 可切换 | CP (Raft) |
| **配置中心** | 需配合 Vault | 内置 | 需二次开发 |
| **Go SDK** | 官方支持，成熟 | 社区 SDK | 官方支持 |
| **K8s 集成** | 良好 | 良好 | 原生（K8s 底层） |

**选型建议**：**Consul**

**理由**：
1. Go 原生实现，与 Go 项目契合度高
2. 内置健康检查、KV 存储，功能完整
3. HashiCorp 生态（Vault、Nomad）无缝集成
4. 文档丰富，社区活跃

**不选其他方案的原因**：
- Nacos：Java 实现，Go SDK 非官方，配置中心功能强但对 Go 友好度一般 //但nacos好像国内大厂用的多 
- etcd：功能单一，需要额外开发服务发现逻辑

---

### 3. 限流熔断：go-resilience vs Sentinel-Go vs gobreaker

| 对比维度 | Sentinel-Go | gobreaker | go-resilience |
|----------|-------------|-----------|---------------|
| **来源** | 阿里巴巴 | Sony | 社区 |
| **功能** | 限流 + 熔断 + 热点 | 仅熔断 | 限流 + 熔断 + 重试 |
| **规则配置** | 动态（支持 Nacos） | 静态 | 静态 |
| **监控** | 内置 Dashboard | 无 | 无 |
| **复杂度** | 中等 | 简单 | 简单 |

**选型建议**：**Sentinel-Go**（核心限流）+ **gobreaker**（简单熔断）

**理由**：
1. Sentinel-Go 功能全面，支持 QPS 限流、热点参数限流
2. 可与 Nacos 动态配置联动
3. gobreaker 作为轻量补充，用于外部服务调用

---

### 4. API 网关：自建 vs Kong vs APISIX

| 对比维度 | 自建 (Go) | Kong | APISIX |
|----------|-----------|------|--------|
| **技术栈** | Go | Lua + Nginx | Lua + Nginx |
| **性能** | 高 | 高 | 更高 |
| **插件生态** | 需自建 | 丰富 | 丰富 |
| **学习成本** | 低 | 中 | 中 |
| **可控性** | 完全可控 | 依赖插件 | 依赖插件 |

**选型建议**：**自建轻量网关** + **Nginx 负载均衡**

**理由**：
1. 项目学习目的，自建可深入理解网关原理
2. Go 实现性能优秀，代码可控
3. 后期可平滑迁移到 Kong/APISIX

**自建网关核心功能**：
- 路由转发
- JWT 鉴权
- 限流（令牌桶/滑动窗口）
- 请求日志
- 灰度发布（按 Header/用户 ID 分流）

---

### 5. 链路追踪：Jaeger vs Zipkin vs SkyWalking

| 对比维度 | Jaeger | Zipkin | SkyWalking |
|----------|--------|--------|------------|
| **语言** | Go | Java | Java |
| **协议** | OpenTelemetry | OpenTelemetry | 私有 + OT |
| **存储** | ES/Cassandra/Kafka | ES/MySQL | ES/H2 |
| **Go SDK** | 官方 | 社区 | Agent 方式 |
| **UI** | 功能全面 | 简洁 | 最丰富 |

**选型建议**：**Jaeger** + **OpenTelemetry**

**理由**：
1. CNCF 毕业项目，云原生标准
2. Go 官方 SDK，集成简单
3. 与 Kubernetes 生态契合

---

### 6. 分布式锁：Redis vs etcd vs Zookeeper

| 对比维度 | Redis (Redlock) | etcd | Zookeeper |
|----------|-----------------|------|-----------|
| **性能** | 最高 | 高 | 中 |
| **一致性** | AP（需 Redlock） | CP | CP |
| **Go SDK** | go-redis | 官方 | go-zookeeper |
| **运维** | 简单 | 中等 | 复杂 |

**选型建议**：**Redis 分布式锁**（使用 `go-redsync`）

**理由**：
1. 已有 Redis 基础设施
2. 性能最高，满足秒杀场景
3. 实现简单，`go-redsync` 库成熟

---

### 7. 监控方案：Prometheus + Grafana

**技术栈**：
- **Prometheus**：指标采集、存储、告警规则
- **Grafana**：可视化 Dashboard
- **AlertManager**：告警通知（钉钉/企微/邮件）

**关键监控指标**：
```
# 业务指标
seckill_request_total          # 秒杀请求总数
seckill_success_total          # 秒杀成功数
seckill_fail_reason{reason=""}  # 失败原因分布
order_create_duration_seconds  # 订单创建耗时

# 系统指标  
go_goroutines                  # Goroutine 数量
process_cpu_seconds_total      # CPU 使用
process_resident_memory_bytes  # 内存使用

# 中间件指标
redis_pool_connections         # Redis 连接池
mysql_connections_open         # MySQL 连接数
kafka_consumer_lag             # Kafka 消费延迟
```

---

## 📋 接口规划

### 用户服务 (user-service)

| 方法 | 路径 | 描述 | 鉴权 |
|------|------|------|------|
| POST | `/api/v1/user/register` | 用户注册 | 否 |
| POST | `/api/v1/user/login` | 用户登录 | 否 |
| POST | `/api/v1/user/logout` | 用户登出 | 是 |
| GET | `/api/v1/user/profile` | 获取用户信息 | 是 |
| PUT | `/api/v1/user/profile` | 更新用户信息 | 是 |
| POST | `/api/v1/user/password` | 修改密码 | 是 |
| POST | `/api/v1/user/send-code` | 发送验证码 | 否 |
| POST | `/api/v1/user/verify-code` | 验证验证码 | 否 |

### 商品服务 (product-service)

| 方法 | 路径 | 描述 | 鉴权 |
|------|------|------|------|
| GET | `/api/v1/products` | 商品列表（分页） | 否 |
| GET | `/api/v1/products/:id` | 商品详情 | 否 |
| GET | `/api/v1/products/:id/stock` | 实时库存查询 | 否 |
| POST | `/api/v1/admin/products` | 创建商品 | 管理员 |
| PUT | `/api/v1/admin/products/:id` | 更新商品 | 管理员 |
| DELETE | `/api/v1/admin/products/:id` | 删除商品 | 管理员 |
| POST | `/api/v1/admin/products/:id/stock` | 设置库存 | 管理员 |

### 秒杀服务 (seckill-service)

| 方法 | 路径 | 描述 | 鉴权 |
|------|------|------|------|
| GET | `/api/v1/seckill/activities` | 秒杀活动列表 | 否 |
| GET | `/api/v1/seckill/activities/:id` | 活动详情 | 否 |
| POST | `/api/v1/seckill/activities/:id/buy` | 秒杀下单 | 是 |
| GET | `/api/v1/seckill/activities/:id/status` | 秒杀状态（未开始/进行中/已结束） | 否 |
| POST | `/api/v1/admin/seckill/activities` | 创建秒杀活动 | 管理员 |
| PUT | `/api/v1/admin/seckill/activities/:id` | 更新活动 | 管理员 |
| POST | `/api/v1/admin/seckill/warmup/:id` | 预热库存到 Redis | 管理员 |

### 订单服务 (order-service)

| 方法 | 路径 | 描述 | 鉴权 |
|------|------|------|------|
| GET | `/api/v1/orders` | 我的订单列表 | 是 |
| GET | `/api/v1/orders/:id` | 订单详情 | 是 |
| POST | `/api/v1/orders/:id/cancel` | 取消订单 | 是 |
| POST | `/api/v1/orders/:id/pay` | 发起支付 | 是 |
| GET | `/api/v1/admin/orders` | 订单管理列表 | 管理员 |
| PUT | `/api/v1/admin/orders/:id/status` | 更新订单状态 | 管理员 |

### 支付服务 (payment-service)

| 方法 | 路径 | 描述 | 鉴权 |
|------|------|------|------|
| POST | `/api/v1/payment/create` | 创建支付单 | 是 |
| GET | `/api/v1/payment/:id/status` | 支付状态查询 | 是 |
| POST | `/api/v1/payment/callback/alipay` | 支付宝回调 | 签名验证 |
| POST | `/api/v1/payment/callback/wechat` | 微信回调 | 签名验证 |
| POST | `/api/v1/payment/:id/refund` | 申请退款 | 是 |

### 网关服务 (gateway-service)

| 方法 | 路径 | 描述 |
|------|------|------|
| ANY | `/api/v1/**` | 路由转发到对应微服务 |
| GET | `/health` | 健康检查 |
| GET | `/metrics` | Prometheus 指标 |

---

## 🚀 分阶段开发计划

### 📅 阶段一：基础加固（1-2 周）✅ 已完成

**目标**：完善现有单体架构，补齐关键功能，确保核心流程稳定

#### 任务清单

| 序号 | 任务 | 优先级 | 状态 | 实现文件 |
|------|------|--------|------|----------|
| 1.1 | 配置文件改造（Viper 支持 YAML/ENV） | P0 | ✅ | `pkg/config/config.go` |
| 1.2 | 优雅停机实现（信号监听、连接排空） | P0 | ✅ | `cmd/main.go` |
| 1.3 | 统一响应格式封装（`pkg/response`） | P0 | ✅ | `pkg/response/response.go` |
| 1.4 | 全局错误处理中间件 | P0 | ✅ | `internal/middleware/recovery.go` |
| 1.5 | 参数校验增强（validator tag） | P1 | ✅ | `pkg/validator/validator.go` |
| 1.6 | 请求限流中间件（令牌桶） | P0 | ✅ | `internal/middleware/ratelimit.go` |
| 1.7 | 接口幂等设计（幂等键 + Redis） | P0 | ✅ | `internal/middleware/idempotent.go` |
| 1.8 | 单元测试（核心 service 覆盖率 > 60%） | P1 | ✅ | `internal/service/service_test.go`, `internal/middleware/middleware_test.go` |
| 1.9 | 商品管理 CRUD 接口 | P1 | ✅ | `internal/controller/product_controller.go`, `internal/service/product_service.go` |
| 1.10 | 订单列表/详情接口 | P1 | ✅ | `internal/controller/order_controller.go`, `internal/service/order_service.go` |

#### 重点学习标记

本阶段代码中包含大量 `// 重点学习` 注释，标记了以下知识点：
- 🔥 Viper 配置加载与热更新原理
- 🔥 优雅停机信号处理机制
- 🔥 令牌桶限流算法实现
- 🔥 幂等性设计模式
- 🔥 Cache Aside Pattern 缓存策略
- 🔥 表格驱动测试方法
- 🔥 订单状态机设计

#### 关键代码示例

**配置文件（config/config.yaml）**：
```yaml
server:
  port: 8080
  mode: debug  # debug/release

mysql:
  host: 127.0.0.1
  port: 3306
  user: root
  password: root123456
  database: seckill
  max_idle_conns: 10
  max_open_conns: 100

redis:
  addr: 127.0.0.1:6379
  password: ""
  db: 0
  pool_size: 100

rabbitmq:
  url: amqp://guest:guest@localhost:5672/
  
jwt:
  secret: your-secret-key
  expire: 24h

log:
  level: info
  format: json
```

**统一响应格式**：
```go
// pkg/response/response.go
type Response struct {
    Code    int         `json:"code"`
    Message string      `json:"message"`
    Data    interface{} `json:"data,omitempty"`
    TraceID string      `json:"trace_id,omitempty"`
}

func Success(c *gin.Context, data interface{}) {
    c.JSON(http.StatusOK, Response{
        Code:    0,
        Message: "success",
        Data:    data,
        TraceID: c.GetString("trace_id"),
    })
}

func Fail(c *gin.Context, code int, message string) {
    c.JSON(http.StatusOK, Response{
        Code:    code,
        Message: message,
        TraceID: c.GetString("trace_id"),
    })
}
```

---

### 📅 阶段二：微服务拆分（2-3 周）

**目标**：将单体拆分为独立微服务，引入服务治理组件

#### 任务清单

| 序号 | 任务 | 优先级 | 预计时间 |
|------|------|--------|----------|
| 2.1 | 服务拆分（user/product/seckill/order） | P0 | 3天 |
| 2.2 | gRPC 服务间通信 | P0 | 2天 |
| 2.3 | Consul 服务注册与发现 | P0 | 2天 |
| 2.4 | API Gateway 网关开发 | P0 | 3天 |
| 2.5 | 配置中心接入（Consul KV） | P1 | 1天 |
| 2.6 | 熔断降级（gobreaker） | P1 | 1天 |
| 2.7 | 链路追踪（Jaeger + OpenTelemetry） | P1 | 2天 |
| 2.8 | 服务间鉴权（内部 JWT） | P1 | 1天 |

#### 微服务目录结构

```
damai-go/
├── api-gateway/           # API 网关
│   ├── cmd/
│   ├── internal/
│   │   ├── handler/       # 路由处理
│   │   ├── middleware/    # 限流、鉴权、日志
│   │   └── proxy/         # 反向代理
│   └── config/
├── user-service/          # 用户服务
│   ├── cmd/
│   ├── internal/
│   │   ├── handler/       # HTTP Handler (可选)
│   │   ├── grpc/          # gRPC Server
│   │   ├── service/       # 业务逻辑
│   │   ├── repository/    # 数据访问
│   │   └── model/
│   ├── proto/             # protobuf 定义
│   └── config/
├── product-service/       # 商品服务
├── seckill-service/       # 秒杀服务
├── order-service/         # 订单服务
├── payment-service/       # 支付服务（阶段三）
├── shared/                # 公共代码
│   ├── proto/             # 共享 protobuf
│   ├── middleware/        
│   └── utils/
└── deploy/
    ├── docker/
    ├── k8s/
    └── docker-compose.yaml
```

#### gRPC Proto 示例

```protobuf
// shared/proto/user/user.proto
syntax = "proto3";
package user;
option go_package = "shared/proto/user";

service UserService {
  rpc GetUser(GetUserRequest) returns (GetUserResponse);
  rpc ValidateToken(ValidateTokenRequest) returns (ValidateTokenResponse);
}

message GetUserRequest {
  int64 user_id = 1;
}

message GetUserResponse {
  int64 id = 1;
  string username = 2;
  string phone = 3;
  int32 status = 4;
}

message ValidateTokenRequest {
  string token = 1;
}

message ValidateTokenResponse {
  bool valid = 1;
  int64 user_id = 2;
}
```

---

### 📅 阶段三：高级特性（2-3 周）✅ 已完成

**目标**：引入高级分布式组件，提升系统可靠性和可观测性

#### 任务清单

| 序号 | 任务 | 优先级 | 状态 | 实现文件 |
|------|------|--------|------|----------|
| 3.1 | 消息队列抽象层（支持 Kafka/RabbitMQ） | P0 | ✅ 完成 | `pkg/broker/` |
| 3.2 | 分布式锁实现（Redis） | P0 | ✅ 完成 | `pkg/distlock/distlock.go` |
| 3.3 | 延迟队列（订单超时取消） | P0 | ✅ 完成 | `pkg/delayqueue/delayqueue.go` |
| 3.4 | Prometheus + Grafana 监控 | P0 | ✅ 完成 | `pkg/metrics/metrics.go` |
| 3.5 | 链路追踪（OpenTelemetry + Jaeger） | P0 | ✅ 完成 | `pkg/tracing/`, `pkg/grpcx/tracing.go` |
| 3.6 | K6 压测脚本 | P0 | ✅ 完成 | `scripts/k6/` |
| 3.7 | CI/CD Pipeline（GitHub Actions） | P0 | ✅ 完成 | `.github/workflows/ci.yml` |
| 3.8 | 分布式事务（Saga 模式） | P1 | ⏳ 待做 | - |
| 3.9 | ELK 日志收集 | P1 | ⏳ 待做 | - |
| 3.10 | 支付服务（支付宝沙箱） | P2 | ⏳ 待做 | - |

#### 已完成组件详解

##### 3.1 消息队列抽象层（pkg/broker）

**功能特性**：
- ✅ 统一的 MessageBroker 接口
- ✅ RabbitMQ 实现（支持延迟队列、死信队列）
- ✅ Kafka 实现（支持分区、消费者组）
- ✅ 消息可靠性保证（确认、重试机制）

**面试考点**：
1. 为什么要对消息队列做抽象？（依赖倒置、便于测试和迁移）
2. RabbitMQ 和 Kafka 的区别？
3. 如何保证消息不丢失？

##### 3.2 分布式锁（pkg/distlock）

**功能特性**：
- ✅ Redis SET NX PX 原子获取锁
- ✅ Lua 脚本保证释放锁的原子性
- ✅ 看门狗（Watchdog）自动续期机制
- ✅ 支持重试策略（自定义重试次数和间隔）
- ✅ 函数式选项模式配置

**核心代码学习点**：

```go
// 获取锁并自动续期
lock, err := distlock.AcquireLock(ctx, "order:123", 
    distlock.WithTTL(30*time.Second),
    distlock.WithWatchDog(),
    distlock.WithRetry(3, 100*time.Millisecond),
)
if err != nil {
    return err
}
defer lock.Unlock(ctx)

// 执行业务逻辑...
```

**面试考点**：
1. 为什么用 Lua 脚本而不是多条 Redis 命令？
2. 看门狗续期间隔如何设置？（TTL/3）
3. Redis 主从切换时锁可能丢失怎么办？（Redlock）

##### 3.3 延迟队列（pkg/delayqueue）

**功能特性**：
- ✅ 基于 Redis ZSET 实现（Score = 到期时间戳）
- ✅ Lua 脚本原子性获取并删除到期任务
- ✅ 支持任务重试机制
- ✅ 死信队列处理失败任务
- ✅ 订单超时取消专用封装

**核心代码学习点**：

```go
// 创建订单超时队列
queue := delayqueue.NewOrderTimeoutQueue(redisClient, 30*time.Minute)

// 设置超时处理器
queue.SetHandler(func(ctx context.Context, payload OrderTimeoutPayload) error {
    return orderService.CancelOrder(ctx, payload.OrderID, "超时未支付")
})

// 创建订单时加入超时队列
queue.AddOrder(ctx, OrderTimeoutPayload{
    OrderID:   "order-001",
    UserID:    1,
    ProductID: 100,
})

// 支付成功后移除
queue.RemoveOrder(ctx, "order-001")
```

**面试考点**：
1. 延迟队列有哪些实现方案？各自优缺点？
2. 如何保证任务不被重复消费？
3. 消费失败的任务如何处理？

##### 3.4 Prometheus 监控（pkg/metrics）

**功能特性**：
- ✅ HTTP 请求指标（QPS、延迟分布、错误率）
- ✅ 秒杀业务指标（请求量、订单量、库存）
- ✅ 中间件指标（Redis、RabbitMQ、熔断器）
- ✅ Gin 中间件自动采集
- ✅ /metrics 端点暴露

**核心指标**：

| 指标名 | 类型 | 说明 |
|--------|------|------|
| `seckill_http_requests_total` | Counter | HTTP 请求总数 |
| `seckill_http_request_duration_seconds` | Histogram | 请求延迟分布 |
| `seckill_business_seckill_requests_total` | Counter | 秒杀请求总数 |
| `seckill_business_product_stock` | Gauge | 商品实时库存 |
| `seckill_circuit_breaker_state` | Gauge | 熔断器状态 |

**Grafana 大盘示例查询**：

```promql
# QPS 计算
rate(seckill_http_requests_total[5m])

# P99 延迟
histogram_quantile(0.99, rate(seckill_http_request_duration_seconds_bucket[5m]))

# 错误率
sum(rate(seckill_http_requests_total{status=~"5.."}[5m])) 
/ sum(rate(seckill_http_requests_total[5m]))
```

**面试考点**：
1. Prometheus 四种指标类型分别用于什么场景？
2. Histogram vs Summary 的区别？
3. 什么是高基数问题？如何避免？

#### Kafka 消息层抽象

```go
// pkg/broker/broker.go
type Message struct {
    Key     string
    Value   []byte
    Headers map[string]string
}

type MessageBroker interface {
    Publish(ctx context.Context, topic string, msg *Message) error
    Subscribe(ctx context.Context, topic string, handler MessageHandler) error
    Close() error
}

type MessageHandler func(ctx context.Context, msg *Message) error

// pkg/broker/kafka/kafka.go - Kafka 实现
// pkg/broker/rabbitmq/rabbitmq.go - RabbitMQ 实现
```

#### 延迟队列实现方案

**方案对比**：
| 方案 | 优点 | 缺点 |
|------|------|------|
| Redis ZSET | 简单、性能好 | 需轮询、精度一般 |
| RabbitMQ DLX | 原生支持 | 不够灵活 |
| Kafka + 时间轮 | 高吞吐 | 实现复杂 |
| 数据库轮询 | 简单可靠 | 性能差 |

**推荐**：Redis ZSET + 定时轮询（简单场景）或 Kafka 分区延迟（大规模）

```go
// 订单超时取消示例
func (s *OrderService) CreateOrder(ctx context.Context, order *Order) error {
    // 1. 创建订单
    if err := s.repo.Create(order); err != nil {
        return err
    }
    
    // 2. 加入延迟队列（30分钟后超时）
    expireAt := time.Now().Add(30 * time.Minute).Unix()
    s.redis.ZAdd(ctx, "order:timeout", redis.Z{
        Score:  float64(expireAt),
        Member: order.ID,
    })
    
    return nil
}

// 定时任务：检查超时订单
func (s *OrderService) CheckTimeoutOrders(ctx context.Context) {
    now := time.Now().Unix()
    orderIDs, _ := s.redis.ZRangeByScore(ctx, "order:timeout", &redis.ZRangeBy{
        Min: "0",
        Max: strconv.FormatInt(now, 10),
    }).Result()
    
    for _, orderID := range orderIDs {
        s.CancelOrder(ctx, orderID, "超时未支付")
        s.redis.ZRem(ctx, "order:timeout", orderID)
    }
}
```

---

### 📅 阶段四：生产就绪（1-2 周）

**目标**：完善部署、测试、文档，达到生产级标准

#### 任务清单

| 序号 | 任务 | 优先级 | 预计时间 |
|------|------|--------|----------|
| 4.1 | Docker 多阶段构建优化 | P0 | 1天 |
| 4.2 | K8s Deployment/Service/Ingress | P0 | 2天 |
| 4.3 | GitHub Actions CI/CD | P0 | 1天 |
| 4.4 | 压测脚本（k6/wrk） | P0 | 1天 |
| 4.5 | 压测报告与性能调优 | P0 | 2天 |
| 4.6 | API 文档完善（Swagger） | P1 | 1天 |
| 4.7 | README 项目展示优化 | P1 | 0.5天 |
| 4.8 | 数据库读写分离 | P2 | 2天 |
| 4.9 | Redis Cluster 部署 | P2 | 1天 |

#### Dockerfile 示例（多阶段构建）

```dockerfile
# Build stage
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/seckill ./cmd/main.go

# Runtime stage
FROM alpine:3.19
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=builder /app/seckill .
COPY config/config.yaml ./config/
EXPOSE 8080
CMD ["./seckill"]
```

#### 压测目标指标

| 指标 | 目标值 | 说明 |
|------|--------|------|
| QPS | > 10,000 | 单服务实例 |
| P99 延迟 | < 100ms | 秒杀接口 |
| 错误率 | < 0.1% | 非业务错误 |
| 超卖率 | 0% | 库存一致性 |

---

## 🧩 微服务拆分方案

### 服务边界

```
┌─────────────────────────────────────────────────────────────────┐
│                        API Gateway                              │
│   • 路由转发  • JWT 验证  • 限流  • 日志  • 灰度                   │
└─────────────────────────────────────────────────────────────────┘
                              │
       ┌──────────────────────┼──────────────────────┐
       ▼                      ▼                      ▼
┌─────────────┐       ┌─────────────┐       ┌─────────────┐
│ User Service│       │Product Svc  │       │Seckill Svc  │
│             │       │             │       │             │
│ • 注册/登录  │       │ • 商品CRUD  │       │ • 秒杀下单   │
│ • 用户信息   │       │ • 库存管理   │       │ • 库存预扣   │
│ • Token管理 │       │ • 商品缓存   │       │ • 防刷校验   │
└─────────────┘       └─────────────┘       └─────────────┘
       │                      │                      │
       │                      │                      │
       │                      ▼                      │
       │              ┌─────────────┐                │
       │              │   MySQL     │                │
       │              │ (商品库)    │                │
       │              └─────────────┘                │
       │                                             │
       ▼                                             ▼
┌─────────────┐                              ┌─────────────┐
│   MySQL     │                              │   Redis     │
│  (用户库)   │                              │ (库存缓存)   │
└─────────────┘                              └─────────────┘
                              │
                              ▼
                      ┌─────────────┐
                      │   Kafka     │
                      │ (订单消息)  │
                      └─────────────┘
                              │
       ┌──────────────────────┼──────────────────────┐
       ▼                                             ▼
┌─────────────┐                              ┌─────────────┐
│Order Service│                              │Payment Svc  │
│             │                              │             │
│ • 订单创建   │◄─────────────────────────────│ • 支付创建   │
│ • 订单查询   │                              │ • 回调处理   │
│ • 超时取消   │                              │ • 退款      │
└─────────────┘                              └─────────────┘
       │                                             │
       ▼                                             ▼
┌─────────────┐                              ┌─────────────┐
│   MySQL     │                              │   MySQL     │
│  (订单库)   │                              │  (支付库)   │
└─────────────┘                              └─────────────┘
```

### 服务通信方式

| 场景 | 通信方式 | 协议 | 说明 |
|------|----------|------|------|
| 网关 -> 服务 | HTTP/gRPC | REST/Protobuf | 外部请求统一走网关 |
| 服务 -> 服务（同步） | gRPC | Protobuf | 低延迟、强类型 |
| 服务 -> 服务（异步） | 消息队列 | Kafka | 解耦、削峰 |
| 服务 -> 缓存 | Redis Client | RESP | 高性能缓存 |
| 服务 -> 数据库 | MySQL Client | MySQL Protocol | 数据持久化 |

---

## 🏃 快速启动

### 环境要求

- **Go 1.24+**
- **Docker & Docker Compose** (用于基础设施)
- **Consul** (服务注册中心) - [下载地址](https://developer.hashicorp.com/consul/downloads)
- **Kubernetes** (可选，用于 K8s 部署)

### 📋 微服务架构启动流程图

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        启动顺序（重要！）                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Step 1: 基础设施                                                            │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐                    │
│  │  MySQL   │  │  Redis   │  │ RabbitMQ │  │  Consul  │                    │
│  │  :3307   │  │  :6379   │  │  :6672   │  │  :8500   │                    │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘                    │
│       │             │             │             │                           │
│       └─────────────┴─────────────┴─────────────┘                           │
│                              │                                               │
│                              ▼                                               │
│  Step 2: 微服务（可并行启动）                                                 │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐                    │
│  │   User   │  │ Product  │  │  Order   │  │ Seckill  │                    │
│  │  :50051  │  │  :50052  │  │  :50053  │  │  :50054  │                    │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘                    │
│       │             │             │             │                           │
│       └─────────────┴─────────────┴─────────────┘                           │
│                              │                                               │
│                              ▼                                               │
│  Step 3: API Gateway                                                         │
│  ┌──────────────────────────────────────┐                                   │
│  │            API Gateway               │                                   │
│  │              :8080                   │                                   │
│  │  (等待所有微服务注册到 Consul 后启动) │                                   │
│  └──────────────────────────────────────┘                                   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

### 🚀 方式一：一键启动（推荐）

#### Step 1: 启动基础设施服务

```powershell
# Windows PowerShell - 启动 K8s 端口转发
.\dev_start.ps1

# 输出示例：
# ✅ MySQL: 3307
# ✅ Redis: 6379
# ✅ RabbitMQ: 6672 / 15672
```

<details>
<summary>📌 没有 K8s？使用 Docker Compose 代替</summary>

```bash
# 在项目根目录执行
docker-compose -f deploy/docker-compose.yaml up -d

# 等待服务就绪
docker-compose -f deploy/docker-compose.yaml ps
```

</details>

#### Step 2: 启动 Consul

```bash
# 新开一个终端，启动 Consul（开发模式）
consul agent -dev

# 成功启动后访问: http://localhost:8500
# 看到 Consul UI 界面即表示启动成功
```

#### Step 3: 启动所有微服务

```powershell
# Windows PowerShell - 一键启动所有微服务
.\start_microservices.ps1

# 该脚本会自动：
# ✅ 检查 Consul 是否运行
# ✅ 检查基础设施服务状态
# ✅ 依次启动 5 个服务（每个在独立窗口）
# ✅ 验证服务健康状态
```

#### Step 4: 验证服务状态

```bash
# 1. 查看 Consul 服务注册
curl http://localhost:8500/v1/catalog/services
# 预期输出: {"consul":[],"order-service":[],"product-service":[],"seckill-service":[],"user-service":[]}

# 2. 测试 Gateway 健康检查
curl http://localhost:8080/health
# 预期输出: {"status":"ok"}

# 3. 测试用户注册接口
curl -X POST http://localhost:8080/api/user/register \
  -H "Content-Type: application/json" \
  -d '{"username":"test","password":"123456","phone":"13800138000"}'
```

#### Step 5: 停止服务

```powershell
# 停止所有微服务
.\stop_microservices.ps1

# 停止 Consul: Ctrl+C

# 停止基础设施
docker-compose -f deploy/docker-compose.yaml down
# 或关闭 K8s 端口转发窗口
```

---

### 🛠 方式二：手动启动（开发调试）

适合需要单独调试某个服务的场景。

#### Step 1: 确认配置文件

```yaml
# config/config.yaml 核心配置项

# Consul 地址（服务注册发现）
consul:
  address: 127.0.0.1:8500

# 数据库连接
mysql:
  host: 127.0.0.1
  port: 3307                   # K8s 转发端口

# Redis 连接
redis:
  addr: 127.0.0.1:6379

# RabbitMQ 连接
rabbitmq:
  url: amqp://guest:guest@localhost:6672/
```

#### Step 2: 启动基础设施 + Consul

```bash
# Terminal 1 - K8s 端口转发（或 Docker Compose）
.\dev_start.ps1

# Terminal 2 - Consul
consul agent -dev
```

#### Step 3: 依次启动微服务

```bash
# Terminal 3 - User Service (gRPC:50051)
cd e:\learngo\seckill
go run services/user/main.go
# 或指定端口: go run services/user/main.go -port 50051

# Terminal 4 - Product Service (gRPC:50052)
go run services/product/main.go -port 50052

# Terminal 5 - Order Service (gRPC:50053)
go run services/order/main.go -port 50053

# Terminal 6 - Seckill Service (gRPC:50054)
go run services/seckill/main.go -port 50054

# Terminal 7 - API Gateway (HTTP:8080)
go run services/gateway/main.go -port 8080
```

#### Step 4: 验证服务

```bash
# 查看 Consul UI
start http://localhost:8500

# 查看 RabbitMQ 管理界面
start http://localhost:15672
# 账号: guest / guest
```

---

### 🐳 方式三：Docker Compose 一键部署

```bash
# 1. 构建所有镜像
docker-compose -f deploy/docker-compose.all.yaml build

# 2. 启动所有服务（包括基础设施和微服务）
docker-compose -f deploy/docker-compose.all.yaml up -d

# 3. 查看服务状态
docker-compose -f deploy/docker-compose.all.yaml ps

# 4. 查看日志
docker-compose -f deploy/docker-compose.all.yaml logs -f gateway

# 5. 停止所有服务
docker-compose -f deploy/docker-compose.all.yaml down
```

---

### ☸️ 方式四：Kubernetes 生产部署

```bash
# 1. 部署基础设施
kubectl apply -f deploy/k8s/namespace.yaml
kubectl apply -f deploy/k8s/mysql.yaml
kubectl apply -f deploy/k8s/redis.yaml
kubectl apply -f deploy/k8s/rabbitmq.yaml
kubectl apply -f deploy/k8s/consul.yaml

# 2. 等待基础设施就绪
kubectl wait --for=condition=ready pod -l app=mysql --timeout=120s
kubectl wait --for=condition=ready pod -l app=redis --timeout=60s

# 3. 部署微服务
kubectl apply -f deploy/k8s/user-service.yaml
kubectl apply -f deploy/k8s/product-service.yaml
kubectl apply -f deploy/k8s/order-service.yaml
kubectl apply -f deploy/k8s/seckill-service.yaml
kubectl apply -f deploy/k8s/gateway.yaml

# 4. 查看 Pod 状态
kubectl get pods -w

# 5. 本地访问（端口转发）
kubectl port-forward svc/gateway-service 8080:8080
```

---

### 📊 服务端口说明

| 服务 | 协议 | 端口 | Consul 名称 | 说明 |
|------|------|------|-------------|------|
| **User Service** | gRPC | 50051 | `user-service` | 用户注册/登录/认证 |
| **Product Service** | gRPC | 50052 | `product-service` | 商品 CRUD/库存预热 |
| **Order Service** | gRPC | 50053 | `order-service` | 订单创建/查询 |
| **Seckill Service** | gRPC | 50054 | `seckill-service` | 秒杀核心逻辑 |
| **API Gateway** | HTTP | 8080 | `gateway` | 统一入口/路由/鉴权 |
| **Consul** | HTTP | 8500 | - | 服务注册中心 |
| **MySQL** | TCP | 3307 | - | 数据持久化 |
| **Redis** | TCP | 6379 | - | 缓存/库存/分布式锁 |
| **RabbitMQ** | AMQP | 6672 | - | 异步消息队列 |
| **RabbitMQ UI** | HTTP | 15672 | - | 管理界面 |

---

### 🔌 API 接口测试

启动成功后，可以使用以下命令测试接口：

```bash
# 1. 用户注册
curl -X POST http://localhost:8080/api/user/register \
  -H "Content-Type: application/json" \
  -d '{"username":"testuser","password":"123456","phone":"13800138000"}'

# 2. 用户登录（获取 Token）
curl -X POST http://localhost:8080/api/user/login \
  -H "Content-Type: application/json" \
  -d '{"username":"testuser","password":"123456"}'

# 3. 获取商品列表（需要 Token）
TOKEN="你的token"
curl -X GET "http://localhost:8080/api/products?page=1&page_size=10" \
  -H "Authorization: Bearer $TOKEN"

# 4. 秒杀商品（需要 Token）
curl -X POST http://localhost:8080/api/seckill \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"product_id":1,"quantity":1}'
```

---

### ❓ 常见问题

<details>
<summary><b>Q1: Consul 连接失败？</b></summary>

```bash
# 1. 确保 Consul 已启动
consul agent -dev

# 2. 检查端口占用
netstat -ano | findstr 8500

# 3. 检查 config.yaml 中的地址配置
consul:
  address: 127.0.0.1:8500
```

</details>

<details>
<summary><b>Q2: gRPC 服务无法注册到 Consul？</b></summary>

```bash
# 1. 查看服务启动日志，是否有错误
# 2. 确保 Consul 已启动并可访问
curl http://localhost:8500/v1/status/leader

# 3. 手动检查服务列表
curl http://localhost:8500/v1/catalog/services
```

</details>

<details>
<summary><b>Q3: API Gateway 调用微服务失败？</b></summary>

```bash
# 1. 检查 Consul 中是否注册了所有服务
curl http://localhost:8500/v1/catalog/services

# 2. 检查服务健康状态
curl http://localhost:8500/v1/health/service/user-service

# 3. 检查 Gateway 日志中的错误信息
```

</details>

<details>
<summary><b>Q4: 数据库连接失败？</b></summary>

```bash
# 1. 检查 MySQL 是否启动
kubectl get pods | grep mysql

# 2. 检查端口转发是否正常
kubectl port-forward svc/mysql-service 3307:3306

# 3. 测试数据库连接
mysql -h 127.0.0.1 -P 3307 -u root -p
```

</details>

<details>
<summary><b>Q5: Windows 下 PowerShell 脚本执行报错？</b></summary>

```powershell
# 设置执行策略（管理员运行）
Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope CurrentUser

# 如果还是不行，尝试：
powershell -ExecutionPolicy Bypass -File .\start_microservices.ps1
```

</details>

# 测试连接
mysql -h 127.0.0.1 -P 3307 -u root -p
```

---

## 📁 项目结构

```
seckill/
├── cmd/                           # 【已废弃】单体应用入口
│   └── main.go                    # 原单体架构主程序
│
├── services/                      # 【核心】微服务目录
│   ├── user/                      # 用户服务 (gRPC:50051)
│   │   ├── main.go                # 服务启动入口
│   │   └── handler/
│   │       └── user_handler.go    # gRPC 接口实现
│   ├── product/                   # 商品服务 (gRPC:50052)
│   │   ├── main.go
│   │   └── handler/
│   │       └── product_handler.go
│   ├── order/                     # 订单服务 (gRPC:50053)
│   │   ├── main.go
│   │   └── handler/
│   │       └── order_handler.go
│   ├── seckill/                   # 秒杀服务 (gRPC:50054)
│   │   ├── main.go
│   │   └── handler/
│   │       └── seckill_handler.go # 核心秒杀逻辑
│   └── gateway/                   # API 网关 (HTTP:8080)
│       ├── main.go
│       └── handlers/
│           └── gateway_handler.go # HTTP → gRPC 转换
│
├── proto/                         # Protocol Buffers 定义
│   ├── user/
│   │   ├── user.proto             # 用户服务接口定义
│   │   └── user.pb.go             # 生成的 Go 代码
│   ├── product/
│   │   ├── product.proto
│   │   └── product.pb.go
│   ├── order/
│   │   ├── order.proto
│   │   └── order.pb.go
│   └── seckill/
│       ├── seckill.proto
│       └── seckill.pb.go
│
├── pkg/                           # 共享包
│   ├── breaker/                   # 【新增】熔断器
│   │   ├── breaker.go             # 三状态熔断器实现
│   │   └── breaker_test.go        # 单元测试
│   ├── consul/                    # 【新增】服务注册发现
│   │   ├── consul.go              # Consul 客户端封装
│   │   └── consul_test.go         # 单元测试
│   ├── grpcx/                     # 【新增】gRPC 工具
│   │   ├── server.go              # gRPC 服务端封装
│   │   └── client.go              # gRPC 客户端连接池
│   ├── config/                    # 配置管理
│   │   └── config.go              # Viper 配置加载
│   ├── database/                  # 数据库
│   │   └── mysql.go               # MySQL 连接池
│   ├── redis/                     # Redis
│   │   ├── redis.go               # Redis 客户端
│   │   └── scripts.go             # Lua 脚本（库存扣减）
│   ├── rabbitmq/                  # 消息队列
│   │   └── rabbitmq.go            # RabbitMQ 封装
│   ├── logger/                    # 日志
│   │   └── logger.go              # Zap 日志封装
│   ├── snowflake/                 # 分布式 ID
│   │   └── snowflake.go           # 雪花算法
│   └── utils/                     # 工具函数
│       └── utils.go               # JWT、密码加密等
│
├── internal/                      # 【保留】单体架构代码
│   ├── controller/                # HTTP 控制器（已迁移到 Gateway）
│   ├── middleware/                # 中间件（限流、鉴权、日志）
│   ├── model/                     # 数据模型（User/Product/Order）
│   ├── router/                    # 路由定义
│   └── service/                   # 业务逻辑（已拆分到微服务）
│
├── config/                        # 配置文件
│   └── config.yaml                # 主配置（数据库、Redis、Consul 等）
│
├── deploy/                        # 部署配置
│   ├── docker/
│   │   ├── Dockerfile.user        # 用户服务镜像
│   │   ├── Dockerfile.product
│   │   ├── Dockerfile.order
│   │   ├── Dockerfile.seckill
│   │   └── Dockerfile.gateway
│   └── k8s/
│       ├── mysql.yaml             # MySQL StatefulSet
│       ├── redis.yaml             # Redis Deployment
│       ├── rabbitmq.yaml          # RabbitMQ Deployment
│       ├── consul.yaml            # Consul Deployment
│       ├── user-service.yaml      # 用户服务 K8s 配置
│       ├── product-service.yaml
│       ├── order-service.yaml
│       ├── seckill-service.yaml
│       └── gateway.yaml
│
├── docs/                          # API 文档
│   ├── docs.go                    # Swagger 生成文件
│   ├── swagger.json
│   └── swagger.yaml
│
├── scripts/                       # 【新增】脚本工具
│   ├── start_microservices.ps1   # 一键启动所有微服务
│   ├── stop_microservices.ps1    # 停止所有微服务
│   └── dev_start.ps1              # 启动基础设施端口转发
│
├── test/                          # 测试（待完善）
│   ├── unit/
│   └── integration/
│
├── go.mod                         # Go 模块定义
├── go.sum                         # 依赖锁定
├── Makefile                       # 构建脚本（待添加）
└── README.md                      # 项目文档
```

### 核心目录说明

| 目录 | 说明 | 重要程度 |
|------|------|----------|
| `services/` | **微服务实现**，每个服务独立运行 | ⭐⭐⭐⭐⭐ |
| `proto/` | **gRPC 接口定义**，服务间通信协议 | ⭐⭐⭐⭐⭐ |
| `pkg/breaker/` | **熔断器**，防止级联故障 | ⭐⭐⭐⭐ |
| `pkg/consul/` | **服务发现**，动态服务注册 | ⭐⭐⭐⭐⭐ |
| `pkg/grpcx/` | **gRPC 封装**，连接池与拦截器 | ⭐⭐⭐⭐ |
| `pkg/redis/scripts.go` | **Lua 脚本**，原子库存扣减 | ⭐⭐⭐⭐⭐ |
| `pkg/tracing/` | **链路追踪**，OpenTelemetry + Jaeger | ⭐⭐⭐⭐⭐ |
| `pkg/broker/` | **消息队列抽象**，支持 RabbitMQ/Kafka | ⭐⭐⭐⭐ |
| `internal/` | 单体架构遗留代码，逐步废弃 | ⭐⭐ |

---

## 🔭 新增功能详细说明

### 1. 链路追踪（OpenTelemetry + Jaeger）

**功能位置**：`pkg/tracing/tracing.go`、`pkg/grpcx/tracing.go`

**核心功能**：
- ✅ 集成 OpenTelemetry SDK
- ✅ 支持 Jaeger 后端（OTLP 协议）
- ✅ gRPC 客户端/服务端拦截器
- ✅ 自定义采样策略
- ✅ TraceID 跨服务传播

**使用示例**：

```go
package main

import (
    "context"
    "seckill/pkg/tracing"
)

func main() {
    // 初始化链路追踪
    tp, err := tracing.InitTracer(&tracing.Config{
        ServiceName:    "user-service",
        JaegerEndpoint: "localhost:4317", // Jaeger OTLP 端点
        SampleRate:     1.0,              // 开发环境全量采样
        Enabled:        true,
    })
    if err != nil {
        panic(err)
    }
    defer tp.Shutdown(context.Background())

    // 在业务代码中创建 Span
    ctx := context.Background()
    ctx, span := tracing.StartSpan(ctx, "processOrder")
    defer span.End()

    // 添加属性
    tracing.AddSpanAttributes(ctx,
        tracing.AttrUserID.Int64(12345),
        tracing.AttrOrderID.String("order-001"),
    )

    // 记录事件
    tracing.AddSpanEvent(ctx, "库存扣减成功")

    // 记录错误
    if err != nil {
        tracing.RecordSpanError(ctx, err)
    }
}
```

**gRPC 集成**：

```go
import "seckill/pkg/grpcx"

// 服务端使用追踪拦截器
server := grpc.NewServer(grpcx.WithTracingServerInterceptors()...)

// 客户端使用追踪拦截器
conn, _ := grpc.Dial(target, grpcx.WithTracingClientInterceptors()...)
```

**启动 Jaeger**：

```bash
# Docker 启动 Jaeger
docker run -d --name jaeger \
  -p 16686:16686 \
  -p 4317:4317 \
  jaegertracing/all-in-one:latest

# 访问 Jaeger UI
open http://localhost:16686
```

---

### 2. 消息队列抽象层（Message Broker）

**功能位置**：`pkg/broker/`

**核心功能**：
- ✅ 统一的 `MessageBroker` 接口
- ✅ RabbitMQ 实现（支持延迟队列）
- ✅ Kafka 实现（支持分区和消费者组）
- ✅ 消息可靠性保证（确认、重试、死信）

**使用示例**：

```go
package main

import (
    "context"
    "seckill/pkg/broker"
)

func main() {
    // 创建 RabbitMQ Broker
    rmqBroker, _ := broker.NewRabbitMQBroker(&broker.RabbitMQConfig{
        URL:          "amqp://guest:guest@localhost:5672/",
        Exchange:     "seckill",
        ExchangeType: "topic",
    })
    defer rmqBroker.Close()

    ctx := context.Background()

    // 发布消息
    rmqBroker.Publish(ctx, "order.created", &broker.Message{
        ID:   "msg-001",
        Body: []byte(`{"order_id": "12345"}`),
    })

    // 发布延迟消息（30分钟后处理）
    rmqBroker.PublishWithDelay(ctx, "order.timeout", &broker.Message{
        ID:   "msg-002",
        Body: []byte(`{"order_id": "12345"}`),
    }, 30*time.Minute)

    // 订阅消息
    rmqBroker.Subscribe(ctx, "order.created", "order-consumer", func(ctx context.Context, msg *broker.Message) error {
        fmt.Printf("收到消息: %s\n", string(msg.Body))
        return nil // 返回 nil 表示处理成功
    })

    // 切换到 Kafka
    kafkaBroker, _ := broker.NewKafkaBroker(&broker.KafkaConfig{
        Brokers: []string{"localhost:9092"},
        GroupID: "seckill-consumer",
    })
    // 使用方式完全一致！
}
```

---

### 3. K6 压测脚本

**功能位置**：`scripts/k6/`

**脚本说明**：
- `seckill_test.js` - 秒杀场景压测
- `api_test.js` - API 全链路压测

**运行压测**：

```bash
# 安装 k6
# Windows
choco install k6

# Mac
brew install k6

# 运行秒杀压测
k6 run scripts/k6/seckill_test.js

# 带参数运行
k6 run -e BASE_URL=http://localhost:8080 -e PRODUCT_ID=1 scripts/k6/seckill_test.js

# 运行全链路测试
k6 run scripts/k6/api_test.js

# 输出 JSON 报告
k6 run --out json=results.json scripts/k6/seckill_test.js
```

**压测场景**：

| 场景 | 说明 | VU | 持续时间 |
|------|------|-----|----------|
| 阶梯加压 | 逐步增加并发找瓶颈 | 100→500→1000 | 5分钟 |
| 峰值测试 | 模拟秒杀瞬时流量 | 1000 RPS | 30秒 |
| 稳定性测试 | 中等负载长时间运行 | 500 | 30分钟 |

---

### 4. CI/CD Pipeline（GitHub Actions）

**功能位置**：`.github/workflows/ci.yml`

**Pipeline 阶段**：

```
代码提交
    │
    ▼
┌─────────────┐
│ 1. Lint     │  代码检查、格式化验证
└─────────────┘
    │
    ▼
┌─────────────┐
│ 2. Test     │  单元测试、覆盖率上传
└─────────────┘
    │
    ▼
┌─────────────┐
│ 3. Build    │  多服务并行构建
└─────────────┘
    │
    ▼ (仅 main 分支)
┌─────────────┐
│ 4. Docker   │  构建并推送镜像到 GHCR
└─────────────┘
    │
    ▼ (需审批)
┌─────────────┐
│ 5. Deploy   │  部署到 Kubernetes
└─────────────┘
    │
    ▼
┌─────────────┐
│ 6. E2E Test │  集成测试验证
└─────────────┘
```

**配置 Secrets**：

在 GitHub 仓库设置中添加以下 Secrets：
- `KUBE_CONFIG` - Kubernetes 配置（Base64 编码）
- `API_URL` - 部署后的 API 地址

---

## 📚 学习资源

- [Go 语言圣经](https://books.studygolang.com/gopl-zh/)
- [Go 微服务实战](https://go-micro.dev/)
- [分布式系统设计](https://www.amazon.com/Designing-Data-Intensive-Applications-Reliable-Maintainable/dp/1449373321)
- [Kafka 权威指南](https://kafka.apache.org/documentation/)

---

## 🤝 贡献指南

1. Fork 本仓库
2. 创建特性分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 提交 Pull Request

---

## 📄 License

MIT License - 详见 [LICENSE](LICENSE) 文件

---

## ⭐ Star History

如果这个项目对你有帮助，请给一个 Star ⭐️

---

> **简历描述建议**：
> 
> 独立设计并实现基于 Go 的高并发票务秒杀系统，采用微服务架构，核心技术栈包括 Gin、gRPC、Redis、Kafka、MySQL。通过 Redis Lua 脚本实现库存原子扣减，消息队列异步削峰，支持万级 QPS。集成 Consul 服务发现、Jaeger 链路追踪、Prometheus 监控，部署于 Kubernetes 集群，具备熔断限流、优雅停机等生产级特性。
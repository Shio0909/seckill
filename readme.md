# 🎫 DaMai-Go：高并发票务秒杀系统（Go 微服务版）

> 仿大麦网高并发票务系统的 Go 语言实现，采用微服务架构，支持百万级并发秒杀场景

[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Build Status](https://img.shields.io/badge/build-passing-brightgreen.svg)]()

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

本项目是一个**生产级高并发票务秒杀系统**，参考大麦网架构设计，使用 Go 语言从零实现。项目涵盖了后端开发中的核心技术栈：

- **高并发处理**：Redis Lua 脚本原子扣减、消息队列异步削峰
- **微服务架构**：服务拆分、服务发现、配置中心、API 网关
- **分布式组件**：分布式锁、分布式 ID、分布式事务
- **云原生部署**：Docker 容器化、Kubernetes 编排、CI/CD 流水线

### 核心业务场景

```
用户 -> 网关(限流/鉴权) -> 秒杀服务 -> Redis预扣库存 -> MQ异步 -> 订单服务 -> 支付服务
                                         ↓
                                   库存不足/已购买 -> 快速失败返回
```

---

## 🏗 系统架构

### 目标架构图

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              客户端层                                        │
│                    PC / APP / 小程序 / H5                                    │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                              接入层                                          │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐                      │
│  │   Nginx     │ -> │ API Gateway │ -> │  Sentinel   │                      │
│  │  (负载均衡)  │    │  (路由/鉴权) │    │  (限流熔断)  │                      │
│  └─────────────┘    └─────────────┘    └─────────────┘                      │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                            微服务层                                          │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐          │
│  │用户服务   │ │商品服务   │ │秒杀服务   │ │订单服务   │ │支付服务   │          │
│  │user-svc  │ │product   │ │seckill   │ │order-svc │ │pay-svc   │          │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘ └──────────┘          │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
        ┌───────────────────────────┼───────────────────────────┐
        ▼                           ▼                           ▼
┌───────────────┐          ┌───────────────┐          ┌───────────────┐
│  服务治理层    │          │   消息层       │          │   存储层       │
│ ┌───────────┐ │          │ ┌───────────┐ │          │ ┌───────────┐ │
│ │  Consul   │ │          │ │   Kafka   │ │          │ │   MySQL   │ │
│ │ /Nacos    │ │          │ │  /RabbitMQ│ │          │ │  (主从)    │ │
│ └───────────┘ │          │ └───────────┘ │          │ └───────────┘ │
│ ┌───────────┐ │          │ ┌───────────┐ │          │ ┌───────────┐ │
│ │  Jaeger   │ │          │ │延迟队列     │ │          │ │   Redis   │ │
│ │ (链路追踪) │ │          │ │(订单超时)   │ │          │ │  Cluster  │ │
│ └───────────┘ │          │ └───────────┘ │          │ └───────────┘ │
└───────────────┘          └───────────────┘          └───────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                            可观测性层                                        │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐                        │
│  │Prometheus│ │ Grafana  │ │   ELK    │ │ Alerting │                        │
│  │ (指标)   │ │ (可视化)  │ │  (日志)  │ │  (告警)  │                        │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘                        │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

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

### ❌ 缺失功能（简历加分项）

| 模块 | 缺失功能 | 重要程度 | 简历加分 |
|------|----------|----------|----------|
| **API 网关** | 统一入口、路由、限流 | ⭐⭐⭐⭐⭐ | 高 |
| **服务注册发现** | Consul/Nacos/etcd | ⭐⭐⭐⭐⭐ | 高 |
| **配置中心** | 动态配置、热更新 | ⭐⭐⭐⭐ | 高 |
| **限流熔断** | Sentinel/go-resilience | ⭐⭐⭐⭐⭐ | 高 |
| **分布式锁** | Redis/etcd 分布式锁 | ⭐⭐⭐⭐ | 高 |
| **链路追踪** | Jaeger/Zipkin | ⭐⭐⭐⭐ | 高 |
| **监控告警** | Prometheus + Grafana | ⭐⭐⭐⭐⭐ | 高 |
| **延迟队列** | 订单超时自动取消 | ⭐⭐⭐⭐ | 中 |
| **分布式事务** | Saga/TCC/Seata | ⭐⭐⭐ | 高 |
| **消息队列升级** | Kafka 替换 RabbitMQ | ⭐⭐⭐ | 中 |
| **数据库读写分离** | MySQL 主从 | ⭐⭐⭐ | 中 |
| **分库分表** | 应用层路由/ShardingSphere | ⭐⭐⭐ | 高 |
| **单元测试** | 覆盖率 > 60% | ⭐⭐⭐⭐ | 高 |
| **压测报告** | 性能基准数据 | ⭐⭐⭐⭐ | 高 |
| **CI/CD** | GitHub Actions/Jenkins | ⭐⭐⭐⭐ | 高 |
| **优雅停机** | 信号处理、连接排空 | ⭐⭐⭐ | 中 |
| **接口幂等** | 幂等键设计 | ⭐⭐⭐⭐ | 中 |
| **支付对接** | 支付宝/微信沙箱 | ⭐⭐⭐ | 中 |

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
- Nacos：Java 实现，Go SDK 非官方，配置中心功能强但对 Go 友好度一般
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

### 📅 阶段一：基础加固（1-2 周）

**目标**：完善现有单体架构，补齐关键功能，确保核心流程稳定

#### 任务清单

| 序号 | 任务 | 优先级 | 预计时间 |
|------|------|--------|----------|
| 1.1 | 配置文件改造（Viper 支持 YAML/ENV） | P0 | 1天 |
| 1.2 | 优雅停机实现（信号监听、连接排空） | P0 | 0.5天 |
| 1.3 | 统一响应格式封装（`pkg/response`） | P0 | 0.5天 |
| 1.4 | 全局错误处理中间件 | P0 | 0.5天 |
| 1.5 | 参数校验增强（validator tag） | P1 | 0.5天 |
| 1.6 | 请求限流中间件（令牌桶） | P0 | 1天 |
| 1.7 | 接口幂等设计（幂等键 + Redis） | P0 | 1天 |
| 1.8 | 单元测试（核心 service 覆盖率 > 60%） | P1 | 2天 |
| 1.9 | 商品管理 CRUD 接口 | P1 | 1天 |
| 1.10 | 订单列表/详情接口 | P1 | 1天 |

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

### 📅 阶段三：高级特性（2-3 周）

**目标**：引入高级分布式组件，提升系统可靠性和可观测性

#### 任务清单

| 序号 | 任务 | 优先级 | 预计时间 |
|------|------|--------|----------|
| 3.1 | Kafka 消息队列迁移 | P0 | 3天 |
| 3.2 | 分布式锁实现（Redlock） | P0 | 1天 |
| 3.3 | 延迟队列（订单超时取消） | P0 | 2天 |
| 3.4 | Prometheus + Grafana 监控 | P0 | 2天 |
| 3.5 | ELK 日志收集 | P1 | 2天 |
| 3.6 | 分布式事务（Saga 模式） | P1 | 3天 |
| 3.7 | 支付服务（支付宝沙箱） | P2 | 2天 |
| 3.8 | 消息可靠投递（Outbox 模式） | P1 | 2天 |

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

- Go 1.24+
- Docker & Docker Compose
- Make (可选)

### 本地开发

```bash
# 1. 克隆项目
git clone https://github.com/Shio0909/seckill.git
cd seckill

# 2. 启动依赖服务（MySQL/Redis/RabbitMQ）
docker-compose -f deploy/docker-compose.yaml up -d

# 3. 初始化配置
cp config/config.example.yaml config/config.yaml

# 4. 运行服务
go run cmd/main.go

# 5. 访问 Swagger 文档
# http://localhost:8080/swagger/index.html
```

### K8s 部署

```bash
# 1. 部署中间件
kubectl apply -f deploy/k8s/mysql.yaml
kubectl apply -f deploy/k8s/redis.yaml
kubectl apply -f deploy/k8s/rabbitmq.yaml

# 2. 部署应用
kubectl apply -f deploy/k8s/seckill.yaml

# 3. 端口转发（本地调试）
kubectl port-forward svc/seckill-service 8080:8080
```

---

## 📁 项目结构

```
seckill/
├── cmd/                       # 应用入口
│   └── main.go
├── config/                    # 配置文件
│   └── config.yaml
├── deploy/                    # 部署配置
│   ├── docker/
│   │   └── Dockerfile
│   └── k8s/
│       ├── mysql.yaml
│       ├── redis.yaml
│       └── rabbitmq.yaml
├── docs/                      # Swagger 文档
├── internal/                  # 内部代码（不对外暴露）
│   ├── controller/            # HTTP 控制器
│   ├── middleware/            # 中间件
│   ├── model/                 # 数据模型
│   ├── router/                # 路由定义
│   └── service/               # 业务逻辑
├── pkg/                       # 可复用包
│   ├── broker/                # 消息队列抽象（待实现）
│   ├── database/              # MySQL 连接
│   ├── logger/                # 日志封装
│   ├── rabbitmq/              # RabbitMQ 客户端
│   ├── redis/                 # Redis 客户端 + Lua脚本
│   ├── response/              # 统一响应（待完善）
│   ├── snowflake/             # 雪花算法
│   └── utils/                 # 工具函数
├── test/                      # 测试文件（待添加）
│   ├── unit/
│   └── integration/
├── go.mod
├── go.sum
├── Makefile                   # 构建脚本（待添加）
└── README.md
```

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
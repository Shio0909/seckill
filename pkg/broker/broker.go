package broker

import (
	"context"
	"time"
)

// ========================================================================
// 【重点学习】消息队列抽象层（Message Broker Abstraction）
// ========================================================================
// 为什么需要抽象层？
// 1. 解耦业务代码和具体 MQ 实现（RabbitMQ/Kafka）
// 2. 便于测试（可以 mock MessageBroker 接口）
// 3. 支持平滑迁移（从 RabbitMQ 迁移到 Kafka）
// 4. 统一的错误处理和重试策略
//
// 📝 简历亮点：
// - 设计消息队列抽象层，支持多种 MQ 实现
// - 接口隔离，业务代码与具体实现解耦
// - 支持消息可靠投递和幂等消费
//
// 🔥 面试高频：
// Q: 为什么要对消息队列做抽象？
// A: 1. 依赖倒置原则：高层模块不依赖低层模块，都依赖抽象
//    2. 开闭原则：扩展新 MQ 不需要修改业务代码
//    3. 便于单元测试：可以 mock 接口进行测试
//    4. 支持灰度迁移：可以同时运行两种 MQ
//
// Q: RabbitMQ 和 Kafka 的主要区别？
// A: 架构模型：
//    - RabbitMQ: 传统消息队列模型，消息消费后即删除
//    - Kafka: 分布式日志系统，消息持久化，支持重复消费
//    性能：
//    - RabbitMQ: 万级 QPS，延迟更低（微秒级）
//    - Kafka: 百万级 QPS，适合大数据场景
//    使用场景：
//    - RabbitMQ: 业务解耦、异步处理、延迟队列
//    - Kafka: 日志收集、流处理、事件溯源
//
// Q: 如何保证消息不丢失？
// A: 三个环节：
//    1. 生产者：消息发送确认（ack）、重试机制
//    2. Broker：消息持久化、副本机制
//    3. 消费者：手动确认消费、幂等处理
// ========================================================================

// Message 消息结构
// 统一的消息格式，屏蔽不同 MQ 的差异
type Message struct {
	ID        string            // 消息唯一 ID（用于幂等）
	Key       string            // 消息 Key（Kafka 分区键）
	Topic     string            // 主题/队列名称
	Body      []byte            // 消息体
	Headers   map[string]string // 消息头（用于链路追踪等）
	Timestamp time.Time         // 消息时间戳
	Attempts  int               // 重试次数
}

// MessageHandler 消息处理函数
// 返回 error 表示处理失败，需要重试
type MessageHandler func(ctx context.Context, msg *Message) error

// ========================================================================
// 【核心接口】MessageBroker 消息代理接口
// ========================================================================
// 这是核心抽象，所有 MQ 实现（RabbitMQ/Kafka）都要实现此接口
// ========================================================================

// MessageBroker 消息代理接口
type MessageBroker interface {
	// Publish 发布消息
	// topic: 主题/队列名称
	// msg: 消息内容
	Publish(ctx context.Context, topic string, msg *Message) error

	// PublishWithDelay 发布延迟消息
	// 用于订单超时取消等场景
	PublishWithDelay(ctx context.Context, topic string, msg *Message, delay time.Duration) error

	// Subscribe 订阅消息
	// topic: 主题/队列名称
	// group: 消费者组（Kafka 概念，RabbitMQ 可忽略）
	// handler: 消息处理函数
	Subscribe(ctx context.Context, topic string, group string, handler MessageHandler) error

	// Close 关闭连接
	Close() error

	// Healthy 健康检查
	Healthy(ctx context.Context) bool
}

// ========================================================================
// 【配置】消息代理配置
// ========================================================================

// BrokerType 消息代理类型
type BrokerType string

const (
	BrokerTypeRabbitMQ BrokerType = "rabbitmq"
	BrokerTypeKafka    BrokerType = "kafka"
)

// Config 消息代理配置
type Config struct {
	Type BrokerType // 代理类型

	// RabbitMQ 配置
	RabbitMQ RabbitMQConfig

	// Kafka 配置
	Kafka KafkaConfig

	// 通用配置
	RetryCount    int           // 重试次数
	RetryInterval time.Duration // 重试间隔
}

// RabbitMQConfig RabbitMQ 配置
type RabbitMQConfig struct {
	URL          string // AMQP 连接地址
	Exchange     string // 交换机名称
	ExchangeType string // 交换机类型：direct/fanout/topic
	Durable      bool   // 是否持久化
	AutoDelete   bool   // 自动删除
}

// KafkaConfig Kafka 配置
type KafkaConfig struct {
	Brokers  []string // Broker 地址列表
	GroupID  string   // 消费者组 ID
	ClientID string   // 客户端 ID
	// SASL 认证（如果需要）
	SASLEnabled  bool
	SASLUser     string
	SASLPassword string
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Type:          BrokerTypeRabbitMQ,
		RetryCount:    3,
		RetryInterval: time.Second,
		RabbitMQ: RabbitMQConfig{
			URL:          "amqp://guest:guest@localhost:5672/",
			Exchange:     "seckill",
			ExchangeType: "topic",
			Durable:      true,
		},
		Kafka: KafkaConfig{
			Brokers: []string{"localhost:9092"},
			GroupID: "seckill-consumer",
		},
	}
}

// ========================================================================
// 【重点】消息可靠性保证
// ========================================================================
// 消息丢失的三个场景及解决方案：
//
// 1. 生产者丢失：
//    - 问题：消息发送后，Broker 未收到
//    - 解决：开启 Publisher Confirm 模式，发送失败重试
//
// 2. Broker 丢失：
//    - 问题：Broker 收到消息后宕机，消息未持久化
//    - 解决：开启消息持久化，设置副本（Kafka replication）
//
// 3. 消费者丢失：
//    - 问题：消费者处理消息失败，但已确认
//    - 解决：先处理业务，成功后再确认（手动 ACK）
//
// 🔥 面试题：
// Q: 如何实现消息的 Exactly Once 语义？
// A: 严格的 Exactly Once 很难实现，通常采用 At Least Once + 幂等消费：
//    1. 生产者：消息携带唯一 ID
//    2. 消费者：处理前检查消息 ID 是否已处理
//    3. 使用 Redis/DB 记录已处理的消息 ID
//
// Q: 消息消费失败如何处理？
// A: 1. 有限重试：设置最大重试次数
//    2. 指数退避：重试间隔逐渐增大
//    3. 死信队列：超过重试次数的消息进入死信队列
//    4. 人工处理：监控死信队列，人工介入
// ========================================================================

// PublishOption 发布选项
type PublishOption func(*publishOptions)

type publishOptions struct {
	Persistent bool          // 消息持久化
	Mandatory  bool          // 消息必须被路由到队列
	Timeout    time.Duration // 发送超时
	Priority   int           // 消息优先级
}

// WithPersistent 设置消息持久化
func WithPersistent(persistent bool) PublishOption {
	return func(o *publishOptions) {
		o.Persistent = persistent
	}
}

// WithMandatory 设置强制路由
func WithMandatory(mandatory bool) PublishOption {
	return func(o *publishOptions) {
		o.Mandatory = mandatory
	}
}

// WithTimeout 设置发送超时
func WithTimeout(timeout time.Duration) PublishOption {
	return func(o *publishOptions) {
		o.Timeout = timeout
	}
}

// WithPriority 设置消息优先级
func WithPriority(priority int) PublishOption {
	return func(o *publishOptions) {
		o.Priority = priority
	}
}

// SubscribeOption 订阅选项
type SubscribeOption func(*subscribeOptions)

type subscribeOptions struct {
	AutoAck       bool          // 自动确认
	Prefetch      int           // 预取数量
	RetryCount    int           // 重试次数
	RetryInterval time.Duration // 重试间隔
}

// WithAutoAck 设置自动确认
func WithAutoAck(autoAck bool) SubscribeOption {
	return func(o *subscribeOptions) {
		o.AutoAck = autoAck
	}
}

// WithPrefetch 设置预取数量
func WithPrefetch(prefetch int) SubscribeOption {
	return func(o *subscribeOptions) {
		o.Prefetch = prefetch
	}
}

// WithRetry 设置重试策略
func WithRetry(count int, interval time.Duration) SubscribeOption {
	return func(o *subscribeOptions) {
		o.RetryCount = count
		o.RetryInterval = interval
	}
}

// ========================================================================
// 【重点】消息序列化
// ========================================================================

// Encoder 消息编码器接口
type Encoder interface {
	Encode(v interface{}) ([]byte, error)
	Decode(data []byte, v interface{}) error
}

// JSONEncoder JSON 编码器
type JSONEncoder struct{}

// Encode 编码为 JSON
func (e *JSONEncoder) Encode(v interface{}) ([]byte, error) {
	// 使用 encoding/json 或 json-iterator
	return nil, nil // 实际实现省略
}

// Decode 从 JSON 解码
func (e *JSONEncoder) Decode(data []byte, v interface{}) error {
	return nil // 实际实现省略
}

// ========================================================================
// 【工具函数】消息 ID 生成
// ========================================================================

// GenerateMessageID 生成消息唯一 ID
// 格式：时间戳-随机数
func GenerateMessageID() string {
	// 实际实现可以使用 UUID 或 Snowflake
	return ""
}

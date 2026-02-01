package broker

import (
	"context"
	"time"
)

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

// 【核心接口】MessageBroker 消息代理接口
// 这是核心抽象，所有 MQ 实现（RabbitMQ/Kafka）都要实现此接口

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

// 【配置】消息代理配置

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

// 【重点】消息序列化

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

// 【工具函数】消息 ID 生成

// GenerateMessageID 生成消息唯一 ID
// 格式：时间戳-随机数
func GenerateMessageID() string {
	// 实际实现可以使用 UUID 或 Snowflake
	return ""
}

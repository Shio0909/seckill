package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

// ========================================================================
// 【重点学习】Kafka 消息代理实现
// ========================================================================
// Kafka 核心概念：
// 1. Topic（主题）：消息的分类，类似于数据库的表
// 2. Partition（分区）：Topic 的物理分片，是并行处理的单元
// 3. Offset（偏移量）：消息在分区中的位置
// 4. Consumer Group（消费者组）：消费者的逻辑分组，组内消费者负载均衡
// 5. Broker：Kafka 服务器节点
//
// 📝 简历亮点：
// - 使用 Kafka 实现高吞吐消息处理
// - 理解 Partition 和 Consumer Group 的负载均衡机制
// - 实现消息的可靠消费和 Offset 管理
//
// 🔥 面试高频：
// Q: Kafka 为什么吞吐量这么高？
// A: 1. 顺序写磁盘：利用磁盘顺序 I/O 的高性能
//    2. Page Cache：利用 OS 页缓存，减少磁盘 I/O
//    3. 零拷贝：sendfile 系统调用，减少数据拷贝
//    4. 批量处理：批量发送/消费，减少网络往返
//    5. 分区机制：并行处理，水平扩展
//
// Q: Kafka 如何保证消息不丢失？
// A: 三个层面：
//    1. 生产者：acks=all，等待所有副本确认
//    2. Broker：replication.factor>=3，min.insync.replicas>=2
//    3. 消费者：手动提交 offset，处理成功后再提交
//
// Q: Kafka 如何保证消息顺序？
// A: 1. 全局有序：只使用一个分区（牺牲并行性）
//    2. 分区有序：相同 Key 的消息发送到同一分区
//    3. 注意重试可能导致乱序（设置 max.in.flight.requests.per.connection=1）
//
// Q: Consumer Group 的 Rebalance 机制？
// A: 触发条件：消费者加入/离开、订阅的 Topic 分区数变化
//    过程：
//    1. JoinGroup：消费者加入组
//    2. SyncGroup：Leader 分配分区策略
//    3. 分区重新分配给消费者
//    问题：Rebalance 期间无法消费，需要优化策略
// ========================================================================

// KafkaBroker Kafka 消息代理实现
type KafkaBroker struct {
	config    *KafkaConfig
	writers   map[string]*kafka.Writer // 每个 topic 一个 writer
	readers   map[string]*kafka.Reader
	mu        sync.RWMutex
	closed    bool
	cancelCtx context.Context
	cancel    context.CancelFunc
}

// NewKafkaBroker 创建 Kafka 代理
func NewKafkaBroker(cfg *KafkaConfig) (*KafkaBroker, error) {
	if cfg == nil {
		cfg = &KafkaConfig{
			Brokers: []string{"localhost:9092"},
			GroupID: "seckill-consumer",
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	broker := &KafkaBroker{
		config:    cfg,
		writers:   make(map[string]*kafka.Writer),
		readers:   make(map[string]*kafka.Reader),
		cancelCtx: ctx,
		cancel:    cancel,
	}

	return broker, nil
}

// getWriter 获取或创建 Writer
func (b *KafkaBroker) getWriter(topic string) *kafka.Writer {
	b.mu.Lock()
	defer b.mu.Unlock()

	if writer, ok := b.writers[topic]; ok {
		return writer
	}

	// 【重点】Kafka Writer 配置
	writer := &kafka.Writer{
		Addr:  kafka.TCP(b.config.Brokers...),
		Topic: topic,
		// 【重点】负载均衡策略
		// Hash：根据 Key 哈希到固定分区（保证相同 Key 的消息顺序）
		Balancer: &kafka.Hash{},
		// 【重点】确认级别
		// RequireAll：等待所有 ISR 副本确认
		RequiredAcks: kafka.RequireAll,
		// 【重点】批量配置
		BatchSize:    100,                   // 批量大小
		BatchTimeout: 10 * time.Millisecond, // 批量超时
		// 【重点】重试配置
		MaxAttempts: 3,
		// 【重点】压缩（减少网络传输）
		Compression: kafka.Snappy,
	}

	b.writers[topic] = writer
	return writer
}

// Publish 发布消息
func (b *KafkaBroker) Publish(ctx context.Context, topic string, msg *Message) error {
	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return fmt.Errorf("broker 已关闭")
	}
	b.mu.RUnlock()

	writer := b.getWriter(topic)

	// 序列化消息
	value, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("序列化消息失败: %w", err)
	}

	// 【重点】构建 Kafka 消息
	kafkaMsg := kafka.Message{
		Key:   []byte(msg.Key), // Key 用于分区路由
		Value: value,
		Time:  time.Now(),
		Headers: []kafka.Header{
			{Key: "message_id", Value: []byte(msg.ID)},
		},
	}

	// 添加自定义 Headers
	for k, v := range msg.Headers {
		kafkaMsg.Headers = append(kafkaMsg.Headers, kafka.Header{
			Key:   k,
			Value: []byte(v),
		})
	}

	// 【重点】发送消息
	// WriteMessages 是阻塞的，等待消息发送确认
	err = writer.WriteMessages(ctx, kafkaMsg)
	if err != nil {
		return fmt.Errorf("发送消息失败: %w", err)
	}

	return nil
}

// PublishWithDelay 发布延迟消息
// Kafka 原生不支持延迟消息，需要借助外部组件实现
func (b *KafkaBroker) PublishWithDelay(ctx context.Context, topic string, msg *Message, delay time.Duration) error {
	// 【重点】Kafka 延迟消息实现方案：
	// 方案1：时间轮 + 内存队列
	// 方案2：额外的延迟服务（如 DelayQueue）
	// 方案3：消息中携带执行时间，消费者判断是否到期

	// 这里采用方案3：在消息中添加执行时间
	msg.Headers["execute_at"] = fmt.Sprintf("%d", time.Now().Add(delay).UnixMilli())
	msg.Headers["delay_ms"] = fmt.Sprintf("%d", delay.Milliseconds())

	// 发送到延迟主题
	delayTopic := topic + ".delay"
	return b.Publish(ctx, delayTopic, msg)
}

// Subscribe 订阅消息
func (b *KafkaBroker) Subscribe(ctx context.Context, topic string, group string, handler MessageHandler) error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return fmt.Errorf("broker 已关闭")
	}
	b.mu.Unlock()

	groupID := group
	if groupID == "" {
		groupID = b.config.GroupID
	}

	// 【重点】Kafka Reader 配置
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: b.config.Brokers,
		Topic:   topic,
		GroupID: groupID, // 消费者组
		// 【重点】分区分配策略
		// GroupBalancers: []kafka.GroupBalancer{kafka.RangeGroupBalancer{}},
		// 【重点】Offset 策略
		// FirstOffset：从最早的消息开始
		// LastOffset：从最新的消息开始
		StartOffset: kafka.FirstOffset,
		// 【重点】消费配置
		MinBytes:       10e3,        // 最小拉取字节数
		MaxBytes:       10e6,        // 最大拉取字节数
		MaxWait:        time.Second, // 最大等待时间
		CommitInterval: time.Second, // 自动提交间隔
		// 【重点】心跳和会话超时
		HeartbeatInterval: 3 * time.Second,
		SessionTimeout:    30 * time.Second,
	})

	b.mu.Lock()
	b.readers[topic] = reader
	b.mu.Unlock()

	// 启动消费 goroutine
	go b.consume(ctx, reader, handler, topic)

	return nil
}

// consume 消费消息
func (b *KafkaBroker) consume(ctx context.Context, reader *kafka.Reader, handler MessageHandler, topic string) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-b.cancelCtx.Done():
			return
		default:
			// 【重点】拉取消息
			kafkaMsg, err := reader.FetchMessage(ctx)
			if err != nil {
				if err == context.Canceled {
					return
				}
				fmt.Printf("拉取消息失败: %v\n", err)
				continue
			}

			// 解析消息
			var msg Message
			if err := json.Unmarshal(kafkaMsg.Value, &msg); err != nil {
				// 解析失败，提交 offset（避免消息重复消费）
				zap.L().Error("failed to unmarshal kafka message",
					zap.Error(err),
					zap.ByteString("value", kafkaMsg.Value))
				if commitErr := reader.CommitMessages(ctx, kafkaMsg); commitErr != nil {
					zap.L().Error("failed to commit offset after unmarshal error", zap.Error(commitErr))
				}
				continue
			}

			// 从 Headers 中提取信息
			for _, h := range kafkaMsg.Headers {
				if msg.Headers == nil {
					msg.Headers = make(map[string]string)
				}
				msg.Headers[h.Key] = string(h.Value)
			}

			// 【重点】处理消息
			err = handler(ctx, &msg)
			if err != nil {
				// 处理失败，根据重试策略决定
				msg.Attempts++
				zap.L().Error("failed to handle kafka message",
					zap.Error(err),
					zap.Int("attempts", msg.Attempts),
					zap.String("message_id", msg.ID))
				if msg.Attempts < 3 {
					// 重试：发送到重试主题
					retryTopic := topic + ".retry"
					if publishErr := b.Publish(ctx, retryTopic, &msg); publishErr != nil {
						zap.L().Error("failed to publish message to retry topic",
							zap.Error(publishErr),
							zap.String("retry_topic", retryTopic))
					}
				} else {
					// 死信：发送到死信主题
					dlqTopic := topic + ".dlq"
					zap.L().Warn("message exceeded max retries, sending to DLQ",
						zap.String("message_id", msg.ID),
						zap.Int("attempts", msg.Attempts),
						zap.String("dlq_topic", dlqTopic))
					if publishErr := b.Publish(ctx, dlqTopic, &msg); publishErr != nil {
						zap.L().Error("failed to publish message to DLQ",
							zap.Error(publishErr),
							zap.String("dlq_topic", dlqTopic))
					}
				}
			}

			// 【重点】手动提交 Offset
			// 只有处理成功才提交，保证 At Least Once
			if err := reader.CommitMessages(ctx, kafkaMsg); err != nil {
				fmt.Printf("提交 offset 失败: %v\n", err)
			}
		}
	}
}

// Close 关闭连接
func (b *KafkaBroker) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil
	}

	b.closed = true
	b.cancel()

	// 关闭所有 Writer
	for _, writer := range b.writers {
		writer.Close()
	}

	// 关闭所有 Reader
	for _, reader := range b.readers {
		reader.Close()
	}

	return nil
}

// Healthy 健康检查
func (b *KafkaBroker) Healthy(ctx context.Context) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return false
	}

	// 尝试连接 Broker
	conn, err := kafka.DialContext(ctx, "tcp", b.config.Brokers[0])
	if err != nil {
		return false
	}
	defer conn.Close()

	return true
}

// ========================================================================
// 【重点】Kafka 最佳实践
// ========================================================================
// 1. 分区设计：
//    - 分区数 = 期望吞吐量 / 单分区吞吐量
//    - 分区数一旦确定，增加容易减少难
//    - 建议分区数为 Broker 数量的整数倍
//
// 2. 消息 Key 设计：
//    - 相同 Key 的消息保证顺序
//    - Key 分布要均匀，避免热点分区
//    - 可以用 用户ID、订单ID 作为 Key
//
// 3. 消费者优化：
//    - 消费者数 <= 分区数（多余的消费者会空闲）
//    - 批量消费，减少提交 offset 次数
//    - 使用异步处理，提高吞吐量
//
// 4. 监控指标：
//    - Consumer Lag：消费延迟
//    - ISR 数量：同步副本数
//    - 分区 Leader 分布
//
// 5. 常见问题：
//    - Rebalance 频繁：增加 session.timeout
//    - 消费延迟：增加消费者数量
//    - 消息积压：检查消费者处理能力
// ========================================================================

// ========================================================================
// 【扩展】Kafka 事务支持
// ========================================================================
// Kafka 0.11+ 支持事务，可以实现 Exactly Once 语义
//
// 使用场景：
// 1. 生产者发送多条消息，要么全部成功，要么全部失败
// 2. 消费-处理-生产 的原子操作
//
// 开启事务：
// 1. 设置 transactional.id
// 2. 调用 InitTransactions
// 3. BeginTransaction -> Send -> CommitTransaction
// ========================================================================

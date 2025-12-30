package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

// ========================================================================
// 【重点学习】RabbitMQ 消息代理实现
// ========================================================================
// RabbitMQ 核心概念：
// 1. Producer（生产者）：发送消息
// 2. Exchange（交换机）：接收消息并路由到队列
// 3. Queue（队列）：存储消息
// 4. Consumer（消费者）：接收并处理消息
// 5. Binding（绑定）：交换机和队列的绑定规则
//
// 交换机类型：
// - Direct：精确匹配 routing key
// - Fanout：广播到所有绑定队列
// - Topic：模式匹配（* 匹配一个单词，# 匹配多个）
// - Headers：根据消息头匹配
//
// 🔥 面试高频：
// Q: RabbitMQ 如何保证消息不丢失？
// A: 三个层面：
//    1. 生产者：开启 Confirm 模式，等待 Broker 确认
//    2. Broker：交换机和队列都设置为 durable，消息设置为 persistent
//    3. 消费者：关闭 autoAck，处理成功后手动 Ack
//
// Q: RabbitMQ 消息积压如何处理？
// A: 1. 增加消费者数量（水平扩展）
//    2. 提高单个消费者的处理能力（批量处理）
//    3. 设置消息 TTL，过期消息进入死信队列
//    4. 考虑业务是否可以丢弃部分消息
//
// Q: 如何实现延迟队列？
// A: 两种方式：
//    1. TTL + 死信队列：消息设置 TTL，过期后路由到死信队列
//    2. 延迟插件：rabbitmq_delayed_message_exchange
// ========================================================================

// RabbitMQBroker RabbitMQ 消息代理实现
type RabbitMQBroker struct {
	config     *RabbitMQConfig
	conn       *amqp.Connection
	channel    *amqp.Channel
	mu         sync.RWMutex
	closed     bool
	consumers  map[string]<-chan amqp.Delivery
	cancelFunc context.CancelFunc
}

// NewRabbitMQBroker 创建 RabbitMQ 代理
func NewRabbitMQBroker(cfg *RabbitMQConfig) (*RabbitMQBroker, error) {
	if cfg == nil {
		cfg = &RabbitMQConfig{
			URL:          "amqp://guest:guest@localhost:5672/",
			Exchange:     "seckill",
			ExchangeType: "topic",
			Durable:      true,
		}
	}

	broker := &RabbitMQBroker{
		config:    cfg,
		consumers: make(map[string]<-chan amqp.Delivery),
	}

	// 建立连接
	if err := broker.connect(); err != nil {
		return nil, err
	}

	return broker, nil
}

// connect 建立连接
func (b *RabbitMQBroker) connect() error {
	var err error

	// 【重点】建立 AMQP 连接
	// RabbitMQ 使用 TCP 长连接，避免频繁建立连接的开销
	b.conn, err = amqp.Dial(b.config.URL)
	if err != nil {
		return fmt.Errorf("连接 RabbitMQ 失败: %w", err)
	}

	// 【重点】创建 Channel
	// Channel 是虚拟连接，多路复用一个 TCP 连接
	// 每个 goroutine 应该使用独立的 Channel
	b.channel, err = b.conn.Channel()
	if err != nil {
		return fmt.Errorf("创建 Channel 失败: %w", err)
	}

	// 【重点】声明交换机
	err = b.channel.ExchangeDeclare(
		b.config.Exchange,     // 交换机名称
		b.config.ExchangeType, // 类型：topic
		b.config.Durable,      // 持久化
		b.config.AutoDelete,   // 自动删除
		false,                 // internal
		false,                 // no-wait
		nil,                   // 参数
	)
	if err != nil {
		return fmt.Errorf("声明交换机失败: %w", err)
	}

	// 【重点】开启 Confirm 模式
	// 确保消息成功投递到 Broker
	if err := b.channel.Confirm(false); err != nil {
		return fmt.Errorf("开启 Confirm 模式失败: %w", err)
	}

	// 监听连接关闭事件，自动重连
	b.notifyClose()

	return nil
}

// Publish 发布消息
func (b *RabbitMQBroker) Publish(ctx context.Context, topic string, msg *Message) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return fmt.Errorf("broker 已关闭")
	}

	// 序列化消息
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("序列化消息失败: %w", err)
	}

	// 【重点】发布消息
	publishing := amqp.Publishing{
		ContentType:  "application/json",
		Body:         body,
		DeliveryMode: amqp.Persistent, // 持久化消息
		MessageId:    msg.ID,
		Timestamp:    time.Now(),
		Headers:      amqp.Table{},
	}

	// 添加自定义 Headers（用于链路追踪）
	for k, v := range msg.Headers {
		publishing.Headers[k] = v
	}

	// 发布到交换机
	err = b.channel.PublishWithContext(ctx,
		b.config.Exchange, // 交换机
		topic,             // routing key
		false,             // mandatory
		false,             // immediate
		publishing,
	)
	if err != nil {
		return fmt.Errorf("发布消息失败: %w", err)
	}

	// 【重点】等待 Confirm
	// 确认消息已被 Broker 接收
	confirm := b.channel.NotifyPublish(make(chan amqp.Confirmation, 1))
	select {
	case c := <-confirm:
		if !c.Ack {
			return fmt.Errorf("消息发布未确认")
		}
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(5 * time.Second):
		return fmt.Errorf("等待确认超时")
	}

	return nil
}

// PublishWithDelay 发布延迟消息
// 使用死信队列实现延迟
func (b *RabbitMQBroker) PublishWithDelay(ctx context.Context, topic string, msg *Message, delay time.Duration) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return fmt.Errorf("broker 已关闭")
	}

	// 【重点】延迟队列实现原理
	// 1. 创建一个临时队列，设置 TTL 和死信交换机
	// 2. 消息发送到临时队列
	// 3. TTL 过期后，消息路由到死信交换机
	// 4. 死信交换机路由到目标队列

	delayQueueName := fmt.Sprintf("%s.delay.%d", topic, delay.Milliseconds())

	// 声明延迟队列
	_, err := b.channel.QueueDeclare(
		delayQueueName,
		true,  // durable
		true,  // auto-delete
		false, // exclusive
		false, // no-wait
		amqp.Table{
			"x-message-ttl":             delay.Milliseconds(), // 消息 TTL
			"x-dead-letter-exchange":    b.config.Exchange,    // 死信交换机
			"x-dead-letter-routing-key": topic,                // 死信路由键
		},
	)
	if err != nil {
		return fmt.Errorf("声明延迟队列失败: %w", err)
	}

	// 序列化消息
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("序列化消息失败: %w", err)
	}

	// 发送到延迟队列
	err = b.channel.PublishWithContext(ctx,
		"",             // 默认交换机
		delayQueueName, // 队列名
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         body,
			DeliveryMode: amqp.Persistent,
			MessageId:    msg.ID,
			Timestamp:    time.Now(),
		},
	)

	return err
}

// Subscribe 订阅消息
func (b *RabbitMQBroker) Subscribe(ctx context.Context, topic string, group string, handler MessageHandler) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return fmt.Errorf("broker 已关闭")
	}

	// 队列名称（使用 group 作为队列名，实现负载均衡）
	queueName := group
	if queueName == "" {
		queueName = topic
	}

	// 【重点】声明队列
	q, err := b.channel.QueueDeclare(
		queueName,
		true,  // durable: 持久化
		false, // auto-delete: 不自动删除
		false, // exclusive: 非独占
		false, // no-wait
		amqp.Table{
			// 设置死信队列（处理失败的消息）
			"x-dead-letter-exchange":    b.config.Exchange,
			"x-dead-letter-routing-key": topic + ".dlq",
		},
	)
	if err != nil {
		return fmt.Errorf("声明队列失败: %w", err)
	}

	// 【重点】绑定队列到交换机
	err = b.channel.QueueBind(
		q.Name,            // 队列名
		topic,             // routing key（支持 # 和 * 通配符）
		b.config.Exchange, // 交换机
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("绑定队列失败: %w", err)
	}

	// 【重点】设置 QoS（预取数量）
	// 限制每个消费者同时处理的消息数量
	err = b.channel.Qos(
		10,    // prefetch count
		0,     // prefetch size
		false, // global
	)
	if err != nil {
		return fmt.Errorf("设置 QoS 失败: %w", err)
	}

	// 【重点】开始消费
	msgs, err := b.channel.Consume(
		q.Name, // 队列名
		"",     // consumer tag
		false,  // auto-ack: 关闭自动确认！
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,
	)
	if err != nil {
		return fmt.Errorf("开始消费失败: %w", err)
	}

	b.consumers[topic] = msgs

	// 启动消费 goroutine
	go b.consume(ctx, msgs, handler, topic)

	return nil
}

// consume 消费消息
func (b *RabbitMQBroker) consume(ctx context.Context, msgs <-chan amqp.Delivery, handler MessageHandler, _ string) {
	for {
		select {
		case <-ctx.Done():
			return
		case d, ok := <-msgs:
			if !ok {
				return
			}

			// 解析消息
			var msg Message
			if err := json.Unmarshal(d.Body, &msg); err != nil {
				// 解析失败，拒绝消息（不重试）
				zap.L().Error("failed to unmarshal message",
					zap.Error(err),
					zap.ByteString("body", d.Body))
				if nackErr := d.Nack(false, false); nackErr != nil {
					zap.L().Error("failed to nack message after unmarshal error", zap.Error(nackErr))
				}
				continue
			}

			// 【重点】处理消息
			// 注意：先处理业务，成功后再 Ack
			err := handler(ctx, &msg)
			if err != nil {
				// 处理失败，根据重试次数决定是否重试
				msg.Attempts++
				zap.L().Error("failed to handle message",
					zap.Error(err),
					zap.Int("attempts", msg.Attempts),
					zap.String("message_id", msg.ID))
				if msg.Attempts < 3 {
					// 重新入队重试
					if nackErr := d.Nack(false, true); nackErr != nil {
						zap.L().Error("failed to nack message for retry", zap.Error(nackErr))
					}
				} else {
					// 超过重试次数，拒绝消息（进入死信队列）
					zap.L().Warn("message exceeded max retries, sending to DLQ",
						zap.String("message_id", msg.ID),
						zap.Int("attempts", msg.Attempts))
					if nackErr := d.Nack(false, false); nackErr != nil {
						zap.L().Error("failed to nack message to DLQ", zap.Error(nackErr))
					}
				}
				continue
			}

			// 【重点】手动确认消息
			d.Ack(false)
		}
	}
}

// Close 关闭连接
func (b *RabbitMQBroker) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil
	}

	b.closed = true

	if b.channel != nil {
		b.channel.Close()
	}
	if b.conn != nil {
		b.conn.Close()
	}

	return nil
}

// Healthy 健康检查
func (b *RabbitMQBroker) Healthy(ctx context.Context) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed || b.conn == nil {
		return false
	}

	return !b.conn.IsClosed()
}

// ========================================================================
// 【重点】RabbitMQ 最佳实践
// ========================================================================
// 1. 连接管理：
//    - 使用长连接，避免频繁创建销毁
//    - 使用连接池管理多个连接
//    - 实现断线重连机制
//
// 2. Channel 管理：
//    - 每个 goroutine 使用独立 Channel
//    - Channel 不是线程安全的
//
// 3. 消息确认：
//    - 生产者开启 Confirm 模式
//    - 消费者关闭 autoAck，手动确认
//
// 4. 错误处理：
//    - 实现重试机制
//    - 使用死信队列处理失败消息
//
// 5. 监控告警：
//    - 监控队列深度
//    - 监控消费者数量
//    - 监控消息堆积
// ========================================================================

// reconnect 重连机制
func (b *RabbitMQBroker) reconnect() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// 关闭旧连接
	if b.channel != nil {
		b.channel.Close()
	}
	if b.conn != nil {
		b.conn.Close()
	}

	// 重新连接
	return b.connect()
}

// notifyClose 监听连接关闭事件
func (b *RabbitMQBroker) notifyClose() {
	closeErr := make(chan *amqp.Error)
	b.conn.NotifyClose(closeErr)

	go func() {
		for err := range closeErr {
			if err != nil {
				fmt.Printf("RabbitMQ 连接关闭: %v, 尝试重连...\n", err)
				for i := 0; i < 3; i++ {
					if err := b.reconnect(); err == nil {
						fmt.Println("RabbitMQ 重连成功")
						break
					}
					time.Sleep(time.Second * time.Duration(i+1))
				}
			}
		}
	}()
}

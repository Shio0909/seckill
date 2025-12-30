// Package delayqueue 提供基于 Redis ZSET 的延迟队列实现
// 触发机制：拉模式
//
// 【重点学习】延迟队列的核心原理
// 使用 Redis 有序集合（ZSET）实现延迟队列：
// - Score: 任务到期时间戳（Unix 秒/毫秒）
// - Member: 任务标识或序列化的任务数据
// - 通过 ZRANGEBYSCORE 获取到期任务
//
// 【面试高频】
// Q1: 延迟队列有哪些实现方案？
// A1: 1) Redis ZSET：简单高效，适合中小规模
//  2. RabbitMQ DLX：利用死信队列实现
//  3. Kafka 时间轮：高吞吐，实现复杂
//  4. 数据库轮询：简单可靠但性能差
//  5. 时间轮算法：内存中实现，配合持久化
//
// Q2: Redis ZSET 实现延迟队列的优缺点？
// A2: 优点：实现简单、O(logN) 复杂度、原生支持范围查询
//
//	缺点：需要轮询、精度取决于轮询间隔、大量任务时内存压力
//
// Q3: 如何保证延迟任务不被重复消费？
// A3: 使用 ZPOPMIN 或 Lua 脚本原子性取出并删除任务
//
// Q4: 如何处理消费失败的任务？
// A4: 重试机制 + 死信队列，超过最大重试次数放入死信队列
//
// 面试高频问题（补充）：
// Q: 时间轮 (Time Wheel) 算法是什么？
// A: 时间轮是一种高效的定时器算法，类似于钟表。
//
//	它将时间划分为多个槽 (Slot)，每个槽代表一个时间间隔。
//	任务根据到期时间放入对应的槽中。指针随着时间转动，指向哪个槽就执行哪个槽里的任务。
//	Netty, Kafka, Dubbo 等框架都使用了时间轮。
//
// Q: Redis ZSET 方案在大规模任务下有什么瓶颈？
// A: 1. 内存占用：所有任务都在内存中，如果积压大量任务，Redis 内存会飙升。
//  2. 大 Key 问题：如果同一个 ZSET 包含数百万元素，操作性能会下降，且可能阻塞 Redis 主线程。
//     解决方案：分片（多个 ZSET），或者使用专门的消息队列（RocketMQ 定时消息）。
package delayqueue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// 【重点学习】延迟队列相关错误定义
var (
	ErrQueueClosed    = errors.New("queue is closed")
	ErrTaskNotFound   = errors.New("task not found")
	ErrInvalidTask    = errors.New("invalid task")
	ErrMaxRetryExceed = errors.New("max retry exceeded")
)

// Task 延迟任务
// 【重点学习】任务数据结构设计
type Task struct {
	ID         string                 `json:"id"`          // 任务唯一标识
	Type       string                 `json:"type"`        // 任务类型（用于路由到不同处理器）
	Payload    map[string]interface{} `json:"payload"`     // 任务数据
	ExecuteAt  time.Time              `json:"execute_at"`  // 计划执行时间
	CreatedAt  time.Time              `json:"created_at"`  // 创建时间
	RetryCount int                    `json:"retry_count"` // 已重试次数
	MaxRetry   int                    `json:"max_retry"`   // 最大重试次数
	RetryDelay time.Duration          `json:"retry_delay"` // 重试间隔
}

// NewTask 创建新任务
func NewTask(id, taskType string, payload map[string]interface{}, delay time.Duration) *Task {
	now := time.Now()
	return &Task{
		ID:         id,
		Type:       taskType,
		Payload:    payload,
		ExecuteAt:  now.Add(delay),
		CreatedAt:  now,
		RetryCount: 0,
		MaxRetry:   3,           // 默认最多重试 3 次
		RetryDelay: time.Minute, // 默认重试间隔 1 分钟
	}
}

// TaskHandler 任务处理函数
// 返回 error 时会触发重试机制
type TaskHandler func(ctx context.Context, task *Task) error

// DelayQueue 延迟队列
// 【重点学习】延迟队列核心组件
type DelayQueue struct {
	client        *redis.Client
	queueKey      string                 // 主队列 key（ZSET）
	deadLetterKey string                 // 死信队列 key（LIST）
	handlers      map[string]TaskHandler // 任务类型 -> 处理器映射
	handlerMu     sync.RWMutex

	// 【重点学习】消费者控制
	pollInterval time.Duration // 轮询间隔
	batchSize    int64         // 每次获取的任务数量
	workers      int           // 并发消费者数量

	closed    chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup
}

// Option 队列配置选项
type Option func(*DelayQueue)

// WithPollInterval 设置轮询间隔
// 【重点学习】轮询间隔的权衡
// - 太短：增加 Redis 压力
// - 太长：任务执行延迟
// - 推荐：100ms ~ 1s
func WithPollInterval(interval time.Duration) Option {
	return func(q *DelayQueue) {
		q.pollInterval = interval
	}
}

// WithBatchSize 设置每次获取的任务数量
func WithBatchSize(size int64) Option {
	return func(q *DelayQueue) {
		q.batchSize = size
	}
}

// WithWorkers 设置并发消费者数量
func WithWorkers(workers int) Option {
	return func(q *DelayQueue) {
		q.workers = workers
	}
}

// New 创建延迟队列
func New(client *redis.Client, queueName string, opts ...Option) *DelayQueue {
	q := &DelayQueue{
		client:        client,
		queueKey:      "delayqueue:" + queueName,
		deadLetterKey: "delayqueue:" + queueName + ":dead",
		handlers:      make(map[string]TaskHandler),
		pollInterval:  500 * time.Millisecond, // 默认 500ms 轮询一次
		batchSize:     100,                    // 默认每次取 100 个任务
		workers:       5,                      // 默认 5 个消费者
		closed:        make(chan struct{}),
	}

	for _, opt := range opts {
		opt(q)
	}

	return q
}

// RegisterHandler 注册任务处理器
// 【重点学习】处理器模式
// 不同类型的任务路由到不同的处理器，实现解耦
func (q *DelayQueue) RegisterHandler(taskType string, handler TaskHandler) {
	q.handlerMu.Lock()
	defer q.handlerMu.Unlock()
	q.handlers[taskType] = handler
}

// Lua 脚本：原子性获取并删除到期任务
// 【面试高频】为什么用 Lua 脚本？
// 避免多个消费者获取同一任务（竞态条件）
var fetchTaskScript = redis.NewScript(`
-- KEYS[1]: 队列 key
-- ARGV[1]: 当前时间戳
-- ARGV[2]: 获取数量

-- 获取到期任务
local tasks = redis.call('ZRANGEBYSCORE', KEYS[1], '-inf', ARGV[1], 'LIMIT', 0, ARGV[2])

if #tasks == 0 then
    return {}
end

-- 删除已获取的任务
redis.call('ZREM', KEYS[1], unpack(tasks))

return tasks
`)

// Push 添加延迟任务
// 【重点学习】任务入队流程
func (q *DelayQueue) Push(ctx context.Context, task *Task) error {
	if task.ID == "" || task.Type == "" {
		return ErrInvalidTask
	}

	// 序列化任务
	data, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("marshal task failed: %w", err)
	}

	// 使用到期时间作为 score
	score := float64(task.ExecuteAt.UnixMilli())

	// 添加到 ZSET
	err = q.client.ZAdd(ctx, q.queueKey, redis.Z{
		Score:  score,
		Member: string(data),
	}).Err()

	if err != nil {
		return fmt.Errorf("push task failed: %w", err)
	}

	return nil
}

// PushWithDelay 添加延迟任务（便捷方法）
func (q *DelayQueue) PushWithDelay(ctx context.Context, id, taskType string, payload map[string]interface{}, delay time.Duration) error {
	task := NewTask(id, taskType, payload, delay)
	return q.Push(ctx, task)
}

// Remove 移除任务
func (q *DelayQueue) Remove(ctx context.Context, taskID string) error {
	// 需要遍历查找，效率较低
	// 生产环境建议额外维护一个 ID -> task 的映射
	// 这里简化处理，实际应用中可以优化
	return nil
}

// Start 启动消费者
// 【重点学习】消费者启动流程
func (q *DelayQueue) Start(ctx context.Context) {
	// 启动 worker 数量的消费者
	for i := 0; i < q.workers; i++ {
		q.wg.Add(1)
		go q.consume(ctx, i)
	}
}

// consume 消费循环
// 【重点学习】消费者核心逻辑
func (q *DelayQueue) consume(ctx context.Context, workerID int) {
	defer q.wg.Done()

	ticker := time.NewTicker(q.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-q.closed:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			q.processTasks(ctx, workerID)
		}
	}
}

// processTasks 处理到期任务
// 【重点学习】批量处理提升效率
func (q *DelayQueue) processTasks(ctx context.Context, _ int) {
	now := time.Now().UnixMilli()

	// 原子性获取到期任务
	result, err := fetchTaskScript.Run(ctx, q.client, []string{q.queueKey}, now, q.batchSize).Result()
	if err != nil {
		return
	}

	tasks, ok := result.([]interface{})
	if !ok || len(tasks) == 0 {
		return
	}

	// 处理每个任务
	for _, item := range tasks {
		taskData, ok := item.(string)
		if !ok {
			continue
		}

		var task Task
		if err := json.Unmarshal([]byte(taskData), &task); err != nil {
			continue
		}

		q.executeTask(ctx, &task)
	}
}

// executeTask 执行单个任务
// 【重点学习】任务执行与重试机制
func (q *DelayQueue) executeTask(ctx context.Context, task *Task) {
	// 获取对应的处理器
	q.handlerMu.RLock()
	handler, exists := q.handlers[task.Type]
	q.handlerMu.RUnlock()

	if !exists {
		// 未知任务类型，放入死信队列
		q.moveToDeadLetter(ctx, task, "unknown task type")
		return
	}

	// 执行任务
	err := handler(ctx, task)
	if err == nil {
		return // 执行成功
	}

	// 【重点学习】执行失败的重试逻辑
	task.RetryCount++
	if task.RetryCount >= task.MaxRetry {
		// 超过最大重试次数，放入死信队列
		q.moveToDeadLetter(ctx, task, err.Error())
		return
	}

	// 重新入队，延迟后重试
	task.ExecuteAt = time.Now().Add(task.RetryDelay)
	if err := q.Push(ctx, task); err != nil {
		zap.L().Error("failed to push task for retry",
			zap.Error(err),
			zap.String("task_id", task.ID),
			zap.String("task_type", task.Type))
	}
}

// moveToDeadLetter 移动到死信队列
// 【面试高频】死信队列的作用
// 保存处理失败的任务，便于后续人工处理或分析
func (q *DelayQueue) moveToDeadLetter(ctx context.Context, task *Task, reason string) {
	deadTask := struct {
		Task   *Task  `json:"task"`
		Reason string `json:"reason"`
		Time   string `json:"time"`
	}{
		Task:   task,
		Reason: reason,
		Time:   time.Now().Format(time.RFC3339),
	}

	data, _ := json.Marshal(deadTask)
	q.client.LPush(ctx, q.deadLetterKey, string(data))
}

// Stop 停止队列
// 【重点学习】优雅停机
func (q *DelayQueue) Stop() {
	q.closeOnce.Do(func() {
		close(q.closed)
	})
	q.wg.Wait()
}

// Len 获取队列长度
func (q *DelayQueue) Len(ctx context.Context) (int64, error) {
	return q.client.ZCard(ctx, q.queueKey).Result()
}

// DeadLetterLen 获取死信队列长度
func (q *DelayQueue) DeadLetterLen(ctx context.Context) (int64, error) {
	return q.client.LLen(ctx, q.deadLetterKey).Result()
}

// ========================================
// 订单超时取消专用组件
// ========================================

// 【重点学习】订单超时取消是延迟队列的典型应用场景
// 用户下单后启动定时器，超时未支付则自动取消订单

const (
	// 订单超时任务类型
	TaskTypeOrderTimeout = "order:timeout"
	// 默认订单超时时间
	DefaultOrderTimeout = 30 * time.Minute
)

// OrderTimeoutPayload 订单超时任务载荷
type OrderTimeoutPayload struct {
	OrderID   string `json:"order_id"`
	UserID    int64  `json:"user_id"`
	ProductID int64  `json:"product_id"`
	Quantity  int    `json:"quantity"`
}

// OrderTimeoutQueue 订单超时专用队列
// 【重点学习】对通用延迟队列的封装，提供更友好的 API
type OrderTimeoutQueue struct {
	*DelayQueue
	timeout time.Duration
}

// NewOrderTimeoutQueue 创建订单超时队列
func NewOrderTimeoutQueue(client *redis.Client, timeout time.Duration, opts ...Option) *OrderTimeoutQueue {
	if timeout == 0 {
		timeout = DefaultOrderTimeout
	}

	return &OrderTimeoutQueue{
		DelayQueue: New(client, "order-timeout", opts...),
		timeout:    timeout,
	}
}

// AddOrder 添加订单到超时队列
// 【重点学习】创建订单时调用，开始超时计时
func (q *OrderTimeoutQueue) AddOrder(ctx context.Context, payload OrderTimeoutPayload) error {
	task := NewTask(
		payload.OrderID,
		TaskTypeOrderTimeout,
		map[string]interface{}{
			"order_id":   payload.OrderID,
			"user_id":    payload.UserID,
			"product_id": payload.ProductID,
			"quantity":   payload.Quantity,
		},
		q.timeout,
	)
	task.MaxRetry = 1 // 订单取消只重试一次
	return q.Push(ctx, task)
}

// RemoveOrder 从超时队列移除订单
// 【重点学习】支付成功时调用，取消超时计时
func (q *OrderTimeoutQueue) RemoveOrder(ctx context.Context, orderID string) error {
	// 通过遍历 ZSET 查找并删除
	// 生产环境建议使用 order_id -> task_data 的映射优化
	iter := q.client.ZScan(ctx, q.queueKey, 0, "*"+orderID+"*", 0).Iterator()
	for iter.Next(ctx) {
		q.client.ZRem(ctx, q.queueKey, iter.Val())
	}
	return iter.Err()
}

// SetHandler 设置订单超时处理器
// 【重点学习】注入订单取消逻辑
func (q *OrderTimeoutQueue) SetHandler(handler func(ctx context.Context, payload OrderTimeoutPayload) error) {
	q.RegisterHandler(TaskTypeOrderTimeout, func(ctx context.Context, task *Task) error {
		payload := OrderTimeoutPayload{
			OrderID:   task.Payload["order_id"].(string),
			UserID:    int64(task.Payload["user_id"].(float64)),
			ProductID: int64(task.Payload["product_id"].(float64)),
			Quantity:  int(task.Payload["quantity"].(float64)),
		}
		return handler(ctx, payload)
	})
}

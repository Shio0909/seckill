package delayqueue

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestRedis(t *testing.T) (*redis.Client, func()) {
	mr, err := miniredis.Run()
	require.NoError(t, err)

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	return client, func() {
		client.Close()
		mr.Close()
	}
}

func TestDelayQueue_PushAndProcess(t *testing.T) {
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	ctx := context.Background()
	queue := New(client, "test",
		WithPollInterval(50*time.Millisecond),
		WithBatchSize(10),
		WithWorkers(1),
	)

	// 记录处理的任务
	var processedTasks []string
	var mu sync.Mutex

	queue.RegisterHandler("test-task", func(ctx context.Context, task *Task) error {
		mu.Lock()
		processedTasks = append(processedTasks, task.ID)
		mu.Unlock()
		return nil
	})

	// 添加延迟任务（100ms 后执行）
	err := queue.PushWithDelay(ctx, "task-1", "test-task", map[string]interface{}{"data": "hello"}, 100*time.Millisecond)
	require.NoError(t, err)

	// 启动消费者
	queue.Start(ctx)

	// 立即检查：任务应该还没被处理
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	assert.Empty(t, processedTasks, "任务不应该立即被处理")
	mu.Unlock()

	// 等待任务执行
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	assert.Contains(t, processedTasks, "task-1", "任务应该被处理")
	mu.Unlock()

	queue.Stop()
}

func TestDelayQueue_ExecutionOrder(t *testing.T) {
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	ctx := context.Background()
	queue := New(client, "order-test",
		WithPollInterval(20*time.Millisecond),
		WithWorkers(1),
	)

	var executionOrder []string
	var mu sync.Mutex

	queue.RegisterHandler("order-task", func(ctx context.Context, task *Task) error {
		mu.Lock()
		executionOrder = append(executionOrder, task.ID)
		mu.Unlock()
		return nil
	})

	// 按不同延迟添加任务
	if err := queue.PushWithDelay(ctx, "task-3", "order-task", nil, 150*time.Millisecond); err != nil {
		t.Fatalf("failed to push task-3: %v", err)
	}
	if err := queue.PushWithDelay(ctx, "task-1", "order-task", nil, 50*time.Millisecond); err != nil {
		t.Fatalf("failed to push task-1: %v", err)
	}
	if err := queue.PushWithDelay(ctx, "task-2", "order-task", nil, 100*time.Millisecond); err != nil {
		t.Fatalf("failed to push task-2: %v", err)
	}

	queue.Start(ctx)
	time.Sleep(300 * time.Millisecond)
	queue.Stop()

	mu.Lock()
	defer mu.Unlock()

	// 验证执行顺序（按延迟时间）
	require.Len(t, executionOrder, 3)
	assert.Equal(t, "task-1", executionOrder[0])
	assert.Equal(t, "task-2", executionOrder[1])
	assert.Equal(t, "task-3", executionOrder[2])
}

func TestDelayQueue_Retry(t *testing.T) {
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	ctx := context.Background()
	queue := New(client, "retry-test",
		WithPollInterval(20*time.Millisecond),
		WithWorkers(1),
	)

	var attemptCount int32

	queue.RegisterHandler("retry-task", func(ctx context.Context, task *Task) error {
		count := atomic.AddInt32(&attemptCount, 1)
		if count < 3 {
			return errors.New("simulated failure")
		}
		return nil // 第三次成功
	})

	// 添加任务，设置短重试间隔便于测试
	task := NewTask("retry-1", "retry-task", nil, 50*time.Millisecond)
	task.MaxRetry = 3
	task.RetryDelay = 50 * time.Millisecond
	if err := queue.Push(ctx, task); err != nil {
		t.Fatalf("failed to push task: %v", err)
	}

	queue.Start(ctx)
	time.Sleep(500 * time.Millisecond)
	queue.Stop()

	// 验证重试次数
	assert.Equal(t, int32(3), atomic.LoadInt32(&attemptCount), "应该重试 3 次")
}

func TestDelayQueue_DeadLetter(t *testing.T) {
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	ctx := context.Background()
	queue := New(client, "dead-letter-test",
		WithPollInterval(20*time.Millisecond),
		WithWorkers(1),
	)

	// 总是失败的处理器
	queue.RegisterHandler("fail-task", func(ctx context.Context, task *Task) error {
		return errors.New("always fail")
	})

	// 添加任务，设置只重试 1 次
	task := NewTask("fail-1", "fail-task", nil, 10*time.Millisecond)
	task.MaxRetry = 1
	task.RetryDelay = 10 * time.Millisecond
	if err := queue.Push(ctx, task); err != nil {
		t.Fatalf("failed to push task: %v", err)
	}

	queue.Start(ctx)
	time.Sleep(200 * time.Millisecond)
	queue.Stop()

	// 验证任务进入死信队列
	deadLen, err := queue.DeadLetterLen(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deadLen, "应该有一个死信任务")
}

func TestDelayQueue_UnknownTaskType(t *testing.T) {
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	ctx := context.Background()
	queue := New(client, "unknown-test",
		WithPollInterval(20*time.Millisecond),
		WithWorkers(1),
	)

	// 不注册任何处理器，直接添加任务
	if err := queue.PushWithDelay(ctx, "unknown-1", "unknown-type", nil, 10*time.Millisecond); err != nil {
		t.Fatalf("failed to push task: %v", err)
	}

	queue.Start(ctx)
	time.Sleep(100 * time.Millisecond)
	queue.Stop()

	// 验证任务进入死信队列
	deadLen, err := queue.DeadLetterLen(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deadLen, "未知类型任务应该进入死信队列")
}

func TestDelayQueue_Len(t *testing.T) {
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	ctx := context.Background()
	queue := New(client, "len-test")

	// 添加多个任务
	for i := 0; i < 5; i++ {
		if err := queue.PushWithDelay(ctx, fmt.Sprintf("task-%d", i), "test", nil, time.Hour); err != nil {
			t.Fatalf("failed to push task-%d: %v", i, err)
		}
	}

	// 验证队列长度
	length, err := queue.Len(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(5), length)
}

// 订单超时队列测试

func TestOrderTimeoutQueue_Basic(t *testing.T) {
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	ctx := context.Background()
	timeout := 100 * time.Millisecond
	queue := NewOrderTimeoutQueue(client, timeout,
		WithPollInterval(20*time.Millisecond),
		WithWorkers(1),
	)

	var canceledOrders []string
	var mu sync.Mutex

	// 设置订单取消处理器
	queue.SetHandler(func(ctx context.Context, payload OrderTimeoutPayload) error {
		mu.Lock()
		canceledOrders = append(canceledOrders, payload.OrderID)
		mu.Unlock()
		return nil
	})

	// 添加订单
	err := queue.AddOrder(ctx, OrderTimeoutPayload{
		OrderID:   "order-001",
		UserID:    1,
		ProductID: 100,
		Quantity:  1,
	})
	require.NoError(t, err)

	// 启动消费者
	queue.Start(ctx)

	// 等待超时
	time.Sleep(200 * time.Millisecond)
	queue.Stop()

	// 验证订单被取消
	mu.Lock()
	assert.Contains(t, canceledOrders, "order-001")
	mu.Unlock()
}

func TestOrderTimeoutQueue_RemoveOnPayment(t *testing.T) {
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	ctx := context.Background()
	timeout := 200 * time.Millisecond
	queue := NewOrderTimeoutQueue(client, timeout,
		WithPollInterval(20*time.Millisecond),
		WithWorkers(1),
	)

	var canceledOrders []string
	var mu sync.Mutex

	queue.SetHandler(func(ctx context.Context, payload OrderTimeoutPayload) error {
		mu.Lock()
		canceledOrders = append(canceledOrders, payload.OrderID)
		mu.Unlock()
		return nil
	})

	// 添加订单
	if err := queue.AddOrder(ctx, OrderTimeoutPayload{
		OrderID:   "order-002",
		UserID:    1,
		ProductID: 100,
		Quantity:  1,
	}); err != nil {
		t.Fatalf("failed to add order: %v", err)
	}

	// 模拟支付成功，移除订单
	time.Sleep(50 * time.Millisecond)
	if err := queue.RemoveOrder(ctx, "order-002"); err != nil {
		t.Fatalf("failed to remove order: %v", err)
	}

	// 启动消费者
	queue.Start(ctx)
	time.Sleep(300 * time.Millisecond)
	queue.Stop()

	// 验证订单没有被取消
	mu.Lock()
	assert.NotContains(t, canceledOrders, "order-002", "已支付订单不应被取消")
	mu.Unlock()
}

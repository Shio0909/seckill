package distlock

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 【重点学习】使用 miniredis 进行单元测试
// miniredis 是一个纯 Go 实现的 Redis 服务器，适合单元测试
// 优点：不依赖外部 Redis、速度快、可控制行为
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

// 【重点学习】表格驱动测试：分布式锁基本功能
func TestLock_BasicOperations(t *testing.T) {
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	distLock := New(client)
	ctx := context.Background()

	tests := []struct {
		name    string
		key     string
		opts    []LockOption
		wantErr bool
	}{
		{
			name:    "基本获取和释放锁",
			key:     "test-lock-1",
			opts:    nil,
			wantErr: false,
		},
		{
			name:    "自定义TTL",
			key:     "test-lock-2",
			opts:    []LockOption{WithTTL(5 * time.Second)},
			wantErr: false,
		},
		{
			name:    "启用看门狗",
			key:     "test-lock-3",
			opts:    []LockOption{WithWatchDog()},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lock := distLock.NewLock(tt.key, tt.opts...)

			// 获取锁
			err := lock.Lock(ctx)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			// 验证锁存在
			val, err := client.Get(ctx, lock.Key()).Result()
			assert.NoError(t, err)
			assert.Equal(t, lock.Value(), val)

			// 释放锁
			err = lock.Unlock(ctx)
			assert.NoError(t, err)

			// 验证锁已释放
			_, err = client.Get(ctx, lock.Key()).Result()
			assert.ErrorIs(t, err, redis.Nil)
		})
	}
}

// 【面试高频】测试锁的互斥性
// 同一时刻只能有一个客户端持有锁
func TestLock_Mutex(t *testing.T) {
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	distLock := New(client)
	ctx := context.Background()
	key := "mutex-test"

	// 第一个客户端获取锁
	lock1 := distLock.NewLock(key)
	err := lock1.Lock(ctx)
	require.NoError(t, err)

	// 第二个客户端尝试获取锁（应该失败）
	lock2 := distLock.NewLock(key)
	ok, err := lock2.TryLock(ctx)
	assert.NoError(t, err)
	assert.False(t, ok, "第二个客户端不应该获取到锁")

	// 释放第一个锁
	err = lock1.Unlock(ctx)
	require.NoError(t, err)

	// 第二个客户端现在可以获取锁
	ok, err = lock2.TryLock(ctx)
	assert.NoError(t, err)
	assert.True(t, ok, "第二个客户端应该能获取锁")

	err = lock2.Unlock(ctx)
	assert.NoError(t, err)
}

// 【重点学习】测试锁的安全释放
// 只有锁的持有者才能释放锁
func TestLock_SafeRelease(t *testing.T) {
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	distLock := New(client)
	ctx := context.Background()
	key := "safe-release-test"

	// 客户端 A 获取锁
	lockA := distLock.NewLock(key)
	err := lockA.Lock(ctx)
	require.NoError(t, err)

	// 客户端 B 尝试释放锁（应该失败）
	lockB := distLock.NewLock(key)
	err = lockB.Unlock(ctx)
	assert.ErrorIs(t, err, ErrLockNotHeld, "不应该能释放别人的锁")

	// 验证锁仍然存在
	val, err := client.Get(ctx, lockA.Key()).Result()
	assert.NoError(t, err)
	assert.Equal(t, lockA.Value(), val)

	// 客户端 A 释放锁
	err = lockA.Unlock(ctx)
	assert.NoError(t, err)
}

// 【重点学习】测试重试机制
// 注意：此测试模拟锁被主动释放后重试成功的场景
// miniredis 的 TTL 过期行为与真实 Redis 不同，因此使用主动释放方式测试
func TestLock_Retry(t *testing.T) {
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	distLock := New(client)
	ctx := context.Background()
	key := "retry-test"

	// 客户端 A 获取锁
	lockA := distLock.NewLock(key, WithTTL(5*time.Second))
	err := lockA.Lock(ctx)
	require.NoError(t, err)

	// 启动一个 goroutine 在 200ms 后释放锁
	go func() {
		time.Sleep(200 * time.Millisecond)
		if err := lockA.Unlock(ctx); err != nil {
			// 在测试中记录错误
			t.Logf("failed to unlock lockA: %v", err)
		}
	}()

	// 客户端 B 尝试获取锁，设置重试
	lockB := distLock.NewLock(key, WithRetry(10, 50*time.Millisecond))

	start := time.Now()
	err = lockB.Lock(ctx)
	elapsed := time.Since(start)

	assert.NoError(t, err, "应该在锁释放后获取成功")
	assert.True(t, elapsed >= 200*time.Millisecond, "应该等待锁释放")

	err = lockB.Unlock(ctx)
	assert.NoError(t, err)
}

// 【面试高频】测试看门狗自动续期
func TestLock_WatchDog(t *testing.T) {
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	distLock := New(client)
	ctx := context.Background()
	key := "watchdog-test"

	// 短 TTL + 启用看门狗
	lock := distLock.NewLock(key,
		WithTTL(300*time.Millisecond),
		WithWatchDog(),
	)

	err := lock.Lock(ctx)
	require.NoError(t, err)

	// 等待超过 TTL 时间
	time.Sleep(600 * time.Millisecond)

	// 由于看门狗续期，锁应该仍然存在
	val, err := client.Get(ctx, lock.Key()).Result()
	assert.NoError(t, err)
	assert.Equal(t, lock.Value(), val)

	err = lock.Unlock(ctx)
	assert.NoError(t, err)
}

// 【重点学习】并发测试：确保只有一个协程能获取锁
func TestLock_Concurrent(t *testing.T) {
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	distLock := New(client)
	ctx := context.Background()
	key := "concurrent-test"

	var (
		successCount int32
		wg           sync.WaitGroup
		goroutines   = 10
	)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			lock := distLock.NewLock(key)
			ok, err := lock.TryLock(ctx)
			if err != nil {
				return
			}
			if ok {
				atomic.AddInt32(&successCount, 1)
				time.Sleep(100 * time.Millisecond) // 模拟业务处理
				if err := lock.Unlock(ctx); err != nil {
					t.Logf("failed to unlock: %v", err)
				}
			}
		}()
	}

	wg.Wait()

	// 只有一个协程应该成功获取锁
	assert.Equal(t, int32(1), successCount, "只有一个协程应该获取到锁")
}

// 【重点学习】测试 WithLock 便捷函数
func TestWithLock(t *testing.T) {
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	Init(client)
	ctx := context.Background()

	executed := false
	err := WithLock(ctx, "with-lock-test", func() error {
		executed = true
		return nil
	})

	assert.NoError(t, err)
	assert.True(t, executed)
}

// 【面试高频】测试 context 取消
func TestLock_ContextCancel(t *testing.T) {
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	distLock := New(client)
	key := "context-cancel-test"

	// 先获取锁
	lock1 := distLock.NewLock(key)
	ctx := context.Background()
	err := lock1.Lock(ctx)
	require.NoError(t, err)

	// 使用带超时的 context 尝试获取锁
	ctxWithTimeout, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	lock2 := distLock.NewLock(key, WithRetry(100, 50*time.Millisecond))
	err = lock2.Lock(ctxWithTimeout)

	assert.Error(t, err, "应该因为 context 超时而失败")

	err = lock1.Unlock(ctx)
	assert.NoError(t, err)
}

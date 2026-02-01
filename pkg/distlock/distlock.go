// Package distlock 提供基于 Redis 的分布式锁实现
//
// 1. 互斥性：同一时刻只能有一个客户端持有锁
// 2. 防死锁：锁必须有过期时间，避免持有者崩溃导致死锁
// 3. 安全性：只有锁的持有者才能释放锁（通过唯一标识）
// 4. 可重入：同一客户端可以多次获取同一把锁
package distlock

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// 定义具体的错误类型便于上层业务针对性处理
var (
	ErrLockNotHeld     = errors.New("lock not held")            // 尝试释放未持有的锁
	ErrLockAcquireFail = errors.New("failed to acquire lock")   // 获取锁失败
	ErrLockTimeout     = errors.New("lock acquisition timeout") // 获取锁超时
)

// Lock 分布式锁实例
type Lock struct {
	client     *redis.Client
	key        string        // 锁的 key
	value      string        // 锁的唯一标识（用于安全释放）
	ttl        time.Duration // 锁的过期时间
	retryDelay time.Duration // 重试间隔
	retryCount int           // 重试次数

	// 看门狗用于自动续期，防止业务未完成锁就过期
	watchDog      bool          // 是否启用看门狗
	watchDogStop  chan struct{} // 停止看门狗的信号
	watchDogMutex sync.Mutex    // 保护 watchDog 状态
}

// LockOption 锁配置选项（函数式选项模式）
type LockOption func(*Lock)

// WithTTL 设置锁过期时间
// - 太短：业务未完成锁就过期，导致并发问题
// - 太长：持有者崩溃后长时间无法释放
// - 推荐：根据业务耗时设置，配合看门狗自动续期
func WithTTL(ttl time.Duration) LockOption {
	return func(l *Lock) {
		l.ttl = ttl
	}
}

// WithRetry 设置重试参数
// - retryCount: 重试次数，0 表示不重试
// - retryDelay: 重试间隔，建议加随机抖动避免惊群效应
func WithRetry(retryCount int, retryDelay time.Duration) LockOption {
	return func(l *Lock) {
		l.retryCount = retryCount
		l.retryDelay = retryDelay
	}
}

// WithWatchDog 启用看门狗自动续期
// - 后台 goroutine 定期检查锁是否仍被持有
// - 如果持有，则自动续期（延长过期时间）
// - 续期间隔通常为 TTL 的 1/3
// - 业务完成后必须调用 Unlock 停止看门狗
func WithWatchDog() LockOption {
	return func(l *Lock) {
		l.watchDog = true
	}
}

// DistLock 分布式锁管理器
type DistLock struct {
	client *redis.Client
}

// New 创建分布式锁管理器
func New(client *redis.Client) *DistLock {
	return &DistLock{client: client}
}

// NewLock 创建一把新锁
// UUID 确保不同客户端、不同时间获取的锁有唯一标识
func (d *DistLock) NewLock(key string, opts ...LockOption) *Lock {
	lock := &Lock{
		client:     d.client,
		key:        "distlock:" + key, // 添加前缀避免 key 冲突
		value:      uuid.New().String(),
		ttl:        30 * time.Second, // 默认 30 秒过期
		retryDelay: 100 * time.Millisecond,
		retryCount: 0,
		watchDog:   false,
	}

	for _, opt := range opts {
		opt(lock)
	}

	return lock
}

// Lua 脚本：原子性获取锁
// Redis 执行 Lua 脚本是原子的，可以保证多条命令的原子性
// 避免了 SETNX 和 EXPIRE 分开执行可能导致的问题
var acquireScript = redis.NewScript(`
-- KEYS[1]: 锁的 key
-- ARGV[1]: 锁的唯一标识
-- ARGV[2]: 过期时间（毫秒）

-- 尝试获取锁（SET NX PX）
-- NX: 仅当 key 不存在时设置
-- PX: 设置过期时间（毫秒）
if redis.call("SET", KEYS[1], ARGV[1], "NX", "PX", ARGV[2]) then
    return 1
end
return 0
`)

// Lua 脚本：原子性释放锁
var releaseScript = redis.NewScript(`
-- KEYS[1]: 锁的 key
-- ARGV[1]: 锁的唯一标识

-- 检查锁是否属于当前持有者
if redis.call("GET", KEYS[1]) == ARGV[1] then
    -- 是自己的锁，删除
    return redis.call("DEL", KEYS[1])
end
-- 不是自己的锁，返回 0
return 0
`)

// Lua 脚本：原子性续期
// 必须先确认锁还是自己的，才能续期
var renewScript = redis.NewScript(`
-- KEYS[1]: 锁的 key
-- ARGV[1]: 锁的唯一标识
-- ARGV[2]: 新的过期时间（毫秒）

if redis.call("GET", KEYS[1]) == ARGV[1] then
    return redis.call("PEXPIRE", KEYS[1], ARGV[2])
end
return 0
`)

// Lock 获取锁
// 1. 尝试 SET NX PX 原子获取锁
// 2. 获取失败则根据配置重试
// 3. 获取成功且启用看门狗则启动续期协程
func (l *Lock) Lock(ctx context.Context) error {
	var err error

	// 加 1 是因为第一次尝试不算重试
	for i := 0; i <= l.retryCount; i++ {
		err = l.tryLock(ctx)
		if err == nil {
			// 获取成功，启动看门狗
			if l.watchDog {
				l.startWatchDog(ctx)
			}
			return nil
		}

		// 最后一次尝试失败，不再等待
		if i == l.retryCount {
			break
		}

		// 等待重试，支持 context 取消
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(l.retryDelay):
			// 继续重试
		}
	}

	return ErrLockAcquireFail
}

// tryLock 单次尝试获取锁
func (l *Lock) tryLock(ctx context.Context) error {
	// 执行 Lua 脚本原子获取锁
	result, err := acquireScript.Run(ctx, l.client, []string{l.key}, l.value, l.ttl.Milliseconds()).Int()
	if err != nil {
		return fmt.Errorf("acquire lock error: %w", err)
	}

	if result == 1 {
		return nil // 获取成功
	}
	return ErrLockAcquireFail
}

// TryLock 尝试获取锁（非阻塞）
func (l *Lock) TryLock(ctx context.Context) (bool, error) {
	err := l.tryLock(ctx)
	if err == nil {
		if l.watchDog {
			l.startWatchDog(ctx)
		}
		return true, nil
	}
	if errors.Is(err, ErrLockAcquireFail) {
		return false, nil
	}
	return false, err
}

// Unlock 释放锁
// 1. 必须停止看门狗
// 2. 必须用 Lua 脚本原子释放
// 3. 返回值表示是否真正释放了锁
func (l *Lock) Unlock(ctx context.Context) error {
	// 停止看门狗
	l.stopWatchDog()

	// 执行 Lua 脚本原子释放锁
	result, err := releaseScript.Run(ctx, l.client, []string{l.key}, l.value).Int()
	if err != nil {
		return fmt.Errorf("release lock error: %w", err)
	}

	if result == 0 {
		return ErrLockNotHeld // 锁不是自己的，无法释放
	}
	return nil
}

// Renew 手动续期
// 适用于不启用看门狗但需要临时延长锁时间的场景
func (l *Lock) Renew(ctx context.Context) error {
	result, err := renewScript.Run(ctx, l.client, []string{l.key}, l.value, l.ttl.Milliseconds()).Int()
	if err != nil {
		return fmt.Errorf("renew lock error: %w", err)
	}

	if result == 0 {
		return ErrLockNotHeld
	}
	return nil
}

// startWatchDog 启动看门狗协程
// - 每隔 TTL/3 续期一次
// - 确保业务执行期间锁不会过期
// - 必须在 Unlock 时停止
func (l *Lock) startWatchDog(ctx context.Context) {
	l.watchDogMutex.Lock()
	defer l.watchDogMutex.Unlock()

	l.watchDogStop = make(chan struct{})

	// 续期间隔：TTL 的 1/3
	renewInterval := l.ttl / 3

	go func() {
		ticker := time.NewTicker(renewInterval)
		defer ticker.Stop()

		for {
			select {
			case <-l.watchDogStop:
				// 收到停止信号
				return
			case <-ctx.Done():
				// context 取消
				return
			case <-ticker.C:
				// 尝试续期
				if err := l.Renew(context.Background()); err != nil {
					// 续期失败（可能锁已被释放或过期）
					return
				}
			}
		}
	}()
}

// stopWatchDog 停止看门狗协程
func (l *Lock) stopWatchDog() {
	l.watchDogMutex.Lock()
	defer l.watchDogMutex.Unlock()

	if l.watchDogStop != nil {
		close(l.watchDogStop)
		l.watchDogStop = nil
	}
}

// Key 返回锁的 key
func (l *Lock) Key() string {
	return l.key
}

// Value 返回锁的唯一标识
func (l *Lock) Value() string {
	return l.value
}

// TTL 返回锁的过期时间
func (l *Lock) TTL() time.Duration {
	return l.ttl
}

// 便捷函数：全局分布式锁管理器

var defaultDistLock *DistLock
var once sync.Once

// Init 初始化全局分布式锁管理器
func Init(client *redis.Client) {
	once.Do(func() {
		defaultDistLock = New(client)
	})
}

// AcquireLock 便捷函数：获取锁
func AcquireLock(ctx context.Context, key string, opts ...LockOption) (*Lock, error) {
	if defaultDistLock == nil {
		return nil, errors.New("distlock not initialized, call Init first")
	}

	lock := defaultDistLock.NewLock(key, opts...)
	if err := lock.Lock(ctx); err != nil {
		return nil, err
	}
	return lock, nil
}

// WithLock 便捷函数：在锁保护下执行函数
func WithLock(ctx context.Context, key string, fn func() error, opts ...LockOption) error {
	lock, err := AcquireLock(ctx, key, opts...)
	if err != nil {
		return fmt.Errorf("acquire lock failed: %w", err)
	}
	defer func() {
		if unlockErr := lock.Unlock(ctx); unlockErr != nil {
			// 记录解锁失败，但不影响主逻辑
			zap.L().Error("failed to unlock in WithLock",
				zap.Error(unlockErr),
				zap.String("key", key))
		}
	}()

	return fn()
}

package middleware

import (
	"sync"
	"testing"
	"time"
)

// ========================================================================
// 【重点学习】令牌桶算法测试
// ========================================================================
// 测试要点：
// 1. 初始状态：桶是满的
// 2. 消耗测试：允许消耗直到桶空
// 3. 恢复测试：等待后应该恢复令牌
// 4. 并发测试：多 goroutine 同时消耗
// ========================================================================

func TestTokenBucket_Basic(t *testing.T) {
	// 创建一个 rate=10/s, capacity=5 的桶
	bucket := NewTokenBucket(10, 5)

	// 初始应该有 5 个令牌
	for i := 0; i < 5; i++ {
		if !bucket.Allow() {
			t.Errorf("第 %d 次请求应该被允许", i+1)
		}
	}

	// 第 6 次应该被拒绝
	if bucket.Allow() {
		t.Error("第 6 次请求应该被拒绝")
	}
}

func TestTokenBucket_Refill(t *testing.T) {
	// 创建一个 rate=100/s, capacity=10 的桶
	bucket := NewTokenBucket(100, 10)

	// 消耗所有令牌
	for i := 0; i < 10; i++ {
		bucket.Allow()
	}

	// 此时应该没有令牌
	if bucket.Allow() {
		t.Error("消耗完毕后不应该有令牌")
	}

	// 等待 100ms，应该恢复约 10 个令牌
	time.Sleep(100 * time.Millisecond)

	// 应该可以再消耗
	if !bucket.Allow() {
		t.Error("等待后应该恢复令牌")
	}
}

func TestTokenBucket_Concurrent(t *testing.T) {
	bucket := NewTokenBucket(1000, 100)

	var wg sync.WaitGroup
	var allowed, denied int64
	var mu sync.Mutex

	// 200 个 goroutine 同时请求
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mu.Lock()
			defer mu.Unlock()
			if bucket.Allow() {
				allowed++
			} else {
				denied++
			}
		}()
	}

	wg.Wait()

	// 最多允许 100 个（桶容量）
	if allowed > 100 {
		t.Errorf("allowed = %d, 应该 <= 100", allowed)
	}

	t.Logf("允许: %d, 拒绝: %d", allowed, denied)
}

// ========================================================================
// 【重点学习】幂等性测试思路
// ========================================================================
// 由于幂等性依赖 Redis，这里只测试辅助函数
// 完整测试需要集成测试环境
// ========================================================================

func TestGenerateIdempotentKey(t *testing.T) {
	tests := []struct {
		name     string
		userID   uint
		resource string
		wantKey  string
	}{
		{
			name:     "正常生成",
			userID:   123,
			resource: "order",
			wantKey:  "idempotent:123:order",
		},
		{
			name:     "用户ID为0",
			userID:   0,
			resource: "payment",
			wantKey:  "idempotent:0:payment",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 模拟 key 生成逻辑
			key := generateTestKey(tt.userID, tt.resource)
			if key != tt.wantKey {
				t.Errorf("key = %s, want %s", key, tt.wantKey)
			}
		})
	}
}

// 测试用的 key 生成函数
func generateTestKey(userID uint, resource string) string {
	return "idempotent:" + uintToString(userID) + ":" + resource
}

func uintToString(n uint) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// BenchmarkTokenBucket_Allow 基准测试：令牌桶允许检查
func BenchmarkTokenBucket_Allow(b *testing.B) {
	bucket := NewTokenBucket(1000000, 1000000) // 高速桶，不会限流

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			bucket.Allow()
		}
	})
}

// BenchmarkTokenBucket_Allow_WithContention 基准测试：有竞争的令牌桶
func BenchmarkTokenBucket_Allow_WithContention(b *testing.B) {
	bucket := NewTokenBucket(100, 10) // 低容量桶，会有竞争

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			bucket.Allow()
		}
	})
}

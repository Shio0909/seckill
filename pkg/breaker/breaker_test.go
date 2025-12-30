package breaker

import (
	"testing"
	"time"
)

// ========================================================================
// 【重点学习】熔断器单元测试
// ========================================================================
// 测试熔断器的三种状态转换：
// 1. Closed -> Open（错误率超过阈值）
// 2. Open -> Half-Open（超时后尝试恢复）
// 3. Half-Open -> Closed（恢复成功）
// 4. Half-Open -> Open（恢复失败）
//
// 📝 简历亮点：
// - 状态机测试
// - 并发安全测试
// ========================================================================

// TestCircuitBreakerStateTransitions 测试状态转换
func TestCircuitBreakerStateTransitions(t *testing.T) {
	// 创建一个测试用的熔断器配置
	cb := NewCircuitBreaker(Settings{
		Name:          "test-service",
		MaxRequests:   3,
		Interval:      time.Second,
		Timeout:       time.Millisecond * 100, // 短超时便于测试
		FailThreshold: 3,
	})

	// 初始状态应该是 Closed
	if cb.State() != StateClosed {
		t.Errorf("Expected initial state to be Closed, got %s", cb.State())
	}

	// 测试 1: Closed -> Open
	// 连续失败达到阈值
	for i := 0; i < 3; i++ {
		err := cb.Allow()
		if err != nil {
			t.Error("Request should be allowed in Closed state")
		}
		cb.Failure()
	}

	// 应该转换到 Open 状态
	if cb.State() != StateOpen {
		t.Errorf("Expected state to be Open after failures, got %s", cb.State())
	}

	// Open 状态下请求应该被拒绝
	err := cb.Allow()
	if err == nil {
		t.Error("Request should be rejected in Open state")
	}

	// 测试 2: Open -> Half-Open
	// 等待超时时间
	time.Sleep(time.Millisecond * 150)

	// 应该转换到 Half-Open 状态
	if cb.State() != StateHalfOpen {
		t.Errorf("Expected state to be Half-Open after timeout, got %s", cb.State())
	}

	// 测试 3: Half-Open -> Closed
	// 连续成功
	for i := 0; i < 3; i++ {
		err := cb.Allow()
		if err != nil {
			t.Error("Request should be allowed in Half-Open state")
		}
		cb.Success()
	}

	// 应该恢复到 Closed 状态
	if cb.State() != StateClosed {
		t.Errorf("Expected state to be Closed after recovery, got %s", cb.State())
	}
}

// TestCircuitBreakerHalfOpenToOpen 测试 Half-Open 失败回退
func TestCircuitBreakerHalfOpenToOpen(t *testing.T) {
	cb := NewCircuitBreaker(Settings{
		Name:          "test-service-2",
		Timeout:       time.Millisecond * 50,
		FailThreshold: 2,
		MaxRequests:   2,
	})

	// 触发熔断
	for i := 0; i < 2; i++ {
		if err := cb.Allow(); err != nil {
			// 在测试中忽略 Allow 错误，继续触发 Failure
		}
		cb.Failure()
	}

	if cb.State() != StateOpen {
		t.Errorf("Expected state to be Open, got %s", cb.State())
	}

	// 等待进入 Half-Open
	time.Sleep(time.Millisecond * 60)

	// 在 Half-Open 状态下失败
	if err := cb.Allow(); err != nil {
		// 在测试中忽略 Allow 错误
	}
	cb.Failure()

	// 应该回退到 Open 状态
	if cb.State() != StateOpen {
		t.Errorf("Expected state to be Open after failure in Half-Open, got %s", cb.State())
	}
}

// TestBreakerManager 测试熔断器管理器
func TestBreakerManager(t *testing.T) {
	manager := NewBreakerManager(DefaultSettings())

	// 获取同一服务的熔断器应该返回同一个实例
	cb1 := manager.GetBreaker("service-a")
	cb2 := manager.GetBreaker("service-a")

	if cb1 != cb2 {
		t.Error("Expected same breaker instance for same service name")
	}

	// 不同服务应该返回不同的熔断器
	cb3 := manager.GetBreaker("service-b")
	if cb1 == cb3 {
		t.Error("Expected different breaker instances for different services")
	}
}

// TestCircuitBreakerAllowConcurrent 测试并发访问
func TestCircuitBreakerAllowConcurrent(t *testing.T) {
	cb := NewCircuitBreaker(Settings{
		Name:          "concurrent-test",
		FailThreshold: 100,
		MaxRequests:   2,
	})

	// 并发请求
	done := make(chan bool, 100)
	for i := 0; i < 100; i++ {
		go func() {
			err := cb.Allow()
			if err == nil {
				cb.Success()
			}
			done <- true
		}()
	}

	// 等待所有 goroutine 完成
	for i := 0; i < 100; i++ {
		<-done
	}

	// 状态应该仍然是 Closed
	if cb.State() != StateClosed {
		t.Errorf("Expected state to be Closed, got %s", cb.State())
	}
}

// TestCircuitBreakerExecute 测试 Execute 方法
func TestCircuitBreakerExecute(t *testing.T) {
	cb := NewCircuitBreaker(Settings{
		Name:          "execute-test",
		FailThreshold: 2,
		Timeout:       time.Millisecond * 50,
	})

	// 测试成功执行
	result, err := cb.Execute(func() (interface{}, error) {
		return "success", nil
	})
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if result != "success" {
		t.Errorf("Expected 'success', got %v", result)
	}

	// 触发熔断
	for i := 0; i < 2; i++ {
		if _, err := cb.Execute(func() (interface{}, error) {
			return nil, ErrCircuitOpen // 返回任意错误
		}); err != nil {
			// 在测试中忽略执行错误
		}
	}

	// 熔断状态下应该直接返回错误
	_, err = cb.Execute(func() (interface{}, error) {
		return "should not reach here", nil
	})
	if err != ErrCircuitOpen {
		t.Errorf("Expected ErrCircuitOpen, got %v", err)
	}
}

// TestDefaultSettings 测试默认配置
func TestDefaultSettings(t *testing.T) {
	settings := DefaultSettings()

	if settings.MaxRequests != 3 {
		t.Errorf("Expected MaxRequests to be 3, got %d", settings.MaxRequests)
	}
	if settings.Interval != 30*time.Second {
		t.Errorf("Expected Interval to be 30s, got %v", settings.Interval)
	}
	if settings.Timeout != 10*time.Second {
		t.Errorf("Expected Timeout to be 10s, got %v", settings.Timeout)
	}
	if settings.FailThreshold != 5 {
		t.Errorf("Expected FailThreshold to be 5, got %d", settings.FailThreshold)
	}
}

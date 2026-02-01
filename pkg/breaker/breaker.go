package breaker

import (
	"errors"
	"sync"
	"time"
)

// 熔断器是微服务中防止级联故障的重要机制，灵感来自电路断路器。
//
// 三种状态：
// 1. Closed（关闭）- 正常状态，请求正常通过
// 2. Open（打开）- 熔断状态，请求直接失败，不调用下游
// 3. Half-Open（半开）- 尝试恢复，允许部分请求通过
//
// 状态转换：
// Closed --失败率超阈值--> Open --超时后--> Half-Open
// Half-Open --请求成功--> Closed
// Half-Open --请求失败--> Open

// State 熔断器状态
type State int

const (
	StateClosed   State = iota // 关闭状态（正常）
	StateOpen                  // 打开状态（熔断中）
	StateHalfOpen              // 半开状态（尝试恢复）
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "Closed"
	case StateOpen:
		return "Open"
	case StateHalfOpen:
		return "Half-Open"
	default:
		return "Unknown"
	}
}

// 预定义错误
var (
	ErrCircuitOpen    = errors.New("circuit breaker is open")
	ErrTooManyRequest = errors.New("too many requests in half-open state")
)

// Settings 熔断器配置
type Settings struct {
	Name          string        // 熔断器名称
	MaxRequests   uint32        // 半开状态允许的最大请求数
	Interval      time.Duration // Closed 状态下统计周期（超过后重置计数）
	Timeout       time.Duration // Open 状态超时时间（超时后转为 Half-Open）
	FailThreshold uint32        // 失败阈值（连续失败多少次触发熔断）
	// 自定义判断函数
	ReadyToTrip   func(counts Counts) bool          // 判断是否应该熔断
	OnStateChange func(name string, from, to State) // 状态变化回调
}

// Counts 统计计数
type Counts struct {
	Requests             uint32 // 总请求数
	TotalSuccesses       uint32 // 总成功数
	TotalFailures        uint32 // 总失败数
	ConsecutiveSuccesses uint32 // 连续成功数
	ConsecutiveFailures  uint32 // 连续失败数
}

func (c *Counts) onRequest() {
	c.Requests++
}

func (c *Counts) onSuccess() {
	c.TotalSuccesses++
	c.ConsecutiveSuccesses++
	c.ConsecutiveFailures = 0
}

func (c *Counts) onFailure() {
	c.TotalFailures++
	c.ConsecutiveFailures++
	c.ConsecutiveSuccesses = 0
}

func (c *Counts) clear() {
	c.Requests = 0
	c.TotalSuccesses = 0
	c.TotalFailures = 0
	c.ConsecutiveSuccesses = 0
	c.ConsecutiveFailures = 0
}

// 关键设计：
// 1. 使用互斥锁保证并发安全
// 2. 状态机模式管理状态转换
// 3. 使用时间戳判断超时

// CircuitBreaker 熔断器
type CircuitBreaker struct {
	name          string
	maxRequests   uint32
	interval      time.Duration
	timeout       time.Duration
	failThreshold uint32
	readyToTrip   func(counts Counts) bool
	onStateChange func(name string, from, to State)

	mu         sync.Mutex
	state      State
	generation uint64 // 代数，用于防止过期回调
	counts     Counts
	expiry     time.Time // Closed 状态下统计周期结束时间，或 Open 状态超时时间
}

// NewCircuitBreaker 创建熔断器
func NewCircuitBreaker(settings Settings) *CircuitBreaker {
	cb := &CircuitBreaker{
		name:          settings.Name,
		maxRequests:   settings.MaxRequests,
		interval:      settings.Interval,
		timeout:       settings.Timeout,
		failThreshold: settings.FailThreshold,
		readyToTrip:   settings.ReadyToTrip,
		onStateChange: settings.OnStateChange,
	}

	// 设置默认值
	if cb.maxRequests == 0 {
		cb.maxRequests = 1
	}
	if cb.interval == 0 {
		cb.interval = 60 * time.Second
	}
	if cb.timeout == 0 {
		cb.timeout = 30 * time.Second
	}
	if cb.failThreshold == 0 {
		cb.failThreshold = 5
	}
	if cb.readyToTrip == nil {
		// 默认：连续失败超过阈值时熔断
		cb.readyToTrip = func(counts Counts) bool {
			return counts.ConsecutiveFailures >= cb.failThreshold
		}
	}

	cb.toNewGeneration(time.Now())

	return cb
}

// State 获取当前状态
func (cb *CircuitBreaker) State() State {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()
	state, _ := cb.currentState(now)
	return state
}

// Counts 获取当前计数
func (cb *CircuitBreaker) Counts() Counts {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.counts
}

// 执行流程：
// 1. beforeRequest：检查是否允许请求通过
//    - Closed：直接通过
//    - Open：检查是否超时，超时则转为 Half-Open
//    - Half-Open：检查是否超过最大请求数
// 2. 执行实际请求
// 3. afterRequest：根据结果更新状态
//    - 成功：增加成功计数，Half-Open 状态可能恢复
//    - 失败：增加失败计数，可能触发熔断

// Execute 执行请求（带熔断保护）
func (cb *CircuitBreaker) Execute(req func() (interface{}, error)) (interface{}, error) {
	// 【关键步骤1】请求前检查
	generation, err := cb.beforeRequest()
	if err != nil {
		return nil, err
	}

	// 【关键步骤2】执行实际请求
	result, err := req()

	// 【关键步骤3】请求后处理
	cb.afterRequest(generation, err == nil)

	return result, err
}

// Allow 检查是否允许请求（不执行，只检查）
func (cb *CircuitBreaker) Allow() error {
	_, err := cb.beforeRequest()
	return err
}

// Success 手动标记成功
func (cb *CircuitBreaker) Success() {
	cb.afterRequest(cb.generation, true)
}

// Failure 手动标记失败
func (cb *CircuitBreaker) Failure() {
	cb.afterRequest(cb.generation, false)
}

func (cb *CircuitBreaker) beforeRequest() (uint64, error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()
	state, generation := cb.currentState(now)

	switch state {
	case StateOpen:
		return generation, ErrCircuitOpen
	case StateHalfOpen:
		if cb.counts.Requests >= cb.maxRequests {
			return generation, ErrTooManyRequest
		}
	}

	cb.counts.onRequest()
	return generation, nil
}

func (cb *CircuitBreaker) afterRequest(generation uint64, success bool) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	// 检查代数，防止过期的回调影响新状态
	if cb.generation != generation {
		return
	}

	if success {
		cb.onSuccess(time.Now())
	} else {
		cb.onFailure(time.Now())
	}
}

func (cb *CircuitBreaker) onSuccess(now time.Time) {
	cb.counts.onSuccess()

	switch cb.state {
	case StateHalfOpen:
		// 半开状态下成功次数达到阈值，恢复为关闭状态
		if cb.counts.ConsecutiveSuccesses >= cb.maxRequests {
			cb.setState(StateClosed, now)
		}
	}
}

func (cb *CircuitBreaker) onFailure(now time.Time) {
	cb.counts.onFailure()

	switch cb.state {
	case StateClosed:
		// 关闭状态下检查是否需要熔断
		if cb.readyToTrip(cb.counts) {
			cb.setState(StateOpen, now)
		}
	case StateHalfOpen:
		// 半开状态下失败，重新熔断
		cb.setState(StateOpen, now)
	}
}

func (cb *CircuitBreaker) currentState(now time.Time) (State, uint64) {
	switch cb.state {
	case StateClosed:
		// 检查统计周期是否过期
		if !cb.expiry.IsZero() && cb.expiry.Before(now) {
			cb.toNewGeneration(now)
		}
	case StateOpen:
		// 检查熔断是否超时
		if cb.expiry.Before(now) {
			cb.setState(StateHalfOpen, now)
		}
	}

	return cb.state, cb.generation
}

func (cb *CircuitBreaker) setState(state State, now time.Time) {
	if cb.state == state {
		return
	}

	prev := cb.state
	cb.state = state

	cb.toNewGeneration(now)

	if cb.onStateChange != nil {
		cb.onStateChange(cb.name, prev, state)
	}
}

func (cb *CircuitBreaker) toNewGeneration(now time.Time) {
	cb.generation++
	cb.counts.clear()

	var expiry time.Time
	switch cb.state {
	case StateClosed:
		if cb.interval > 0 {
			expiry = now.Add(cb.interval)
		}
	case StateOpen:
		expiry = now.Add(cb.timeout)
	}
	cb.expiry = expiry
}

// 在微服务架构中，通常需要为每个下游服务创建独立的熔断器
// BreakerManager 提供统一的管理和获取接口

// BreakerManager 熔断器管理器
type BreakerManager struct {
	breakers sync.Map
	settings Settings
}

// NewBreakerManager 创建熔断器管理器
func NewBreakerManager(defaultSettings Settings) *BreakerManager {
	return &BreakerManager{
		settings: defaultSettings,
	}
}

// GetBreaker 获取或创建熔断器
func (m *BreakerManager) GetBreaker(name string) *CircuitBreaker {
	if cb, ok := m.breakers.Load(name); ok {
		return cb.(*CircuitBreaker)
	}

	settings := m.settings
	settings.Name = name
	cb := NewCircuitBreaker(settings)

	actual, _ := m.breakers.LoadOrStore(name, cb)
	return actual.(*CircuitBreaker)
}

// DefaultSettings 返回推荐的默认配置
func DefaultSettings() Settings {
	return Settings{
		MaxRequests:   3,                // 半开状态允许 3 个请求
		Interval:      30 * time.Second, // 30 秒统计周期
		Timeout:       10 * time.Second, // 熔断 10 秒后尝试恢复
		FailThreshold: 5,                // 连续 5 次失败触发熔断
		OnStateChange: func(name string, from, to State) {
			// 可以在这里添加日志或告警
		},
	}
}

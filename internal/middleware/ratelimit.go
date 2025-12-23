package middleware

// ========================================================================
// 【重点学习】令牌桶限流算法
// ========================================================================
// 为什么需要限流？
// 1. 保护服务不被突发流量打垮
// 2. 秒杀场景必备：防止恶意刷接口
// 3. 公平性：给每个用户相对公平的抢购机会
//
// 常见限流算法对比：
// ┌─────────────┬───────────────────────────────────────────────────┐
// │ 算法        │ 特点                                              │
// ├─────────────┼───────────────────────────────────────────────────┤
// │ 固定窗口    │ 简单，但有临界问题（窗口边界可能双倍流量）         │
// │ 滑动窗口    │ 解决临界问题，但实现复杂                          │
// │ 漏桶        │ 流量平滑，但无法应对突发                          │
// │ 令牌桶      │ 允许一定突发，最常用 ✅                           │
// └─────────────┴───────────────────────────────────────────────────┘
//
// 令牌桶原理：
// 1. 桶中放入令牌，以固定速率生成（如每秒100个）
// 2. 请求到来时，尝试从桶中获取令牌
// 3. 获取到令牌则放行，否则拒绝
// 4. 桶有最大容量，超出的令牌会丢弃（这允许一定程度的突发）
//
// 面试高频问题：
// Q1: 令牌桶 (Token Bucket) 和漏桶 (Leaky Bucket) 的区别是什么？
// A1: 漏桶强制限制流出的速率，流出速率是恒定的，无法应对突发流量；
//     令牌桶限制的是平均流入速率，但允许一定程度的突发流量（只要桶里有令牌）。
//     秒杀场景通常需要应对突发，所以令牌桶更合适。
//
// Q2: 分布式限流怎么做？
// A2: 单机限流（如本文件实现）只能限制单个实例的流量。分布式限流通常使用 Redis + Lua 脚本实现。
//     原理是所有服务实例共享 Redis 中的同一个 Key（计数器或令牌桶），通过 Lua 脚本保证原子性操作。
//     常用库：go-redis/redis_rate。
//
// Q3: 限流阈值怎么设置？
// A3: 需要进行全链路压测。先压测出单机的最大承载能力 (QPS)，然后根据机器数量计算总阈值。
//     通常设置为最大承载能力的 70%-80% 作为安全缓冲。
// ========================================================================

import (
	"net/http"
	"sync"
	"time"

	"seckill/pkg/e"
	"seckill/pkg/response"

	"github.com/gin-gonic/gin"
)

// TokenBucket 令牌桶
type TokenBucket struct {
	rate       float64    // 令牌生成速率（每秒生成多少个）
	capacity   float64    // 桶容量（最多存储多少令牌）
	tokens     float64    // 当前令牌数
	lastUpdate time.Time  // 上次更新时间
	mu         sync.Mutex // 互斥锁，保证并发安全
}

// NewTokenBucket 创建令牌桶
// rate: 每秒生成的令牌数
// capacity: 桶的最大容量
func NewTokenBucket(rate, capacity float64) *TokenBucket {
	return &TokenBucket{
		rate:       rate,
		capacity:   capacity,
		tokens:     capacity, // 初始时桶是满的
		lastUpdate: time.Now(),
	}
}

// ========================================================================
// 【重点学习】令牌桶核心算法
// ========================================================================
// 1. 计算距离上次的时间差
// 2. 根据时间差计算应该新增的令牌数
// 3. 更新令牌数（不超过容量）
// 4. 尝试消费一个令牌
// ========================================================================
func (tb *TokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	// 计算从上次到现在应该生成的令牌数
	elapsed := now.Sub(tb.lastUpdate).Seconds()
	tb.tokens += elapsed * tb.rate

	// 令牌数不能超过桶容量
	if tb.tokens > tb.capacity {
		tb.tokens = tb.capacity
	}

	tb.lastUpdate = now

	// 尝试消费一个令牌
	if tb.tokens >= 1 {
		tb.tokens--
		return true
	}

	return false
}

// ========================================================================
// 全局限流器管理
// ========================================================================
// 可以为不同的接口设置不同的限流策略
// 如：秒杀接口限流严格，普通查询接口限流宽松
// ========================================================================

var (
	limiters  = make(map[string]*TokenBucket)
	limiterMu sync.RWMutex
)

// GetLimiter 获取或创建限流器
func GetLimiter(key string, rate, capacity float64) *TokenBucket {
	limiterMu.RLock()
	if limiter, ok := limiters[key]; ok {
		limiterMu.RUnlock()
		return limiter
	}
	limiterMu.RUnlock()

	limiterMu.Lock()
	defer limiterMu.Unlock()

	// 双重检查
	if limiter, ok := limiters[key]; ok {
		return limiter
	}

	limiter := NewTokenBucket(rate, capacity)
	limiters[key] = limiter
	return limiter
}

// ========================================================================
// 【重点学习】限流中间件
// ========================================================================
// 可以按不同维度限流：
// 1. 全局限流：所有请求共享一个桶
// 2. IP 限流：每个 IP 一个桶
// 3. 用户限流：每个用户一个桶
// 4. 接口限流：每个接口一个桶
// ========================================================================

// RateLimiter 全局限流中间件
// rate: 每秒允许的请求数
// capacity: 桶容量（允许的突发量）
func RateLimiter(rate, capacity float64) gin.HandlerFunc {
	limiter := NewTokenBucket(rate, capacity)

	return func(c *gin.Context) {
		if !limiter.Allow() {
			response.FailWithMsg(c, e.ERROR, "系统繁忙，请稍后再试")
			c.Abort()
			return
		}
		c.Next()
	}
}

// IPRateLimiter 基于 IP 的限流中间件
// 每个 IP 地址独立限流
func IPRateLimiter(rate, capacity float64) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 获取客户端 IP 作为限流 key
		clientIP := c.ClientIP()
		limiter := GetLimiter("ip:"+clientIP, rate, capacity)

		if !limiter.Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"code":    http.StatusTooManyRequests,
				"message": "请求过于频繁，请稍后再试",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

// PathRateLimiter 基于接口路径的限流中间件
// 不同接口可以有不同的限流阈值
func PathRateLimiter(rate, capacity float64) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 使用请求路径作为限流 key
		path := c.Request.URL.Path
		limiter := GetLimiter("path:"+path, rate, capacity)

		if !limiter.Allow() {
			response.FailWithMsg(c, e.ERROR, "接口访问过于频繁")
			c.Abort()
			return
		}
		c.Next()
	}
}

// SeckillRateLimiter 秒杀专用限流（更严格）
// 结合 IP + 用户 ID 双重限流
func SeckillRateLimiter() gin.HandlerFunc {
	return func(c *gin.Context) {
		clientIP := c.ClientIP()

		// IP 限流：每个 IP 每秒最多 10 次请求
		ipLimiter := GetLimiter("seckill:ip:"+clientIP, 10, 20)
		if !ipLimiter.Allow() {
			response.FailWithMsg(c, e.ERROR, "请求过于频繁，请稍后再试")
			c.Abort()
			return
		}

		// 如果用户已登录，增加用户维度限流
		if uid, exists := c.Get("uid"); exists {
			userLimiter := GetLimiter("seckill:user:"+string(rune(uid.(int))), 5, 10)
			if !userLimiter.Allow() {
				response.FailWithMsg(c, e.ERROR, "操作过于频繁，请稍后再试")
				c.Abort()
				return
			}
		}

		c.Next()
	}
}

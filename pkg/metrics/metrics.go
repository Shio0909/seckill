// Package metrics 提供 Prometheus 监控指标
//
// 【重点学习】Prometheus 监控的核心概念
// 1. Counter（计数器）：只增不减，用于累计值如请求总数
// 2. Gauge（仪表盘）：可增可减，用于瞬时值如当前连接数
// 3. Histogram（直方图）：数据分布，用于请求延迟统计
// 4. Summary（摘要）：类似直方图，客户端计算分位数
//
// 【面试高频】
// Q1: Prometheus 的四种指标类型分别用于什么场景？
// A1: Counter - 累计值（请求数、错误数）
//
//	Gauge - 瞬时值（CPU、内存、连接数）
//	Histogram - 分布统计（延迟 P50/P90/P99）
//	Summary - 类似 Histogram，在客户端计算分位数
//
// Q2: Histogram vs Summary 的区别？
// A2: Histogram 在服务端聚合，支持多实例聚合，分位数精度固定
//
//	Summary 在客户端计算，不能跨实例聚合，分位数更精确
//	推荐：分布式系统用 Histogram
//
// Q3: Prometheus 指标命名规范？
// A3: 应用名_模块名_指标名_单位，如 seckill_http_request_duration_seconds
//
// Q4: 高基数问题（High Cardinality）是什么？如何避免？
// A4: 标签值太多导致时间序列爆炸。避免方法：
//   - 不要用用户 ID、请求 ID 作为标签
//   - 对路径参数化处理（/user/123 -> /user/:id）
//
// 面试高频问题（补充）：
// Q: Prometheus 是 Pull 模式还是 Push 模式？
// A: 默认是 Pull 模式（Server 主动去 Client 拉取）。
//
//	优点：Server 端控制采集频率，Client 端实现简单。
//	缺点：Client 必须有可访问的 IP/Port。
//	对于短作业（Short-lived Jobs），可以使用 PushGateway 配合 Push 模式。
//
// Q: 什么是 P99 延迟？
// A: 表示 99% 的请求延迟都低于该值。
//
//	比平均值更能反映长尾效应（Long Tail）和用户体验。
//	例如：平均延迟 100ms，但 P99 是 2s，说明有 1% 的用户体验很差。
package metrics

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// 【重点学习】全局指标变量
// 使用 promauto 自动注册指标，简化代码
var (
	// HTTP 请求总数（Counter）
	// 【重点学习】标签设计
	// - method: HTTP 方法
	// - path: 请求路径（需参数化）
	// - status: 状态码
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "seckill",
			Subsystem: "http",
			Name:      "requests_total",
			Help:      "HTTP 请求总数",
		},
		[]string{"method", "path", "status"},
	)

	// HTTP 请求延迟（Histogram）
	// 【重点学习】Bucket 设计
	// 根据业务 SLA 设置合理的分桶
	// 秒杀场景延迟敏感，需要更细的低延迟分桶
	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "seckill",
			Subsystem: "http",
			Name:      "request_duration_seconds",
			Help:      "HTTP 请求延迟分布",
			// 秒杀场景的延迟分桶：5ms, 10ms, 25ms, 50ms, 100ms, 250ms, 500ms, 1s, 2.5s, 5s
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		},
		[]string{"method", "path"},
	)

	// 当前进行中的请求数（Gauge）
	httpRequestsInFlight = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "seckill",
			Subsystem: "http",
			Name:      "requests_in_flight",
			Help:      "当前进行中的 HTTP 请求数",
		},
	)

	// HTTP 请求体大小（Histogram）
	httpRequestSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "seckill",
			Subsystem: "http",
			Name:      "request_size_bytes",
			Help:      "HTTP 请求体大小分布",
			Buckets:   prometheus.ExponentialBuckets(100, 10, 7), // 100B, 1KB, 10KB, 100KB, 1MB, 10MB, 100MB
		},
		[]string{"method", "path"},
	)

	// HTTP 响应体大小（Histogram）
	httpResponseSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "seckill",
			Subsystem: "http",
			Name:      "response_size_bytes",
			Help:      "HTTP 响应体大小分布",
			Buckets:   prometheus.ExponentialBuckets(100, 10, 7),
		},
		[]string{"method", "path"},
	)
)

// ========================================
// 秒杀业务指标
// ========================================

var (
	// 秒杀请求总数
	seckillRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "seckill",
			Subsystem: "business",
			Name:      "seckill_requests_total",
			Help:      "秒杀请求总数",
		},
		[]string{"product_id", "result"}, // result: success, sold_out, rate_limited, error
	)

	// 秒杀成功订单数
	seckillOrdersTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: "seckill",
			Subsystem: "business",
			Name:      "seckill_orders_total",
			Help:      "秒杀成功创建的订单总数",
		},
	)

	// 商品库存（Gauge）
	// 【重点学习】库存是典型的 Gauge 场景
	productStock = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "seckill",
			Subsystem: "business",
			Name:      "product_stock",
			Help:      "商品当前库存",
		},
		[]string{"product_id"},
	)

	// 秒杀延迟
	seckillDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: "seckill",
			Subsystem: "business",
			Name:      "seckill_duration_seconds",
			Help:      "秒杀请求处理延迟",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5},
		},
	)

	// 订单状态分布
	orderStatusGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "seckill",
			Subsystem: "business",
			Name:      "orders_by_status",
			Help:      "各状态订单数量",
		},
		[]string{"status"}, // pending, paid, cancelled, timeout
	)
)

// ========================================
// 中间件组件指标
// ========================================

var (
	// Redis 操作指标
	redisOperationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "seckill",
			Subsystem: "redis",
			Name:      "operations_total",
			Help:      "Redis 操作总数",
		},
		[]string{"operation", "result"}, // operation: get, set, del, lua; result: success, error
	)

	redisOperationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "seckill",
			Subsystem: "redis",
			Name:      "operation_duration_seconds",
			Help:      "Redis 操作延迟",
			Buckets:   []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1},
		},
		[]string{"operation"},
	)

	// RabbitMQ 消息指标
	mqMessagesPublished = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "seckill",
			Subsystem: "rabbitmq",
			Name:      "messages_published_total",
			Help:      "发布的消息总数",
		},
		[]string{"queue"},
	)

	mqMessagesConsumed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "seckill",
			Subsystem: "rabbitmq",
			Name:      "messages_consumed_total",
			Help:      "消费的消息总数",
		},
		[]string{"queue", "result"}, // result: success, error
	)

	// 熔断器状态
	// 【重点学习】熔断器状态监控非常重要
	circuitBreakerState = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "seckill",
			Subsystem: "circuit_breaker",
			Name:      "state",
			Help:      "熔断器状态 (0=closed, 1=half-open, 2=open)",
		},
		[]string{"name"},
	)

	circuitBreakerRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "seckill",
			Subsystem: "circuit_breaker",
			Name:      "requests_total",
			Help:      "熔断器请求总数",
		},
		[]string{"name", "result"}, // result: success, failure, rejected
	)

	// 分布式锁指标
	distlockAcquired = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "seckill",
			Subsystem: "distlock",
			Name:      "acquired_total",
			Help:      "分布式锁获取总数",
		},
		[]string{"key", "result"}, // result: success, failed
	)

	distlockHoldDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "seckill",
			Subsystem: "distlock",
			Name:      "hold_duration_seconds",
			Help:      "分布式锁持有时间",
			Buckets:   []float64{0.01, 0.05, 0.1, 0.5, 1, 5, 10},
		},
		[]string{"key"},
	)
)

// ========================================
// Gin 中间件
// ========================================

// PrometheusMiddleware Gin Prometheus 监控中间件
// 【重点学习】中间件采集请求指标
func PrometheusMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 跳过 /metrics 端点本身
		if c.Request.URL.Path == "/metrics" {
			c.Next()
			return
		}

		start := time.Now()

		// 增加进行中请求计数
		httpRequestsInFlight.Inc()

		// 记录请求体大小
		requestSize := float64(c.Request.ContentLength)
		if requestSize < 0 {
			requestSize = 0
		}

		// 处理请求
		c.Next()

		// 减少进行中请求计数
		httpRequestsInFlight.Dec()

		// 计算延迟
		duration := time.Since(start).Seconds()

		// 【重点学习】路径参数化处理
		// 避免高基数问题：/user/123 -> /user/:id
		path := c.FullPath()
		if path == "" {
			path = "unknown"
		}

		method := c.Request.Method
		status := strconv.Itoa(c.Writer.Status())

		// 记录指标
		httpRequestsTotal.WithLabelValues(method, path, status).Inc()
		httpRequestDuration.WithLabelValues(method, path).Observe(duration)
		httpRequestSize.WithLabelValues(method, path).Observe(requestSize)
		httpResponseSize.WithLabelValues(method, path).Observe(float64(c.Writer.Size()))
	}
}

// MetricsHandler 返回 Prometheus metrics handler
func MetricsHandler() gin.HandlerFunc {
	handler := promhttp.Handler()
	return func(c *gin.Context) {
		handler.ServeHTTP(c.Writer, c.Request)
	}
}

// ========================================
// 业务指标记录函数
// ========================================

// RecordSeckillRequest 记录秒杀请求
// 【重点学习】业务代码中调用记录指标
func RecordSeckillRequest(productID string, result string, duration time.Duration) {
	seckillRequestsTotal.WithLabelValues(productID, result).Inc()
	seckillDuration.Observe(duration.Seconds())
}

// RecordSeckillOrder 记录秒杀订单
func RecordSeckillOrder() {
	seckillOrdersTotal.Inc()
}

// SetProductStock 设置商品库存
func SetProductStock(productID string, stock int64) {
	productStock.WithLabelValues(productID).Set(float64(stock))
}

// SetOrderStatusCount 设置订单状态计数
func SetOrderStatusCount(status string, count int64) {
	orderStatusGauge.WithLabelValues(status).Set(float64(count))
}

// RecordRedisOperation 记录 Redis 操作
func RecordRedisOperation(operation string, result string, duration time.Duration) {
	redisOperationsTotal.WithLabelValues(operation, result).Inc()
	redisOperationDuration.WithLabelValues(operation).Observe(duration.Seconds())
}

// RecordMQPublish 记录 MQ 发布
func RecordMQPublish(queue string) {
	mqMessagesPublished.WithLabelValues(queue).Inc()
}

// RecordMQConsume 记录 MQ 消费
func RecordMQConsume(queue string, result string) {
	mqMessagesConsumed.WithLabelValues(queue, result).Inc()
}

// SetCircuitBreakerState 设置熔断器状态
func SetCircuitBreakerState(name string, state int) {
	circuitBreakerState.WithLabelValues(name).Set(float64(state))
}

// RecordCircuitBreakerRequest 记录熔断器请求
func RecordCircuitBreakerRequest(name string, result string) {
	circuitBreakerRequests.WithLabelValues(name, result).Inc()
}

// RecordDistLockAcquire 记录分布式锁获取
func RecordDistLockAcquire(key string, success bool) {
	result := "success"
	if !success {
		result = "failed"
	}
	distlockAcquired.WithLabelValues(key, result).Inc()
}

// RecordDistLockHold 记录分布式锁持有时间
func RecordDistLockHold(key string, duration time.Duration) {
	distlockHoldDuration.WithLabelValues(key).Observe(duration.Seconds())
}

// ========================================
// 系统资源指标（自动收集）
// ========================================

// 【重点学习】Go 运行时指标
// prometheus 客户端库会自动收集以下指标：
// - go_goroutines: goroutine 数量
// - go_gc_duration_seconds: GC 耗时
// - go_memstats_*: 内存统计
// - process_*: 进程资源使用

func init() {
	// 可以在这里注册自定义 Collector
	// prometheus.MustRegister(newCustomCollector())
}

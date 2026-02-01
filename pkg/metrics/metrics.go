// Package metrics 提供 Prometheus 监控指标
//
// 1. Counter（计数器）：只增不减，用于累计值如请求总数
// 2. Gauge（仪表盘）：可增可减，用于瞬时值如当前连接数
// 3. Histogram（直方图）：数据分布，用于请求延迟统计
// 4. Summary（摘要）：类似直方图，客户端计算分位数
package metrics

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// 使用 promauto 自动注册指标，简化代码
var (
	// HTTP 请求总数（Counter）
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

// 秒杀业务指标

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

// 中间件组件指标

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

// Gin 中间件

// PrometheusMiddleware Gin Prometheus 监控中间件
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

// 业务指标记录函数

// RecordSeckillRequest 记录秒杀请求
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

// 系统资源指标（自动收集）

// prometheus 客户端库会自动收集以下指标：
// - go_goroutines: goroutine 数量
// - go_gc_duration_seconds: GC 耗时
// - go_memstats_*: 内存统计
// - process_*: 进程资源使用

func init() {
	// 可以在这里注册自定义 Collector
	// prometheus.MustRegister(newCustomCollector())
}

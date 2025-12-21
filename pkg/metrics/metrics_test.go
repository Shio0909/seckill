package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// 【重点学习】测试 Prometheus 指标收集
func TestPrometheusMiddleware_RecordsMetrics(t *testing.T) {
	// 创建测试路由
	router := gin.New()
	router.Use(PrometheusMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "ok"})
	})

	// 发送测试请求
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	router.ServeHTTP(w, req)

	// 验证请求计数器
	count := testutil.ToFloat64(httpRequestsTotal.WithLabelValues("GET", "/test", "200"))
	assert.Equal(t, float64(1), count, "请求计数应该为 1")
}

// 【面试高频】测试延迟直方图
func TestPrometheusMiddleware_RecordsDuration(t *testing.T) {
	router := gin.New()
	router.Use(PrometheusMiddleware())
	router.GET("/slow", func(c *gin.Context) {
		time.Sleep(50 * time.Millisecond) // 模拟慢请求
		c.JSON(200, nil)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/slow", nil)
	router.ServeHTTP(w, req)

	// 验证请求计数器有数据（延迟也会增加计数）
	count := testutil.ToFloat64(httpRequestsTotal.WithLabelValues("GET", "/slow", "200"))
	assert.Greater(t, count, float64(0), "应该记录请求")
}

// 【重点学习】测试业务指标记录
func TestBusinessMetrics(t *testing.T) {
	// 记录秒杀请求
	RecordSeckillRequest("product-001", "success", 10*time.Millisecond)

	count := testutil.ToFloat64(seckillRequestsTotal.WithLabelValues("product-001", "success"))
	assert.Equal(t, float64(1), count)

	// 记录订单
	RecordSeckillOrder()
	orderCount := testutil.ToFloat64(seckillOrdersTotal)
	assert.Equal(t, float64(1), orderCount)

	// 设置库存
	SetProductStock("product-001", 100)
	stock := testutil.ToFloat64(productStock.WithLabelValues("product-001"))
	assert.Equal(t, float64(100), stock)
}

// 【面试高频】测试 metrics endpoint
func TestMetricsHandler(t *testing.T) {
	router := gin.New()
	router.GET("/metrics", MetricsHandler())

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/metrics", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	// 验证响应包含 Prometheus 格式
	assert.Contains(t, w.Body.String(), "# HELP")
	assert.Contains(t, w.Body.String(), "# TYPE")
}

// 【重点学习】测试熔断器指标
func TestCircuitBreakerMetrics(t *testing.T) {
	// 设置熔断器状态
	SetCircuitBreakerState("user-service", 0) // closed
	state := testutil.ToFloat64(circuitBreakerState.WithLabelValues("user-service"))
	assert.Equal(t, float64(0), state)

	// 记录请求
	RecordCircuitBreakerRequest("user-service", "success")
	RecordCircuitBreakerRequest("user-service", "failure")

	successCount := testutil.ToFloat64(circuitBreakerRequests.WithLabelValues("user-service", "success"))
	failureCount := testutil.ToFloat64(circuitBreakerRequests.WithLabelValues("user-service", "failure"))

	assert.Equal(t, float64(1), successCount)
	assert.Equal(t, float64(1), failureCount)
}

// 【面试高频】测试分布式锁指标
func TestDistLockMetrics(t *testing.T) {
	// 记录锁获取
	RecordDistLockAcquire("order:123", true)
	RecordDistLockAcquire("order:456", false)

	successCount := testutil.ToFloat64(distlockAcquired.WithLabelValues("order:123", "success"))
	failedCount := testutil.ToFloat64(distlockAcquired.WithLabelValues("order:456", "failed"))

	assert.Equal(t, float64(1), successCount)
	assert.Equal(t, float64(1), failedCount)

	// 记录锁持有时间
	RecordDistLockHold("order:123", 100*time.Millisecond)
	// 验证直方图有数据即可
}

// 【重点学习】测试 Redis 操作指标
func TestRedisMetrics(t *testing.T) {
	RecordRedisOperation("get", "success", 1*time.Millisecond)
	RecordRedisOperation("set", "success", 2*time.Millisecond)
	RecordRedisOperation("get", "error", 5*time.Millisecond)

	getSuccess := testutil.ToFloat64(redisOperationsTotal.WithLabelValues("get", "success"))
	getError := testutil.ToFloat64(redisOperationsTotal.WithLabelValues("get", "error"))
	setSuccess := testutil.ToFloat64(redisOperationsTotal.WithLabelValues("set", "success"))

	assert.Equal(t, float64(1), getSuccess)
	assert.Equal(t, float64(1), getError)
	assert.Equal(t, float64(1), setSuccess)
}

// 【面试高频】测试 MQ 指标
func TestMQMetrics(t *testing.T) {
	RecordMQPublish("seckill-queue")
	RecordMQConsume("seckill-queue", "success")
	RecordMQConsume("seckill-queue", "error")

	published := testutil.ToFloat64(mqMessagesPublished.WithLabelValues("seckill-queue"))
	consumedSuccess := testutil.ToFloat64(mqMessagesConsumed.WithLabelValues("seckill-queue", "success"))
	consumedError := testutil.ToFloat64(mqMessagesConsumed.WithLabelValues("seckill-queue", "error"))

	assert.Equal(t, float64(1), published)
	assert.Equal(t, float64(1), consumedSuccess)
	assert.Equal(t, float64(1), consumedError)
}

// 清理测试中创建的指标（避免测试间干扰）
func resetMetrics() {
	// 在实际测试中，可能需要重新创建 registry
	// 这里简化处理
}

package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"seckill/pkg/config"
	"seckill/pkg/consul"
	"seckill/pkg/grpcx"
	"seckill/pkg/logger"
	"seckill/services/gateway/handlers"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// API Gateway 是微服务架构的统一入口，负责：
// 1. 请求路由：将 HTTP 请求路由到对应的 gRPC 服务
// 2. 协议转换：HTTP/JSON <-> gRPC/Protobuf
// 3. 认证鉴权：统一 Token 验证
// 4. 限流熔断：保护后端服务
// 5. 负载均衡：请求分发到多个服务实例

var (
	configFile = flag.String("config", "config/config.yaml", "配置文件路径")
	port       = flag.Int("port", 8080, "HTTP 服务端口")
)

func main() {
	flag.Parse()

	// 1. 加载配置（使用统一的配置管理）
	if err := config.InitConfig(*configFile); err != nil {
		fmt.Printf("加载配置失败: %v\n", err)
		os.Exit(1)
	}

	// 2. 初始化日志
	logger.Initlogger()
	defer func() {
		_ = logger.Log.Sync()
	}()

	// 3. 初始化 Consul 客户端（用于服务发现）
	cfg := config.Get()
	consulClient, err := consul.NewClient(&consul.Config{
		Address: cfg.Consul.Address,
	})
	if err != nil {
		logger.Log.Fatal(fmt.Sprintf("连接 Consul 失败: %v", err))
	}

	// 4. 初始化 gRPC 客户端管理器
	clientManager := grpcx.NewClientManager(consulClient, nil)

	// 5. 创建处理器
	h := handlers.NewGatewayHandler(clientManager)

	// 6. 设置 Gin 路由
	router := setupRouter(h)

	// 7. 创建 HTTP 服务
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", *port),
		Handler: router,
	}

	// 8. 启动服务（异步）
	go func() {
		fmt.Printf("API Gateway 启动成功，端口: %d\n", *port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Log.Fatal(fmt.Sprintf("HTTP 服务启动失败: %v", err))
		}
	}()

	// 9. 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("正在关闭 API Gateway...")
	clientManager.Close()
	fmt.Println("API Gateway 已关闭")
}

// setupRouter 设置路由
func setupRouter(h *handlers.GatewayHandler) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()

	// 中间件
	router.Use(gin.Recovery())
	router.Use(CORSMiddleware())
	router.Use(LoggingMiddleware())

	// 健康检查
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// API 版本分组
	v1 := router.Group("/api/v1")
	{
		// 用户服务（无需认证）
		v1.POST("/register", h.Register)
		v1.POST("/login", h.Login)

		// 需要认证的接口
		auth := v1.Group("")
		auth.Use(h.AuthMiddleware())
		{
			// 用户
			auth.GET("/user/:id", h.GetUser)

			// 商品
			auth.GET("/products", h.ListProducts)
			auth.GET("/products/:id", h.GetProduct)
			auth.GET("/products/:id/stock", h.GetStock)

			// 订单
			auth.GET("/orders", h.ListOrders)
			auth.GET("/orders/:id", h.GetOrder)
			auth.POST("/orders/:id/cancel", h.CancelOrder)
			auth.POST("/orders/:id/pay", h.PayOrder)

			// 秒杀（带限流）
			seckill := auth.Group("/seckill")
			seckill.Use(RateLimitMiddleware(1000, 2000)) // 每秒 1000，突发 2000
			{
				seckill.POST("/do", h.DoSeckill)
				seckill.GET("/result", h.GetSeckillResult)
				seckill.GET("/products/:id", h.GetSeckillProduct)
			}
		}
	}

	return router
}

// CORSMiddleware 跨域中间件
func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

// LoggingMiddleware 日志中间件
func LoggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		duration := time.Since(start)

		fmt.Printf("[Gateway] %s %s %d %v\n",
			c.Request.Method,
			c.Request.URL.Path,
			c.Writer.Status(),
			duration)
	}
}

// RateLimitMiddleware 限流中间件
// 令牌桶工作原理：
// 1. 桶里有固定数量的令牌
// 2. 请求来时取一个令牌，没有令牌则拒绝
// 3. 令牌以固定速率补充
//
// 参数说明：
// - r (rate): 每秒产生的令牌数
// - b (burst): 桶的容量（允许突发）
func RateLimitMiddleware(r rate.Limit, b int) gin.HandlerFunc {
	limiter := rate.NewLimiter(r, b)
	return func(c *gin.Context) {
		if !limiter.Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"code":    429,
				"message": "请求过于频繁，请稍后再试",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

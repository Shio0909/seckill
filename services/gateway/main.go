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

// ========================================================================
// 【重点学习】API Gateway 网关
// ========================================================================
// API Gateway 是微服务架构的统一入口，负责：
// 1. 请求路由：将 HTTP 请求路由到对应的 gRPC 服务
// 2. 协议转换：HTTP/JSON <-> gRPC/Protobuf
// 3. 认证鉴权：统一 Token 验证
// 4. 限流熔断：保护后端服务
// 5. 负载均衡：请求分发到多个服务实例
//
// 📝 简历亮点：
// - API Gateway 设计模式
// - HTTP 到 gRPC 的协议转换
// - 统一认证和限流
// - 服务发现和负载均衡
//
// 🔥 面试高频：
// Q: API Gateway 和 BFF（Backend for Frontend）有什么区别？
// A: API Gateway 是通用网关，处理认证、限流、路由等横切关注点
//    BFF 是针对特定前端的后端，做数据聚合和格式适配
//    实际项目中可以结合使用
//
// Q: Gateway 如何做负载均衡？
// A: 1. 从 Consul 获取服务实例列表
//    2. 使用负载均衡算法（轮询、随机、权重）选择实例
//    3. 建立连接并转发请求
//
// Q: Gateway 单点故障怎么解决？
// A: 1. 部署多个 Gateway 实例
//    2. 前端使用 Nginx 或 LVS 做 4 层负载均衡，将流量分发到多个 Gateway 节点
//
// 面试高频问题（补充）：
// Q: 网关如何处理高并发下的性能瓶颈？
// A: 1. 使用高性能语言（Go/Lua）和框架（Gin/OpenResty）。
//    2. 启用连接池复用后端连接。
//    3. 异步非阻塞 I/O 模型。
//    4. 扩容：网关本身是无状态的，可以轻松水平扩展，配合 Nginx/LVS 做入口负载均衡。
//
// Q: 什么是“惊群效应”？网关如何避免？
// A: 惊群效应指大量等待的进程/线程被同时唤醒，竞争资源。
//    在网关层面，主要关注后端服务重启或扩容时的连接风暴。
//    可以通过指数退避重试、随机化重试间隔等机制缓解。
//    2. 前面加 Nginx/LVS 做负载均衡
//    3. 使用云厂商的 API Gateway 服务
// ========================================================================

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
	defer logger.Log.Sync()

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
// ========================================================================
// 【重点学习】令牌桶限流算法
// ========================================================================
// 令牌桶工作原理：
// 1. 桶里有固定数量的令牌
// 2. 请求来时取一个令牌，没有令牌则拒绝
// 3. 令牌以固定速率补充
//
// 参数说明：
// - r (rate): 每秒产生的令牌数
// - b (burst): 桶的容量（允许突发）
//
// 🔥 面试高频：
// Q: 令牌桶和漏桶有什么区别？
// A: 漏桶：输出速率恒定，适合平滑流量
//
//	令牌桶：允许突发，更适合处理短时峰值
//
// ========================================================================
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

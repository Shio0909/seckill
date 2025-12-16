package router

import (
	"github.com/gin-gonic/gin"

	// 引入 Swagger 相关包
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	// 引入生成的 docs 包
	_ "seckill/docs"

	// 引入业务包
	"seckill/internal/controller"
	"seckill/internal/middleware"
)

// ========================================================================
// 【重点学习】路由设计原则
// ========================================================================
// RESTful API 设计：
// - GET    /resources      -> 列表
// - GET    /resources/:id  -> 详情
// - POST   /resources      -> 创建
// - PUT    /resources/:id  -> 更新
// - DELETE /resources/:id  -> 删除
//
// 中间件加载顺序很重要：
// 1. TraceLogger - 生成 trace_id（最先执行，确保所有日志都有 trace_id）
// 2. Recovery    - 捕获 panic（尽早捕获，防止程序崩溃）
// 3. RateLimiter - 限流（在业务逻辑前拦截，减少无效请求）
// 4. ZapLogger   - 记录请求日志
// 5. Cors        - 处理跨域
// 6. Auth        - 身份认证
// ========================================================================

// NewRouter 负责初始化 Gin 引擎，加载中间件和注册路由
func NewRouter() *gin.Engine {
	// 1. 创建引擎
	r := gin.New()

	// 2. 加载全局中间件（注意顺序！）
	r.Use(middleware.TraceLogger())           // 先生成 trace_id
	r.Use(middleware.Recovery())              // 捕获 panic
	r.Use(middleware.IPRateLimiter(100, 200)) // IP 限流：100 QPS，桶容量 200
	r.Use(middleware.ZapLogger())             // 记录日志
	r.Use(middleware.Cors())                  // 解决跨域

	// 3. 注册 Swagger 文档路由
	// 访问地址: http://localhost:8080/swagger/index.html
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// 4. 实例化 Controller
	userCtrl := &controller.UserController{}
	seckillCtrl := &controller.SeckillController{}
	productCtrl := controller.NewProductController()
	orderCtrl := controller.NewOrderController()

	// 5. 注册业务路由
	api := r.Group("/api/v1")
	{
		// ============================================================
		// 公开接口（无需登录）
		// ============================================================
		api.POST("/register", userCtrl.Register)
		api.POST("/login", userCtrl.Login)

		// 商品列表（公开）
		api.GET("/products", productCtrl.List)
		api.GET("/products/:id", productCtrl.Get)
		api.GET("/products/:id/stock", productCtrl.GetStock)

		// 幂等 Token 获取（公开）
		api.GET("/idempotent/token", middleware.GenerateIdempotentToken)

		// ============================================================
		// 需要登录的接口
		// ============================================================
		authGroup := api.Group("/")
		authGroup.Use(middleware.JWTAuth())
		{
			// ----------------------------------------------------------
			// 秒杀接口（高并发，特殊限流 + 幂等）
			// ----------------------------------------------------------
			seckillGroup := authGroup.Group("/seckill")
			seckillGroup.Use(middleware.SeckillRateLimiter()) // 秒杀专用限流
			{
				// 秒杀下单需要幂等性保证
				seckillGroup.POST("/buy",
					middleware.Idempotent(), // 幂等中间件
					seckillCtrl.Buy,
				)
			}

			// ----------------------------------------------------------
			// 订单接口
			// ----------------------------------------------------------
			orderGroup := authGroup.Group("/orders")
			{
				orderGroup.GET("", orderCtrl.List)
				orderGroup.GET("/:id", orderCtrl.Get)
				orderGroup.GET("/no/:order_no", orderCtrl.GetByOrderNo)
				orderGroup.POST("/:id/cancel", orderCtrl.Cancel)
				orderGroup.POST("/:id/pay", orderCtrl.Pay)
			}

			// ----------------------------------------------------------
			// 用户接口（可扩展）
			// ----------------------------------------------------------
			// userGroup := authGroup.Group("/user")
			// {
			//     userGroup.GET("/profile", userCtrl.Profile)
			//     userGroup.PUT("/profile", userCtrl.UpdateProfile)
			// }
		}

		// ============================================================
		// 管理员接口（需要管理员权限）
		// ============================================================
		adminGroup := api.Group("/admin")
		adminGroup.Use(middleware.JWTAuth())
		// TODO: adminGroup.Use(middleware.AdminRequired()) // 管理员权限检查
		{
			// 商品管理
			adminProducts := adminGroup.Group("/products")
			{
				adminProducts.POST("", productCtrl.Create)
				adminProducts.PUT("/:id", productCtrl.Update)
				adminProducts.DELETE("/:id", productCtrl.Delete)
				adminProducts.PUT("/:id/stock", productCtrl.SetStock)
				adminProducts.POST("/:id/warmup", productCtrl.WarmUp) // 库存预热
			}

			// 订单统计
			adminGroup.GET("/orders/stats", orderCtrl.Stats)
		}
	}

	// 6. 健康检查接口
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "ok",
			"service": "seckill",
		})
	})

	// 7. 返回配置好的引擎
	return r
}

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

// NewRouter 负责初始化 Gin 引擎，加载中间件和注册路由
func NewRouter() *gin.Engine {
	// 1. 创建引擎
	r := gin.New()

	// 2. 加载全局中间件
	r.Use(middleware.ZapLogger()) // 先记日志
	r.Use(gin.Recovery())         // 再防崩溃
	r.Use(middleware.Cors())      // 解决跨域

	// 3. 注册 Swagger 文档路由
	// 访问地址: http://localhost:8080/swagger/index.html
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// 4. 实例化 Controller
	seckillCtrl := &controller.SeckillController{}

	// 5. 注册业务路由
	api := r.Group("/api")
	{
		// 用户模块 (测试用)
		userGroup := api.Group("/user")
		{
			userGroup.GET("/info", func(c *gin.Context) {
				c.JSON(200, gin.H{"msg": "user info"})
			})
		}

		// 秒杀模块
		seckillGroup := api.Group("/seckill")
		{
			// POST /api/seckill/buy
			seckillGroup.POST("/buy", seckillCtrl.Buy)
		}
	}

	// 6. 返回配置好的引擎
	return r
}

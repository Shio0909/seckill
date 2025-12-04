package main

//只负责启动，不负责具体配置细节
import (
	"log"

	"seckill/internal/model"
	"seckill/internal/router"
	"seckill/internal/service"
	"seckill/pkg/config"
	"seckill/pkg/database"
	"seckill/pkg/logger"
	"seckill/pkg/rabbitmq"
	"seckill/pkg/redis"
	"seckill/pkg/snowflake"

	"go.uber.org/zap"
)

// @title Go秒杀系统 API
// @version 1.0
// @description 基于 Gin + Redis + RabbitMQ 的高并发秒杀系统
// @host localhost:8080
// @BasePath /
// 定义安全模式
// @securityDefinitions.apikey Bearer
// @in header
// @name Authorization
func main() {
	// 0、加载配置文件（最先执行）
	if err := config.InitConfig("config/config.yaml"); err != nil {
		log.Fatalf("配置加载失败: %v", err)
	}

	// 1、初始化各组件
	logger.Initlogger()
	defer logger.Sync()     // 确保程序退出前最后一条日志被写入
	database.InitMySQL()    // 连接 MySQL
	redis.InitRedis()       // 连接 Redis
	redis.InitLuaScripts()  // 初始化 Lua 脚本
	snowflake.Init(1)       // 雪花算法初始化，机器ID=1
	rabbitmq.InitRabbitMQ() // RabbitMQ初始化
	service.StartConsumer()

	// 2、表结构设置
	err := database.DB.AutoMigrate(&model.User{}, &model.Product{}, &model.Order{}) // 自动建表
	if err != nil {
		logger.Log.Fatal("建表失败", zap.Error(err))
	}
	logger.Log.Info("数据库表结构同步成功")

	// 3、初始化测试商品数据
	service.InitProductData()

	// 4、启动web服务
	r := router.NewRouter()
	cfg := config.Get()
	logger.Log.Info("程序启动成功",
		zap.String("service", cfg.Server.Name),
		zap.String("mode", cfg.Server.Mode),
		zap.Int("port", cfg.Server.Port),
	)
	r.Run(config.GetServerAddr())
}

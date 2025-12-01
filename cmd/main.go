package main

//只负责启动，不负责具体配置细节
import (
	"seckill/internal/model"
	"seckill/internal/router"
	"seckill/internal/service"
	"seckill/pkg/database"
	"seckill/pkg/logger"
	"seckill/pkg/redis"

	"go.uber.org/zap"
)

// @title Go秒杀系统 API
// @version 1.0
// @description 基于 Gin + Redis + RabbitMQ 的高并发秒杀系统
// @host localhost:8080
// @BasePath /
func main() {
	//1、初始化
	logger.Initlogger()
	defer logger.Sync()    //确保程序退出前最后一条日志被写入
	database.InitMySQL()   // 连接 MySQL
	redis.InitRedis()      // 连接 Redis
	redis.InitLuaScripts() // 初始化 Lua 脚本
	//2、表结构设置
	err := database.DB.AutoMigrate(&model.User{}, &model.Product{}, &model.Order{}) // 自动建表
	if err != nil {
		logger.Log.Fatal("建表失败", zap.Error(err))
	}
	logger.Log.Info("数据库表结构同步成功")
	//3、初始化测试商品数据
	service.InitProductData()
	//4、启动web服务
	r := router.NewRouter()
	logger.Log.Info("程序启动成功",
		zap.String("env", "dev"),
		zap.Int("port", 8080),
	)
	r.Run(":8080")
}

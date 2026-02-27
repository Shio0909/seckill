package main

// 只负责启动，不负责具体配置细节
import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"seckill/internal/model"
	"seckill/internal/router"
	"seckill/internal/service"
	"seckill/pkg/config"
	"seckill/pkg/database"
	"seckill/pkg/logger"
	"seckill/pkg/rabbitmq"
	"seckill/pkg/redis"
	"seckill/pkg/snowflake"

	"github.com/gin-gonic/gin"
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

	// 4、启动web服务（支持优雅停机）
	cfg := config.Get()
	gin.SetMode(cfg.Server.Mode)
	r := router.NewRouter()

	srv := &http.Server{
		Addr:         config.GetServerAddr(),
		Handler:      r,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// 在 goroutine 中启动服务器（非阻塞）
	go func() {
		logger.Log.Info("程序启动成功",
			zap.String("service", cfg.Server.Name),
			zap.String("mode", cfg.Server.Mode),
			zap.Int("port", cfg.Server.Port),
		)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Log.Fatal("服务启动失败", zap.Error(err))
		}
	}()

	// os.Signal 是 Go 中表示操作系统信号的类型
	// SIGINT: 用户按 Ctrl+C 产生
	// SIGTERM: kill 命令默认发送的信号（K8s 停止 Pod 时也发送此信号）
	// 使用 buffered channel 避免信号丢失
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit // 阻塞，直到收到信号
	logger.Log.Info("正在关闭服务...")

	// 设置 5 秒超时，等待现有请求完成
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 关闭 HTTP 服务
	if err := srv.Shutdown(ctx); err != nil {
		logger.Log.Error("服务关闭异常", zap.Error(err))
	}

	// 关闭其他资源连接
	closeResources()

	logger.Log.Info("服务已安全退出")
}

// closeResources 关闭所有资源连接
func closeResources() {
	// 关闭 MySQL 连接
	if sqlDB, err := database.DB.DB(); err == nil {
		sqlDB.Close()
		logger.Log.Info("MySQL 连接已关闭")
	}

	// 关闭 Redis 连接
	if redis.Client != nil {
		redis.Client.Close()
		logger.Log.Info("Redis 连接已关闭")
	}

	// 关闭 RabbitMQ 连接
	if rabbitmq.Channel != nil {
		rabbitmq.Channel.Close()
	}
	if rabbitmq.Conn != nil {
		rabbitmq.Conn.Close()
		logger.Log.Info("RabbitMQ 连接已关闭")
	}
}

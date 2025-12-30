package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"seckill/pkg/config"
	"seckill/pkg/database"
	"seckill/pkg/grpcx"
	"seckill/pkg/logger"
	"seckill/pkg/rabbitmq"
	"seckill/pkg/redis"
	pb "seckill/proto/seckill"
	"seckill/services/seckill/handler"
)

// ========================================================================
// 【重点学习】Seckill 微服务入口
// ========================================================================
// Seckill 服务是整个秒杀系统的核心，需要：
// 1. Redis - 库存预扣和重复购买检查
// 2. RabbitMQ - 异步消息处理
// 3. 熔断器 - 保护下游服务
//
// 📝 简历亮点：
// - 高并发场景下的服务设计
// - 多组件协调配合
// ========================================================================

var (
	configFile = flag.String("config", "config/config.yaml", "配置文件路径")
	port       = flag.Int("port", 50054, "gRPC 服务端口")
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

	// 3. 初始化数据库（秒杀服务可能需要查询商品信息）
	database.InitMySQL()

	// 4. 初始化 Redis
	redis.InitRedis()

	// 5. 初始化 RabbitMQ
	rabbitmq.InitRabbitMQ()

	// 6. 创建 gRPC 服务
	cfg := config.Get()
	serverConfig := &grpcx.ServerConfig{
		ServiceName: "seckill-service",
		Address:     "0.0.0.0",
		Port:        *port,
		ConsulAddr:  cfg.Consul.Address,
	}

	server, err := grpcx.NewServer(serverConfig)
	if err != nil {
		logger.Log.Fatal(fmt.Sprintf("创建 gRPC 服务失败: %v", err))
	}

	// 7. 注册服务实现
	seckillHandler := handler.NewSeckillHandler()
	pb.RegisterSeckillServiceServer(server.GRPCServer(), seckillHandler)

	// 8. 启动服务
	go func() {
		if err := server.Start(); err != nil {
			logger.Log.Fatal(fmt.Sprintf("gRPC 服务启动失败: %v", err))
		}
	}()

	fmt.Printf("Seckill Service 启动成功，端口: %d\n", *port)

	// 9. 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("正在关闭 Seckill Service...")
	server.Stop()
	rabbitmq.Close()
	fmt.Println("Seckill Service 已关闭")
}

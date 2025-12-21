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
	"seckill/pkg/redis"
	pb "seckill/proto/product"
	"seckill/services/product/handler"
)

// ========================================================================
// 【重点学习】Product 微服务入口
// ========================================================================
// Product 服务启动流程：
// 1. 加载配置
// 2. 初始化日志、数据库、Redis
// 3. 创建 gRPC 服务
// 4. 注册到 Consul
// 5. 启动并等待信号
//
// 📝 简历亮点：
// - 微服务依赖初始化顺序管理
// - 资源清理和优雅关闭
// ========================================================================

var (
	configFile = flag.String("config", "config/config.yaml", "配置文件路径")
	port       = flag.Int("port", 50052, "gRPC 服务端口")
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

	// 3. 初始化数据库
	database.InitMySQL()

	// 4. 初始化 Redis
	redis.InitRedis()

	// 5. 创建 gRPC 服务
	cfg := config.Get()
	serverConfig := &grpcx.ServerConfig{
		ServiceName: "product-service",
		Address:     "0.0.0.0",
		Port:        *port,
		ConsulAddr:  cfg.Consul.Address,
	}

	server, err := grpcx.NewServer(serverConfig)
	if err != nil {
		logger.Log.Fatal(fmt.Sprintf("创建 gRPC 服务失败: %v", err))
	}

	// 6. 注册服务实现
	productHandler := handler.NewProductHandler()
	pb.RegisterProductServiceServer(server.GRPCServer(), productHandler)

	// 7. 启动服务
	go func() {
		if err := server.Start(); err != nil {
			logger.Log.Fatal(fmt.Sprintf("gRPC 服务启动失败: %v", err))
		}
	}()

	fmt.Printf("Product Service 启动成功，端口: %d\n", *port)

	// 8. 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("正在关闭 Product Service...")
	server.Stop()
	fmt.Println("Product Service 已关闭")
}

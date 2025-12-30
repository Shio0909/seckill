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
	"seckill/pkg/snowflake"
	pb "seckill/proto/order"
	"seckill/services/order/handler"
)

// ========================================================================
// 【重点学习】Order 微服务入口
// ========================================================================

var (
	configFile = flag.String("config", "config/config.yaml", "配置文件路径")
	port       = flag.Int("port", 50053, "gRPC 服务端口")
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

	// 3. 初始化数据库
	database.InitMySQL()

	// 4. 初始化雪花算法
	snowflake.Init(1) // 节点 ID，分布式部署时每个节点不同

	// 5. 创建 gRPC 服务
	cfg := config.Get()
	serverConfig := &grpcx.ServerConfig{
		ServiceName: "order-service",
		Address:     "0.0.0.0",
		Port:        *port,
		ConsulAddr:  cfg.Consul.Address,
	}

	server, err := grpcx.NewServer(serverConfig)
	if err != nil {
		logger.Log.Fatal(fmt.Sprintf("创建 gRPC 服务失败: %v", err))
	}

	// 6. 注册服务实现
	orderHandler := handler.NewOrderHandler()
	pb.RegisterOrderServiceServer(server.GRPCServer(), orderHandler)

	// 7. 启动服务
	go func() {
		if err := server.Start(); err != nil {
			logger.Log.Fatal(fmt.Sprintf("gRPC 服务启动失败: %v", err))
		}
	}()

	fmt.Printf("Order Service 启动成功，端口: %d\n", *port)

	// 8. 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("正在关闭 Order Service...")
	server.Stop()
	fmt.Println("Order Service 已关闭")
}

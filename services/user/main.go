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
	pb "seckill/proto/user"
	"seckill/services/user/handler"
)

// ========================================================================
// 【重点学习】微服务入口 - User Service
// ========================================================================
// 微服务的 main 函数负责：
// 1. 加载配置
// 2. 初始化依赖（数据库、日志等）
// 3. 创建并启动 gRPC 服务
// 4. 注册到 Consul
// 5. 优雅关闭
//
// 📝 简历亮点：
// - 微服务独立部署和运行
// - 配置热加载支持
// - 优雅关闭机制
//
// 🔥 面试高频：
// Q: 微服务如何实现配置管理？
// A: 1. 本地配置文件 2. 环境变量 3. 配置中心（Consul KV）
//    本项目采用 Viper + Consul KV 的组合方案
//
// Q: 微服务如何保证高可用？
// A: 1. 多实例部署 2. 健康检查 3. 自动剔除故障节点 4. 负载均衡
// ========================================================================

var (
	configFile = flag.String("config", "config/config.yaml", "配置文件路径")
	port       = flag.Int("port", 50051, "gRPC 服务端口")
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

	// 4. 创建 gRPC 服务
	cfg := config.Get()
	serverConfig := &grpcx.ServerConfig{
		ServiceName: "user-service",
		Address:     "0.0.0.0",
		Port:        *port,
		ConsulAddr:  cfg.Consul.Address,
	}

	server, err := grpcx.NewServer(serverConfig)
	if err != nil {
		logger.Log.Fatal(fmt.Sprintf("创建 gRPC 服务失败: %v", err))
	}

	// 5. 注册服务实现
	// 【重点】将 Handler 注册到 gRPC Server
	userHandler := handler.NewUserHandler()
	pb.RegisterUserServiceServer(server.GRPCServer(), userHandler)

	// 6. 启动服务（异步）
	go func() {
		if err := server.Start(); err != nil {
			logger.Log.Fatal(fmt.Sprintf("gRPC 服务启动失败: %v", err))
		}
	}()

	fmt.Printf("User Service 启动成功，端口: %d\n", *port)

	// 7. 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("正在关闭 User Service...")
	server.Stop()
	fmt.Println("User Service 已关闭")
}

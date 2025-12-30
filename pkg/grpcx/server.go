package grpcx

import (
	"context"
	"fmt"
	"net"
	"time"

	"seckill/pkg/consul"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

// ========================================================================
// 【重点学习】gRPC 服务端实现
// ========================================================================
// gRPC 服务端需要处理：
// 1. 监听端口
// 2. 注册服务实现
// 3. 健康检查
// 4. 服务注册到 Consul
// 5. 优雅关闭
//
// 📝 简历亮点：
// - gRPC 服务端实现和配置
// - 集成健康检查协议
// - 服务注册与反注册
// ========================================================================

// ServerConfig gRPC 服务端配置
type ServerConfig struct {
	ServiceName string // 服务名称
	Address     string // 监听地址，如 "0.0.0.0"
	Port        int    // 监听端口
	// Consul 配置
	ConsulAddr     string // Consul 地址
	HealthCheckURL string // 健康检查地址
}

// Server gRPC 服务端封装
type Server struct {
	config       *ServerConfig
	grpcServer   *grpc.Server
	consulClient *consul.Client
	healthServer *health.Server
	listener     net.Listener
}

// NewServer 创建 gRPC 服务端
func NewServer(config *ServerConfig) (*Server, error) {
	// 创建 gRPC 服务器
	grpcServer := grpc.NewServer(
		// 可以添加服务端拦截器
		grpc.ChainUnaryInterceptor(
			ServerLoggingInterceptor(),
			ServerRecoveryInterceptor(),
		),
	)

	// 【重点】注册健康检查服务
	// Consul 会通过这个服务检查 gRPC 服务是否健康
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)

	// 注册反射服务（方便调试，生产环境可关闭）
	reflection.Register(grpcServer)

	server := &Server{
		config:       config,
		grpcServer:   grpcServer,
		healthServer: healthServer,
	}

	// 连接 Consul
	if config.ConsulAddr != "" {
		consulClient, err := consul.NewClient(&consul.Config{
			Address: config.ConsulAddr,
		})
		if err != nil {
			return nil, fmt.Errorf("连接 Consul 失败: %w", err)
		}
		server.consulClient = consulClient
	}

	return server, nil
}

// GRPCServer 获取原生 gRPC Server（用于注册服务）
func (s *Server) GRPCServer() *grpc.Server {
	return s.grpcServer
}

// SetServingStatus 设置服务健康状态
func (s *Server) SetServingStatus(service string, serving bool) {
	if serving {
		s.healthServer.SetServingStatus(service, grpc_health_v1.HealthCheckResponse_SERVING)
	} else {
		s.healthServer.SetServingStatus(service, grpc_health_v1.HealthCheckResponse_NOT_SERVING)
	}
}

// Start 启动服务
func (s *Server) Start() error {
	// 监听端口
	addr := fmt.Sprintf("%s:%d", s.config.Address, s.config.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("监听端口失败: %w", err)
	}
	s.listener = listener

	// 注册到 Consul
	if s.consulClient != nil {
		if err := s.registerToConsul(); err != nil {
			return fmt.Errorf("注册到 Consul 失败: %w", err)
		}
	}

	// 设置服务为健康状态
	s.SetServingStatus(s.config.ServiceName, true)
	s.SetServingStatus("", true) // 空字符串表示整体健康状态

	fmt.Printf("[gRPC] %s 服务启动成功，监听 %s\n", s.config.ServiceName, addr)

	// 启动 gRPC 服务（阻塞）
	return s.grpcServer.Serve(listener)
}

func (s *Server) registerToConsul() error {
	// 获取本机 IP（简化处理，实际应该更智能地获取）
	localIP := s.config.Address
	if localIP == "0.0.0.0" || localIP == "" {
		localIP = "127.0.0.1" // 开发环境使用 localhost
	}

	reg := &consul.ServiceRegistration{
		ID:            consul.GenerateServiceID(s.config.ServiceName, localIP, s.config.Port),
		Name:          s.config.ServiceName,
		Tags:          []string{"grpc", "microservice"},
		Address:       localIP,
		Port:          s.config.Port,
		CheckGRPC:     fmt.Sprintf("%s:%d/%s", localIP, s.config.Port, s.config.ServiceName),
		CheckInterval: "10s",
		CheckTimeout:  "5s",
	}

	return s.consulClient.Register(reg)
}

// Stop 停止服务
func (s *Server) Stop() {
	fmt.Printf("[gRPC] %s 服务正在关闭...\n", s.config.ServiceName)

	// 设置服务为不健康状态
	s.SetServingStatus(s.config.ServiceName, false)
	s.SetServingStatus("", false)

	// 从 Consul 注销
	if s.consulClient != nil {
		localIP := s.config.Address
		if localIP == "0.0.0.0" || localIP == "" {
			localIP = "127.0.0.1"
		}
		serviceID := consul.GenerateServiceID(s.config.ServiceName, localIP, s.config.Port)
		if err := s.consulClient.Deregister(serviceID); err != nil {
			fmt.Printf("[gRPC] 从 Consul 注销失败: %v\n", err)
		}
	}

	// 优雅关闭 gRPC 服务
	s.grpcServer.GracefulStop()

	fmt.Printf("[gRPC] %s 服务已关闭\n", s.config.ServiceName)
}

// ========================================================================
// 【重点学习】服务端拦截器
// ========================================================================

// ServerLoggingInterceptor 服务端日志拦截器
func ServerLoggingInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler) (interface{}, error) {

		start := time.Now()
		resp, err := handler(ctx, req)
		duration := time.Since(start)

		if err != nil {
			fmt.Printf("[gRPC Server] %s error=%v duration=%v\n", info.FullMethod, err, duration)
		} else {
			fmt.Printf("[gRPC Server] %s duration=%v\n", info.FullMethod, duration)
		}

		return resp, err
	}
}

// ServerRecoveryInterceptor 服务端 panic 恢复拦截器
func ServerRecoveryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler) (resp interface{}, err error) {

		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("[gRPC Server] panic recovered: %v\n", r)
				err = fmt.Errorf("internal server error")
			}
		}()

		return handler(ctx, req)
	}
}

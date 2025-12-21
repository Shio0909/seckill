package grpcx

import (
	"context"
	"fmt"
	"time"

	"seckill/pkg/breaker"
	"seckill/pkg/consul"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
)

// ========================================================================
// 【重点学习】gRPC 客户端连接管理
// ========================================================================
// 在微服务架构中，服务间通信需要考虑：
// 1. 连接池管理（复用连接，避免频繁建立连接）
// 2. 服务发现（动态获取服务地址）
// 3. 负载均衡（分散请求到多个实例）
// 4. 熔断保护（防止级联故障）
// 5. 超时控制（避免长时间等待）
//
// 📝 简历亮点：
// - gRPC 连接池管理和 keepalive 配置
// - 集成 Consul 服务发现
// - 熔断器保护下游调用
//
// 🔥 面试高频：
// Q: gRPC 的连接是长连接还是短连接？
// A: gRPC 基于 HTTP/2，默认使用长连接，支持多路复用
//
// Q: 如何处理 gRPC 连接的健康检查？
// A: gRPC 内置健康检查协议（grpc.health.v1.Health），
//    客户端可以定期调用 Check 方法检查服务状态
// ========================================================================

// ClientConfig gRPC 客户端配置
type ClientConfig struct {
	ServiceName    string        // 服务名称（用于服务发现）
	DirectAddress  string        // 直连地址（不使用服务发现时）
	Timeout        time.Duration // 请求超时时间
	MaxRetries     int           // 最大重试次数
	EnableBreaker  bool          // 是否启用熔断器
	BreakerSetting breaker.Settings
}

// ClientManager gRPC 客户端管理器
type ClientManager struct {
	consulClient   *consul.Client
	breakerManager *breaker.BreakerManager
	connections    map[string]*grpc.ClientConn
}

// NewClientManager 创建客户端管理器
func NewClientManager(consulClient *consul.Client, defaultConfig *ClientConfig) *ClientManager {
	return &ClientManager{
		consulClient:   consulClient,
		breakerManager: breaker.NewBreakerManager(breaker.DefaultSettings()),
		connections:    make(map[string]*grpc.ClientConn),
	}
}

// GetConnection 获取 gRPC 连接（通过服务名）
func (m *ClientManager) GetConnection(serviceName string) (*grpc.ClientConn, error) {
	return m.GetConnectionWithConfig(&ClientConfig{ServiceName: serviceName})
}

// GetConnectionWithConfig 获取 gRPC 连接（通过配置）
func (m *ClientManager) GetConnectionWithConfig(config *ClientConfig) (*grpc.ClientConn, error) {
	// 确定目标地址
	var target string
	var err error

	if config.DirectAddress != "" {
		target = config.DirectAddress
	} else if m.consulClient != nil {
		// 从 Consul 发现服务
		target, err = m.consulClient.GetServiceAddress(config.ServiceName)
		if err != nil {
			return nil, fmt.Errorf("服务发现失败: %w", err)
		}
	} else {
		return nil, fmt.Errorf("未配置服务地址")
	}

	// 检查是否已有连接
	if conn, ok := m.connections[target]; ok {
		return conn, nil
	}

	// 创建新连接
	conn, err := m.dial(target, config)
	if err != nil {
		return nil, err
	}

	m.connections[target] = conn
	return conn, nil
}

func (m *ClientManager) dial(target string, config *ClientConfig) (*grpc.ClientConn, error) {
	// 【重点】gRPC 连接选项配置
	opts := []grpc.DialOption{
		// 使用不安全连接（生产环境应使用 TLS）
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		// Keepalive 配置
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                10 * time.Second, // 每 10 秒发送 keepalive ping
			Timeout:             3 * time.Second,  // ping 超时时间
			PermitWithoutStream: true,             // 无活跃流时也发送 ping
		}),
	}

	// 设置超时
	timeout := config.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, target, opts...)
	if err != nil {
		return nil, fmt.Errorf("连接 %s 失败: %w", target, err)
	}

	return conn, nil
}

// CallWithBreaker 带熔断器的调用
func (m *ClientManager) CallWithBreaker(serviceName string, call func() (interface{}, error)) (interface{}, error) {
	cb := m.breakerManager.GetBreaker(serviceName)
	return cb.Execute(call)
}

// Close 关闭所有连接
func (m *ClientManager) Close() {
	for _, conn := range m.connections {
		conn.Close()
	}
}

// ========================================================================
// 【重点学习】gRPC 健康检查
// ========================================================================
// gRPC 提供标准的健康检查协议，用于：
// 1. 负载均衡器判断实例是否健康
// 2. Consul 等服务发现组件进行健康检查
// 3. 客户端主动检测服务状态
// ========================================================================

// HealthChecker gRPC 健康检查客户端
type HealthChecker struct {
	conn *grpc.ClientConn
}

// NewHealthChecker 创建健康检查客户端
func NewHealthChecker(conn *grpc.ClientConn) *HealthChecker {
	return &HealthChecker{conn: conn}
}

// Check 检查服务健康状态
func (h *HealthChecker) Check(ctx context.Context, service string) (bool, error) {
	client := grpc_health_v1.NewHealthClient(h.conn)

	resp, err := client.Check(ctx, &grpc_health_v1.HealthCheckRequest{
		Service: service,
	})
	if err != nil {
		return false, err
	}

	return resp.Status == grpc_health_v1.HealthCheckResponse_SERVING, nil
}

// ========================================================================
// 【重点学习】gRPC 拦截器（Interceptor）
// ========================================================================
// 拦截器类似于 HTTP 中间件，用于在 RPC 调用前后执行通用逻辑：
// 1. UnaryInterceptor - 一元调用拦截器
// 2. StreamInterceptor - 流式调用拦截器
//
// 常见用途：日志、认证、限流、熔断、链路追踪
// ========================================================================

// LoggingInterceptor 日志拦截器
func LoggingInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{},
		cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {

		start := time.Now()
		err := invoker(ctx, method, req, reply, cc, opts...)
		duration := time.Since(start)

		// 这里可以替换为结构化日志
		if err != nil {
			fmt.Printf("[gRPC] %s error=%v duration=%v\n", method, err, duration)
		} else {
			fmt.Printf("[gRPC] %s duration=%v\n", method, duration)
		}

		return err
	}
}

// TimeoutInterceptor 超时拦截器
func TimeoutInterceptor(timeout time.Duration) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{},
		cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {

		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// BreakerInterceptor 熔断拦截器
func BreakerInterceptor(bm *breaker.BreakerManager) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{},
		cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {

		cb := bm.GetBreaker(method)

		_, err := cb.Execute(func() (interface{}, error) {
			return nil, invoker(ctx, method, req, reply, cc, opts...)
		})

		return err
	}
}

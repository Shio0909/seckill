package consul

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/consul/api"
)

// ========================================================================
// 【重点学习】Consul 服务注册与发现
// ========================================================================
// Consul 是 HashiCorp 开发的服务网格解决方案，提供：
// 1. 服务注册（Service Registration）
// 2. 服务发现（Service Discovery）
// 3. 健康检查（Health Check）
// 4. KV 存储（配置中心）
//
// 📝 简历亮点：
// - 理解服务注册与发现的原理
// - Consul 的一致性模型（Raft 协议）
// - 健康检查机制（HTTP/TCP/gRPC）
//
// 🔥 面试高频：
// Q: 服务发现有哪些方式？
// A: 1) 客户端发现（Client-side Discovery）- 客户端直接查询注册中心
//    2) 服务端发现（Server-side Discovery）- 通过负载均衡器转发
//    Consul 属于客户端发现模式
//
// Q: Consul vs etcd vs Nacos 的区别？
// A: Consul - Go 语言，内置服务发现+健康检查，CP 模型
//    etcd - Go 语言，K8s 底层存储，强一致性
//    Nacos - Java 语言，阿里开源，支持 AP/CP 切换
// ========================================================================

// Config Consul 客户端配置
type Config struct {
	Address string // Consul 地址，如 "127.0.0.1:8500"
}

// Client Consul 客户端封装
type Client struct {
	client *api.Client
	config *Config
}

// NewClient 创建 Consul 客户端
func NewClient(config *Config) (*Client, error) {
	cfg := api.DefaultConfig()

	// 如果提供了配置，使用配置的地址
	if config != nil && config.Address != "" {
		cfg.Address = config.Address
	}

	client, err := api.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("创建 Consul 客户端失败: %w", err)
	}

	return &Client{
		client: client,
		config: config,
	}, nil
}

// ServiceRegistration 服务注册信息
type ServiceRegistration struct {
	ID      string   // 服务实例 ID，如 "user-service-1"
	Name    string   // 服务名称，如 "user-service"
	Tags    []string // 服务标签
	Address string   // 服务地址
	Port    int      // 服务端口
	// 健康检查配置
	CheckHTTP     string // HTTP 健康检查地址
	CheckGRPC     string // gRPC 健康检查地址
	CheckInterval string // 检查间隔，如 "10s"
	CheckTimeout  string // 检查超时，如 "5s"
}

// ========================================================================
// 【重点学习】服务注册流程
// ========================================================================
// 1. 服务启动时向 Consul 注册自己的信息
// 2. Consul 定期对服务进行健康检查
// 3. 健康检查失败时，Consul 将服务标记为不健康
// 4. 服务关闭时主动注销
//
// 关键点：服务 ID 必须唯一，通常用 "服务名-IP-端口" 格式
// ========================================================================

// Register 注册服务
func (c *Client) Register(reg *ServiceRegistration) error {
	registration := &api.AgentServiceRegistration{
		ID:      reg.ID,
		Name:    reg.Name,
		Tags:    reg.Tags,
		Address: reg.Address,
		Port:    reg.Port,
	}

	// 配置健康检查
	// 【重点】gRPC 服务使用 gRPC 健康检查协议
	if reg.CheckGRPC != "" {
		registration.Check = &api.AgentServiceCheck{
			GRPC:                           reg.CheckGRPC,
			Interval:                       reg.CheckInterval,
			Timeout:                        reg.CheckTimeout,
			DeregisterCriticalServiceAfter: "30s", // 30秒不健康后自动注销
		}
	} else if reg.CheckHTTP != "" {
		registration.Check = &api.AgentServiceCheck{
			HTTP:                           reg.CheckHTTP,
			Interval:                       reg.CheckInterval,
			Timeout:                        reg.CheckTimeout,
			DeregisterCriticalServiceAfter: "30s",
		}
	}

	if err := c.client.Agent().ServiceRegister(registration); err != nil {
		return fmt.Errorf("服务注册失败: %w", err)
	}

	return nil
}

// Deregister 注销服务
func (c *Client) Deregister(serviceID string) error {
	return c.client.Agent().ServiceDeregister(serviceID)
}

// ========================================================================
// 【重点学习】服务发现流程
// ========================================================================
// 1. 客户端向 Consul 查询目标服务的健康实例列表
// 2. 从返回的实例中选择一个（负载均衡）
// 3. 直接连接选中的服务实例
//
// 负载均衡策略：
// - 轮询（Round Robin）
// - 随机（Random）
// - 加权轮询（Weighted Round Robin）
// - 最少连接（Least Connections）
// ========================================================================

// ServiceInstance 服务实例信息
type ServiceInstance struct {
	ID      string
	Name    string
	Address string
	Port    int
	Tags    []string
}

// DiscoverService 发现服务（获取健康实例列表）
func (c *Client) DiscoverService(serviceName string) ([]*ServiceInstance, error) {
	// 只获取健康的服务实例
	services, _, err := c.client.Health().Service(serviceName, "", true, nil)
	if err != nil {
		return nil, fmt.Errorf("服务发现失败: %w", err)
	}

	instances := make([]*ServiceInstance, 0, len(services))
	for _, svc := range services {
		instances = append(instances, &ServiceInstance{
			ID:      svc.Service.ID,
			Name:    svc.Service.Service,
			Address: svc.Service.Address,
			Port:    svc.Service.Port,
			Tags:    svc.Service.Tags,
		})
	}

	return instances, nil
}

// GetServiceAddress 获取服务地址（简化版，返回第一个健康实例）
func (c *Client) GetServiceAddress(serviceName string) (string, error) {
	instances, err := c.DiscoverService(serviceName)
	if err != nil {
		return "", err
	}

	if len(instances) == 0 {
		return "", fmt.Errorf("没有可用的服务实例: %s", serviceName)
	}

	// 简单返回第一个实例，实际应该实现负载均衡
	inst := instances[0]
	return fmt.Sprintf("%s:%d", inst.Address, inst.Port), nil
}

// ========================================================================
// 【重点学习】Consul KV 存储 - 配置中心
// ========================================================================
// Consul KV 可以用作轻量级配置中心：
// 1. 存储服务配置（数据库连接、Redis 地址等）
// 2. 支持 Watch 机制实现配置热更新
// 3. 支持事务操作
//
// 📝 简历亮点：使用 Consul KV 实现配置中心，支持配置热更新
// ========================================================================

// SetKV 设置键值对
func (c *Client) SetKV(key, value string) error {
	p := &api.KVPair{Key: key, Value: []byte(value)}
	_, err := c.client.KV().Put(p, nil)
	return err
}

// GetKV 获取键值对
func (c *Client) GetKV(key string) (string, error) {
	pair, _, err := c.client.KV().Get(key, nil)
	if err != nil {
		return "", err
	}
	if pair == nil {
		return "", fmt.Errorf("key not found: %s", key)
	}
	return string(pair.Value), nil
}

// GetKVWithPrefix 获取指定前缀的所有键值对
func (c *Client) GetKVWithPrefix(prefix string) (map[string]string, error) {
	pairs, _, err := c.client.KV().List(prefix, nil)
	if err != nil {
		return nil, err
	}

	result := make(map[string]string)
	for _, pair := range pairs {
		// 去掉前缀，只保留相对路径
		key := strings.TrimPrefix(pair.Key, prefix)
		result[key] = string(pair.Value)
	}
	return result, nil
}

// WatchKV 监听键变化（阻塞式）
// 返回新值和是否有变化
func (c *Client) WatchKV(key string, lastIndex uint64) (string, uint64, error) {
	opts := &api.QueryOptions{
		WaitIndex: lastIndex,
		WaitTime:  0, // 0 表示使用默认超时（5分钟）
	}

	pair, meta, err := c.client.KV().Get(key, opts)
	if err != nil {
		return "", 0, err
	}

	if pair == nil {
		return "", meta.LastIndex, nil
	}

	return string(pair.Value), meta.LastIndex, nil
}

// GenerateServiceID 生成服务实例 ID
func GenerateServiceID(serviceName, address string, port int) string {
	return fmt.Sprintf("%s-%s-%d", serviceName, strings.ReplaceAll(address, ".", "-"), port)
}

// ParseAddress 解析地址字符串为 host 和 port
func ParseAddress(addr string) (string, int, error) {
	parts := strings.Split(addr, ":")
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("invalid address format: %s", addr)
	}
	port, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, fmt.Errorf("invalid port: %s", parts[1])
	}
	return parts[0], port, nil
}

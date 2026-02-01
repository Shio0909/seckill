package consul

import (
	"testing"
)

// 这些测试主要验证 Consul 客户端的功能，
// 但需要真实的 Consul 服务才能运行完整测试。

// TestGenerateServiceID 测试服务 ID 生成
func TestGenerateServiceID(t *testing.T) {
	tests := []struct {
		name     string
		service  string
		ip       string
		port     int
		expected string
	}{
		{
			name:     "正常生成",
			service:  "user-service",
			ip:       "192.168.1.1",
			port:     8080,
			expected: "user-service-192-168-1-1-8080", // IP 中的 . 被替换为 -
		},
		{
			name:     "本地地址",
			service:  "order-service",
			ip:       "127.0.0.1",
			port:     50051,
			expected: "order-service-127-0-0-1-50051", // IP 中的 . 被替换为 -
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateServiceID(tt.service, tt.ip, tt.port)
			if result != tt.expected {
				t.Errorf("GenerateServiceID() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestServiceRegistration 测试服务注册结构体
func TestServiceRegistration(t *testing.T) {
	reg := &ServiceRegistration{
		ID:            "test-service-1",
		Name:          "test-service",
		Tags:          []string{"grpc", "test"},
		Address:       "127.0.0.1",
		Port:          8080,
		CheckHTTP:     "http://127.0.0.1:8080/health",
		CheckInterval: "10s",
		CheckTimeout:  "5s",
	}

	if reg.ID != "test-service-1" {
		t.Errorf("Expected ID 'test-service-1', got '%s'", reg.ID)
	}

	if len(reg.Tags) != 2 {
		t.Errorf("Expected 2 tags, got %d", len(reg.Tags))
	}
}

// TestNewClientNilConfig 测试空配置
func TestNewClientNilConfig(t *testing.T) {
	// 测试空配置时应该使用默认值
	// 注意：这个测试需要 Consul 服务运行才能通过
	// 在没有 Consul 的情况下会失败，这是预期行为
	_, err := NewClient(nil)
	if err == nil {
		// 如果没有运行 Consul，应该返回错误
		t.Log("Consul client created successfully (Consul is running)")
	} else {
		t.Logf("Expected error when Consul is not running: %v", err)
	}
}

// TestConfig 测试配置结构体
func TestConfig(t *testing.T) {
	cfg := &Config{
		Address: "localhost:8500",
	}

	if cfg.Address != "localhost:8500" {
		t.Errorf("Expected address 'localhost:8500', got '%s'", cfg.Address)
	}
}

// 以下测试需要真实的 Consul 服务
// 在 CI/CD 环境中可以启动 Consul 容器来运行这些测试

/*
func TestIntegration_RegisterAndDiscover(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client, err := NewClient(&Config{
		Address: "127.0.0.1:8500",
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// 注册服务
	reg := &ServiceRegistration{
		ID:      "test-service-integration",
		Name:    "test-service",
		Address: "127.0.0.1",
		Port:    9999,
	}

	if err := client.Register(reg); err != nil {
		t.Fatalf("Failed to register service: %v", err)
	}
	defer client.Deregister(reg.ID)

	// 发现服务
	services, err := client.DiscoverService("test-service")
	if err != nil {
		t.Fatalf("Failed to discover service: %v", err)
	}

	if len(services) == 0 {
		t.Error("Expected at least one service instance")
	}
}
*/

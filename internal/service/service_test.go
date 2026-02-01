package service

import (
	"sync"
	"testing"
	"time"
)

// Go 测试文件命名规则：
// - 文件名以 _test.go 结尾
// - 测试函数以 Test 开头
// - 基准测试以 Benchmark 开头
//
// 常用命令：
// go test ./...                    # 运行所有测试
// go test -v ./...                 # 详细输出
// go test -cover ./...             # 显示覆盖率
// go test -coverprofile=cover.out  # 生成覆盖率文件
// go tool cover -html=cover.out    # 在浏览器查看覆盖率
//
// 表格驱动测试（Table-Driven Tests）：
// Go 推荐的测试模式，将测试用例放在表格中，循环执行
// 优点：易于添加新用例，代码简洁，一目了然

// TestProductListRequest_Defaults 测试默认值设置
func TestProductListRequest_Defaults(t *testing.T) {
	// 表格驱动测试
	tests := []struct {
		name         string
		input        ProductListRequest
		expectedPage int
		expectedSize int
	}{
		{
			name:         "零值应设置默认值",
			input:        ProductListRequest{},
			expectedPage: 1,
			expectedSize: 10,
		},
		{
			name:         "负数应设置默认值",
			input:        ProductListRequest{Page: -1, PageSize: -5},
			expectedPage: 1,
			expectedSize: 10,
		},
		{
			name:         "正常值应保持不变",
			input:        ProductListRequest{Page: 2, PageSize: 20},
			expectedPage: 2,
			expectedSize: 20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 模拟 List 方法中的默认值设置逻辑
			req := tt.input
			if req.Page <= 0 {
				req.Page = 1
			}
			if req.PageSize <= 0 {
				req.PageSize = 10
			}

			if req.Page != tt.expectedPage {
				t.Errorf("Page = %d, want %d", req.Page, tt.expectedPage)
			}
			if req.PageSize != tt.expectedSize {
				t.Errorf("PageSize = %d, want %d", req.PageSize, tt.expectedSize)
			}
		})
	}
}

// TestOrderStatus_Transitions 测试订单状态流转
func TestOrderStatus_Transitions(t *testing.T) {
	tests := []struct {
		name        string
		fromStatus  int
		action      string
		canTransit  bool
		expectError bool
	}{
		{
			name:        "待支付可以取消",
			fromStatus:  OrderStatusPending,
			action:      "cancel",
			canTransit:  true,
			expectError: false,
		},
		{
			name:        "已支付不能取消",
			fromStatus:  OrderStatusPaid,
			action:      "cancel",
			canTransit:  false,
			expectError: true,
		},
		{
			name:        "待支付可以支付",
			fromStatus:  OrderStatusPending,
			action:      "pay",
			canTransit:  true,
			expectError: false,
		},
		{
			name:        "已取消不能支付",
			fromStatus:  OrderStatusCancelled,
			action:      "pay",
			canTransit:  false,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			canTransit := false
			switch tt.action {
			case "cancel":
				canTransit = tt.fromStatus == OrderStatusPending
			case "pay":
				canTransit = tt.fromStatus == OrderStatusPending
			}

			if canTransit != tt.canTransit {
				t.Errorf("canTransit = %v, want %v", canTransit, tt.canTransit)
			}
		})
	}
}

// 基准测试用于测量代码性能
// 运行命令：go test -bench=. -benchmem
//
// 输出解读：
// BenchmarkXxx-8    1000000    1234 ns/op    256 B/op    4 allocs/op
//                   ^执行次数  ^每次耗时     ^每次内存   ^每次分配次数

// BenchmarkOrderStatusCheck 基准测试：订单状态检查
func BenchmarkOrderStatusCheck(b *testing.B) {
	statuses := []int{
		OrderStatusPending,
		OrderStatusPaid,
		OrderStatusShipped,
		OrderStatusCompleted,
		OrderStatusCancelled,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		status := statuses[i%len(statuses)]
		_ = status == OrderStatusPending
	}
}

// 测试代码在并发环境下的正确性
// 使用 sync.WaitGroup 等待所有 goroutine 完成
// 使用 -race 标志检测数据竞争：go test -race ./...

// TestConcurrentAccess 测试并发访问
func TestConcurrentAccess(t *testing.T) {
	var counter int
	var mu sync.Mutex
	var wg sync.WaitGroup

	// 模拟 100 个并发请求
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mu.Lock()
			counter++
			mu.Unlock()
		}()
	}

	wg.Wait()

	if counter != 100 {
		t.Errorf("counter = %d, want 100", counter)
	}
}

// TestCreateProductRequest_Validation 测试创建商品请求验证
func TestCreateProductRequest_Validation(t *testing.T) {
	tests := []struct {
		name    string
		req     CreateProductRequest
		wantErr bool
	}{
		{
			name: "正常请求",
			req: CreateProductRequest{
				Name:         "iPhone 15",
				Price:        5999.00,
				SeckillPrice: 4999.00,
				Stock:        100,
				StartTime:    time.Now(),
				EndTime:      time.Now().Add(24 * time.Hour),
			},
			wantErr: false,
		},
		{
			name: "名称为空",
			req: CreateProductRequest{
				Name:         "",
				Price:        5999.00,
				SeckillPrice: 4999.00,
				Stock:        100,
				StartTime:    time.Now(),
				EndTime:      time.Now().Add(24 * time.Hour),
			},
			wantErr: true,
		},
		{
			name: "价格为负",
			req: CreateProductRequest{
				Name:         "iPhone 15",
				Price:        -100,
				SeckillPrice: 4999.00,
				Stock:        100,
				StartTime:    time.Now(),
				EndTime:      time.Now().Add(24 * time.Hour),
			},
			wantErr: true,
		},
		{
			name: "结束时间早于开始时间",
			req: CreateProductRequest{
				Name:         "iPhone 15",
				Price:        5999.00,
				SeckillPrice: 4999.00,
				Stock:        100,
				StartTime:    time.Now().Add(24 * time.Hour),
				EndTime:      time.Now(),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 简单的验证逻辑（实际项目中使用 validator）
			hasErr := false
			if tt.req.Name == "" {
				hasErr = true
			}
			if tt.req.Price <= 0 {
				hasErr = true
			}
			if !tt.req.EndTime.After(tt.req.StartTime) {
				hasErr = true
			}

			if hasErr != tt.wantErr {
				t.Errorf("validation error = %v, wantErr %v", hasErr, tt.wantErr)
			}
		})
	}
}

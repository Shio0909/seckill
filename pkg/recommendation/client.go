package recommendation

import (
	"context"
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"

	pb "seckill/proto/recommendation"
)

// Client 推荐服务客户端
type Client struct {
	conn   *grpc.ClientConn
	client pb.RecommendationServiceClient
	mu     sync.RWMutex
}

// Config 客户端配置
type Config struct {
	Address         string        `yaml:"address"`
	Timeout         time.Duration `yaml:"timeout"`
	MaxRetries      int           `yaml:"maxRetries"`
	KeepAliveTime   time.Duration `yaml:"keepAliveTime"`
	KeepAliveTimout time.Duration `yaml:"keepAliveTimeout"`
}

// DefaultConfig 默认配置
func DefaultConfig() *Config {
	return &Config{
		Address:         "localhost:50052",
		Timeout:         time.Second * 3,
		MaxRetries:      3,
		KeepAliveTime:   time.Second * 30,
		KeepAliveTimout: time.Second * 10,
	}
}

// NewClient 创建客户端
func NewClient(cfg *Config) (*Client, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	// gRPC连接选项
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                cfg.KeepAliveTime,
			Timeout:             cfg.KeepAliveTimout,
			PermitWithoutStream: true,
		}),
	}

	// 建立连接
	conn, err := grpc.Dial(cfg.Address, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect recommendation service: %w", err)
	}

	return &Client{
		conn:   conn,
		client: pb.NewRecommendationServiceClient(conn),
	}, nil
}

// Close 关闭连接
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// RecommendItem 推荐物品
type RecommendItem struct {
	EventID      int64   `json:"event_id"`
	Score        float32 `json:"score"`
	RecallSource string  `json:"recall_source"`
	DebugInfo    string  `json:"debug_info,omitempty"`
}

// RecommendResult 推荐结果
type RecommendResult struct {
	Items     []RecommendItem `json:"items"`
	RequestID string          `json:"request_id"`
	LatencyMs int64           `json:"latency_ms"`
}

// GetRecommendations 获取推荐列表
func (c *Client) GetRecommendations(ctx context.Context, userID int64, scene string, count int32, city string, categoryID int64, debug bool) (*RecommendResult, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	req := &pb.RecommendRequest{
		UserId:     userID,
		Scene:      scene,
		Count:      count,
		City:       city,
		CategoryId: categoryID,
		Debug:      debug,
	}

	resp, err := c.client.GetRecommendations(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("get recommendations failed: %w", err)
	}

	result := &RecommendResult{
		Items:     make([]RecommendItem, 0, len(resp.Items)),
		RequestID: resp.RequestId,
		LatencyMs: resp.LatencyMs,
	}

	for _, item := range resp.Items {
		result.Items = append(result.Items, RecommendItem{
			EventID:      item.EventId,
			Score:        item.Score,
			RecallSource: item.RecallSource,
			DebugInfo:    item.DebugInfo,
		})
	}

	return result, nil
}

// GetSimilarEvents 获取相似活动
func (c *Client) GetSimilarEvents(ctx context.Context, eventID int64, count int32) (*RecommendResult, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	req := &pb.SimilarRequest{
		EventId: eventID,
		Count:   count,
	}

	resp, err := c.client.GetSimilarEvents(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("get similar events failed: %w", err)
	}

	result := &RecommendResult{
		Items:     make([]RecommendItem, 0, len(resp.Items)),
		RequestID: resp.RequestId,
		LatencyMs: resp.LatencyMs,
	}

	for _, item := range resp.Items {
		result.Items = append(result.Items, RecommendItem{
			EventID:      item.EventId,
			Score:        item.Score,
			RecallSource: item.RecallSource,
		})
	}

	return result, nil
}

// RecordBehavior 记录用户行为
func (c *Client) RecordBehavior(ctx context.Context, userID, eventID int64, behavior string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	req := &pb.BehaviorRequest{
		UserId:    userID,
		EventId:   eventID,
		Behavior:  behavior,
		Timestamp: time.Now().Unix(),
	}

	_, err := c.client.RecordBehavior(ctx, req)
	if err != nil {
		return fmt.Errorf("record behavior failed: %w", err)
	}

	return nil
}

// RefreshUserProfile 刷新用户画像
func (c *Client) RefreshUserProfile(ctx context.Context, userID int64) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	req := &pb.RefreshRequest{
		UserId: userID,
	}

	_, err := c.client.RefreshUserProfile(ctx, req)
	if err != nil {
		return fmt.Errorf("refresh user profile failed: %w", err)
	}

	return nil
}

// HealthCheck 健康检查
func (c *Client) HealthCheck(ctx context.Context) (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	resp, err := c.client.HealthCheck(ctx, &pb.HealthRequest{})
	if err != nil {
		return "", fmt.Errorf("health check failed: %w", err)
	}

	return resp.Status, nil
}

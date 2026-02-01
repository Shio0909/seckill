package recall

import (
	"context"
	"fmt"

	"github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

// VectorRecall 向量召回器
// 基于Milvus的向量相似度搜索
type VectorRecall struct {
	milvus client.Client
	config VectorConfig
}

// NewVectorRecall 创建向量召回器
func NewVectorRecall(cfg VectorConfig) (*VectorRecall, error) {
	ctx := context.Background()

	// 连接Milvus
	c, err := client.NewClient(ctx, client.Config{
		Address: fmt.Sprintf("%s:%d", cfg.MilvusHost, cfg.MilvusPort),
	})
	if err != nil {
		return nil, fmt.Errorf("connect milvus failed: %w", err)
	}

	return &VectorRecall{
		milvus: c,
		config: cfg,
	}, nil
}

func (r *VectorRecall) Name() string {
	return "vector"
}

func (r *VectorRecall) Weight() float64 {
	return r.config.Weight
}

// Recall 执行向量召回
// 基于用户向量查询最相似的物品
func (r *VectorRecall) Recall(ctx context.Context, req *RecallRequest) ([]RecallItem, error) {
	// 获取用户向量(需要先从特征存储获取或实时计算)
	userVector, err := r.getUserVector(ctx, req.UserID)
	if err != nil {
		return nil, err
	}

	if userVector == nil {
		return nil, nil // 用户无向量,跳过
	}

	// 构建排除ID表达式
	excludeExpr := ""
	if len(req.Exclude) > 0 || len(req.History) > 0 {
		excludeIDs := append(req.Exclude, req.History...)
		excludeExpr = fmt.Sprintf("event_id not in %v", excludeIDs)
	}

	// 向量搜索
	searchResult, err := r.milvus.Search(
		ctx,
		r.config.CollectionName,
		[]string{}, // partitions
		excludeExpr,
		[]string{"event_id"},
		[]entity.Vector{entity.FloatVector(userVector)},
		"embedding",
		entity.IP, // Inner Product (需要向量已归一化)
		r.config.TopK,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("milvus search failed: %w", err)
	}

	items := make([]RecallItem, 0, r.config.TopK)
	for _, result := range searchResult {
		for i := 0; i < result.ResultCount; i++ {
			eventID, _ := result.Fields.GetColumn("event_id").Get(i)

			items = append(items, RecallItem{
				EventID: eventID.(int64),
				Score:   float64(result.Scores[i]),
				Source:  r.Name(),
			})
		}
	}

	return items, nil
}

// getUserVector 获取用户向量
func (r *VectorRecall) getUserVector(ctx context.Context, userID int64) ([]float32, error) {
	// 从Milvus查询用户向量
	// 这里假设用户向量也存在Milvus中,或者可以从Redis获取
	result, err := r.milvus.Query(
		ctx,
		"user_embeddings",
		[]string{},
		fmt.Sprintf("user_id == %d", userID),
		[]string{"embedding"},
	)
	if err != nil {
		return nil, nil // 查询失败返回nil,让上层跳过向量召回
	}

	if len(result) == 0 {
		return nil, nil
	}

	embCol := result.GetColumn("embedding")
	if embCol == nil {
		return nil, nil
	}

	vec, _ := embCol.Get(0)
	return vec.([]float32), nil
}

// SearchSimilarItems 搜索相似物品(用于"看了又看")
func (r *VectorRecall) SearchSimilarItems(ctx context.Context, itemID int64, count int) ([]RecallItem, error) {
	// 获取物品向量
	result, err := r.milvus.Query(
		ctx,
		r.config.CollectionName,
		[]string{},
		fmt.Sprintf("event_id == %d", itemID),
		[]string{"embedding"},
	)
	if err != nil {
		return nil, err
	}

	if len(result) == 0 {
		return nil, nil
	}

	embCol := result.GetColumn("embedding")
	vec, _ := embCol.Get(0)
	itemVector := vec.([]float32)

	// 排除自己
	excludeExpr := fmt.Sprintf("event_id != %d", itemID)

	// 搜索相似物品
	searchResult, err := r.milvus.Search(
		ctx,
		r.config.CollectionName,
		[]string{},
		excludeExpr,
		[]string{"event_id"},
		[]entity.Vector{entity.FloatVector(itemVector)},
		"embedding",
		entity.IP,
		count,
		nil,
	)
	if err != nil {
		return nil, err
	}

	items := make([]RecallItem, 0, count)
	for _, sr := range searchResult {
		for i := 0; i < sr.ResultCount; i++ {
			eventID, _ := sr.Fields.GetColumn("event_id").Get(i)
			items = append(items, RecallItem{
				EventID: eventID.(int64),
				Score:   float64(sr.Scores[i]),
				Source:  r.Name(),
			})
		}
	}

	return items, nil
}

// Close 关闭连接
func (r *VectorRecall) Close() error {
	if r.milvus != nil {
		return r.milvus.Close()
	}
	return nil
}

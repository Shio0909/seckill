package rerank

import "context"

// RerankItem 重排物品
type RerankItem struct {
	EventID      int64              `json:"event_id"`
	Score        float64            `json:"score"`        // 排序分数
	RecallSource string             `json:"recall_source"` // 召回来源
	Features     map[string]float64 `json:"features"`     // 特征
}

// Reranker 重排器接口
type Reranker interface {
	Rerank(ctx context.Context, items []RerankItem, req *RerankRequest) ([]RerankItem, error)
}

// RerankRequest 重排请求
type RerankRequest struct {
	UserID     int64   `json:"user_id"`
	Count      int     `json:"count"`
	History    []int64 `json:"history"`
	Blacklist  []int64 `json:"blacklist"`
	City       string  `json:"city"`
	CategoryID int64   `json:"category_id"`
}

// Pipeline 重排管道
type Pipeline struct {
	rerankers []Reranker
}

// NewPipeline 创建重排管道
func NewPipeline(rerankers ...Reranker) *Pipeline {
	return &Pipeline{rerankers: rerankers}
}

// Execute 执行重排管道
func (p *Pipeline) Execute(ctx context.Context, items []RerankItem, req *RerankRequest) ([]RerankItem, error) {
	result := items

	for _, reranker := range p.rerankers {
		var err error
		result, err = reranker.Rerank(ctx, result, req)
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

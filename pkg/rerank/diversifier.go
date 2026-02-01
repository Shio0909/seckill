package rerank

import (
	"context"
	"math"
)

// MMRDiversifier MMR多样性重排器
// Maximal Marginal Relevance: 平衡相关性和多样性
type MMRDiversifier struct {
	lambda float64 // 相关性权重 (0-1), 越大越偏向相关性
}

// NewMMRDiversifier 创建MMR重排器
func NewMMRDiversifier(lambda float64) *MMRDiversifier {
	if lambda <= 0 || lambda > 1 {
		lambda = 0.7 // 默认0.7
	}
	return &MMRDiversifier{lambda: lambda}
}

// Rerank 执行MMR重排
// MMR = λ * Relevance - (1-λ) * max(Similarity to selected)
func (d *MMRDiversifier) Rerank(ctx context.Context, items []RerankItem, req *RerankRequest) ([]RerankItem, error) {
	if len(items) <= 1 {
		return items, nil
	}

	n := len(items)
	if req.Count > 0 && req.Count < n {
		n = req.Count
	}

	selected := make([]RerankItem, 0, n)
	remaining := make(map[int]RerankItem)

	// 初始化候选集
	for i, item := range items {
		remaining[i] = item
	}

	// 归一化分数
	maxScore := items[0].Score
	minScore := items[len(items)-1].Score
	scoreRange := maxScore - minScore
	if scoreRange == 0 {
		scoreRange = 1
	}

	// 贪心选择
	for len(selected) < n && len(remaining) > 0 {
		bestIdx := -1
		bestMMR := math.Inf(-1)

		for idx, item := range remaining {
			// 归一化相关性分数
			relevance := (item.Score - minScore) / scoreRange

			// 计算与已选物品的最大相似度
			maxSim := 0.0
			for _, sel := range selected {
				sim := d.similarity(item, sel)
				if sim > maxSim {
					maxSim = sim
				}
			}

			// MMR分数
			mmr := d.lambda*relevance - (1-d.lambda)*maxSim

			if mmr > bestMMR {
				bestMMR = mmr
				bestIdx = idx
			}
		}

		if bestIdx >= 0 {
			selected = append(selected, remaining[bestIdx])
			delete(remaining, bestIdx)
		}
	}

	return selected, nil
}

// similarity 计算物品相似度
// 基于类别和城市的简单相似度
func (d *MMRDiversifier) similarity(a, b RerankItem) float64 {
	sim := 0.0

	// 同类别
	if catA, ok := a.Features["category_id"]; ok {
		if catB, ok := b.Features["category_id"]; ok {
			if catA == catB {
				sim += 0.5
			}
		}
	}

	// 同城市
	if cityA, ok := a.Features["city_id"]; ok {
		if cityB, ok := b.Features["city_id"]; ok {
			if cityA == cityB {
				sim += 0.3
			}
		}
	}

	// 价格相近
	if priceA, ok := a.Features["price"]; ok {
		if priceB, ok := b.Features["price"]; ok {
			priceDiff := math.Abs(priceA - priceB)
			if priceDiff < 100 {
				sim += 0.2 * (1 - priceDiff/100)
			}
		}
	}

	return sim
}

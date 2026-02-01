package recall

import (
	"context"
	"sort"
	"strconv"

	"github.com/redis/go-redis/v9"
)

// ItemCFRecall ItemCF召回器
// 基于物品的协同过滤: 找到用户历史交互物品的相似物品
type ItemCFRecall struct {
	rdb    *redis.Client
	config ItemCFConfig
}

// NewItemCFRecall 创建ItemCF召回器
func NewItemCFRecall(rdb *redis.Client, cfg ItemCFConfig) *ItemCFRecall {
	return &ItemCFRecall{
		rdb:    rdb,
		config: cfg,
	}
}

func (r *ItemCFRecall) Name() string {
	return "item_cf"
}

func (r *ItemCFRecall) Weight() float64 {
	return r.config.Weight
}

// Recall 执行ItemCF召回
// 算法: 遍历用户历史物品, 查询每个物品的相似物品, 汇总打分
func (r *ItemCFRecall) Recall(ctx context.Context, req *RecallRequest) ([]RecallItem, error) {
	if len(req.History) == 0 {
		return nil, nil
	}

	// 用于汇总相似物品的分数
	scoreMap := make(map[int64]float64)
	excludeSet := make(map[int64]struct{})
	for _, id := range req.Exclude {
		excludeSet[id] = struct{}{}
	}
	for _, id := range req.History {
		excludeSet[id] = struct{}{}
	}

	// 只取最近N个历史物品
	historyLimit := 50
	if len(req.History) > historyLimit {
		req.History = req.History[len(req.History)-historyLimit:]
	}

	// 使用Pipeline批量查询
	pipe := r.rdb.Pipeline()
	cmds := make([]*redis.ZSliceCmd, len(req.History))

	for i, itemID := range req.History {
		key := "cf:item:" + strconv.FormatInt(itemID, 10) + ":similar"
		cmds[i] = pipe.ZRevRangeWithScores(ctx, key, 0, int64(r.config.TopK/len(req.History)))
	}

	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return nil, err
	}

	// 汇总分数
	for _, cmd := range cmds {
		result, err := cmd.Result()
		if err != nil && err != redis.Nil {
			continue
		}

		for _, z := range result {
			itemID, err := strconv.ParseInt(z.Member.(string), 10, 64)
			if err != nil {
				continue
			}

			// 排除已交互物品
			if _, excluded := excludeSet[itemID]; excluded {
				continue
			}

			scoreMap[itemID] += z.Score
		}
	}

	// 转换为列表并排序
	items := make([]RecallItem, 0, len(scoreMap))
	for itemID, score := range scoreMap {
		items = append(items, RecallItem{
			EventID: itemID,
			Score:   score,
			Source:  r.Name(),
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Score > items[j].Score
	})

	// 截取TopK
	if len(items) > r.config.TopK {
		items = items[:r.config.TopK]
	}

	return items, nil
}

// GetSimilarItems 获取单个物品的相似物品
func (r *ItemCFRecall) GetSimilarItems(ctx context.Context, itemID int64, count int) ([]RecallItem, error) {
	key := "cf:item:" + strconv.FormatInt(itemID, 10) + ":similar"

	result, err := r.rdb.ZRevRangeWithScores(ctx, key, 0, int64(count-1)).Result()
	if err != nil && err != redis.Nil {
		return nil, err
	}

	items := make([]RecallItem, 0, len(result))
	for _, z := range result {
		id, _ := strconv.ParseInt(z.Member.(string), 10, 64)
		items = append(items, RecallItem{
			EventID: id,
			Score:   z.Score,
			Source:  r.Name(),
		})
	}

	return items, nil
}

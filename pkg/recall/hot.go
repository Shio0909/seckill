package recall

import (
	"context"
	"strconv"

	"github.com/redis/go-redis/v9"
)

// HotRecall 热门召回器
// 基于热度分的召回，支持全站热门和城市热门
type HotRecall struct {
	rdb    *redis.Client
	config HotConfig
}

// NewHotRecall 创建热门召回器
func NewHotRecall(rdb *redis.Client, cfg HotConfig) *HotRecall {
	return &HotRecall{
		rdb:    rdb,
		config: cfg,
	}
}

func (r *HotRecall) Name() string {
	return "hot"
}

func (r *HotRecall) Weight() float64 {
	return r.config.Weight
}

// Recall 执行热门召回
// 优先返回城市热门,不足时用全站热门补充
func (r *HotRecall) Recall(ctx context.Context, req *RecallRequest) ([]RecallItem, error) {
	excludeSet := make(map[int64]struct{})
	for _, id := range req.Exclude {
		excludeSet[id] = struct{}{}
	}
	for _, id := range req.History {
		excludeSet[id] = struct{}{}
	}

	var items []RecallItem

	// 1. 先查城市热门
	if req.City != "" {
		cityItems, err := r.getHotList(ctx, "hot:city:"+req.City, r.config.TopK, excludeSet)
		if err == nil {
			items = append(items, cityItems...)
		}
	}

	// 2. 不足时用全站热门补充
	if len(items) < r.config.TopK {
		// 更新排除集合
		for _, item := range items {
			excludeSet[item.EventID] = struct{}{}
		}

		remain := r.config.TopK - len(items)
		globalItems, err := r.getHotList(ctx, "hot:all", remain, excludeSet)
		if err == nil {
			items = append(items, globalItems...)
		}
	}

	return items, nil
}

// getHotList 获取热门列表
func (r *HotRecall) getHotList(ctx context.Context, key string, count int, exclude map[int64]struct{}) ([]RecallItem, error) {
	// 多取一些以应对过滤
	fetchCount := count * 2
	if fetchCount > 500 {
		fetchCount = 500
	}

	result, err := r.rdb.ZRevRangeWithScores(ctx, key, 0, int64(fetchCount-1)).Result()
	if err != nil && err != redis.Nil {
		return nil, err
	}

	items := make([]RecallItem, 0, count)
	for _, z := range result {
		if len(items) >= count {
			break
		}

		itemID, err := strconv.ParseInt(z.Member.(string), 10, 64)
		if err != nil {
			continue
		}

		if _, excluded := exclude[itemID]; excluded {
			continue
		}

		items = append(items, RecallItem{
			EventID: itemID,
			Score:   z.Score,
			Source:  r.Name(),
		})
	}

	return items, nil
}

// GetCategoryHot 获取分类热门
func (r *HotRecall) GetCategoryHot(ctx context.Context, categoryID int64, count int) ([]RecallItem, error) {
	key := "hot:category:" + strconv.FormatInt(categoryID, 10)
	return r.getHotList(ctx, key, count, nil)
}

// UpdateHotScore 更新热度分(实时行为触发)
func (r *HotRecall) UpdateHotScore(ctx context.Context, eventID int64, city string, delta float64) error {
	pipe := r.rdb.Pipeline()

	// 更新全站热门
	pipe.ZIncrBy(ctx, "hot:all", delta, strconv.FormatInt(eventID, 10))

	// 更新城市热门
	if city != "" {
		pipe.ZIncrBy(ctx, "hot:city:"+city, delta, strconv.FormatInt(eventID, 10))
	}

	_, err := pipe.Exec(ctx)
	return err
}

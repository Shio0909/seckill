package rerank

import (
	"context"
	"sort"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// BusinessFilter 业务规则过滤器
type BusinessFilter struct {
	rdb              *redis.Client
	cooldownHours    int // 已购买物品冷却期(小时)
	maxSameCategorry int // 同类别最大数量
}

// NewBusinessFilter 创建业务过滤器
func NewBusinessFilter(rdb *redis.Client) *BusinessFilter {
	return &BusinessFilter{
		rdb:              rdb,
		cooldownHours:    168, // 7天
		maxSameCategorry: 5,
	}
}

// Rerank 执行业务规则过滤
func (f *BusinessFilter) Rerank(ctx context.Context, items []RerankItem, req *RerankRequest) ([]RerankItem, error) {
	// 1. 获取用户黑名单
	blacklist := make(map[int64]struct{})
	for _, id := range req.Blacklist {
		blacklist[id] = struct{}{}
	}

	// 2. 获取用户已购买物品(冷却期内)
	purchased, _ := f.getPurchasedItems(ctx, req.UserID)
	for _, id := range purchased {
		blacklist[id] = struct{}{}
	}

	// 3. 过滤已看过的物品
	for _, id := range req.History {
		blacklist[id] = struct{}{}
	}

	// 4. 获取置顶物品
	pinnedItems, _ := f.getPinnedItems(ctx, req.City)

	// 5. 获取降权物品
	demotedItems, _ := f.getDemotedItems(ctx)

	// 6. 执行过滤和调整
	result := make([]RerankItem, 0, len(items))
	categoryCount := make(map[float64]int)

	for _, item := range items {
		// 黑名单过滤
		if _, blocked := blacklist[item.EventID]; blocked {
			continue
		}

		// 降权处理
		if _, demoted := demotedItems[item.EventID]; demoted {
			item.Score *= 0.5
		}

		// 同类别限制
		if catID, ok := item.Features["category_id"]; ok {
			if categoryCount[catID] >= f.maxSameCategorry {
				continue
			}
			categoryCount[catID]++
		}

		result = append(result, item)
	}

	// 7. 置顶物品插入
	for i, pinnedID := range pinnedItems {
		// 在结果中找到置顶物品
		for j, item := range result {
			if item.EventID == pinnedID {
				// 移动到第i位
				result = append(result[:i], append([]RerankItem{item}, append(result[i:j], result[j+1:]...)...)...)
				break
			}
		}
	}

	// 8. 重新排序(置顶之外的按分数排)
	pinnedCount := len(pinnedItems)
	if pinnedCount < len(result) {
		nonPinned := result[pinnedCount:]
		sort.Slice(nonPinned, func(i, j int) bool {
			return nonPinned[i].Score > nonPinned[j].Score
		})
	}

	return result, nil
}

// getPurchasedItems 获取用户已购买物品
func (f *BusinessFilter) getPurchasedItems(ctx context.Context, userID int64) ([]int64, error) {
	key := "user:" + strconv.FormatInt(userID, 10) + ":purchased"

	// 使用ZRANGEBYSCORE获取冷却期内的购买
	minTime := time.Now().Add(-time.Duration(f.cooldownHours) * time.Hour).Unix()

	result, err := f.rdb.ZRangeByScore(ctx, key, &redis.ZRangeBy{
		Min: strconv.FormatInt(minTime, 10),
		Max: "+inf",
	}).Result()

	if err != nil && err != redis.Nil {
		return nil, err
	}

	items := make([]int64, 0, len(result))
	for _, s := range result {
		id, _ := strconv.ParseInt(s, 10, 64)
		items = append(items, id)
	}

	return items, nil
}

// getPinnedItems 获取置顶物品
func (f *BusinessFilter) getPinnedItems(ctx context.Context, city string) ([]int64, error) {
	key := "ops:pinned"
	if city != "" {
		key = "ops:pinned:" + city
	}

	result, err := f.rdb.ZRevRange(ctx, key, 0, 2).Result() // 最多3个置顶
	if err != nil && err != redis.Nil {
		return nil, err
	}

	items := make([]int64, 0, len(result))
	for _, s := range result {
		id, _ := strconv.ParseInt(s, 10, 64)
		items = append(items, id)
	}

	return items, nil
}

// getDemotedItems 获取降权物品
func (f *BusinessFilter) getDemotedItems(ctx context.Context) (map[int64]struct{}, error) {
	result, err := f.rdb.SMembers(ctx, "ops:demoted").Result()
	if err != nil && err != redis.Nil {
		return nil, err
	}

	items := make(map[int64]struct{}, len(result))
	for _, s := range result {
		id, _ := strconv.ParseInt(s, 10, 64)
		items[id] = struct{}{}
	}

	return items, nil
}

// RecordPurchase 记录购买
func (f *BusinessFilter) RecordPurchase(ctx context.Context, userID, eventID int64) error {
	key := "user:" + strconv.FormatInt(userID, 10) + ":purchased"
	return f.rdb.ZAdd(ctx, key, redis.Z{
		Score:  float64(time.Now().Unix()),
		Member: strconv.FormatInt(eventID, 10),
	}).Err()
}

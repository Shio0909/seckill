package recall

import (
	"context"
	"sort"
	"strconv"
	"sync"

	"github.com/redis/go-redis/v9"
)

// Merger 多路召回合并器
type Merger struct {
	recallers []Recaller
	rdb       *redis.Client
}

// NewMerger 创建合并器
func NewMerger(rdb *redis.Client, recallers ...Recaller) *Merger {
	return &Merger{
		recallers: recallers,
		rdb:       rdb,
	}
}

// MergeResult 合并结果
type MergeResult struct {
	Items       []RecallItem
	SourceStats map[string]int // 各召回源的贡献数量
}

// Merge 执行多路召回并合并
// 使用goroutine并发执行各召回器
func (m *Merger) Merge(ctx context.Context, req *RecallRequest) (*MergeResult, error) {
	// 1. 获取用户历史行为
	if len(req.History) == 0 {
		history, err := m.getUserHistory(ctx, req.UserID)
		if err == nil {
			req.History = history
		}
	}

	// 2. 判断冷启动级别
	coldLevel := m.getColdStartLevel(len(req.History))

	// 3. 根据冷启动级别选择召回器
	activeRecallers := m.selectRecallers(coldLevel)

	// 4. 并发执行召回
	var wg sync.WaitGroup
	resultChan := make(chan []RecallItem, len(activeRecallers))
	errChan := make(chan error, len(activeRecallers))

	for _, recaller := range activeRecallers {
		wg.Add(1)
		go func(r Recaller) {
			defer wg.Done()

			items, err := r.Recall(ctx, req)
			if err != nil {
				errChan <- err
				return
			}

			// 应用权重
			for i := range items {
				items[i].Score *= r.Weight()
			}

			resultChan <- items
		}(recaller)
	}

	// 等待所有召回完成
	go func() {
		wg.Wait()
		close(resultChan)
		close(errChan)
	}()

	// 5. 合并结果
	scoreMap := make(map[int64]*RecallItem)
	sourceStats := make(map[string]int)

	for items := range resultChan {
		for _, item := range items {
			sourceStats[item.Source]++

			if existing, ok := scoreMap[item.EventID]; ok {
				// 同一物品多路召回,分数累加
				existing.Score += item.Score
				existing.Source += "+" + item.Source
			} else {
				itemCopy := item
				scoreMap[item.EventID] = &itemCopy
			}
		}
	}

	// 6. 转换为列表并排序
	items := make([]RecallItem, 0, len(scoreMap))
	for _, item := range scoreMap {
		items = append(items, *item)
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Score > items[j].Score
	})

	// 7. 截取数量
	if len(items) > req.Count {
		items = items[:req.Count]
	}

	return &MergeResult{
		Items:       items,
		SourceStats: sourceStats,
	}, nil
}

// getColdStartLevel 获取冷启动级别
// 0: 极冷(行为<5) 1: 冷(5-20) 2: 正常(>20)
func (m *Merger) getColdStartLevel(historyCount int) int {
	if historyCount < 5 {
		return 0
	}
	if historyCount < 20 {
		return 1
	}
	return 2
}

// selectRecallers 根据冷启动级别选择召回器
func (m *Merger) selectRecallers(coldLevel int) []Recaller {
	var selected []Recaller

	for _, r := range m.recallers {
		name := r.Name()

		switch coldLevel {
		case 0: // 极冷: 只用热门+标签
			if name == "hot" || name == "tag" {
				selected = append(selected, r)
			}
		case 1: // 冷: 热门+ItemCF+标签
			if name == "hot" || name == "item_cf" || name == "tag" {
				selected = append(selected, r)
			}
		default: // 正常: 全部启用
			selected = append(selected, r)
		}
	}

	return selected
}

// getUserHistory 获取用户历史行为
func (m *Merger) getUserHistory(ctx context.Context, userID int64) ([]int64, error) {
	key := "user:" + strconv.FormatInt(userID, 10) + ":history"

	result, err := m.rdb.LRange(ctx, key, -100, -1).Result()
	if err != nil && err != redis.Nil {
		return nil, err
	}

	history := make([]int64, 0, len(result))
	for _, s := range result {
		id, err := strconv.ParseInt(s, 10, 64)
		if err == nil {
			history = append(history, id)
		}
	}

	return history, nil
}

// RecordBehavior 记录用户行为
func (m *Merger) RecordBehavior(ctx context.Context, userID, eventID int64) error {
	key := "user:" + strconv.FormatInt(userID, 10) + ":history"

	pipe := m.rdb.Pipeline()
	pipe.RPush(ctx, key, strconv.FormatInt(eventID, 10))
	pipe.LTrim(ctx, key, -200, -1) // 只保留最近200条

	_, err := pipe.Exec(ctx)
	return err
}

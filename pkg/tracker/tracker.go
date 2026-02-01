package tracker

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
)

// EventType 用户行为类型
type EventType string

const (
	EventView     EventType = "view"     // 浏览
	EventClick    EventType = "click"    // 点击
	EventAddCart  EventType = "add_cart" // 加入收藏/购物车
	EventPurchase EventType = "purchase" // 购买
	EventShare    EventType = "share"    // 分享
	EventSearch   EventType = "search"   // 搜索
)

// UserEvent 用户行为事件
type UserEvent struct {
	UserID    int64             `json:"user_id"`
	EventType EventType         `json:"event_type"`
	ItemID    int64             `json:"item_id"`    // 活动/商品ID
	ItemType  string            `json:"item_type"`  // product, category, search
	Timestamp int64             `json:"timestamp"`
	SessionID string            `json:"session_id,omitempty"`
	Source    string            `json:"source,omitempty"`    // 来源：home, search, recommend
	Position  int               `json:"position,omitempty"`  // 曝光位置
	Extra     map[string]string `json:"extra,omitempty"`     // 扩展字段
}

// TrackerConfig 埋点配置
type TrackerConfig struct {
	RedisAddr     string
	RedisPassword string
	RedisDB       int
	BatchSize     int           // 批量写入大小
	FlushInterval time.Duration // 刷新间隔
	BufferSize    int           // 缓冲区大小
}

// Tracker 用户行为埋点追踪器
type Tracker struct {
	config  TrackerConfig
	rdb     *redis.Client
	buffer  chan *UserEvent
	wg      sync.WaitGroup
	ctx     context.Context
	cancel  context.CancelFunc
	metrics *TrackerMetrics
}

// TrackerMetrics 埋点指标
type TrackerMetrics struct {
	mu            sync.RWMutex
	TotalEvents   int64
	EventsByType  map[EventType]int64
	DroppedEvents int64
	FlushCount    int64
	LastFlushTime time.Time
}

// NewTracker 创建追踪器
func NewTracker(config TrackerConfig) (*Tracker, error) {
	if config.BatchSize == 0 {
		config.BatchSize = 100
	}
	if config.FlushInterval == 0 {
		config.FlushInterval = 5 * time.Second
	}
	if config.BufferSize == 0 {
		config.BufferSize = 10000
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     config.RedisAddr,
		Password: config.RedisPassword,
		DB:       config.RedisDB,
	})

	// 测试连接
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis连接失败: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	t := &Tracker{
		config: config,
		rdb:    rdb,
		buffer: make(chan *UserEvent, config.BufferSize),
		ctx:    ctx,
		cancel: cancel,
		metrics: &TrackerMetrics{
			EventsByType: make(map[EventType]int64),
		},
	}

	// 启动后台刷新goroutine
	t.wg.Add(1)
	go t.backgroundFlush()

	return t, nil
}

// Track 记录用户行为
func (t *Tracker) Track(event *UserEvent) error {
	if event.Timestamp == 0 {
		event.Timestamp = time.Now().UnixMilli()
	}

	select {
	case t.buffer <- event:
		t.metrics.mu.Lock()
		t.metrics.TotalEvents++
		t.metrics.EventsByType[event.EventType]++
		t.metrics.mu.Unlock()
		return nil
	default:
		t.metrics.mu.Lock()
		t.metrics.DroppedEvents++
		t.metrics.mu.Unlock()
		return fmt.Errorf("埋点缓冲区已满，事件被丢弃")
	}
}

// TrackView 记录浏览行为
func (t *Tracker) TrackView(userID, itemID int64, source string) error {
	return t.Track(&UserEvent{
		UserID:    userID,
		EventType: EventView,
		ItemID:    itemID,
		ItemType:  "product",
		Source:    source,
	})
}

// TrackClick 记录点击行为
func (t *Tracker) TrackClick(userID, itemID int64, source string, position int) error {
	return t.Track(&UserEvent{
		UserID:    userID,
		EventType: EventClick,
		ItemID:    itemID,
		ItemType:  "product",
		Source:    source,
		Position:  position,
	})
}

// TrackPurchase 记录购买行为
func (t *Tracker) TrackPurchase(userID, itemID int64, orderID string) error {
	return t.Track(&UserEvent{
		UserID:    userID,
		EventType: EventPurchase,
		ItemID:    itemID,
		ItemType:  "product",
		Extra: map[string]string{
			"order_id": orderID,
		},
	})
}

// TrackSearch 记录搜索行为
func (t *Tracker) TrackSearch(userID int64, keyword string, resultCount int) error {
	return t.Track(&UserEvent{
		UserID:    userID,
		EventType: EventSearch,
		ItemType:  "search",
		Extra: map[string]string{
			"keyword":      keyword,
			"result_count": fmt.Sprintf("%d", resultCount),
		},
	})
}

// backgroundFlush 后台定期刷新
func (t *Tracker) backgroundFlush() {
	defer t.wg.Done()

	ticker := time.NewTicker(t.config.FlushInterval)
	defer ticker.Stop()

	batch := make([]*UserEvent, 0, t.config.BatchSize)

	for {
		select {
		case <-t.ctx.Done():
			// 关闭时刷新剩余数据
			t.flush(batch)
			return

		case event := <-t.buffer:
			batch = append(batch, event)
			if len(batch) >= t.config.BatchSize {
				t.flush(batch)
				batch = batch[:0]
			}

		case <-ticker.C:
			if len(batch) > 0 {
				t.flush(batch)
				batch = batch[:0]
			}
		}
	}
}

// flush 批量写入Redis
func (t *Tracker) flush(events []*UserEvent) {
	if len(events) == 0 {
		return
	}

	ctx := context.Background()
	pipe := t.rdb.Pipeline()

	for _, event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			continue
		}

		// 1. 写入用户行为流（List）
		userKey := fmt.Sprintf("user:events:%d", event.UserID)
		pipe.LPush(ctx, userKey, data)
		pipe.LTrim(ctx, userKey, 0, 999) // 保留最近1000条
		pipe.Expire(ctx, userKey, 30*24*time.Hour)

		// 2. 写入物品行为统计（ZSet）
		if event.ItemID > 0 {
			itemKey := fmt.Sprintf("item:stats:%d", event.ItemID)
			pipe.HIncrBy(ctx, itemKey, string(event.EventType), 1)
			pipe.Expire(ctx, itemKey, 30*24*time.Hour)

			// 3. 更新物品热度（用于热门召回）
			if event.EventType == EventClick || event.EventType == EventPurchase {
				weight := 1.0
				if event.EventType == EventPurchase {
					weight = 5.0
				}
				pipe.ZIncrBy(ctx, "hot:realtime", weight, fmt.Sprintf("%d", event.ItemID))
			}
		}

		// 4. 写入日志流（用于离线分析）
		logKey := fmt.Sprintf("log:events:%s", time.Now().Format("2006-01-02"))
		pipe.RPush(ctx, logKey, data)
		pipe.Expire(ctx, logKey, 7*24*time.Hour)
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		// 日志记录错误，但不阻塞
		fmt.Printf("flush事件失败: %v\n", err)
	}

	t.metrics.mu.Lock()
	t.metrics.FlushCount++
	t.metrics.LastFlushTime = time.Now()
	t.metrics.mu.Unlock()
}

// GetUserHistory 获取用户历史行为
func (t *Tracker) GetUserHistory(ctx context.Context, userID int64, limit int) ([]*UserEvent, error) {
	key := fmt.Sprintf("user:events:%d", userID)
	data, err := t.rdb.LRange(ctx, key, 0, int64(limit-1)).Result()
	if err != nil {
		return nil, err
	}

	events := make([]*UserEvent, 0, len(data))
	for _, d := range data {
		var event UserEvent
		if err := json.Unmarshal([]byte(d), &event); err != nil {
			continue
		}
		events = append(events, &event)
	}

	return events, nil
}

// GetUserClickedItems 获取用户点击过的物品ID列表
func (t *Tracker) GetUserClickedItems(ctx context.Context, userID int64, limit int) ([]int64, error) {
	events, err := t.GetUserHistory(ctx, userID, 500)
	if err != nil {
		return nil, err
	}

	seen := make(map[int64]bool)
	items := make([]int64, 0, limit)

	for _, event := range events {
		if event.EventType == EventClick && event.ItemID > 0 {
			if !seen[event.ItemID] {
				seen[event.ItemID] = true
				items = append(items, event.ItemID)
				if len(items) >= limit {
					break
				}
			}
		}
	}

	return items, nil
}

// GetUserPurchasedItems 获取用户购买过的物品ID列表
func (t *Tracker) GetUserPurchasedItems(ctx context.Context, userID int64) ([]int64, error) {
	events, err := t.GetUserHistory(ctx, userID, 500)
	if err != nil {
		return nil, err
	}

	seen := make(map[int64]bool)
	items := make([]int64, 0)

	for _, event := range events {
		if event.EventType == EventPurchase && event.ItemID > 0 {
			if !seen[event.ItemID] {
				seen[event.ItemID] = true
				items = append(items, event.ItemID)
			}
		}
	}

	return items, nil
}

// GetItemStats 获取物品统计
func (t *Tracker) GetItemStats(ctx context.Context, itemID int64) (map[string]int64, error) {
	key := fmt.Sprintf("item:stats:%d", itemID)
	result, err := t.rdb.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, err
	}

	stats := make(map[string]int64)
	for k, v := range result {
		var count int64
		fmt.Sscanf(v, "%d", &count)
		stats[k] = count
	}

	return stats, nil
}

// GetRealtimeHot 获取实时热门物品
func (t *Tracker) GetRealtimeHot(ctx context.Context, limit int) ([]int64, error) {
	result, err := t.rdb.ZRevRangeWithScores(ctx, "hot:realtime", 0, int64(limit-1)).Result()
	if err != nil {
		return nil, err
	}

	items := make([]int64, 0, len(result))
	for _, z := range result {
		var itemID int64
		fmt.Sscanf(z.Member.(string), "%d", &itemID)
		items = append(items, itemID)
	}

	return items, nil
}

// GetMetrics 获取埋点指标
func (t *Tracker) GetMetrics() TrackerMetrics {
	t.metrics.mu.RLock()
	defer t.metrics.mu.RUnlock()

	// 复制一份返回
	m := TrackerMetrics{
		TotalEvents:   t.metrics.TotalEvents,
		DroppedEvents: t.metrics.DroppedEvents,
		FlushCount:    t.metrics.FlushCount,
		LastFlushTime: t.metrics.LastFlushTime,
		EventsByType:  make(map[EventType]int64),
	}
	for k, v := range t.metrics.EventsByType {
		m.EventsByType[k] = v
	}

	return m
}

// Close 关闭追踪器
func (t *Tracker) Close() error {
	t.cancel()
	t.wg.Wait()
	return t.rdb.Close()
}

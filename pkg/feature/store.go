package feature

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Store 特征存储
type Store struct {
	rdb *redis.Client
}

// NewStore 创建特征存储
func NewStore(rdb *redis.Client) *Store {
	return &Store{rdb: rdb}
}

// UserFeatures 用户特征
type UserFeatures struct {
	UserID         int64   `json:"user_id"`
	AgeGroup       int     `json:"age_group"`       // 年龄段
	Gender         int     `json:"gender"`          // 性别
	CityID         int     `json:"city_id"`         // 城市ID
	BehaviorCount  int     `json:"behavior_count"`  // 总行为数
	ViewCount      int     `json:"view_count"`      // 浏览数
	ClickCount     int     `json:"click_count"`     // 点击数
	OrderCount     int     `json:"order_count"`     // 购买数
	PreferCategory int     `json:"prefer_category"` // 偏好类别
	AvgPrice       float64 `json:"avg_price"`       // 平均消费
	LastActiveTime int64   `json:"last_active"`     // 最后活跃时间
}

// EventFeatures 活动特征
type EventFeatures struct {
	EventID     int64   `json:"event_id"`
	CategoryID  int     `json:"category_id"`  // 类别ID
	Price       float64 `json:"price"`        // 价格
	CityID      int     `json:"city_id"`      // 城市ID
	VenueID     int     `json:"venue_id"`     // 场馆ID
	HotScore    float64 `json:"hot_score"`    // 热度分
	TotalViews  int     `json:"total_views"`  // 总浏览数
	TotalOrders int     `json:"total_orders"` // 总订单数
	DaysToStart int     `json:"days_to_start"` // 距离开始天数
	TimeSlot    int     `json:"time_slot"`    // 时间段 (0早/1午/2晚)
	Weekday     int     `json:"weekday"`      // 星期几
}

// GetUserFeatures 获取用户特征
func (s *Store) GetUserFeatures(ctx context.Context, userID int64) (*UserFeatures, error) {
	key := "user:feature:" + strconv.FormatInt(userID, 10)

	result, err := s.rdb.HGetAll(ctx, key).Result()
	if err != nil && err != redis.Nil {
		return nil, err
	}

	if len(result) == 0 {
		return nil, nil
	}

	return &UserFeatures{
		UserID:         userID,
		AgeGroup:       parseInt(result["age_group"]),
		Gender:         parseInt(result["gender"]),
		CityID:         parseInt(result["city_id"]),
		BehaviorCount:  parseInt(result["behavior_count"]),
		ViewCount:      parseInt(result["view_count"]),
		ClickCount:     parseInt(result["click_count"]),
		OrderCount:     parseInt(result["order_count"]),
		PreferCategory: parseInt(result["prefer_category"]),
		AvgPrice:       parseFloat(result["avg_price"]),
		LastActiveTime: parseInt64(result["last_active"]),
	}, nil
}

// GetEventFeatures 获取活动特征
func (s *Store) GetEventFeatures(ctx context.Context, eventID int64) (*EventFeatures, error) {
	key := "event:feature:" + strconv.FormatInt(eventID, 10)

	result, err := s.rdb.HGetAll(ctx, key).Result()
	if err != nil && err != redis.Nil {
		return nil, err
	}

	if len(result) == 0 {
		return nil, nil
	}

	return &EventFeatures{
		EventID:     eventID,
		CategoryID:  parseInt(result["category_id"]),
		Price:       parseFloat(result["price"]),
		CityID:      parseInt(result["city_id"]),
		VenueID:     parseInt(result["venue_id"]),
		HotScore:    parseFloat(result["hot_score"]),
		TotalViews:  parseInt(result["total_views"]),
		TotalOrders: parseInt(result["total_orders"]),
		DaysToStart: parseInt(result["days_to_start"]),
		TimeSlot:    parseInt(result["time_slot"]),
		Weekday:     parseInt(result["weekday"]),
	}, nil
}

// BatchGetEventFeatures 批量获取活动特征
func (s *Store) BatchGetEventFeatures(ctx context.Context, eventIDs []int64) (map[int64]*EventFeatures, error) {
	if len(eventIDs) == 0 {
		return nil, nil
	}

	result := make(map[int64]*EventFeatures, len(eventIDs))
	var mu sync.Mutex
	var wg sync.WaitGroup

	// 使用Pipeline批量查询
	pipe := s.rdb.Pipeline()
	cmds := make(map[int64]*redis.MapStringStringCmd)

	for _, eventID := range eventIDs {
		key := "event:feature:" + strconv.FormatInt(eventID, 10)
		cmds[eventID] = pipe.HGetAll(ctx, key)
	}

	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return nil, err
	}

	// 解析结果
	for eventID, cmd := range cmds {
		wg.Add(1)
		go func(id int64, c *redis.MapStringStringCmd) {
			defer wg.Done()

			data, _ := c.Result()
			if len(data) == 0 {
				return
			}

			feat := &EventFeatures{
				EventID:     id,
				CategoryID:  parseInt(data["category_id"]),
				Price:       parseFloat(data["price"]),
				CityID:      parseInt(data["city_id"]),
				VenueID:     parseInt(data["venue_id"]),
				HotScore:    parseFloat(data["hot_score"]),
				TotalViews:  parseInt(data["total_views"]),
				TotalOrders: parseInt(data["total_orders"]),
				DaysToStart: parseInt(data["days_to_start"]),
				TimeSlot:    parseInt(data["time_slot"]),
				Weekday:     parseInt(data["weekday"]),
			}

			mu.Lock()
			result[id] = feat
			mu.Unlock()
		}(eventID, cmd)
	}

	wg.Wait()
	return result, nil
}

// RecordBehavior 记录用户行为并更新实时特征
func (s *Store) RecordBehavior(ctx context.Context, userID, eventID int64, behaviorType string) error {
	userKey := "user:feature:" + strconv.FormatInt(userID, 10)
	eventKey := "event:feature:" + strconv.FormatInt(eventID, 10)

	pipe := s.rdb.Pipeline()

	// 更新用户特征
	pipe.HIncrBy(ctx, userKey, "behavior_count", 1)
	pipe.HSet(ctx, userKey, "last_active", time.Now().Unix())

	switch behaviorType {
	case "view":
		pipe.HIncrBy(ctx, userKey, "view_count", 1)
		pipe.HIncrBy(ctx, eventKey, "total_views", 1)
	case "click":
		pipe.HIncrBy(ctx, userKey, "click_count", 1)
	case "order":
		pipe.HIncrBy(ctx, userKey, "order_count", 1)
		pipe.HIncrBy(ctx, eventKey, "total_orders", 1)
	}

	_, err := pipe.Exec(ctx)
	return err
}

// BuildFeatureVector 构建特征向量(用于排序模型)
func (s *Store) BuildFeatureVector(user *UserFeatures, event *EventFeatures) []float64 {
	if user == nil || event == nil {
		return make([]float64, 16) // 返回零向量
	}

	// 用户特征 (8维)
	features := []float64{
		float64(user.AgeGroup),
		float64(user.Gender),
		float64(user.CityID),
		float64(user.BehaviorCount),
		float64(user.ViewCount),
		float64(user.ClickCount),
		float64(user.OrderCount),
		float64(user.PreferCategory),
	}

	// 活动特征 (6维)
	features = append(features,
		float64(event.CategoryID),
		event.Price,
		float64(event.CityID),
		event.HotScore,
		float64(event.TotalViews),
		float64(event.TotalOrders),
	)

	// 交叉特征 (2维)
	cityMatch := 0.0
	if user.CityID == event.CityID {
		cityMatch = 1.0
	}
	categoryMatch := 0.0
	if user.PreferCategory == event.CategoryID {
		categoryMatch = 1.0
	}
	features = append(features, cityMatch, categoryMatch)

	return features
}

// Helper functions
func parseInt(s string) int {
	v, _ := strconv.Atoi(s)
	return v
}

func parseInt64(s string) int64 {
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}

func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

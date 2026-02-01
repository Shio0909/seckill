package tracker

import (
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// TrackingMiddleware 埋点中间件
func TrackingMiddleware(t *Tracker) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 生成或获取SessionID
		sessionID := c.GetHeader("X-Session-ID")
		if sessionID == "" {
			sessionID = uuid.New().String()
		}
		c.Set("session_id", sessionID)

		// 记录请求开始时间
		startTime := time.Now()
		c.Set("request_start", startTime)

		// 处理请求
		c.Next()

		// 请求处理完成后，记录埋点
		go recordRequestEvent(t, c, sessionID, startTime)
	}
}

// recordRequestEvent 记录请求事件
func recordRequestEvent(t *Tracker, c *gin.Context, sessionID string, startTime time.Time) {
	// 获取用户ID（从JWT中获取）
	userIDVal, exists := c.Get("user_id")
	if !exists {
		return // 未登录用户不记录
	}

	userID, ok := userIDVal.(int64)
	if !ok {
		return
	}

	path := c.Request.URL.Path
	method := c.Request.Method

	// 根据路由自动识别行为类型
	event := parseRouteToEvent(path, method, userID, c)
	if event == nil {
		return
	}

	event.SessionID = sessionID
	event.Extra["duration_ms"] = strconv.FormatInt(time.Since(startTime).Milliseconds(), 10)
	event.Extra["status_code"] = strconv.Itoa(c.Writer.Status())

	t.Track(event)
}

// parseRouteToEvent 根据路由解析行为类型
func parseRouteToEvent(path, method string, userID int64, c *gin.Context) *UserEvent {
	event := &UserEvent{
		UserID:    userID,
		Timestamp: time.Now().UnixMilli(),
		Extra:     make(map[string]string),
	}

	// 商品/活动相关
	if strings.HasPrefix(path, "/api/v1/products/") || strings.HasPrefix(path, "/api/v1/events/") {
		if method == "GET" {
			// 获取商品详情 = 浏览行为
			parts := strings.Split(path, "/")
			if len(parts) >= 5 {
				if itemID, err := strconv.ParseInt(parts[4], 10, 64); err == nil {
					event.EventType = EventView
					event.ItemID = itemID
					event.ItemType = "product"
					event.Source = c.Query("source")
					return event
				}
			}
		}
	}

	// 推荐点击
	if strings.HasPrefix(path, "/api/v1/recommend/click") {
		itemIDStr := c.Query("item_id")
		positionStr := c.Query("position")
		if itemID, err := strconv.ParseInt(itemIDStr, 10, 64); err == nil {
			position, _ := strconv.Atoi(positionStr)
			event.EventType = EventClick
			event.ItemID = itemID
			event.ItemType = "product"
			event.Source = "recommend"
			event.Position = position
			return event
		}
	}

	// 搜索
	if strings.HasPrefix(path, "/api/v1/search") && method == "GET" {
		keyword := c.Query("keyword")
		if keyword != "" {
			event.EventType = EventSearch
			event.ItemType = "search"
			event.Extra["keyword"] = keyword
			return event
		}
	}

	// 订单相关
	if strings.HasPrefix(path, "/api/v1/orders") && method == "POST" {
		// 创建订单 = 购买行为
		// 从请求体中获取商品ID（需要在handler中设置）
		itemIDVal, exists := c.Get("order_item_id")
		if exists {
			if itemID, ok := itemIDVal.(int64); ok {
				event.EventType = EventPurchase
				event.ItemID = itemID
				event.ItemType = "product"
				return event
			}
		}
	}

	// 收藏/加购物车
	if strings.HasPrefix(path, "/api/v1/favorites") && method == "POST" {
		itemIDStr := c.PostForm("item_id")
		if itemIDStr == "" {
			itemIDStr = c.Query("item_id")
		}
		if itemID, err := strconv.ParseInt(itemIDStr, 10, 64); err == nil {
			event.EventType = EventAddCart
			event.ItemID = itemID
			event.ItemType = "product"
			return event
		}
	}

	// 分享
	if strings.HasPrefix(path, "/api/v1/share") && method == "POST" {
		itemIDStr := c.PostForm("item_id")
		if itemID, err := strconv.ParseInt(itemIDStr, 10, 64); err == nil {
			event.EventType = EventShare
			event.ItemID = itemID
			event.ItemType = "product"
			event.Extra["platform"] = c.PostForm("platform")
			return event
		}
	}

	return nil
}

// ClientTrackHandler 客户端主动上报埋点的Handler
func ClientTrackHandler(t *Tracker) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			EventType string            `json:"event_type" binding:"required"`
			ItemID    int64             `json:"item_id"`
			ItemType  string            `json:"item_type"`
			Source    string            `json:"source"`
			Position  int               `json:"position"`
			Extra     map[string]string `json:"extra"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, gin.H{"code": 400, "msg": "参数错误"})
			return
		}

		// 获取用户ID
		userIDVal, exists := c.Get("user_id")
		if !exists {
			c.JSON(401, gin.H{"code": 401, "msg": "未登录"})
			return
		}

		userID, ok := userIDVal.(int64)
		if !ok {
			c.JSON(500, gin.H{"code": 500, "msg": "用户ID错误"})
			return
		}

		sessionID, _ := c.Get("session_id")

		event := &UserEvent{
			UserID:    userID,
			EventType: EventType(req.EventType),
			ItemID:    req.ItemID,
			ItemType:  req.ItemType,
			Source:    req.Source,
			Position:  req.Position,
			Extra:     req.Extra,
			Timestamp: time.Now().UnixMilli(),
		}

		if sid, ok := sessionID.(string); ok {
			event.SessionID = sid
		}

		if err := t.Track(event); err != nil {
			c.JSON(500, gin.H{"code": 500, "msg": "记录失败"})
			return
		}

		c.JSON(200, gin.H{"code": 0, "msg": "success"})
	}
}

// ExposureTrackHandler 曝光埋点Handler（批量上报）
func ExposureTrackHandler(t *Tracker) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Items []struct {
				ItemID   int64  `json:"item_id"`
				Position int    `json:"position"`
				Source   string `json:"source"`
			} `json:"items" binding:"required"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, gin.H{"code": 400, "msg": "参数错误"})
			return
		}

		// 获取用户ID
		userIDVal, exists := c.Get("user_id")
		if !exists {
			c.JSON(401, gin.H{"code": 401, "msg": "未登录"})
			return
		}

		userID, ok := userIDVal.(int64)
		if !ok {
			c.JSON(500, gin.H{"code": 500, "msg": "用户ID错误"})
			return
		}

		sessionID, _ := c.Get("session_id")
		sid := ""
		if s, ok := sessionID.(string); ok {
			sid = s
		}

		timestamp := time.Now().UnixMilli()

		for _, item := range req.Items {
			event := &UserEvent{
				UserID:    userID,
				EventType: EventView,
				ItemID:    item.ItemID,
				ItemType:  "product",
				Source:    item.Source,
				Position:  item.Position,
				SessionID: sid,
				Timestamp: timestamp,
			}
			t.Track(event)
		}

		c.JSON(200, gin.H{"code": 0, "msg": "success", "count": len(req.Items)})
	}
}

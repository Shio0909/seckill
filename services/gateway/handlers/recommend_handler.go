package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"seckill/pkg/e"
	"seckill/pkg/recommend"
	"seckill/pkg/response"
)

// RecommendHandler 推荐处理器
// 架构: Go负责召回+重排, Python只负责排序
type RecommendHandler struct {
	service *recommend.Service
}

// NewRecommendHandler 创建推荐处理器
func NewRecommendHandler(service *recommend.Service) *RecommendHandler {
	return &RecommendHandler{service: service}
}

// GetRecommendationsRequest 获取推荐请求
type GetRecommendationsRequest struct {
	Scene      string `form:"scene" binding:"required,oneof=home category similar"`
	Count      int    `form:"count" binding:"omitempty,min=1,max=100"`
	City       string `form:"city" binding:"omitempty"`
	CategoryID int64  `form:"category_id" binding:"omitempty"`
	Debug      bool   `form:"debug" binding:"omitempty"`
}

// GetRecommendations 获取推荐列表
// @Summary 获取个性化推荐
// @Description 根据用户画像返回个性化推荐的活动列表
// @Tags 推荐
// @Accept json
// @Produce json
// @Param scene query string true "场景: home-首页推荐, category-分类推荐, similar-相似推荐"
// @Param count query int false "数量 (默认20)"
// @Param city query string false "城市"
// @Param category_id query int false "类别ID (category场景必填)"
// @Param debug query bool false "是否返回调试信息"
// @Success 200 {object} response.Response{data=recommend.RecommendResponse}
// @Router /api/v1/recommend [get]
func (h *RecommendHandler) GetRecommendations(c *gin.Context) {
	var req GetRecommendationsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, http.StatusBadRequest, e.InvalidParams, err.Error())
		return
	}

	// 获取用户ID
	userID, _ := c.Get("user_id")
	uid, ok := userID.(int64)
	if !ok {
		uid = 0 // 未登录用户
	}

	// 默认数量
	count := req.Count
	if count == 0 {
		count = 20
	}

	// 调用推荐服务 (Go内部完成召回+重排, 只调用Python做排序)
	result, err := h.service.Recommend(c.Request.Context(), &recommend.RecommendRequest{
		UserID:     uid,
		Scene:      req.Scene,
		City:       req.City,
		CategoryID: req.CategoryID,
		Count:      count,
		Debug:      req.Debug,
	})
	if err != nil {
		response.Error(c, http.StatusInternalServerError, e.ServerError, "获取推荐失败")
		return
	}

	response.Success(c, result)
}

// GetSimilarEvents 获取相似活动
// @Summary 获取相似活动
// @Description 根据指定活动返回相似的活动列表
// @Tags 推荐
// @Accept json
// @Produce json
// @Param event_id path int true "活动ID"
// @Param count query int false "数量 (默认10)"
// @Success 200 {object} response.Response{data=recommend.RecommendResponse}
// @Router /api/v1/events/{event_id}/similar [get]
func (h *RecommendHandler) GetSimilarEvents(c *gin.Context) {
	// 获取活动ID
	eventIDStr := c.Param("event_id")
	eventID, err := strconv.ParseInt(eventIDStr, 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, e.InvalidParams, "无效的活动ID")
		return
	}

	// 获取数量
	countStr := c.DefaultQuery("count", "10")
	count, _ := strconv.Atoi(countStr)
	if count <= 0 || count > 50 {
		count = 10
	}

	// TODO: 实现相似活动接口
	_ = eventID
	_ = count

	response.Success(c, nil)
}

// RecordBehavior 记录用户行为
// @Summary 记录用户行为
// @Description 记录用户的浏览、点击、购买等行为
// @Tags 推荐
// @Accept json
// @Produce json
// @Param body body RecordBehaviorRequest true "行为数据"
// @Success 200 {object} response.Response
// @Router /api/v1/behavior [post]
func (h *RecommendHandler) RecordBehavior(c *gin.Context) {
	var req RecordBehaviorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, e.InvalidParams, err.Error())
		return
	}

	// 获取用户ID
	userID, _ := c.Get("user_id")
	uid, ok := userID.(int64)
	if !ok {
		response.Error(c, http.StatusUnauthorized, e.Unauthorized, "用户未登录")
		return
	}

	// 记录行为 (Go内部处理)
	err := h.service.RecordBehavior(c.Request.Context(), uid, req.EventID, req.Behavior)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, e.ServerError, "记录行为失败")
		return
	}

	response.Success(c, nil)
}

// RecordBehaviorRequest 记录行为请求
type RecordBehaviorRequest struct {
	EventID  int64  `json:"event_id" binding:"required"`
	Behavior string `json:"behavior" binding:"required,oneof=view click order"`
}

// RegisterRoutes 注册路由
func (h *RecommendHandler) RegisterRoutes(r *gin.RouterGroup) {
	rec := r.Group("/recommend")
	{
		rec.GET("", h.GetRecommendations)
	}

	events := r.Group("/events")
	{
		events.GET("/:event_id/similar", h.GetSimilarEvents)
	}

	behavior := r.Group("/behavior")
	{
		behavior.POST("", h.RecordBehavior)
	}
}

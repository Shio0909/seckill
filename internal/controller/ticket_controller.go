package controller

import (
	"net/http"
	"seckill/internal/service"
	"seckill/pkg/logger"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// TicketController 负责处理票务抢购相关请求
// 适用于演唱会、体育赛事、话剧等活动的票务购买
type TicketController struct{}

// NewTicketController 创建TicketController
func NewTicketController() *TicketController {
	return &TicketController{}
}

// Purchase 处理抢票请求
// @Summary 用户抢票下单
// @Description 发起抢票请求，扣减票务库存
// @Tags 票务模块
// @Accept x-www-form-urlencoded
// @Produce json
// @Security Bearer
// @Param event_id formData int true "活动ID"
// @Param session_id formData int false "场次ID（可选）"
// @Param seat_type formData string false "座位类型：vip, premium, standard, economy"
// @Success 200 {object} map[string]interface{} "{"code":0,"msg":"抢票成功"}"
// @Failure 401 {object} map[string]interface{} "{"code":401,"msg":"请先登录"}"
// @Failure 400 {object} map[string]interface{} "{"code":400,"msg":"无效的活动ID"}"
// @Router /api/v1/ticket/purchase [post]
func (tc *TicketController) Purchase(c *gin.Context) {
	// 1. 获取用户ID
	uid, exists := c.Get("uid")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code": 401,
			"msg":  "请先登录",
		})
		return
	}
	userID := uid.(int)

	// 2. 获取活动ID
	eventIDStr := c.PostForm("event_id")
	eventID, err := strconv.Atoi(eventIDStr)
	if err != nil {
		logger.Log.Error("获取活动ID失败", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "无效的活动ID",
		})
		return
	}

	// 3. 可选：获取场次和座位类型
	sessionIDStr := c.PostForm("session_id")
	seatType := c.DefaultPostForm("seat_type", "standard")

	var result bool
	var message string

	if sessionIDStr != "" {
		// 带场次的抢票
		sessionID, err := strconv.Atoi(sessionIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code": 400,
				"msg":  "无效的场次ID",
			})
			return
		}
		result, message = service.TicketPurchaseWithSession(userID, eventID, sessionID, seatType)
	} else {
		// 简单抢票（无场次）
		result, message = service.TicketPurchase(userID, eventID)
	}

	// 4. 返回结果
	if result {
		c.JSON(http.StatusOK, gin.H{
			"code":    0,
			"success": true,
			"msg":     message,
		})
	} else {
		c.JSON(http.StatusOK, gin.H{
			"code":    1,
			"success": false,
			"msg":     message,
		})
	}
}

// CheckAvailability 检查票务可用性
// @Summary 检查票务库存
// @Description 获取指定活动场次的各档位票务库存
// @Tags 票务模块
// @Accept json
// @Produce json
// @Param event_id query int true "活动ID"
// @Param session_id query int true "场次ID"
// @Success 200 {object} map[string]interface{} "{"code":0,"data":{"vip":100,"premium":200}}"
// @Router /api/v1/ticket/availability [get]
func (tc *TicketController) CheckAvailability(c *gin.Context) {
	eventIDStr := c.Query("event_id")
	sessionIDStr := c.Query("session_id")

	eventID, err := strconv.Atoi(eventIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "无效的活动ID",
		})
		return
	}

	sessionID, err := strconv.Atoi(sessionIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "无效的场次ID",
		})
		return
	}

	availability, err := service.CheckTicketAvailability(eventID, sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "获取库存失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"msg":  "success",
		"data": availability,
	})
}

// CheckPurchaseStatus 检查用户购票状态
// @Summary 检查用户是否已购票
// @Description 检查用户是否已经购买过指定活动的票
// @Tags 票务模块
// @Accept json
// @Produce json
// @Security Bearer
// @Param event_id query int true "活动ID"
// @Success 200 {object} map[string]interface{} "{"code":0,"data":{"purchased":true}}"
// @Router /api/v1/ticket/status [get]
func (tc *TicketController) CheckPurchaseStatus(c *gin.Context) {
	uid, exists := c.Get("uid")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code": 401,
			"msg":  "请先登录",
		})
		return
	}
	userID := uid.(int)

	eventIDStr := c.Query("event_id")
	eventID, err := strconv.Atoi(eventIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "无效的活动ID",
		})
		return
	}

	purchased, err := service.GetUserPurchaseHistory(userID, eventID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "查询失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"msg":  "success",
		"data": gin.H{
			"purchased": purchased,
		},
	})
}

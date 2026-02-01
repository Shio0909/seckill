package controller

import (
	"strconv"

	"seckill/internal/service"
	"seckill/pkg/e"
	"seckill/pkg/response"

	"github.com/gin-gonic/gin"
)

// 在 auth 中间件中，JWT 解析后会将用户信息存入 gin.Context
// 在 Controller 中通过 ctx.Get("userID") 获取
//
// 常见模式：
// userID, exists := ctx.Get("userID")
// if !exists {
//     response.Fail(ctx, e.Unauthorized, "未登录")
//     return
// }
// uid := userID.(uint)

type OrderController struct {
	orderService *service.OrderService
}

func NewOrderController() *OrderController {
	return &OrderController{
		orderService: &service.OrderService{},
	}
}

// List 获取订单列表
// @Summary 获取订单列表
// @Description 获取当前用户的订单列表
// @Tags 订单管理
// @Accept json
// @Produce json
// @Security Bearer
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(10)
// @Param status query int false "订单状态"
// @Success 200 {object} response.Response{data=service.OrderListResponse}
// @Router /api/v1/orders [get]
func (c *OrderController) List(ctx *gin.Context) {
	var req service.OrderListRequest
	if err := ctx.ShouldBindQuery(&req); err != nil {
		response.FailWithMsg(ctx, e.INVALID_PARAMS, err.Error())
		return
	}

	// 从 JWT 中获取用户ID
	userID, exists := ctx.Get("userID")
	if !exists {
		response.Fail(ctx, e.ERROR_AUTH_CHECK_TOKEN)
		return
	}
	req.UserID = userID.(uint)

	result, err := c.orderService.List(&req)
	if err != nil {
		response.FailWithMsg(ctx, e.ERROR, err.Error())
		return
	}

	response.Success(ctx, result)
}

// Get 获取订单详情
// @Summary 获取订单详情
// @Description 根据ID获取订单详情
// @Tags 订单管理
// @Accept json
// @Produce json
// @Security Bearer
// @Param id path int true "订单ID"
// @Success 200 {object} response.Response{data=service.OrderDetailResponse}
// @Router /api/v1/orders/{id} [get]
func (c *OrderController) Get(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil {
		response.FailWithMsg(ctx, e.INVALID_PARAMS, "无效的订单ID")
		return
	}

	// 从 JWT 中获取用户ID
	userID, exists := ctx.Get("userID")
	if !exists {
		response.Fail(ctx, e.ERROR_AUTH_CHECK_TOKEN)
		return
	}

	order, err := c.orderService.GetByID(uint(id), userID.(uint))
	if err != nil {
		response.FailWithMsg(ctx, e.ERROR, err.Error())
		return
	}

	response.Success(ctx, order)
}

// GetByOrderNo 根据订单号获取订单
// @Summary 根据订单号获取订单
// @Description 根据订单号获取订单详情
// @Tags 订单管理
// @Accept json
// @Produce json
// @Security Bearer
// @Param order_no path string true "订单号"
// @Success 200 {object} response.Response{data=service.OrderDetailResponse}
// @Router /api/v1/orders/no/{order_no} [get]
func (c *OrderController) GetByOrderNo(ctx *gin.Context) {
	orderNo := ctx.Param("order_no")
	if orderNo == "" {
		response.FailWithMsg(ctx, e.INVALID_PARAMS, "订单号不能为空")
		return
	}

	// 从 JWT 中获取用户ID
	userID, exists := ctx.Get("userID")
	if !exists {
		response.Fail(ctx, e.ERROR_AUTH_CHECK_TOKEN)
		return
	}

	order, err := c.orderService.GetByOrderNo(orderNo, userID.(uint))
	if err != nil {
		response.FailWithMsg(ctx, e.ERROR, err.Error())
		return
	}

	response.Success(ctx, order)
}

// Cancel 取消订单
// @Summary 取消订单
// @Description 取消待支付的订单
// @Tags 订单管理
// @Accept json
// @Produce json
// @Security Bearer
// @Param id path int true "订单ID"
// @Success 200 {object} response.Response
// @Router /api/v1/orders/{id}/cancel [post]
func (c *OrderController) Cancel(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil {
		response.FailWithMsg(ctx, e.INVALID_PARAMS, "无效的订单ID")
		return
	}

	// 从 JWT 中获取用户ID
	userID, exists := ctx.Get("userID")
	if !exists {
		response.Fail(ctx, e.ERROR_AUTH_CHECK_TOKEN)
		return
	}

	if err := c.orderService.Cancel(uint(id), userID.(uint)); err != nil {
		response.FailWithMsg(ctx, e.ERROR, err.Error())
		return
	}

	response.Success(ctx, gin.H{"message": "订单已取消"})
}

// Pay 支付订单（模拟）
// @Summary 支付订单
// @Description 模拟支付订单
// @Tags 订单管理
// @Accept json
// @Produce json
// @Security Bearer
// @Param id path int true "订单ID"
// @Success 200 {object} response.Response
// @Router /api/v1/orders/{id}/pay [post]
func (c *OrderController) Pay(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil {
		response.FailWithMsg(ctx, e.INVALID_PARAMS, "无效的订单ID")
		return
	}

	// 从 JWT 中获取用户ID
	userID, exists := ctx.Get("userID")
	if !exists {
		response.Fail(ctx, e.ERROR_AUTH_CHECK_TOKEN)
		return
	}

	if err := c.orderService.Pay(uint(id), userID.(uint)); err != nil {
		response.FailWithMsg(ctx, e.ERROR, err.Error())
		return
	}

	response.Success(ctx, gin.H{"message": "支付成功"})
}

// Stats 订单统计（管理员）
// @Summary 订单统计
// @Description 获取订单统计信息（管理员专用）
// @Tags 订单管理
// @Accept json
// @Produce json
// @Security Bearer
// @Success 200 {object} response.Response{data=service.OrderStats}
// @Router /api/v1/admin/orders/stats [get]
func (c *OrderController) Stats(ctx *gin.Context) {
	// TODO: 检查管理员权限

	stats, err := c.orderService.GetStats()
	if err != nil {
		response.FailWithMsg(ctx, e.ERROR, err.Error())
		return
	}

	response.Success(ctx, stats)
}

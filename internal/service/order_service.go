package service

import (
	"errors"

	"seckill/internal/model"
	"seckill/pkg/database"

	"gorm.io/gorm"
)

// 订单是秒杀系统的核心实体之一，订单服务主要负责：
// 1. 订单查询（列表、详情）
// 2. 订单状态管理（创建、支付、取消）
// 3. 订单统计
//
// 注意：秒杀下单的逻辑在 seckill_service.go 中
// 这里只处理订单的查询和管理

// OrderService 订单服务
type OrderService struct{}

// OrderListRequest 订单列表请求
type OrderListRequest struct {
	Page     int  `form:"page" binding:"min=1"`
	PageSize int  `form:"page_size" binding:"min=1,max=100"`
	Status   *int `form:"status"`
	UserID   uint `form:"-"` // 从 JWT 中获取，不从参数绑定
}

// OrderListResponse 订单列表响应
type OrderListResponse struct {
	Total int64         `json:"total"`
	List  []model.Order `json:"list"`
}

// OrderDetailResponse 订单详情响应（包含商品信息）
type OrderDetailResponse struct {
	Order   model.Order   `json:"order"`
	Product model.Product `json:"product"`
}

// List 获取订单列表
func (s *OrderService) List(req *OrderListRequest) (*OrderListResponse, error) {
	// 设置默认值
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 10
	}

	var orders []model.Order
	var total int64

	query := database.DB.Model(&model.Order{})

	// 按用户过滤（普通用户只能看自己的订单）
	if req.UserID > 0 {
		query = query.Where("user_id = ?", req.UserID)
	}

	// 按状态过滤
	if req.Status != nil {
		query = query.Where("status = ?", *req.Status)
	}

	// 获取总数
	query.Count(&total)

	// 分页查询（预加载商品信息）
	offset := (req.Page - 1) * req.PageSize
	err := query.
		Preload("Product"). // 预加载关联的商品
		Offset(offset).
		Limit(req.PageSize).
		Order("id DESC").
		Find(&orders).Error

	if err != nil {
		return nil, err
	}

	return &OrderListResponse{
		Total: total,
		List:  orders,
	}, nil
}

// GetByID 根据ID获取订单详情
func (s *OrderService) GetByID(id uint, userID uint) (*OrderDetailResponse, error) {
	var order model.Order

	query := database.DB.Model(&model.Order{}).Where("id = ?", id)

	// 普通用户只能查看自己的订单
	if userID > 0 {
		query = query.Where("user_id = ?", userID)
	}

	err := query.First(&order).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("订单不存在")
		}
		return nil, err
	}

	// 获取商品信息
	var product model.Product
	if err := database.DB.First(&product, order.ProductID).Error; err != nil {
		return nil, err
	}

	return &OrderDetailResponse{
		Order:   order,
		Product: product,
	}, nil
}

// GetByOrderNo 根据订单号获取订单
func (s *OrderService) GetByOrderNo(orderNo string, userID uint) (*OrderDetailResponse, error) {
	var order model.Order

	query := database.DB.Model(&model.Order{}).Where("order_no = ?", orderNo)

	// 普通用户只能查看自己的订单
	if userID > 0 {
		query = query.Where("user_id = ?", userID)
	}

	err := query.First(&order).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("订单不存在")
		}
		return nil, err
	}

	// 获取商品信息
	var product model.Product
	if err := database.DB.First(&product, order.ProductID).Error; err != nil {
		return nil, err
	}

	return &OrderDetailResponse{
		Order:   order,
		Product: product,
	}, nil
}

// 订单状态流转：
//
// 待支付(0) --支付--> 已支付(1) --发货--> 已发货(2) --收货--> 已完成(3)
//     |
//     |--取消--> 已取消(-1)
//     |--超时--> 已关闭(-2)
//
// 状态机设计原则：
// 1. 状态只能单向流转（不能回退）
// 2. 每次状态变更都要记录日志
// 3. 状态变更要加锁（防止并发问题）

// 订单状态常量
const (
	OrderStatusPending   = 0  // 待支付
	OrderStatusPaid      = 1  // 已支付
	OrderStatusShipped   = 2  // 已发货
	OrderStatusCompleted = 3  // 已完成
	OrderStatusCancelled = -1 // 已取消
	OrderStatusClosed    = -2 // 已关闭（超时）
)

// Cancel 取消订单
func (s *OrderService) Cancel(id uint, userID uint) error {
	var order model.Order

	// 查询订单（带行锁）
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		// FOR UPDATE 加行锁
		if err := tx.Set("gorm:query_option", "FOR UPDATE").
			Where("id = ? AND user_id = ?", id, userID).
			First(&order).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("订单不存在")
			}
			return err
		}

		// 检查订单状态（只有待支付的订单可以取消）
		if order.Status != OrderStatusPending {
			return errors.New("当前订单状态不允许取消")
		}

		// 更新订单状态
		if err := tx.Model(&order).Update("status", OrderStatusCancelled).Error; err != nil {
			return err
		}

		// TODO: 恢复库存（发送消息到队列处理）

		return nil
	})

	return err
}

// Pay 支付订单（模拟）
func (s *OrderService) Pay(id uint, userID uint) error {
	var order model.Order

	err := database.DB.Transaction(func(tx *gorm.DB) error {
		// 查询订单（带行锁）
		if err := tx.Set("gorm:query_option", "FOR UPDATE").
			Where("id = ? AND user_id = ?", id, userID).
			First(&order).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("订单不存在")
			}
			return err
		}

		// 检查订单状态
		if order.Status != OrderStatusPending {
			return errors.New("当前订单状态不允许支付")
		}

		// 更新订单状态
		if err := tx.Model(&order).Update("status", OrderStatusPaid).Error; err != nil {
			return err
		}

		return nil
	})

	return err
}

// OrderStats 订单统计
type OrderStats struct {
	TotalOrders     int64 `json:"total_orders"`
	PendingOrders   int64 `json:"pending_orders"`
	PaidOrders      int64 `json:"paid_orders"`
	CompletedOrders int64 `json:"completed_orders"`
	CancelledOrders int64 `json:"cancelled_orders"`
}

// GetStats 获取订单统计（管理员用）
func (s *OrderService) GetStats() (*OrderStats, error) {
	var stats OrderStats

	// 总订单数
	database.DB.Model(&model.Order{}).Count(&stats.TotalOrders)

	// 各状态订单数
	database.DB.Model(&model.Order{}).Where("status = ?", OrderStatusPending).Count(&stats.PendingOrders)
	database.DB.Model(&model.Order{}).Where("status = ?", OrderStatusPaid).Count(&stats.PaidOrders)
	database.DB.Model(&model.Order{}).Where("status = ?", OrderStatusCompleted).Count(&stats.CompletedOrders)
	database.DB.Model(&model.Order{}).Where("status = ?", OrderStatusCancelled).Count(&stats.CancelledOrders)

	return &stats, nil
}

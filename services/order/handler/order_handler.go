package handler

import (
	"context"
	"fmt"
	"time"

	"seckill/internal/model"
	"seckill/pkg/database"
	"seckill/pkg/logger"
	"seckill/pkg/snowflake"
	pb "seckill/proto/order"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ========================================================================
// 【重点学习】Order 微服务 gRPC Handler 实现
// ========================================================================
// Order 服务负责：
// 1. 订单创建（由秒杀消费者调用）
// 2. 订单查询
// 3. 订单状态管理（支付、取消）
//
// 📝 简历亮点：
// - 分布式订单号生成（雪花算法）
// - 订单状态机设计
// - 幂等性保证
//
// 🔥 面试高频：
// Q: 为什么订单号要用雪花算法而不是数据库自增ID？
// A: 1. 分布式环境下自增ID需要协调，性能差
//    2. 雪花算法可以本地生成，无网络开销
//    3. 订单号有时间信息，便于排序和分表
//    4. 自增ID容易被猜测，有安全隐患
//
// Q: 订单如何保证幂等性？
// A: 1. 联合唯一索引 (user_id, product_id)
//    2. 创建前先查询是否存在
//    3. 使用幂等性 Key（如请求ID）
// ========================================================================

// OrderHandler 订单服务处理器
type OrderHandler struct {
	pb.UnimplementedOrderServiceServer
}

// NewOrderHandler 创建订单处理器
func NewOrderHandler() *OrderHandler {
	return &OrderHandler{}
}

// CreateOrder 创建订单
// 【重点】这个方法通常由秒杀消费者异步调用
func (h *OrderHandler) CreateOrder(ctx context.Context, req *pb.CreateOrderRequest) (*pb.OrderInfo, error) {
	if req.UserId <= 0 || req.ProductId <= 0 {
		return nil, status.Error(codes.InvalidArgument, "参数无效")
	}

	// 【重点】幂等性检查：检查是否已存在订单
	var existingOrder model.Order
	err := database.DB.Where("user_id = ? AND product_id = ?", req.UserId, req.ProductId).
		First(&existingOrder).Error
	if err == nil {
		// 订单已存在，返回已有订单（幂等）
		logger.Log.Info("订单已存在，返回已有订单",
			zap.Int64("user_id", req.UserId),
			zap.Int64("product_id", req.ProductId))
		return modelToProto(&existingOrder), nil
	}

	// 生成订单号（雪花算法）
	orderNum := snowflake.GenerateID()

	// 创建订单
	order := model.Order{
		UserID:    uint(req.UserId),
		ProductID: uint(req.ProductId),
		OrderNum:  orderNum,
		Status:    0, // 未支付
	}

	if err := database.DB.Create(&order).Error; err != nil {
		logger.Log.Error("创建订单失败", zap.Error(err))
		return nil, status.Error(codes.Internal, "创建订单失败")
	}

	logger.Log.Info("订单创建成功",
		zap.Uint("order_id", order.ID),
		zap.String("order_num", order.OrderNum),
		zap.Int64("user_id", req.UserId))

	return modelToProto(&order), nil
}

// GetOrder 获取订单详情
func (h *OrderHandler) GetOrder(ctx context.Context, req *pb.GetOrderRequest) (*pb.OrderInfo, error) {
	if req.OrderId <= 0 {
		return nil, status.Error(codes.InvalidArgument, "订单ID无效")
	}

	var order model.Order
	// 预加载关联数据
	if err := database.DB.Preload("Product").Preload("User").
		First(&order, req.OrderId).Error; err != nil {
		return nil, status.Error(codes.NotFound, "订单不存在")
	}

	return modelToProto(&order), nil
}

// GetOrderByNo 根据订单号获取订单
func (h *OrderHandler) GetOrderByNo(ctx context.Context, req *pb.GetOrderByNoRequest) (*pb.OrderInfo, error) {
	if req.OrderNo == "" {
		return nil, status.Error(codes.InvalidArgument, "订单号不能为空")
	}

	var order model.Order
	if err := database.DB.Preload("Product").Preload("User").
		Where("order_num = ?", req.OrderNo).First(&order).Error; err != nil {
		return nil, status.Error(codes.NotFound, "订单不存在")
	}

	return modelToProto(&order), nil
}

// ListOrders 获取用户订单列表
func (h *OrderHandler) ListOrders(ctx context.Context, req *pb.ListOrdersRequest) (*pb.ListOrdersResponse, error) {
	if req.UserId <= 0 {
		return nil, status.Error(codes.InvalidArgument, "用户ID无效")
	}

	var orders []model.Order
	var total int64

	// 分页参数
	page := int(req.Page)
	pageSize := int(req.PageSize)
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 10
	}
	offset := (page - 1) * pageSize

	// 查询条件
	query := database.DB.Model(&model.Order{}).Where("user_id = ?", req.UserId)

	// 状态过滤
	if req.Status != nil {
		query = query.Where("status = ?", *req.Status)
	}

	// 查询总数
	query.Count(&total)

	// 查询订单列表
	if err := query.Preload("Product").Offset(offset).Limit(pageSize).
		Order("created_at DESC").Find(&orders).Error; err != nil {
		logger.Log.Error("查询订单列表失败", zap.Error(err))
		return nil, status.Error(codes.Internal, "查询订单列表失败")
	}

	// 转换
	var pbOrders []*pb.OrderInfo
	for _, o := range orders {
		pbOrders = append(pbOrders, modelToProto(&o))
	}

	return &pb.ListOrdersResponse{
		Orders: pbOrders,
		Total:  total,
	}, nil
}

// CancelOrder 取消订单
// 【重点】订单状态机：只有未支付的订单可以取消
func (h *OrderHandler) CancelOrder(ctx context.Context, req *pb.CancelOrderRequest) (*pb.CancelOrderResponse, error) {
	if req.OrderId <= 0 {
		return nil, status.Error(codes.InvalidArgument, "订单ID无效")
	}

	var order model.Order
	if err := database.DB.First(&order, req.OrderId).Error; err != nil {
		return nil, status.Error(codes.NotFound, "订单不存在")
	}

	// 状态检查
	if order.Status != 0 {
		return &pb.CancelOrderResponse{
			Success: false,
			Message: "只有未支付的订单可以取消",
		}, nil
	}

	// 更新状态
	if err := database.DB.Model(&order).Update("status", 2).Error; err != nil {
		logger.Log.Error("取消订单失败", zap.Error(err))
		return nil, status.Error(codes.Internal, "取消订单失败")
	}

	logger.Log.Info("订单已取消", zap.Uint("order_id", order.ID))

	// TODO: 这里应该回滚库存（发消息到 Product 服务）

	return &pb.CancelOrderResponse{
		Success: true,
		Message: "订单已取消",
	}, nil
}

// PayOrder 支付订单
// 【重点】实际项目中这里会调用支付网关
func (h *OrderHandler) PayOrder(ctx context.Context, req *pb.PayOrderRequest) (*pb.PayOrderResponse, error) {
	if req.OrderId <= 0 {
		return nil, status.Error(codes.InvalidArgument, "订单ID无效")
	}

	var order model.Order
	if err := database.DB.First(&order, req.OrderId).Error; err != nil {
		return nil, status.Error(codes.NotFound, "订单不存在")
	}

	// 状态检查
	if order.Status != 0 {
		return &pb.PayOrderResponse{
			Success: false,
			Message: "订单状态不允许支付",
		}, nil
	}

	// 模拟支付（实际应调用支付网关）
	payTime := time.Now()

	// 更新状态
	if err := database.DB.Model(&order).Updates(map[string]interface{}{
		"status": 1,
	}).Error; err != nil {
		logger.Log.Error("支付订单失败", zap.Error(err))
		return nil, status.Error(codes.Internal, "支付失败")
	}

	logger.Log.Info("订单支付成功",
		zap.Uint("order_id", order.ID),
		zap.String("payment_method", req.PaymentMethod))

	return &pb.PayOrderResponse{
		Success:   true,
		Message:   "支付成功",
		PaidAt:    timestamppb.New(payTime),
		PaymentId: fmt.Sprintf("PAY_%d_%d", order.ID, payTime.Unix()),
	}, nil
}

// modelToProto 转换为 protobuf
func modelToProto(o *model.Order) *pb.OrderInfo {
	info := &pb.OrderInfo{
		OrderId:   int64(o.ID),
		OrderNo:   o.OrderNum,
		UserId:    int64(o.UserID),
		ProductId: int64(o.ProductID),
		Status:    pb.OrderStatus(o.Status),
		CreatedAt: timestamppb.New(o.CreatedAt),
		UpdatedAt: timestamppb.New(o.UpdatedAt),
	}

	// 如果有商品信息
	if o.Product.ID > 0 {
		info.ProductName = o.Product.Name
		info.ProductPrice = o.Product.SeckillPrice
	}

	return info
}

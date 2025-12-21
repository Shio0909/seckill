package handler

import (
	"context"
	"fmt"
	"time"

	"seckill/internal/model"
	"seckill/pkg/database"
	"seckill/pkg/logger"
	"seckill/pkg/redis"
	pb "seckill/proto/product"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ========================================================================
// 【重点学习】Product 微服务 gRPC Handler 实现
// ========================================================================
// Product 服务负责：
// 1. 商品 CRUD 操作
// 2. 库存管理（预热到 Redis）
// 3. 库存扣减（配合秒杀）
//
// 📝 简历亮点：
// - 库存预热机制设计
// - Redis 缓存与数据库同步
// - 库存扣减原子操作
//
// 🔥 面试高频：
// Q: 为什么要做库存预热？
// A: 秒杀开始时并发量极高，直接查数据库会打崩 DB。
//    预热将库存加载到 Redis，利用内存操作的高性能承接流量。
//
// Q: Redis 库存和数据库库存如何保持一致？
// A: 1. 预热时从 DB 加载到 Redis
//    2. 扣减时先扣 Redis（Lua 原子操作）
//    3. 异步消息队列更新数据库
//    4. 最终一致性，允许短暂不一致
// ========================================================================

// ProductHandler 商品服务处理器
type ProductHandler struct {
	pb.UnimplementedProductServiceServer
}

// NewProductHandler 创建商品处理器
func NewProductHandler() *ProductHandler {
	return &ProductHandler{}
}

// ListProducts 获取商品列表
func (h *ProductHandler) ListProducts(ctx context.Context, req *pb.ListProductsRequest) (*pb.ListProductsResponse, error) {
	var products []model.Product
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

	// 查询总数
	database.DB.Model(&model.Product{}).Count(&total)

	// 查询商品列表
	if err := database.DB.Offset(offset).Limit(pageSize).Find(&products).Error; err != nil {
		logger.Log.Error("查询商品列表失败", zap.Error(err))
		return nil, status.Error(codes.Internal, "查询商品列表失败")
	}

	// 转换为 protobuf 格式
	var pbProducts []*pb.ProductInfo
	for _, p := range products {
		pbProducts = append(pbProducts, modelToProto(&p))
	}

	return &pb.ListProductsResponse{
		Products: pbProducts,
		Total:    total,
	}, nil
}

// GetProduct 获取单个商品
func (h *ProductHandler) GetProduct(ctx context.Context, req *pb.GetProductRequest) (*pb.ProductInfo, error) {
	if req.ProductId <= 0 {
		return nil, status.Error(codes.InvalidArgument, "商品ID无效")
	}

	var product model.Product
	if err := database.DB.First(&product, req.ProductId).Error; err != nil {
		return nil, status.Error(codes.NotFound, "商品不存在")
	}

	return modelToProto(&product), nil
}

// GetStock 获取库存（优先从 Redis 获取）
// 【重点】缓存优先策略
func (h *ProductHandler) GetStock(ctx context.Context, req *pb.GetStockRequest) (*pb.GetStockResponse, error) {
	if req.ProductId <= 0 {
		return nil, status.Error(codes.InvalidArgument, "商品ID无效")
	}

	// 优先从 Redis 获取
	stockKey := fmt.Sprintf("seckill:stock:%d", req.ProductId)
	stock, err := redis.Client.Get(ctx, stockKey).Int()
	if err == nil {
		return &pb.GetStockResponse{
			ProductId: req.ProductId,
			Stock:     int32(stock),
			Source:    "redis",
		}, nil
	}

	// Redis 没有，从数据库获取
	var product model.Product
	if err := database.DB.Select("stock").First(&product, req.ProductId).Error; err != nil {
		return nil, status.Error(codes.NotFound, "商品不存在")
	}

	return &pb.GetStockResponse{
		ProductId: req.ProductId,
		Stock:     int32(product.Stock),
		Source:    "database",
	}, nil
}

// DeductStock 扣减库存
// 【重点】这个方法通常由消费者调用，用于数据库层面的库存扣减
func (h *ProductHandler) DeductStock(ctx context.Context, req *pb.DeductStockRequest) (*pb.DeductStockResponse, error) {
	if req.ProductId <= 0 || req.Quantity <= 0 {
		return nil, status.Error(codes.InvalidArgument, "参数无效")
	}

	// 使用数据库事务扣减库存
	// 【重点】乐观锁实现：通过 WHERE stock >= quantity 防止超卖
	result := database.DB.Model(&model.Product{}).
		Where("id = ? AND stock >= ?", req.ProductId, req.Quantity).
		UpdateColumn("stock", database.DB.Raw("stock - ?", req.Quantity))

	if result.Error != nil {
		logger.Log.Error("扣减库存失败", zap.Error(result.Error))
		return nil, status.Error(codes.Internal, "扣减库存失败")
	}

	if result.RowsAffected == 0 {
		return &pb.DeductStockResponse{
			Success: false,
			Message: "库存不足",
		}, nil
	}

	logger.Log.Info("库存扣减成功",
		zap.Int64("product_id", req.ProductId),
		zap.Int32("quantity", req.Quantity))

	return &pb.DeductStockResponse{
		Success: true,
		Message: "扣减成功",
	}, nil
}

// CreateProduct 创建商品
func (h *ProductHandler) CreateProduct(ctx context.Context, req *pb.CreateProductRequest) (*pb.ProductInfo, error) {
	// 参数校验
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "商品名称不能为空")
	}
	if req.Price <= 0 || req.SeckillPrice <= 0 {
		return nil, status.Error(codes.InvalidArgument, "价格必须大于0")
	}
	if req.Stock < 0 {
		return nil, status.Error(codes.InvalidArgument, "库存不能为负")
	}

	product := model.Product{
		Name:         req.Name,
		Price:        req.Price,
		SeckillPrice: req.SeckillPrice,
		Stock:        int(req.Stock),
		Description:  req.Description,
		ImageURL:     req.ImageUrl,
		StartTime:    req.StartTime.AsTime(),
		EndTime:      req.EndTime.AsTime(),
	}

	if err := database.DB.Create(&product).Error; err != nil {
		logger.Log.Error("创建商品失败", zap.Error(err))
		return nil, status.Error(codes.Internal, "创建商品失败")
	}

	logger.Log.Info("商品创建成功", zap.Uint("product_id", product.ID))

	return modelToProto(&product), nil
}

// WarmUpStock 库存预热
// 【重点】秒杀前调用，将库存加载到 Redis
// ========================================================================
// 🔥 面试高频：
// Q: 库存预热的时机是什么？
// A: 1. 秒杀活动开始前几分钟
//  2. 可以通过定时任务自动触发
//  3. 也可以手动通过管理后台触发
//
// Q: 预热时需要注意什么？
// A: 1. 设置合理的过期时间（活动结束后自动清理）
//  2. 记录预热日志便于排查问题
//  3. 预热失败要有告警机制
//
// ========================================================================
func (h *ProductHandler) WarmUpStock(ctx context.Context, req *pb.WarmUpStockRequest) (*pb.WarmUpStockResponse, error) {
	if req.ProductId <= 0 {
		return nil, status.Error(codes.InvalidArgument, "商品ID无效")
	}

	// 从数据库获取商品信息
	var product model.Product
	if err := database.DB.First(&product, req.ProductId).Error; err != nil {
		return nil, status.Error(codes.NotFound, "商品不存在")
	}

	// 预热库存到 Redis
	stockKey := fmt.Sprintf("seckill:stock:%d", req.ProductId)
	boughtKey := fmt.Sprintf("seckill:bought:%d", req.ProductId)

	// 计算过期时间（活动结束后1小时）
	expiration := time.Until(product.EndTime) + time.Hour

	// 设置库存
	if err := redis.Client.Set(ctx, stockKey, product.Stock, expiration).Err(); err != nil {
		logger.Log.Error("预热库存失败", zap.Error(err))
		return nil, status.Error(codes.Internal, "预热库存失败")
	}

	// 清空已购买用户集合（如果存在旧数据）
	redis.Client.Del(ctx, boughtKey)

	logger.Log.Info("库存预热成功",
		zap.Uint("product_id", product.ID),
		zap.Int("stock", product.Stock),
		zap.Duration("expiration", expiration))

	return &pb.WarmUpStockResponse{
		Success:   true,
		Message:   fmt.Sprintf("库存预热成功，当前库存: %d", product.Stock),
		Stock:     int32(product.Stock),
		ExpiredAt: timestamppb.New(time.Now().Add(expiration)),
	}, nil
}

// modelToProto 将数据库模型转换为 protobuf
func modelToProto(p *model.Product) *pb.ProductInfo {
	return &pb.ProductInfo{
		ProductId:    int64(p.ID),
		Name:         p.Name,
		Price:        p.Price,
		SeckillPrice: p.SeckillPrice,
		Stock:        int32(p.Stock),
		Description:  p.Description,
		ImageUrl:     p.ImageURL,
		StartTime:    timestamppb.New(p.StartTime),
		EndTime:      timestamppb.New(p.EndTime),
		CreatedAt:    timestamppb.New(p.CreatedAt),
	}
}

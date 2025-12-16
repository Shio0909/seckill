package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"seckill/internal/model"
	"seckill/pkg/database"
	"seckill/pkg/redis"

	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// ========================================================================
// 【重点学习】Service 层设计
// ========================================================================
// Service 层负责业务逻辑，是 Controller 和 Repository 之间的桥梁
// 职责：
// 1. 业务逻辑编排（调用多个 Repository 方法）
// 2. 事务管理
// 3. 缓存策略
// 4. 参数校验（复杂的业务校验）
//
// 不应该在 Service 层：
// - 直接操作 HTTP 请求/响应（那是 Controller 的事）
// - 直接写 SQL（那是 Repository 的事）
// ========================================================================

// ProductService 商品服务
type ProductService struct{}

// ProductListRequest 商品列表请求参数
type ProductListRequest struct {
	Page     int    `form:"page" binding:"min=1"`
	PageSize int    `form:"page_size" binding:"min=1,max=100"`
	Keyword  string `form:"keyword"`
	Status   *int   `form:"status"` // 指针类型，区分0和未传
}

// ProductListResponse 商品列表响应
type ProductListResponse struct {
	Total int64           `json:"total"`
	List  []model.Product `json:"list"`
}

// List 获取商品列表（分页）
func (s *ProductService) List(req *ProductListRequest) (*ProductListResponse, error) {
	// 设置默认值
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 10
	}

	var products []model.Product
	var total int64

	query := database.DB.Model(&model.Product{})

	// 关键词搜索
	if req.Keyword != "" {
		query = query.Where("name LIKE ?", "%"+req.Keyword+"%")
	}

	// 获取总数
	query.Count(&total)

	// 分页查询
	offset := (req.Page - 1) * req.PageSize
	err := query.Offset(offset).Limit(req.PageSize).Order("id DESC").Find(&products).Error
	if err != nil {
		return nil, err
	}

	return &ProductListResponse{
		Total: total,
		List:  products,
	}, nil
}

// GetByID 根据ID获取商品详情
func (s *ProductService) GetByID(id uint) (*model.Product, error) {
	var product model.Product
	err := database.DB.First(&product, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("商品不存在")
		}
		return nil, err
	}
	return &product, nil
}

// ========================================================================
// 【重点学习】缓存策略 - Cache Aside Pattern
// ========================================================================
// 读取时：先读缓存，缓存没有则读数据库，再写入缓存
// 写入时：先更新数据库，再删除缓存（不是更新缓存！）
//
// 为什么删除而不是更新缓存？
// 1. 避免并发写导致的数据不一致
// 2. 懒加载，只有真正被读取时才缓存
//
// 更严格的方案：延迟双删
// 1. 删除缓存
// 2. 更新数据库
// 3. 延迟一段时间再删除缓存（防止并发读写不一致）
// ========================================================================

// GetStock 获取商品库存（优先读缓存）
func (s *ProductService) GetStock(productID uint) (int, error) {
	ctx := context.Background()
	key := fmt.Sprintf("seckill:stock:%d", productID)

	// 1. 先读 Redis 缓存
	stock, err := redis.Client.Get(ctx, key).Int()
	if err == nil {
		return stock, nil
	}

	// 2. 缓存未命中，读数据库
	if err != goredis.Nil {
		return 0, err // Redis 错误
	}

	var product model.Product
	if err := database.DB.Select("stock").First(&product, productID).Error; err != nil {
		return 0, err
	}

	// 3. 写入缓存（设置过期时间防止缓存雪崩）
	redis.Client.Set(ctx, key, product.Stock, 5*time.Minute)

	return product.Stock, nil
}

// CreateProductRequest 创建商品请求
type CreateProductRequest struct {
	Name         string    `json:"name" binding:"required,min=1,max=100"`
	Price        float64   `json:"price" binding:"required,gt=0"`
	SeckillPrice float64   `json:"seckill_price" binding:"required,gt=0"`
	Stock        int       `json:"stock" binding:"required,min=0"`
	Description  string    `json:"description"`
	ImageURL     string    `json:"image_url"`
	StartTime    time.Time `json:"start_time" binding:"required"`
	EndTime      time.Time `json:"end_time" binding:"required,gtfield=StartTime"`
}

// Create 创建商品
func (s *ProductService) Create(req *CreateProductRequest) (*model.Product, error) {
	product := &model.Product{
		Name:         req.Name,
		Price:        req.Price,
		SeckillPrice: req.SeckillPrice,
		Stock:        req.Stock,
		Description:  req.Description,
		ImageURL:     req.ImageURL,
		StartTime:    req.StartTime,
		EndTime:      req.EndTime,
	}

	if err := database.DB.Create(product).Error; err != nil {
		return nil, err
	}

	return product, nil
}

// UpdateProductRequest 更新商品请求
type UpdateProductRequest struct {
	Name         *string    `json:"name" binding:"omitempty,min=1,max=100"`
	Price        *float64   `json:"price" binding:"omitempty,gt=0"`
	SeckillPrice *float64   `json:"seckill_price" binding:"omitempty,gt=0"`
	Stock        *int       `json:"stock" binding:"omitempty,min=0"`
	Description  *string    `json:"description"`
	ImageURL     *string    `json:"image_url"`
	StartTime    *time.Time `json:"start_time"`
	EndTime      *time.Time `json:"end_time"`
}

// Update 更新商品
func (s *ProductService) Update(id uint, req *UpdateProductRequest) error {
	// 构建更新字段 map
	updates := make(map[string]interface{})

	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Price != nil {
		updates["price"] = *req.Price
	}
	if req.SeckillPrice != nil {
		updates["seckill_price"] = *req.SeckillPrice
	}
	if req.Stock != nil {
		updates["stock"] = *req.Stock
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.ImageURL != nil {
		updates["image_url"] = *req.ImageURL
	}
	if req.StartTime != nil {
		updates["start_time"] = *req.StartTime
	}
	if req.EndTime != nil {
		updates["end_time"] = *req.EndTime
	}

	if len(updates) == 0 {
		return errors.New("没有需要更新的字段")
	}

	result := database.DB.Model(&model.Product{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("商品不存在")
	}

	// ====================================================================
	// 【重点学习】更新后删除缓存
	// ====================================================================
	ctx := context.Background()
	redis.Client.Del(ctx, fmt.Sprintf("seckill:stock:%d", id))

	return nil
}

// Delete 删除商品（软删除）
func (s *ProductService) Delete(id uint) error {
	result := database.DB.Delete(&model.Product{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("商品不存在")
	}

	// 删除缓存
	ctx := context.Background()
	redis.Client.Del(ctx, fmt.Sprintf("seckill:stock:%d", id))

	return nil
}

// SetStock 设置商品库存（同时更新 Redis）
func (s *ProductService) SetStock(id uint, stock int) error {
	// ====================================================================
	// 【重点学习】库存预热
	// ====================================================================
	// 秒杀开始前，需要将库存从 MySQL 同步到 Redis
	// 这个操作通常由管理员在秒杀开始前手动触发
	// 或者通过定时任务自动执行
	// ====================================================================

	// 1. 更新数据库
	result := database.DB.Model(&model.Product{}).Where("id = ?", id).Update("stock", stock)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("商品不存在")
	}

	// 2. 同步到 Redis
	ctx := context.Background()
	key := fmt.Sprintf("seckill:stock:%d", id)
	err := redis.Client.Set(ctx, key, stock, 0).Err() // 不设置过期时间
	if err != nil {
		return fmt.Errorf("同步 Redis 失败: %w", err)
	}

	return nil
}

// WarmUp 库存预热（秒杀前调用）
func (s *ProductService) WarmUp(id uint) error {
	// 1. 从数据库读取库存
	var product model.Product
	if err := database.DB.Select("stock").First(&product, id).Error; err != nil {
		return err
	}

	// 2. 写入 Redis
	ctx := context.Background()
	stockKey := fmt.Sprintf("seckill:stock:%d", id)
	boughtKey := fmt.Sprintf("seckill:bought:%d", id)

	// 设置库存
	if err := redis.Client.Set(ctx, stockKey, product.Stock, 0).Err(); err != nil {
		return err
	}

	// 清空已购买用户集合（如果是重新预热）
	redis.Client.Del(ctx, boughtKey)

	return nil
}

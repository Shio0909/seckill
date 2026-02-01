package controller

import (
	"strconv"

	"seckill/internal/model"
	"seckill/internal/service"
	"seckill/pkg/e"
	"seckill/pkg/response"

	"github.com/gin-gonic/gin"
)

// Controller 层（或叫 Handler 层）的职责：
// 1. 接收 HTTP 请求，解析参数
// 2. 调用 Service 层处理业务逻辑
// 3. 封装响应返回给客户端
//
// 不应该在 Controller 层：
// - 直接操作数据库
// - 编写复杂的业务逻辑
// - 直接操作缓存
//
// 命名规范：
// - List   - 列表查询 (GET /products)
// - Get    - 详情查询 (GET /products/:id)
// - Create - 创建资源 (POST /products)
// - Update - 更新资源 (PUT /products/:id)
// - Delete - 删除资源 (DELETE /products/:id)

type ProductController struct {
	productService *service.ProductService
}

func NewProductController() *ProductController {
	return &ProductController{
		productService: &service.ProductService{},
	}
}

// 引入 model.Product 供 Swagger 注释解析使用，避免在注释中引用未导入的类型导致 swag 解析失败
var _ = model.Product{}

// List 获取商品列表
// @Summary 获取商品列表
// @Description 分页获取商品列表，支持关键词搜索
// @Tags 商品管理
// @Accept json
// @Produce json
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(10)
// @Param keyword query string false "搜索关键词"
// @Success 200 {object} response.Response{data=service.ProductListResponse}
// @Router /api/v1/products [get]
func (c *ProductController) List(ctx *gin.Context) {
	var req service.ProductListRequest
	if err := ctx.ShouldBindQuery(&req); err != nil {
		response.FailWithMsg(ctx, e.INVALID_PARAMS, err.Error())
		return
	}

	result, err := c.productService.List(&req)
	if err != nil {
		response.FailWithMsg(ctx, e.ERROR, err.Error())
		return
	}

	response.Success(ctx, result)
}

// Get 获取商品详情
// @Summary 获取商品详情
// @Description 根据ID获取商品详情
// @Tags 商品管理
// @Accept json
// @Produce json
// @Param id path int true "商品ID"
// @Success 200 {object} response.Response{data=model.Product}
// @Router /api/v1/products/{id} [get]
func (c *ProductController) Get(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil {
		response.FailWithMsg(ctx, e.INVALID_PARAMS, "无效的商品ID")
		return
	}

	product, err := c.productService.GetByID(uint(id))
	if err != nil {
		response.FailWithMsg(ctx, e.ERROR_NOT_EXIST_PRODUCT, err.Error())
		return
	}

	response.Success(ctx, product)
}

// Create 创建商品
// @Summary 创建商品
// @Description 创建新商品
// @Tags 商品管理
// @Accept json
// @Produce json
// @Param product body service.CreateProductRequest true "商品信息"
// @Success 200 {object} response.Response{data=model.Product}
// @Router /api/v1/products [post]
func (c *ProductController) Create(ctx *gin.Context) {
	var req service.CreateProductRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(ctx, e.INVALID_PARAMS, err.Error())
		return
	}

	product, err := c.productService.Create(&req)
	if err != nil {
		response.FailWithMsg(ctx, e.ERROR, err.Error())
		return
	}

	response.Success(ctx, product)
}

// Update 更新商品
// @Summary 更新商品
// @Description 根据ID更新商品信息
// @Tags 商品管理
// @Accept json
// @Produce json
// @Param id path int true "商品ID"
// @Param product body service.UpdateProductRequest true "商品信息"
// @Success 200 {object} response.Response
// @Router /api/v1/products/{id} [put]
func (c *ProductController) Update(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil {
		response.FailWithMsg(ctx, e.INVALID_PARAMS, "无效的商品ID")
		return
	}

	var req service.UpdateProductRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(ctx, e.INVALID_PARAMS, err.Error())
		return
	}

	if err := c.productService.Update(uint(id), &req); err != nil {
		response.FailWithMsg(ctx, e.ERROR, err.Error())
		return
	}

	response.Success(ctx, nil)
}

// Delete 删除商品
// @Summary 删除商品
// @Description 根据ID删除商品（软删除）
// @Tags 商品管理
// @Accept json
// @Produce json
// @Param id path int true "商品ID"
// @Success 200 {object} response.Response
// @Router /api/v1/products/{id} [delete]
func (c *ProductController) Delete(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil {
		response.FailWithMsg(ctx, e.INVALID_PARAMS, "无效的商品ID")
		return
	}

	if err := c.productService.Delete(uint(id)); err != nil {
		response.FailWithMsg(ctx, e.ERROR, err.Error())
		return
	}

	response.Success(ctx, nil)
}

// GetStock 获取商品库存
// @Summary 获取商品库存
// @Description 获取商品当前库存（优先读缓存）
// @Tags 商品管理
// @Accept json
// @Produce json
// @Param id path int true "商品ID"
// @Success 200 {object} response.Response{data=int}
// @Router /api/v1/products/{id}/stock [get]
func (c *ProductController) GetStock(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil {
		response.FailWithMsg(ctx, e.INVALID_PARAMS, "无效的商品ID")
		return
	}

	stock, err := c.productService.GetStock(uint(id))
	if err != nil {
		response.FailWithMsg(ctx, e.ERROR, err.Error())
		return
	}

	response.Success(ctx, gin.H{"stock": stock})
}

// SetStock 设置商品库存
// @Summary 设置商品库存
// @Description 设置商品库存（同时更新数据库和缓存）
// @Tags 商品管理
// @Accept json
// @Produce json
// @Param id path int true "商品ID"
// @Param stock body object true "库存信息" example({"stock": 100})
// @Success 200 {object} response.Response
// @Router /api/v1/products/{id}/stock [put]
func (c *ProductController) SetStock(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil {
		response.FailWithMsg(ctx, e.INVALID_PARAMS, "无效的商品ID")
		return
	}

	var req struct {
		Stock int `json:"stock" binding:"required,min=0"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(ctx, e.INVALID_PARAMS, err.Error())
		return
	}

	if err := c.productService.SetStock(uint(id), req.Stock); err != nil {
		response.FailWithMsg(ctx, e.ERROR, err.Error())
		return
	}

	response.Success(ctx, nil)
}

// WarmUp 库存预热
// @Summary 库存预热
// @Description 秒杀开始前预热库存到Redis
// @Tags 商品管理
// @Accept json
// @Produce json
// @Param id path int true "商品ID"
// @Success 200 {object} response.Response
// @Router /api/v1/products/{id}/warmup [post]
func (c *ProductController) WarmUp(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil {
		response.FailWithMsg(ctx, e.INVALID_PARAMS, "无效的商品ID")
		return
	}

	if err := c.productService.WarmUp(uint(id)); err != nil {
		response.FailWithMsg(ctx, e.ERROR, err.Error())
		return
	}

	response.Success(ctx, gin.H{"message": "库存预热成功"})
}

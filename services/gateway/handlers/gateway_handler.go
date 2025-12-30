package handlers

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"seckill/pkg/grpcx"
	"seckill/pkg/utils"
	pbOrder "seckill/proto/order"
	pbProduct "seckill/proto/product"
	pbSeckill "seckill/proto/seckill"
	pbUser "seckill/proto/user"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ========================================================================
// 【重点学习】API Gateway Handler
// ========================================================================
// Gateway Handler 负责：
// 1. 接收 HTTP 请求
// 2. 参数校验和转换
// 3. 调用 gRPC 服务
// 4. 将 gRPC 响应转换为 JSON
//
// 📝 简历亮点：
// - HTTP 到 gRPC 的协议转换
// - 统一错误处理
// - 上下文传递
//
// 🔥 面试高频：
// Q: 为什么不直接暴露 gRPC 服务？
// A: 1. 浏览器不支持 gRPC（需要 grpc-web）
//    2. HTTP 更通用，易于调试
//    3. Gateway 可以做统一的认证、限流
//    4. 可以聚合多个服务的调用
// ========================================================================

// GatewayHandler 网关处理器
type GatewayHandler struct {
	clientManager *grpcx.ClientManager
}

// NewGatewayHandler 创建网关处理器
func NewGatewayHandler(cm *grpcx.ClientManager) *GatewayHandler {
	return &GatewayHandler{
		clientManager: cm,
	}
}

// ========================= 认证中间件 =========================

// AuthMiddleware 认证中间件
func (h *GatewayHandler) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 获取 Token
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    401,
				"message": "未提供认证信息",
			})
			c.Abort()
			return
		}

		// 解析 Bearer Token
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    401,
				"message": "认证格式错误",
			})
			c.Abort()
			return
		}

		token := parts[1]

		// 【方案1】本地验证（快速）
		claims, err := utils.ParseToken(token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    401,
				"message": "Token 无效或已过期",
			})
			c.Abort()
			return
		}

		// 将用户信息存入上下文
		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Next()

		// 【方案2】调用 User 服务验证（更安全，可以检查用户状态）
		// 这种方式有额外的网络开销，但更准确
		/*
			conn, err := h.clientManager.GetConnection("user-service")
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "服务不可用"})
				c.Abort()
				return
			}
			client := pbUser.NewUserServiceClient(conn)
			resp, err := client.ValidateToken(c.Request.Context(), &pbUser.ValidateTokenRequest{Token: token})
			if err != nil || !resp.Valid {
				c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "Token 验证失败"})
				c.Abort()
				return
			}
			c.Set("user_id", uint(resp.UserId))
			c.Set("username", resp.Username)
			c.Next()
		*/
	}
}

// ========================= 用户服务接口 =========================

// Register 用户注册
func (h *GatewayHandler) Register(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
		Phone    string `json:"phone" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误: " + err.Error(),
		})
		return
	}

	// 获取 gRPC 连接
	conn, err := h.clientManager.GetConnection("user-service")
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"code":    503,
			"message": "用户服务不可用",
		})
		return
	}

	// 调用 gRPC 服务
	client := pbUser.NewUserServiceClient(conn)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	resp, err := client.Register(ctx, &pbUser.RegisterRequest{
		Username: req.Username,
		Password: req.Password,
		Phone:    req.Phone,
	})

	if err != nil {
		handleGRPCError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": resp.Message,
		"data": gin.H{
			"user_id": resp.UserId,
		},
	})
}

// Login 用户登录
func (h *GatewayHandler) Login(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误",
		})
		return
	}

	conn, err := h.clientManager.GetConnection("user-service")
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"code":    503,
			"message": "用户服务不可用",
		})
		return
	}

	client := pbUser.NewUserServiceClient(conn)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	resp, err := client.Login(ctx, &pbUser.LoginRequest{
		Username: req.Username,
		Password: req.Password,
	})

	if err != nil {
		handleGRPCError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "登录成功",
		"data": gin.H{
			"token":   resp.Token,
			"user_id": resp.UserId,
		},
	})
}

// GetUser 获取用户信息
func (h *GatewayHandler) GetUser(c *gin.Context) {
	userID, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if userID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "用户ID无效",
		})
		return
	}

	conn, err := h.clientManager.GetConnection("user-service")
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "message": "服务不可用"})
		return
	}

	client := pbUser.NewUserServiceClient(conn)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	resp, err := client.GetUser(ctx, &pbUser.GetUserRequest{UserId: userID})
	if err != nil {
		handleGRPCError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"user_id":  resp.UserId,
			"username": resp.Username,
			"phone":    resp.Phone,
			"avatar":   resp.Avatar,
			"status":   resp.Status,
		},
	})
}

// ========================= 商品服务接口 =========================

// ListProducts 商品列表
func (h *GatewayHandler) ListProducts(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	conn, err := h.clientManager.GetConnection("product-service")
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "message": "服务不可用"})
		return
	}

	client := pbProduct.NewProductServiceClient(conn)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	resp, err := client.ListProducts(ctx, &pbProduct.ListProductsRequest{
		Page:     int32(page),
		PageSize: int32(pageSize),
	})
	if err != nil {
		handleGRPCError(c, err)
		return
	}

	// 转换响应
	var products []gin.H
	for _, p := range resp.Products {
		products = append(products, productToJSON(p))
	}

	c.JSON(http.StatusOK, gin.H{
		"code":  0,
		"data":  products,
		"total": resp.Total,
	})
}

// GetProduct 获取商品详情
func (h *GatewayHandler) GetProduct(c *gin.Context) {
	productID, _ := strconv.ParseInt(c.Param("id"), 10, 64)

	conn, err := h.clientManager.GetConnection("product-service")
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "message": "服务不可用"})
		return
	}

	client := pbProduct.NewProductServiceClient(conn)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	resp, err := client.GetProduct(ctx, &pbProduct.GetProductRequest{ProductId: productID})
	if err != nil {
		handleGRPCError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": productToJSON(resp),
	})
}

// GetStock 获取库存
func (h *GatewayHandler) GetStock(c *gin.Context) {
	productID, _ := strconv.ParseInt(c.Param("id"), 10, 64)

	conn, err := h.clientManager.GetConnection("product-service")
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "message": "服务不可用"})
		return
	}

	client := pbProduct.NewProductServiceClient(conn)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	resp, err := client.GetStock(ctx, &pbProduct.GetStockRequest{ProductId: productID})
	if err != nil {
		handleGRPCError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"product_id": resp.ProductId,
			"stock":      resp.Stock,
			"source":     resp.Source,
		},
	})
}

// ========================= 订单服务接口 =========================

// ListOrders 订单列表
func (h *GatewayHandler) ListOrders(c *gin.Context) {
	userID := c.GetUint("user_id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	conn, err := h.clientManager.GetConnection("order-service")
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "message": "服务不可用"})
		return
	}

	client := pbOrder.NewOrderServiceClient(conn)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	resp, err := client.ListOrders(ctx, &pbOrder.ListOrdersRequest{
		UserId:   int64(userID),
		Page:     int32(page),
		PageSize: int32(pageSize),
	})
	if err != nil {
		handleGRPCError(c, err)
		return
	}

	var orders []gin.H
	for _, o := range resp.Orders {
		orders = append(orders, orderToJSON(o))
	}

	c.JSON(http.StatusOK, gin.H{
		"code":  0,
		"data":  orders,
		"total": resp.Total,
	})
}

// GetOrder 获取订单详情
func (h *GatewayHandler) GetOrder(c *gin.Context) {
	orderID, _ := strconv.ParseInt(c.Param("id"), 10, 64)

	conn, err := h.clientManager.GetConnection("order-service")
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "message": "服务不可用"})
		return
	}

	client := pbOrder.NewOrderServiceClient(conn)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	resp, err := client.GetOrder(ctx, &pbOrder.GetOrderRequest{OrderId: orderID})
	if err != nil {
		handleGRPCError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": orderToJSON(resp),
	})
}

// CancelOrder 取消订单
func (h *GatewayHandler) CancelOrder(c *gin.Context) {
	orderID, _ := strconv.ParseInt(c.Param("id"), 10, 64)

	conn, err := h.clientManager.GetConnection("order-service")
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "message": "服务不可用"})
		return
	}

	client := pbOrder.NewOrderServiceClient(conn)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	resp, err := client.CancelOrder(ctx, &pbOrder.CancelOrderRequest{OrderId: orderID})
	if err != nil {
		handleGRPCError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"success": resp.Success,
		"message": resp.Message,
	})
}

// PayOrder 支付订单
func (h *GatewayHandler) PayOrder(c *gin.Context) {
	orderID, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req struct {
		PaymentMethod string `json:"payment_method"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}

	conn, err := h.clientManager.GetConnection("order-service")
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "message": "服务不可用"})
		return
	}

	client := pbOrder.NewOrderServiceClient(conn)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	resp, err := client.PayOrder(ctx, &pbOrder.PayOrderRequest{
		OrderId:       orderID,
		PaymentMethod: req.PaymentMethod,
	})
	if err != nil {
		handleGRPCError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":       0,
		"success":    resp.Success,
		"message":    resp.Message,
		"payment_id": resp.PaymentId,
	})
}

// ========================= 秒杀服务接口 =========================

// DoSeckill 执行秒杀
func (h *GatewayHandler) DoSeckill(c *gin.Context) {
	userID := c.GetUint("user_id")
	var req struct {
		ProductID int64 `json:"product_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误",
		})
		return
	}

	conn, err := h.clientManager.GetConnection("seckill-service")
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "message": "服务不可用"})
		return
	}

	client := pbSeckill.NewSeckillServiceClient(conn)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	resp, err := client.DoSeckill(ctx, &pbSeckill.SeckillRequest{
		UserId:    int64(userID),
		ProductId: req.ProductID,
	})
	if err != nil {
		handleGRPCError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"success": resp.Success,
		"message": resp.Message,
		"status":  resp.Status.String(),
	})
}

// GetSeckillResult 获取秒杀结果
func (h *GatewayHandler) GetSeckillResult(c *gin.Context) {
	userID := c.GetUint("user_id")
	productID, _ := strconv.ParseInt(c.Query("product_id"), 10, 64)

	conn, err := h.clientManager.GetConnection("seckill-service")
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "message": "服务不可用"})
		return
	}

	client := pbSeckill.NewSeckillServiceClient(conn)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	resp, err := client.GetSeckillResult(ctx, &pbSeckill.GetResultRequest{
		UserId:    int64(userID),
		ProductId: productID,
	})
	if err != nil {
		handleGRPCError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":     0,
		"status":   resp.Status.String(),
		"message":  resp.Message,
		"order_no": resp.OrderNo,
	})
}

// GetSeckillProduct 获取秒杀商品信息
func (h *GatewayHandler) GetSeckillProduct(c *gin.Context) {
	productID, _ := strconv.ParseInt(c.Param("id"), 10, 64)

	conn, err := h.clientManager.GetConnection("seckill-service")
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "message": "服务不可用"})
		return
	}

	client := pbSeckill.NewSeckillServiceClient(conn)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	resp, err := client.GetSeckillProduct(ctx, &pbSeckill.GetSeckillProductRequest{
		ProductId: productID,
	})
	if err != nil {
		handleGRPCError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"product_id":  resp.ProductId,
			"stock":       resp.Stock,
			"status":      resp.Status,
			"status_text": resp.StatusText,
			"server_time": resp.ServerTime,
		},
	})
}

// ========================= 辅助函数 =========================

// handleGRPCError 处理 gRPC 错误
// 【重点】gRPC 错误码到 HTTP 状态码的映射
func handleGRPCError(c *gin.Context, err error) {
	st, ok := status.FromError(err)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "内部错误",
		})
		return
	}

	// gRPC Code -> HTTP Status
	var httpStatus int
	switch st.Code() {
	case codes.InvalidArgument:
		httpStatus = http.StatusBadRequest
	case codes.NotFound:
		httpStatus = http.StatusNotFound
	case codes.AlreadyExists:
		httpStatus = http.StatusConflict
	case codes.PermissionDenied:
		httpStatus = http.StatusForbidden
	case codes.Unauthenticated:
		httpStatus = http.StatusUnauthorized
	case codes.ResourceExhausted:
		httpStatus = http.StatusTooManyRequests
	case codes.Unavailable:
		httpStatus = http.StatusServiceUnavailable
	default:
		httpStatus = http.StatusInternalServerError
	}

	c.JSON(httpStatus, gin.H{
		"code":    int(st.Code()),
		"message": st.Message(),
	})
}

// productToJSON 商品转 JSON
func productToJSON(p *pbProduct.ProductInfo) gin.H {
	return gin.H{
		"product_id":    p.ProductId,
		"name":          p.Name,
		"price":         p.Price,
		"seckill_price": p.SeckillPrice,
		"stock":         p.Stock,
		"description":   p.Description,
		"image_url":     p.ImageUrl,
		"start_time":    timestampToString(p.StartTime),
		"end_time":      timestampToString(p.EndTime),
		"created_at":    timestampToString(p.CreatedAt),
	}
}

// orderToJSON 订单转 JSON
func orderToJSON(o *pbOrder.OrderInfo) gin.H {
	return gin.H{
		"order_id":      o.OrderId,
		"order_no":      o.OrderNo,
		"user_id":       o.UserId,
		"product_id":    o.ProductId,
		"product_name":  o.ProductName,
		"product_price": o.ProductPrice,
		"status":        o.Status.String(),
		"created_at":    timestampToString(o.CreatedAt),
		"updated_at":    timestampToString(o.UpdatedAt),
	}
}

func timestampToString(ts *timestamppb.Timestamp) string {
	if ts == nil {
		return ""
	}
	return ts.AsTime().Format("2006-01-02 15:04:05")
}

package handler

import (
	"context"
	"fmt"
	"time"

	"seckill/pkg/breaker"
	"seckill/pkg/logger"
	"seckill/pkg/rabbitmq"
	"seckill/pkg/redis"
	pb "seckill/proto/seckill"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Seckill 服务是整个秒杀系统的核心，负责：
// 1. 处理秒杀请求
// 2. Redis 预扣库存
// 3. 发送消息到 MQ
// 4. 返回秒杀结果

// SeckillHandler 秒杀服务处理器
type SeckillHandler struct {
	pb.UnimplementedSeckillServiceServer
	breakerManager *breaker.BreakerManager // 熔断器管理
}

// NewSeckillHandler 创建秒杀处理器
func NewSeckillHandler() *SeckillHandler {
	return &SeckillHandler{
		breakerManager: breaker.NewBreakerManager(breaker.DefaultSettings()),
	}
}

// DoSeckill 执行秒杀
// 【重点】这是秒杀的核心入口
func (h *SeckillHandler) DoSeckill(ctx context.Context, req *pb.SeckillRequest) (*pb.SeckillResponse, error) {
	// 参数校验
	if req.UserId <= 0 || req.ProductId <= 0 {
		return nil, status.Error(codes.InvalidArgument, "参数无效")
	}

	// 【重点】熔断检查
	// 如果 Redis 或 MQ 故障，熔断器会快速失败，避免请求堆积
	cb := h.breakerManager.GetBreaker("seckill")
	if err := cb.Allow(); err != nil {
		logger.Log.Warn("熔断器开启，拒绝请求",
			zap.Int64("user_id", req.UserId),
			zap.Int64("product_id", req.ProductId))
		return &pb.SeckillResponse{
			Success: false,
			Message: "系统繁忙，请稍后再试",
			Status:  pb.SeckillStatus_FAIL,
		}, nil
	}

	// 执行秒杀逻辑
	success, message := h.doSeckillWithLua(ctx, req.UserId, req.ProductId)

	// 更新熔断器状态
	if !success && message == "系统繁忙" {
		cb.Failure()
	} else {
		cb.Success()
	}

	if success {
		return &pb.SeckillResponse{
			Success: true,
			Message: message,
			Status:  pb.SeckillStatus_QUEUING, // 排队中，等待消费者处理
		}, nil
	}

	return &pb.SeckillResponse{
		Success: false,
		Message: message,
		Status:  pb.SeckillStatus_FAIL,
	}, nil
}

// doSeckillWithLua 使用 Lua 脚本执行秒杀
// 为什么用 Lua？
// 1. 原子性：整个脚本作为一个命令执行，不会被打断
// 2. 性能：减少网络往返，多个操作一次完成
// 3. 事务：避免 WATCH/MULTI/EXEC 的复杂性
//
// 脚本逻辑：
// 1. 检查用户是否已购买（SISMEMBER）
// 2. 检查库存是否充足（GET）
// 3. 扣减库存（DECR）
// 4. 记录用户已购买（SADD）
func (h *SeckillHandler) doSeckillWithLua(ctx context.Context, userID, productID int64) (bool, string) {
	// 准备 Key
	stockKey := fmt.Sprintf("seckill:stock:%d", productID)
	boughtKey := fmt.Sprintf("seckill:bought:%d", productID)

	// 执行 Lua 脚本
	result, err := redis.SeckillScript.Run(ctx, redis.Client,
		[]string{stockKey, boughtKey},
		userID).Int()

	if err != nil {
		logger.Log.Error("执行 Lua 脚本失败", zap.Error(err))
		return false, "系统繁忙"
	}

	// 处理返回值
	switch result {
	case -1:
		logger.Log.Warn("重复购买拦截",
			zap.Int64("user_id", userID),
			zap.Int64("product_id", productID))
		return false, "您已经抢购过了，请勿重复下单"

	case -2:
		logger.Log.Warn("库存不足",
			zap.Int64("product_id", productID))
		return false, "手慢了，商品已抢光"

	case 1:
		logger.Log.Info("Redis 预扣库存成功",
			zap.Int64("user_id", userID),
			zap.Int64("product_id", productID))

		// 【重点】发送消息到 MQ
		// 这里实现了异步削峰：快速返回用户，后台慢慢处理
		err := rabbitmq.SendSeckillMessage(userID, productID)
		if err != nil {
			logger.Log.Error("发送 MQ 消息失败", zap.Error(err))
			// TODO: 这里应该回滚 Redis 库存
			return false, "订单创建失败，请稍后再试"
		}

		return true, "抢购成功！正在生成订单..."
	}

	return false, "未知错误"
}

// GetSeckillResult 获取秒杀结果
// 【重点】用户轮询此接口获取秒杀结果
func (h *SeckillHandler) GetSeckillResult(ctx context.Context, req *pb.GetResultRequest) (*pb.GetResultResponse, error) {
	if req.UserId <= 0 || req.ProductId <= 0 {
		return nil, status.Error(codes.InvalidArgument, "参数无效")
	}

	// 检查秒杀结果
	// 策略：检查是否有订单生成
	resultKey := fmt.Sprintf("seckill:result:%d:%d", req.UserId, req.ProductId)

	// 先检查 Redis 中的结果缓存
	result, err := redis.Client.Get(ctx, resultKey).Result()
	if err == nil {
		// 有缓存结果
		switch result {
		case "success":
			// 获取订单号
			orderKey := fmt.Sprintf("seckill:order:%d:%d", req.UserId, req.ProductId)
			orderNo, _ := redis.Client.Get(ctx, orderKey).Result()
			return &pb.GetResultResponse{
				Status:  pb.SeckillStatus_SUCCESS,
				Message: "秒杀成功",
				OrderNo: orderNo,
			}, nil
		case "fail":
			return &pb.GetResultResponse{
				Status:  pb.SeckillStatus_FAIL,
				Message: "秒杀失败",
			}, nil
		}
	}

	// 没有缓存，说明还在排队
	return &pb.GetResultResponse{
		Status:  pb.SeckillStatus_QUEUING,
		Message: "正在处理中，请稍后查询",
	}, nil
}

// GetSeckillProduct 获取秒杀商品信息
// 【重点】从 Redis 获取实时库存
func (h *SeckillHandler) GetSeckillProduct(ctx context.Context, req *pb.GetSeckillProductRequest) (*pb.SeckillProductInfo, error) {
	if req.ProductId <= 0 {
		return nil, status.Error(codes.InvalidArgument, "商品ID无效")
	}

	// 从 Redis 获取库存
	stockKey := fmt.Sprintf("seckill:stock:%d", req.ProductId)
	stock, err := redis.Client.Get(ctx, stockKey).Int()
	if err != nil {
		// Redis 中没有，可能活动未开始或已结束
		return &pb.SeckillProductInfo{
			ProductId:  req.ProductId,
			Stock:      0,
			Status:     "not_started",
			StatusText: "活动未开始或已结束",
		}, nil
	}

	// 判断状态
	status := "ongoing"
	statusText := "正在进行"
	if stock <= 0 {
		status = "sold_out"
		statusText = "已售罄"
	}

	return &pb.SeckillProductInfo{
		ProductId:  req.ProductId,
		Stock:      int32(stock),
		Status:     status,
		StatusText: statusText,
		ServerTime: time.Now().Unix(),
	}, nil
}

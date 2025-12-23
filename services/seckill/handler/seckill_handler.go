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

// ========================================================================
// 【重点学习】Seckill 微服务 gRPC Handler 实现
// ========================================================================
// Seckill 服务是整个秒杀系统的核心，负责：
// 1. 处理秒杀请求
// 2. Redis 预扣库存
// 3. 发送消息到 MQ
// 4. 返回秒杀结果
//
// 📝 简历亮点：
// - Redis Lua 脚本原子操作
// - 消息队列异步削峰
// - 熔断器保护下游服务
// - 结果轮询机制
//
// 🔥 面试高频：
// Q: 秒杀系统如何解决并发问题？
// A: 分层过滤思想：
//    1. 前端：倒计时、按钮置灰、验证码
//    2. 网关：限流、黑名单
//    3. 缓存层：Redis 预扣库存（Lua 原子操作）
//    4. 消息队列：异步处理，削峰填谷
//    5. 数据库：乐观锁防超卖
//
// Q: 如何保证不超卖？
// A: 1. Redis Lua 脚本原子扣减
//    2. 数据库层面使用乐观锁（WHERE stock >= quantity）
//    3. 联合唯一索引防止重复下单
//
// Q: 用户如何知道秒杀是否成功？
// A: 两种方案：
//    1. 同步返回：直接等待消息处理完成（延迟高）
//    2. 异步轮询：先返回"排队中"，用户轮询结果（本项目方案）
//
// 面试高频问题（补充）：
// Q: Redis 挂了怎么办？
// A: 1. 主从哨兵/集群模式保证高可用。
//    2. 本地缓存（Local Cache）兜底（如 GoCache），但要注意数据一致性问题。
//    3. 降级处理：直接返回“活动太火爆，请稍后再试”。
//
// Q: 消息队列消息丢失怎么办？
// A: 1. 生产者：开启 Confirm 模式，确保消息发送到 Broker。
//    2. Broker：开启持久化（Exchange, Queue, Message）。
//    3. 消费者：手动 ACK，确保业务逻辑执行成功后再确认消息。
//
// Q: 为什么秒杀要用 Lua 脚本？
// A: Redis 的单个命令是原子的，但多个命令组合不是。
//    Lua 脚本可以将多个命令（检查库存、扣减库存、记录用户）打包成一个原子操作，
//    避免并发竞争条件 (Race Condition)，且减少网络 RTT。
// ========================================================================

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
// ========================================================================
// 【重点学习】Redis Lua 脚本原子操作
// ========================================================================
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
// ========================================================================
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
// ========================================================================
// 🔥 面试高频：
// Q: 为什么用轮询而不是 WebSocket？
// A: 1. 秒杀场景：用户主动刷新，轮询更简单
//  2. WebSocket 需要维护长连接，服务端压力大
//  3. 轮询可以利用 HTTP 缓存（CDN）
//  4. 实际可以结合使用：先轮询，超时后降级提示
//
// ========================================================================
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

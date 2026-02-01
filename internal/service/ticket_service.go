package service

import (
	"context"
	"fmt"
	"seckill/pkg/logger"
	"seckill/pkg/rabbitmq"
	"seckill/pkg/redis" // 引入 Redis 包

	"go.uber.org/zap"
)

// TicketPurchase 抢票服务（原SeckillV2）
// 使用 Redis Lua 脚本进行原子扣减，适用于演唱会、体育赛事等票务抢购场景
func TicketPurchase(userID int, eventID int) (bool, string) {
	ctx := context.Background()

	// 1. 准备 Key
	// ticket:stock:1 (String 类型，存票务库存)
	stockKey := fmt.Sprintf("ticket:stock:%d", eventID)
	// ticket:bought:1 (Set 类型，存已购票用户ID)
	boughtKey := fmt.Sprintf("ticket:bought:%d", eventID)

	// 2. 执行 Lua 脚本（原子操作）
	// Keys: [stockKey, boughtKey]
	// Args: [userID]
	result, err := redis.SeckillScript.Run(ctx, redis.Client,
		[]string{stockKey, boughtKey},
		userID).Int()

	if err != nil {
		logger.Log.Error("执行抢票Lua脚本失败", zap.Error(err))
		return false, "系统繁忙，请稍后再试"
	}

	// 3. 处理 Lua 返回值
	switch result {
	case -1:
		// 重复购票拦截
		logger.Log.Warn("重复购票拦截", zap.Int("uid", userID), zap.Int("eventID", eventID))
		return false, "您已经购买过该场次，请勿重复下单"
	case -2:
		// 票已售罄
		logger.Log.Warn("票务库存不足", zap.Int("eventID", eventID))
		return false, "手慢了，票已售罄"
	case 1:
		// 抢票成功，发送到消息队列异步创建订单
		logger.Log.Info("Redis抢票成功", zap.Int("uid", userID), zap.Int("eventID", eventID))

		// RabbitMQ 发送订单创建消息
		err := rabbitmq.SendSeckillMessage(int64(userID), int64(eventID))
		if err != nil {
			logger.Log.Error("发送订单消息失败", zap.Error(err))
			return false, "订单创建失败，请稍后再试"
		}

		return true, "抢票成功！正在生成订单..."
	}

	return false, "未知错误"
}

// TicketPurchaseWithSession 带场次的抢票服务
// 支持同一活动多场次的票务购买
func TicketPurchaseWithSession(userID int, eventID int, sessionID int, seatType string) (bool, string) {
	ctx := context.Background()

	// 使用 活动ID:场次ID:座位类型 作为唯一标识
	stockKey := fmt.Sprintf("ticket:stock:%d:%d:%s", eventID, sessionID, seatType)
	boughtKey := fmt.Sprintf("ticket:bought:%d:%d", eventID, sessionID)

	// 执行 Lua 脚本
	result, err := redis.SeckillScript.Run(ctx, redis.Client,
		[]string{stockKey, boughtKey},
		userID).Int()

	if err != nil {
		logger.Log.Error("执行抢票Lua脚本失败", zap.Error(err))
		return false, "系统繁忙，请稍后再试"
	}

	switch result {
	case -1:
		return false, "您已经购买过该场次，请勿重复下单"
	case -2:
		return false, "该票档已售罄，请选择其他档位"
	case 1:
		logger.Log.Info("抢票成功",
			zap.Int("uid", userID),
			zap.Int("eventID", eventID),
			zap.Int("sessionID", sessionID),
			zap.String("seatType", seatType))

		// 发送订单消息
		err := rabbitmq.SendTicketMessage(int64(userID), int64(eventID), int64(sessionID), seatType)
		if err != nil {
			return false, "订单创建失败，请稍后再试"
		}

		return true, "抢票成功！正在生成订单..."
	}

	return false, "未知错误"
}

// CheckTicketAvailability 检查票务可用性
func CheckTicketAvailability(eventID int, sessionID int) (map[string]int64, error) {
	ctx := context.Background()

	// 获取各档位库存
	seatTypes := []string{"vip", "premium", "standard", "economy"}
	availability := make(map[string]int64)

	for _, seatType := range seatTypes {
		stockKey := fmt.Sprintf("ticket:stock:%d:%d:%s", eventID, sessionID, seatType)
		stock, err := redis.Client.Get(ctx, stockKey).Int64()
		if err != nil {
			stock = 0
		}
		availability[seatType] = stock
	}

	return availability, nil
}

// GetUserPurchaseHistory 获取用户购票记录
func GetUserPurchaseHistory(userID int, eventID int) (bool, error) {
	ctx := context.Background()

	boughtKey := fmt.Sprintf("ticket:bought:%d", eventID)
	isMember, err := redis.Client.SIsMember(ctx, boughtKey, userID).Result()
	if err != nil {
		return false, err
	}

	return isMember, nil
}

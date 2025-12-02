package service

import (
	"context"
	"fmt"
	"seckill/pkg/logger"
	"seckill/pkg/rabbitmq"
	"seckill/pkg/redis" // 引入 Redis 包

	"go.uber.org/zap"
)

// SeckillV2 使用 Redis Lua 脚本进行原子扣减
func SeckillV2(userID int, productID int) (bool, string) {
	ctx := context.Background()

	// 1. 准备 Key
	// seckill:stock:1 (String 类型，存库存数)
	stockKey := fmt.Sprintf("seckill:stock:%d", productID)
	// seckill:bought:1 (Set 类型，存买到的用户ID)
	boughtKey := fmt.Sprintf("seckill:bought:%d", productID)

	// 2. 执行 Lua 脚本
	// Keys: [stockKey, boughtKey]
	// Args: [userID]
	result, err := redis.SeckillScript.Run(ctx, redis.Client,
		[]string{stockKey, boughtKey},
		userID).Int()

	if err != nil {
		logger.Log.Error("执行 Lua 脚本失败", zap.Error(err))
		return false, "系统繁忙，请稍后再试"
	}

	// 3. 处理 Lua 返回值
	switch result {
	case -1:
		// 对应 Lua 里的 return -1
		logger.Log.Warn("重复购买拦截", zap.Int("uid", userID))
		return false, "您已经抢购过了，请勿重复下单"
	case -2:
		// 对应 Lua 里的 return -2
		logger.Log.Warn("库存不足", zap.Int("pid", productID))
		return false, "手慢了，商品已抢光"
	case 1:
		// 对应 Lua 里的 return 1
		logger.Log.Info("Redis 抢购成功", zap.Int("uid", userID))

		// RabbitMQ 发送逻辑
		err := rabbitmq.SendSeckillMessage(int64(userID), int64(productID))
		if err != nil {
			return false, "订单创建失败，请稍后再试"
		}

		return true, "抢购成功！正在生成订单..."
	}

	return false, "未知错误"
}

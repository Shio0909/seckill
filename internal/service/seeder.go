package service

import (
	"context"
	"fmt"
	"time"

	"seckill/internal/model"
	"seckill/pkg/database"
	"seckill/pkg/logger"
	"seckill/pkg/redis"

	"go.uber.org/zap"
)

// InitProductData 负责初始化测试商品
func InitProductData() {
	var count int64
	if err := database.DB.Model(&model.Product{}).Count(&count).Error; err != nil {
		logger.Log.Error("查询商品数量失败", zap.Error(err))
		return
	}

	if count == 0 {
		logger.Log.Info("检测到数据库为空，正在初始化测试商品...")

		p := model.Product{
			Name:         "iPhone 15 Pro",
			Description:  "双十一特价抢购 iPhone 15 Pro 256G，手慢无！",  // 对应 Description
			ImageURL:     "http://image.test.com/iphone.jpg", // 对应 ImageURL
			Price:        8999.00,
			SeckillPrice: 1.00,
			Stock:        100,
			StartTime:    time.Now(),                     // 对应 StartTime (大写)
			EndTime:      time.Now().Add(24 * time.Hour), // 对应 EndTime (大写)
		}
		//1、写入mysql
		if err := database.DB.Create(&p).Error; err != nil {
			logger.Log.Error("初始化商品失败", zap.Error(err))
			return
		}
		logger.Log.Info("mysql数据写入成功", zap.Uint("id", p.ID))
		//2、库存预热：写入redis
		//key格式：seckill:stock:商品ID
		redisKey := fmt.Sprintf("seckill:stock:%d", p.ID)
		//写入redis，不设置过期时间
		err := redis.Client.Set(context.Background(), redisKey, p.Stock, 0).Err()
		if err != nil {
			logger.Log.Error("初始化商品库存到Redis失败", zap.Error(err))
			return
		}
		logger.Log.Info("Redis库存预热成功",
			zap.String("key", redisKey),
			zap.Int("stock", p.Stock),
		)
	}
}

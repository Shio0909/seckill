package redis

import (
	"context"
	"fmt"
	"log"
	"time"

	"seckill/pkg/config"

	"github.com/redis/go-redis/v9"
)

var Client *redis.Client

func InitRedis() {
	cfg := config.Get().Redis

	Client = redis.NewClient(&redis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := Client.Ping(ctx).Result()
	if err != nil {
		log.Fatalf("连接 Redis 失败: %v", err)
	}

	fmt.Printf("✅ Redis 连接成功 [%s]\n", cfg.Addr)
}

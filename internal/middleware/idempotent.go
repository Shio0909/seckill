package middleware

// ========================================================================
// 【重点学习】接口幂等性设计
// ========================================================================
// 什么是幂等性？
// 同一个请求执行一次和执行多次，对系统状态的影响是相同的
//
// 为什么需要幂等性？
// 1. 网络抖动导致客户端重试
// 2. 用户重复点击提交按钮
// 3. MQ 消息重复投递
// 4. 前端 bug 导致重复请求
//
// 不做幂等会怎样？
// - 订单重复创建
// - 库存重复扣减
// - 用户重复扣款
//
// 常见幂等方案：
// ┌─────────────────┬────────────────────────────────────────────────┐
// │ 方案            │ 适用场景                                        │
// ├─────────────────┼────────────────────────────────────────────────┤
// │ Token 机制      │ 表单提交（先获取 Token，提交时带上）             │
// │ 唯一索引        │ 数据库层面防重（如订单号唯一索引）               │
// │ 乐观锁/版本号   │ 更新操作（where version = x）                   │
// │ 分布式锁        │ 复杂业务场景                                    │
// │ 状态机          │ 订单状态流转（只能从 A->B，不能重复）            │
// └─────────────────┴────────────────────────────────────────────────┘
//
// 本实现采用：Token + Redis 方案
// 流程：
// 1. 客户端先调用 /api/idempotent/token 获取幂等令牌
// 2. 提交请求时在 Header 中带上令牌：X-Idempotent-Token: xxx
// 3. 服务端验证令牌，有效则处理请求并删除令牌
// 4. 令牌不存在或已被使用，返回重复请求错误
//
// 面试高频问题：
// Q1: 先删除 Token 还是先执行业务？
// A1: 这是一个经典问题。
//     - 先删除 Token：如果业务执行失败，Token 已删，用户无法重试（除非重新获取 Token）。
//     - 先执行业务：如果业务执行成功但删除 Token 失败，可能导致重复提交。
//     推荐方案：先删除 Token。如果业务失败，由客户端重新获取 Token 发起请求。
//     或者使用 Lua 脚本保证 "检查+删除" 的原子性，结合业务层的唯一索引兜底。
//
// Q2: Token 方案有什么缺点？
// A2: 需要多一次网络请求（获取 Token）。对于极致性能要求的秒杀场景，可能不是最优解。
//     秒杀场景通常利用 "用户ID + 商品ID + 活动ID" 作为唯一键，在 Redis 或数据库层面做去重。
//
// Q3: 数据库唯一索引能完全解决幂等吗？
// A3: 可以解决插入类的幂等（如创建订单）。但对于更新类操作（如扣减库存），需要配合乐观锁或分布式锁。
//     另外，分库分表场景下，唯一索引的维护成本较高。
// ========================================================================

import (
	"context"
	"fmt"
	"time"

	"seckill/pkg/e"
	"seckill/pkg/redis"
	"seckill/pkg/response"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	// 幂等令牌在 Redis 中的前缀
	IdempotentKeyPrefix = "idempotent:token:"
	// 令牌有效期（10分钟）
	IdempotentTokenExpire = 10 * time.Minute
	// 请求头中的令牌字段名
	IdempotentTokenHeader = "X-Idempotent-Token"
)

// ========================================================================
// 【重点学习】生成幂等令牌
// ========================================================================
// 使用 UUID 作为令牌，存入 Redis 设置过期时间
// 客户端获取后在有效期内使用
// ========================================================================

// GenerateIdempotentToken 生成幂等令牌
func GenerateIdempotentToken(c *gin.Context) {
	// 生成 UUID 作为令牌
	token := uuid.New().String()

	// 存入 Redis，设置过期时间
	key := IdempotentKeyPrefix + token
	ctx := context.Background()

	// ====================================================================
	// 【重点学习】使用 Redis SETNX（SET if Not eXists）
	// ====================================================================
	// SETNX 是原子操作，只有 key 不存在时才会设置成功
	// 结合 TTL 可以实现简单的分布式锁和令牌机制
	// ====================================================================
	err := redis.Client.SetNX(ctx, key, "1", IdempotentTokenExpire).Err()
	if err != nil {
		response.FailWithMsg(c, e.ERROR, "生成令牌失败")
		return
	}

	response.Success(c, gin.H{
		"token":      token,
		"expire":     IdempotentTokenExpire.Seconds(),
		"header_key": IdempotentTokenHeader,
	})
}

// ========================================================================
// 【重点学习】幂等性校验中间件
// ========================================================================
// 使用 Redis 的 DEL 命令实现"检查并删除"的原子性
// DEL 返回删除的 key 数量：
// - 返回 1：key 存在且被删除，说明是首次请求
// - 返回 0：key 不存在，说明是重复请求或令牌无效
// ========================================================================

// Idempotent 幂等性校验中间件
func Idempotent() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. 从请求头获取幂等令牌
		token := c.GetHeader(IdempotentTokenHeader)
		if token == "" {
			response.FailWithMsg(c, e.INVALID_PARAMS, "缺少幂等令牌，请先获取")
			c.Abort()
			return
		}

		// 2. 尝试删除 Redis 中的令牌
		key := IdempotentKeyPrefix + token
		ctx := context.Background()

		// ================================================================
		// 【重点学习】Redis DEL 的原子性
		// ================================================================
		// DEL 命令是原子的，不需要额外加锁
		// 即使并发请求，也只有一个能成功删除
		// 这是实现幂等的关键！
		// ================================================================
		deleted, err := redis.Client.Del(ctx, key).Result()
		if err != nil {
			response.FailWithMsg(c, e.ERROR, "系统错误")
			c.Abort()
			return
		}

		// 3. 判断删除结果
		if deleted == 0 {
			// 令牌不存在或已被使用
			response.FailWithMsg(c, e.ERROR, "请勿重复提交")
			c.Abort()
			return
		}

		// 4. 令牌有效，继续处理请求
		c.Next()
	}
}

// ========================================================================
// 【重点学习】基于业务 key 的幂等（更通用）
// ========================================================================
// 有些场景不方便预先获取 Token，可以根据业务参数生成唯一 key
// 如：用户ID + 商品ID + 操作类型 作为幂等 key
// ========================================================================

// IdempotentByKey 基于业务 key 的幂等中间件
// keyFunc: 从请求中提取幂等 key 的函数
// expire: key 的有效期
func IdempotentByKey(keyFunc func(*gin.Context) string, expire time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. 生成幂等 key
		bizKey := keyFunc(c)
		if bizKey == "" {
			c.Next() // 无法生成 key 时跳过幂等检查
			return
		}

		key := "idempotent:biz:" + bizKey
		ctx := context.Background()

		// 2. 尝试设置 key（SETNX）
		// ================================================================
		// 【重点学习】SETNX + TTL 实现分布式锁思想
		// ================================================================
		// 如果 key 不存在，设置成功，返回 true -> 首次请求
		// 如果 key 存在，设置失败，返回 false -> 重复请求
		// TTL 保证即使程序崩溃，key 也会自动过期
		// ================================================================
		success, err := redis.Client.SetNX(ctx, key, "1", expire).Result()
		if err != nil {
			response.FailWithMsg(c, e.ERROR, "系统错误")
			c.Abort()
			return
		}

		if !success {
			response.FailWithMsg(c, e.ERROR, "请求处理中，请勿重复提交")
			c.Abort()
			return
		}

		c.Next()
	}
}

// SeckillIdempotentKey 秒杀幂等 key 生成函数
// 使用 用户ID + 商品ID 作为幂等 key
func SeckillIdempotentKey(c *gin.Context) string {
	uid, exists := c.Get("uid")
	if !exists {
		return ""
	}
	productID := c.PostForm("product_id")
	if productID == "" {
		return ""
	}
	return fmt.Sprintf("%v:%s", uid, productID)
}

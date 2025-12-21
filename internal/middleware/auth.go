package middleware

import (
	"net/http"
	"seckill/pkg/utils"
	"strings"

	"github.com/gin-gonic/gin"
)

// JWTAuth 鉴权中间件
func JWTAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. 获取 Header 中的 Authorization
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未携带Token"})
			return
		}

		// 2. 格式通常是 "Bearer xxxxxxx"
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Token格式错误"})
			return
		}

		// 3. 解析 Token
		claims, err := utils.ParseToken(parts[1])
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Token无效或已过期"})
			return
		}

		// 4. 将 UserID 存入 Context，供后续接口使用
		// 使用自定义 Claims 类型直接访问字段
		c.Set("uid", int(claims.UserID))
		c.Set("username", claims.Username)

		c.Next()
	}
}

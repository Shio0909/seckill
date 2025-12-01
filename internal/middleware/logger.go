package middleware

import (
	"seckill/pkg/logger"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// zaplogger 中间件,接受gin的默认日志并用zap输出
func ZapLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery
		//处理请求
		c.Next()
		//处理完请求后记录耗时
		cost := time.Since(start)
		//获取状态码
		status := c.Writer.Status()
		//组装zap字段
		field := []zap.Field{
			zap.Int("status", status),
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.String("query", query),
			zap.String("ip", c.ClientIP()),
			zap.String("user-agent", c.Request.UserAgent()),
			zap.Duration("cost", cost),
		}
		//根据状态码不同，使用不同级别的日志记录
		if status >= 500 {
			logger.Log.Error("server error", field...)
		} else {
			logger.Log.Info("request success", field...)
		}
	}
}

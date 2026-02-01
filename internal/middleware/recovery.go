package middleware

import (
	"bytes"
	"fmt"
	"net/http"
	"runtime"

	"seckill/pkg/response"

	"github.com/gin-gonic/gin"
)

// Recovery 错误恢复中间件
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// 1. 捕获 panic 并打印错误信息
				var buf [4096]byte
				n := runtime.Stack(buf[:], false)
				stackInfo := string(buf[:n])

				// 获取 trace_id 用于日志关联
				traceID := c.GetString("trace_id")
				fmt.Printf("[Recovery] trace_id=%s panic=%v\nStack trace:\n%s\n",
					traceID, err, stackInfo)

				// 2. 检测是否是客户端断开连接导致的 panic
				// 常见于客户端主动取消请求
				if isBrokenPipe(err) {
					c.Abort()
					return
				}

				// 3. 返回统一的错误响应
				response.FailWithMsg(c, 500, "服务器内部错误")
				c.Abort()
			}
		}()
		c.Next()
	}
}

// isBrokenPipe 检测是否是客户端断开连接
func isBrokenPipe(err interface{}) bool {
	if err == nil {
		return false
	}

	errStr := fmt.Sprintf("%v", err)
	brokenPipeErrors := []string{
		"broken pipe",
		"connection reset by peer",
	}

	for _, s := range brokenPipeErrors {
		if bytes.Contains([]byte(errStr), []byte(s)) {
			return true
		}
	}
	return false
}

// RecoveryWithWriter 带自定义 writer 的 Recovery（可用于将错误写入特定位置）
func RecoveryWithWriter(out func(format string, args ...interface{})) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				var buf [4096]byte
				n := runtime.Stack(buf[:], false)
				stackInfo := string(buf[:n])

				traceID := c.GetString("trace_id")
				out("[Recovery] trace_id=%s panic=%v\nStack trace:\n%s\n",
					traceID, err, stackInfo)

				if isBrokenPipe(err) {
					c.Abort()
					return
				}

				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"code":    500,
					"message": "服务器内部错误",
				})
			}
		}()
		c.Next()
	}
}

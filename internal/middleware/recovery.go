package middleware

// ========================================================================
// 【重点学习】全局异常捕获中间件
// ========================================================================
// 发生 panic 时，捕获错误并返回标准错误响应
// 四个模块：
// 1. 捕获 panic
// 2. 堆栈追踪
// 3. 断连检测
// 4. 统一响应
//
// 为什么需要 Recovery？
// - Go 的 panic 会导致程序崩溃
// - 在 HTTP 服务中，一个请求的 panic 不应该影响其他请求
// - Recovery 中间件捕获 panic，记录日志，返回 500 错误
//
// 面试高频问题：
// Q1: Go 的 panic 和 recover 机制是怎样的？
// A1: panic 会停止当前 Goroutine 的正常执行，开始执行 defer 函数栈。
//     recover 只能在 defer 函数中调用，用于捕获 panic 并恢复正常执行。
//     注意：recover 无法捕获其他 Goroutine 抛出的 panic。
//
// Q2: 为什么 Gin 框架默认提供了 Recovery 中间件？
// A2: Web 服务器必须保证高可用。如果某个请求处理逻辑触发了 panic（如空指针引用），
//     如果没有 Recovery，整个进程会退出，导致服务不可用。
//     Recovery 确保单个请求的崩溃只影响该请求，不影响整个服务。
//
// Q3: 如何获取 panic 时的堆栈信息？
// A3: 使用 runtime.Stack(buf, false) 获取当前 Goroutine 的堆栈信息。
//     这对于排查线上问题至关重要。
// ========================================================================

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

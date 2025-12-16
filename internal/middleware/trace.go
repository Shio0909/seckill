package middleware

//生成traceid并存入gin.Context中
import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// TraceLogger 生成traceid,存入上下文
func TraceLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		//1、优先从请求头中获取traceid
		traceID := c.GetHeader("X-Trace-ID")
		//2、如果请求头中没有traceid，则生成一个新的traceid
		if traceID == "" {
			traceID = uuid.New().String()
		}
		//3、将traceid存入gin.Context中，方便后续处理使用
		c.Set("traceid", traceID)
		//4、放入响应头，并继续处理请求
		c.Writer.Header().Set("X-Trace-ID", traceID)
		c.Next()
	}
}

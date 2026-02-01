package tracing

import (
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
)

// HTTP 链路追踪是分布式追踪的入口点，负责：
// 1. 从请求 Header 中提取 TraceContext（如果有）
// 2. 创建 Server Span
// 3. 将 TraceID 注入到 Context 和 Response Header
// 4. 记录请求相关信息

// GinMiddleware 返回 Gin 链路追踪中间件
func GinMiddleware(serviceName string) gin.HandlerFunc {
	tracer := otel.Tracer(serviceName)
	propagator := otel.GetTextMapPropagator()

	return func(c *gin.Context) {
		// 【步骤1】从请求 Header 中提取 TraceContext
		ctx := propagator.Extract(c.Request.Context(), propagation.HeaderCarrier(c.Request.Header))

		// 【步骤2】创建 Server Span
		spanName := c.Request.Method + " " + c.FullPath()
		if c.FullPath() == "" {
			spanName = c.Request.Method + " " + c.Request.URL.Path
		}

		ctx, span := tracer.Start(ctx, spanName,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				// HTTP 相关属性（遵循 OpenTelemetry 语义约定）
				semconv.HTTPMethodKey.String(c.Request.Method),
				semconv.HTTPURLKey.String(c.Request.URL.String()),
				semconv.HTTPTargetKey.String(c.Request.URL.Path),
				semconv.HTTPHostKey.String(c.Request.Host),
				semconv.HTTPSchemeKey.String(c.Request.URL.Scheme),
				semconv.HTTPUserAgentKey.String(c.Request.UserAgent()),
				semconv.HTTPClientIPKey.String(c.ClientIP()),
			),
		)
		defer span.End()

		// 【步骤3】将 TraceID 设置到 Context 和 Response Header
		traceID := span.SpanContext().TraceID().String()
		c.Set("trace_id", traceID)
		c.Header("X-Trace-ID", traceID)

		// 将带 Span 的 Context 注入到 gin.Context
		c.Request = c.Request.WithContext(ctx)

		// 【步骤4】执行后续处理
		c.Next()

		// 【步骤5】记录响应信息
		statusCode := c.Writer.Status()
		span.SetAttributes(
			semconv.HTTPStatusCodeKey.Int(statusCode),
			attribute.Int("http.response_size", c.Writer.Size()),
		)

		// 根据状态码设置 Span 状态
		if statusCode >= 400 {
			span.SetStatus(codes.Error, "HTTP "+string(rune(statusCode)))
		} else {
			span.SetStatus(codes.Ok, "")
		}

		// 记录错误（如果有）
		if len(c.Errors) > 0 {
			span.SetAttributes(attribute.String("gin.errors", c.Errors.String()))
			for _, e := range c.Errors {
				span.RecordError(e.Err)
			}
		}
	}
}

// 【工具函数】

// InjectTraceHeader 将 TraceContext 注入到 HTTP Header（用于出站请求）
func InjectTraceHeader(c *gin.Context, header map[string]string) {
	propagator := otel.GetTextMapPropagator()
	carrier := propagation.MapCarrier(header)
	propagator.Inject(c.Request.Context(), carrier)
}

// GetTraceIDFromGin 从 Gin Context 获取 TraceID
func GetTraceIDFromGin(c *gin.Context) string {
	if traceID, exists := c.Get("trace_id"); exists {
		return traceID.(string)
	}
	return GetTraceID(c.Request.Context())
}

// 【重点】最佳实践
// 1. Span 命名：
//    - 使用 "HTTP METHOD /path" 格式
//    - 路径中的变量使用占位符（如 /users/{id}）
//
// 2. 属性记录：
//    - 遵循 OpenTelemetry 语义约定
//    - 不要记录敏感信息（密码、Token）
//    - 记录有助于排查问题的信息
//
// 3. 错误处理：
//    - 4xx 错误设置 Span 状态为 Error
//    - 记录错误原因和堆栈
//
// 4. 性能考虑：
//    - 采样策略减少数据量
//    - 异步上报不阻塞请求

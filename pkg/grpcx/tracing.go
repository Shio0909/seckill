package grpcx

import (
	"context"
	"fmt"
	"time"

	"seckill/pkg/tracing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// ========================================================================
// 【重点学习】gRPC 链路追踪拦截器
// ========================================================================
// gRPC 拦截器是实现链路追踪的关键，负责：
// 1. 客户端：在请求中注入 TraceContext
// 2. 服务端：从请求中提取 TraceContext
// 3. 创建 Span 记录 RPC 调用信息
//
// 📝 简历亮点：
// - 使用 OpenTelemetry 实现 gRPC 链路追踪
// - 理解 Context 传播机制
// - 自定义 Span 属性记录业务信息
//
// 🔥 面试高频：
// Q: gRPC 拦截器和 HTTP 中间件的区别？
// A: 1. gRPC 拦截器分为 Unary（一元）和 Stream（流式）
//    2. gRPC 使用 metadata 传递上下文（类似 HTTP Header）
//    3. gRPC 是强类型的，拦截器可以获取完整的请求响应结构
//
// Q: 如何在 gRPC 调用中传播 TraceID？
// A: 1. 客户端：将 TraceContext 注入到 gRPC metadata
//    2. 服务端：从 metadata 中提取 TraceContext
//    3. 使用 OpenTelemetry 的 Propagator 自动完成
//
// Q: gRPC 链路追踪需要记录哪些信息？
// A: 1. RPC 方法名（如 /user.UserService/GetUser）
//    2. 请求参数（注意敏感信息脱敏）
//    3. 响应状态码
//    4. 调用耗时
//    5. 错误信息（如果有）
// ========================================================================

// metadataCarrier 实现 OpenTelemetry 的 TextMapCarrier 接口
// 用于在 gRPC metadata 中传播 TraceContext
type metadataCarrier struct {
	md metadata.MD
}

// Get 获取 metadata 中的值
func (c *metadataCarrier) Get(key string) string {
	values := c.md.Get(key)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// Set 设置 metadata 中的值
func (c *metadataCarrier) Set(key, value string) {
	c.md.Set(key, value)
}

// Keys 返回所有 key
func (c *metadataCarrier) Keys() []string {
	keys := make([]string, 0, len(c.md))
	for k := range c.md {
		keys = append(keys, k)
	}
	return keys
}

// ========================================================================
// 【重点】客户端链路追踪拦截器
// ========================================================================

// TracingClientInterceptor 客户端链路追踪拦截器
// 负责：
// 1. 创建客户端 Span
// 2. 将 TraceContext 注入到 metadata
// 3. 记录 RPC 调用结果
func TracingClientInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{},
		cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {

		// 获取 Tracer
		tracer := otel.Tracer("grpc-client")

		// 【步骤1】创建客户端 Span
		ctx, span := tracer.Start(ctx, method,
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(
				semconv.RPCSystemKey.String("grpc"),
				semconv.RPCMethodKey.String(method),
				attribute.String("rpc.target", cc.Target()),
			),
		)
		defer span.End()

		// 【步骤2】将 TraceContext 注入到 metadata
		// 这样服务端就能接收到 TraceID
		md, ok := metadata.FromOutgoingContext(ctx)
		if !ok {
			md = metadata.MD{}
		} else {
			md = md.Copy()
		}

		// 使用 OpenTelemetry Propagator 注入 TraceContext
		propagator := otel.GetTextMapPropagator()
		propagator.Inject(ctx, &metadataCarrier{md: md})

		// 将注入后的 metadata 放回 context
		ctx = metadata.NewOutgoingContext(ctx, md)

		// 【步骤3】执行 RPC 调用
		start := time.Now()
		err := invoker(ctx, method, req, reply, cc, opts...)
		duration := time.Since(start)

		// 【步骤4】记录调用结果
		span.SetAttributes(
			attribute.Int64("rpc.duration_ms", duration.Milliseconds()),
		)

		if err != nil {
			// 记录错误
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())

			// 解析 gRPC 状态码
			if st, ok := status.FromError(err); ok {
				span.SetAttributes(
					semconv.RPCGRPCStatusCodeKey.Int(int(st.Code())),
				)
			}
		} else {
			span.SetStatus(codes.Ok, "")
			span.SetAttributes(
				semconv.RPCGRPCStatusCodeKey.Int(0),
			)
		}

		return err
	}
}

// ========================================================================
// 【重点】服务端链路追踪拦截器
// ========================================================================

// TracingServerInterceptor 服务端链路追踪拦截器
// 负责：
// 1. 从 metadata 中提取 TraceContext
// 2. 创建服务端 Span
// 3. 记录处理结果
func TracingServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler) (interface{}, error) {

		// 【步骤1】从 metadata 中提取 TraceContext
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			md = metadata.MD{}
		}

		// 使用 Propagator 从 metadata 中提取 TraceContext
		propagator := otel.GetTextMapPropagator()
		ctx = propagator.Extract(ctx, &metadataCarrier{md: md})

		// 【步骤2】创建服务端 Span
		tracer := otel.Tracer("grpc-server")
		ctx, span := tracer.Start(ctx, info.FullMethod,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				semconv.RPCSystemKey.String("grpc"),
				semconv.RPCMethodKey.String(info.FullMethod),
				// 记录 TraceID 便于日志关联
				attribute.String("trace_id", tracing.GetTraceID(ctx)),
			),
		)
		defer span.End()

		// 【步骤3】执行处理器
		start := time.Now()
		resp, err := handler(ctx, req)
		duration := time.Since(start)

		// 【步骤4】记录处理结果
		span.SetAttributes(
			attribute.Int64("rpc.duration_ms", duration.Milliseconds()),
		)

		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())

			if st, ok := status.FromError(err); ok {
				span.SetAttributes(
					semconv.RPCGRPCStatusCodeKey.Int(int(st.Code())),
				)
			}
		} else {
			span.SetStatus(codes.Ok, "")
			span.SetAttributes(
				semconv.RPCGRPCStatusCodeKey.Int(0),
			)
		}

		return resp, err
	}
}

// ========================================================================
// 【扩展】流式 RPC 链路追踪拦截器
// ========================================================================

// TracingStreamClientInterceptor 流式客户端拦截器
func TracingStreamClientInterceptor() grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn,
		method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {

		tracer := otel.Tracer("grpc-client")
		ctx, span := tracer.Start(ctx, method,
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(
				semconv.RPCSystemKey.String("grpc"),
				semconv.RPCMethodKey.String(method),
				attribute.Bool("rpc.is_streaming", true),
			),
		)

		// 注入 TraceContext
		md, ok := metadata.FromOutgoingContext(ctx)
		if !ok {
			md = metadata.MD{}
		} else {
			md = md.Copy()
		}
		otel.GetTextMapPropagator().Inject(ctx, &metadataCarrier{md: md})
		ctx = metadata.NewOutgoingContext(ctx, md)

		stream, err := streamer(ctx, desc, cc, method, opts...)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			span.End()
			return nil, err
		}

		// 返回包装的 stream，在 stream 关闭时结束 span
		return &tracingClientStream{
			ClientStream: stream,
			span:         span,
		}, nil
	}
}

// tracingClientStream 包装的客户端流
type tracingClientStream struct {
	grpc.ClientStream
	span trace.Span
}

func (s *tracingClientStream) CloseSend() error {
	err := s.ClientStream.CloseSend()
	if err != nil {
		s.span.RecordError(err)
		s.span.SetStatus(codes.Error, err.Error())
	} else {
		s.span.SetStatus(codes.Ok, "")
	}
	s.span.End()
	return err
}

// TracingStreamServerInterceptor 流式服务端拦截器
func TracingStreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo,
		handler grpc.StreamHandler) error {

		// 提取 TraceContext
		ctx := ss.Context()
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			md = metadata.MD{}
		}

		propagator := otel.GetTextMapPropagator()
		ctx = propagator.Extract(ctx, &metadataCarrier{md: md})

		tracer := otel.Tracer("grpc-server")
		ctx, span := tracer.Start(ctx, info.FullMethod,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				semconv.RPCSystemKey.String("grpc"),
				semconv.RPCMethodKey.String(info.FullMethod),
				attribute.Bool("rpc.is_streaming", true),
			),
		)
		defer span.End()

		// 执行处理器
		err := handler(srv, &tracingServerStream{ServerStream: ss, ctx: ctx})
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		} else {
			span.SetStatus(codes.Ok, "")
		}

		return err
	}
}

// tracingServerStream 包装的服务端流
type tracingServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *tracingServerStream) Context() context.Context {
	return s.ctx
}

// ========================================================================
// 【工具函数】创建带追踪的拦截器链
// ========================================================================

// WithTracingClientInterceptors 返回带追踪的客户端拦截器
func WithTracingClientInterceptors() []grpc.DialOption {
	return []grpc.DialOption{
		grpc.WithChainUnaryInterceptor(
			TracingClientInterceptor(),
			LoggingInterceptor(),
		),
		grpc.WithChainStreamInterceptor(
			TracingStreamClientInterceptor(),
		),
	}
}

// WithTracingServerInterceptors 返回带追踪的服务端选项
func WithTracingServerInterceptors() []grpc.ServerOption {
	return []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(
			TracingServerInterceptor(),
			ServerLoggingInterceptor(),
			ServerRecoveryInterceptor(),
		),
		grpc.ChainStreamInterceptor(
			TracingStreamServerInterceptor(),
		),
	}
}

// ========================================================================
// 【重点】在业务代码中添加 Span 属性
// ========================================================================

// AddUserAttributes 添加用户相关属性到当前 Span
func AddUserAttributes(ctx context.Context, userID int64, username string) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.SetAttributes(
			attribute.Int64("user.id", userID),
			attribute.String("user.name", username),
		)
	}
}

// AddBusinessEvent 添加业务事件到当前 Span
func AddBusinessEvent(ctx context.Context, name string, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.AddEvent(name, trace.WithAttributes(attrs...))
	}
}

// RecordError 记录错误到当前 Span
func RecordError(ctx context.Context, err error, description string) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() && err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, description)
	}
}

// ========================================================================
// 【重点】链路追踪最佳实践
// ========================================================================
// 1. Span 命名规范：
//    - 格式：<service>/<operation>
//    - 示例：user-service/GetUser
//    - 避免包含可变参数（如用户 ID）
//
// 2. 属性命名规范：
//    - 使用 OpenTelemetry 语义约定
//    - 自定义属性使用业务域前缀（如 seckill.）
//
// 3. 敏感信息处理：
//    - 不要记录密码、Token 等敏感信息
//    - 对手机号、身份证等进行脱敏
//
// 4. 性能优化：
//    - 使用采样减少数据量
//    - 大量属性会增加传输开销
//    - 避免在热点路径记录过多信息
//
// 5. 错误处理：
//    - 记录错误类型和消息
//    - 设置 Span 状态为 Error
//    - 添加错误发生的上下文
// ========================================================================

// Example 使用示例
func Example() {
	// 在 gRPC 服务端 handler 中：
	/*
		func (s *UserService) GetUser(ctx context.Context, req *pb.GetUserRequest) (*pb.GetUserResponse, error) {
			// 添加用户属性
			AddUserAttributes(ctx, req.UserId, "")

			// 添加业务事件
			AddBusinessEvent(ctx, "query_user_start",
				attribute.Int64("user_id", req.UserId),
			)

			// 查询数据库
			user, err := s.repo.GetUser(ctx, req.UserId)
			if err != nil {
				RecordError(ctx, err, "查询用户失败")
				return nil, err
			}

			AddBusinessEvent(ctx, "query_user_success")

			return &pb.GetUserResponse{User: user}, nil
		}
	*/
	fmt.Println("See code comments for usage example")
}

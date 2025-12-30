package tracing

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ========================================================================
// 【重点学习】分布式链路追踪（Distributed Tracing）
// ========================================================================
// 在微服务架构中，一个请求可能会经过多个服务，链路追踪帮助我们：
// 1. 可视化请求在各服务间的流转路径
// 2. 定位性能瓶颈（哪个服务/方法耗时最长）
// 3. 排查分布式系统中的问题
// 4. 分析服务依赖关系
//
// 📝 简历亮点：
// - 使用 OpenTelemetry 实现分布式链路追踪
// - 集成 Jaeger 进行链路可视化
// - 自动 Context 传播和 TraceID 透传
//
// 🔥 面试高频：
// Q: 什么是分布式链路追踪？核心概念有哪些？
// A: 分布式链路追踪是跟踪请求在分布式系统中流转的技术。核心概念：
//    - Trace（追踪）：代表一个完整的请求链路，由一个唯一的 TraceID 标识
//    - Span（跨度）：链路中的一个操作单元，有开始和结束时间
//    - SpanContext：跨进程传播的上下文信息
//    - Baggage：跨服务传递的业务数据
//
// Q: OpenTelemetry 和 OpenTracing、OpenCensus 的关系？
// A: OpenTelemetry 是 CNCF 项目，由 OpenTracing 和 OpenCensus 合并而来
//    它提供了统一的 API 规范，支持 Traces、Metrics、Logs 三大支柱
//    目前是云原生可观测性的事实标准
//
// Q: 如何在微服务间传播 TraceID？
// A: 1. HTTP：通过 Header 传播（如 traceparent、x-trace-id）
//    2. gRPC：通过 Metadata 传播
//    3. MQ：通过消息 Header 传播
//    OpenTelemetry 提供了自动传播机制（W3C Trace Context）
//
// Q: 链路追踪对性能有影响吗？如何降低影响？
// A: 会有一定影响，主要体现在：
//    1. 序列化/反序列化开销
//    2. 网络传输开销（上报 Span 数据）
//    优化方式：
//    1. 采样：只追踪部分请求（如 1%）
//    2. 异步上报：不阻塞主流程
//    3. 批量发送：减少网络调用次数
// ========================================================================

// Config 链路追踪配置
type Config struct {
	ServiceName    string  // 服务名称
	ServiceVersion string  // 服务版本
	Environment    string  // 环境：dev/staging/prod
	JaegerEndpoint string  // Jaeger OTLP 端点，如 "localhost:4317"
	SampleRate     float64 // 采样率，0.0-1.0，1.0 表示全量采样
	Enabled        bool    // 是否启用
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		ServiceName:    "seckill-service",
		ServiceVersion: "1.0.0",
		Environment:    "dev",
		JaegerEndpoint: "localhost:4317",
		SampleRate:     1.0, // 开发环境全量采样
		Enabled:        true,
	}
}

// TracerProvider 追踪器提供者封装
type TracerProvider struct {
	provider *sdktrace.TracerProvider
	config   *Config
}

// ========================================================================
// 【重点】初始化链路追踪
// ========================================================================
// 初始化流程：
// 1. 创建 Resource（标识服务信息）
// 2. 创建 Exporter（数据导出器，发送到 Jaeger）
// 3. 创建 TracerProvider（管理 Tracer）
// 4. 设置全局 TracerProvider 和 Propagator
// ========================================================================

// InitTracer 初始化链路追踪
func InitTracer(cfg *Config) (*TracerProvider, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	if !cfg.Enabled {
		// 未启用时返回 noop provider
		return &TracerProvider{config: cfg}, nil
	}

	ctx := context.Background()

	// 【步骤1】创建 Resource
	// Resource 描述了产生遥测数据的实体（服务信息）
	res, err := resource.New(ctx,
		resource.WithAttributes(
			// 服务基本信息
			semconv.ServiceNameKey.String(cfg.ServiceName),
			semconv.ServiceVersionKey.String(cfg.ServiceVersion),
			// 部署环境
			semconv.DeploymentEnvironmentKey.String(cfg.Environment),
			// 自定义属性
			attribute.String("service.team", "backend"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("创建 resource 失败: %w", err)
	}

	// 【步骤2】创建 OTLP Exporter（通过 gRPC 发送到 Jaeger）
	// OTLP (OpenTelemetry Protocol) 是 OpenTelemetry 的原生协议
	conn, err := grpc.NewClient(cfg.JaegerEndpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("连接 Jaeger 失败: %w", err)
	}

	exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
	if err != nil {
		return nil, fmt.Errorf("创建 exporter 失败: %w", err)
	}

	// 【步骤3】创建 TracerProvider
	// 配置采样策略和 Span 处理器
	tp := sdktrace.NewTracerProvider(
		// 设置 Resource
		sdktrace.WithResource(res),
		// 使用批量处理器（提高性能）
		sdktrace.WithBatcher(exporter,
			// 批量发送配置
			sdktrace.WithBatchTimeout(5*time.Second),
			sdktrace.WithMaxExportBatchSize(512),
		),
		// 设置采样率
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(cfg.SampleRate)),
	)

	// 【步骤4】设置全局 TracerProvider
	otel.SetTracerProvider(tp)

	// 【步骤5】设置全局 Propagator（用于跨服务传播 Context）
	// W3C Trace Context 是标准的传播格式
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, // W3C Trace Context
		propagation.Baggage{},      // W3C Baggage
	))

	return &TracerProvider{
		provider: tp,
		config:   cfg,
	}, nil
}

// Shutdown 关闭追踪器（优雅关闭时调用）
func (t *TracerProvider) Shutdown(ctx context.Context) error {
	if t.provider == nil {
		return nil
	}
	return t.provider.Shutdown(ctx)
}

// ========================================================================
// 【重点】获取 Tracer 和创建 Span
// ========================================================================
// Tracer 是创建 Span 的工厂
// Span 代表一个操作，包含名称、开始时间、结束时间、属性等
// ========================================================================

// GetTracer 获取指定名称的 Tracer
func GetTracer(name string) trace.Tracer {
	return otel.Tracer(name)
}

// StartSpan 开始一个新的 Span
// 使用方式：
//
//	ctx, span := tracing.StartSpan(ctx, "operationName")
//	defer span.End()
func StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return GetTracer("seckill").Start(ctx, name, opts...)
}

// StartSpanWithTracer 使用指定 Tracer 开始 Span
func StartSpanWithTracer(ctx context.Context, tracerName, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return GetTracer(tracerName).Start(ctx, spanName, opts...)
}

// ========================================================================
// 【重点】Span 属性和事件
// ========================================================================
// 可以给 Span 添加属性和事件，便于问题排查：
// - 属性（Attributes）：键值对，描述操作的特征
// - 事件（Events）：时间点标记，记录特定时刻发生的事情
// ========================================================================

// SpanAttributes 常用 Span 属性键
var (
	// 用户相关
	AttrUserID   = attribute.Key("user.id")
	AttrUsername = attribute.Key("user.name")

	// 业务相关
	AttrProductID = attribute.Key("product.id")
	AttrOrderID   = attribute.Key("order.id")
	AttrQuantity  = attribute.Key("quantity")

	// 结果相关
	AttrSuccess = attribute.Key("success")
	AttrReason  = attribute.Key("reason")
)

// AddSpanAttributes 给当前 Span 添加属性
func AddSpanAttributes(ctx context.Context, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.SetAttributes(attrs...)
	}
}

// RecordSpanError 记录错误到 Span
func RecordSpanError(ctx context.Context, err error) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() && err != nil {
		span.RecordError(err)
	}
}

// AddSpanEvent 添加事件到 Span
func AddSpanEvent(ctx context.Context, name string, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.AddEvent(name, trace.WithAttributes(attrs...))
	}
}

// ========================================================================
// 【重点】TraceID 和 SpanID 获取
// ========================================================================
// 用于日志关联：在日志中打印 TraceID，便于查找完整链路
// ========================================================================

// GetTraceID 从 Context 获取 TraceID
func GetTraceID(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().HasTraceID() {
		return span.SpanContext().TraceID().String()
	}
	return ""
}

// GetSpanID 从 Context 获取 SpanID
func GetSpanID(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().HasSpanID() {
		return span.SpanContext().SpanID().String()
	}
	return ""
}

// ========================================================================
// 【重点学习】采样策略
// ========================================================================
// 生产环境不可能追踪所有请求，需要采样策略：
// 1. AlwaysSample：全量采样（开发环境）
// 2. NeverSample：不采样
// 3. TraceIDRatioBased：按比例采样（如 1%）
// 4. ParentBased：基于父 Span 决定
// 5. 自定义采样：根据请求特征决定（如 VIP 用户全量）
//
// 🔥 面试题：
// Q: 如何设计一个智能采样策略？
// A: 1. 错误请求必采样（便于排查问题）
//    2. 慢请求必采样（性能分析）
//    3. 正常请求按比例采样
//    4. 特定用户/业务全量采样（调试）
// ========================================================================

// CustomSampler 自定义采样器示例
// 实现 sdktrace.Sampler 接口
type CustomSampler struct {
	baseRate float64 // 基础采样率
}

// NewCustomSampler 创建自定义采样器
func NewCustomSampler(baseRate float64) *CustomSampler {
	return &CustomSampler{baseRate: baseRate}
}

// ShouldSample 决定是否采样
func (s *CustomSampler) ShouldSample(params sdktrace.SamplingParameters) sdktrace.SamplingResult {
	// 示例：根据 Span 名称决定采样策略
	// 秒杀相关操作全量采样
	if params.Name == "seckill.deduct_stock" || params.Name == "seckill.create_order" {
		return sdktrace.SamplingResult{
			Decision:   sdktrace.RecordAndSample,
			Tracestate: trace.SpanContextFromContext(params.ParentContext).TraceState(),
		}
	}

	// 其他按基础采样率
	return sdktrace.TraceIDRatioBased(s.baseRate).ShouldSample(params)
}

// Description 返回采样器描述
func (s *CustomSampler) Description() string {
	return fmt.Sprintf("CustomSampler{baseRate=%f}", s.baseRate)
}

// ========================================================================
// 【重点】常用追踪场景封装
// ========================================================================

// TraceHTTPRequest HTTP 请求追踪
func TraceHTTPRequest(ctx context.Context, method, path string) (context.Context, trace.Span) {
	return StartSpan(ctx, fmt.Sprintf("HTTP %s %s", method, path),
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(
			semconv.HTTPMethodKey.String(method),
			semconv.HTTPTargetKey.String(path),
		),
	)
}

// TraceGRPCCall gRPC 调用追踪
func TraceGRPCCall(ctx context.Context, service, method string) (context.Context, trace.Span) {
	return StartSpan(ctx, fmt.Sprintf("gRPC %s/%s", service, method),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			semconv.RPCServiceKey.String(service),
			semconv.RPCMethodKey.String(method),
			semconv.RPCSystemKey.String("grpc"),
		),
	)
}

// TraceDBQuery 数据库查询追踪
func TraceDBQuery(ctx context.Context, operation, table string) (context.Context, trace.Span) {
	return StartSpan(ctx, fmt.Sprintf("DB %s %s", operation, table),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			semconv.DBSystemKey.String("mysql"),
			semconv.DBOperationKey.String(operation),
			semconv.DBSQLTableKey.String(table),
		),
	)
}

// TraceRedisCommand Redis 命令追踪
func TraceRedisCommand(ctx context.Context, cmd string) (context.Context, trace.Span) {
	return StartSpan(ctx, fmt.Sprintf("Redis %s", cmd),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			semconv.DBSystemKey.String("redis"),
			semconv.DBOperationKey.String(cmd),
		),
	)
}

// TraceMQPublish 消息发布追踪
func TraceMQPublish(ctx context.Context, queue string) (context.Context, trace.Span) {
	return StartSpan(ctx, fmt.Sprintf("MQ Publish %s", queue),
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(
			attribute.String("messaging.system", "rabbitmq"),
			attribute.String("messaging.destination", queue),
		),
	)
}

// TraceMQConsume 消息消费追踪
func TraceMQConsume(ctx context.Context, queue string) (context.Context, trace.Span) {
	return StartSpan(ctx, fmt.Sprintf("MQ Consume %s", queue),
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String("messaging.system", "rabbitmq"),
			attribute.String("messaging.destination", queue),
		),
	)
}

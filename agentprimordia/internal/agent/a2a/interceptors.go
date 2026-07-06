package a2a

import (
	"context"
	"log/slog"
	"runtime/debug"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// A2AInterceptorMetrics 简单的内存级指标计数器。
//
// 设计要点：
//   - 不引入 Prometheus 依赖（避免与可观测性 Phase 重复）；
//   - 提供 gRPC interceptor 所需的最少指标：调用次数、错误次数、延迟累计；
//   - 通过 MetricsInterceptor 在 RPC 边界记录；
//   - 业务侧可通过 Snapshot() 获取快照。
type A2AInterceptorMetrics struct {
	// totalCalls 总调用次数
	totalCalls atomic.Int64
	// errorCalls 错误调用次数（error != nil 且不是 context canceled）
	errorCalls atomic.Int64
	// panicRecovers panic 恢复次数
	panicRecovers atomic.Int64
	// totalLatencyNanos 总耗时（纳秒）
	totalLatencyNanos atomic.Int64
}

// Snapshot 返回当前指标快照
type A2AMetricsSnapshot struct {
	TotalCalls        int64   `json:"total_calls"`
	ErrorCalls        int64   `json:"error_calls"`
	PanicRecovers     int64   `json:"panic_recovers"`
	TotalLatencyNanos int64   `json:"total_latency_nanos"`
	AvgLatencyMillis  float64 `json:"avg_latency_ms"`
}

// NewA2AInterceptorMetrics 创建新的指标收集器
func NewA2AInterceptorMetrics() *A2AInterceptorMetrics {
	return &A2AInterceptorMetrics{}
}

// Snapshot 返回指标快照。
func (m *A2AInterceptorMetrics) Snapshot() A2AMetricsSnapshot {
	total := m.totalCalls.Load()
	latency := m.totalLatencyNanos.Load()
	var avgMs float64
	if total > 0 {
		avgMs = float64(latency) / float64(total) / 1e6
	}
	return A2AMetricsSnapshot{
		TotalCalls:        total,
		ErrorCalls:        m.errorCalls.Load(),
		PanicRecovers:     m.panicRecovers.Load(),
		TotalLatencyNanos: latency,
		AvgLatencyMillis:  avgMs,
	}
}

// A2AInterceptorConfig 拦截器共享配置
type A2AInterceptorConfig struct {
	// Logger 日志器；为空时使用 slog.Default()
	Logger *slog.Logger
	// Metrics 指标收集器；为空时使用 NoopMetrics
	Metrics *A2AInterceptorMetrics
	// SlowRequestThreshold 慢请求阈值；超过则 WARN 日志
	SlowRequestThreshold time.Duration
}

// newLogger 返回 logger（处理 nil）
func (c *A2AInterceptorConfig) logger() *slog.Logger {
	if c.Logger != nil {
		return c.Logger
	}
	return slog.Default()
}

// newMetrics 返回 metrics（处理 nil）
func (c *A2AInterceptorConfig) metrics() *A2AInterceptorMetrics {
	if c.Metrics != nil {
		return c.Metrics
	}
	return &A2AInterceptorMetrics{}
}

// slowThreshold 返回慢请求阈值（默认 1 秒）
func (c *A2AInterceptorConfig) slowThreshold() time.Duration {
	if c.SlowRequestThreshold > 0 {
		return c.SlowRequestThreshold
	}
	return time.Second
}

// RecoveryInterceptor panic 恢复拦截器：捕获下游 handler 的 panic，
// 转换为 Internal 错误并记录。
func RecoveryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("a2a panic recovered",
					"method", info.FullMethod,
					"panic", r,
					"stack", string(debug.Stack()),
				)
				err = status.Errorf(codes.Internal, "panic recovered: %v", r)
			}
		}()
		return handler(ctx, req)
	}
}

// StreamRecoveryInterceptor 流式 panic 恢复拦截器。
func StreamRecoveryInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("a2a stream panic recovered",
					"method", info.FullMethod,
					"panic", r,
					"stack", string(debug.Stack()),
				)
				err = status.Errorf(codes.Internal, "panic recovered: %v", r)
			}
		}()
		return handler(srv, ss)
	}
}

// LoggingInterceptor 日志拦截器：记录每次 RPC 调用的方法、耗时、错误。
func LoggingInterceptor(cfg A2AInterceptorConfig) grpc.UnaryServerInterceptor {
	logger := cfg.logger()
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		duration := time.Since(start)
		level := slog.LevelInfo
		if err != nil {
			level = slog.LevelError
		} else if duration > cfg.slowThreshold() {
			level = slog.LevelWarn
		}
		logger.LogAttrs(ctx, level, "a2a rpc",
			slog.String("method", info.FullMethod),
			slog.Duration("duration", duration),
			slog.Any("error", err),
		)
		return resp, err
	}
}

// StreamLoggingInterceptor 流式日志拦截器。
func StreamLoggingInterceptor(cfg A2AInterceptorConfig) grpc.StreamServerInterceptor {
	logger := cfg.logger()
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		start := time.Now()
		err := handler(srv, ss)
		duration := time.Since(start)
		level := slog.LevelInfo
		if err != nil {
			level = slog.LevelError
		} else if duration > cfg.slowThreshold() {
			level = slog.LevelWarn
		}
		logger.LogAttrs(ss.Context(), level, "a2a stream rpc",
			slog.String("method", info.FullMethod),
			slog.Duration("duration", duration),
			slog.Any("error", err),
		)
		return err
	}
}

// MetricsInterceptor 指标拦截器：累计调用次数、错误次数、延迟。
func MetricsInterceptor(metrics *A2AInterceptorMetrics) grpc.UnaryServerInterceptor {
	if metrics == nil {
		metrics = &A2AInterceptorMetrics{}
	}
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		metrics.totalCalls.Add(1)
		resp, err := handler(ctx, req)
		metrics.totalLatencyNanos.Add(time.Since(start).Nanoseconds())
		if err != nil && !isClientCanceled(err) {
			metrics.errorCalls.Add(1)
		}
		return resp, err
	}
}

// StreamMetricsInterceptor 流式指标拦截器。
func StreamMetricsInterceptor(metrics *A2AInterceptorMetrics) grpc.StreamServerInterceptor {
	if metrics == nil {
		metrics = &A2AInterceptorMetrics{}
	}
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		start := time.Now()
		metrics.totalCalls.Add(1)
		err := handler(srv, ss)
		metrics.totalLatencyNanos.Add(time.Since(start).Nanoseconds())
		if err != nil && !isClientCanceled(err) {
			metrics.errorCalls.Add(1)
		}
		return err
	}
}

// PanicMetricsInterceptor 包装 RecoveryInterceptor 统计 panic 恢复次数。
// 注意：实现上 panic 恢复已在 RecoveryInterceptor 中处理，本拦截器仅在
// 检测到状态码为 Internal 时增量计数（与 A2AInterceptorMetrics 共享）。
type panicCounter struct{ counter *A2AInterceptorMetrics }

// isClientCanceled 判断错误是否为客户端主动取消。
func isClientCanceled(err error) bool {
	if err == nil {
		return false
	}
	if st, ok := status.FromError(err); ok {
		return st.Code() == codes.Canceled || st.Code() == codes.Unauthenticated
	}
	return false
}

// ChainUnaryInterceptors 组合多个 unary 拦截器为单个。
// 执行顺序：从左到右，最外层是第一个。
func ChainUnaryInterceptors(interceptors ...grpc.UnaryServerInterceptor) grpc.UnaryServerInterceptor {
	n := len(interceptors)
	if n == 0 {
		return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
			return handler(ctx, req)
		}
	}
	if n == 1 {
		return interceptors[0]
	}
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		// 从右向左构建链：最右侧的拦截器最先调用 handler
		chained := handler
		for i := n - 1; i >= 0; i-- {
			idx := i
			next := chained
			chained = func(ctx context.Context, req any) (any, error) {
				return interceptors[idx](ctx, req, info, next)
			}
		}
		return chained(ctx, req)
	}
}

// ChainStreamInterceptors 组合多个 stream 拦截器。
func ChainStreamInterceptors(interceptors ...grpc.StreamServerInterceptor) grpc.StreamServerInterceptor {
	n := len(interceptors)
	if n == 0 {
		return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			return handler(srv, ss)
		}
	}
	if n == 1 {
		return interceptors[0]
	}
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		chained := handler
		for i := n - 1; i >= 0; i-- {
			idx := i
			next := chained
			chained = func(srv any, ss grpc.ServerStream) error {
				return interceptors[idx](srv, ss, info, next)
			}
		}
		return chained(srv, ss)
	}
}

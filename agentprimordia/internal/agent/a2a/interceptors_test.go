package a2a

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// 辅助函数：构造一个简单的 unary handler 返回值
func okHandler(_ context.Context, _ any) (any, error) {
	return "ok", nil
}

func errHandler(_ context.Context, _ any) (any, error) {
	return nil, status.Error(codes.Internal, "test error")
}

func panicHandler(_ context.Context, _ any) (any, error) {
	panic("boom")
}

type dummyInfo struct{ method string }

func (d *dummyInfo) FullMethod() string { return d.method }

// TestRecoveryInterceptor 测试 panic 恢复
func TestRecoveryInterceptor(t *testing.T) {
	interceptor := RecoveryInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/test/method"}

	resp, err := interceptor(context.Background(), nil, info, panicHandler)
	if err == nil {
		t.Fatal("期望 panic 后返回错误，但得到 nil")
	}
	if resp != nil {
		t.Errorf("panic 恢复后响应应为 nil，实际: %v", resp)
	}
	if st, ok := status.FromError(err); !ok || st.Code() != codes.Internal {
		t.Errorf("期望 Internal 错误，实际: %v", err)
	}
}

// TestRecoveryInterceptor_NoPanic 正常流程不应受影响
func TestRecoveryInterceptor_NoPanic(t *testing.T) {
	interceptor := RecoveryInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/test/method"}

	resp, err := interceptor(context.Background(), nil, info, okHandler)
	if err != nil {
		t.Errorf("正常调用不应有错误: %v", err)
	}
	if resp != "ok" {
		t.Errorf("resp = %v, 期望 'ok'", resp)
	}
}

// TestLoggingInterceptor 测试日志拦截器记录耗时和错误
func TestLoggingInterceptor(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	cfg := A2AInterceptorConfig{
		Logger:               logger,
		SlowRequestThreshold: 50 * time.Millisecond,
	}
	interceptor := LoggingInterceptor(cfg)
	info := &grpc.UnaryServerInfo{FullMethod: "/test/method"}

	// 正常调用
	_, _ = interceptor(context.Background(), nil, info, okHandler)
	if !strings.Contains(buf.String(), "/test/method") {
		t.Error("日志应包含方法名")
	}
	if !strings.Contains(buf.String(), "duration") {
		t.Error("日志应包含 duration")
	}

	// 错误调用
	buf.Reset()
	_, _ = interceptor(context.Background(), nil, info, errHandler)
	if !strings.Contains(buf.String(), "level=ERROR") {
		t.Error("错误调用应记为 ERROR 级别")
	}
}

// TestLoggingInterceptor_SlowRequest 测试慢请求日志
func TestLoggingInterceptor_SlowRequest(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	cfg := A2AInterceptorConfig{
		Logger:               logger,
		SlowRequestThreshold: 10 * time.Millisecond,
	}
	interceptor := LoggingInterceptor(cfg)
	info := &grpc.UnaryServerInfo{FullMethod: "/test/slow"}

	slowHandler := func(_ context.Context, _ any) (any, error) {
		time.Sleep(50 * time.Millisecond)
		return nil, nil
	}
	_, _ = interceptor(context.Background(), nil, info, slowHandler)
	if !strings.Contains(buf.String(), "level=WARN") {
		t.Error("慢请求应记为 WARN 级别")
	}
}

// TestMetricsInterceptor 测试指标拦截器
func TestMetricsInterceptor(t *testing.T) {
	metrics := &A2AInterceptorMetrics{}
	interceptor := MetricsInterceptor(metrics)
	info := &grpc.UnaryServerInfo{FullMethod: "/test/method"}

	slowOkHandler := func(_ context.Context, _ any) (any, error) {
		time.Sleep(time.Millisecond) // 保证每次调用有可观测的延迟
		return "ok", nil
	}
	slowErrHandler := func(_ context.Context, _ any) (any, error) {
		time.Sleep(time.Millisecond)
		return nil, status.Error(codes.Internal, "test error")
	}

	// 3 个成功 + 2 个失败
	for i := 0; i < 3; i++ {
		_, _ = interceptor(context.Background(), nil, info, slowOkHandler)
	}
	for i := 0; i < 2; i++ {
		_, _ = interceptor(context.Background(), nil, info, slowErrHandler)
	}

	snap := metrics.Snapshot()
	if snap.TotalCalls != 5 {
		t.Errorf("TotalCalls = %d, 期望 5", snap.TotalCalls)
	}
	if snap.ErrorCalls != 2 {
		t.Errorf("ErrorCalls = %d, 期望 2", snap.ErrorCalls)
	}
	if snap.TotalLatencyNanos <= 0 {
		t.Error("TotalLatencyNanos 应 > 0")
	}
	if snap.AvgLatencyMillis <= 0 {
		t.Error("AvgLatencyMillis 应 > 0")
	}
}

// TestMetricsInterceptor_ClientCanceled 测试客户端取消不算错误
func TestMetricsInterceptor_ClientCanceled(t *testing.T) {
	metrics := &A2AInterceptorMetrics{}
	interceptor := MetricsInterceptor(metrics)
	info := &grpc.UnaryServerInfo{FullMethod: "/test/method"}

	cancelHandler := func(_ context.Context, _ any) (any, error) {
		return nil, status.Error(codes.Canceled, "client canceled")
	}
	_, _ = interceptor(context.Background(), nil, info, cancelHandler)

	snap := metrics.Snapshot()
	if snap.TotalCalls != 1 {
		t.Errorf("TotalCalls = %d, 期望 1", snap.TotalCalls)
	}
	if snap.ErrorCalls != 0 {
		t.Errorf("ErrorCalls = %d, 期望 0 (客户端取消不算错误)", snap.ErrorCalls)
	}
}

// TestChainUnaryInterceptors 测试拦截器链按顺序执行
func TestChainUnaryInterceptors(t *testing.T) {
	var order []string

	mkRecorder := func(name string) grpc.UnaryServerInterceptor {
		return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
			order = append(order, name+":before")
			resp, err := handler(ctx, req)
			order = append(order, name+":after")
			return resp, err
		}
	}

	chain := ChainUnaryInterceptors(
		mkRecorder("A"),
		mkRecorder("B"),
		mkRecorder("C"),
	)
	info := &grpc.UnaryServerInfo{FullMethod: "/test/chain"}
	final := func(_ context.Context, _ any) (any, error) {
		order = append(order, "handler")
		return "ok", nil
	}
	_, _ = chain(context.Background(), nil, info, final)

	// 期望顺序：A:before → B:before → C:before → handler → C:after → B:after → A:after
	expected := []string{
		"A:before", "B:before", "C:before",
		"handler",
		"C:after", "B:after", "A:after",
	}
	if len(order) != len(expected) {
		t.Fatalf("顺序数 = %d, 期望 %d, 实际: %v", len(order), len(expected), order)
	}
	for i, want := range expected {
		if order[i] != want {
			t.Errorf("位置 %d = %q, 期望 %q", i, order[i], want)
		}
	}
}

// TestChainUnaryInterceptors_Single 单个拦截器直接返回
func TestChainUnaryInterceptors_Single(t *testing.T) {
	called := false
	one := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		called = true
		return handler(ctx, req)
	}
	chain := ChainUnaryInterceptors(one)
	_, _ = chain(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/x"},
		func(_ context.Context, _ any) (any, error) { return "ok", nil })
	if !called {
		t.Error("单个拦截器应被调用")
	}
}

// TestChainUnaryInterceptors_Empty 空链应直接调用 handler
func TestChainUnaryInterceptors_Empty(t *testing.T) {
	called := false
	chain := ChainUnaryInterceptors()
	_, _ = chain(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/x"},
		func(_ context.Context, _ any) (any, error) { called = true; return nil, nil })
	if !called {
		t.Error("空链应直接调用 handler")
	}
}

// TestStreamRecoveryInterceptor 测试流式 panic 恢复
func TestStreamRecoveryInterceptor(t *testing.T) {
	interceptor := StreamRecoveryInterceptor()
	info := &grpc.StreamServerInfo{FullMethod: "/test/stream"}

	panicHandler := func(_ any, _ grpc.ServerStream) error {
		panic("stream boom")
	}
	// wrappedServerStream 用于模拟 ServerStream
	srv := &struct{}{}
	ss := &nopStream{ctx: context.Background()}

	err := interceptor(srv, ss, info, panicHandler)
	if err == nil {
		t.Fatal("流式 panic 后应返回错误")
	}
	if st, ok := status.FromError(err); !ok || st.Code() != codes.Internal {
		t.Errorf("期望 Internal 错误: %v", err)
	}
}

// TestStreamLoggingInterceptor 测试流式日志拦截器
func TestStreamLoggingInterceptor(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	cfg := A2AInterceptorConfig{Logger: logger}
	interceptor := StreamLoggingInterceptor(cfg)
	info := &grpc.StreamServerInfo{FullMethod: "/test/stream"}

	srv := &struct{}{}
	ss := &nopStream{ctx: context.Background()}

	errHandler := func(_ any, _ grpc.ServerStream) error {
		return io.EOF
	}
	_ = interceptor(srv, ss, info, errHandler)
	if !strings.Contains(buf.String(), "a2a stream rpc") {
		t.Error("流式日志拦截器应记录")
	}
	if !strings.Contains(buf.String(), "method=/test/stream") {
		t.Error("日志应包含方法名")
	}
}

// TestStreamMetricsInterceptor 测试流式指标拦截器
func TestStreamMetricsInterceptor(t *testing.T) {
	metrics := &A2AInterceptorMetrics{}
	interceptor := StreamMetricsInterceptor(metrics)
	info := &grpc.StreamServerInfo{FullMethod: "/test/stream"}

	srv := &struct{}{}
	ss := &nopStream{ctx: context.Background()}

	for i := 0; i < 4; i++ {
		_ = interceptor(srv, ss, info, func(_ any, _ grpc.ServerStream) error {
			return nil
		})
	}
	if got := metrics.Snapshot().TotalCalls; got != 4 {
		t.Errorf("TotalCalls = %d, 期望 4", got)
	}
}

// TestChainStreamInterceptors 测试流式拦截器链
func TestChainStreamInterceptors(t *testing.T) {
	var count atomic.Int32
	mk := func(name string) grpc.StreamServerInterceptor {
		return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			count.Add(1)
			_ = name
			return handler(srv, ss)
		}
	}
	chain := ChainStreamInterceptors(mk("A"), mk("B"))
	_ = chain(&struct{}{}, &nopStream{ctx: context.Background()}, &grpc.StreamServerInfo{FullMethod: "/x"},
		func(_ any, _ grpc.ServerStream) error { return nil })
	if count.Load() != 2 {
		t.Errorf("流式拦截器链调用次数 = %d, 期望 2", count.Load())
	}
}

// TestIsClientCanceled 测试客户端取消识别
func TestIsClientCanceled(t *testing.T) {
	if isClientCanceled(nil) {
		t.Error("nil 错误应返回 false")
	}
	if !isClientCanceled(status.Error(codes.Canceled, "")) {
		t.Error("Canceled 应识别为客户端取消")
	}
	if !isClientCanceled(status.Error(codes.Unauthenticated, "")) {
		t.Error("Unauthenticated 应识别为客户端取消")
	}
	if isClientCanceled(status.Error(codes.Internal, "")) {
		t.Error("Internal 不应识别为客户端取消")
	}
	if isClientCanceled(errors.New("plain error")) {
		t.Error("普通 error 不应识别为客户端取消")
	}
}

// TestInterceptorConfig_Defaults 测试配置默认值
func TestInterceptorConfig_Defaults(t *testing.T) {
	cfg := A2AInterceptorConfig{}
	if cfg.logger() == nil {
		t.Error("logger 默认值不应为 nil")
	}
	if cfg.metrics() == nil {
		t.Error("metrics 默认值不应为 nil")
	}
	if cfg.slowThreshold() != time.Second {
		t.Errorf("slowThreshold 默认值 = %v, 期望 1s", cfg.slowThreshold())
	}
}

// 辅助类型：用于测试的最小 ServerStream 实现
type nopStream struct {
	ctx context.Context
	grpc.ServerStream
}

func (n *nopStream) Context() context.Context { return n.ctx }

package protocol

import (
	"context"
	"encoding/hex"
	"sync/atomic"
	"time"
)

// TraceContext W3C Trace Context 兼容。
type TraceContext struct {
	TraceID string
	SpanID  string
	Sampled bool
}

type traceKey struct{}

func WithTrace(ctx context.Context, tc *TraceContext) context.Context {
	return context.WithValue(ctx, traceKey{}, tc)
}

func FromTrace(ctx context.Context) (*TraceContext, bool) {
	v, ok := ctx.Value(traceKey{}).(*TraceContext)
	return v, ok
}

// ExtractTraceContext 从 HTTP headers 提取 W3C traceparent。
func ExtractTraceContext(headers map[string]string) *TraceContext {
	tc := &TraceContext{}
	v, ok := headers["traceparent"]
	if !ok || len(v) < 5 {
		return tc
	}
	parts := splitTraceparent(v)
	if len(parts) >= 4 {
		tc.TraceID = parts[1]
		tc.SpanID = parts[2]
		tc.Sampled = len(parts[3]) > 0 && parts[3][len(parts[3])-1] == '1'
	}
	return tc
}

// InjectTraceContext 将 trace context 注入 HTTP headers。
func InjectTraceContext(tc *TraceContext) map[string]string {
	if tc == nil {
		return nil
	}
	flags := "00"
	if tc.Sampled {
		flags = "01"
	}
	return map[string]string{
		"traceparent": "00-" + tc.TraceID + "-" + tc.SpanID + "-" + flags,
	}
}

// NewTraceContext 生成新的 trace context。
func NewTraceContext() *TraceContext {
	return &TraceContext{
		TraceID: generateHex(16),
		SpanID:  generateHex(8),
		Sampled: true,
	}
}

var hexCounter uint64

func generateHex(length int) string {
	buf := make([]byte, length/2+1)
	binaryPut(buf, uint64(time.Now().UnixNano())+atomic.AddUint64(&hexCounter, 1))
	return hex.EncodeToString(buf)[:length]
}

// binaryPut 将 uint64 写入 buffer（小端）。
func binaryPut(buf []byte, v uint64) {
	for i := 0; i < len(buf) && i < 8; i++ {
		buf[i] = byte(v >> (i * 8))
	}
}

func splitTraceparent(s string) []string {
	result := make([]string, 0, 4)
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '-' {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	result = append(result, s[start:])
	return result
}

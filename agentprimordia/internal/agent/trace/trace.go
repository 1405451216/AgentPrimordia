// Package trace 提供追踪和可观测性功能
package trace

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
)

// SpanKind Span 类型
type SpanKind string

const (
	SpanKindInternal SpanKind = "internal"
	SpanKindClient   SpanKind = "client"
	SpanKindServer   SpanKind = "server"
)

// SpanStatus Span 状态
type SpanStatus string

const (
	SpanStatusOK    SpanStatus = "ok"
	SpanStatusError SpanStatus = "error"
)

// SpanContext Span 上下文，用于跨服务传播
type SpanContext struct {
	TraceID    string            `json:"trace_id"`
	SpanID     string            `json:"span_id"`
	TraceFlags byte              `json:"trace_flags"`
	TraceState map[string]string `json:"trace_state,omitempty"`
	Remote     bool              `json:"remote"`
}

// IsValid 检查 SpanContext 是否有效
func (sc SpanContext) IsValid() bool {
	return sc.TraceID != "" && sc.SpanID != ""
}

// ToW3CTraceParent 将 SpanContext 转换为 W3C TraceContext traceparent 格式
func (sc SpanContext) ToW3CTraceParent() string {
	flags := "00"
	if sc.TraceFlags&0x01 != 0 {
		flags = "01"
	}
	return fmt.Sprintf("00-%s-%s-%s", sc.TraceID, sc.SpanID, flags)
}

// FromW3CTraceParent 从 W3C traceparent 字符串解析 SpanContext
func FromW3CTraceParent(s string) (SpanContext, error) {
	parts := strings.Split(s, "-")
	if len(parts) != 4 || parts[0] != "00" {
		return SpanContext{}, fmt.Errorf("invalid traceparent format: %s", s)
	}
	if len(parts[1]) != 32 {
		return SpanContext{}, fmt.Errorf("invalid trace ID length: %d", len(parts[1]))
	}
	if len(parts[2]) != 16 {
		return SpanContext{}, fmt.Errorf("invalid span ID length: %d", len(parts[2]))
	}
	var flags byte
	if parts[3] == "01" {
		flags = 1
	}
	return SpanContext{
		TraceID:    parts[1],
		SpanID:     parts[2],
		TraceFlags: flags,
	}, nil
}

// WithTraceState 返回携带指定 TraceState 的新 SpanContext
func (sc SpanContext) WithTraceState(key, value string) SpanContext {
	newState := make(map[string]string)
	for k, v := range sc.TraceState {
		newState[k] = v
	}
	newState[key] = value
	sc.TraceState = newState
	return sc
}

// Span 追踪 Span 接口
type Span interface {
	SetName(name string)
	SetAttribute(key string, value any)
	SetStatus(status SpanStatus, description string)
	SpanContext() SpanContext
	IsEnded() bool
	End()
}

// SpanConfig Span 创建配置
type SpanConfig struct {
	ParentContext SpanContext
	Attributes    map[string]any
}

// SpanOption Span 创建选项
type SpanOption func(*SpanConfig)

// WithParent 设置父 Span 上下文
func WithParent(parent SpanContext) SpanOption {
	return func(cfg *SpanConfig) {
		cfg.ParentContext = parent
	}
}

// WithAttributes 设置 Span 属性
func WithAttributes(attrs map[string]any) SpanOption {
	return func(cfg *SpanConfig) {
		if cfg.Attributes == nil {
			cfg.Attributes = make(map[string]any)
		}
		for k, v := range attrs {
			cfg.Attributes[k] = v
		}
	}
}

// NoopSpan 空操作 Span，用于未启用追踪时
type NoopSpan struct{}

func (s *NoopSpan) SetName(string)               {}
func (s *NoopSpan) SetAttribute(string, any)     {}
func (s *NoopSpan) SetStatus(SpanStatus, string) {}
func (s *NoopSpan) SpanContext() SpanContext     { return SpanContext{} }
func (s *NoopSpan) IsEnded() bool                { return false }
func (s *NoopSpan) End()                         {}

// LoggingSpan 日志 Span 实现
type LoggingSpan struct {
	name      string
	kind      SpanKind
	context   SpanContext
	Status    SpanStatus
	attrs     map[string]any
	startTime time.Time
	Duration  time.Duration
	ended     bool
	mu        sync.RWMutex
}

func (s *LoggingSpan) SetName(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.name = name
}

func (s *LoggingSpan) SetAttribute(key string, value any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.attrs == nil {
		s.attrs = make(map[string]any)
	}
	s.attrs[key] = value
}

func (s *LoggingSpan) SetStatus(status SpanStatus, _ string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Status = status
}

func (s *LoggingSpan) SpanContext() SpanContext {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.context
}

func (s *LoggingSpan) IsEnded() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ended
}

func (s *LoggingSpan) End() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.ended {
		s.Duration = time.Since(s.startTime)
		s.ended = true
	}
}

// LoggingTracer 日志追踪器
type LoggingTracer struct {
	spans []*LoggingSpan
	mu    sync.RWMutex
}

// NewLoggingTracer 创建日志追踪器
func NewLoggingTracer() *LoggingTracer {
	return &LoggingTracer{
		spans: make([]*LoggingSpan, 0),
	}
}

// Start 创建并启动一个新 Span
func (t *LoggingTracer) Start(name string, kind SpanKind, opts ...SpanOption) Span {
	cfg := &SpanConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	traceID := cfg.ParentContext.TraceID
	if traceID == "" {
		traceID = generateID(16)
	}

	span := &LoggingSpan{
		name:      name,
		kind:      kind,
		context:   SpanContext{TraceID: traceID, SpanID: generateID(8)},
		Status:    SpanStatusOK,
		attrs:     cfg.Attributes,
		startTime: time.Now(),
	}

	t.mu.Lock()
	t.spans = append(t.spans, span)
	t.mu.Unlock()

	return span
}

// Reset 重置追踪器
func (t *LoggingTracer) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.spans = make([]*LoggingSpan, 0)
}

// String 输出所有已结束的 Span 信息
func (t *LoggingTracer) String() string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var sb strings.Builder
	for _, s := range t.spans {
		s.mu.RLock()
		if s.ended {
			sb.WriteString(fmt.Sprintf("[%s] %s kind=%s status=%s duration=%v trace=%s span=%s",
				s.startTime.Format(time.RFC3339),
				s.name,
				s.kind,
				s.Status,
				s.Duration,
				s.context.TraceID,
				s.context.SpanID,
			))
			if len(s.attrs) > 0 {
				sb.WriteString(fmt.Sprintf(" attrs=%v", s.attrs))
			}
			sb.WriteString("\n")
		}
		s.mu.RUnlock()
	}
	return sb.String()
}

// generateID 生成随机 ID
func generateID(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

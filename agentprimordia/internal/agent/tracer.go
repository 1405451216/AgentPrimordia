package agent

// Tracer 追踪器接口，支持多种实现替换
type Tracer interface {
	Start(name string, kind SpanKind, opts ...SpanOption) Span
}

// TracerDebug 调试扩展接口，仅 LoggingTracer 实现
type TracerDebug interface {
	Tracer
	Reset()
	String() string
}

// NoopTracer 空操作追踪器
type NoopTracer struct{}

// NewNoopTracer 创建空操作追踪器
func NewNoopTracer() *NoopTracer { return &NoopTracer{} }

// Start 返回空操作 Span
func (t *NoopTracer) Start(_ string, _ SpanKind, _ ...SpanOption) Span {
	return &NoopSpan{}
}

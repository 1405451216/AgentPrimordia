//go:build !otel

package otel

import "time"

// BridgeEnabled 标识 OTel SDK 桥接是否启用
const BridgeEnabled = false

// OTelBridge OTel SDK 桥接（默认 noop 实现）
type OTelBridge struct{}

// NewOTelBridge 创建空桥接
func NewOTelBridge() *OTelBridge { return &OTelBridge{} }

// StartSpan 创建空 Span（无 OTel SDK）
func (b *OTelBridge) StartSpan(name string) SpanBridge {
	return &noopSpanBridge{name: name, startTime: time.Now()}
}

// Shutdown 关闭桥接
func (b *OTelBridge) Shutdown() error { return nil }

// SpanBridge Span 桥接接口
type SpanBridge interface {
	SetAttribute(key string, value any)
	End()
	RecordError(err error)
	SetStatus(code string, description string)
	AddEvent(name string, attrs map[string]any)
	SpanContext() map[string]string
}

type noopSpanBridge struct {
	name      string
	startTime time.Time
	attrs     map[string]any
	status    string
	events    []spanEvent
}

type spanEvent struct {
	name  string
	attrs map[string]any
	time  time.Time
}

func (s *noopSpanBridge) SetAttribute(key string, value any) {
	if s.attrs == nil {
		s.attrs = make(map[string]any)
	}
	s.attrs[key] = value
}

func (s *noopSpanBridge) End() {}

func (s *noopSpanBridge) RecordError(err error) {}

func (s *noopSpanBridge) SetStatus(code string, description string) {
	s.status = code
}

func (s *noopSpanBridge) AddEvent(name string, attrs map[string]any) {
	s.events = append(s.events, spanEvent{
		name:  name,
		attrs: attrs,
		time:  time.Now(),
	})
}

func (s *noopSpanBridge) SpanContext() map[string]string {
	return map[string]string{
		"trace_id": "noop",
		"span_id":  "noop",
	}
}

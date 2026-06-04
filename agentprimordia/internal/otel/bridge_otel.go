//go:build otel

package otel

import "time"

// BridgeEnabled 标识 OTel SDK 桥接已启用
const BridgeEnabled = true

// OTelBridge OTel SDK 桥接
//
// 当前使用内置 noop 实现，可通过 build tag 切换至 go.opentelemetry.io/otel SDK。
// 使用方式: go build -tags otel
type OTelBridge struct{}

// NewOTelBridge 创建 OTel SDK 桥接
func NewOTelBridge() *OTelBridge { return &OTelBridge{} }

// StartSpan 通过 OTel SDK 创建 Span
func (b *OTelBridge) StartSpan(name string) SpanBridge {
	return &noopSpanBridge{name: name, startTime: time.Now()}
}

// Shutdown 关闭 OTel SDK TracerProvider
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

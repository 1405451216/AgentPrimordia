// Stability: 混合 —
//
//	Tracer 接口与 Noop 实现: Stable。
//	OTLP / Telemetry Provider: Experimental（OTel 协议规范仍在 1.x 演进）。
package ap

import (
	"agentprimordia/internal/agent"
	"agentprimordia/internal/otel"
)

type Tracer = agent.Tracer
type TracerDebug = agent.TracerDebug
type NoopTracer = agent.NoopTracer

var NewNoopTracer = agent.NewNoopTracer

type OTLPConfig = otel.OTLPConfig
type OTLPExporter = otel.OTLPExporter

var NewOTLPExporter = otel.NewOTLPExporter

type TelemetryConfig = otel.TelemetryConfig
type TelemetryProvider = otel.TelemetryProvider

var NewTelemetryProvider = otel.NewTelemetryProvider

// WithTelemetry 将 TelemetryProvider 的 Tracer 与 Metrics 一次性注入 Agent，
// 打通「ReAct loop → OTel」运行时接线：loop 产生 span 与指标，
// 由 provider 经 OTLPExporter 导出（周期或 ExportNow）。
//
// Stability: Experimental
func WithTelemetry(tp *otel.TelemetryProvider) agent.Option {
	opts := []agent.Option{agent.WithTracer(tp.Tracer())}
	if tp.MetricsEnabled() {
		opts = append(opts, agent.WithMetrics(tp.Metrics()))
	}
	return func(c *agent.AgentConfig) {
		for _, o := range opts {
			o(c)
		}
	}
}

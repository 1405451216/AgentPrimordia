// Stability: 混合 —
//
//	Tracer 接口与 Noop 实现: Stable。
//	OTLP / Telemetry Provider: Experimental（OTel 协议规范仍在 1.x 演进）。
package ap

import (
	"agentprimordia/internal/agent"
	obsotel "agentprimordia/internal/observability/export/otel"
)

type Tracer = agent.Tracer
type TracerDebug = agent.TracerDebug
type NoopTracer = agent.NoopTracer

var NewNoopTracer = agent.NewNoopTracer

type OTLPConfig = obsotel.OTLPConfig
type OTLPExporter = obsotel.OTLPExporter

var NewOTLPExporter = obsotel.NewOTLPExporter

type TelemetryConfig = obsotel.TelemetryConfig
type TelemetryProvider = obsotel.TelemetryProvider

var NewTelemetryProvider = obsotel.NewTelemetryProvider

// WithTelemetry 将 TelemetryProvider 的 Tracer 与 Metrics 一次性注入 Agent，
// 打通「ReAct loop → OTel」运行时接线：loop 产生 span 与指标，
// 由 provider 经 OTLPExporter 导出（周期或 ExportNow）。
//
// Stability: Experimental
func WithTelemetry(tp *obsotel.TelemetryProvider) agent.Option {
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

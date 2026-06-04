// Stability: 混合 —
//   Tracer 接口与 Noop 实现: Stable。
//   OTLP / Telemetry Provider: Experimental（OTel 协议规范仍在 1.x 演进）。
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

# OpenTelemetry 桥接（OTel）

OTel 模块为 AgentPrimordia 提供 OpenTelemetry 兼容的分布式追踪能力，无需引入外部 OTel SDK。

## 设计思路

采用内建轻量实现，生成符合 W3C Trace Context 标准的 Trace/Span ID：
- Trace ID：128-bit 随机十六进制
- Span ID：64-bit 随机十六进制
- 支持父子 Span 关系

## 核心组件

### OTelBridge

OTel 兼容桥接器：

```go
bridge := otel.NewOTelBridge()

span := bridge.StartSpan("agent-run")
span.SetAttribute("agent.name", "my-agent")
childSpan := span.StartChild("llm-call")
childSpan.End()
span.End()
```

### OTLPExporter

支持将 Span 数据导出到 OTLP 兼容后端（Jaeger、Tempo 等）：

```go
exporter := otel.NewOTLPExporter("http://localhost:4318")
bridge.WithExporter(exporter)
```

## 与 Agent 集成

通过 Tracer 接口注入 ReAct 循环，自动为每轮推理、LLM 调用、工具执行创建 Span：

```go
agent := ap.NewReActAgent(cfg).WithTracer(bridge)
```

## 部署配置

`deploy/` 目录下提供了完整的可观测性栈配置：
- `deploy/prometheus/`：Prometheus 抓取配置
- `deploy/grafana/`：Grafana dashboard 模板

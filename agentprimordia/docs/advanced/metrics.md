# Prometheus 指标

AgentPrimordia 内置 Prometheus 风格的指标收集，支持多维标签聚合。

## 指标类型

```go
type AgentMetrics struct {
    LLMTotalCalls   int64
    LLMTotalErrors  int64
    ToolTotalCalls  int64
    ToolTotalErrors int64
    TotalTurns      int64
    TotalEpisodes   int64

    LLMLatencyMs   *Histogram
    ToolLatencyMs  *Histogram
    TurnDurationMs *Histogram

    TokenUsageByModel map[string]*TokenUsageStats
}
```

## 内置指标

| 指标 | 类型 | 说明 |
|------|------|------|
| `llm_calls_total` | Counter | LLM 调用次数 |
| `llm_errors_total` | Counter | LLM 错误次数 |
| `tool_calls_total` | Counter | 工具调用次数 |
| `tool_errors_total` | Counter | 工具错误次数 |
| `turns_total` | Counter | ReAct 轮数 |
| `llm_latency_ms` | Histogram | LLM 延迟 |
| `tool_latency_ms` | Histogram | 工具延迟 |

## 带标签的指标

```go
type LabeledMetricsRecorder interface {
    MetricsRecorder
    RecordLLMCallWithLabels(duration time.Duration, err error, provider, model string)
    RecordToolCallWithLabels(duration time.Duration, err error, toolName string)
    RecordTurnWithAgent(duration time.Duration, agentName string)
}
```

启用后，指标会附加 `provider`、`model`、`tool_name`、`agent_name` 等标签。

## 快速开始

```go
recorder := metrics.NewAgentMetrics()

agent := NewReActAgent(cfg).WithMetrics(recorder)
```

## 暴露 /metrics 端点

```go
http.Handle("/metrics", promhttp.HandlerFor(recorder.Gatherer(), promhttp.HandlerOpts{}))
http.ListenAndServe(":9090", nil)
```

> `promhttp` 来自 `github.com/prometheus/client_golang/prometheus/promhttp`，如不需要外部依赖，也可使用 `recorder.MarshalText()` 自行暴露。

## Grafana Dashboard

项目提供预设 Dashboard：

- `deploy/grafana/dashboard-agent.json`
- `deploy/grafana/dashboard-llm.json`
- `deploy/grafana/dashboard-cost.json`

## 下一步

- 查看 [Grafana 部署说明](../deploy/grafana/README.md)
- 了解 [OpenTelemetry 桥接](otel.md)

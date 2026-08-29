# 指标系统（Metrics）

Metrics 模块提供 Agent 运行时的 Prometheus 风格指标收集，支持多维标签聚合。

## 核心类型

### AgentMetrics

主指标收集器，线程安全：

```go
m := metrics.NewAgentMetrics()

// 记录 LLM 调用
m.RecordLLMCall(duration, err)
m.RecordLLMCallWithLabels(duration, err, "openai", "gpt-4o")

// 记录工具调用
m.RecordToolCall(duration, err)
m.RecordToolCallWithLabels(duration, err, "filesystem")

// 记录轮次
m.RecordTurn(duration)
m.RecordTurnWithAgent(duration, "agent-1")

// Token 用量
m.RecordTokenUsage("gpt-4o", promptTokens, completionTokens)
```

## 指标维度

| 指标 | 类型 | 标签 |
|------|------|------|
| LLM 调用次数/错误 | Counter | provider, model |
| 工具调用次数/错误 | Counter | tool_name |
| 轮次耗时 | Histogram | agent_name |
| LLM 延迟 | Histogram | — |
| 工具延迟 | Histogram | — |
| 活跃 Agent 数 | Gauge | — |
| Token 用量 | Counter | model |
| 成本追踪 | Counter | provider, model, agent |

## Histogram

内置直方图实现，支持自定义桶边界，提供 P50/P90/P99 分位数计算。

## 与 Grafana 集成

通过 `deploy/grafana/` 下的 dashboard JSON 模板，可直接导入 Grafana 查看多维聚合面板。PromQL 示例：

```promql
rate(ap_llm_calls_total{provider="openai"}[5m])
histogram_quantile(0.99, ap_turn_duration_ms_bucket)
```

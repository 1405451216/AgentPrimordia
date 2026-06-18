# OpenTelemetry 桥接

AgentPrimordia 的 `otel` 包提供 OpenTelemetry 桥接适配层，默认使用零外部依赖的 OTLP HTTP/JSON 导出器。

## 架构

```
AgentPrimordia Span → OTel Bridge → OTLP HTTP/JSON Exporter → Collector
```

## 默认导出器

```go
exporter := otel.NewOTLPExporter(otel.OTLPExporterConfig{
    Endpoint: "http://localhost:4318/v1/traces",
    Headers: map[string]string{
        "Authorization": "Bearer token",
    },
})
```

## 启用完整 OTel SDK 桥接

使用构建标签 `otel` 启用完整 SDK 桥接：

```bash
go build -tags otel ./...
```

代码中：

```go
bridge := otel.NewBridge(otel.BridgeConfig{
    ServiceName:    "my-agent",
    ServiceVersion: "1.0.0",
    Exporter:       exporter,
})

agent := NewReActAgent(cfg).WithTracer(bridge)
```

## 与 Inspector 的关系

| 组件 | 用途 | 部署 |
|------|------|------|
| Inspector | 本地调试、可视化 | 内置 HTTP |
| OTel | 生产链路追踪 | 导出到 Collector |

两者可同时启用：Inspector 用于开发调试，OTel 用于生产观测。

## 下一步

- 查看 [调试与 Inspector](debugger.md)
- 阅读 `internal/otel/doc.go`

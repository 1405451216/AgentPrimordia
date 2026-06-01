// Package otel 提供 OpenTelemetry 桥接适配层。
//
// 默认提供 OTLP HTTP/JSON 导出器（零外部依赖），
// 可通过构建标签 `otel` 启用 OTel SDK 完整桥接。
package otel

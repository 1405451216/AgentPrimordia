// Stability: Stable — Prometheus 指标收集与导出。
package ap

import (
	"agentprimordia/internal/metrics"
)

// AgentMetricsCollector 是 Agent 指标收集器，记录 LLM 调用、工具调用和轮次等指标
type AgentMetricsCollector = metrics.AgentMetrics

// Histogram 是直方图指标，用于统计值分布（如延迟、Token 用量）
type Histogram = metrics.Histogram

// HistogramSnapshot 是直方图的快照，包含分位数和统计值
type HistogramSnapshot = metrics.HistogramSnapshot

// MetricsSnapshot 是指标系统的完整快照
type MetricsSnapshot = metrics.MetricsSnapshot

// PrometheusHandler 是 Prometheus 格式的指标 HTTP 处理器
type PrometheusHandler = metrics.PrometheusHandler

// TelemetryExporter 是遥测数据导出接口
type TelemetryExporter = metrics.TelemetryExporter

// MultiExporter 是多目标导出器，支持同时导出到多个目标
type MultiExporter = metrics.MultiExporter

// LogExporter 是日志格式的指标导出器
type LogExporter = metrics.LogExporter

// JSONExporter 是 JSON 格式的指标导出器
type JSONExporter = metrics.JSONExporter

// MetricsExporter 是指标导出器的通用接口
type MetricsExporter = metrics.MetricsExporter

var (
	// NewMetrics 创建指标收集器实例
	NewMetrics = metrics.NewMetrics
	// NewHistogram 创建直方图指标，参数为桶边界列表
	NewHistogram = metrics.NewHistogram
	// NewPrometheusHandler 创建 Prometheus 格式的 HTTP 处理器
	NewPrometheusHandler = metrics.NewPrometheusHandler
	// NewMultiExporter 创建多目标导出器
	NewMultiExporter = metrics.NewMultiExporter
	// NewLogExporter 创建日志格式导出器
	NewLogExporter = metrics.NewLogExporter
	// NewJSONExporter 创建 JSON 格式导出器
	NewJSONExporter = metrics.NewJSONExporter
	// NewMetricsExporter 创建通用指标导出器
	NewMetricsExporter = metrics.NewMetricsExporter
)

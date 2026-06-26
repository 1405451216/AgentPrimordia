package otel

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
)

// CounterMetric 计数器指标
type CounterMetric struct {
	Name   string
	Labels map[string]string
	Value  float64
}

// GaugeMetric 仪表盘指标
type GaugeMetric struct {
	Name   string
	Labels map[string]string
	Value  float64
}

// HistogramMetric 直方图指标
type HistogramMetric struct {
	Name    string
	Labels  map[string]string
	Buckets []float64
	Counts  []uint64 // 每个 bucket 的累计计数
	Sum     float64
	Count   uint64
}

// MetricExporter 指标导出器
type MetricExporter struct {
	mu         sync.Mutex
	counters   map[string]*CounterMetric
	gauges     map[string]*GaugeMetric
	histograms map[string]*HistogramMetric
}

// NewMetricExporter 创建指标导出器
func NewMetricExporter() *MetricExporter {
	return &MetricExporter{
		counters:   make(map[string]*CounterMetric),
		gauges:     make(map[string]*GaugeMetric),
		histograms: make(map[string]*HistogramMetric),
	}
}

// RecordCounter 记录计数器（相同标签累加）
func (e *MetricExporter) RecordCounter(name string, value float64, labels map[string]string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	key := counterKey(name, labels)
	if existing, ok := e.counters[key]; ok {
		existing.Value += value
	} else {
		e.counters[key] = &CounterMetric{
			Name:   name,
			Labels: copyLabels(labels),
			Value:  value,
		}
	}
}

// RecordGauge 记录仪表盘（相同标签覆盖）
func (e *MetricExporter) RecordGauge(name string, value float64, labels map[string]string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	key := gaugeKey(name, labels)
	e.gauges[key] = &GaugeMetric{
		Name:   name,
		Labels: copyLabels(labels),
		Value:  value,
	}
}

// RecordHistogram 记录直方图（相同标签累加 bucket 计数、sum、count）
func (e *MetricExporter) RecordHistogram(name string, value float64, labels map[string]string, buckets []float64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	key := histogramKey(name, labels)
	if existing, ok := e.histograms[key]; ok {
		// 累加到已有直方图
		existing.Sum += value
		existing.Count++
		// 更新 bucket 累计计数
		for i, b := range existing.Buckets {
			if value <= b {
				existing.Counts[i]++
			}
		}
	} else {
		// 新建直方图
		sortedBuckets := make([]float64, len(buckets))
		copy(sortedBuckets, buckets)
		sort.Float64s(sortedBuckets)

		counts := make([]uint64, len(sortedBuckets))
		for i, b := range sortedBuckets {
			if value <= b {
				counts[i]++
			}
		}
		e.histograms[key] = &HistogramMetric{
			Name:    name,
			Labels:  copyLabels(labels),
			Buckets: sortedBuckets,
			Counts:  counts,
			Sum:     value,
			Count:   1,
		}
	}
}

// ExportPrometheus 导出 Prometheus text format
func (e *MetricExporter) ExportPrometheus() string {
	e.mu.Lock()
	defer e.mu.Unlock()

	if len(e.counters) == 0 && len(e.gauges) == 0 && len(e.histograms) == 0 {
		return ""
	}

	var b strings.Builder

	// 按指标名分组输出计数器
	countersByName := e.groupCounters()
	for _, name := range sortedStringKeys(countersByName) {
		fmt.Fprintf(&b, "# TYPE %s counter\n", name)
		for _, c := range countersByName[name] {
			fmt.Fprintf(&b, "%s%s %g\n", name, formatLabels(c.Labels), c.Value)
		}
	}

	// 按指标名分组输出仪表盘
	gaugesByName := e.groupGauges()
	for _, name := range sortedStringKeys(gaugesByName) {
		fmt.Fprintf(&b, "# TYPE %s gauge\n", name)
		for _, g := range gaugesByName[name] {
			fmt.Fprintf(&b, "%s%s %g\n", name, formatLabels(g.Labels), g.Value)
		}
	}

	// 按指标名分组输出直方图
	histogramsByName := e.groupHistograms()
	for _, name := range sortedStringKeys(histogramsByName) {
		fmt.Fprintf(&b, "# TYPE %s histogram\n", name)
		for _, h := range histogramsByName[name] {
			// bucket 行
			for i, le := range h.Buckets {
				fmt.Fprintf(&b, "%s_bucket%s %d\n", name,
					formatLabelsWithLe(h.Labels, le), h.Counts[i])
			}
			// +Inf bucket
			fmt.Fprintf(&b, "%s_bucket%s %d\n", name,
				formatLabelsWithLe(h.Labels, math.Inf(1)), h.Count)
			// sum
			fmt.Fprintf(&b, "%s_sum%s %g\n", name, formatLabels(h.Labels), h.Sum)
			// count
			fmt.Fprintf(&b, "%s_count%s %d\n", name, formatLabels(h.Labels), h.Count)
		}
	}

	return b.String()
}

// ExportOTLP 导出 OTLP JSON format（简化版）
func (e *MetricExporter) ExportOTLP() ([]byte, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	var metricsData []any

	// 计数器
	for _, c := range e.counters {
		metricsData = append(metricsData, map[string]any{
			"name": c.Name,
			"data": map[string]any{
				"dataPoints": []any{
					map[string]any{
						"asDouble":   c.Value,
						"attributes": labelsToOTLP(c.Labels),
					},
				},
				"isMonotonic": true,
			},
		})
	}

	// 仪表盘
	for _, g := range e.gauges {
		metricsData = append(metricsData, map[string]any{
			"name": g.Name,
			"data": map[string]any{
				"dataPoints": []any{
					map[string]any{
						"asDouble":   g.Value,
						"attributes": labelsToOTLP(g.Labels),
					},
				},
			},
		})
	}

	// 直方图
	for _, h := range e.histograms {
		var explicitBounds []any
		var bucketCounts []any
		for i, b := range h.Buckets {
			explicitBounds = append(explicitBounds, b)
			bucketCounts = append(bucketCounts, h.Counts[i])
		}
		metricsData = append(metricsData, map[string]any{
			"name": h.Name,
			"data": map[string]any{
				"dataPoints": []any{
					map[string]any{
						"sum":            h.Sum,
						"count":          h.Count,
						"explicitBounds": explicitBounds,
						"bucketCounts":   bucketCounts,
						"attributes":     labelsToOTLP(h.Labels),
					},
				},
			},
		})
	}

	payload := map[string]any{
		"resourceMetrics": []any{
			map[string]any{
				"scopeMetrics": []any{
					map[string]any{
						"metrics": metricsData,
					},
				},
			},
		},
	}

	return json.Marshal(payload)
}

// ===== 辅助函数 =====

// counterKey 生成计数器唯一键
func counterKey(name string, labels map[string]string) string {
	return name + "{" + sortedLabelString(labels) + "}"
}

// gaugeKey 生成仪表盘唯一键
func gaugeKey(name string, labels map[string]string) string {
	return name + "{" + sortedLabelString(labels) + "}"
}

// histogramKey 生成直方图唯一键
func histogramKey(name string, labels map[string]string) string {
	return name + "{" + sortedLabelString(labels) + "}"
}

// sortedLabelString 将标签按 key 排序后序列化
func sortedLabelString(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, k+"="+labels[k])
	}
	return strings.Join(pairs, ",")
}

// formatLabels 格式化 Prometheus 标签
func formatLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, fmt.Sprintf(`%s="%s"`, k, labels[k]))
	}
	return "{" + strings.Join(pairs, ",") + "}"
}

// formatLabelsWithLe 格式化带 le 标签的 Prometheus 标签
func formatLabelsWithLe(labels map[string]string, le float64) string {
	// 复制标签并添加 le
	combined := make(map[string]string, len(labels)+1)
	for k, v := range labels {
		combined[k] = v
	}
	combined["le"] = formatFloat(le)
	return formatLabels(combined)
}

// formatFloat 格式化浮点数（去掉不必要的尾零）
func formatFloat(f float64) string {
	if math.IsInf(f, 1) {
		return "+Inf"
	}
	s := fmt.Sprintf("%g", f)
	return s
}

// copyLabels 复制标签 map
func copyLabels(labels map[string]string) map[string]string {
	if labels == nil {
		return nil
	}
	result := make(map[string]string, len(labels))
	for k, v := range labels {
		result[k] = v
	}
	return result
}

// groupCounters 按指标名分组计数器
func (e *MetricExporter) groupCounters() map[string][]*CounterMetric {
	result := make(map[string][]*CounterMetric)
	for _, c := range e.counters {
		result[c.Name] = append(result[c.Name], c)
	}
	return result
}

// groupGauges 按指标名分组仪表盘
func (e *MetricExporter) groupGauges() map[string][]*GaugeMetric {
	result := make(map[string][]*GaugeMetric)
	for _, g := range e.gauges {
		result[g.Name] = append(result[g.Name], g)
	}
	return result
}

// groupHistograms 按指标名分组直方图
func (e *MetricExporter) groupHistograms() map[string][]*HistogramMetric {
	result := make(map[string][]*HistogramMetric)
	for _, h := range e.histograms {
		result[h.Name] = append(result[h.Name], h)
	}
	return result
}

// sortedStringKeys 返回排序后的 map 键（通用版本，接受任意 map[string]T）
func sortedStringKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// labelsToOTLP 将标签转为 OTLP 属性格式
func labelsToOTLP(labels map[string]string) []any {
	var attrs []any
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		attrs = append(attrs, map[string]any{
			"key":   k,
			"value": map[string]any{"stringValue": labels[k]},
		})
	}
	return attrs
}

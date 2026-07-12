// Package health 提供健康检查、SLO/SLI 监控能力。
//
// SLI（Service Level Indicator）是服务级别指标的具体度量值，
// 本文件定义了可用性和延迟两种核心 SLI 的计算函数。
package health

import (
	"math"
	"sort"
	"time"
)

// SLIMetric 服务级别指标。
type SLIMetric struct {
	// Name 指标名称（如 "availability"、"latency_p99"）
	Name string
	// Value 当前测量值
	Value float64
	// Target SLO 目标值
	Target float64
	// Window 测量时间窗口
	Window time.Duration
	// Timestamp 测量时间戳
	Timestamp time.Time
}

// CalculateAvailability 计算可用性。
//
// 公式：Availability = (total - failures) / total
// 边界情况：total=0 时返回 1.0（无请求视为完全可用）。
func CalculateAvailability(total, failures int) float64 {
	if total <= 0 {
		return 1.0
	}
	if failures < 0 {
		failures = 0
	}
	if failures > total {
		failures = total
	}
	return float64(total-failures) / float64(total)
}

// CalculateLatencyP99 计算 P99 延迟。
//
// 使用线性插值法（Nearest-Rank）计算 P99 分位值。
// 空切片返回 0。
func CalculateLatencyP99(latencies []time.Duration) time.Duration {
	if len(latencies) == 0 {
		return 0
	}

	// 复制并排序
	sorted := make([]time.Duration, len(latencies))
	copy(sorted, latencies)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	// Nearest-Rank 方法：ceil(P/100 * N) - 1 作为索引
	rank := math.Ceil(0.99 * float64(len(sorted)))
	idx := int(rank) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// CalculatePercentile 计算指定的百分位延迟值。
//
// percentile 取值范围 0-100，用于计算延迟分布。
func CalculatePercentile(latencies []time.Duration, percentile float64) time.Duration {
	if len(latencies) == 0 {
		return 0
	}
	if percentile <= 0 {
		percentile = 0
	}
	if percentile >= 100 {
		percentile = 100
	}

	sorted := make([]time.Duration, len(latencies))
	copy(sorted, latencies)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	if percentile == 100 {
		return sorted[len(sorted)-1]
	}

	rank := math.Ceil(percentile / 100.0 * float64(len(sorted)))
	idx := int(rank) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

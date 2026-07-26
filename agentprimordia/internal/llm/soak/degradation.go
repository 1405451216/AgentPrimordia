package soak

// ===== 退化检测器 =====

// Trend 趋势类型
type Trend string

const (
	TrendStable    Trend = "stable"    // 稳定
	TrendImproving Trend = "improving" // 改善
	TrendDegrading Trend = "degrading" // 退化
)

// DegradationThreshold 退化检测阈值
type DegradationThreshold struct {
	// LatencyChangePercent 延迟变化百分比阈值（超过此值视为退化）
	LatencyChangePercent float64
	// ErrorRateChangePercent 错误率变化百分比阈值
	ErrorRateChangePercent float64
	// ThroughputChangePercent 吞吐量变化百分比阈值
	ThroughputChangePercent float64
}

// DefaultDegradationThreshold 默认退化检测阈值
func DefaultDegradationThreshold() *DegradationThreshold {
	return &DegradationThreshold{
		LatencyChangePercent:    50.0, // 延迟增加 50% 以上
		ErrorRateChangePercent:  100.0, // 错误率翻倍
		ThroughputChangePercent: 30.0,  // 吞吐量下降 30%
	}
}

// DegradationReport 退化分析报告
type DegradationReport struct {
	HasDegradation           bool
	LatencyTrend             Trend
	LatencyChangePercent     float64
	ErrorRateTrend           Trend
	ErrorRateChangePercent   float64
	ThroughputTrend           Trend
	ThroughputChangePercent  float64
	// 前半段 vs 后半段对比
	FirstHalfAvgLatency     float64 // 毫秒
	SecondHalfAvgLatency    float64 // 毫秒
	FirstHalfAvgErrorRate   float64
	SecondHalfAvgErrorRate  float64
	FirstHalfAvgThroughput  float64
	SecondHalfAvgThroughput float64
}

// AnalyzeDegradation 分析采样数据，检测退化趋势
//
// 将采样数据分为前半段和后半段，对比关键指标的变化趋势
func AnalyzeDegradation(samples []Sample, threshold *DegradationThreshold) *DegradationReport {
	if threshold == nil {
		threshold = DefaultDegradationThreshold()
	}

	report := &DegradationReport{}

	if len(samples) < 2 {
		return report
	}

	// 分成前半段和后半段
	mid := len(samples) / 2
	if mid == 0 {
		mid = 1
	}
	firstHalf := samples[:mid]
	secondHalf := samples[mid:]

	// 计算前半段平均值
	report.FirstHalfAvgLatency = avgLatencyMs(firstHalf)
	report.SecondHalfAvgLatency = avgLatencyMs(secondHalf)
	report.FirstHalfAvgErrorRate = avgErrorRate(firstHalf)
	report.SecondHalfAvgErrorRate = avgErrorRate(secondHalf)
	report.FirstHalfAvgThroughput = avgThroughput(firstHalf)
	report.SecondHalfAvgThroughput = avgThroughput(secondHalf)

	// 延迟趋势
	report.LatencyChangePercent = changePercent(report.FirstHalfAvgLatency, report.SecondHalfAvgLatency)
	if report.LatencyChangePercent > threshold.LatencyChangePercent {
		report.LatencyTrend = TrendDegrading
	} else if report.LatencyChangePercent < -threshold.LatencyChangePercent {
		report.LatencyTrend = TrendImproving
	} else {
		report.LatencyTrend = TrendStable
	}

	// 错误率趋势
	report.ErrorRateChangePercent = changePercent(report.FirstHalfAvgErrorRate, report.SecondHalfAvgErrorRate)
	if report.ErrorRateChangePercent > threshold.ErrorRateChangePercent {
		report.ErrorRateTrend = TrendDegrading
	} else if report.ErrorRateChangePercent < -threshold.ErrorRateChangePercent {
		report.ErrorRateTrend = TrendImproving
	} else {
		report.ErrorRateTrend = TrendStable
	}

	// 吞吐量趋势（吞吐量下降是退化）
	report.ThroughputChangePercent = changePercent(report.FirstHalfAvgThroughput, report.SecondHalfAvgThroughput)
	if report.ThroughputChangePercent < -threshold.ThroughputChangePercent {
		report.ThroughputTrend = TrendDegrading
	} else if report.ThroughputChangePercent > threshold.ThroughputChangePercent {
		report.ThroughputTrend = TrendImproving
	} else {
		report.ThroughputTrend = TrendStable
	}

	// 总体退化判定
	report.HasDegradation = report.LatencyTrend == TrendDegrading ||
		report.ErrorRateTrend == TrendDegrading ||
		report.ThroughputTrend == TrendDegrading

	return report
}

// avgLatencyMs 计算平均延迟（毫秒）
func avgLatencyMs(samples []Sample) float64 {
	if len(samples) == 0 {
		return 0
	}
	var total float64
	for _, s := range samples {
		total += float64(s.AvgLatency.Milliseconds())
	}
	return total / float64(len(samples))
}

// avgErrorRate 计算平均错误率
func avgErrorRate(samples []Sample) float64 {
	if len(samples) == 0 {
		return 0
	}
	var total float64
	for _, s := range samples {
		if s.Requests > 0 {
			total += float64(s.Errors) / float64(s.Requests)
		}
	}
	return total / float64(len(samples))
}

// avgThroughput 计算平均吞吐量
func avgThroughput(samples []Sample) float64 {
	if len(samples) == 0 {
		return 0
	}
	var total float64
	for _, s := range samples {
		total += s.Throughput
	}
	return total / float64(len(samples))
}

// changePercent 计算变化百分比
// 返回正值表示增加，负值表示减少
func changePercent(old, new float64) float64 {
	if old == 0 {
		if new == 0 {
			return 0
		}
		return 100 // 从 0 增长到非 0
	}
	return ((new - old) / old) * 100
}

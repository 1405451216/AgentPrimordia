// Package soak 提供持续负载测试（Soak Test）框架。
//
// Soak Test 是一种长时间运行（数小时至数天）的负载测试，
// 用于检测系统在持续压力下的退化行为：
//   - 内存泄漏（RSS 缓慢增长）
//   - 延迟退化（响应时间逐渐增加）
//   - 错误率攀升（故障逐渐累积）
//   - 连接泄漏（连接数不释放）
//
// 使用方式：
//
//	runner := soak.NewRunner(soak.RunnerConfig{
//	    Duration: 1 * time.Hour,
//	    Pattern:  soak.ConstantPattern(10), // 10 RPS 恒定负载
//	    RequestFn: func(ctx context.Context) (*soak.Response, error) {
//	        // 发送请求
//	    },
//	})
//	result := runner.Run(ctx)
//	report := soak.FormatReport(result)
package soak

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Response 请求响应
type Response struct {
	Latency  time.Duration
	Status   int
	Success  bool
	BodySize int
	Error    error
}

// RunnerConfig Soak Test 运行器配置
type RunnerConfig struct {
	// Duration 测试总持续时间
	Duration time.Duration
	// Pattern 负载模式
	Pattern LoadPattern
	// RequestFn 请求发送函数
	RequestFn func(ctx context.Context) (*Response, error)
	// SamplingInterval 采样间隔（指标收集）
	SamplingInterval time.Duration
	// DegradationThreshold 退化告警阈值
	DegradationThreshold *DegradationThreshold
	// Logger 日志器
	Logger *slog.Logger
}

// RunnerConfigWithDefaults 填充默认值
func RunnerConfigWithDefaults(cfg RunnerConfig) RunnerConfig {
	if cfg.Duration == 0 {
		cfg.Duration = 30 * time.Minute
	}
	if cfg.Pattern == nil {
		cfg.Pattern = ConstantPattern(5)
	}
	if cfg.SamplingInterval == 0 {
		cfg.SamplingInterval = 10 * time.Second
	}
	if cfg.DegradationThreshold == nil {
		cfg.DegradationThreshold = DefaultDegradationThreshold()
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return cfg
}

// SoakResult Soak Test 结果
type SoakResult struct {
	Config         RunnerConfig
	StartTime      time.Time
	EndTime        time.Time
	Duration       time.Duration
	TotalRequests  int64
	TotalErrors    int64
	TotalLatencyNs int64 // 纳秒

	// 采样点
	Samples []Sample
	// 退化分析
	Degradation *DegradationReport
	// 错误
	Error error
}

// AvgLatency 计算全程平均延迟
func (r *SoakResult) AvgLatency() time.Duration {
	if r.TotalRequests == 0 {
		return 0
	}
	return time.Duration(r.TotalLatencyNs/r.TotalRequests) * time.Nanosecond
}

// ErrorRate 计算错误率
func (r *SoakResult) ErrorRate() float64 {
	if r.TotalRequests == 0 {
		return 0
	}
	return float64(r.TotalErrors) / float64(r.TotalRequests)
}

// Sample 采样点
type Sample struct {
	Timestamp   time.Time
	Elapsed     time.Duration // 从开始至今的时间
	Requests    int64         // 采样区间请求数
	Errors      int64         // 采样区间错误数
	AvgLatency  time.Duration // 采样区间平均延迟
	MaxLatency  time.Duration // 采样区间最大延迟
	MinLatency  time.Duration // 采样区间最小延迟
	P99Latency  time.Duration // 采样区间 P99 延迟
	Throughput  float64       // RPS
	SuccessRate float64       // 成功率
}

// SoakRunner Soak Test 运行器
type SoakRunner struct {
	config    RunnerConfig
	logger    *slog.Logger
	startTime time.Time

	mu      sync.Mutex
	samples []Sample
	stopCh  chan struct{}
	running atomic.Bool

	// 共享计数器
	totalRequests  atomic.Int64
	totalErrors    atomic.Int64
	totalLatencyNs atomic.Int64

	// 采样区间统计
	intervalRequests  atomic.Int64
	intervalErrors    atomic.Int64
	intervalLatencies []time.Duration // 受 mu 保护
}

// NewRunner 创建 Soak Test 运行器
func NewRunner(cfg RunnerConfig) *SoakRunner {
	cfg = RunnerConfigWithDefaults(cfg)
	return &SoakRunner{
		config: cfg,
		logger: cfg.Logger,
		stopCh: make(chan struct{}),
	}
}

// Run 运行 Soak Test
func (r *SoakRunner) Run(ctx context.Context) *SoakResult {
	r.startTime = time.Now()
	r.running.Store(true)
	defer r.running.Store(false)

	// 创建可取消的 context，用于控制采样器生命周期
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// 测试持续时间定时器
	durationTimer := time.NewTimer(r.config.Duration)
	defer durationTimer.Stop()

	// 启动采样器
	sampleDone := make(chan struct{})
	go r.sampler(runCtx, sampleDone)

	// 启动负载生成
	ticker := time.NewTimer(r.config.Pattern.NextInterval())
	defer ticker.Stop()

	r.logger.Info("Soak Test 启动",
		"duration", r.config.Duration,
		"pattern", r.config.Pattern.Name(),
	)

	for {
		select {
		case <-durationTimer.C:
			// 测试时间到，取消采样器
			cancel()
			<-sampleDone
			return r.buildResult()

		case <-ctx.Done():
			// 外部取消
			cancel()
			<-sampleDone
			return r.buildResult()

		case <-r.stopCh:
			// 手动停止
			cancel()
			<-sampleDone
			return r.buildResult()

		case <-ticker.C:
			// 发送请求
			go r.executeRequest(runCtx)
			interval := r.config.Pattern.NextInterval()
			ticker.Reset(interval)
		}
	}
}

// Stop 手动停止 Soak Test
func (r *SoakRunner) Stop() {
	if r.running.Load() {
		close(r.stopCh)
	}
}

// buildResult 构建最终结果
func (r *SoakRunner) buildResult() *SoakResult {
	result := &SoakResult{
		Config:         r.config,
		StartTime:      r.startTime,
		EndTime:        time.Now(),
		TotalRequests:  r.totalRequests.Load(),
		TotalErrors:    r.totalErrors.Load(),
		TotalLatencyNs: r.totalLatencyNs.Load(),
	}
	result.Duration = result.EndTime.Sub(result.StartTime)

	r.mu.Lock()
	result.Samples = make([]Sample, len(r.samples))
	copy(result.Samples, r.samples)
	r.mu.Unlock()

	// 退化分析
	if len(result.Samples) > 0 {
		result.Degradation = AnalyzeDegradation(result.Samples, r.config.DegradationThreshold)
	}

	r.logger.Info("Soak Test 完成",
		"total_requests", result.TotalRequests,
		"total_errors", result.TotalErrors,
		"duration", result.Duration,
		"avg_latency", result.AvgLatency(),
		"error_rate", result.ErrorRate(),
	)

	return result
}

// executeRequest 执行单个请求
func (r *SoakRunner) executeRequest(ctx context.Context) {
	start := time.Now()
	resp, err := r.config.RequestFn(ctx)
	latency := time.Since(start)

	r.totalRequests.Add(1)
	r.intervalRequests.Add(1)
	r.totalLatencyNs.Add(int64(latency))

	if err != nil || resp == nil || !resp.Success {
		r.totalErrors.Add(1)
		r.intervalErrors.Add(1)
	}

	// 记录延迟
	r.mu.Lock()
	r.intervalLatencies = append(r.intervalLatencies, latency)
	r.mu.Unlock()
}

// sampler 采样器 goroutine
func (r *SoakRunner) sampler(ctx context.Context, done chan struct{}) {
	defer close(done)

	ticker := time.NewTicker(r.config.SamplingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.takeSample()
			return
		case <-ticker.C:
			r.takeSample()
		}
	}
}

// takeSample 采集一个采样点
func (r *SoakRunner) takeSample() {
	requests := r.intervalRequests.Swap(0)
	errors := r.intervalErrors.Swap(0)

	r.mu.Lock()
	latencies := r.intervalLatencies
	r.intervalLatencies = nil
	r.mu.Unlock()

	elapsed := time.Since(r.startTime)

	sample := Sample{
		Timestamp: time.Now(),
		Elapsed:   elapsed,
		Requests:  requests,
		Errors:    errors,
	}

	if len(latencies) > 0 {
		var sum, max, min time.Duration
		min = latencies[0]
		for _, l := range latencies {
			sum += l
			if l > max {
				max = l
			}
			if l < min {
				min = l
			}
		}
		sample.AvgLatency = sum / time.Duration(len(latencies))
		sample.MaxLatency = max
		sample.MinLatency = min
		sample.P99Latency = calculateP99(latencies)

		elapsedSeconds := r.config.SamplingInterval.Seconds()
		if elapsedSeconds > 0 {
			sample.Throughput = float64(requests) / elapsedSeconds
		}
		if requests > 0 {
			sample.SuccessRate = float64(requests-errors) / float64(requests)
		}
	}

	r.mu.Lock()
	r.samples = append(r.samples, sample)
	r.mu.Unlock()

	r.logger.Info("采样",
		"elapsed", sample.Elapsed,
		"requests", sample.Requests,
		"errors", sample.Errors,
		"avg_latency", sample.AvgLatency,
		"p99_latency", sample.P99Latency,
		"throughput", sample.Throughput,
	)
}

// calculateP99 计算 P99 延迟
func calculateP99(latencies []time.Duration) time.Duration {
	if len(latencies) == 0 {
		return 0
	}
	sorted := make([]time.Duration, len(latencies))
	copy(sorted, latencies)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	idx := int(math.Ceil(0.99*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// FormatReport 将 Soak Test 结果格式化为 Markdown 报告
func FormatReport(result *SoakResult) string {
	var sb strings.Builder

	sb.WriteString("# Soak Test 报告\n\n")
	sb.WriteString(fmt.Sprintf("**持续时间**: %v\n\n", result.Duration))
	sb.WriteString(fmt.Sprintf("**总请求数**: %d\n\n", result.TotalRequests))
	sb.WriteString(fmt.Sprintf("**总错误数**: %d\n\n", result.TotalErrors))
	sb.WriteString(fmt.Sprintf("**错误率**: %.4f%%\n\n", result.ErrorRate()*100))
	sb.WriteString(fmt.Sprintf("**平均延迟**: %v\n\n", result.AvgLatency()))

	if result.Degradation != nil {
		sb.WriteString("## 退化分析\n\n")
		if result.Degradation.HasDegradation {
			sb.WriteString("⚠️ **检测到退化**\n\n")
		} else {
			sb.WriteString("✅ **未检测到退化**\n\n")
		}

		if result.Degradation.LatencyTrend != TrendStable {
			sb.WriteString(fmt.Sprintf("- 延迟趋势: %s (%.2f%% 变化)\n",
				result.Degradation.LatencyTrend,
				result.Degradation.LatencyChangePercent,
			))
		}
		if result.Degradation.ErrorRateTrend != TrendStable {
			sb.WriteString(fmt.Sprintf("- 错误率趋势: %s (%.2f%% 变化)\n",
				result.Degradation.ErrorRateTrend,
				result.Degradation.ErrorRateChangePercent,
			))
		}
		if result.Degradation.ThroughputTrend != TrendStable {
			sb.WriteString(fmt.Sprintf("- 吞吐量趋势: %s (%.2f%% 变化)\n",
				result.Degradation.ThroughputTrend,
				result.Degradation.ThroughputChangePercent,
			))
		}
	}

	if len(result.Samples) > 0 {
		sb.WriteString("\n## 采样数据\n\n")
		sb.WriteString("| 时间 | 请求数 | 错误数 | 平均延迟 | P99 延迟 | RPS | 成功率 |\n")
		sb.WriteString("|------|--------|--------|----------|----------|-----|--------|\n")
		for _, s := range result.Samples {
			sb.WriteString(fmt.Sprintf("| %v | %d | %d | %v | %v | %.1f | %.2f%% |\n",
				s.Elapsed.Round(time.Second),
				s.Requests,
				s.Errors,
				s.AvgLatency.Round(time.Millisecond),
				s.P99Latency.Round(time.Millisecond),
				s.Throughput,
				s.SuccessRate*100,
			))
		}
	}

	return sb.String()
}

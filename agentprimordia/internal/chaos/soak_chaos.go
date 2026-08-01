// Phase 2.4: 混沌×Soak — CI/CD 集成
//
// 将 chaos.Engine 与 soak.SoakRunner 集成：
//   - Soak Test 运行期间自动注入混沌故障
//   - 退化检测自动触发混沌实验报告
//   - 提供 CI/CD 友好的运行器和报告格式
//
// 使用方式：
//
//	runner := chaos.NewSoakChaosRunner(chaos.SoakChaosConfig{
//	    SoakDuration: 30 * time.Minute,
//	    ChaosInterval: 5 * time.Minute,
//	    Experiments: []chaos.Experiment{...},
//	})
//	result := runner.Run(ctx)

package chaos

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// ===== Soak + Chaos 联动配置 =====

// SoakChaosConfig Soak + Chaos 联动配置
type SoakChaosConfig struct {
	// SoakDuration Soak 测试总持续时间
	SoakDuration time.Duration
	// ChaosInterval 混沌实验注入间隔（每隔多久注入一次）
	ChaosInterval time.Duration
	// ChaosDuration 每次混沌实验持续时间
	ChaosDuration time.Duration
	// Experiments 要循环注入的实验列表
	Experiments []Experiment
	// RequestFn Soak 请求函数
	RequestFn func(ctx context.Context) (*SoakResponse, error)
	// RequestsPerSecond 每秒请求数
	RequestsPerSecond int
	// DegradationThreshold 退化阈值（延迟变化百分比）
	DegradationThreshold float64
	// StopOnDegradation 检测到退化时是否停止
	StopOnDegradation bool
	// Logger 日志器
	Logger *slog.Logger
}

// SoakChaosConfigWithDefaults 填充默认值
func SoakChaosConfigWithDefaults(cfg SoakChaosConfig) SoakChaosConfig {
	if cfg.SoakDuration == 0 {
		cfg.SoakDuration = 30 * time.Minute
	}
	if cfg.ChaosInterval == 0 {
		cfg.ChaosInterval = 5 * time.Minute
	}
	if cfg.ChaosDuration == 0 {
		cfg.ChaosDuration = 30 * time.Second
	}
	if cfg.RequestsPerSecond <= 0 {
		cfg.RequestsPerSecond = 5
	}
	if cfg.DegradationThreshold <= 0 {
		cfg.DegradationThreshold = 50.0
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return cfg
}

// SoakResponse Soak 请求响应
type SoakResponse struct {
	Latency time.Duration
	Success bool
	Error   error
}

// ===== Soak + Chaos 运行结果 =====

// SoakChaosResult Soak + Chaos 联动结果
type SoakChaosResult struct {
	// StartTime 开始时间
	StartTime time.Time
	// EndTime 结束时间
	EndTime time.Time
	// Duration 实际持续时间
	Duration time.Duration
	// TotalRequests 总请求数
	TotalRequests int64
	// TotalErrors 总错误数
	TotalErrors int64
	// Samples 采样点
	Samples []SoakSample
	// ChaosResults 混沌实验结果
	ChaosResults []*ExperimentResult
	// DegradationDetected 是否检测到退化
	DegradationDetected bool
	// DegradationDetails 退化详情
	DegradationDetails string
	// StoppedEarly 是否提前停止
	StoppedEarly bool
	// Error 运行错误
	Error error
}

// SoakSample 采样点
type SoakSample struct {
	Timestamp    time.Time
	Requests     int64
	Errors       int64
	AvgLatency   time.Duration
	P99Latency   time.Duration
	ChaosActive  bool
	ChaosName    string
}

// AvgLatencyMs 平均延迟（毫秒）
func (r *SoakChaosResult) AvgLatencyMs() float64 {
	if r.TotalRequests == 0 {
		return 0
	}
	var total float64
	for _, s := range r.Samples {
		total += float64(s.AvgLatency.Milliseconds())
	}
	return total / float64(len(r.Samples))
}

// ErrorRate 错误率
func (r *SoakChaosResult) ErrorRate() float64 {
	if r.TotalRequests == 0 {
		return 0
	}
	return float64(r.TotalErrors) / float64(r.TotalRequests)
}

// ===== Soak + Chaos 运行器 =====

// SoakChaosRunner Soak + Chaos 联动运行器
type SoakChaosRunner struct {
	config SoakChaosConfig
	engine *ChaosEngine
	logger *slog.Logger

	mu      sync.Mutex
	running bool
}

// NewSoakChaosRunner 创建 Soak + Chaos 联动运行器
func NewSoakChaosRunner(cfg SoakChaosConfig) *SoakChaosRunner {
	cfg = SoakChaosConfigWithDefaults(cfg)
	return &SoakChaosRunner{
		config: cfg,
		engine: NewEngine().WithLogger(cfg.Logger),
		logger: cfg.Logger,
	}
}

// Run 运行 Soak + Chaos 联动测试
//
// 流程：
//  1. 启动 Soak 负载（恒定 RPS）
//  2. 每隔 ChaosInterval 注入一个混沌实验
//  3. 持续采样指标
//  4. 检测退化 → 可选提前停止
//  5. 生成综合报告
func (r *SoakChaosRunner) Run(ctx context.Context) *SoakChaosResult {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return &SoakChaosResult{Error: fmt.Errorf("soak_chaos: already running")}
	}
	r.running = true
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.running = false
		r.mu.Unlock()
	}()

	result := &SoakChaosResult{
		StartTime: time.Now(),
	}

	// 创建超时 context
	runCtx, cancel := context.WithTimeout(ctx, r.config.SoakDuration)
	defer cancel()

	r.logger.Info("Soak + Chaos 联动测试启动",
		"duration", r.config.SoakDuration,
		"rps", r.config.RequestsPerSecond,
		"chaos_interval", r.config.ChaosInterval,
		"experiments", len(r.config.Experiments),
	)

	// 采样通道
	sampleCh := make(chan SoakSample, 100)
	// 混沌结果通道
	chaosCh := make(chan *ExperimentResult, 10)

	// 启动 Soak 负载
	var soakWG sync.WaitGroup
	soakWG.Add(1)
	go func() {
		defer soakWG.Done()
		r.runSoakLoad(runCtx, sampleCh)
	}()

	// 启动混沌注入调度
	var chaosWG sync.WaitGroup
	chaosWG.Add(1)
	go func() {
		defer chaosWG.Done()
		r.scheduleChaos(runCtx, chaosCh)
	}()

	// 收集采样
	var samples []SoakSample
	var totalReqs, totalErrs int64
	sampleInterval := 10 * time.Second
	ticker := time.NewTicker(sampleInterval)
	defer ticker.Stop()

	// 退化检测
	var prevAvgLatency float64

collectLoop:
	for {
		select {
		case <-runCtx.Done():
			break collectLoop
		case sample := <-sampleCh:
			samples = append(samples, sample)
			totalReqs += sample.Requests
			totalErrs += sample.Errors

			// 退化检测
			currentLatency := float64(sample.AvgLatency.Milliseconds())
			if prevAvgLatency > 0 && currentLatency > 0 {
				change := ((currentLatency - prevAvgLatency) / prevAvgLatency) * 100
				if change > r.config.DegradationThreshold {
					result.DegradationDetected = true
					result.DegradationDetails = fmt.Sprintf(
						"延迟退化 %.1f%%（%.1fms → %.1fms）",
						change, prevAvgLatency, currentLatency,
					)
					r.logger.Warn("检测到退化",
						"change_percent", change,
						"prev_latency_ms", prevAvgLatency,
						"current_latency_ms", currentLatency,
					)
					if r.config.StopOnDegradation {
						result.StoppedEarly = true
						cancel()
						break collectLoop
					}
				}
			}
			prevAvgLatency = currentLatency

		case chaosResult := <-chaosCh:
			result.ChaosResults = append(result.ChaosResults, chaosResult)
		}
	}

	// 等待 goroutine 结束
	soakWG.Wait()
	chaosWG.Wait()

	// 收集剩余
	close(sampleCh)
	for sample := range sampleCh {
		samples = append(samples, sample)
		totalReqs += sample.Requests
		totalErrs += sample.Errors
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	result.Samples = samples
	result.TotalRequests = totalReqs
	result.TotalErrors = totalErrs

	r.logger.Info("Soak + Chaos 联动测试完成",
		"duration", result.Duration,
		"total_requests", result.TotalRequests,
		"total_errors", result.TotalErrors,
		"chaos_experiments", len(result.ChaosResults),
		"degradation", result.DegradationDetected,
	)

	return result
}

// ===== 内部方法 =====

// runSoakLoad 运行 Soak 负载
func (r *SoakChaosRunner) runSoakLoad(ctx context.Context, sampleCh chan<- SoakSample) {
	if r.config.RequestFn == nil {
		return
	}

	interval := time.Second / time.Duration(r.config.RequestsPerSecond)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	sampleTicker := time.NewTicker(10 * time.Second)
	defer sampleTicker.Stop()

	var sampleReqs, sampleErrs int64
	var sampleLatencies []time.Duration

	for {
		select {
		case <-ctx.Done():
			// 发送最后一个采样
			if len(sampleLatencies) > 0 {
				sampleCh <- r.buildSample(sampleReqs, sampleErrs, sampleLatencies, false, "")
			}
			return
		case <-ticker.C:
			resp, err := r.config.RequestFn(ctx)
			sampleReqs++
			if err != nil || (resp != nil && !resp.Success) {
				sampleErrs++
			}
			if resp != nil {
				sampleLatencies = append(sampleLatencies, resp.Latency)
			}
		case <-sampleTicker.C:
			if len(sampleLatencies) > 0 {
				sampleCh <- r.buildSample(sampleReqs, sampleErrs, sampleLatencies, false, "")
				sampleReqs = 0
				sampleErrs = 0
				sampleLatencies = nil
			}
		}
	}
}

// scheduleChaos 调度混沌实验
func (r *SoakChaosRunner) scheduleChaos(ctx context.Context, chaosCh chan<- *ExperimentResult) {
	if len(r.config.Experiments) == 0 {
		return
	}

	// 等待第一个间隔
	timer := time.NewTimer(r.config.ChaosInterval)
	defer timer.Stop()

	expIdx := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			// 选择下一个实验
			exp := r.config.Experiments[expIdx%len(r.config.Experiments)]
			expIdx++

			// 设置实验持续时间
			if exp.Duration == 0 {
				exp.Duration = r.config.ChaosDuration
			}

			r.logger.Info("注入混沌实验",
				"name", exp.Name,
				"duration", exp.Duration,
			)

			// 运行实验
			result, err := r.engine.Run(ctx, exp)
			if err != nil {
				r.logger.Warn("混沌实验执行失败",
					"name", exp.Name,
					"error", err,
				)
			} else {
				chaosCh <- result
			}

			// 重置计时器
			timer.Reset(r.config.ChaosInterval)
		}
	}
}

// buildSample 构建采样点
func (r *SoakChaosRunner) buildSample(reqs, errs int64, latencies []time.Duration, chaosActive bool, chaosName string) SoakSample {
	var avgLatency time.Duration
	var p99Latency time.Duration

	if len(latencies) > 0 {
		var total time.Duration
		for _, l := range latencies {
			total += l
		}
		avgLatency = total / time.Duration(len(latencies))

		// P99
		sorted := make([]time.Duration, len(latencies))
		copy(sorted, latencies)
		sortDurations(sorted)
		p99Idx := int(float64(len(sorted)) * 0.99)
		if p99Idx >= len(sorted) {
			p99Idx = len(sorted) - 1
		}
		p99Latency = sorted[p99Idx]
	}

	return SoakSample{
		Timestamp:   time.Now(),
		Requests:    reqs,
		Errors:      errs,
		AvgLatency:  avgLatency,
		P99Latency:  p99Latency,
		ChaosActive: chaosActive,
		ChaosName:   chaosName,
	}
}

// sortDurations 排序延迟切片
func sortDurations(d []time.Duration) {
	for i := 1; i < len(d); i++ {
		for j := i; j > 0 && d[j] < d[j-1]; j-- {
			d[j], d[j-1] = d[j-1], d[j]
		}
	}
}

// ===== 报告生成 =====

// FormatSoakChaosReport 生成 Soak + Chaos 综合报告（Markdown）
func FormatSoakChaosReport(result *SoakChaosResult) string {
	var sb strings.Builder

	sb.WriteString("# Soak + Chaos 联动测试报告\n\n")
	sb.WriteString(fmt.Sprintf("**持续时间**: %v\n\n", result.Duration))
	sb.WriteString(fmt.Sprintf("**总请求数**: %d\n\n", result.TotalRequests))
	sb.WriteString(fmt.Sprintf("**总错误数**: %d\n\n", result.TotalErrors))
	sb.WriteString(fmt.Sprintf("**错误率**: %.2f%%\n\n", result.ErrorRate()*100))
	sb.WriteString(fmt.Sprintf("**平均延迟**: %.1f ms\n\n", result.AvgLatencyMs()))

	// 退化检测
	if result.DegradationDetected {
		sb.WriteString("## ⚠️ 退化检测\n\n")
		sb.WriteString("**状态**: 检测到退化\n\n")
		sb.WriteString(fmt.Sprintf("**详情**: %s\n\n", result.DegradationDetails))
		if result.StoppedEarly {
			sb.WriteString("**操作**: 已提前停止测试\n\n")
		}
	} else {
		sb.WriteString("## ✅ 退化检测\n\n")
		sb.WriteString("**状态**: 未检测到退化\n\n")
	}

	// 混沌实验结果
	if len(result.ChaosResults) > 0 {
		sb.WriteString("## 混沌实验结果\n\n")
		sb.WriteString("| # | 实验 | 状态 | 假设验证 | 持续时间 |\n")
		sb.WriteString("|---|------|------|----------|----------|\n")
		for i, cr := range result.ChaosResults {
			validated := "✅"
			if !cr.HypothesisValidated {
				validated = "❌"
			}
			sb.WriteString(fmt.Sprintf("| %d | %s | %s | %s | %v |\n",
				i+1, cr.Experiment.Name, cr.Status, validated, cr.Duration))
		}
		sb.WriteString("\n")
	}

	// 采样摘要
	if len(result.Samples) > 0 {
		sb.WriteString("## 采样摘要\n\n")
		sb.WriteString(fmt.Sprintf("**采样点数**: %d\n\n", len(result.Samples)))

		// 前 5 个和后 5 个采样
		sb.WriteString("### 初始采样\n\n")
		sb.WriteString("| 时间 | 请求 | 错误 | 平均延迟 | P99 |\n")
		sb.WriteString("|------|------|------|----------|-----|\n")
		for i := 0; i < 5 && i < len(result.Samples); i++ {
			s := result.Samples[i]
			sb.WriteString(fmt.Sprintf("| %s | %d | %d | %v | %v |\n",
				s.Timestamp.Format("15:04:05"), s.Requests, s.Errors, s.AvgLatency, s.P99Latency))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

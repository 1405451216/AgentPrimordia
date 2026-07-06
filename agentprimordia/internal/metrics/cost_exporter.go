package metrics

import (
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// CostSource 抽象 CostTracker 数据源（让 metrics 包不直接依赖 agent 包，
// 避免与 internal/agent/*_test.go 的反向引用产生 import cycle）
//
// 生产实现是 *agent.CostTracker（其 Summary/Records/LastRecord 方法自动满足）。
// 用户也可以为测试桩实现该接口。
type CostSource interface {
	// Summary 返回成本汇总（含按 model 聚合的 ByModel 维度）
	Summary() CostSourceSummary
	// Records 返回所有成本记录（按时间顺序）
	Records() []CostSourceRecord
}

// CostSourceSummary 抽象汇总数据
type CostSourceSummary struct {
	ByModel map[string]CostSourceModelCost
}

// CostSourceModelCost 单模型成本
type CostSourceModelCost struct {
	CostUSD float64
	Calls   int
	Tokens  int64
}

// CostSourceRecord 抽象单次调用记录
type CostSourceRecord struct {
	Model            string
	AgentName        string
	CostUSD          float64
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// CostExporter 将 CostSource 数据导出为 Prometheus 指标
//
// 该导出器定期从数据源拉取数据并写入 Prometheus 格式的指标。
// 设计动机：CostSource 内部存储为 rich records（cost records），
// 但 Prometheus 仅支持 counter/gauge/histogram 三种类型，因此需要
// 在两者之间架设桥接。
//
// 暴露的指标（Prometheus 格式）：
//
//	ap_cost_usd_total{provider, model, agent_name}        -- Counter
//	ap_cost_tokens_total{kind="prompt|completion", ...}  -- Counter
//	ap_cost_calls_total{provider, model, agent_name}     -- Counter
//	ap_cost_last_call_usd{provider, model, agent_name}   -- Gauge
type CostExporter struct {
	source     CostSource
	logger     *slog.Logger
	interval   time.Duration
	stopCh     chan struct{}
	stopped    atomic.Bool
	mu         sync.RWMutex
	lastExport time.Time
}

// CostExporterConfig 导出器配置
type CostExporterConfig struct {
	// Source 要导出的成本数据源；nil 时 ExportOnce 是 no-op
	Source CostSource
	// Interval 导出间隔；默认 15s
	Interval time.Duration
	// Logger 日志器；nil 时使用 slog.Default()
	Logger *slog.Logger
}

// NewCostExporter 创建成本导出器
func NewCostExporter(cfg CostExporterConfig) *CostExporter {
	if cfg.Interval <= 0 {
		cfg.Interval = 15 * time.Second
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &CostExporter{
		source:   cfg.Source,
		logger:   cfg.Logger,
		interval: cfg.Interval,
		stopCh:   make(chan struct{}),
	}
}

// Start 启动后台导出循环
func (e *CostExporter) Start() {
	if e.stopped.Load() {
		return
	}
	go e.loop()
}

// Stop 停止后台循环（幂等）
func (e *CostExporter) Stop() {
	if e.stopped.Swap(true) {
		return
	}
	close(e.stopCh)
}

// loop 周期性拉取 CostSource 数据并更新指标
func (e *CostExporter) loop() {
	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()
	for {
		select {
		case <-e.stopCh:
			return
		case <-ticker.C:
			if err := e.ExportOnce(); err != nil {
				e.logger.Warn("cost export failed", "error", err)
			}
		}
	}
}

// ExportOnce 执行一次导出，返回错误（如果有）
//
// 调用方应确保 CostSource 是非 nil。
func (e *CostExporter) ExportOnce() error {
	if e.source == nil {
		return nil
	}

	summary := e.source.Summary()

	e.mu.Lock()
	e.lastExport = time.Now()
	e.mu.Unlock()

	// 按 model 维度聚合并写入 Prometheus counter
	for model, modelCost := range summary.ByModel {
		provider := inferProviderFromModel(model)
		// ap_cost_usd_total{provider, model}
		RecordCostUSD(provider, model, "", modelCost.CostUSD)
		// ap_cost_calls_total
		RecordCostCalls(provider, model, "", float64(modelCost.Calls))
		// ap_cost_tokens_total (按 prompt/completion 拆分)
		RecordCostTokens(provider, model, "", "total", float64(modelCost.Tokens))
	}

	// ap_cost_last_call_usd（基于 records 末尾）
	records := e.source.Records()
	if len(records) > 0 {
		last := records[len(records)-1]
		provider := inferProviderFromModel(last.Model)
		SetLastCostUSD(provider, last.Model, last.AgentName, last.CostUSD)
	}

	return nil
}

// LastExportTime 返回最后一次成功导出的时间
func (e *CostExporter) LastExportTime() time.Time {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.lastExport
}

// inferProviderFromModel 从模型名推断 provider
//
// 这是一个简化实现：基于常见的 provider 前缀匹配（用 strings.HasPrefix 避免切片越界）。
// 对于混合部署或多 provider 场景，建议在 CostSource 实现层维护 provider 字段。
func inferProviderFromModel(model string) string {
	switch {
	case strings.HasPrefix(model, "gpt-") || strings.HasPrefix(model, "o1") || strings.HasPrefix(model, "o3"):
		return "openai"
	case strings.HasPrefix(model, "claude"):
		return "anthropic"
	case strings.HasPrefix(model, "gemini"):
		return "google"
	case strings.HasPrefix(model, "qwen"):
		return "alibaba"
	case strings.HasPrefix(model, "deepseek"):
		return "deepseek"
	case strings.HasPrefix(model, "mistral") || strings.HasPrefix(model, "mixtral"):
		return "mistral"
	case strings.HasPrefix(model, "glm") || strings.HasPrefix(model, "chatglm"):
		return "zhipu"
	case strings.HasPrefix(model, "cohere") || strings.HasPrefix(model, "command"):
		return "cohere"
	}
	return "unknown"
}

// CostExporterSnapshot 导出器运行时快照（用于 /metrics 端点调试）
type CostExporterSnapshot struct {
	Enabled    bool      `json:"enabled"`
	LastExport time.Time `json:"last_export"`
	Interval   string    `json:"interval"`
	SourceAddr string    `json:"source_addr"`
}

// Snapshot 返回导出器运行时快照
func (e *CostExporter) Snapshot() CostExporterSnapshot {
	e.mu.RLock()
	last := e.lastExport
	e.mu.RUnlock()

	addr := "<nil>"
	if e.source != nil {
		addr = "configured"
	}

	return CostExporterSnapshot{
		Enabled:    !e.stopped.Load(),
		LastExport: last,
		Interval:   e.interval.String(),
		SourceAddr: addr,
	}
}

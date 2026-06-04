package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

const (
	defaultMetricsAddr   = ":9090"
	defaultExportBufSize = 1024
)

// ===== Prometheus HTTP Handler =====

// PrometheusHandler 提供 /metrics 端点，供 Prometheus 抓取
type PrometheusHandler struct {
	metrics *AgentMetrics
	server  *http.Server
	logger  *slog.Logger
	ready   chan struct{}
}

// NewPrometheusHandler 创建 Prometheus HTTP handler
func NewPrometheusHandler(metrics *AgentMetrics, addr string) *PrometheusHandler {
	if addr == "" {
		addr = defaultMetricsAddr
	}
	h := &PrometheusHandler{
		metrics: metrics,
		logger:  slog.Default(),
		ready:   make(chan struct{}),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", h.handleMetrics)
	mux.HandleFunc("/health", h.handleHealth)

	h.server = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	return h
}

// Start 启动 Prometheus HTTP 服务器
func (h *PrometheusHandler) Start() error {
	h.logger.Info("Prometheus 指标服务器启动", "addr", h.server.Addr)
	go func() {
		close(h.ready)
		if err := h.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			h.logger.Error("Prometheus 服务器错误", "error", err)
		}
	}()
	<-h.ready
	return nil
}

// Stop 优雅停止 Prometheus 服务器
func (h *PrometheusHandler) Stop(ctx context.Context) error {
	return h.server.Shutdown(ctx)
}

func (h *PrometheusHandler) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = fmt.Fprint(w, h.metrics.String())
}

func (h *PrometheusHandler) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// ===== OpenTelemetry 导出适配器 =====

// TelemetryExporter 是可观测性导出接口
type TelemetryExporter interface {
	// ExportMetrics 导出指标快照
	ExportMetrics(snapshot MetricsSnapshot)
	// ExportEvent 导出事件
	ExportEvent(eventType string, source string, payload any)
	// Close 关闭导出器
	Close() error
}

// MultiExporter 组合多个导出器
type MultiExporter struct {
	exporters []TelemetryExporter
	mu        sync.RWMutex
}

// NewMultiExporter 创建多导出器组合
func NewMultiExporter(exporters ...TelemetryExporter) *MultiExporter {
	return &MultiExporter{exporters: exporters}
}

// Add 添加导出器
func (m *MultiExporter) Add(exporter TelemetryExporter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.exporters = append(m.exporters, exporter)
}

// ExportMetrics 导出指标到所有导出器
func (m *MultiExporter) ExportMetrics(snapshot MetricsSnapshot) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, e := range m.exporters {
		e.ExportMetrics(snapshot)
	}
}

// ExportEvent 导出事件到所有导出器
func (m *MultiExporter) ExportEvent(eventType string, source string, payload any) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, e := range m.exporters {
		e.ExportEvent(eventType, source, payload)
	}
}

// Close 关闭所有导出器
func (m *MultiExporter) Close() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var lastErr error
	for _, e := range m.exporters {
		if err := e.Close(); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// ===== 日志导出器 =====

// LogExporter 将指标和事件输出到 slog
type LogExporter struct {
	logger *slog.Logger
}

// NewLogExporter 创建日志导出器
func NewLogExporter() *LogExporter {
	return &LogExporter{logger: slog.Default()}
}

// ExportMetrics 记录指标到日志
func (l *LogExporter) ExportMetrics(snapshot MetricsSnapshot) {
	l.logger.Info("指标导出",
		"llm_calls", snapshot.LLMTotalCalls,
		"llm_errors", snapshot.LLMTotalErrors,
		"tool_calls", snapshot.ToolTotalCalls,
		"tool_errors", snapshot.ToolTotalErrors,
		"total_turns", snapshot.TotalTurns,
		"active_agents", snapshot.ActiveAgents,
	)
}

// ExportEvent 记录事件到日志
func (l *LogExporter) ExportEvent(eventType string, source string, payload any) {
	l.logger.Info("事件",
		"type", eventType,
		"source", source,
		"payload", payload,
	)
}

// Close 关闭日志导出器
func (l *LogExporter) Close() error {
	return nil
}

// ===== JSON 文件导出器 =====

// JSONExporter 将指标导出为 JSON 行格式文件
type JSONExporter struct {
	output chan string
	done   chan struct{}
}

// NewJSONExporter 创建 JSON 导出器，写入指定输出
// 使用方式：通过 MetricsChan() 获取 channel 进行消费
func NewJSONExporter(bufferSize int) *JSONExporter {
	if bufferSize <= 0 {
		bufferSize = defaultExportBufSize
	}
	return &JSONExporter{
		output: make(chan string, bufferSize),
		done:   make(chan struct{}),
	}
}

func (j *JSONExporter) ExportMetrics(snapshot MetricsSnapshot) {
	type metricLine struct {
		Timestamp    string `json:"ts"`
		Type         string `json:"type"`
		LLMCalls     int64  `json:"llm_calls"`
		LLMErrors    int64  `json:"llm_errors"`
		ToolCalls    int64  `json:"tool_calls"`
		ToolErrors   int64  `json:"tool_errors"`
		Turns        int64  `json:"turns"`
		ActiveAgents int64  `json:"active_agents"`
	}
	ml := metricLine{
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		Type:         "metrics",
		LLMCalls:     snapshot.LLMTotalCalls,
		LLMErrors:    snapshot.LLMTotalErrors,
		ToolCalls:    snapshot.ToolTotalCalls,
		ToolErrors:   snapshot.ToolTotalErrors,
		Turns:        snapshot.TotalTurns,
		ActiveAgents: snapshot.ActiveAgents,
	}
	data, err := json.Marshal(ml)
	if err != nil {
		return
	}
	select {
	case j.output <- string(data):
	default:
	}
}

// ExportEvent 导出事件为 JSON
func (j *JSONExporter) ExportEvent(eventType string, source string, payload any) {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		payloadJSON = []byte("null")
	}
	line := fmt.Sprintf(`{"ts":"%s","type":"event","event_type":"%s","source":"%s","payload":%s}`,
		time.Now().UTC().Format(time.RFC3339),
		eventType,
		source,
		string(payloadJSON),
	)
	select {
	case j.output <- line:
	default:
	}
}

// Channel 返回输出 channel，用于外部消费
func (j *JSONExporter) Channel() <-chan string {
	return j.output
}

// Close 关闭 JSON 导出器
func (j *JSONExporter) Close() error {
	close(j.done)
	return nil
}

// ===== 指标周期性导出 =====

// MetricsExporter 是一个后台 goroutine，定期导出指标
type MetricsExporter struct {
	metrics  *AgentMetrics
	exporter TelemetryExporter
	interval time.Duration
	cancel   context.CancelFunc
	logger   *slog.Logger
}

// NewMetricsExporter 创建周期性指标导出器
func NewMetricsExporter(metrics *AgentMetrics, exporter TelemetryExporter, interval time.Duration) *MetricsExporter {
	if interval <= 0 {
		interval = 15 * time.Second
	}
	return &MetricsExporter{
		metrics:  metrics,
		exporter: exporter,
		interval: interval,
		logger:   slog.Default(),
	}
}

// Start 启动周期性导出
func (e *MetricsExporter) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	e.cancel = cancel

	go func() {
		ticker := time.NewTicker(e.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				snapshot := e.metrics.Snapshot()
				e.exporter.ExportMetrics(snapshot)
			}
		}
	}()
	e.logger.Info("指标导出器启动", "interval", e.interval)
}

// Stop 停止导出器
func (e *MetricsExporter) Stop() {
	if e.cancel != nil {
		e.cancel()
	}
	e.logger.Info("指标导出器停止")
}

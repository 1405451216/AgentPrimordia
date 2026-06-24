package metrics

import "net/http"

// Handler 提供 Prometheus 兼容的 /metrics HTTP 端点
// 可组合到任意 http.ServeMux 中，与 PrometheusHandler（独立服务器）互补
type Handler struct {
	metrics *AgentMetrics
}

// NewHandler 创建 metrics HTTP handler
func NewHandler(m *AgentMetrics) *Handler {
	return &Handler{metrics: m}
}

// ServeHTTP 实现 http.Handler 接口
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(h.metrics.String()))
}

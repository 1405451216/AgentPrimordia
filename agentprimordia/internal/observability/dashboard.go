package observability

import (
	"encoding/json"
	"net/http"
	"strings"
)

// DashboardHandler 提供仪表盘 HTTP API。
//
// 路由：
//   - GET /dashboard/summary  — 全局摘要（total_traces, total_spans, total_audits, agents）
//   - GET /dashboard/alerts   — 当前告警事件列表
func DashboardHandler(store *CorrelationStore, engine *AlertEngine) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/dashboard/summary", func(w http.ResponseWriter, r *http.Request) {
		if !strings.EqualFold(r.Method, http.MethodGet) {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		traces := store.List(0)
		totalSpans := 0
		totalAudits := 0
		agentSet := make(map[string]bool)

		for _, rt := range traces {
			totalSpans += len(rt.Spans)
			totalAudits += len(rt.AuditEvents)
			if rt.AgentName != "" {
				agentSet[rt.AgentName] = true
			}
		}

		summary := map[string]any{
			"total_traces": len(traces),
			"total_spans":  totalSpans,
			"total_audits": totalAudits,
			"agents":       len(agentSet),
		}
		writeJSON(w, http.StatusOK, summary)
	})

	mux.HandleFunc("/dashboard/alerts", func(w http.ResponseWriter, r *http.Request) {
		if !strings.EqualFold(r.Method, http.MethodGet) {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		alerts := engine.Evaluate()
		if alerts == nil {
			alerts = []AlertEvent{}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"alerts": alerts,
			"count":  len(alerts),
		})
	})

	return mux
}

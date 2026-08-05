package observability

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// Handler 提供单请求全链路回溯的 HTTP 查询 API。
//
// 路由：
//   - GET /traces          → 请求列表（?limit= 控制数量，默认 20）
//   - GET /traces/{id}     → 单请求全链路视图（trace + 指标 + 审计）
//
// 供 debugger / admin 等调试与管理端消费，实现"单请求可全链路回溯"。
func Handler(store *CorrelationStore) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/traces", func(w http.ResponseWriter, r *http.Request) {
		if !strings.EqualFold(r.Method, http.MethodGet) {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		limit := 20
		if v := r.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				limit = n
			}
		}
		traces := store.List(limit)
		SortTraceByStart(traces, true)
		writeJSON(w, http.StatusOK, map[string]any{
			"total":  len(traces),
			"traces": traces,
		})
	})

	mux.HandleFunc("/traces/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.EqualFold(r.Method, http.MethodGet) {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/traces/")
		if id == "" {
			http.Error(w, "trace id required", http.StatusBadRequest)
			return
		}
		rt := store.Get(id)
		if rt == nil {
			http.Error(w, "trace not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, rt)
	})

	return mux
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

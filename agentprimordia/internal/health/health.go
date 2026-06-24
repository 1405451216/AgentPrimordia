package health

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// Checker 健康检查接口
type Checker interface {
	Name() string
	Check(ctx context.Context) error
}

// HealthChecker 聚合多个健康检查器
type HealthChecker struct {
	mu       sync.RWMutex
	checkers []Checker
	ready    atomic.Bool
}

// NewChecker 创建健康检查器
func NewChecker() *HealthChecker {
	return &HealthChecker{
		checkers: make([]Checker, 0),
	}
}

// Register 注册健康检查器
func (h *HealthChecker) Register(c Checker) {
	h.mu.Lock()
	h.checkers = append(h.checkers, c)
	h.mu.Unlock()
}

// SetReady 标记服务就绪
func (h *HealthChecker) SetReady() {
	h.ready.Store(true)
}

// ServeHTTP 处理 /healthz 和 /readyz 请求
func (h *HealthChecker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/healthz":
		h.handleHealthz(w, r)
	case "/readyz":
		h.handleReadyz(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *HealthChecker) handleHealthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	h.mu.RLock()
	checkers := make([]Checker, len(h.checkers))
	copy(checkers, h.checkers)
	h.mu.RUnlock()

	type componentStatus struct {
		Name   string `json:"name"`
		Status string `json:"status"`
		Error  string `json:"error,omitempty"`
	}

	statuses := make([]componentStatus, 0, len(checkers))
	allHealthy := true

	for _, c := range checkers {
		cs := componentStatus{Name: c.Name(), Status: "ok"}
		if err := c.Check(ctx); err != nil {
			cs.Status = "error"
			cs.Error = err.Error()
			allHealthy = false
		}
		statuses = append(statuses, cs)
	}

	resp := map[string]any{
		"status":     "ok",
		"components": statuses,
	}
	if !allHealthy {
		resp["status"] = "degraded"
	}

	w.Header().Set("Content-Type", "application/json")
	if !allHealthy {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	json.NewEncoder(w).Encode(resp)
}

func (h *HealthChecker) handleReadyz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !h.ready.Load() {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status":"not ready"}`))
		return
	}
	_, _ = w.Write([]byte(`{"status":"ready"}`))
}

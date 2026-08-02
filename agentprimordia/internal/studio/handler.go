// handler.go — Studio /api/v1/* HTTP 端点实现
package studio

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// registerRoutes 注册 Studio 面板端点（Go 1.22+ 方法路由）。
func (h *StudioHandler) registerRoutes() {
	// Chaos Lab
	h.mux.HandleFunc("GET /api/v1/chaos/experiments", h.listExperiments)
	h.mux.HandleFunc("POST /api/v1/chaos/experiments", h.createExperiment)
	// Cluster Dashboard
	h.mux.HandleFunc("GET /api/v1/cluster/status", h.clusterStatus)
	// Learning Monitor
	h.mux.HandleFunc("GET /api/v1/learning/stats", h.learningStats)
	h.mux.HandleFunc("GET /api/v1/learning/capabilities", h.learningCapabilities)
	h.mux.HandleFunc("GET /api/v1/learning/pipeline/stats", h.learningPipelineStats)
	// Marketplace
	h.mux.HandleFunc("GET /api/v1/marketplace/templates", h.marketplaceTemplates)
	h.mux.HandleFunc("POST /api/v1/marketplace/deploy", h.marketplaceDeploy)
	// Autonomy Monitor (v3.3)
	h.mux.HandleFunc("GET /api/v1/autonomy/goals", h.autonomyGoals)
	h.mux.HandleFunc("GET /api/v1/autonomy/alerts", h.autonomyAlerts)
	h.mux.HandleFunc("POST /api/v1/autonomy/goals/{id}/resume", h.autonomyResume)
	// Skill Library (v3.4)
	h.mux.HandleFunc("GET /api/v1/skills", h.skillsList)
	h.mux.HandleFunc("POST /api/v1/skills/{id}/verify", h.skillsVerify)
	h.mux.HandleFunc("POST /api/v1/skills/{id}/deprecate", h.skillsDeprecate)
	// A2A Interop (v3.5)
	h.mux.HandleFunc("GET /api/v1/a2a/interop/status", h.a2aInteropStatus)
	// Realtime Console (v3.6)
	h.mux.HandleFunc("GET /api/v1/realtime/sessions", h.realtimeSessions)
	h.mux.HandleFunc("GET /api/v1/realtime/events", h.realtimeEvents)
	h.mux.HandleFunc("POST /api/v1/realtime/sessions/{id}/barge-in", h.realtimeBargeIn)
}

// ===== Chaos Lab =====

func (h *StudioHandler) listExperiments(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	items, err := h.chaos.ListExperiments(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if items == nil {
		items = []ExperimentResult{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *StudioHandler) createExperiment(w http.ResponseWriter, r *http.Request) {
	var req CreateExperimentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求体解析失败: " + err.Error()})
		return
	}
	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "实验名称不能为空"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := h.chaos.CreateExperiment(ctx, req); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "created"})
}

// ===== Cluster Dashboard =====

func (h *StudioHandler) clusterStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	status, err := h.cluster.Status(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// ===== Learning Monitor =====

func (h *StudioHandler) learningStats(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	stats, err := h.learning.Stats(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (h *StudioHandler) learningCapabilities(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	caps, err := h.learning.Capabilities(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if caps == nil {
		caps = []Capability{}
	}
	writeJSON(w, http.StatusOK, caps)
}

func (h *StudioHandler) learningPipelineStats(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	stats, err := h.learning.PipelineStats(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// ===== Marketplace =====

func (h *StudioHandler) marketplaceTemplates(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	category := r.URL.Query().Get("category")
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	templates, err := h.marketplace.SearchTemplates(ctx, query, category)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if templates == nil {
		templates = []AgentTemplate{}
	}
	writeJSON(w, http.StatusOK, templates)
}

func (h *StudioHandler) marketplaceDeploy(w http.ResponseWriter, r *http.Request) {
	var req DeployRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求体解析失败: " + err.Error()})
		return
	}
	if req.TemplateID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "template_id 不能为空"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := h.marketplace.Deploy(ctx, req.TemplateID); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deployed", "template_id": req.TemplateID})
}

// ===== 工具函数 =====

// writeJSON 输出 JSON 响应。
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

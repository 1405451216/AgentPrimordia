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
	h.mux.HandleFunc("POST /api/v1/chaos/experiments/abort", h.abortExperiment)
	// Cluster Dashboard
	h.mux.HandleFunc("GET /api/v1/cluster/status", h.clusterStatus)
	// Learning Monitor
	h.mux.HandleFunc("GET /api/v1/learning/stats", h.learningStats)
	h.mux.HandleFunc("GET /api/v1/learning/capabilities", h.learningCapabilities)
	h.mux.HandleFunc("GET /api/v1/learning/capability-history", h.learningCapabilityHistory)
	h.mux.HandleFunc("GET /api/v1/learning/pipeline/stats", h.learningPipelineStats)
	// Marketplace
	h.mux.HandleFunc("GET /api/v1/marketplace/templates", h.marketplaceTemplates)
	h.mux.HandleFunc("POST /api/v1/marketplace/deploy", h.marketplaceDeploy)
	h.mux.HandleFunc("GET /api/v1/marketplace/deployments", h.marketplaceDeployments)
	h.mux.HandleFunc("POST /api/v1/marketplace/deployments/{id}/stop", h.marketplaceStopDeployment)
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

// AbortRequest POST /api/v1/chaos/experiments/abort 请求体。
type AbortRequest struct {
	Name string `json:"name"`
}

func (h *StudioHandler) abortExperiment(w http.ResponseWriter, r *http.Request) {
	var req AbortRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求体解析失败: " + err.Error()})
		return
	}
	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "实验名称不能为空"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := h.chaos.AbortExperiment(ctx, req.Name); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "aborted", "name": req.Name})
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

func (h *StudioHandler) learningCapabilityHistory(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	history, err := h.learning.CapabilityHistory(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if history == nil {
		history = []CapabilityHistory{}
	}
	writeJSON(w, http.StatusOK, history)
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
	dep, err := h.marketplace.Deploy(ctx, req.TemplateID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, dep)
}

func (h *StudioHandler) marketplaceDeployments(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	deps, err := h.marketplace.ListDeployments(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if deps == nil {
		deps = []Deployment{}
	}
	writeJSON(w, http.StatusOK, deps)
}

func (h *StudioHandler) marketplaceStopDeployment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := h.marketplace.StopDeployment(ctx, id); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped", "id": id})
}

// ===== 工具函数 =====

// writeJSON 输出 JSON 响应。
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

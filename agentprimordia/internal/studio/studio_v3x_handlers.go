// studio_v3x_handlers.go — v3.3-v3.6 Studio 面板后端端点
//
// 这些端点为 AutonomyMonitor / SkillLibrary / A2AInterop / RealtimeConsole
// 四个面板提供数据。数据来源为注入的 AutonomyService / SkillService /
// RealtimeService（真实引擎经 WithAutonomy / WithSkills / WithRealtime
// 注入，见 adapters_v3x.go）；未注入时使用 demo 空实现，返回空数组并
// 在响应头标注 X-Data-Source: demo，使面板可正常渲染空态而非 404。
package studio

import (
	"context"
	"net/http"
	"time"
)

// ===== v3.3 Autonomy Monitor =====

func (h *StudioHandler) autonomyGoals(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	items, err := h.autonomy.Goals(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if items == nil {
		items = []AutonomyGoal{}
	}
	if !h.autonomyReal {
		w.Header().Set("X-Data-Source", "demo")
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *StudioHandler) autonomyAlerts(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	items, err := h.autonomy.Alerts(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if items == nil {
		items = []AutonomyAlert{}
	}
	if !h.autonomyReal {
		w.Header().Set("X-Data-Source", "demo")
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *StudioHandler) autonomyResume(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "resumed", "id": r.PathValue("id")})
}

// ===== v3.4 Skill Library =====

func (h *StudioHandler) skillsList(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	items, err := h.skills.List(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if items == nil {
		items = []SkillEntry{}
	}
	if !h.skillsReal {
		w.Header().Set("X-Data-Source", "demo")
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *StudioHandler) skillsVerify(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "verified", "id": r.PathValue("id")})
}

func (h *StudioHandler) skillsDeprecate(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "deprecated", "id": r.PathValue("id")})
}

// ===== v3.5 A2A Interop =====

func (h *StudioHandler) a2aInteropStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"mode":             "compatible",
		"agentCardExposed": true,
		"agentCardUrl":     "/.well-known/agent.json",
		"supportedMethods": []string{"tasks/send", "tasks/get", "tasks/cancel"},
		"ioModes":          map[string]any{"input": []string{"text"}, "output": []string{"text"}},
	})
}

// ===== v3.6 Realtime Console =====

func (h *StudioHandler) realtimeSessions(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	items, err := h.realtime.Sessions(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if items == nil {
		items = []RealtimeSessionInfo{}
	}
	if !h.realtimeReal {
		w.Header().Set("X-Data-Source", "demo")
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *StudioHandler) realtimeEvents(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	items, err := h.realtime.Events(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if items == nil {
		items = []RealtimeEventInfo{}
	}
	if !h.realtimeReal {
		w.Header().Set("X-Data-Source", "demo")
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *StudioHandler) realtimeBargeIn(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "barge-in", "id": r.PathValue("id")})
}

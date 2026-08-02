// studio_v3x_handlers.go — v3.3-v3.6 Studio 面板后端端点
//
// 这些端点为 AutonomyMonitor / SkillLibrary / A2AInterop / RealtimeConsole
// 四个面板提供数据。当前返回空集合 / 静态配置（与现有面板的 demo 数据同模式），
// 使面板可正常渲染空态而非 404。接入真实运行时数据需将对应实例注入 StudioHandler。
package studio

import "net/http"

// ===== v3.3 Autonomy Monitor =====

func (h *StudioHandler) autonomyGoals(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []any{})
}

func (h *StudioHandler) autonomyAlerts(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []any{})
}

func (h *StudioHandler) autonomyResume(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "resumed", "id": r.PathValue("id")})
}

// ===== v3.4 Skill Library =====

func (h *StudioHandler) skillsList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []any{})
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
	writeJSON(w, http.StatusOK, []any{})
}

func (h *StudioHandler) realtimeEvents(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []any{})
}

func (h *StudioHandler) realtimeBargeIn(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "barge-in", "id": r.PathValue("id")})
}

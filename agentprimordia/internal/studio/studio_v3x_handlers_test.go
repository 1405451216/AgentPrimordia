package studio

import (
	"net/http"
	"testing"
)

// v3.3-v3.6 Studio 面板后端端点测试：验证路由已注册且返回 200（非 404 孤岛）。

func TestStudioV3X_AutonomyEndpoints(t *testing.T) {
	h := NewStudioHandler()
	for _, p := range []string{"/api/v1/autonomy/goals", "/api/v1/autonomy/alerts"} {
		if rec := doJSON(t, h, "GET", p, ""); rec.Code != http.StatusOK {
			t.Errorf("%s status=%d, want 200", p, rec.Code)
		}
	}
	if rec := doJSON(t, h, "POST", "/api/v1/autonomy/goals/g1/resume", ""); rec.Code != http.StatusOK {
		t.Errorf("resume status=%d, want 200", rec.Code)
	}
}

func TestStudioV3X_SkillsEndpoints(t *testing.T) {
	h := NewStudioHandler()
	if rec := doJSON(t, h, "GET", "/api/v1/skills", ""); rec.Code != http.StatusOK {
		t.Errorf("skills list status=%d", rec.Code)
	}
	if rec := doJSON(t, h, "POST", "/api/v1/skills/s1/verify", ""); rec.Code != http.StatusOK {
		t.Errorf("verify status=%d", rec.Code)
	}
	if rec := doJSON(t, h, "POST", "/api/v1/skills/s1/deprecate", ""); rec.Code != http.StatusOK {
		t.Errorf("deprecate status=%d", rec.Code)
	}
}

func TestStudioV3X_A2AInteropStatus(t *testing.T) {
	h := NewStudioHandler()
	rec := doJSON(t, h, "GET", "/api/v1/a2a/interop/status", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	status := decode[map[string]any](t, rec)
	if status["mode"] != "compatible" {
		t.Errorf("mode=%v, want compatible", status["mode"])
	}
	if status["agentCardExposed"] != true {
		t.Errorf("agentCardExposed=%v", status["agentCardExposed"])
	}
}

func TestStudioV3X_RealtimeEndpoints(t *testing.T) {
	h := NewStudioHandler()
	for _, p := range []string{"/api/v1/realtime/sessions", "/api/v1/realtime/events"} {
		if rec := doJSON(t, h, "GET", p, ""); rec.Code != http.StatusOK {
			t.Errorf("%s status=%d", p, rec.Code)
		}
	}
	if rec := doJSON(t, h, "POST", "/api/v1/realtime/sessions/s1/barge-in", ""); rec.Code != http.StatusOK {
		t.Errorf("barge-in status=%d", rec.Code)
	}
}

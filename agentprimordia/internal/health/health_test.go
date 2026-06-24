package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mockChecker 模拟健康检查器
type mockChecker struct {
	healthy bool
	name    string
}

func (m *mockChecker) Name() string { return m.name }
func (m *mockChecker) Check(ctx context.Context) error {
	if !m.healthy {
		return context.DeadlineExceeded
	}
	return nil
}

func TestHealthz_AllHealthy(t *testing.T) {
	h := NewChecker()
	h.Register(&mockChecker{healthy: true, name: "db"})
	h.Register(&mockChecker{healthy: true, name: "llm"})

	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("状态码 = %d, 期望 %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"status":"ok"`) {
		t.Errorf("响应体 = %q, 期望包含 ok", body)
	}
}

func TestHealthz_OneUnhealthy(t *testing.T) {
	h := NewChecker()
	h.Register(&mockChecker{healthy: true, name: "db"})
	h.Register(&mockChecker{healthy: false, name: "llm"})

	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("状态码 = %d, 期望 %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestReadyz_BeforeReady(t *testing.T) {
	h := NewChecker()
	// 未调用 SetReady()

	req := httptest.NewRequest("GET", "/readyz", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("状态码 = %d, 期望 %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestReadyz_AfterReady(t *testing.T) {
	h := NewChecker()
	h.SetReady()

	req := httptest.NewRequest("GET", "/readyz", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("状态码 = %d, 期望 %d", w.Code, http.StatusOK)
	}
}

// TestHealthz_UnknownPath 验证未知路径返回 404
func TestHealthz_UnknownPath(t *testing.T) {
	h := NewChecker()

	req := httptest.NewRequest("GET", "/unknown", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("未知路径状态码 = %d, 期望 %d", w.Code, http.StatusNotFound)
	}
}

// TestHealthz_NoCheckers 验证无 checker 注册时 healthz 仍返回 200
func TestHealthz_NoCheckers(t *testing.T) {
	h := NewChecker()

	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("无 checker 时状态码 = %d, 期望 %d", w.Code, http.StatusOK)
	}
}

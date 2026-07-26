package health

import (
	"context"
	"errors"
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

// ===== pprof Bearer Token 鉴权测试 =====

func TestPProfAuth_NoTokenSet_AllowsAll(t *testing.T) {
	// 确保 PPROF_TOKEN 未设置
	t.Setenv("PPROF_TOKEN", "")

	mux := http.NewServeMux()
	RegisterPProfSecure(mux)

	req := httptest.NewRequest("GET", "/debug/pprof/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("未设置 PPROF_TOKEN 时期望状态码 %d, 得到 %d", http.StatusOK, w.Code)
	}
}

func TestPProfAuth_WithToken_MissingAuth(t *testing.T) {
	t.Setenv("PPROF_TOKEN", "secret-token-123")

	mux := http.NewServeMux()
	RegisterPProfSecure(mux)

	req := httptest.NewRequest("GET", "/debug/pprof/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("缺少 Authorization 头时期望状态码 %d, 得到 %d", http.StatusUnauthorized, w.Code)
	}
}

func TestPProfAuth_WithToken_InvalidFormat(t *testing.T) {
	t.Setenv("PPROF_TOKEN", "secret-token-123")

	mux := http.NewServeMux()
	RegisterPProfSecure(mux)

	req := httptest.NewRequest("GET", "/debug/pprof/", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz") // 非 Bearer
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("非 Bearer 格式时期望状态码 %d, 得到 %d", http.StatusUnauthorized, w.Code)
	}
}

func TestPProfAuth_WithToken_WrongToken(t *testing.T) {
	t.Setenv("PPROF_TOKEN", "secret-token-123")

	mux := http.NewServeMux()
	RegisterPProfSecure(mux)

	req := httptest.NewRequest("GET", "/debug/pprof/", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("错误 token 时期望状态码 %d, 得到 %d", http.StatusForbidden, w.Code)
	}
}

func TestPProfAuth_WithToken_CorrectToken(t *testing.T) {
	t.Setenv("PPROF_TOKEN", "secret-token-123")

	mux := http.NewServeMux()
	RegisterPProfSecure(mux)

	req := httptest.NewRequest("GET", "/debug/pprof/", nil)
	req.Header.Set("Authorization", "Bearer secret-token-123")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("正确 token 时期望状态码 %d, 得到 %d", http.StatusOK, w.Code)
	}
}

func TestPProfHandlerSecure_WithToken(t *testing.T) {
	t.Setenv("PPROF_TOKEN", "secure-test-token")

	handler := PProfHandlerSecure()

	// 正确 token
	req := httptest.NewRequest("GET", "/debug/pprof/", nil)
	req.Header.Set("Authorization", "Bearer secure-test-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("PProfHandlerSecure 正确 token 时期望状态码 %d, 得到 %d", http.StatusOK, w.Code)
	}
}

// ===== pprof 生产强制鉴权测试（RegisterPProfStrict） =====

func TestPProfStrict_NoToken_ReturnsError(t *testing.T) {
	// 确保 PPROF_TOKEN 未设置
	t.Setenv("PPROF_TOKEN", "")

	mux := http.NewServeMux()
	err := RegisterPProfStrict(mux)
	if err == nil {
		t.Fatal("期望 PPROF_TOKEN 未设置时返回错误，得到 nil")
	}
	if !errors.Is(err, ErrPProfTokenRequired) {
		t.Errorf("期望 ErrPProfTokenRequired，得到: %v", err)
	}
}

func TestPProfStrict_WithToken_Success(t *testing.T) {
	t.Setenv("PPROF_TOKEN", "prod-secret-token")

	mux := http.NewServeMux()
	err := RegisterPProfStrict(mux)
	if err != nil {
		t.Fatalf("PPROF_TOKEN 已设置时不应返回错误，得到: %v", err)
	}

	// 验证鉴权生效：无 token 请求应被拒绝
	req := httptest.NewRequest("GET", "/debug/pprof/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("无 Authorization 时期望 %d, 得到 %d", http.StatusUnauthorized, w.Code)
	}

	// 正确 token 应放行
	req2 := httptest.NewRequest("GET", "/debug/pprof/", nil)
	req2.Header.Set("Authorization", "Bearer prod-secret-token")
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("正确 token 时期望 %d, 得到 %d", http.StatusOK, w2.Code)
	}
}

func TestPProfHandlerStrict_NoToken_ReturnsError(t *testing.T) {
	t.Setenv("PPROF_TOKEN", "")

	_, err := PProfHandlerStrict()
	if err == nil {
		t.Fatal("期望 PPROF_TOKEN 未设置时返回错误")
	}
	if !errors.Is(err, ErrPProfTokenRequired) {
		t.Errorf("期望 ErrPProfTokenRequired，得到: %v", err)
	}
}

func TestPProfHandlerStrict_WithToken_Success(t *testing.T) {
	t.Setenv("PPROF_TOKEN", "handler-strict-token")

	handler, err := PProfHandlerStrict()
	if err != nil {
		t.Fatalf("不应返回错误: %v", err)
	}

	req := httptest.NewRequest("GET", "/debug/pprof/", nil)
	req.Header.Set("Authorization", "Bearer handler-strict-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("期望 %d, 得到 %d", http.StatusOK, w.Code)
	}
}

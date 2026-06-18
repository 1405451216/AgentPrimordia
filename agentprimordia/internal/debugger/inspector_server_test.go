package debugger

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInspectorServer_HandleInspectorUI(t *testing.T) {
	inspector := NewInspector(100)
	server := NewInspectorServer(inspector)

	req := httptest.NewRequest("GET", "/inspector", nil)
	w := httptest.NewRecorder()

	server.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if w.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Errorf("expected Content-Type 'text/html; charset=utf-8', got '%s'", w.Header().Get("Content-Type"))
	}

	body := w.Body.String()
	if len(body) == 0 {
		t.Error("expected non-empty HTML body")
	}

	// 验证HTML包含关键元素
	if !contains(body, "AP Inspector") {
		t.Error("expected HTML to contain 'AP Inspector'")
	}

	if !contains(body, "Agent Trace Viewer") {
		t.Error("expected HTML to contain 'Agent Trace Viewer'")
	}
}

func TestInspectorServer_HandleGetTraces(t *testing.T) {
	inspector := NewInspector(100)
	server := NewInspectorServer(inspector)

	req := httptest.NewRequest("GET", "/api/inspector/traces", nil)
	w := httptest.NewRecorder()

	server.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got '%s'", w.Header().Get("Content-Type"))
	}

	var traces []*TraceSpan
	if err := json.Unmarshal(w.Body.Bytes(), &traces); err != nil {
		t.Errorf("failed to unmarshal response: %v", err)
	}

	if len(traces) != 0 {
		t.Errorf("expected 0 traces initially, got %d", len(traces))
	}
}

func TestInspectorServer_HandleGetSessions(t *testing.T) {
	inspector := NewInspector(100)
	server := NewInspectorServer(inspector)

	req := httptest.NewRequest("GET", "/api/inspector/sessions", nil)
	w := httptest.NewRecorder()

	server.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var sessions []*SessionTrace
	if err := json.Unmarshal(w.Body.Bytes(), &sessions); err != nil {
		t.Errorf("failed to unmarshal response: %v", err)
	}

	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions initially, got %d", len(sessions))
	}
}

func TestInspectorServer_HandleGetStats(t *testing.T) {
	inspector := NewInspector(100)
	server := NewInspectorServer(inspector)

	req := httptest.NewRequest("GET", "/api/inspector/stats", nil)
	w := httptest.NewRecorder()

	server.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var stats InspectorStats
	if err := json.Unmarshal(w.Body.Bytes(), &stats); err != nil {
		t.Errorf("failed to unmarshal response: %v", err)
	}

	if stats.TotalSpans != 0 {
		t.Errorf("expected 0 total spans, got %d", stats.TotalSpans)
	}

	if stats.TotalSessions != 0 {
		t.Errorf("expected 0 total sessions, got %d", stats.TotalSessions)
	}
}

func TestInspectorServer_HandleGetSessionTrace_NotFound(t *testing.T) {
	inspector := NewInspector(100)
	server := NewInspectorServer(inspector)

	req := httptest.NewRequest("GET", "/api/inspector/session/nonexistent", nil)
	w := httptest.NewRecorder()

	server.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestInspectorServer_HandleGetSessionTrace_EmptyID(t *testing.T) {
	inspector := NewInspector(100)
	server := NewInspectorServer(inspector)

	req := httptest.NewRequest("GET", "/api/inspector/session/", nil)
	w := httptest.NewRecorder()

	server.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestInspectorServer_Integration(t *testing.T) {
	inspector := NewInspector(100)
	server := NewInspectorServer(inspector)

	// 模拟创建一些追踪数据
	// 注意：这里需要context，但为了简化测试，我们直接验证API端点

	// 验证stats端点
	req := httptest.NewRequest("GET", "/api/inspector/stats", nil)
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// 验证traces端点
	req = httptest.NewRequest("GET", "/api/inspector/traces", nil)
	w = httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// 验证sessions端点
	req = httptest.NewRequest("GET", "/api/inspector/sessions", nil)
	w = httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

// contains 辅助函数，检查字符串是否包含子串
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

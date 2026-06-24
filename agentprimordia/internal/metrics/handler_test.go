package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMetricsHandler_ContentType(t *testing.T) {
	m := NewMetrics()
	h := NewHandler(m)

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Errorf("Content-Type = %q, 期望包含 text/plain", ct)
	}
}

func TestMetricsHandler_ContainsCounters(t *testing.T) {
	m := NewMetrics()
	m.RecordLLMCall(100*time.Millisecond, nil)
	m.RecordToolCall(50*time.Millisecond, nil)

	h := NewHandler(m)
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "ap_llm_total_calls 1") {
		t.Error("缺少 ap_llm_total_calls 计数器")
	}
	if !strings.Contains(body, "ap_tool_total_calls 1") {
		t.Error("缺少 ap_tool_total_calls 计数器")
	}
}

func TestMetricsHandler_HistogramBuckets(t *testing.T) {
	m := NewMetrics()
	m.RecordLLMCall(150*time.Millisecond, nil)

	h := NewHandler(m)
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "ap_llm_latency_ms_bucket") {
		t.Error("缺少 histogram bucket 数据")
	}
}

func TestMetricsHandler_MethodNotAllowed(t *testing.T) {
	m := NewMetrics()
	h := NewHandler(m)

	req := httptest.NewRequest("POST", "/metrics", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST 状态码 = %d, 期望 %d", w.Code, http.StatusMethodNotAllowed)
	}
}

// TestMetricsHandler_EmptyMetrics 验证无记录时 metrics 端点仍返回 200 和有效输出
func TestMetricsHandler_EmptyMetrics(t *testing.T) {
	m := NewMetrics()
	h := NewHandler(m)

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("空 metrics 状态码 = %d, 期望 %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()
	// 验证计数器为 0
	if !strings.Contains(body, "ap_llm_total_calls 0") {
		t.Error("空 metrics 缺少 ap_llm_total_calls 0")
	}
	if !strings.Contains(body, "ap_tool_total_calls 0") {
		t.Error("空 metrics 缺少 ap_tool_total_calls 0")
	}
}

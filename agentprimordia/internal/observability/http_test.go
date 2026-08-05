package observability

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestHandler_ListTraces 验证 /traces 列表 API。
func TestHandler_ListTraces(t *testing.T) {
	s := NewCorrelationStore()
	s.Start("t1", "agent-a", "sess-1")
	s.Start("t2", "agent-b", "sess-2")
	s.End("t1")
	s.End("t2")

	h := Handler(s)
	req := httptest.NewRequest(http.MethodGet, "/traces", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		Total  int             `json:"total"`
		Traces []*RequestTrace `json:"traces"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if body.Total != 2 || len(body.Traces) != 2 {
		t.Errorf("total/traces = %d/%d, want 2/2", body.Total, len(body.Traces))
	}
}

// TestHandler_GetTrace 验证 /traces/{id} 返回单请求全链路视图。
func TestHandler_GetTrace(t *testing.T) {
	s := NewCorrelationStore()
	s.Start("trace-xyz", "agent-c", "sess-3")
	s.AddSpan("trace-xyz", SpanRecord{Name: "agent.run", SpanID: "s1"})
	s.AddAuditEvent("trace-xyz", AuditEvent{Action: "agent.start", Result: "success"})
	s.RecordLLM("trace-xyz", time.Millisecond, 10, 20, 0.001)
	s.End("trace-xyz")

	h := Handler(s)
	req := httptest.NewRequest(http.MethodGet, "/traces/trace-xyz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var rt RequestTrace
	if err := json.Unmarshal(rec.Body.Bytes(), &rt); err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if rt.TraceID != "trace-xyz" {
		t.Errorf("TraceID = %q", rt.TraceID)
	}
	if len(rt.Spans) != 1 || len(rt.AuditEvents) != 1 {
		t.Errorf("spans/audit = %d/%d, want 1/1", len(rt.Spans), len(rt.AuditEvents))
	}
	if rt.Metrics.LLMCalls != 1 {
		t.Errorf("LLMCalls = %d, want 1", rt.Metrics.LLMCalls)
	}
}

// TestHandler_GetTraceNotFound 验证不存在的 trace 返回 404。
func TestHandler_GetTraceNotFound(t *testing.T) {
	s := NewCorrelationStore()
	h := Handler(s)
	req := httptest.NewRequest(http.MethodGet, "/traces/nope", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

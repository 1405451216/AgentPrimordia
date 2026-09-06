package observability

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestDashboardHandler_Summary 验证 /dashboard/summary 返回正确的聚合数据。
func TestDashboardHandler_Summary(t *testing.T) {
	store := NewCorrelationStore()
	// 创建 3 个请求，不同 agent
	store.Start("t1", "agent-a", "sess-1")
	store.AddSpan("t1", SpanRecord{Name: "agent.run", SpanID: "s1"})
	store.AddSpan("t1", SpanRecord{Name: "llm.call", SpanID: "s2"})
	store.AddAuditEvent("t1", AuditEvent{Action: "agent.start", Result: "success"})
	store.RecordLLM("t1", 100*time.Millisecond, 50, 100, 0.01)
	store.End("t1")

	store.Start("t2", "agent-b", "sess-2")
	store.AddSpan("t2", SpanRecord{Name: "agent.run", SpanID: "s3"})
	store.AddAuditEvent("t2", AuditEvent{Action: "agent.start", Result: "success"})
	store.AddAuditEvent("t2", AuditEvent{Action: "tool.call", Result: "success"})
	store.End("t2")

	store.Start("t3", "agent-a", "sess-3")
	store.End("t3")

	engine := NewAlertEngine(store)
	h := DashboardHandler(store, engine)

	req := httptest.NewRequest(http.MethodGet, "/dashboard/summary", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var body struct {
		TotalTraces int `json:"total_traces"`
		TotalSpans  int `json:"total_spans"`
		TotalAudits int `json:"total_audits"`
		Agents      int `json:"agents"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	if body.TotalTraces != 3 {
		t.Errorf("total_traces = %d, want 3", body.TotalTraces)
	}
	if body.TotalSpans != 3 {
		t.Errorf("total_spans = %d, want 3", body.TotalSpans)
	}
	if body.TotalAudits != 3 {
		t.Errorf("total_audits = %d, want 3", body.TotalAudits)
	}
	if body.Agents != 2 {
		t.Errorf("agents = %d, want 2 (agent-a, agent-b)", body.Agents)
	}
}

// TestDashboardHandler_Summary_Empty 验证空存储的摘要返回。
func TestDashboardHandler_Summary_Empty(t *testing.T) {
	store := NewCorrelationStore()
	engine := NewAlertEngine(store)
	h := DashboardHandler(store, engine)

	req := httptest.NewRequest(http.MethodGet, "/dashboard/summary", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var body struct {
		TotalTraces int `json:"total_traces"`
		TotalSpans  int `json:"total_spans"`
		TotalAudits int `json:"total_audits"`
		Agents      int `json:"agents"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if body.TotalTraces != 0 || body.TotalSpans != 0 || body.TotalAudits != 0 || body.Agents != 0 {
		t.Errorf("空存储摘要应为全零, got: %+v", body)
	}
}

// TestDashboardHandler_Alerts 验证 /dashboard/alerts 返回当前告警事件。
func TestDashboardHandler_Alerts(t *testing.T) {
	store := NewCorrelationStore()
	// 创建 10 个请求使阈值规则触发
	for i := 0; i < 10; i++ {
		traceID := "trace-d-" + string(rune('a'+i))
		store.Start(traceID, "agent-a", "sess-1")
		store.End(traceID)
	}

	engine := NewAlertEngine(store)
	engine.RegisterRule(NewThresholdAlertRule(ThresholdAlertConfig{
		Name:      "high_request_count",
		Threshold: 5,
		Severity:  SeverityWarning,
		MetricFn: func(store *CorrelationStore) float64 {
			return float64(store.Len())
		},
	}))

	h := DashboardHandler(store, engine)

	req := httptest.NewRequest(http.MethodGet, "/dashboard/alerts", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var body struct {
		Alerts []AlertEvent `json:"alerts"`
		Count  int          `json:"count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	if body.Count != 1 {
		t.Errorf("告警数 = %d, want 1", body.Count)
	}
	if len(body.Alerts) != 1 {
		t.Fatalf("告警列表长度 = %d, want 1", len(body.Alerts))
	}
	if body.Alerts[0].Rule != "high_request_count" {
		t.Errorf("告警规则名 = %q, want high_request_count", body.Alerts[0].Rule)
	}
}

// TestDashboardHandler_Alerts_Empty 验证无告警时返回空列表。
func TestDashboardHandler_Alerts_Empty(t *testing.T) {
	store := NewCorrelationStore()
	engine := NewAlertEngine(store)
	h := DashboardHandler(store, engine)

	req := httptest.NewRequest(http.MethodGet, "/dashboard/alerts", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var body struct {
		Alerts []AlertEvent `json:"alerts"`
		Count  int          `json:"count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if body.Count != 0 {
		t.Errorf("告警数 = %d, want 0", body.Count)
	}
}

// TestDashboardHandler_MethodNotAllowed 验证非 GET 方法返回 405。
func TestDashboardHandler_MethodNotAllowed(t *testing.T) {
	store := NewCorrelationStore()
	engine := NewAlertEngine(store)
	h := DashboardHandler(store, engine)

	for _, path := range []string{"/dashboard/summary", "/dashboard/alerts"} {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s POST status = %d, want 405", path, rec.Code)
		}
	}
}

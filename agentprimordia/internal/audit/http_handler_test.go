package audit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"
)

// TestHTTPHandler_Health 测试健康检查接口
func TestHTTPHandler_Health(t *testing.T) {
	logger, err := NewLogger(LoggerConfig{Output: &memoryOutput{}})
	if err != nil {
		t.Fatalf("NewLogger error: %v", err)
	}
	handler := NewHTTPHandler(logger)

	req := httptest.NewRequest(http.MethodGet, "/audit/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("status = %v, want ok", resp["status"])
	}
}

// TestHTTPHandler_Query 测试事件查询接口
func TestHTTPHandler_Query(t *testing.T) {
	output := &memoryOutput{}
	logger, _ := NewLogger(LoggerConfig{Output: output})
	handler := NewHTTPHandler(logger)

	// 写入测试数据
	_ = logger.Log(context.TODO(), Event{
		Actor:    "agent-1",
		Action:   "llm.call",
		Resource: "gpt-4",
		Result:   "success",
	})
	_ = logger.Log(context.TODO(), Event{
		Actor:    "agent-2",
		Action:   "tool.call",
		Resource: "search",
		Result:   "success",
	})

	// 按 actor 查询
	req := httptest.NewRequest(http.MethodGet, "/audit/events?actor=agent-1", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Count  int     `json:"count"`
		Events []Event `json:"events"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.Count != 1 {
		t.Errorf("count = %d, want 1", resp.Count)
	}
	if len(resp.Events) > 0 && resp.Events[0].Actor != "agent-1" {
		t.Errorf("actor = %q, want agent-1", resp.Events[0].Actor)
	}
}

// TestHTTPHandler_Query_ByAction 测试按 action 查询
func TestHTTPHandler_Query_ByAction(t *testing.T) {
	output := &memoryOutput{}
	logger, _ := NewLogger(LoggerConfig{Output: output})
	handler := NewHTTPHandler(logger)

	_ = logger.Log(context.TODO(), Event{Actor: "a1", Action: "llm.call", Result: "success"})
	_ = logger.Log(context.TODO(), Event{Actor: "a1", Action: "tool.call", Result: "success"})
	_ = logger.Log(context.TODO(), Event{Actor: "a1", Action: "llm.call", Result: "success"})

	req := httptest.NewRequest(http.MethodGet, "/audit/events?action=llm.call", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var resp struct {
		Count int `json:"count"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Count != 2 {
		t.Errorf("count = %d, want 2", resp.Count)
	}
}

// TestHTTPHandler_Query_ByTimeRange 测试按时间范围查询
func TestHTTPHandler_Query_ByTimeRange(t *testing.T) {
	output := &memoryOutput{}
	logger, _ := NewLogger(LoggerConfig{Output: output})
	handler := NewHTTPHandler(logger)

	now := time.Now()
	_ = logger.Log(context.TODO(), Event{Actor: "a1", Action: "x", Timestamp: now.Add(-2 * time.Hour), Result: "ok"})
	_ = logger.Log(context.TODO(), Event{Actor: "a1", Action: "x", Timestamp: now.Add(-1 * time.Hour), Result: "ok"})
	_ = logger.Log(context.TODO(), Event{Actor: "a1", Action: "x", Timestamp: now, Result: "ok"})

	start := now.Add(-90 * time.Minute).Format(time.RFC3339)
	end := now.Add(1 * time.Minute).Format(time.RFC3339)

	q := url.Values{}
	q.Set("start", start)
	q.Set("end", end)
	req := httptest.NewRequest(http.MethodGet, "/audit/events?"+q.Encode(), nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var resp struct {
		Count int `json:"count"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Count != 2 {
		t.Errorf("count = %d, want 2", resp.Count)
	}
}

// TestHTTPHandler_Query_Limit 测试 limit 参数
func TestHTTPHandler_Query_Limit(t *testing.T) {
	output := &memoryOutput{}
	logger, _ := NewLogger(LoggerConfig{Output: output})
	handler := NewHTTPHandler(logger)

	for i := 0; i < 10; i++ {
		_ = logger.Log(context.TODO(), Event{Actor: "a1", Action: "x", Result: "ok"})
	}

	req := httptest.NewRequest(http.MethodGet, "/audit/events?limit=3", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var resp struct {
		Count int `json:"count"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Count != 3 {
		t.Errorf("count = %d, want 3", resp.Count)
	}
}

// TestHTTPHandler_Report 测试报告生成接口
func TestHTTPHandler_Report(t *testing.T) {
	output := &memoryOutput{}
	logger, _ := NewLogger(LoggerConfig{Output: output})
	handler := NewHTTPHandler(logger)

	_ = logger.Log(context.TODO(), Event{Actor: "a1", Action: "llm.call", Result: "success"})
	_ = logger.Log(context.TODO(), Event{Actor: "a1", Action: "tool.call", Result: "success"})
	_ = logger.Log(context.TODO(), Event{Actor: "a2", Action: "llm.call", Result: "success"})

	now := time.Now()
	start := now.Add(-1 * time.Hour).Format(time.RFC3339)
	end := now.Add(1 * time.Hour).Format(time.RFC3339)

	q := url.Values{}
	q.Set("start", start)
	q.Set("end", end)
	req := httptest.NewRequest(http.MethodGet, "/audit/report?"+q.Encode(), nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var report ComplianceReport
	if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if report.TotalEvents != 3 {
		t.Errorf("TotalEvents = %d, want 3", report.TotalEvents)
	}
	if len(report.ActorStats) != 2 {
		t.Errorf("ActorStats 数 = %d, want 2", len(report.ActorStats))
	}
}

// TestHTTPHandler_Report_MissingParams 测试缺少参数的错误处理
func TestHTTPHandler_Report_MissingParams(t *testing.T) {
	output := &memoryOutput{}
	logger, _ := NewLogger(LoggerConfig{Output: output})
	handler := NewHTTPHandler(logger)

	req := httptest.NewRequest(http.MethodGet, "/audit/report", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// TestHTTPHandler_Query_BadLimit 测试非法 limit 参数
func TestHTTPHandler_Query_BadLimit(t *testing.T) {
	output := &memoryOutput{}
	logger, _ := NewLogger(LoggerConfig{Output: output})
	handler := NewHTTPHandler(logger)

	req := httptest.NewRequest(http.MethodGet, "/audit/events?limit=abc", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// TestHTTPHandler_NotFound 测试未知路由
func TestHTTPHandler_NotFound(t *testing.T) {
	output := &memoryOutput{}
	logger, _ := NewLogger(LoggerConfig{Output: output})
	handler := NewHTTPHandler(logger)

	req := httptest.NewRequest(http.MethodGet, "/audit/unknown", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// TestHTTPHandler_MethodNotAllowed 测试非法方法
func TestHTTPHandler_MethodNotAllowed(t *testing.T) {
	output := &memoryOutput{}
	logger, _ := NewLogger(LoggerConfig{Output: output})
	handler := NewHTTPHandler(logger)

	req := httptest.NewRequest(http.MethodPost, "/audit/events", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

// TestHTTPHandler_Query_UnixTimestamp 测试 Unix 时间戳格式
func TestHTTPHandler_Query_UnixTimestamp(t *testing.T) {
	output := &memoryOutput{}
	logger, _ := NewLogger(LoggerConfig{Output: output})
	handler := NewHTTPHandler(logger)

	now := time.Now()
	_ = logger.Log(context.TODO(), Event{Actor: "a1", Action: "x", Timestamp: now, Result: "ok"})

	// Unix 时间戳（秒）
	startTs := strconv.FormatInt(now.Add(-1*time.Hour).Unix(), 10)
	endTs := strconv.FormatInt(now.Add(1*time.Hour).Unix(), 10)

	req := httptest.NewRequest(http.MethodGet, "/audit/events?start="+startTs+"&end="+endTs, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

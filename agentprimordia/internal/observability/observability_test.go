package observability

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestContextTraceID 验证 context 关联键传播。
func TestContextTraceID(t *testing.T) {
	ctx := context.Background()
	if got := TraceIDFromContext(ctx); got != "" {
		t.Errorf("空 context 应返回空串, got %q", got)
	}
	ctx = WithTraceID(ctx, "trace-abc")
	if got := TraceIDFromContext(ctx); got != "trace-abc" {
		t.Errorf("TraceIDFromContext = %q, want trace-abc", got)
	}
}

// TestCorrelationStore_StartEnd 验证登记与结束。
func TestCorrelationStore_StartEnd(t *testing.T) {
	s := NewCorrelationStore()
	s.Start("t1", "agent-a", "sess-1")
	if s.Len() != 1 {
		t.Errorf("Len = %d, want 1", s.Len())
	}
	rt := s.Get("t1")
	if rt == nil {
		t.Fatal("Get(t1) 应为非空")
	}
	if rt.AgentName != "agent-a" || rt.SessionID != "sess-1" {
		t.Errorf("agent/session = %q/%q", rt.AgentName, rt.SessionID)
	}
	s.End("t1")
	rt = s.Get("t1")
	if rt.EndedAt.IsZero() {
		t.Error("End 后 EndedAt 不应为零值")
	}
	if rt.DurationMs < 0 {
		t.Errorf("DurationMs = %d, 不应为负", rt.DurationMs)
	}
}

// TestCorrelationStore_AddSpan 验证 Span 记录。
func TestCorrelationStore_AddSpan(t *testing.T) {
	s := NewCorrelationStore()
	s.Start("t2", "agent-b", "")
	s.AddSpan("t2", SpanRecord{Name: "agent.run", Kind: "server", SpanID: "s1"})
	s.AddSpan("t2", SpanRecord{Name: "llm.call", Kind: "client", SpanID: "s2", ParentSpanID: "s1"})
	s.End("t2")

	rt := s.Get("t2")
	if s.SpanCount("t2") != 2 {
		t.Errorf("SpanCount = %d, want 2", s.SpanCount("t2"))
	}
	if rt.Spans[0].TraceID != "t2" {
		t.Errorf("Span 应携带 traceID = t2, got %q", rt.Spans[0].TraceID)
	}
	if rt.Spans[1].ParentSpanID != "s1" {
		t.Errorf("ParentSpanID = %q, want s1", rt.Spans[1].ParentSpanID)
	}
}

// TestCorrelationStore_AuditAndMetrics 验证审计事件与指标聚合（全链路闭环核心）。
func TestCorrelationStore_AuditAndMetrics(t *testing.T) {
	s := NewCorrelationStore()
	s.Start("t3", "agent-c", "sess-3")

	s.AddAuditEvent("t3", AuditEvent{Actor: "agent-c", Action: "agent.start", Result: "success"})
	s.AddAuditEvent("t3", AuditEvent{Actor: "agent-c", Action: "llm.call", Result: "success"})
	s.AddAuditEvent("t3", AuditEvent{Actor: "agent-c", Action: "agent.stop", Result: "success"})

	s.RecordLLM("t3", 150*time.Millisecond, 100, 200, 0.0012)
	s.RecordLLM("t3", 80*time.Millisecond, 50, 60, 0.0006)
	s.RecordTool("t3", 30*time.Millisecond)
	s.RecordTurn("t3")
	s.RecordTurn("t3")
	s.End("t3")

	rt := s.Get("t3")
	if s.AuditCount("t3") != 3 {
		t.Errorf("AuditCount = %d, want 3", s.AuditCount("t3"))
	}
	// 审计事件必须携带 trace_id（关联键）
	for _, ev := range rt.AuditEvents {
		if ev.TraceID != "t3" {
			t.Errorf("审计事件 TraceID = %q, want t3", ev.TraceID)
		}
	}
	m := rt.Metrics
	if m.LLMCalls != 2 {
		t.Errorf("LLMCalls = %d, want 2", m.LLMCalls)
	}
	if m.ToolCalls != 1 {
		t.Errorf("ToolCalls = %d, want 1", m.ToolCalls)
	}
	if m.Turns != 2 {
		t.Errorf("Turns = %d, want 2", m.Turns)
	}
	if m.PromptTokens != 150 || m.CompletionTokens != 260 || m.TotalTokens != 410 {
		t.Errorf("tokens = %d/%d/%d, want 150/260/410", m.PromptTokens, m.CompletionTokens, m.TotalTokens)
	}
	if m.LLMLatencyMs != 230 {
		t.Errorf("LLMLatencyMs = %d, want 230", m.LLMLatencyMs)
	}
	if m.ToolLatencyMs != 30 {
		t.Errorf("ToolLatencyMs = %d, want 30", m.ToolLatencyMs)
	}
	if m.CostUSD <= 0 {
		t.Errorf("CostUSD = %f, 应为正", m.CostUSD)
	}
}

// TestCorrelationStore_GetSnapshotIsolated 验证 Get 返回深拷贝。
func TestCorrelationStore_GetSnapshotIsolated(t *testing.T) {
	s := NewCorrelationStore()
	s.Start("t4", "agent-d", "")
	s.AddSpan("t4", SpanRecord{Name: "a", SpanID: "s1"})
	snapshot := s.Get("t4")
	// 修改快照不应影响内部存储
	snapshot.Spans[0].Name = "hacked"
	snapshot.AuditEvents = append(snapshot.AuditEvents, AuditEvent{Action: "x"})
	if got := s.Get("t4").Spans[0].Name; got != "a" {
		t.Errorf("快照隔离失败, span name = %q", got)
	}
}

// TestCorrelationStore_List 验证列表与按 Agent 筛选。
func TestCorrelationStore_List(t *testing.T) {
	s := NewCorrelationStore()
	s.Start("t-a", "agent-a", "")
	s.Start("t-b", "agent-b", "")
	s.Start("t-c", "agent-a", "")

	if s.Len() != 3 {
		t.Errorf("Len = %d, want 3", s.Len())
	}
	list := s.List(0)
	if len(list) != 3 {
		t.Errorf("List = %d, want 3", len(list))
	}
	if list[0].TraceID != "t-a" {
		t.Errorf("List 应保持插入顺序, first = %q", list[0].TraceID)
	}
	limited := s.List(2)
	if len(limited) != 2 {
		t.Errorf("List(2) = %d, want 2", len(limited))
	}
	byAgent := s.ListByAgent("agent-a", 0)
	if len(byAgent) != 2 {
		t.Errorf("ListByAgent(agent-a) = %d, want 2", len(byAgent))
	}
}

// TestCorrelationStore_UnknownTrace 验证未知 trace 的幂等性。
func TestCorrelationStore_UnknownTrace(t *testing.T) {
	s := NewCorrelationStore()
	s.Start("known", "a", "")
	s.End("unknown")
	s.AddSpan("unknown", SpanRecord{Name: "x"})
	s.AddAuditEvent("unknown", AuditEvent{Action: "y"})
	s.RecordLLM("unknown", time.Second, 1, 1, 0)
	if s.Get("unknown") != nil {
		t.Error("未知 trace 应返回 nil")
	}
	if s.Len() != 1 {
		t.Errorf("Len = %d, want 1", s.Len())
	}
}

// TestCorrelationStore_Concurrent 验证并发写入安全（-race 覆盖）。
func TestCorrelationStore_Concurrent(t *testing.T) {
	s := NewCorrelationStore()
	s.Start("conc", "agent-x", "sess-x")

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			s.AddSpan("conc", SpanRecord{Name: "span", SpanID: strings.Repeat("x", n)})
			s.AddAuditEvent("conc", AuditEvent{Action: "evt", Result: "ok"})
			s.RecordLLM("conc", time.Millisecond, 1, 1, 0)
			s.RecordTool("conc", time.Millisecond)
		}(i)
	}
	wg.Wait()
	s.End("conc")

	rt := s.Get("conc")
	if s.SpanCount("conc") != 20 {
		t.Errorf("SpanCount = %d, want 20", s.SpanCount("conc"))
	}
	if s.AuditCount("conc") != 20 {
		t.Errorf("AuditCount = %d, want 20", s.AuditCount("conc"))
	}
	if rt.Metrics.LLMCalls != 20 || rt.Metrics.ToolCalls != 20 {
		t.Errorf("metrics llm/tool = %d/%d, want 20/20", rt.Metrics.LLMCalls, rt.Metrics.ToolCalls)
	}
}

// TestRequestTrace_JSON 验证 RequestTrace 可序列化（发布/查询附件格式）。
func TestRequestTrace_JSON(t *testing.T) {
	s := NewCorrelationStore()
	s.Start("t-json", "agent-j", "sess-j")
	s.AddSpan("t-json", SpanRecord{Name: "agent.run", SpanID: "s1"})
	s.AddAuditEvent("t-json", AuditEvent{Action: "agent.start", Result: "success"})
	s.RecordLLM("t-json", time.Millisecond, 1, 1, 0)
	s.End("t-json")

	data, err := json.Marshal(s.Get("t-json"))
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	str := string(data)
	for _, field := range []string{"trace_id", "spans", "audit_events", "metrics", "llm_calls"} {
		if !strings.Contains(str, field) {
			t.Errorf("JSON 缺少字段 %s", field)
		}
	}
}

// TestSortTraceByStart 验证排序。
func TestSortTraceByStart(t *testing.T) {
	s := NewCorrelationStore()
	s.Start("first", "a", "")
	time.Sleep(2 * time.Millisecond)
	s.Start("second", "b", "")

	traces := s.List(0)
	SortTraceByStart(traces, true)
	if traces[0].TraceID != "second" {
		t.Errorf("倒序第一个 = %q, want second", traces[0].TraceID)
	}
}

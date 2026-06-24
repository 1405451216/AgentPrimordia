// Package audit 提供审计日志功能，用于记录和查询 Agent 操作的合规性事件。
package audit

import (
	"context"
	"sync"
	"testing"
	"time"
)

// memoryOutput 是基于内存的 Output 实现，用于测试。
type memoryOutput struct {
	mu     sync.RWMutex
	events []Event
}

func newMemoryOutput() *memoryOutput {
	return &memoryOutput{
		events: make([]Event, 0),
	}
}

func (m *memoryOutput) Write(e Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, e)
	return nil
}

func (m *memoryOutput) Query(f QueryFilter) ([]Event, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []Event
	for _, e := range m.events {
		// 按 Actor 过滤
		if f.Actor != "" && e.Actor != f.Actor {
			continue
		}
		// 按 Action 过滤
		if f.Action != "" && e.Action != f.Action {
			continue
		}
		// 按 Resource 过滤
		if f.Resource != "" && e.Resource != f.Resource {
			continue
		}
		// 按时间范围过滤
		if !f.Start.IsZero() && e.Timestamp.Before(f.Start) {
			continue
		}
		if !f.End.IsZero() && e.Timestamp.After(f.End) {
			continue
		}
		result = append(result, e)
	}

	// 应用 Limit
	if f.Limit > 0 && len(result) > f.Limit {
		result = result[:f.Limit]
	}

	return result, nil
}

// TestAuditLogger_Log 测试记录审计事件
func TestAuditLogger_Log(t *testing.T) {
	out := newMemoryOutput()
	logger := NewLogger(LoggerConfig{Output: out})

	ctx := context.Background()
	evt := Event{
		Actor:    "agent-1",
		Action:   "file.read",
		Resource: "/data/config.yaml",
		Details:  map[string]any{"bytes": 1024},
		Result:   "success",
	}

	err := logger.Log(ctx, evt)
	if err != nil {
		t.Fatalf("Log 返回意外错误: %v", err)
	}

	// 验证事件已写入
	out.mu.RLock()
	defer out.mu.RUnlock()
	if len(out.events) != 1 {
		t.Fatalf("期望 1 条事件，实际 %d 条", len(out.events))
	}

	// 验证 Timestamp 自动填充
	if out.events[0].Timestamp.IsZero() {
		t.Error("Timestamp 未自动填充")
	}

	// 验证字段值
	got := out.events[0]
	if got.Actor != "agent-1" {
		t.Errorf("Actor = %q, 期望 %q", got.Actor, "agent-1")
	}
	if got.Action != "file.read" {
		t.Errorf("Action = %q, 期望 %q", got.Action, "file.read")
	}
	if got.Resource != "/data/config.yaml" {
		t.Errorf("Resource = %q, 期望 %q", got.Resource, "/data/config.yaml")
	}
	if got.Result != "success" {
		t.Errorf("Result = %q, 期望 %q", got.Result, "success")
	}
}

// TestAuditLogger_Query 测试按条件查询审计事件
func TestAuditLogger_Query(t *testing.T) {
	out := newMemoryOutput()
	logger := NewLogger(LoggerConfig{Output: out})

	ctx := context.Background()

	// 记录多条事件
	events := []Event{
		{Actor: "agent-1", Action: "file.read", Resource: "/data/a.yaml", Result: "success"},
		{Actor: "agent-1", Action: "file.write", Resource: "/data/b.yaml", Result: "success"},
		{Actor: "agent-2", Action: "file.read", Resource: "/data/c.yaml", Result: "denied"},
		{Actor: "agent-2", Action: "shell.exec", Resource: "/bin/ls", Result: "success"},
		{Actor: "agent-1", Action: "file.read", Resource: "/data/d.yaml", Result: "success"},
	}

	for _, e := range events {
		if err := logger.Log(ctx, e); err != nil {
			t.Fatalf("Log 返回意外错误: %v", err)
		}
	}

	// 按 Actor 查询
	results, err := logger.Query(ctx, QueryFilter{Actor: "agent-1"})
	if err != nil {
		t.Fatalf("Query 返回意外错误: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("按 Actor=agent-1 查询，期望 3 条，实际 %d 条", len(results))
	}

	// 按 Action 查询
	results, err = logger.Query(ctx, QueryFilter{Action: "file.read"})
	if err != nil {
		t.Fatalf("Query 返回意外错误: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("按 Action=file.read 查询，期望 3 条，实际 %d 条", len(results))
	}

	// 按 Actor + Action 组合查询
	results, err = logger.Query(ctx, QueryFilter{Actor: "agent-2", Action: "shell.exec"})
	if err != nil {
		t.Fatalf("Query 返回意外错误: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("按 Actor=agent-2 + Action=shell.exec 查询，期望 1 条，实际 %d 条", len(results))
	}

	// 按 Limit 限制返回数量
	results, err = logger.Query(ctx, QueryFilter{Actor: "agent-1", Limit: 2})
	if err != nil {
		t.Fatalf("Query 返回意外错误: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("按 Actor=agent-1 + Limit=2 查询，期望 2 条，实际 %d 条", len(results))
	}
}

// TestAuditLogger_ComplianceReport 测试合规报告生成
func TestAuditLogger_ComplianceReport(t *testing.T) {
	out := newMemoryOutput()
	logger := NewLogger(LoggerConfig{Output: out})

	ctx := context.Background()

	// 记录不同 Actor 和 Action 的事件
	events := []Event{
		{Actor: "agent-1", Action: "file.read", Resource: "/data/a.yaml", Result: "success"},
		{Actor: "agent-1", Action: "file.write", Resource: "/data/b.yaml", Result: "success"},
		{Actor: "agent-1", Action: "file.read", Resource: "/data/c.yaml", Result: "success"},
		{Actor: "agent-2", Action: "shell.exec", Resource: "/bin/ls", Result: "success"},
		{Actor: "agent-2", Action: "shell.exec", Resource: "/bin/rm", Result: "denied"},
	}

	for _, e := range events {
		if err := logger.Log(ctx, e); err != nil {
			t.Fatalf("Log 返回意外错误: %v", err)
		}
	}

	// 生成报告（使用零值时间表示无时间范围限制）
	start := time.Time{}
	end := time.Time{}
	report, err := logger.GenerateReport(ctx, start, end)
	if err != nil {
		t.Fatalf("GenerateReport 返回意外错误: %v", err)
	}

	// 验证总事件数
	if report.TotalEvents != 5 {
		t.Errorf("TotalEvents = %d, 期望 5", report.TotalEvents)
	}

	// 验证 Actor 统计
	if len(report.ActorStats) != 2 {
		t.Errorf("ActorStats 数量 = %d, 期望 2", len(report.ActorStats))
	}

	// 验证 agent-1 的统计
	a1, ok := report.ActorStats["agent-1"]
	if !ok {
		t.Fatal("缺少 agent-1 的统计")
	}
	if a1.TotalActions != 3 {
		t.Errorf("agent-1 TotalActions = %d, 期望 3", a1.TotalActions)
	}
	if a1.Actions["file.read"] != 2 {
		t.Errorf("agent-1 file.read 次数 = %d, 期望 2", a1.Actions["file.read"])
	}
	if a1.Actions["file.write"] != 1 {
		t.Errorf("agent-1 file.write 次数 = %d, 期望 1", a1.Actions["file.write"])
	}

	// 验证 agent-2 的统计
	a2, ok := report.ActorStats["agent-2"]
	if !ok {
		t.Fatal("缺少 agent-2 的统计")
	}
	if a2.TotalActions != 2 {
		t.Errorf("agent-2 TotalActions = %d, 期望 2", a2.TotalActions)
	}
	if a2.Actions["shell.exec"] != 2 {
		t.Errorf("agent-2 shell.exec 次数 = %d, 期望 2", a2.Actions["shell.exec"])
	}

	// 验证 Action 统计
	if report.ActionStats["file.read"] != 2 {
		t.Errorf("ActionStats[file.read] = %d, 期望 2", report.ActionStats["file.read"])
	}
	if report.ActionStats["file.write"] != 1 {
		t.Errorf("ActionStats[file.write] = %d, 期望 1", report.ActionStats["file.write"])
	}
	if report.ActionStats["shell.exec"] != 2 {
		t.Errorf("ActionStats[shell.exec] = %d, 期望 2", report.ActionStats["shell.exec"])
	}

	// 验证 ExportJSON 输出
	jsonStr, err := report.ExportJSON()
	if err != nil {
		t.Fatalf("ExportJSON 返回意外错误: %v", err)
	}
	if len(jsonStr) == 0 {
		t.Error("ExportJSON 返回空字符串")
	}
	// 验证 JSON 包含关键字段
	if !contains(jsonStr, "total_events") {
		t.Error("ExportJSON 输出缺少 total_events 字段")
	}
	if !contains(jsonStr, "agent-1") {
		t.Error("ExportJSON 输出缺少 agent-1")
	}
}

// contains 检查字符串是否包含子串
func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(len(s) > 0 && len(sub) > 0 && findSubstring(s, sub)))
}

func findSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

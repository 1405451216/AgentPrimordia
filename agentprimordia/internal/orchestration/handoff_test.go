package orchestration

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"agentprimordia/cmd/example/demo"
	"agentprimordia/internal/agent"
)

func TestHandoffProtocol_InitiateAndAccept(t *testing.T) {
	protocol := NewHandoffProtocol(HandoffConfig{
		EnableValidation: true,
	})

	ctx := CreateStandardContext("测试交接", map[string]any{"data": "test"}, 5)
	record, err := protocol.InitiateHandoff(context.Background(), "AgentA", "AgentB", HandoffDirect, ctx)
	if err != nil {
		t.Fatalf("InitiateHandoff error: %v", err)
	}

	if record.Status != HandoffPending {
		t.Errorf("expected pending status, got %s", record.Status)
	}
	if record.SourceAgent != "AgentA" || record.TargetAgent != "AgentB" {
		t.Error("agent names mismatch")
	}

	err = protocol.AcceptHandoff(record.ID, "AgentB")
	if err != nil {
		t.Fatalf("AcceptHandoff error: %v", err)
	}

	record, _ = protocol.GetHandoff(record.ID)
	if record.Status != HandoffAccepted {
		t.Errorf("expected accepted status, got %s", record.Status)
	}
	if !record.Acknowledged {
		t.Error("handoff should be acknowledged")
	}

	t.Logf("✅ Initiate & Accept: id=%s status=%s acknowledged=%v", record.ID, record.Status, record.Acknowledged)
}

func TestHandoffProtocol_CompleteAndFail(t *testing.T) {
	protocol := NewHandoffProtocol(HandoffConfig{})

	ctx := CreateStandardContext("完整流程测试", nil, 3)
	record, _ := protocol.InitiateHandoff(context.Background(), "Source", "Target", HandoffDirect, ctx)
	_ = protocol.AcceptHandoff(record.ID, "Target")

	err := protocol.CompleteHandoff(record.ID)
	if err != nil {
		t.Fatalf("CompleteHandoff error: %v", err)
	}

	record, _ = protocol.GetHandoff(record.ID)
	if record.Status != HandoffCompleted {
		t.Errorf("expected completed status, got %s", record.Status)
	}
	if record.Duration < 0 { // 允许为0（快速执行）
		t.Error("duration should be non-negative")
	}

	stats := protocol.GetStats()
	if stats.Successful.Load() != 1 {
		t.Errorf("expected 1 successful, got %d", stats.Successful.Load())
	}

	t.Logf("✅ Complete: duration=%v success=%d", record.Duration, stats.Successful.Load())

	// 测试失败
	ctx2 := CreateStandardContext("失败测试", nil, 1)
	record2, _ := protocol.InitiateHandoff(context.Background(), "S2", "T2", HandoffDirect, ctx2)
	_ = protocol.AcceptHandoff(record2.ID, "T2")

	failErr := fmt.Errorf("模拟执行错误")
	_ = protocol.FailHandoff(record2.ID, failErr)

	record2, _ = protocol.GetHandoff(record2.ID)
	if record2.Status != HandoffFailed {
		t.Errorf("expected failed status, got %s", record2.Status)
	}

	stats2 := protocol.GetStats()
	if stats2.Failed.Load() != 1 {
		t.Errorf("expected 1 failed, got %d", stats2.Failed.Load())
	}

	t.Logf("✅ Fail: failed=%d total=%d", stats2.Failed.Load(), stats2.TotalHandoffs.Load())
}

func TestHandoffProtocol_Reject(t *testing.T) {
	protocol := NewHandoffProtocol(HandoffConfig{})

	ctx := CreateStandardContext("拒绝测试", nil, 7)
	record, _ := protocol.InitiateHandoff(context.Background(), "A", "B", HandoffConditional, ctx)

	reason := "当前负载过高，无法接受新任务"
	err := protocol.RejectHandoff(record.ID, reason)
	if err != nil {
		t.Fatalf("RejectHandoff error: %v", err)
	}

	record, _ = protocol.GetHandoff(record.ID)
	if record.Status != HandoffRejected {
		t.Errorf("expected rejected status, got %s", record.Status)
	}

	stats := protocol.GetStats()
	if stats.Rejected.Load() != 1 {
		t.Errorf("expected 1 rejected, got %d", stats.Rejected.Load())
	}

	t.Logf("✅ Reject: reason=%s rejected=%d", reason, stats.Rejected.Load())
}

func TestHandoffProtocol_Validation(t *testing.T) {
	protocol := NewHandoffProtocol(HandoffConfig{EnableValidation: true})

	// 测试缺少必要字段
	_, err := protocol.InitiateHandoff(context.Background(), "", "Target", HandoffDirect, &HandoffContext{})
	if err == nil {
		t.Fatal("expected validation error for empty source agent")
	}

	_, err = protocol.InitiateHandoff(context.Background(), "Source", "", HandoffDirect, &HandoffContext{})
	if err == nil {
		t.Fatal("expected validation error for empty target agent")
	}

	_, err = protocol.InitiateHandoff(context.Background(), "Source", "Target", HandoffDirect, nil)
	if err == nil {
		t.Fatal("expected validation error for nil context")
	}

	// 测试无效类型
	invalidType := HandoffType("invalid_type")
	_, err = protocol.InitiateHandoff(context.Background(), "Source", "Target", invalidType, &HandoffContext{})
	if err == nil {
		t.Fatal("expected validation error for invalid type")
	}

	t.Logf("✅ Validation: all invalid cases caught correctly")
}

func TestHandoffContext_Manipulation(t *testing.T) {
	ctx := CreateStandardContext("上下文操作测试", map[string]any{"key": "value"}, 8)

	ctx.AddVariable("var1", "value1")
	ctx.AddVariable("var2", 12345)
	ctx.AddTask("完成任务A")
	ctx.AddTask("完成任务B")
	ctx.AddAttachment("report.pdf", "file", []byte("PDF内容"))
	ctx.AddAttachment("data.json", "json", map[string]string{"status": "ok"})
	ctx.Priority = 9
	ctx.Urgency = "high"

	if len(ctx.Variables) != 2 {
		t.Errorf("expected 2 variables, got %d", len(ctx.Variables))
	}
	if len(ctx.TasksRemaining) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(ctx.TasksRemaining))
	}
	if len(ctx.Attachments) != 2 {
		t.Errorf("expected 2 attachments, got %d", len(ctx.Attachments))
	}

	jsonData, err := ctx.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON error: %v", err)
	}

	restoredCtx, err := FromJSON(jsonData)
	if err != nil {
		t.Fatalf("FromJSON error: %v", err)
	}

	if restoredCtx.Message != ctx.Message {
		t.Error("message mismatch after round-trip")
	}
	if len(restoredCtx.Variables) != len(ctx.Variables) {
		t.Error("variables count mismatch after round-trip")
	}

	t.Logf("✅ Context Manipulation: vars=%d tasks=%d attachments=%d json_size=%d",
		len(ctx.Variables), len(ctx.TasksRemaining), len(ctx.Attachments), len(jsonData))
}

func TestHandoffManager_Integration(t *testing.T) {
	manager := NewHandoffManager(HandoffConfig{
		EnableValidation: true,
		RequireAck:       true,
	})

	targetAgent, err := agent.NewAgent("TargetAgent", "接收交接并继续工作", demo.NewDemoLLM("已接收交接，继续处理"), agent.WithMaxTurns(1))
	if err != nil {
		t.Fatal(err)
	}
	manager.RegisterAgent("TargetAgent", targetAgent)

	// 添加规则：高优先级必须接受
	manager.AddRule(HandoffRule{
		Name:        "HighPriorityAccept",
		Description: "高优先级交接必须接受",
		Condition:   func(ctx *HandoffContext) bool { return ctx.Priority >= 8 },
		Action:      "allow",
		Priority:    10,
	})

	handoffCtx := CreateStandardContext(
		"重要任务交接",
		map[string]any{"user_id": "12345", "task_type": "urgent"},
		9,
	)
	handoffCtx.Urgency = "critical"
	handoffCtx.AddVariable("deadline", "2024-01-15")

	record, err := manager.ExecuteHandoff(context.Background(), "SourceAgent", "TargetAgent", handoffCtx)
	if err != nil {
		t.Fatalf("ExecuteHandoff error: %v", err)
	}

	if record.Status != HandoffCompleted {
		t.Errorf("expected completed, got %s", record.Status)
	}

	stats := manager.GetProtocol().GetStats()
	t.Logf("✅ Manager Integration: status=%s successful=%d", record.Status, stats.Successful.Load())
	t.Logf("   Duration: %v", record.Duration)
}

func TestHandoffProtocol_Events(t *testing.T) {
	protocol := NewHandoffProtocol(HandoffConfig{})

	events := make([]*HandoffEvent, 0)
	go func() {
		for event := range protocol.Events() {
			events = append(events, event)
			if len(events) >= 4 {
				break
			}
		}
	}()

	ctx := CreateStandardContext("事件测试", nil, 5)
	record, _ := protocol.InitiateHandoff(context.Background(), "E1", "E2", HandoffDirect, ctx)
	_ = protocol.AcceptHandoff(record.ID, "E2")
	_ = protocol.CompleteHandoff(record.ID)

	time.Sleep(50 * time.Millisecond) // 等待事件收集

	eventTypes := make(map[string]bool)
	for _, e := range events {
		eventTypes[e.Type] = true
	}

	expectedTypes := []string{"initiated", "accepted", "completed"}
	for _, expectedType := range expectedTypes {
		if !eventTypes[expectedType] {
			t.Errorf("missing event type: %s", expectedType)
		}
	}

	t.Logf("✅ Events: collected %d events", len(events))
	for _, e := range events {
		t.Logf("   - %s (id=%s)", e.Type, e.HandoffID)
	}
}

func TestHandoffProtocol_Export(t *testing.T) {
	protocol := NewHandoffProtocol(HandoffConfig{
		EnableValidation: true,
		LogLevel:         "debug",
	})

	ctx := CreateStandardContext("导出测试", map[string]any{"export": true}, 6)
	record1, _ := protocol.InitiateHandoff(context.Background(), "X1", "Y1", HandoffDirect, ctx)
	_ = protocol.AcceptHandoff(record1.ID, "Y1")
	_ = protocol.CompleteHandoff(record1.ID)

	data, err := protocol.Export()
	if err != nil {
		t.Fatalf("Export error: %v", err)
	}

	var exported map[string]any
	_ = json.Unmarshal(data, &exported)

	if exported["config"] == nil {
		t.Error("missing config in export")
	}
	if exported["stats"] == nil {
		t.Error("missing stats in export")
	}

	t.Logf("✅ Export: size=%d bytes", len(data))

	formattedJSON, _ := json.MarshalIndent(json.RawMessage(data), "", "  ")
	t.Logf("   Preview:\n%s", string(formattedJSON)[:min(len(string(formattedJSON)), 300)])
}

func TestHandoffProtocol_ConcurrentHandoffs(t *testing.T) {
	protocol := NewHandoffProtocol(HandoffConfig{})

	var wg sync.WaitGroup
	errors := make(chan error, 10)

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			ctx := CreateStandardContext(fmt.Sprintf("并发交接%d", idx), nil, idx%10+1)
			record, err := protocol.InitiateHandoff(context.Background(), fmt.Sprintf("Src%d", idx), fmt.Sprintf("Tgt%d", idx), HandoffDirect, ctx)
			if err != nil {
				errors <- err
				return
			}

			_ = protocol.AcceptHandoff(record.ID, fmt.Sprintf("Tgt%d", idx))
			_ = protocol.CompleteHandoff(record.ID)
		}(i)
	}

	wg.Wait()
	close(errors)

	errCount := 0
	for err := range errors {
		errCount++
		t.Logf("Concurrent error: %v", err)
	}

	stats := protocol.GetStats()
	if stats.TotalHandoffs.Load() != 5 {
		t.Errorf("expected 5 handoffs, got %d", stats.TotalHandoffs.Load())
	}
	if stats.Successful.Load() != int64(5-errCount) {
		t.Errorf("expected %d successful, got %d", 5-errCount, stats.Successful.Load())
	}

	active := protocol.GetActiveHandoffs()
	if len(active) != 0 {
		t.Errorf("expected no active handoffs, got %d", len(active))
	}

	t.Logf("✅ Concurrent Handoffs: total=%d successful=%d errors=%d", stats.TotalHandoffs.Load(), stats.Successful.Load(), errCount)
}

func BenchmarkHandoffProtocol_Initiate(b *testing.B) {
	protocol := NewHandoffProtocol(HandoffConfig{EnableValidation: false})
	ctx := CreateStandardContext("benchmark", nil, 5)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			i++
			_, _ = protocol.InitiateHandoff(context.Background(), fmt.Sprintf("S%d", i), fmt.Sprintf("T%d", i), HandoffDirect, ctx)
		}
	})
}

func BenchmarkHandoffManager_Execute(b *testing.B) {
	manager := NewHandoffManager(HandoffConfig{EnableValidation: false})

	targetAgent, err := agent.NewAgent("BenchmarkTarget", "", demo.NewDemoLLM("done"), agent.WithMaxTurns(1))
	if err != nil {
		b.Fatal(err)
	}
	manager.RegisterAgent("BenchmarkTarget", targetAgent)

	ctx := CreateStandardContext("bench", nil, 5)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = manager.ExecuteHandoff(context.Background(), "Source", "BenchmarkTarget", ctx)
	}
}

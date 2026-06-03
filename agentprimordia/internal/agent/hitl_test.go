package agent

import (
	"context"
	"testing"
	"time"
)

func TestInterruptReason_Constants(t *testing.T) {
	reasons := []InterruptReason{
		InterruptToolConfirm,
		InterruptDecisionPoint,
		InterruptBudgetExceed,
		InterruptCustom,
	}
	for _, r := range reasons {
		if string(r) == "" {
			t.Errorf("InterruptReason constant should not be empty")
		}
	}
}

func TestHITLManager_ShouldInterrupt_ToolConfirm(t *testing.T) {
	config := HITLConfig{
		InterruptPoints: []InterruptPoint{
			{Type: InterruptToolConfirm, ToolName: "delete_file"},
		},
		AutoApproveTools: []string{"read_file"},
	}
	mgr := NewHITLManager(config)

	if !mgr.ShouldInterrupt("delete_file", InterruptToolConfirm) {
		t.Error("delete_file should trigger interrupt")
	}
	if mgr.ShouldInterrupt("read_file", InterruptToolConfirm) {
		t.Error("read_file is in AutoApproveTools, should not interrupt")
	}
	if mgr.ShouldInterrupt("write_file", InterruptToolConfirm) {
		t.Error("write_file is not in InterruptPoints, should not interrupt")
	}
}

func TestHITLManager_ShouldInterrupt_AllTools(t *testing.T) {
	config := HITLConfig{
		InterruptPoints: []InterruptPoint{
			{Type: InterruptToolConfirm, ToolName: ""}, // 空字符串 = 所有工具
		},
	}
	mgr := NewHITLManager(config)

	if !mgr.ShouldInterrupt("any_tool", InterruptToolConfirm) {
		t.Error("empty ToolName should match all tools")
	}
}

func TestHITLManager_ShouldInterrupt_AutoApproveOverrides(t *testing.T) {
	config := HITLConfig{
		InterruptPoints: []InterruptPoint{
			{Type: InterruptToolConfirm, ToolName: ""}, // 所有工具需确认
		},
		AutoApproveTools: []string{"safe_tool"},
	}
	mgr := NewHITLManager(config)

	if mgr.ShouldInterrupt("safe_tool", InterruptToolConfirm) {
		t.Error("AutoApproveTools should override InterruptPoints")
	}
	if !mgr.ShouldInterrupt("dangerous_tool", InterruptToolConfirm) {
		t.Error("non-auto-approve tool should still interrupt")
	}
}

func TestHITLManager_ShouldInterrupt_NoConfig(t *testing.T) {
	mgr := NewHITLManager(HITLConfig{})

	if mgr.ShouldInterrupt("any_tool", InterruptToolConfirm) {
		t.Error("no InterruptPoints configured should not interrupt")
	}
}

func TestHITLManager_RequestInterrupt_Approved(t *testing.T) {
	responseCh := make(chan *HumanResponse, 1)
	responseCh <- &HumanResponse{Approved: true}

	config := HITLConfig{
		InterruptPoints: []InterruptPoint{
			{Type: InterruptToolConfirm, ToolName: "delete_file"},
		},
		HumanInputChan: responseCh,
	}
	mgr := NewHITLManager(config)

	req := &InterruptRequest{
		Reason:  InterruptToolConfirm,
		Message: "确认删除文件？",
		Data:    map[string]any{"file": "test.txt"},
		Turn:    3,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := mgr.RequestInterrupt(ctx, req)
	if err != nil {
		t.Fatalf("RequestInterrupt error: %v", err)
	}
	if !resp.Approved {
		t.Error("expected approval")
	}
}

func TestHITLManager_RequestInterrupt_Rejected(t *testing.T) {
	responseCh := make(chan *HumanResponse, 1)
	responseCh <- &HumanResponse{Approved: false, Input: "取消操作"}

	config := HITLConfig{
		InterruptPoints: []InterruptPoint{
			{Type: InterruptToolConfirm, ToolName: "delete_file"},
		},
		HumanInputChan: responseCh,
	}
	mgr := NewHITLManager(config)

	req := &InterruptRequest{
		Reason:  InterruptToolConfirm,
		Message: "确认删除？",
		Turn:    1,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := mgr.RequestInterrupt(ctx, req)
	if err != nil {
		t.Fatalf("RequestInterrupt error: %v", err)
	}
	if resp.Approved {
		t.Error("expected rejection")
	}
	if resp.Input != "取消操作" {
		t.Errorf("Input = %q, want %q", resp.Input, "取消操作")
	}
}

func TestHITLManager_RequestInterrupt_Timeout(t *testing.T) {
	responseCh := make(chan *HumanResponse)

	config := HITLConfig{
		HumanInputChan: responseCh,
	}
	mgr := NewHITLManager(config)

	req := &InterruptRequest{
		Reason:  InterruptToolConfirm,
		Message: "等待超时",
		Turn:    0,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := mgr.RequestInterrupt(ctx, req)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestHITLManager_Resume(t *testing.T) {
	responseCh := make(chan *HumanResponse, 1)
	config := HITLConfig{
		HumanInputChan: responseCh,
	}
	mgr := NewHITLManager(config)

	go func() {
		time.Sleep(50 * time.Millisecond)
		mgr.Resume(&HumanResponse{Approved: true, Input: "确认"})
	}()

	req := &InterruptRequest{
		Reason:  InterruptToolConfirm,
		Message: "请确认",
		Turn:    0,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := mgr.RequestInterrupt(ctx, req)
	if err != nil {
		t.Fatalf("RequestInterrupt error: %v", err)
	}
	if !resp.Approved {
		t.Error("expected approval via Resume")
	}
}

func TestHITLManager_PendingRequest(t *testing.T) {
	responseCh := make(chan *HumanResponse)
	config := HITLConfig{
		InterruptPoints: []InterruptPoint{
			{Type: InterruptToolConfirm, ToolName: "delete_file"},
		},
		HumanInputChan: responseCh,
	}
	mgr := NewHITLManager(config)

	if mgr.Pending() != nil {
		t.Error("initially no pending request")
	}

	req := &InterruptRequest{
		Reason:  InterruptToolConfirm,
		Message: "确认",
		Turn:    0,
	}

	pendingReady := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		<-pendingReady
		_, _ = mgr.RequestInterrupt(ctx, req)
	}()

	go func() {
		time.Sleep(100 * time.Millisecond)
		close(pendingReady)
		time.Sleep(100 * time.Millisecond)
		responseCh <- &HumanResponse{Approved: true}
	}()

	<-done

	pending := mgr.Pending()
	if pending != nil {
		t.Error("after completion, no pending request expected")
	}
}

func TestHITLManager_OnInterruptCallback(t *testing.T) {
	var callbackCalled bool
	var capturedReq *InterruptRequest

	responseCh := make(chan *HumanResponse, 1)
	responseCh <- &HumanResponse{Approved: true}

	config := HITLConfig{
		HumanInputChan: responseCh,
		OnInterrupt: func(req *InterruptRequest) {
			callbackCalled = true
			capturedReq = req
		},
	}
	mgr := NewHITLManager(config)

	req := &InterruptRequest{
		Reason:  InterruptDecisionPoint,
		Message: "需要决策",
		Turn:    5,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, _ = mgr.RequestInterrupt(ctx, req)

	if !callbackCalled {
		t.Error("OnInterrupt callback should have been called")
	}
	if capturedReq.Reason != InterruptDecisionPoint {
		t.Errorf("captured reason = %q, want %q", capturedReq.Reason, InterruptDecisionPoint)
	}
}

func TestHITLManager_HumanResponseWithModified(t *testing.T) {
	responseCh := make(chan *HumanResponse, 1)
	responseCh <- &HumanResponse{
		Approved: true,
		Modified: map[string]any{"file": "safe_backup.txt"},
	}

	config := HITLConfig{
		HumanInputChan: responseCh,
	}
	mgr := NewHITLManager(config)

	req := &InterruptRequest{
		Reason:  InterruptToolConfirm,
		Message: "确认",
		Turn:    0,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := mgr.RequestInterrupt(ctx, req)
	if err != nil {
		t.Fatalf("RequestInterrupt error: %v", err)
	}
	if resp.Modified["file"] != "safe_backup.txt" {
		t.Errorf("Modified[file] = %v, want %q", resp.Modified["file"], "safe_backup.txt")
	}
}

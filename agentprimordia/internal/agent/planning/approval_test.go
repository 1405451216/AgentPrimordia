package planning

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestPolicyApprovalGate_RequiresApproval 验证高风险动作判定
func TestPolicyApprovalGate_RequiresApproval(t *testing.T) {
	gate := NewPolicyApprovalGate([]string{"delete_file", "deploy_prod", "drop_table"})

	if !gate.RequiresApproval(context.Background(), "delete_file") {
		t.Fatal("delete_file 应为高风险动作")
	}
	if !gate.RequiresApproval(context.Background(), "deploy_prod") {
		t.Fatal("deploy_prod 应为高风险动作")
	}
	if gate.RequiresApproval(context.Background(), "read_file") {
		t.Fatal("read_file 不应为高风险动作")
	}
}

// TestPolicyApprovalGate_RequestApproval_NonHighRisk 非高风险动作直接放行
func TestPolicyApprovalGate_RequestApproval_NonHighRisk(t *testing.T) {
	gate := NewPolicyApprovalGate([]string{"delete_file"})

	err := gate.RequestApproval(context.Background(), "read_file", "读取文件")
	if err != nil {
		t.Fatalf("非高风险动作应直接放行: %v", err)
	}
}

// TestPolicyApprovalGate_RequestApproval_Duplicate 重复提交审批请求应返回错误
func TestPolicyApprovalGate_RequestApproval_Duplicate(t *testing.T) {
	gate := NewPolicyApprovalGate([]string{"delete_file"})

	err := gate.RequestApproval(context.Background(), "delete_file", "删除重要文件")
	if err != nil {
		t.Fatalf("首次提交应成功: %v", err)
	}

	err = gate.RequestApproval(context.Background(), "delete_file", "再次删除")
	if err == nil {
		t.Fatal("重复提交应返回错误")
	}
}

// TestPolicyApprovalGate_WaitApproval_NoPending 无待审批请求时直接放行
func TestPolicyApprovalGate_WaitApproval_NoPending(t *testing.T) {
	gate := NewPolicyApprovalGate([]string{"delete_file"})

	err := gate.WaitApproval(context.Background(), "read_file")
	if err != nil {
		t.Fatalf("无待审批请求应直接返回: %v", err)
	}
}

// TestPolicyApprovalGate_ApproveAndWait 验证审批通过流程
func TestPolicyApprovalGate_ApproveAndWait(t *testing.T) {
	gate := NewPolicyApprovalGate([]string{"deploy_prod"})
	ctx := context.Background()

	// 提交审批请求
	if err := gate.RequestApproval(ctx, "deploy_prod", "部署到生产环境"); err != nil {
		t.Fatalf("提交审批应成功: %v", err)
	}

	// 异步批准
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// 短暂延迟后批准
		time.Sleep(10 * time.Millisecond)
		if !gate.Approve("deploy_prod") {
			t.Error("批准应返回 true")
		}
	}()

	// 等待审批
	err := gate.WaitApproval(ctx, "deploy_prod")
	if err != nil {
		t.Fatalf("等待审批应成功: %v", err)
	}

	wg.Wait()
}

// TestPolicyApprovalGate_WaitApproval_ContextCancel 验证 context 取消
func TestPolicyApprovalGate_WaitApproval_ContextCancel(t *testing.T) {
	gate := NewPolicyApprovalGate([]string{"delete_file"})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_ = gate.RequestApproval(ctx, "delete_file", "删除")

	err := gate.WaitApproval(ctx, "delete_file")
	if err == nil {
		t.Fatal("context 超时后应返回错误")
	}
}

// TestPolicyApprovalGate_Approve_NoPending 批准无待审批的动作返回 false
func TestPolicyApprovalGate_Approve_NoPending(t *testing.T) {
	gate := NewPolicyApprovalGate([]string{"delete_file"})

	if gate.Approve("delete_file") {
		t.Fatal("无待审批请求时 Approve 应返回 false")
	}
}

// TestPolicyApprovalGate_PendingActions 验证待审批列表
func TestPolicyApprovalGate_PendingActions(t *testing.T) {
	gate := NewPolicyApprovalGate([]string{"a1", "a2", "a3"})
	ctx := context.Background()

	_ = gate.RequestApproval(ctx, "a1", "原因1")
	_ = gate.RequestApproval(ctx, "a2", "原因2")

	pending := gate.PendingActions()
	if len(pending) != 2 {
		t.Fatalf("待审批动作应为 2 个，实际 %d", len(pending))
	}

	// 批准一个后应只剩 1 个
	gate.Approve("a1")
	pending = gate.PendingActions()
	if len(pending) != 1 {
		t.Fatalf("批准后待审批动作应为 1 个，实际 %d", len(pending))
	}
}

// TestPolicyApprovalGate_ConcurrentApprove 验证并发安全性
func TestPolicyApprovalGate_ConcurrentApprove(t *testing.T) {
	gate := NewPolicyApprovalGate([]string{"action"})
	ctx := context.Background()

	_ = gate.RequestApproval(ctx, "action", "test")

	var wg sync.WaitGroup
	// 多个 goroutine 同时等待审批
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = gate.WaitApproval(ctx, "action")
		}()
	}

	// 短暂等待让所有 goroutine 进入等待状态
	time.Sleep(10 * time.Millisecond)
	gate.Approve("action")

	wg.Wait()
}

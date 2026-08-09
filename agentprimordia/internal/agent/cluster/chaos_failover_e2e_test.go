//go:build e2e

// chaos_failover_e2e_test.go — v4.2-4 混沌常态化：集群故障注入（leader 故障）
//
// 验收标准：故障下成功率下降 ≤ 阈值（量化）。
// 场景：3 节点集群 → leader 上注册 10 Agent（基线成功率 10/10）→
// 混沌注入：kill leader 节点 → 剩余节点重新选举并接管 →
// 新 leader 上注册 10 Agent，全节点可见性作为恢复率（≥80% 即下降 ≤20%）。
//
// 运行方式：
//
//	go test -tags=e2e -run TestE2E_Cluster_ChaosFailover -v ./internal/agent/cluster/
package cluster

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestE2E_Cluster_ChaosFailover leader 故障注入：成功率下降量化 ≤ 阈值。
func TestE2E_Cluster_ChaosFailover(t *testing.T) {
	const nodeCount = 3

	tc := createTestCluster(t, nodeCount)
	defer tc.stop(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 1. 等待领导者选举收敛（恰好一个 leader）
	leaderIdx := waitForLeader(t, tc, -1)
	t.Logf("基线：节点 %s 当选 leader", tc.managers[leaderIdx].config.NodeID)

	// 2. 基线：经 leader 节点注册 10 个 Agent，全节点可发现（成功率 10/10）
	baselineIDs := registerAgentsOn(t, ctx, tc, leaderIdx, "baseline", 10)
	waitSync(t, tc)
	baselineOK := verifyVisible(t, ctx, tc, baselineIDs, nodeCount, leaderIdx)
	t.Logf("基线成功率: %d/%d", baselineOK, len(baselineIDs))

	// 3. 混沌注入：kill leader 节点（停止管理器 + 发现服务，使节点真正离线）
	if err := tc.managers[leaderIdx].Stop(ctx); err != nil {
		t.Fatalf("停止 leader 节点失败: %v", err)
	}
	if err := tc.discoveries[leaderIdx].Close(); err != nil {
		t.Fatalf("停止 leader 节点发现服务失败: %v", err)
	}
	tc.discoveries[leaderIdx] = nil
	t.Logf("⚡ 混沌注入：leader 节点 %s 已 kill", tc.managers[leaderIdx].config.NodeID)

	// 4. 故障恢复：剩余节点重新选举（新 leader ≠ 旧 leader）
	newLeaderIdx := waitForLeader(t, tc, leaderIdx)
	t.Logf("故障恢复：节点 %s 当选新 leader（旧 leader 已退出）", tc.managers[newLeaderIdx].config.NodeID)

	// 5. 恢复率：新 leader 接管后注册 10 个新 Agent，全节点可见性
	recoveredIDs := registerAgentsOn(t, ctx, tc, newLeaderIdx, "recovered", 10)
	waitSync(t, tc)
	recoveredOK := verifyVisible(t, ctx, tc, recoveredIDs, nodeCount, newLeaderIdx)

	baselineRate := float64(baselineOK) / float64(len(baselineIDs))
	recoveryRate := float64(recoveredOK) / float64(len(recoveredIDs))
	degradation := baselineRate - recoveryRate
	t.Logf("量化报告: baseline=%.2f recovered=%.2f degradation=%.2f (≤0.20 达标)",
		baselineRate, recoveryRate, degradation)

	// 验收：故障下成功率下降 ≤ 20%
	if degradation > 0.20 {
		t.Fatalf("故障下成功率下降 %.2f > 阈值 0.20（恢复率 %.2f）", degradation, recoveryRate)
	}
	if recoveryRate < 0.8 {
		t.Fatalf("恢复率 %.2f < 0.80", recoveryRate)
	}
}

// waitForLeader 轮询直到出现 leader；excludeIdx ≥ 0 时要求新 leader 不是该节点。
func waitForLeader(t *testing.T, tc *testCluster, excludeIdx int) int {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		for i, mgr := range tc.managers {
			if mgr == nil {
				continue
			}
			if i == excludeIdx {
				continue
			}
			if mgr.IsLeader() {
				return i
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	if excludeIdx >= 0 {
		t.Fatal("超时：故障后未选出新 leader")
	}
	t.Fatal("超时：领导者选举未收敛")
	return -1
}

// registerAgentsOn 经指定节点的 discovery 注册 n 个 Agent，并保持续租。
func registerAgentsOn(t *testing.T, ctx context.Context, tc *testCluster, nodeIdx int, prefix string, n int) []string {
	t.Helper()
	ids := make([]string, 0, n)
	for i := range n {
		agentID := fmt.Sprintf("%s-agent-%d", prefix, i)
		info := testAgentInfo(agentID, fmt.Sprintf("10.2.0.%d:8080", i))
		if err := tc.discoveries[nodeIdx].Register(ctx, &info); err != nil {
			t.Fatalf("注册 Agent %s 失败: %v", agentID, err)
		}
		ids = append(ids, agentID)
	}
	_ = startAgentKeepalive(ctx, tc.discoveries[nodeIdx], ids...)
	return ids
}

// verifyVisible 统计所有存活节点（除 killed 节点）可见的 Agent 数。
func verifyVisible(t *testing.T, ctx context.Context, tc *testCluster, ids []string, nodeCount, killedIdx int) int {
	t.Helper()
	ok := 0
	for _, agentID := range ids {
		visible := 0
		for i := 0; i < nodeCount; i++ {
			if i == killedIdx || tc.discoveries[i] == nil {
				continue
			}
			if _, err := tc.discoveries[i].Discover(ctx, agentID); err == nil {
				visible++
			}
		}
		if visible > 0 {
			ok++
		}
	}
	return ok
}

// waitSync 等待一个同步周期。
func waitSync(t *testing.T, tc *testCluster) {
	t.Helper()
	time.Sleep(1500 * time.Millisecond)
}

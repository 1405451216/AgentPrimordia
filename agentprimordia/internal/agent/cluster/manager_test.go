package cluster

import (
	"context"
	"testing"
	"time"

	"agentprimordia/internal/agent/discovery"
)

func TestConsistentHashAddRemove(t *testing.T) {
	h := NewConsistentHash(64)

	h.AddNode("node-1")
	h.AddNode("node-2")
	h.AddNode("node-3")

	if h.RingSize() != 192 { // 3 * 64
		t.Errorf("RingSize = %d, expected 192", h.RingSize())
	}

	nodes := h.GetNodesList()
	if len(nodes) != 3 {
		t.Errorf("节点数 = %d, expected 3", len(nodes))
	}

	h.RemoveNode("node-2")
	if h.RingSize() != 128 { // 2 * 64
		t.Errorf("移除后 RingSize = %d, expected 128", h.RingSize())
	}
}

func TestConsistentHashGetNode(t *testing.T) {
	h := NewConsistentHash(64)
	h.AddNode("node-1")
	h.AddNode("node-2")
	h.AddNode("node-3")

	// 同一 key 应始终映射到同一节点
	node1, ok := h.GetNode("task-1")
	if !ok {
		t.Fatal("GetNode 失败")
	}
	node2, ok := h.GetNode("task-1")
	if !ok || node1 != node2 {
		t.Errorf("同一 key 映射到不同节点: %s vs %s", node1, node2)
	}

	// 不同 key 可能映射到不同节点
	nodes := make(map[string]bool)
	for i := 0; i < 100; i++ {
		if node, ok := h.GetNode(string(rune('a' + i))); ok {
			nodes[node] = true
		}
	}
	if len(nodes) < 2 {
		t.Logf("警告：100 个 key 只映射到 %d 个节点", len(nodes))
	}
}

func TestConsistentHashGetNodes(t *testing.T) {
	h := NewConsistentHash(64)
	h.AddNode("node-1")
	h.AddNode("node-2")
	h.AddNode("node-3")

	// 获取 2 个副本节点
	nodes := h.GetNodes("test-key", 2)
	if len(nodes) != 2 {
		t.Errorf("副本数 = %d, expected 2", len(nodes))
	}

	// 副本节点应不同
	if nodes[0] == nodes[1] {
		t.Error("两个副本不应相同")
	}

	// 获取超过节点数的副本
	nodes = h.GetNodes("test-key", 10)
	if len(nodes) > 3 {
		t.Errorf("副本数 = %d, 应 <= 3", len(nodes))
	}
}

func TestConsistentHashEmpty(t *testing.T) {
	h := NewConsistentHash(64)
	_, ok := h.GetNode("key")
	if ok {
		t.Error("空环应返回 false")
	}
}

func TestDistributedStateSetGet(t *testing.T) {
	s := NewDistributedState()

	s.Set("key1", "value1", 0)

	val, ok := s.Get("key1")
	if !ok {
		t.Fatal("key1 不存在")
	}
	if val != "value1" {
		t.Errorf("val = %s, expected value1", val)
	}
}

func TestDistributedStateTTL(t *testing.T) {
	s := NewDistributedState()

	s.Set("key1", "value1", 50*time.Millisecond)

	val, ok := s.Get("key1")
	if !ok || val != "value1" {
		t.Fatal("TTL 内应可获取")
	}

	time.Sleep(100 * time.Millisecond)

	_, ok = s.Get("key1")
	if ok {
		t.Error("TTL 过期后不应可获取")
	}
}

func TestDistributedStateDelete(t *testing.T) {
	s := NewDistributedState()
	s.Set("key1", "value1", 0)

	if !s.Delete("key1") {
		t.Error("Delete 应返回 true")
	}
	if _, ok := s.Get("key1"); ok {
		t.Error("删除后不应可获取")
	}
	if s.Delete("nonexistent") {
		t.Error("删除不存在的 key 应返回 false")
	}
}

func TestDistributedStateKeys(t *testing.T) {
	s := NewDistributedState()
	s.Set("key1", "value1", 0)
	s.Set("key2", "value2", 0)
	s.Set("key3", "value3", 0)

	keys := s.Keys()
	if len(keys) != 3 {
		t.Errorf("Keys = %d, expected 3", len(keys))
	}
}

func TestDistributedStateCleanup(t *testing.T) {
	s := NewDistributedState()
	s.Set("key1", "value1", 50*time.Millisecond)
	s.Set("key2", "value2", 0)

	time.Sleep(100 * time.Millisecond)

	removed := s.Cleanup()
	if removed != 1 {
		t.Errorf("Cleanup 移除 = %d, expected 1", removed)
	}

	if _, ok := s.Get("key2"); !ok {
		t.Error("key2 不应被清理")
	}
}

func TestDistributedStateSnapshot(t *testing.T) {
	s := NewDistributedState()
	s.Set("key1", "value1", 0)
	s.Set("key2", "value2", 0)

	snap := s.Snapshot()
	if len(snap) != 2 {
		t.Errorf("Snapshot = %d, expected 2", len(snap))
	}
	if snap["key1"] != "value1" {
		t.Error("Snapshot 值不正确")
	}
}

func TestDistributedStateMerge(t *testing.T) {
	s := NewDistributedState()
	s.Set("key1", "local", 0) // version 1

	// 远程版本更高
	remote := map[string]RemoteEntry{
		"key1": {Value: "remote", Version: 2},
		"key2": {Value: "new", Version: 1},
	}

	merged := s.Merge(remote)
	if merged != 2 {
		t.Errorf("Merged = %d, expected 2", merged)
	}

	val, _ := s.Get("key1")
	if val != "remote" {
		t.Errorf("key1 = %s, expected remote", val)
	}

	val, _ = s.Get("key2")
	if val != "new" {
		t.Errorf("key2 = %s, expected new", val)
	}
}

func TestClusterManagerStartStop(t *testing.T) {
	disc := discovery.NewLocalDiscovery()
	mgr := NewClusterManager(ClusterConfig{
		NodeID:     "test-node-1",
		ListenAddr: ":18080",
		Discovery:  disc,
		HeartbeatInterval: 1 * time.Second,
		HeartbeatTimeout:  3 * time.Second,
		ElectionTimeout:   2 * time.Second,
	})

	ctx := context.Background()
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start 失败: %v", err)
	}

	// 检查本地节点信息
	node := mgr.GetLocalNode()
	if node.ID != "test-node-1" {
		t.Errorf("NodeID = %s", node.ID)
	}

	// 列出节点
	nodes := mgr.ListNodes()
	if len(nodes) < 1 {
		t.Error("至少应有 1 个节点")
	}

	// 停止
	if err := mgr.Stop(ctx); err != nil {
		t.Fatalf("Stop 失败: %v", err)
	}
}

func TestClusterManagerLeaderElection(t *testing.T) {
	disc := discovery.NewLocalDiscovery()
	mgr := NewClusterManager(ClusterConfig{
		NodeID:     "solo-node",
		ListenAddr: ":18081",
		Discovery:  disc,
		HeartbeatInterval: 100 * time.Millisecond,
		HeartbeatTimeout:  300 * time.Millisecond,
		ElectionTimeout:   200 * time.Millisecond,
	})

	ctx := context.Background()
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start 失败: %v", err)
	}
	defer mgr.Stop(ctx)

	// 等待选举完成
	time.Sleep(500 * time.Millisecond)

	// 单节点应成为领导者
	if !mgr.IsLeader() {
		t.Error("单节点应成为领导者")
	}
	if mgr.GetLeader() != "solo-node" {
		t.Errorf("Leader = %s, expected solo-node", mgr.GetLeader())
	}
}

func TestClusterManagerHashRing(t *testing.T) {
	disc := discovery.NewLocalDiscovery()
	mgr := NewClusterManager(ClusterConfig{
		NodeID:    "node-1",
		Discovery: disc,
	})

	ring := mgr.GetHashRing()
	if ring == nil {
		t.Fatal("HashRing 为 nil")
	}

	// 本地节点应在环上
	if ring.RingSize() == 0 {
		t.Error("环应为空")
	}
}

func TestClusterManagerState(t *testing.T) {
	disc := discovery.NewLocalDiscovery()
	mgr := NewClusterManager(ClusterConfig{
		NodeID:    "node-1",
		Discovery: disc,
	})

	state := mgr.GetState()
	if state == nil {
		t.Fatal("State 为 nil")
	}

	state.Set("test-key", "test-value", 0)
	val, ok := state.Get("test-key")
	if !ok || val != "test-value" {
		t.Errorf("Get = %s, %v", val, ok)
	}
}

package marketplace

import (
	"encoding/json"
	"testing"
	"time"
)

// ===== Mock 实现 =====

// mockState 模拟分布式状态存储
type mockState struct {
	data map[string]string
}

func newMockState() *mockState {
	return &mockState{data: make(map[string]string)}
}

func (s *mockState) Set(key, value string, ttl time.Duration) {
	s.data[key] = value
}

func (s *mockState) Get(key string) (string, bool) {
	v, ok := s.data[key]
	return v, ok
}

func (s *mockState) Delete(key string) bool {
	_, ok := s.data[key]
	delete(s.data, key)
	return ok
}

func (s *mockState) Keys() []string {
	keys := make([]string, 0, len(s.data))
	for k := range s.data {
		keys = append(keys, k)
	}
	return keys
}

// mockHashRing 模拟一致性哈希环
type mockHashRing struct {
	nodes []string
}

func (h *mockHashRing) GetNode(key string) (string, bool) {
	if len(h.nodes) == 0 {
		return "", false
	}
	// 简单取模
	idx := 0
	for _, c := range key {
		idx += int(c)
	}
	return h.nodes[idx%len(h.nodes)], true
}

func (h *mockHashRing) GetNodes(key string, count int) []string {
	if len(h.nodes) == 0 {
		return nil
	}
	result := make([]string, 0, count)
	start := 0
	for _, c := range key {
		start += int(c)
	}
	for i := 0; i < count && i < len(h.nodes); i++ {
		result = append(result, h.nodes[(start+i)%len(h.nodes)])
	}
	return result
}

// mockClusterProvider 模拟集群提供者
type mockClusterProvider struct {
	hashRing *mockHashRing
	state    *mockState
	nodes    []ClusterNode
	leader   bool
	localID  string
}

func newMockCluster(nodeIDs []string) *mockClusterProvider {
	nodes := make([]ClusterNode, len(nodeIDs))
	for i, id := range nodeIDs {
		nodes[i] = ClusterNode{ID: id, Address: "localhost:" + id, Status: "online"}
	}
	return &mockClusterProvider{
		hashRing: &mockHashRing{nodes: nodeIDs},
		state:    newMockState(),
		nodes:    nodes,
		leader:   true,
		localID:  nodeIDs[0],
	}
}

func (c *mockClusterProvider) GetNodeForKey(key string) (string, bool) {
	return c.hashRing.GetNode(key)
}

func (c *mockClusterProvider) GetNodesForKey(key string, count int) []string {
	return c.hashRing.GetNodes(key, count)
}

func (c *mockClusterProvider) ListNodes() []ClusterNode {
	return c.nodes
}

func (c *mockClusterProvider) GetState() StateProvider {
	return c.state
}

func (c *mockClusterProvider) IsLeader() bool {
	return c.leader
}

func (c *mockClusterProvider) GetLocalNodeID() string {
	return c.localID
}

// mockLoadReporter 模拟负载报告
type mockLoadReporter struct {
	depth    int
	active   int
	capacity int
}

func (r *mockLoadReporter) QueueDepth() int   { return r.depth }
func (r *mockLoadReporter) ActiveAgents() int { return r.active }
func (r *mockLoadReporter) MaxCapacity() int  { return r.capacity }

// ===== 测试用例 =====

func TestDistributedDeployer_DeployDistributed(t *testing.T) {
	registry := NewTemplateRegistry()
	tmpl := &AgentTemplate{
		ID:           "tmpl-1",
		Name:         "Test Agent",
		Version:      "1.0.0",
		Author:       "tester",
		SystemPrompt: "You are a test agent.",
		Category:     "chat",
	}
	if err := registry.Register(tmpl); err != nil {
		t.Fatalf("注册模板失败: %v", err)
	}

	deployer := NewDeployer(registry)
	cluster := newMockCluster([]string{"node-1", "node-2", "node-3"})

	dd := NewDistributedDeployer(deployer, cluster, DistributedDeployerConfig{
		DefaultReplicas: 2,
		StateTTL:        time.Hour,
	})

	record, err := dd.DeployDistributed(DeployConfig{TemplateID: "tmpl-1"}, 2)
	if err != nil {
		t.Fatalf("分布式部署失败: %v", err)
	}

	if record.TemplateID != "tmpl-1" {
		t.Errorf("期望 TemplateID=tmpl-1, 得到 %s", record.TemplateID)
	}
	if record.Status != DeployStatusDeploying {
		t.Errorf("期望状态 deploying, 得到 %s", record.Status)
	}
	if len(record.ReplicaNodes) != 2 {
		t.Errorf("期望 2 个副本节点, 得到 %d", len(record.ReplicaNodes))
	}
	if record.AgentConfig == nil {
		t.Error("AgentConfig 不应为空")
	}

	// 验证状态已同步到分布式存储
	stateKey := "deploy:{" + record.DeploymentID + "}"
	val, ok := cluster.state.Get(stateKey)
	if !ok {
		t.Fatal("部署状态未同步到分布式存储")
	}
	var synced DeploymentRecord
	if err := json.Unmarshal([]byte(val), &synced); err != nil {
		t.Fatalf("反序列化部署记录失败: %v", err)
	}
	if synced.DeploymentID != record.DeploymentID {
		t.Errorf("同步的 DeploymentID 不匹配")
	}
}

func TestDistributedDeployer_DeployNotFound(t *testing.T) {
	registry := NewTemplateRegistry()
	deployer := NewDeployer(registry)
	cluster := newMockCluster([]string{"node-1"})

	dd := NewDistributedDeployer(deployer, cluster, DistributedDeployerConfig{})

	_, err := dd.DeployDistributed(DeployConfig{TemplateID: "nonexistent"}, 1)
	if err == nil {
		t.Fatal("期望部署不存在的模板返回错误")
	}
}

func TestDistributedDeployer_LoadAwareSelection(t *testing.T) {
	registry := NewTemplateRegistry()
	tmpl := &AgentTemplate{
		ID:           "tmpl-load",
		Name:         "Load Test",
		Version:      "1.0.0",
		Author:       "tester",
		SystemPrompt: "test",
		Category:     "chat",
	}
	_ = registry.Register(tmpl)

	deployer := NewDeployer(registry)
	cluster := newMockCluster([]string{"node-a", "node-b", "node-c"})

	dd := NewDistributedDeployer(deployer, cluster, DistributedDeployerConfig{
		DefaultReplicas: 1,
		MaxQueueDepth:   50,
		LoadWeight:      0.9,
	})

	// node-a 高负载，node-b 低负载，node-c 中等
	dd.RegisterLoadReporter("node-a", &mockLoadReporter{depth: 90, active: 9, capacity: 100})
	dd.RegisterLoadReporter("node-b", &mockLoadReporter{depth: 5, active: 1, capacity: 100})
	dd.RegisterLoadReporter("node-c", &mockLoadReporter{depth: 40, active: 4, capacity: 100})

	record, err := dd.DeployDistributed(DeployConfig{TemplateID: "tmpl-load"}, 1)
	if err != nil {
		t.Fatalf("部署失败: %v", err)
	}

	// 低负载节点应该被优先选择
	if record.TargetNode != "node-b" {
		t.Errorf("期望选择低负载节点 node-b, 得到 %s", record.TargetNode)
	}
}

func TestDistributedDeployer_UpdateStatus(t *testing.T) {
	registry := NewTemplateRegistry()
	tmpl := &AgentTemplate{
		ID:           "tmpl-status",
		Name:         "Status Test",
		Version:      "1.0.0",
		Author:       "tester",
		SystemPrompt: "test",
		Category:     "chat",
	}
	_ = registry.Register(tmpl)

	deployer := NewDeployer(registry)
	cluster := newMockCluster([]string{"node-1"})
	dd := NewDistributedDeployer(deployer, cluster, DistributedDeployerConfig{})

	record, _ := dd.DeployDistributed(DeployConfig{TemplateID: "tmpl-status"}, 1)

	// 更新为运行中
	err := dd.UpdateDeploymentStatus(record.DeploymentID, DeployStatusRunning, "")
	if err != nil {
		t.Fatalf("更新状态失败: %v", err)
	}

	got, ok := dd.GetDeployment(record.DeploymentID)
	if !ok {
		t.Fatal("获取部署记录失败")
	}
	if got.Status != DeployStatusRunning {
		t.Errorf("期望状态 running, 得到 %s", got.Status)
	}

	// 更新为失败
	err = dd.UpdateDeploymentStatus(record.DeploymentID, DeployStatusFailed, "OOM killed")
	if err != nil {
		t.Fatalf("更新失败状态出错: %v", err)
	}
	got, _ = dd.GetDeployment(record.DeploymentID)
	if got.Status != DeployStatusFailed {
		t.Errorf("期望状态 failed, 得到 %s", got.Status)
	}
	if got.Error != "OOM killed" {
		t.Errorf("期望错误信息 'OOM killed', 得到 %q", got.Error)
	}
}

func TestDistributedDeployer_StopDeployment(t *testing.T) {
	registry := NewTemplateRegistry()
	tmpl := &AgentTemplate{
		ID:           "tmpl-stop",
		Name:         "Stop Test",
		Version:      "1.0.0",
		Author:       "tester",
		SystemPrompt: "test",
		Category:     "chat",
	}
	_ = registry.Register(tmpl)

	deployer := NewDeployer(registry)
	cluster := newMockCluster([]string{"node-1"})
	dd := NewDistributedDeployer(deployer, cluster, DistributedDeployerConfig{})

	record, _ := dd.DeployDistributed(DeployConfig{TemplateID: "tmpl-stop"}, 1)

	err := dd.StopDeployment(record.DeploymentID)
	if err != nil {
		t.Fatalf("停止部署失败: %v", err)
	}

	got, _ := dd.GetDeployment(record.DeploymentID)
	if got.Status != DeployStatusStopped {
		t.Errorf("期望状态 stopped, 得到 %s", got.Status)
	}
}

func TestDistributedDeployer_ListDeployments(t *testing.T) {
	registry := NewTemplateRegistry()
	for _, id := range []string{"tmpl-a", "tmpl-b", "tmpl-c"} {
		_ = registry.Register(&AgentTemplate{
			ID:           id,
			Name:         id,
			Version:      "1.0.0",
			Author:       "tester",
			SystemPrompt: "test",
			Category:     "chat",
		})
	}

	deployer := NewDeployer(registry)
	cluster := newMockCluster([]string{"node-1", "node-2"})
	dd := NewDistributedDeployer(deployer, cluster, DistributedDeployerConfig{})

	for _, id := range []string{"tmpl-a", "tmpl-b", "tmpl-c"} {
		_, err := dd.DeployDistributed(DeployConfig{TemplateID: id}, 1)
		if err != nil {
			t.Fatalf("部署 %s 失败: %v", id, err)
		}
	}

	deployments := dd.ListDeployments()
	if len(deployments) != 3 {
		t.Errorf("期望 3 个部署, 得到 %d", len(deployments))
	}
}

func TestDistributedDeployer_RecoverFromState(t *testing.T) {
	registry := NewTemplateRegistry()
	tmpl := &AgentTemplate{
		ID:           "tmpl-recover",
		Name:         "Recover Test",
		Version:      "1.0.0",
		Author:       "tester",
		SystemPrompt: "test",
		Category:     "chat",
	}
	_ = registry.Register(tmpl)

	deployer := NewDeployer(registry)
	cluster := newMockCluster([]string{"node-1"})

	// 第一个部署器创建部署
	dd1 := NewDistributedDeployer(deployer, cluster, DistributedDeployerConfig{})
	record, _ := dd1.DeployDistributed(DeployConfig{TemplateID: "tmpl-recover"}, 1)

	// 模拟节点重启：创建新的部署器（共享同一个 state）
	dd2 := NewDistributedDeployer(deployer, cluster, DistributedDeployerConfig{})

	// 恢复前应为空
	if len(dd2.ListDeployments()) != 0 {
		t.Fatal("新部署器不应有部署记录")
	}

	// 从状态恢复
	recovered := dd2.RecoverFromState()
	if recovered != 1 {
		t.Errorf("期望恢复 1 条记录, 得到 %d", recovered)
	}

	got, ok := dd2.GetDeployment(record.DeploymentID)
	if !ok {
		t.Fatal("恢复后应能获取部署记录")
	}
	if got.TemplateID != "tmpl-recover" {
		t.Errorf("恢复的记录 TemplateID 不匹配")
	}
}

func TestDistributedDeployer_NoNodes(t *testing.T) {
	registry := NewTemplateRegistry()
	tmpl := &AgentTemplate{
		ID:           "tmpl-nonodes",
		Name:         "No Nodes",
		Version:      "1.0.0",
		Author:       "tester",
		SystemPrompt: "test",
		Category:     "chat",
	}
	_ = registry.Register(tmpl)

	deployer := NewDeployer(registry)
	// 空集群（无节点）
	cluster := &mockClusterProvider{
		hashRing: &mockHashRing{nodes: []string{}},
		state:    newMockState(),
		nodes:    []ClusterNode{},
		localID:  "node-0",
	}

	dd := NewDistributedDeployer(deployer, cluster, DistributedDeployerConfig{})

	_, err := dd.DeployDistributed(DeployConfig{TemplateID: "tmpl-nonodes"}, 1)
	if err == nil {
		t.Fatal("无可用节点时应返回错误")
	}
}

func TestClusterAdapter(t *testing.T) {
	ring := &mockHashRing{nodes: []string{"n1", "n2", "n3"}}
	state := newMockState()

	adapter := NewClusterAdapter(ClusterAdapterConfig{
		HashRing:    ring,
		State:       state,
		LocalNodeID: "n1",
		IsLeaderFn:  func() bool { return true },
		ListNodesFn: func() []ClusterNode {
			return []ClusterNode{
				{ID: "n1", Status: "online"},
				{ID: "n2", Status: "online"},
			}
		},
	})

	// 测试 GetNodeForKey
	node, ok := adapter.GetNodeForKey("test-key")
	if !ok {
		t.Fatal("GetNodeForKey 应返回节点")
	}
	if node == "" {
		t.Error("节点 ID 不应为空")
	}

	// 测试 GetNodesForKey
	nodes := adapter.GetNodesForKey("test-key", 2)
	if len(nodes) != 2 {
		t.Errorf("期望 2 个节点, 得到 %d", len(nodes))
	}

	// 测试 ListNodes
	allNodes := adapter.ListNodes()
	if len(allNodes) != 2 {
		t.Errorf("期望 2 个节点, 得到 %d", len(allNodes))
	}

	// 测试 IsLeader
	if !adapter.IsLeader() {
		t.Error("应为领导者")
	}

	// 测试 GetLocalNodeID
	if adapter.GetLocalNodeID() != "n1" {
		t.Errorf("期望本地节点 n1, 得到 %s", adapter.GetLocalNodeID())
	}

	// 测试 GetState
	if adapter.GetState() == nil {
		t.Error("GetState 不应返回 nil")
	}
}

func TestDistributedDeployer_DefaultConfig(t *testing.T) {
	registry := NewTemplateRegistry()
	deployer := NewDeployer(registry)
	cluster := newMockCluster([]string{"node-1"})

	// 零值配置应使用默认值
	dd := NewDistributedDeployer(deployer, cluster, DistributedDeployerConfig{})

	if dd.config.DefaultReplicas != 1 {
		t.Errorf("默认副本数应为 1, 得到 %d", dd.config.DefaultReplicas)
	}
	if dd.config.StateTTL != 24*time.Hour {
		t.Errorf("默认 TTL 应为 24h, 得到 %v", dd.config.StateTTL)
	}
	if dd.config.MaxQueueDepth != 100 {
		t.Errorf("默认最大队列深度应为 100, 得到 %d", dd.config.MaxQueueDepth)
	}
	if dd.config.LoadWeight != 0.7 {
		t.Errorf("默认负载权重应为 0.7, 得到 %f", dd.config.LoadWeight)
	}
}

func TestDistributedDeployer_OverloadPenalty(t *testing.T) {
	registry := NewTemplateRegistry()
	tmpl := &AgentTemplate{
		ID:           "tmpl-overload",
		Name:         "Overload Test",
		Version:      "1.0.0",
		Author:       "tester",
		SystemPrompt: "test",
		Category:     "chat",
	}
	_ = registry.Register(tmpl)

	deployer := NewDeployer(registry)
	cluster := newMockCluster([]string{"node-x", "node-y"})

	dd := NewDistributedDeployer(deployer, cluster, DistributedDeployerConfig{
		MaxQueueDepth: 50,
		LoadWeight:    0.5,
	})

	// node-x 超载（超过 MaxQueueDepth），node-y 正常
	dd.RegisterLoadReporter("node-x", &mockLoadReporter{depth: 200, active: 20, capacity: 100})
	dd.RegisterLoadReporter("node-y", &mockLoadReporter{depth: 10, active: 1, capacity: 100})

	record, err := dd.DeployDistributed(DeployConfig{TemplateID: "tmpl-overload"}, 1)
	if err != nil {
		t.Fatalf("部署失败: %v", err)
	}

	// 超载节点应被跳过
	if record.TargetNode != "node-y" {
		t.Errorf("期望选择未超载的 node-y, 得到 %s", record.TargetNode)
	}
}

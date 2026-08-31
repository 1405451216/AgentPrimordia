// Phase 2.1: 集群×市场 — 分布式模板部署
//
// 将 marketplace.Deployer 与 cluster.ConsistentHash / DistributedState 集成，
// 实现跨节点模板部署：
//   - 基于一致性哈希选择最优部署节点
//   - 通过 DistributedState 在节点间同步部署状态
//   - 负载感知：根据节点队列深度选择部署目标

package marketplace

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"
)

// ===== 集群抽象接口（解耦 cluster 包，避免循环依赖） =====

// ClusterNode 集群节点信息（由 cluster.NodeInfo 适配）
type ClusterNode struct {
	ID           string            `json:"id"`
	Address      string            `json:"address"`
	Status       string            `json:"status"`
	Capabilities []string          `json:"capabilities,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// ClusterProvider 集群能力提供者接口
// 由 cluster.ClusterManager 实现，marketplace 通过此接口访问集群
type ClusterProvider interface {
	// GetNode 根据一致性哈希获取负责指定 key 的节点 ID
	GetNodeForKey(key string) (string, bool)
	// GetNodesForKey 获取负责指定 key 的前 N 个节点（副本选择）
	GetNodesForKey(key string, count int) []string
	// ListNodes 列出所有在线节点
	ListNodes() []ClusterNode
	// GetState 获取分布式状态存储
	GetState() StateProvider
	// IsLeader 当前节点是否为领导者
	IsLeader() bool
	// GetLocalNodeID 获取本地节点 ID
	GetLocalNodeID() string
}

// StateProvider 分布式状态存储接口
type StateProvider interface {
	Set(key, value string, ttl time.Duration)
	Get(key string) (string, bool)
	Delete(key string) bool
	Keys() []string
}

// LoadReporter 节点负载报告接口
type LoadReporter interface {
	// QueueDepth 返回当前 Pool 队列深度
	QueueDepth() int
	// ActiveAgents 返回活跃 Agent 数
	ActiveAgents() int
	// MaxCapacity 返回最大容量
	MaxCapacity() int
}

// ===== 分布式部署状态 =====

// DeployStatus 部署状态
type DeployStatus string

const (
	DeployStatusPending   DeployStatus = "pending"
	DeployStatusDeploying DeployStatus = "deploying"
	DeployStatusRunning   DeployStatus = "running"
	DeployStatusFailed    DeployStatus = "failed"
	DeployStatusStopped   DeployStatus = "stopped"
)

// DeploymentRecord 部署记录
type DeploymentRecord struct {
	// DeploymentID 部署唯一标识
	DeploymentID string `json:"deployment_id"`
	// TemplateID 模板 ID
	TemplateID string `json:"template_id"`
	// TargetNode 目标节点 ID
	TargetNode string `json:"target_node"`
	// Status 部署状态
	Status DeployStatus `json:"status"`
	// AgentConfig 生成的 Agent 配置
	AgentConfig json.RawMessage `json:"agent_config,omitempty"`
	// CreatedAt 创建时间
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt 更新时间
	UpdatedAt time.Time `json:"updated_at"`
	// Error 错误信息（失败时）
	Error string `json:"error,omitempty"`
	// Replicas 副本数
	Replicas int `json:"replicas"`
	// ReplicaNodes 副本所在节点
	ReplicaNodes []string `json:"replica_nodes,omitempty"`
}

// ===== 分布式部署器 =====

// DistributedDeployerConfig 分布式部署器配置
type DistributedDeployerConfig struct {
	// DefaultReplicas 默认副本数（默认 1）
	DefaultReplicas int
	// StateTTL 部署状态 TTL（默认 24h）
	StateTTL time.Duration
	// MaxQueueDepth 最大队列深度阈值（超过则跳过该节点）
	MaxQueueDepth int
	// LoadWeight 负载权重（0-1，越高越倾向选低负载节点）
	LoadWeight float64
}

// DistributedDeployer 分布式模板部署器
//
// 在 Deployer 基础上增加集群感知能力：
//   - 一致性哈希选择目标节点
//   - 负载感知节点选择
//   - 部署状态跨节点同步
type DistributedDeployer struct {
	deployer *Deployer
	cluster  ClusterProvider
	loaders  map[string]LoadReporter // nodeID -> 负载报告
	config   DistributedDeployerConfig
	logger   *slog.Logger

	mu          sync.RWMutex
	deployments map[string]*DeploymentRecord // deploymentID -> record
}

// NewDistributedDeployer 创建分布式部署器
func NewDistributedDeployer(deployer *Deployer, cluster ClusterProvider, cfg DistributedDeployerConfig) *DistributedDeployer {
	if cfg.DefaultReplicas <= 0 {
		cfg.DefaultReplicas = 1
	}
	if cfg.StateTTL <= 0 {
		cfg.StateTTL = 24 * time.Hour
	}
	if cfg.MaxQueueDepth <= 0 {
		cfg.MaxQueueDepth = 100
	}
	if cfg.LoadWeight <= 0 || cfg.LoadWeight > 1 {
		cfg.LoadWeight = 0.7
	}

	return &DistributedDeployer{
		deployer:    deployer,
		cluster:     cluster,
		loaders:     make(map[string]LoadReporter),
		config:      cfg,
		logger:      slog.Default(),
		deployments: make(map[string]*DeploymentRecord),
	}
}

// WithLogger 设置日志器
func (dd *DistributedDeployer) WithLogger(logger *slog.Logger) *DistributedDeployer {
	dd.logger = logger
	return dd
}

// RegisterLoadReporter 注册节点负载报告器
func (dd *DistributedDeployer) RegisterLoadReporter(nodeID string, reporter LoadReporter) {
	dd.mu.Lock()
	defer dd.mu.Unlock()
	dd.loaders[nodeID] = reporter
}

// UnregisterLoadReporter 注销节点负载报告器
func (dd *DistributedDeployer) UnregisterLoadReporter(nodeID string) {
	dd.mu.Lock()
	defer dd.mu.Unlock()
	delete(dd.loaders, nodeID)
}

// DeployDistributed 分布式部署模板
//
// 流程：
//  1. 调用本地 Deployer 生成 Agent 配置
//  2. 基于一致性哈希 + 负载感知选择目标节点
//  3. 将部署状态写入 DistributedState
//  4. 返回部署记录
func (dd *DistributedDeployer) DeployDistributed(cfg DeployConfig, replicas int) (*DeploymentRecord, error) {
	if replicas <= 0 {
		replicas = dd.config.DefaultReplicas
	}

	// 1. 生成本地部署配置
	result, err := dd.deployer.Deploy(cfg)
	if err != nil {
		return nil, fmt.Errorf("distributed_deploy: local deploy failed: %w", err)
	}
	if !result.Success {
		return nil, fmt.Errorf("distributed_deploy: %s", result.Message)
	}

	// 2. 选择目标节点
	targetNodes := dd.selectNodes(cfg.TemplateID, replicas)
	if len(targetNodes) == 0 {
		return nil, fmt.Errorf("distributed_deploy: no available nodes for deployment")
	}

	// 3. 创建部署记录
	now := time.Now()
	deploymentID := fmt.Sprintf("deploy-%s-%d", cfg.TemplateID, now.UnixNano())
	record := &DeploymentRecord{
		DeploymentID: deploymentID,
		TemplateID:   cfg.TemplateID,
		TargetNode:   targetNodes[0],
		Status:       DeployStatusDeploying,
		AgentConfig:  result.AgentConfig,
		CreatedAt:    now,
		UpdatedAt:    now,
		Replicas:     replicas,
		ReplicaNodes: targetNodes,
	}

	// 4. 保存本地记录
	dd.mu.Lock()
	dd.deployments[deploymentID] = record
	dd.mu.Unlock()

	// 5. 同步到分布式状态
	dd.syncDeploymentState(record)

	dd.logger.Info("分布式部署完成",
		"deployment_id", deploymentID,
		"template_id", cfg.TemplateID,
		"target_nodes", targetNodes,
		"replicas", replicas,
	)

	return record, nil
}

// GetDeployment 获取部署记录
func (dd *DistributedDeployer) GetDeployment(deploymentID string) (*DeploymentRecord, bool) {
	dd.mu.RLock()
	defer dd.mu.RUnlock()
	rec, ok := dd.deployments[deploymentID]
	if !ok {
		return nil, false
	}
	cp := *rec
	return &cp, true
}

// ListDeployments 列出所有部署
func (dd *DistributedDeployer) ListDeployments() []*DeploymentRecord {
	dd.mu.RLock()
	defer dd.mu.RUnlock()
	result := make([]*DeploymentRecord, 0, len(dd.deployments))
	for _, rec := range dd.deployments {
		cp := *rec
		result = append(result, &cp)
	}
	return result
}

// UpdateDeploymentStatus 更新部署状态
func (dd *DistributedDeployer) UpdateDeploymentStatus(deploymentID string, status DeployStatus, errMsg string) error {
	dd.mu.Lock()
	rec, ok := dd.deployments[deploymentID]
	if !ok {
		dd.mu.Unlock()
		return fmt.Errorf("distributed_deploy: deployment %q not found", deploymentID)
	}
	rec.Status = status
	rec.Error = errMsg
	rec.UpdatedAt = time.Now()
	dd.mu.Unlock()

	// 同步状态
	dd.syncDeploymentState(rec)
	return nil
}

// StopDeployment 停止部署
func (dd *DistributedDeployer) StopDeployment(deploymentID string) error {
	return dd.UpdateDeploymentStatus(deploymentID, DeployStatusStopped, "")
}

// RecoverFromState 从分布式状态恢复部署记录
// 节点重启后调用，从 DistributedState 中恢复之前的部署信息
func (dd *DistributedDeployer) RecoverFromState() int {
	state := dd.cluster.GetState()
	if state == nil {
		return 0
	}

	recovered := 0
	keys := state.Keys()
	for _, key := range keys {
		// 只处理部署状态键
		if len(key) < 8 || key[:8] != "deploy:{" {
			continue
		}
		val, ok := state.Get(key)
		if !ok {
			continue
		}

		var rec DeploymentRecord
		if err := json.Unmarshal([]byte(val), &rec); err != nil {
			dd.logger.Warn("恢复部署记录失败", "key", key, "error", err)
			continue
		}

		dd.mu.Lock()
		if _, exists := dd.deployments[rec.DeploymentID]; !exists {
			cp := rec
			dd.deployments[rec.DeploymentID] = &cp
			recovered++
		}
		dd.mu.Unlock()
	}

	if recovered > 0 {
		dd.logger.Info("从分布式状态恢复部署记录", "count", recovered)
	}
	return recovered
}

// ===== 内部方法 =====

// selectNodes 基于一致性哈希 + 负载感知选择部署节点
func (dd *DistributedDeployer) selectNodes(templateID string, count int) []string {
	// 先通过一致性哈希获取候选节点
	candidates := dd.cluster.GetNodesForKey(templateID, count*3) // 多取一些候选
	if len(candidates) == 0 {
		// 回退：使用所有在线节点
		nodes := dd.cluster.ListNodes()
		for _, n := range nodes {
			if n.Status == "online" {
				candidates = append(candidates, n.ID)
			}
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	// 负载感知排序
	scored := dd.scoreNodes(candidates)

	// 取前 count 个
	if len(scored) > count {
		scored = scored[:count]
	}

	result := make([]string, len(scored))
	for i, s := range scored {
		result[i] = s.nodeID
	}
	return result
}

// nodeScore 节点评分
type nodeScore struct {
	nodeID string
	score  float64 // 越低越好
}

// scoreNodes 对候选节点进行负载评分
func (dd *DistributedDeployer) scoreNodes(nodeIDs []string) []nodeScore {
	dd.mu.RLock()
	loaders := dd.loaders
	dd.mu.RUnlock()

	scores := make([]nodeScore, 0, len(nodeIDs))
	for _, id := range nodeIDs {
		score := 0.0
		if reporter, ok := loaders[id]; ok {
			depth := reporter.QueueDepth()
			capacity := reporter.MaxCapacity()
			if capacity > 0 {
				// 负载率 = 队列深度 / 最大容量
				loadRatio := float64(depth) / float64(capacity)
				score = loadRatio * dd.config.LoadWeight
			}
			// 超过阈值的节点加大惩罚
			if depth > dd.config.MaxQueueDepth {
				score += 10.0
			}
		}
		scores = append(scores, nodeScore{nodeID: id, score: score})
	}

	// 按分数升序排列（低分优先）
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].score < scores[j].score
	})

	return scores
}

// syncDeploymentState 将部署记录同步到分布式状态
func (dd *DistributedDeployer) syncDeploymentState(rec *DeploymentRecord) {
	state := dd.cluster.GetState()
	if state == nil {
		return
	}

	data, err := json.Marshal(rec)
	if err != nil {
		dd.logger.Warn("序列化部署记录失败", "deployment_id", rec.DeploymentID, "error", err)
		return
	}

	key := fmt.Sprintf("deploy:{%s}", rec.DeploymentID)
	state.Set(key, string(data), dd.config.StateTTL)
}

// ===== 集群适配器 =====

// ClusterAdapter 将 cluster.ClusterManager 适配为 ClusterProvider
// 使用方式：
//
//	mgr := cluster.NewClusterManager(cfg)
//	provider := marketplace.NewClusterAdapter(mgr)
//	dd := marketplace.NewDistributedDeployer(deployer, provider, cfg)
type ClusterAdapter struct {
	hashRing    HashRingProvider
	state       StateProvider
	localNodeID string
	isLeaderFn  func() bool
	listNodesFn func() []ClusterNode
}

// HashRingProvider 哈希环接口
type HashRingProvider interface {
	GetNode(key string) (string, bool)
	GetNodes(key string, count int) []string
}

// ClusterAdapterConfig 适配器配置
type ClusterAdapterConfig struct {
	HashRing    HashRingProvider
	State       StateProvider
	LocalNodeID string
	IsLeaderFn  func() bool
	ListNodesFn func() []ClusterNode
}

// NewClusterAdapter 创建集群适配器
func NewClusterAdapter(cfg ClusterAdapterConfig) *ClusterAdapter {
	return &ClusterAdapter{
		hashRing:    cfg.HashRing,
		state:       cfg.State,
		localNodeID: cfg.LocalNodeID,
		isLeaderFn:  cfg.IsLeaderFn,
		listNodesFn: cfg.ListNodesFn,
	}
}

func (a *ClusterAdapter) GetNodeForKey(key string) (string, bool) {
	if a.hashRing == nil {
		return "", false
	}
	return a.hashRing.GetNode(key)
}

func (a *ClusterAdapter) GetNodesForKey(key string, count int) []string {
	if a.hashRing == nil {
		return nil
	}
	return a.hashRing.GetNodes(key, count)
}

func (a *ClusterAdapter) ListNodes() []ClusterNode {
	if a.listNodesFn == nil {
		return nil
	}
	return a.listNodesFn()
}

func (a *ClusterAdapter) GetState() StateProvider {
	return a.state
}

func (a *ClusterAdapter) IsLeader() bool {
	if a.isLeaderFn == nil {
		return false
	}
	return a.isLeaderFn()
}

func (a *ClusterAdapter) GetLocalNodeID() string {
	return a.localNodeID
}

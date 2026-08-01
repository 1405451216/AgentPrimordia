// Package cluster 提供分布式集群协调能力。
//
// 本包在现有 discovery（服务发现）和 bus（消息总线）的基础上，
// 增加跨节点 Agent 协作所需的：
//   - ClusterManager：集群管理器，管理节点加入/离开/心跳/选举
//   - NodeInfo：节点信息（ID、地址、角色、状态）
//   - ConsistentHash：一致性哈希分片，支持节点动态增减
//   - LeaderElection：简化的领导者选举（基于租约）
//   - DistributedState：分布式 KV 状态存储
//
// 使用方式：
//
//	mgr := cluster.NewClusterManager(cluster.ClusterConfig{
//	    NodeID:     "node-1",
//	    ListenAddr: ":8080",
//	    Discovery:  discovery.NewLocalDiscovery(),
//	})
//	mgr.Start(ctx)
package cluster

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"agentprimordia/internal/agent/discovery"
)

// NodeRole 节点角色
type NodeRole string

const (
	RoleFollower NodeRole = "follower"
	RoleLeader   NodeRole = "leader"
	RoleCandidate NodeRole = "candidate"
)

// NodeStatus 节点状态
type NodeStatus string

const (
	StatusOnline  NodeStatus = "online"
	StatusOffline NodeStatus = "offline"
	StatusLeaving NodeStatus = "leaving"
)

// NodeInfo 节点信息
type NodeInfo struct {
	ID          string            `json:"id"`
	Address     string            `json:"address"`
	Role        NodeRole          `json:"role"`
	Status      NodeStatus        `json:"status"`
	Capabilities []string         `json:"capabilities,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	JoinTime    time.Time         `json:"join_time"`
	LastSeen    time.Time         `json:"last_seen"`
}

// electionKey 共享领导者租约在 KV 中的键
const electionKey = "_leader_lease"

// ClusterConfig 集群配置
type ClusterConfig struct {
	// NodeID 当前节点 ID
	NodeID string
	// ListenAddr 监听地址
	ListenAddr string
	// Discovery 服务发现接口
	Discovery discovery.Discovery
	// StateStore 共享 KV 后端（可选）。设置后领导者选举基于共享租约跨节点收敛；
	// 为空时选举退化为纯本地行为（仅单节点场景可用）。
	StateStore KVStore
	// HeartbeatInterval 心跳间隔
	HeartbeatInterval time.Duration
	// HeartbeatTimeout 心跳超时（超过此时间未收到心跳视为离线）
	HeartbeatTimeout time.Duration
	// ElectionTimeout 选举超时
	ElectionTimeout time.Duration
}

// ClusterConfigWithDefaults 填充默认值
func ClusterConfigWithDefaults(cfg ClusterConfig) ClusterConfig {
	if cfg.HeartbeatInterval == 0 {
		cfg.HeartbeatInterval = 5 * time.Second
	}
	if cfg.HeartbeatTimeout == 0 {
		cfg.HeartbeatTimeout = 15 * time.Second
	}
	if cfg.ElectionTimeout == 0 {
		cfg.ElectionTimeout = 10 * time.Second
	}
	return cfg
}

// ClusterManager 集群管理器
type ClusterManager struct {
	config  ClusterConfig
	logger *slog.Logger

	mu      sync.RWMutex
	localNode *NodeInfo
	nodes   map[string]*NodeInfo // 节点缓存

	// 领导者选举
	leaderID  atomic.Value // string
	term      atomic.Int64
	votedFor  atomic.Value // string
	role      atomic.Value // NodeRole

	// 一致性哈希
	hashRing  *ConsistentHash

	// 分布式状态
	state     *DistributedState

	// 运行控制
	stopCh    chan struct{}
	running   atomic.Bool
}

// NewClusterManager 创建集群管理器
func NewClusterManager(cfg ClusterConfig) *ClusterManager {
	cfg = ClusterConfigWithDefaults(cfg)

	node := &NodeInfo{
		ID:       cfg.NodeID,
		Address:  cfg.ListenAddr,
		Role:     RoleFollower,
		Status:   StatusOnline,
		JoinTime: time.Now(),
		LastSeen: time.Now(),
	}

	mgr := &ClusterManager{
		config:    cfg,
		logger:    slog.Default(),
		localNode: node,
		nodes:     make(map[string]*NodeInfo),
		hashRing:  NewConsistentHash(64), // 每个节点 64 个虚拟节点
		state:     NewDistributedState(),
		stopCh:    make(chan struct{}),
	}

	mgr.leaderID.Store("")
	mgr.votedFor.Store("")
	mgr.role.Store(RoleFollower)

	// 将本地节点加入哈希环
	mgr.hashRing.AddNode(cfg.NodeID)

	return mgr
}

// WithLogger 设置日志器
func (m *ClusterManager) WithLogger(logger *slog.Logger) *ClusterManager {
	m.logger = logger
	return m
}

// Start 启动集群管理器
func (m *ClusterManager) Start(ctx context.Context) error {
	m.running.Store(true)

	// 注册本地节点到发现服务
	if m.config.Discovery != nil {
		err := m.config.Discovery.Register(ctx, &discovery.AgentInfo{
			ID:       m.config.NodeID,
			Name:     m.config.NodeID,
			Address:  m.config.ListenAddr,
			LastSeen: time.Now(),
			Metadata: map[string]string{
				"role":   string(RoleFollower),
				"status": string(StatusOnline),
			},
		})
		if err != nil {
			return fmt.Errorf("cluster: register node to discovery: %w", err)
		}
		m.logger.Info("节点注册成功", "node_id", m.config.NodeID, "addr", m.config.ListenAddr)
	}

	// 启动心跳 goroutine
	go m.heartbeatLoop(ctx)

	// 启动节点同步 goroutine
	go m.syncNodesLoop(ctx)

	// 启动选举 goroutine
	go m.electionLoop(ctx)

	m.logger.Info("集群管理器启动", "node_id", m.config.NodeID)
	return nil
}

// Stop 停止集群管理器
func (m *ClusterManager) Stop(ctx context.Context) error {
	if !m.running.Load() {
		return nil
	}
	m.running.Store(false)
	close(m.stopCh)

	// 从发现服务注销
	if m.config.Discovery != nil {
		_ = m.config.Discovery.Unregister(ctx, m.config.NodeID)
	}

	// 从哈希环移除
	m.hashRing.RemoveNode(m.config.NodeID)

	m.logger.Info("集群管理器停止", "node_id", m.config.NodeID)
	return nil
}

// GetLocalNode 获取本地节点信息
func (m *ClusterManager) GetLocalNode() *NodeInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cp := *m.localNode
	return &cp
}

// ListNodes 列出所有已知节点
func (m *ClusterManager) ListNodes() []NodeInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]NodeInfo, 0, len(m.nodes)+1)
	result = append(result, *m.localNode)
	for _, node := range m.nodes {
		result = append(result, *node)
	}
	return result
}

// GetNode 获取指定节点信息
func (m *ClusterManager) GetNode(nodeID string) (*NodeInfo, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if nodeID == m.config.NodeID {
		cp := *m.localNode
		return &cp, true
	}

	node, exists := m.nodes[nodeID]
	if !exists {
		return nil, false
	}
	cp := *node
	return &cp, true
}

// GetLeader 获取当前领导者 ID
func (m *ClusterManager) GetLeader() string {
	return m.leaderID.Load().(string)
}

// IsLeader 判断当前节点是否为领导者
func (m *ClusterManager) IsLeader() bool {
	return m.GetLeader() == m.config.NodeID
}

// GetRole 获取当前节点角色
func (m *ClusterManager) GetRole() NodeRole {
	return m.role.Load().(NodeRole)
}

// GetState 获取分布式状态存储
func (m *ClusterManager) GetState() *DistributedState {
	return m.state
}

// GetHashRing 获取一致性哈希环
func (m *ClusterManager) GetHashRing() *ConsistentHash {
	return m.hashRing
}

// heartbeatLoop 心跳循环
func (m *ClusterManager) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(m.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.sendHeartbeat(ctx)
		}
	}
}

// sendHeartbeat 发送心跳
func (m *ClusterManager) sendHeartbeat(ctx context.Context) {
	if m.config.Discovery == nil {
		return
	}
	if err := m.config.Discovery.Heartbeat(ctx, m.config.NodeID); err != nil {
		m.logger.Warn("心跳发送失败", "error", err)
	}

	// 更新本地节点的 LastSeen
	m.mu.Lock()
	m.localNode.LastSeen = time.Now()
	m.mu.Unlock()
}

// syncNodesLoop 节点同步循环
func (m *ClusterManager) syncNodesLoop(ctx context.Context) {
	ticker := time.NewTicker(m.config.HeartbeatInterval * 2)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.syncNodes(ctx)
		}
	}
}

// syncNodes 从发现服务同步节点列表
func (m *ClusterManager) syncNodes(ctx context.Context) {
	if m.config.Discovery == nil {
		return
	}

	agents, err := m.config.Discovery.ListAgents(ctx)
	if err != nil {
		m.logger.Warn("获取节点列表失败", "error", err)
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// 构建新节点集合
	newNodes := make(map[string]*NodeInfo)
	for _, agent := range agents {
		if agent.ID == m.config.NodeID {
			continue // 跳过本地节点
		}

		node := &NodeInfo{
			ID:       agent.ID,
			Address:  agent.Address,
			Role:     RoleFollower,
			Status:   StatusOnline,
			JoinTime: agent.LastSeen,
			LastSeen: agent.LastSeen,
			Metadata: agent.Metadata,
		}

		// 从角色/状态元数据恢复
		if role, ok := agent.Metadata["role"]; ok {
			node.Role = NodeRole(role)
		}
		if status, ok := agent.Metadata["status"]; ok {
			node.Status = NodeStatus(status)
		}

		newNodes[agent.ID] = node

		// 如果是新节点，添加到哈希环
		if _, exists := m.nodes[agent.ID]; !exists {
			m.hashRing.AddNode(agent.ID)
			m.logger.Info("发现新节点", "node_id", agent.ID, "addr", agent.Address)
		}
	}

	// 检查离线节点
	for id, node := range m.nodes {
		if _, exists := newNodes[id]; !exists {
			// 节点已从发现服务消失
			m.hashRing.RemoveNode(id)
			m.logger.Info("节点离线", "node_id", id)
		} else if time.Since(node.LastSeen) > m.config.HeartbeatTimeout {
			// 心跳超时
			node.Status = StatusOffline
			m.hashRing.RemoveNode(id)
			m.logger.Warn("节点心跳超时", "node_id", id, "last_seen", node.LastSeen)
		}
	}

	m.nodes = newNodes
}

// electionLoop 选举循环（简化版基于租约的选举）
func (m *ClusterManager) electionLoop(ctx context.Context) {
	ticker := time.NewTicker(m.config.ElectionTimeout)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.checkLeadership(ctx)
		}
	}
}

// checkLeadership 检查领导权
//
// 选举基于共享租约收敛：设置 StateStore（共享 KV 后端）后，各节点以
// 共享租约 _leader_lease 为权威事实源——持有租约的在线节点成为领导者，
// 其余节点跟随，从而收敛共识。StateStore 为空时退化为纯本地行为（仅单节点
// 场景可用，各节点不会互相认可）。
func (m *ClusterManager) checkLeadership(ctx context.Context) {
	leader := m.GetLeader()

	// 共享租约是权威事实源：若其他节点持有有效租约且在线，跟随它
	if lease, ok := m.leaseValue(ctx); ok && lease != "" && lease != m.config.NodeID {
		if m.nodeIsOnline(lease) {
			m.becomeFollower(lease)
			return
		}
		// 持租约节点离线：租约视为失效，允许接管（走下方选举）
		m.logger.Info("租约持有者离线，接管选举", "lease_holder", lease)
	}

	// 已有本地领导者视图
	if leader != "" {
		if leader == m.config.NodeID {
			// 当前节点是领导者，续租
			m.renewLease(ctx)
			return
		}

		// 其他节点：跟随在线领导者（收敛共识的关键路径）
		if m.nodeIsOnline(leader) {
			m.becomeFollower(leader)
			return
		}

		// 领导者离线，触发选举
		m.logger.Info("领导者离线，启动选举", "old_leader", leader)
	}

	// 尝试成为领导者
	m.startElection(ctx)
}

// nodeIsOnline 判断某节点是否在本地视图中处于在线状态
func (m *ClusterManager) nodeIsOnline(nodeID string) bool {
	if nodeID == m.config.NodeID {
		return true
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	node, exists := m.nodes[nodeID]
	return exists && node.Status == StatusOnline &&
		time.Since(node.LastSeen) <= m.config.HeartbeatTimeout
}

// leaseValue 读取共享领导者租约。StateStore 为空时读本地 state。
func (m *ClusterManager) leaseValue(ctx context.Context) (string, bool) {
	if m.config.StateStore != nil {
		value, err := m.config.StateStore.Get(ctx, electionKey)
		if err != nil || value == "" {
			return "", false
		}
		return value, true
	}
	value, ok := m.state.Get(electionKey)
	return value, ok
}

// renewLease 续租领导者租约（优先写共享 StateStore，其次本地 state）
func (m *ClusterManager) renewLease(ctx context.Context) {
	ttl := m.config.ElectionTimeout * 2
	if m.config.StateStore != nil {
		_ = m.config.StateStore.Put(ctx, electionKey, m.config.NodeID, ttl)
		return
	}
	m.state.Set(electionKey, m.config.NodeID, ttl)
}

// startElection 启动选举
func (m *ClusterManager) startElection(ctx context.Context) {
	// 增加任期
	newTerm := m.term.Add(1)

	// 投票给自己
	m.votedFor.Store(m.config.NodeID)
	m.role.Store(RoleCandidate)

	m.logger.Info("开始选举", "term", newTerm, "candidate", m.config.NodeID)

	// 简化版：如果当前节点是 ID 最小的在线节点，则成为领导者
	m.mu.RLock()
	nodes := make([]NodeInfo, 0, len(m.nodes)+1)
	nodes = append(nodes, *m.localNode)
	for _, node := range m.nodes {
		if node.Status == StatusOnline {
			nodes = append(nodes, *node)
		}
	}
	m.mu.RUnlock()

	// 找到最小 ID
	minID := m.config.NodeID
	for _, node := range nodes {
		if node.ID < minID {
			minID = node.ID
		}
	}

	// 如果当前节点是最小 ID，成为领导者
	if minID == m.config.NodeID {
		// 二次确认共享租约未被其他在线节点持有（防 checkLeadership 竞态）
		if lease, ok := m.leaseValue(ctx); ok && lease != "" && lease != m.config.NodeID && m.nodeIsOnline(lease) {
			m.becomeFollower(lease)
			return
		}
		m.becomeLeader(ctx)
	}
}

// becomeLeader 成为领导者
func (m *ClusterManager) becomeLeader(ctx context.Context) {
	m.role.Store(RoleLeader)
	m.leaderID.Store(m.config.NodeID)

	// 更新本地节点角色
	m.mu.Lock()
	m.localNode.Role = RoleLeader
	m.mu.Unlock()

	// 设置领导者租约（优先写共享 StateStore，其次本地 state）
	m.renewLease(ctx)

	m.logger.Info("成为领导者", "node_id", m.config.NodeID, "term", m.term.Load())
}

// becomeFollower 成为跟随者
func (m *ClusterManager) becomeFollower(leaderID string) {
	m.role.Store(RoleFollower)
	m.leaderID.Store(leaderID)

	// 更新本地节点角色
	m.mu.Lock()
	m.localNode.Role = RoleFollower
	m.mu.Unlock()

	m.logger.Info("成为跟随者", "leader", leaderID)
}

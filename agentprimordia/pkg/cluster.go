// Stability: Experimental — v3.0.0 新增分布式集群协调能力，API 可能随部署场景演进而调整。
package ap

import (
	"agentprimordia/internal/agent/cluster"
)

// ===== 集群管理 =====

// ClusterManager 集群管理器，管理节点加入/离开/心跳/选举
type ClusterManager = cluster.ClusterManager

// ClusterConfig 集群配置
type ClusterConfig = cluster.ClusterConfig

// ClusterNodeInfo 节点信息
type ClusterNodeInfo = cluster.NodeInfo

// ClusterNodeRole 节点角色
type ClusterNodeRole = cluster.NodeRole

// ClusterNodeStatus 节点状态
type ClusterNodeStatus = cluster.NodeStatus

const (
	// ClusterRoleFollower 跟随者角色
	ClusterRoleFollower = cluster.RoleFollower
	// ClusterRoleLeader 领导者角色
	ClusterRoleLeader = cluster.RoleLeader
	// ClusterRoleCandidate 候选者角色
	ClusterRoleCandidate = cluster.RoleCandidate

	// ClusterStatusOnline 节点在线
	ClusterStatusOnline = cluster.StatusOnline
	// ClusterStatusOffline 节点离线
	ClusterStatusOffline = cluster.StatusOffline
	// ClusterStatusLeaving 节点正在离开
	ClusterStatusLeaving = cluster.StatusLeaving
)

var (
	// NewClusterManager 创建集群管理器
	NewClusterManager = cluster.NewClusterManager
	// ClusterConfigWithDefaults 填充集群配置默认值
	ClusterConfigWithDefaults = cluster.ClusterConfigWithDefaults
)

// ===== 一致性哈希 =====

// ConsistentHash 一致性哈希环，用于将 Agent/任务分片到不同节点
type ConsistentHash = cluster.ConsistentHash

var (
	// NewConsistentHash 创建一致性哈希环
	NewConsistentHash = cluster.NewConsistentHash
)

// ===== 分布式状态 =====

// DistributedState 分布式 KV 状态存储，提供带 TTL 的状态管理
type DistributedState = cluster.DistributedState

// ClusterRemoteEntry 远程状态条目（用于节点间同步）
type ClusterRemoteEntry = cluster.RemoteEntry

var (
	// NewDistributedState 创建分布式状态存储
	NewDistributedState = cluster.NewDistributedState
)

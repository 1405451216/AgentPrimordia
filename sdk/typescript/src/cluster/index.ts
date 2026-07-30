/**
 * cluster/ — 集群协调模块
 *
 * 对齐 Go 端 internal/agent/cluster/ 包，提供：
 *   - Discovery：Agent 服务发现（KVStore 后端）
 *   - Coordination：集群消息协调（轻量实现）
 *
 * Stability: Experimental
 */

// 服务发现
export { MemKVStore, DistributedDiscovery } from './discovery.js';
export type {
  AgentInfo, KVStore, KVEvent, KVEventType, Discovery,
  DistributedDiscoveryConfig,
} from './discovery.js';

// 集群协调
export { ClusterCoordinator } from './coordination.js';
export type {
  ClusterMessage, ClusterReply, RemoteNode, CoordinationConfig,
} from './coordination.js';

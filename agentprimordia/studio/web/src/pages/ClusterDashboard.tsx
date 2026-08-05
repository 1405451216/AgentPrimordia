/**
 * Phase 3.4: Studio Web UI — Cluster Dashboard
 *
 * 节点拓扑 + 分片视图 + 领导者状态
 *
 * 加固点：
 *  - 轮询失败时保留旧数据，仅显示内联错误提示
 *  - 「上次刷新」陈旧提示
 */
import { useState, useEffect } from 'react';
import { ErrorPanel, Staleness } from '../Status';

interface NodeInfo {
  id: string;
  address: string;
  role: 'leader' | 'follower' | 'candidate';
  status: 'online' | 'offline' | 'leaving';
  capabilities: string[];
  lastSeen: string;
}

interface ClusterState {
  nodes: NodeInfo[];
  leaderId: string;
  hashRingSize: number;
  totalShards: number;
}

export function ClusterDashboard() {
  const [cluster, setCluster] = useState<ClusterState | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [refreshing, setRefreshing] = useState(false);
  const [lastUpdatedAt, setLastUpdatedAt] = useState<number | null>(null);

  const fetchCluster = async () => {
    setRefreshing(true);
    try {
      const res = await fetch('/api/v1/cluster/status');
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = await res.json();
      setCluster(data);
      setError(null);
      setLastUpdatedAt(Date.now());
    } catch (e) {
      // 保留旧数据，仅记录错误供顶部提示
      setError(e instanceof Error ? e.message : 'Unknown error');
    } finally {
      setRefreshing(false);
    }
  };

  useEffect(() => {
    fetchCluster();
    const timer = setInterval(fetchCluster, 5000);
    return () => clearInterval(timer);
  }, []);

  // 首次加载失败且无数据 → 整页错误
  if (!cluster) {
    if (error) {
      return (
        <div className="panel error">
          <h2>Cluster Dashboard</h2>
          <ErrorPanel message={`无法连接集群: ${error}`} onRetry={fetchCluster} />
        </div>
      );
    }
    return <div className="panel loading">加载集群状态...</div>;
  }

  const leader = cluster.nodes.find((n) => n.id === cluster.leaderId);
  const onlineNodes = cluster.nodes.filter((n) => n.status === 'online');

  return (
    <div className="panel cluster-dashboard">
      <h2>Cluster Dashboard</h2>

      {/* 轮询失败提示：保留旧数据，仅提示刷新失败 */}
      {error && (
        <ErrorPanel message={`刷新失败：${error}（显示上次数据）`} onRetry={fetchCluster} />
      )}

      {/* 概览 */}
      <section className="overview">
        <div className="stat">
          <span className="label">节点数</span>
          <span className="value">{onlineNodes.length}/{cluster.nodes.length}</span>
        </div>
        <div className="stat">
          <span className="label">领导者</span>
          <span className="value">{leader?.id ?? '选举中...'}</span>
        </div>
        <div className="stat">
          <span className="label">哈希环</span>
          <span className="value">{cluster.hashRingSize} vnodes</span>
        </div>
        <div className="stat">
          <span className="label">分片数</span>
          <span className="value">{cluster.totalShards}</span>
        </div>
      </section>

      {/* 节点列表 */}
      <section className="node-list">
        <h3>节点拓扑</h3>
        <table>
          <thead>
            <tr>
              <th>节点 ID</th>
              <th>地址</th>
              <th>角色</th>
              <th>状态</th>
              <th>能力</th>
              <th>最后心跳</th>
            </tr>
          </thead>
          <tbody>
            {cluster.nodes.map((node) => (
              <tr key={node.id} className={`node-${node.status}`}>
                <td className="node-id">
                  {node.id === cluster.leaderId && <span className="badge leader">👑</span>}
                  {node.id}
                </td>
                <td>{node.address}</td>
                <td><span className={`role-${node.role}`}>{node.role}</span></td>
                <td><span className={`status-${node.status}`}>{node.status}</span></td>
                <td>{node.capabilities?.join(', ') || '-'}</td>
                <td>{node.lastSeen ? new Date(node.lastSeen).toLocaleTimeString() : '-'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>

      {/* 分片视图 */}
      <section className="shard-view">
        <h3>分片分布</h3>
        <div className="shard-bar">
          {onlineNodes.map((node, i) => (
            <div
              key={node.id}
              className="shard-segment"
              style={{ flex: 1, backgroundColor: `hsl(${i * 60}, 70%, 60%)` }}
              title={`${node.id}: ${Math.round(100 / onlineNodes.length)}%`}
            >
              {node.id}
            </div>
          ))}
        </div>
      </section>

      <footer className="dashboard-footer">
        <Staleness lastUpdatedAt={lastUpdatedAt} />
        <button onClick={fetchCluster} disabled={refreshing}>
          {refreshing ? '刷新中...' : '刷新'}
        </button>
      </footer>
    </div>
  );
}

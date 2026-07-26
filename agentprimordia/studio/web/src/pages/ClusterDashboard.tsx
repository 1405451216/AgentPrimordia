/**
 * Phase 3.4: Studio Web UI — Cluster Dashboard
 *
 * 节点拓扑 + 分片视图 + 领导者状态
 */
import { useState, useEffect } from 'react';

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

  const fetchCluster = async () => {
    setRefreshing(true);
    try {
      const res = await fetch('/api/v1/cluster/status');
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = await res.json();
      setCluster(data);
      setError(null);
    } catch (e) {
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

  if (error) {
    return (
      <div className="panel error">
        <h2>Cluster Dashboard</h2>
        <p className="error-msg">无法连接集群: {error}</p>
        <button onClick={fetchCluster}>重试</button>
      </div>
    );
  }

  if (!cluster) {
    return <div className="panel loading">加载集群状态...</div>;
  }

  const leader = cluster.nodes.find((n) => n.id === cluster.leaderId);
  const onlineNodes = cluster.nodes.filter((n) => n.status === 'online');

  return (
    <div className="panel cluster-dashboard">
      <h2>Cluster Dashboard</h2>

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
                <td>{new Date(node.lastSeen).toLocaleTimeString()}</td>
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

      <footer>
        <button onClick={fetchCluster} disabled={refreshing}>
          {refreshing ? '刷新中...' : '刷新'}
        </button>
      </footer>
    </div>
  );
}

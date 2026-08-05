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
import { nodeStatusLabel, nodeRoleLabel, nodeStatusGlyph } from '../labels';
import { IconLeader } from '../icons';
import { useTableSort, SortableTh } from '../useTableSort';
import { FlashValue } from '../useValueFlash';

interface NodeInfo {
  id: string;
  address: string;
  role: 'leader' | 'follower' | 'candidate';
  status: 'online' | 'offline' | 'leaving';
  capabilities: string[];
  lastSeen: string;
  shards?: number;
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

  // 本地表格排序（hook 必须在条件 return 之前，遵守 hooks 规则）
  const nodes = cluster?.nodes ?? [];
  const { sortedRows, sort, toggleSort } = useTableSort(nodes, {
    id: (n) => n.id,
    address: (n) => n.address,
    role: (n) => n.role,
    status: (n) => n.status,
    lastSeen: (n) => n.lastSeen ?? '',
  });

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
    return (
      <div className="panel cluster-dashboard">
        <h2>Cluster Dashboard</h2>
        <div className="skeleton-list" aria-busy="true">
          <div className="skeleton-row" />
          <div className="skeleton-row" />
          <div className="skeleton-row" />
        </div>
      </div>
    );
  }

  const leader = cluster.nodes.find((n) => n.id === cluster.leaderId);
  const onlineNodes = cluster.nodes.filter((n) => n.status === 'online');
  const degradedNodes = cluster.nodes.filter((n) => n.status === 'offline' || n.status === 'leaving');

  return (
    <div className="panel cluster-dashboard">
      <h2>Cluster Dashboard</h2>

      {/* 轮询失败提示：保留旧数据，仅提示刷新失败 */}
      {error && (
        <ErrorPanel message={`刷新失败：${error}（显示上次数据）`} onRetry={fetchCluster} />
      )}

      {/* 节点降级告警：主动打断 Sam 的扫描 */}
      {degradedNodes.length > 0 && (
        <div className="alert-banner alert-danger" role="alert">
          <span className="alert-glyph" aria-hidden="true">⚠</span>
          <span>
            {degradedNodes.length} 个节点状态异常（
            {degradedNodes.map((n) => n.id).join('、')}
            ），请检查节点进程与网络。
          </span>
        </div>
      )}

      {/* 概览 */}
      <section className="overview">
        <div className="stat">
          <span className="label">节点数</span>
          <FlashValue value={`${onlineNodes.length}/${cluster.nodes.length}`} />
        </div>
        <div className="stat">
          <span className="label">领导者</span>
          <FlashValue value={leader?.id ?? '选举中...'} />
        </div>
        <div className="stat">
          <span className="label">哈希环</span>
          <FlashValue value={`${cluster.hashRingSize} vnodes`} />
        </div>
        <div className="stat">
          <span className="label">分片数</span>
          <FlashValue value={cluster.totalShards} />
        </div>
      </section>

      {/* 节点列表 */}
      <section className="node-list">
        <h3>节点拓扑</h3>
        <table>
          <thead>
            <tr>
              <SortableTh sortKey="id" sort={sort} onToggle={toggleSort}>节点 ID</SortableTh>
              <SortableTh sortKey="address" sort={sort} onToggle={toggleSort}>地址</SortableTh>
              <SortableTh sortKey="role" sort={sort} onToggle={toggleSort}>角色</SortableTh>
              <SortableTh sortKey="status" sort={sort} onToggle={toggleSort}>状态</SortableTh>
              <th>能力</th>
              <SortableTh sortKey="lastSeen" sort={sort} onToggle={toggleSort}>最后心跳</SortableTh>
            </tr>
          </thead>
          <tbody>
            {sortedRows.map((node) => (
              <tr key={node.id} className={`node-${node.status}`}>
                <td className="node-id">
                  {node.id === cluster.leaderId && (
                    <span className="badge leader" aria-label="领导者"><IconLeader size={12} /></span>
                  )}
                  {node.id}
                </td>
                <td>{node.address}</td>
                <td><span className={`role-${node.role}`}>{nodeRoleLabel(node.role)}</span></td>
                <td>
                  <span
                    className={`status-badge status-${node.status}`}
                    aria-label={nodeStatusLabel(node.status)}
                  >
                    <span aria-hidden="true">{nodeStatusGlyph(node.status)}</span>
                    {nodeStatusLabel(node.status)}
                  </span>
                </td>
                <td>{node.capabilities?.join(', ') || '-'}</td>
                <td>{node.lastSeen ? new Date(node.lastSeen).toLocaleTimeString() : '-'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>

      {/* 分片视图 */}
      <section className="shard-view">
        <h3>分片分布（共 {cluster.totalShards} 片）</h3>
        {onlineNodes.length === 0 ? (
          <p className="empty">暂无在线节点</p>
        ) : (
          <div className="shard-bar" role="img" aria-label="分片分布">
            {onlineNodes.map((node, i) => {
              const shards = node.shards ?? 0;
              const pct = shards > 0 ? Math.round((shards / cluster.totalShards) * 1000) / 10 : 0;
              return (
                <div
                  key={node.id}
                  className="shard-segment"
                  style={{ flex: `${shards} ${shards} auto`, backgroundColor: `var(--shard-${i % 6})` }}
                  title={`${node.id}: ${pct}%（${shards}/${cluster.totalShards} 片）`}
                >
                  {node.id}
                </div>
              );
            })}
          </div>
        )}
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

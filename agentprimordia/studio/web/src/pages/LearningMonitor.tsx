/**
 * Phase 3.4: Studio Web UI — Learning Monitor
 *
 * 知识蒸馏统计 + 能力进化趋势
 *
 * 加固点：
 *  - 错误不再静默：展示错误面板 + 重试
 *  - 加载/空/错误 状态区分（骨架屏）
 *  - 「上次刷新」陈旧提示
 */
import { useState, useEffect } from 'react';

interface DistillerStats {
  totalInteractions: number;
  totalDistilled: number;
  totalKnowledgeItems: number;
}

interface Capability {
  name: string;
  description: string;
  score: number;
  timesTested: number;
  timesPassed: number;
}

interface PipelineStats {
  totalProcessed: number;
  totalFactsWritten: number;
  totalPatternsWritten: number;
  totalRAGQueries: number;
  lastProcessTime: string;
}

export function LearningMonitor() {
  const [stats, setStats] = useState<DistillerStats | null>(null);
  const [capabilities, setCapabilities] = useState<Capability[]>([]);
  const [pipeline, setPipeline] = useState<PipelineStats | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [lastUpdatedAt, setLastUpdatedAt] = useState<number | null>(null);

  const refresh = async () => {
    setLoading(true);
    setError(null);
    try {
      const [statsRes, capRes, pipeRes] = await Promise.all([
        fetch('/api/v1/learning/stats'),
        fetch('/api/v1/learning/capabilities'),
        fetch('/api/v1/learning/pipeline/stats'),
      ]);
      const [s, c, p] = await Promise.all([
        statsRes.json(), capRes.json(), pipeRes.json(),
      ]);
      if (statsRes.ok) setStats(s);
      if (capRes.ok) setCapabilities(c);
      if (pipeRes.ok) setPipeline(p);
      if (!statsRes.ok || !capRes.ok || !pipeRes.ok) {
        throw new Error('部分接口返回非 2xx');
      }
      setLastUpdatedAt(Date.now());
    } catch (e) {
      setError(e instanceof Error ? e.message : '未知错误');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    refresh();
    const timer = setInterval(refresh, 10000);
    return () => clearInterval(timer);
  }, []);

  const statGrid = (items: [string, number][]) => (
    <div className="stat-grid">
      {items.map(([label, value]) => (
        <div className="stat" key={label}>
          <span className="label">{label}</span>
          <span className="value">{value ?? 0}</span>
        </div>
      ))}
    </div>
  );

  return (
    <div className="panel learning-monitor">
      <h2>Learning Monitor</h2>

      {error && (
        <div className="error-panel" role="alert">
          <p className="error-msg">刷新失败：{error}</p>
          <button className="btn-secondary" onClick={refresh}>重试</button>
        </div>
      )}

      {/* 蒸馏统计 */}
      <section className="distill-stats">
        <h3>知识蒸馏</h3>
        {loading && !stats ? (
          <div className="skeleton-list" aria-busy="true">
            <div className="skeleton-row" />
            <div className="skeleton-row" />
          </div>
        ) : stats ? (
          statGrid([
            ['处理交互', stats.totalInteractions],
            ['蒸馏知识', stats.totalDistilled],
            ['知识库存量', stats.totalKnowledgeItems],
          ])
        ) : (
          <p className="empty">暂无数据</p>
        )}
      </section>

      {/* 管道统计 */}
      {pipeline && (
        <section className="pipeline-stats">
          <h3>蒸馏管道</h3>
          {statGrid([
            ['已处理', pipeline.totalProcessed],
            ['事实写入', pipeline.totalFactsWritten],
            ['模式写入', pipeline.totalPatternsWritten],
            ['RAG 查询', pipeline.totalRAGQueries],
          ])}
        </section>
      )}

      {/* 能力进化 */}
      <section className="capabilities">
        <h3>能力进化趋势</h3>
        {loading && capabilities.length === 0 ? (
          <div className="skeleton-list" aria-busy="true">
            <div className="skeleton-row" />
            <div className="skeleton-row" />
          </div>
        ) : capabilities.length === 0 ? (
          <p className="empty">暂无能力数据</p>
        ) : (
          <div className="capability-list">
            {capabilities.map((cap) => (
              <div key={cap.name} className="capability-item">
                <div className="cap-header">
                  <span className="cap-name">{cap.name}</span>
                  <span className={`cap-score ${cap.score < 0.5 ? 'weak' : cap.score < 0.8 ? 'medium' : 'strong'}`}>
                    {(cap.score * 100).toFixed(0)}%
                  </span>
                </div>
                <div className="progress-bar">
                  <div
                    className="progress-fill"
                    style={{
                      width: `${cap.score * 100}%`,
                      backgroundColor: cap.score < 0.5 ? '#e74c3c' : cap.score < 0.8 ? '#f39c12' : '#27ae60',
                    }}
                  />
                </div>
                <div className="cap-meta">
                  测试 {cap.timesTested} 次 | 通过 {cap.timesPassed} 次
                </div>
              </div>
            ))}
          </div>
        )}
      </section>

      <footer className="dashboard-footer">
        <span
          className={`staleness${lastUpdatedAt && Date.now() - lastUpdatedAt > 30000 ? ' stale' : ''}`}
          title="数据最后刷新时间"
        >
          {lastUpdatedAt
            ? `上次刷新 ${Math.max(0, Math.round((Date.now() - lastUpdatedAt) / 1000))} 秒前`
            : '尚未刷新'}
        </span>
      </footer>
    </div>
  );
}

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
import { ErrorPanel, Staleness } from '../Status';
import { FlashValue } from '../useValueFlash';
import { Sparkline } from '../Sparkline';

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
  // 能力历史分数（趋势线数据）
  const [history, setHistory] = useState<Record<string, { score: number }[]>>({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [lastUpdatedAt, setLastUpdatedAt] = useState<number | null>(null);

  const refresh = async () => {
    // 已有数据时不闪骨架，仅首次加载显示
    setLoading(stats === null && pipeline === null && capabilities.length === 0);
    setError(null);
    try {
      const [statsRes, capRes, pipeRes, histRes] = await Promise.all([
        fetch('/api/v1/learning/stats'),
        fetch('/api/v1/learning/capabilities'),
        fetch('/api/v1/learning/pipeline/stats'),
        fetch('/api/v1/learning/capability-history'),
      ]);
      const results = await Promise.all([
        statsRes.ok ? statsRes.json() : Promise.resolve(null),
        capRes.ok ? capRes.json() : Promise.resolve(null),
        pipeRes.ok ? pipeRes.json() : Promise.resolve(null),
        histRes.ok ? histRes.json() : Promise.resolve(null),
      ]);
      const [s, c, p, h] = results;
      // 逐端点提交成功数据：部分失败时仍保留已成功部分
      if (s !== null) setStats(s);
      if (c !== null) setCapabilities(c);
      if (p !== null) setPipeline(p);
      if (h !== null && Array.isArray(h)) {
        // 归并为 name → history 映射，供卡片趋势线查询
        const map: Record<string, { score: number }[]> = {};
        for (const item of h) {
          if (item?.name && Array.isArray(item.history)) {
            map[item.name] = item.history;
          }
        }
        setHistory(map);
      }
      // 任一接口成功即视为刷新成功，更新陈旧提示
      if (s !== null || c !== null || p !== null || h !== null) {
        setLastUpdatedAt(Date.now());
      }
      if (!statsRes.ok || !capRes.ok || !pipeRes.ok || !histRes.ok) {
        const failed = [statsRes, capRes, pipeRes, histRes]
          .map((res, i) => (!res.ok ? ['统计', '能力', '管道', '趋势'][i] : null))
          .filter(Boolean)
          .join('、');
        setError(`部分接口返回异常（${failed}）`);
      }
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
          <FlashValue value={value ?? 0} />
        </div>
      ))}
    </div>
  );

  return (
    <div className="panel learning-monitor">
      <h2>Learning Monitor</h2>

      {error && (
        <ErrorPanel message={`刷新失败：${error}`} onRetry={refresh} />
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
                {history[cap.name] && history[cap.name].length >= 2 && (
                  <div className="cap-trend">
                    <Sparkline data={history[cap.name]} />
                    <span className="cap-trend-label">
                      近 {history[cap.name].length} 次评估趋势
                    </span>
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </section>

      <footer className="dashboard-footer">
        <Staleness lastUpdatedAt={lastUpdatedAt} />
      </footer>
    </div>
  );
}

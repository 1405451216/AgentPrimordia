/**
 * Phase 3.4: Studio Web UI — Learning Monitor
 *
 * 知识蒸馏统计 + 能力进化趋势
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

  useEffect(() => {
    const fetch = async () => {
      try {
        const [statsRes, capRes, pipeRes] = await Promise.all([
          fetch('/api/v1/learning/stats'),
          fetch('/api/v1/learning/capabilities'),
          fetch('/api/v1/learning/pipeline/stats'),
        ]);
        if (statsRes.ok) setStats(await statsRes.json());
        if (capRes.ok) setCapabilities(await capRes.json());
        if (pipeRes.ok) setPipeline(await pipeRes.json());
      } catch { /* 忽略 */ }
    };
    fetch();
    const timer = setInterval(fetch, 10000);
    return () => clearInterval(timer);
  }, []);

  return (
    <div className="panel learning-monitor">
      <h2>Learning Monitor</h2>

      {/* 蒸馏统计 */}
      <section className="distill-stats">
        <h3>知识蒸馏</h3>
        {stats ? (
          <div className="stat-grid">
            <div className="stat">
              <span className="label">处理交互</span>
              <span className="value">{stats.totalInteractions}</span>
            </div>
            <div className="stat">
              <span className="label">蒸馏知识</span>
              <span className="value">{stats.totalDistilled}</span>
            </div>
            <div className="stat">
              <span className="label">知识库存量</span>
              <span className="value">{stats.totalKnowledgeItems}</span>
            </div>
          </div>
        ) : (
          <p className="empty">暂无数据</p>
        )}
      </section>

      {/* 管道统计 */}
      {pipeline && (
        <section className="pipeline-stats">
          <h3>蒸馏管道</h3>
          <div className="stat-grid">
            <div className="stat">
              <span className="label">已处理</span>
              <span className="value">{pipeline.totalProcessed}</span>
            </div>
            <div className="stat">
              <span className="label">事实写入</span>
              <span className="value">{pipeline.totalFactsWritten}</span>
            </div>
            <div className="stat">
              <span className="label">模式写入</span>
              <span className="value">{pipeline.totalPatternsWritten}</span>
            </div>
            <div className="stat">
              <span className="label">RAG 查询</span>
              <span className="value">{pipeline.totalRAGQueries}</span>
            </div>
          </div>
        </section>
      )}

      {/* 能力进化 */}
      <section className="capabilities">
        <h3>能力进化趋势</h3>
        {capabilities.length === 0 ? (
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
    </div>
  );
}

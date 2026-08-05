/**
 * Studio 首页概览
 *
 * 聚合集群健康 / 学习脉动 / 近期实验，作为登录后的第一屏，
 * 避免用户直接落进最危险的混沌实验页。
 */
import { useState, useEffect } from 'react';
import { Link } from 'react-router-dom';
import { ErrorPanel, Staleness } from '../Status';
import { experimentStatusLabel, experimentStatusGlyph } from '../labels';
import { FlashValue } from '../useValueFlash';
import { PageTitle } from '../PageTitle';
import { Primordium } from '../Primordium';

interface ClusterState {
  nodes: { id: string; status: string; role: string }[];
  leaderId: string;
  hashRingSize: number;
  totalShards: number;
}

interface LearningStats {
  totalInteractions: number;
  totalDistilled: number;
  totalKnowledgeItems: number;
}

interface Capability {
  name: string;
  score: number;
}

interface ExperimentRow {
  experiment: { name: string; status: string; hypothesisValidated: boolean };
  startTime: string;
}

export function Overview() {
  const [cluster, setCluster] = useState<ClusterState | null>(null);
  const [learning, setLearning] = useState<LearningStats | null>(null);
  const [capabilities, setCapabilities] = useState<Capability[]>([]);
  const [experiments, setExperiments] = useState<ExperimentRow[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [lastUpdatedAt, setLastUpdatedAt] = useState<number | null>(null);

  const refresh = async () => {
    setError(null);
    try {
      // 逐端点提交成功数据：单个接口失败不拖垮整个概览
      const [clusterRes, learningRes, capRes, expRes] = await Promise.all([
        fetch('/api/v1/cluster/status'),
        fetch('/api/v1/learning/stats'),
        fetch('/api/v1/learning/capabilities'),
        fetch('/api/v1/chaos/experiments'),
      ]);
      const results = await Promise.all([
        clusterRes.ok ? clusterRes.json() : Promise.resolve(null),
        learningRes.ok ? learningRes.json() : Promise.resolve(null),
        capRes.ok ? capRes.json() : Promise.resolve(null),
        expRes.ok ? expRes.json() : Promise.resolve(null),
      ]);
      const [c, l, caps, e] = results;
      if (c !== null) setCluster(c);
      if (l !== null) setLearning(l);
      if (caps !== null && Array.isArray(caps)) setCapabilities(caps);
      if (e !== null) setExperiments(e);
      if (c !== null || l !== null || caps !== null || e !== null) {
        setLastUpdatedAt(Date.now());
      }
      if (!clusterRes.ok || !learningRes.ok || !capRes.ok || !expRes.ok) {
        const failed = [clusterRes, learningRes, capRes, expRes]
          .map((res, i) => (!res.ok ? ['集群', '学习', '能力', '实验'][i] : null))
          .filter(Boolean)
          .join('、');
        setError(`部分接口返回异常（${failed}）`);
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : '未知错误');
    }
  };

  useEffect(() => {
    refresh();
    const timer = setInterval(refresh, 10000);
    return () => clearInterval(timer);
  }, []);

  const online = cluster?.nodes.filter((n) => n.status === 'online').length ?? 0;
  const total = cluster?.nodes.length ?? 0;
  const leader = cluster?.nodes.find((n) => n.id === cluster?.leaderId);
  const recent = [...experiments].slice(-5).reverse();
  // 原初体脉搏：真实能力平均分（无数据时取 0.5 中性值）
  const pulse = capabilities.length > 0
    ? Math.min(Math.max(capabilities.reduce((s, c) => s + c.score, 0) / capabilities.length, 0.15), 0.95)
    : 0.5;

  return (
    <div className="overview-page">
      {error && (
        <ErrorPanel message={`刷新失败：${error}`} onRetry={refresh} />
      )}

      <section className="overview-hero">
        <div className="overview-hero-top">
          <div>
            <PageTitle title="系统概览" subtitle="Overview" />
            <p className="overview-sub">集群、学习与实验的实时状态。</p>
          </div>
        </div>
        <div className="overview-hero-body">
          <div className="overview-primordium">
            <Primordium
              nodes={cluster?.nodes ?? []}
              pulse={pulse}
            />
            <p className="overview-primordium-caption">
              {online > 0
                ? `${online} 个节点在线 · 能力脉搏 ${Math.round(pulse * 100)}%`
                : '等待集群接入...'}
            </p>
          </div>
          <div className="overview-grid">
            <div className="overview-card">
              <span className="overview-card-label">在线节点</span>
              <FlashValue className="overview-card-value" value={total > 0 ? `${online}/${total}` : '—'} />
              <span className="overview-card-meta">领导者 {leader?.id ?? '选举中...'}</span>
            </div>
            <div className="overview-card">
              <span className="overview-card-label">知识蒸馏</span>
              <FlashValue className="overview-card-value" value={learning?.totalDistilled ?? '—'} />
            <span className="overview-card-meta">
              交互 {learning?.totalInteractions ?? 0} · 库存 {learning?.totalKnowledgeItems ?? 0}
            </span>
          </div>
          <div className="overview-card">
            <span className="overview-card-label">实验记录</span>
            <FlashValue className="overview-card-value" value={experiments.length} />
            <span className="overview-card-meta">最近运行见下方</span>
          </div>
          </div>
        </div>
      </section>

      <section className="overview-panel">
        <div className="overview-panel-header">
          <h3>近期实验</h3>
          <Staleness lastUpdatedAt={lastUpdatedAt} />
        </div>
        {experiments.length === 0 ? (
          <p className="empty">暂无实验记录</p>
        ) : (
          <table>
            <thead>
              <tr>
                <th>名称</th>
                <th>状态</th>
                <th>假设验证</th>
                <th>时间</th>
              </tr>
            </thead>
            <tbody>
              {recent.map((r, i) => (
                <tr key={`${r.experiment.name}-${r.startTime}-${i}`}>
                  <td>{r.experiment.name}</td>
                  <td>
                    <span className={`status-badge status-${r.experiment.status}`} aria-label={experimentStatusLabel(r.experiment.status)}>
                      <span aria-hidden="true">{experimentStatusGlyph(r.experiment.status)}</span>
                      {experimentStatusLabel(r.experiment.status)}
                    </span>
                  </td>
                  <td className={r.experiment.hypothesisValidated ? 'glyph-ok' : 'glyph-bad'} aria-label={r.experiment.hypothesisValidated ? '已验证' : '未验证'}>
                    {r.experiment.hypothesisValidated ? '✓' : '✕'}
                  </td>
                  <td>{r.startTime ? new Date(r.startTime).toLocaleString() : '-'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>

      <section className="overview-actions">
        <Link className="btn-secondary overview-action" to="/chaos">前往混沌实验</Link>
        <Link className="btn-secondary overview-action" to="/cluster">查看集群</Link>
      </section>
    </div>
  );
}

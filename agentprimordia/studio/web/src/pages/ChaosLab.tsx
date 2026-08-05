/**
 * Phase 3.4: Studio Web UI — Chaos Lab
 *
 * 可视化混沌实验创建/运行/报告
 *
 * 加固点：
 *  - 破坏性操作（进程终止等）两步确认
 *  - POST 检查 res.ok，失败展示 HTTP 错误
 *  - 提交成功后展示持久「实验已提交」横幅
 *  - 实验历史区分 加载/空/错误 三种状态
 */
import { useState, useEffect } from 'react';

interface Experiment {
  name: string;
  description: string;
  hypothesis: string;
  status: 'pending' | 'running' | 'completed' | 'aborted' | 'failed';
  duration: string;
  faults: { type: string; description: string }[];
  hypothesisValidated: boolean;
}

interface ExperimentResult {
  experiment: Experiment;
  startTime: string;
  endTime: string;
  preSteadyState: { met: boolean; message: string };
  postSteadyState: { met: boolean; message: string };
}

/** 故障类型元数据：展示名 + 是否破坏性（需要强确认） */
const FAULT_TYPES: Record<string, { label: string; destructive: boolean }> = {
  latency: { label: '延迟注入', destructive: false },
  error: { label: '错误注入', destructive: false },
  timeout: { label: '超时注入', destructive: false },
  partition: { label: '网络分区', destructive: true },
  kill: { label: '进程终止', destructive: true },
};

export function ChaosLab() {
  const [experiments, setExperiments] = useState<ExperimentResult[]>([]);
  const [running, setRunning] = useState(false);
  const [loading, setLoading] = useState(true);
  const [fetchError, setFetchError] = useState<string | null>(null);
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [submittedName, setSubmittedName] = useState<string | null>(null);
  const [newExp, setNewExp] = useState({ name: '', hypothesis: '', faultType: 'latency' });
  // 待确认的实验：非空时弹出两步确认对话框
  const [confirming, setConfirming] = useState<typeof newExp | null>(null);

  const fetchExperiments = async () => {
    setLoading(true);
    setFetchError(null);
    try {
      const res = await fetch('/api/v1/chaos/experiments');
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      setExperiments(await res.json());
    } catch (e) {
      setFetchError(e instanceof Error ? e.message : '未知错误');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { fetchExperiments(); }, []);

  const runExperiment = async (draft: typeof newExp) => {
    setRunning(true);
    setSubmitError(null);
    try {
      const res = await fetch('/api/v1/chaos/experiments', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(draft),
      });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      setSubmittedName(draft.name);
      await fetchExperiments();
      setNewExp({ name: '', hypothesis: '', faultType: 'latency' });
      setConfirming(null);
    } catch (e) {
      setSubmitError(e instanceof Error ? e.message : '未知错误');
    } finally {
      setRunning(false);
    }
  };

  const faultLabel = (type: string) => FAULT_TYPES[type]?.label ?? type;
  const isDestructive = (type: string) => FAULT_TYPES[type]?.destructive ?? false;

  return (
    <div className="panel chaos-lab">
      <h2>Chaos Lab</h2>

      {/* 创建实验 */}
      <section className="create-experiment">
        <h3>新建实验</h3>
        {submitError && (
          <p className="error-msg" role="alert">提交失败：{submitError}</p>
        )}
        {submittedName && (
          <div className="success-banner" role="status">
            <span>实验「{submittedName}」已提交</span>
            <button className="banner-close" onClick={() => setSubmittedName(null)} aria-label="关闭">✕</button>
          </div>
        )}
        <div className="form-row">
          <input
            placeholder="实验名称"
            value={newExp.name}
            onChange={(e) => setNewExp({ ...newExp, name: e.target.value })}
          />
          <input
            placeholder="假设（预期行为）"
            value={newExp.hypothesis}
            onChange={(e) => setNewExp({ ...newExp, hypothesis: e.target.value })}
          />
          <select
            value={newExp.faultType}
            onChange={(e) => setNewExp({ ...newExp, faultType: e.target.value })}
          >
            {Object.entries(FAULT_TYPES).map(([value, meta]) => (
              <option key={value} value={value}>{meta.label}</option>
            ))}
          </select>
          <button
            onClick={() => newExp.name.trim() && setConfirming(newExp)}
            disabled={running || !newExp.name.trim()}
          >
            {running ? '运行中...' : '运行实验'}
          </button>
        </div>
      </section>

      {/* 实验列表 */}
      <section className="experiment-list">
        <h3>实验历史</h3>
        {fetchError ? (
          <div className="error-panel" role="alert">
            <p className="error-msg">加载失败：{fetchError}</p>
            <button className="btn-secondary" onClick={fetchExperiments}>重试</button>
          </div>
        ) : loading ? (
          <div className="skeleton-list" aria-busy="true">
            <div className="skeleton-row" />
            <div className="skeleton-row" />
          </div>
        ) : experiments.length === 0 ? (
          <p className="empty">暂无实验记录</p>
        ) : (
          <table>
            <thead>
              <tr>
                <th>名称</th>
                <th>状态</th>
                <th>假设验证</th>
                <th>稳态(前)</th>
                <th>稳态(后)</th>
                <th>时间</th>
              </tr>
            </thead>
            <tbody>
              {experiments.map((r, i) => (
                <tr key={r.experiment.name || i}>
                  <td>{r.experiment.name}</td>
                  <td><span className={`status-${r.experiment.status}`}>{r.experiment.status}</span></td>
                  <td>{r.experiment.hypothesisValidated ? '✅' : '❌'}</td>
                  <td>{r.preSteadyState?.met ? '✅' : '❌'}</td>
                  <td>{r.postSteadyState?.met ? '✅' : '❌'}</td>
                  <td>{r.startTime ? new Date(r.startTime).toLocaleString() : '-'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>

      {/* 两步确认对话框 */}
      {confirming && (
        <div className="modal-overlay" onClick={() => !running && setConfirming(null)}>
          <div
            className="modal"
            role="dialog"
            aria-modal="true"
            aria-labelledby="confirm-title"
            onClick={(e) => e.stopPropagation()}
          >
            <h3 id="confirm-title">确认运行混沌实验</h3>
            {isDestructive(confirming.faultType) && (
              <p className="confirm-warning" role="alert">
                该实验将注入「{faultLabel(confirming.faultType)}」，可能中断集群服务或终止节点进程。请确认影响范围。
              </p>
            )}
            <dl className="confirm-detail">
              <dt>实验名称</dt><dd>{confirming.name}</dd>
              <dt>假设</dt><dd>{confirming.hypothesis || '（未填写）'}</dd>
              <dt>故障类型</dt><dd>{faultLabel(confirming.faultType)}</dd>
            </dl>
            <div className="confirm-actions">
              <button className="btn-secondary" onClick={() => setConfirming(null)} disabled={running}>取消</button>
              <button
                className={isDestructive(confirming.faultType) ? 'btn-danger' : ''}
                onClick={() => runExperiment(confirming)}
                disabled={running}
              >
                {running ? '提交中...' : '确认运行'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

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
import { useState, useEffect, useRef } from 'react';
import { ErrorPanel, SuccessBanner } from '../Status';
import { experimentStatusLabel } from '../labels';
import { useTableSort, SortableTh } from '../useTableSort';

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
  const [abortingName, setAbortingName] = useState<string | null>(null);
  const runButtonRef = useRef<HTMLButtonElement>(null);
  const modalRef = useRef<HTMLDivElement>(null);

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

  // 中止运行中的实验
  const abortExperiment = async (name: string) => {
    setAbortingName(name);
    try {
      const res = await fetch('/api/v1/chaos/experiments/abort', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name }),
      });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      await refreshExperiments();
    } catch (e) {
      setFetchError(e instanceof Error ? e.message : '未知错误');
    } finally {
      setAbortingName(null);
    }
  };

  // 对话框焦点管理：打开时聚焦取消按钮，Esc 关闭，Tab 陷阱，关闭后恢复焦点
  useEffect(() => {
    if (!confirming) return;
    const cancelBtn = modalRef.current?.querySelector<HTMLButtonElement>('button:first-of-type');
    cancelBtn?.focus();
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && !running) {
        setConfirming(null);
        runButtonRef.current?.focus();
        return;
      }
      if (e.key !== 'Tab' || !modalRef.current) return;
      const focusables = modalRef.current.querySelectorAll<HTMLElement>(
        'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])',
      );
      if (focusables.length === 0) return;
      const first = focusables[0];
      const last = focusables[focusables.length - 1];
      if (e.shiftKey && document.activeElement === first) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault();
        first.focus();
      }
    };
    document.addEventListener('keydown', onKeyDown);
    return () => {
      document.removeEventListener('keydown', onKeyDown);
      if (!confirming) runButtonRef.current?.focus();
    };
  }, [confirming, running]);

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
      await refreshExperiments();
      setNewExp({ name: '', hypothesis: '', faultType: 'latency' });
      setConfirming(null);
    } catch (e) {
      setSubmitError(e instanceof Error ? e.message : '未知错误');
    } finally {
      setRunning(false);
    }
  };

  // 刷新实验历史：仅在无数据（首次/全空）时显示骨架，避免刷新闪烁
  const refreshExperiments = async () => {
    setFetchError(null);
    try {
      const res = await fetch('/api/v1/chaos/experiments');
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      setExperiments(await res.json());
    } catch (e) {
      setFetchError(e instanceof Error ? e.message : '未知错误');
    }
  };

  const faultLabel = (type: string) => FAULT_TYPES[type]?.label ?? type;
  const isDestructive = (type: string) => FAULT_TYPES[type]?.destructive ?? false;

  // 本地表格排序（实验历史）
  const { sortedRows, sort, toggleSort } = useTableSort(experiments, {
    name: (r) => r.experiment.name,
    status: (r) => r.experiment.status,
    hypothesisValidated: (r) => (r.experiment.hypothesisValidated ? 1 : 0),
    startTime: (r) => r.startTime ?? '',
  });

  return (
    <div className="panel chaos-lab">
      <h2>Chaos Lab</h2>

      {/* 创建实验 */}
      <section className="create-experiment">
        <h3>新建实验</h3>
        {submitError && (
          <ErrorPanel message={`提交失败：${submitError}`} />
        )}
        {submittedName && (
          <SuccessBanner onDismiss={() => setSubmittedName(null)}>
            实验「{submittedName}」已提交
          </SuccessBanner>
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
            ref={runButtonRef}
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
          <ErrorPanel message={`加载失败：${fetchError}`} onRetry={fetchExperiments} />
        ) : loading && experiments.length === 0 ? (
          <div className="skeleton-list" aria-busy="true">
            <div className="skeleton-row" />
            <div className="skeleton-row" />
          </div>
        ) : experiments.length === 0 ? (
          <div className="empty-state">
            <p className="empty">暂无实验记录</p>
            <p className="empty-hint">
              混沌实验用于验证系统在故障注入下的稳态表现。
              填写上方表单并选择故障类型即可创建第一个实验。
            </p>
          </div>
        ) : (
          <table>
            <thead>
              <tr>
                <SortableTh sortKey="name" sort={sort} onToggle={toggleSort}>名称</SortableTh>
                <SortableTh sortKey="status" sort={sort} onToggle={toggleSort}>状态</SortableTh>
                <th className="has-tip" title="实验结束后判断假设是否被验证">假设验证 <span className="tip-mark" aria-hidden="true">ⓘ</span></th>
                <th className="has-tip" title="故障注入前系统是否处于稳态基线">稳态(前) <span className="tip-mark" aria-hidden="true">ⓘ</span></th>
                <th className="has-tip" title="故障恢复后系统是否回到稳态">稳态(后) <span className="tip-mark" aria-hidden="true">ⓘ</span></th>
                <SortableTh sortKey="startTime" sort={sort} onToggle={toggleSort}>时间</SortableTh>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              {sortedRows.map((r, i) => (
                <tr key={`${r.experiment.name}-${r.startTime}-${i}`}>
                  <td>{r.experiment.name}</td>
                  <td><span className={`status-${r.experiment.status}`}>{experimentStatusLabel(r.experiment.status)}</span></td>
                  <td>{r.experiment.hypothesisValidated ? '✅' : '❌'}</td>
                  <td>{r.preSteadyState?.met ? '✅' : '❌'}</td>
                  <td>{r.postSteadyState?.met ? '✅' : '❌'}</td>
                  <td>{r.startTime ? new Date(r.startTime).toLocaleString() : '-'}</td>
                  <td>
                    {(r.experiment.status === 'running' || r.experiment.status === 'pending') && (
                      <button
                        className="btn-danger btn-sm"
                        onClick={() => abortExperiment(r.experiment.name)}
                        disabled={abortingName === r.experiment.name}
                      >
                        {abortingName === r.experiment.name ? '中止中...' : '中止'}
                      </button>
                    )}
                  </td>
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
            ref={modalRef}
            className="modal"
            role="dialog"
            aria-modal="true"
            aria-labelledby="confirm-title"
            aria-describedby="confirm-desc"
            onClick={(e) => e.stopPropagation()}
          >
            <h3 id="confirm-title">确认运行混沌实验</h3>
            <p id="confirm-desc" className="confirm-desc">
              即将提交以下实验；{isDestructive(confirming.faultType) ? '该故障类型为破坏性操作。' : ''}
            </p>
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
              <button className="btn-secondary" onClick={() => { setConfirming(null); runButtonRef.current?.focus(); }} disabled={running}>取消</button>
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

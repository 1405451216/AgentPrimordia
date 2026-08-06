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
import { Fragment, useState, useEffect } from 'react';
import { ErrorPanel, SuccessBanner, Staleness } from '../Status';
import { experimentStatusLabel, experimentStatusGlyph } from '../labels';
import { useTableSort, SortableTh } from '../useTableSort';
import { PageTitle } from '../PageTitle';
import { useConfirmDialog } from '../useConfirmDialog';

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
  const [lastUpdatedAt, setLastUpdatedAt] = useState<number | null>(null);
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [submittedName, setSubmittedName] = useState<string | null>(null);
  const [newExp, setNewExp] = useState({ name: '', hypothesis: '', faultType: 'latency' });
  // 待确认的实验：非空时弹出两步确认对话框
  const [confirming, setConfirming] = useState<typeof newExp | null>(null);
  // 待确认中止的实验名：非空时弹出中止确认
  const [confirmingAbort, setConfirmingAbort] = useState<string | null>(null);
  const [abortingName, setAbortingName] = useState<string | null>(null);
  // 中止操作的独立错误（不再误标为「加载失败」）
  const [abortError, setAbortError] = useState<string | null>(null);
  // 详情面板展开的实验名（null = 收起）；按名称匹配而非排序索引，避免排序后错位
  const [detailName, setDetailName] = useState<string | null>(null);

  const fetchExperiments = async () => {
    setLoading(true);
    setFetchError(null);
    try {
      const res = await fetch('/api/v1/chaos/experiments');
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      setExperiments(await res.json());
      setLastUpdatedAt(Date.now());
    } catch (e) {
      setFetchError(e instanceof Error ? e.message : '未知错误');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { fetchExperiments(); }, []);

  // 中止运行中的实验；返回是否成功（失败时保留对话框展示错误）
  const abortExperiment = async (name: string): Promise<boolean> => {
    setAbortingName(name);
    try {
      const res = await fetch('/api/v1/chaos/experiments/abort', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name }),
      });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      await refreshExperiments();
      return true;
    } catch (e) {
      setAbortError(e instanceof Error ? e.message : '未知错误');
      return false;
    } finally {
      setAbortingName(null);
    }
  };

  // 运行确认对话框：统一焦点管理
  const runDialog = useConfirmDialog({
    open: confirming !== null,
    busy: running,
    onClose: () => setConfirming(null),
  });

  // 中止确认对话框：统一焦点管理
  const abortDialog = useConfirmDialog({
    open: confirmingAbort !== null,
    busy: abortingName !== null,
    onClose: () => setConfirmingAbort(null),
  });

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

  // 刷新实验历史：保留旧数据，仅首次/空态显示骨架，避免闪烁
  const refreshExperiments = async () => {
    setFetchError(null);
    try {
      const res = await fetch('/api/v1/chaos/experiments');
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      setExperiments(await res.json());
      setLastUpdatedAt(Date.now());
    } catch (e) {
      setFetchError(e instanceof Error ? e.message : '未知错误');
    }
  };

  // 轮询实验历史：运行中的实验状态实时可见（5s 周期）
  useEffect(() => {
    const timer = setInterval(refreshExperiments, 5000);
    return () => clearInterval(timer);
  }, []);

  const faultLabel = (type: string) => FAULT_TYPES[type]?.label ?? type;
  const isDestructive = (type: string) => FAULT_TYPES[type]?.destructive ?? false;

  // 本地表格排序（实验历史）；状态按生命周期序而非英文枚举
  const STATUS_ORDER: Record<string, number> = {
    pending: 0, running: 1, completed: 2, aborted: 3, failed: 4,
  };
  const { sortedRows, sort, toggleSort } = useTableSort(experiments, {
    name: (r) => r.experiment.name,
    status: (r) => STATUS_ORDER[r.experiment.status] ?? 99,
    hypothesisValidated: (r) => (r.experiment.hypothesisValidated ? 1 : 0),
    startTime: (r) => r.startTime ?? '',
  });

  return (
    <div className="panel chaos-lab">
      <PageTitle title="混沌实验" subtitle="Chaos Lab" />

      {/* 创建实验 */}
      <section className="create-experiment">
        <h3>新建实验</h3>
        {submittedName && (
          <SuccessBanner onDismiss={() => setSubmittedName(null)}>
            实验「{submittedName}」已提交
          </SuccessBanner>
        )}
        <div className="form-row">
          <label className="field">
            <span className="field-label">实验名称</span>
            <input
              placeholder="例如：P99 延迟压测"
              value={newExp.name}
              onChange={(e) => setNewExp({ ...newExp, name: e.target.value })}
            />
          </label>
          <label className="field">
            <span className="field-label">假设</span>
            <input
              placeholder="预期行为（可选）"
              value={newExp.hypothesis}
              onChange={(e) => setNewExp({ ...newExp, hypothesis: e.target.value })}
            />
          </label>
          <label className="field">
            <span className="field-label">故障类型</span>
            <select
              value={newExp.faultType}
              onChange={(e) => setNewExp({ ...newExp, faultType: e.target.value })}
            >
              {Object.entries(FAULT_TYPES).map(([value, meta]) => (
                <option key={value} value={value}>{meta.label}</option>
              ))}
            </select>
          </label>
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
                <Fragment key={`${r.experiment.name}-${r.startTime}-${i}`}>
                  <tr className={detailName === r.experiment.name ? 'row-expanded' : ''}>
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
                    <td className={r.preSteadyState?.met ? 'glyph-ok' : 'glyph-bad'} aria-label={r.preSteadyState?.met ? '稳态达成' : '稳态未达成'}>
                      {r.preSteadyState?.met ? '✓' : '✕'}
                    </td>
                    <td className={r.postSteadyState?.met ? 'glyph-ok' : 'glyph-bad'} aria-label={r.postSteadyState?.met ? '稳态达成' : '稳态未达成'}>
                      {r.postSteadyState?.met ? '✓' : '✕'}
                    </td>
                    <td>{r.startTime ? new Date(r.startTime).toLocaleString() : '-'}</td>
                    <td className="row-actions">
                      <button
                        className="btn-secondary btn-sm"
                        onClick={() => setDetailName(detailName === r.experiment.name ? null : r.experiment.name)}
                      >
                        {detailName === r.experiment.name ? '收起' : '详情'}
                      </button>
                      {(r.experiment.status === 'running' || r.experiment.status === 'pending') && (
                        <button
                          className="btn-danger btn-sm"
                          onClick={() => setConfirmingAbort(r.experiment.name)}
                          disabled={abortingName === r.experiment.name}
                        >
                          {abortingName === r.experiment.name ? '中止中...' : '中止'}
                        </button>
                      )}
                    </td>
                  </tr>
                  {detailName === r.experiment.name && (
                    <tr className="detail-row">
                      <td colSpan={7}>
                        <div className="experiment-detail">
                          <dl className="detail-grid">
                            <dt>描述</dt><dd>{r.experiment.description || '（无）'}</dd>
                            <dt>假设</dt><dd>{r.experiment.hypothesis || '（未填写）'}</dd>
                            <dt>时长</dt><dd>{r.experiment.duration || '-'}</dd>
                            <dt>开始</dt><dd>{r.startTime ? new Date(r.startTime).toLocaleString() : '-'}</dd>
                            <dt>结束</dt><dd>{r.endTime ? new Date(r.endTime).toLocaleString() : '-'}</dd>
                            <dt>稳态(前)</dt>
                            <dd>
                              <span className={r.preSteadyState?.met ? 'text-ok' : 'text-bad'}>
                                {r.preSteadyState?.met ? '✓ 达成' : '✕ 未达成'}
                              </span>
                              {r.preSteadyState?.message && <span className="detail-msg"> — {r.preSteadyState.message}</span>}
                            </dd>
                            <dt>稳态(后)</dt>
                            <dd>
                              <span className={r.postSteadyState?.met ? 'text-ok' : 'text-bad'}>
                                {r.postSteadyState?.met ? '✓ 达成' : '✕ 未达成'}
                              </span>
                              {r.postSteadyState?.message && <span className="detail-msg"> — {r.postSteadyState.message}</span>}
                            </dd>
                          </dl>
                          <h4 className="detail-heading">故障注入</h4>
                          {r.experiment.faults?.length ? (
                            <ul className="fault-list">
                              {r.experiment.faults.map((f, fi) => (
                                <li key={fi}>
                                  <span className="tag">{faultLabel(f.type)}</span>
                                  <span className="fault-desc">{f.description || '（无描述）'}</span>
                                </li>
                              ))}
                            </ul>
                          ) : (
                            <p className="empty">无故障注入记录</p>
                          )}
                        </div>
                      </td>
                    </tr>
                  )}
                </Fragment>
              ))}
            </tbody>
          </table>
        )}
      </section>

      <footer className="dashboard-footer">
        <Staleness lastUpdatedAt={lastUpdatedAt} />
      </footer>

      {/* 两步确认对话框 */}
      {confirming && (
        <div className="modal-overlay" onClick={() => !running && runDialog.closeAndRestore()}>
          <div
            ref={runDialog.dialogRef}
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
            {submitError && (
              <p className="error-msg" role="alert">提交失败：{submitError}</p>
            )}
            <div className="confirm-actions">
              <button className="btn-secondary" onClick={runDialog.closeAndRestore} disabled={running}>取消</button>
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

      {/* 中止实验确认对话框 */}
      {confirmingAbort && (
        <div className="modal-overlay" onClick={() => !abortingName && abortDialog.closeAndRestore()}>
          <div
            ref={abortDialog.dialogRef}
            className="modal"
            role="dialog"
            aria-modal="true"
            aria-labelledby="abort-confirm-title"
            aria-describedby="abort-confirm-desc"
            onClick={(e) => e.stopPropagation()}
          >
            <h3 id="abort-confirm-title">确认中止实验</h3>
            <p id="abort-confirm-desc" className="confirm-desc">
              中止后该实验将立即停止注入故障，实验状态标记为已中止。
            </p>
            <dl className="confirm-detail">
              <dt>实验名称</dt><dd>{confirmingAbort}</dd>
            </dl>
            {abortError && (
              <p className="error-msg" role="alert">中止失败：{abortError}</p>
            )}
            <div className="confirm-actions">
              <button className="btn-secondary" onClick={abortDialog.closeAndRestore} disabled={abortingName !== null}>取消</button>
              <button
                className="btn-danger"
                onClick={async () => {
                  const name = confirmingAbort;
                  const ok = await abortExperiment(name);
                  if (ok) setConfirmingAbort(null);
                }}
                disabled={abortingName !== null}
              >
                {abortingName !== null ? '中止中...' : '确认中止'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

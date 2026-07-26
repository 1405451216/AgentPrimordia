/**
 * Phase 3.4: Studio Web UI — Chaos Lab
 *
 * 可视化混沌实验创建/运行/报告
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

export function ChaosLab() {
  const [experiments, setExperiments] = useState<ExperimentResult[]>([]);
  const [running, setRunning] = useState(false);
  const [newExp, setNewExp] = useState({ name: '', hypothesis: '', faultType: 'latency' });

  const fetchExperiments = async () => {
    try {
      const res = await fetch('/api/v1/chaos/experiments');
      if (res.ok) setExperiments(await res.json());
    } catch { /* 忽略 */ }
  };

  useEffect(() => { fetchExperiments(); }, []);

  const runExperiment = async () => {
    if (!newExp.name) return;
    setRunning(true);
    try {
      await fetch('/api/v1/chaos/experiments', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(newExp),
      });
      await fetchExperiments();
      setNewExp({ name: '', hypothesis: '', faultType: 'latency' });
    } finally {
      setRunning(false);
    }
  };

  return (
    <div className="panel chaos-lab">
      <h2>Chaos Lab</h2>

      {/* 创建实验 */}
      <section className="create-experiment">
        <h3>新建实验</h3>
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
            <option value="latency">延迟注入</option>
            <option value="error">错误注入</option>
            <option value="timeout">超时注入</option>
            <option value="partition">网络分区</option>
            <option value="kill">进程终止</option>
          </select>
          <button onClick={runExperiment} disabled={running || !newExp.name}>
            {running ? '运行中...' : '运行实验'}
          </button>
        </div>
      </section>

      {/* 实验列表 */}
      <section className="experiment-list">
        <h3>实验历史</h3>
        {experiments.length === 0 ? (
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
                <tr key={i}>
                  <td>{r.experiment.name}</td>
                  <td><span className={`status-${r.experiment.status}`}>{r.experiment.status}</span></td>
                  <td>{r.experiment.hypothesisValidated ? '✅' : '❌'}</td>
                  <td>{r.preSteadyState?.met ? '✅' : '❌'}</td>
                  <td>{r.postSteadyState?.met ? '✅' : '❌'}</td>
                  <td>{new Date(r.startTime).toLocaleString()}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>
    </div>
  );
}

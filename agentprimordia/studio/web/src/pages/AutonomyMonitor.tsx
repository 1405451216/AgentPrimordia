/**
 * v3.3 Studio Web UI — Autonomy Monitor
 *
 * 目标列表 + 进度 + 停滞告警 + 恢复操作
 */
import { useState, useEffect } from 'react';

interface GoalInfo {
  id: string;
  description: string;
  state: string;
  priority: number;
  progress: number;
  retryCount: number;
  createdAt: string;
}

interface AlertInfo {
  goalId: string;
  level: string;
  message: string;
  timestamp: string;
}

export function AutonomyMonitor() {
  const [goals, setGoals] = useState<GoalInfo[]>([]);
  const [alerts, setAlerts] = useState<AlertInfo[]>([]);

  useEffect(() => {
    const refresh = async () => {
      try {
        const [goalsRes, alertsRes] = await Promise.all([
          fetch('/api/v1/autonomy/goals'),
          fetch('/api/v1/autonomy/alerts'),
        ]);
        if (goalsRes.ok) setGoals(await goalsRes.json());
        if (alertsRes.ok) setAlerts(await alertsRes.json());
      } catch { /* 忽略 */ }
    };
    refresh();
    const timer = setInterval(refresh, 5000);
    return () => clearInterval(timer);
  }, []);

  const resumeGoal = async (id: string) => {
    await fetch(`/api/v1/autonomy/goals/${id}/resume`, { method: 'POST' });
  };

  const stateColor = (state: string) => {
    switch (state) {
      case 'done': return '#22c55e';
      case 'executing': return '#3b82f6';
      case 'failed': return '#ef4444';
      case 'planned': return '#f59e0b';
      default: return '#6b7280';
    }
  };

  return (
    <div style={{ padding: 24 }}>
      <h1>🚀 自治目标监控</h1>

      {alerts.length > 0 && (
        <div style={{ marginBottom: 16 }}>
          <h3>⚠️ 告警</h3>
          {alerts.map((a, i) => (
            <div key={i} style={{ padding: 8, margin: 4, background: a.level === 'critical' ? '#fee2e2' : '#fef3c7', borderRadius: 4 }}>
              <strong>[{a.level}]</strong> {a.goalId}: {a.message}
            </div>
          ))}
        </div>
      )}

      <table style={{ width: '100%', borderCollapse: 'collapse' }}>
        <thead>
          <tr>
            <th style={{ textAlign: 'left', padding: 8 }}>目标</th>
            <th>状态</th>
            <th>进度</th>
            <th>重试</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          {goals.map(g => (
            <tr key={g.id} style={{ borderTop: '1px solid #e5e7eb' }}>
              <td style={{ padding: 8 }}>{g.description}</td>
              <td style={{ textAlign: 'center' }}>
                <span style={{ color: stateColor(g.state), fontWeight: 'bold' }}>{g.state}</span>
              </td>
              <td style={{ textAlign: 'center' }}>
                <div style={{ background: '#e5e7eb', borderRadius: 4, height: 8, width: 100, display: 'inline-block' }}>
                  <div style={{ background: '#3b82f6', borderRadius: 4, height: 8, width: `${g.progress * 100}%` }} />
                </div>
              </td>
              <td style={{ textAlign: 'center' }}>{g.retryCount}</td>
              <td style={{ textAlign: 'center' }}>
                {(g.state === 'failed' || g.state === 'executing') && (
                  <button onClick={() => resumeGoal(g.id)} style={{ cursor: 'pointer' }}>恢复</button>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>

      {goals.length === 0 && <p style={{ color: '#6b7280' }}>暂无自治目标</p>}
    </div>
  );
}

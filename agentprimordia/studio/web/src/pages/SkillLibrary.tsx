/**
 * v3.4 Studio Web UI — Skill Library
 *
 * 技能列表 + 命中率 + 手动验证/停用
 */
import { useState, useEffect } from 'react';

interface SkillInfo {
  id: string;
  name: string;
  description: string;
  version: string;
  status: string;
  usageCount: number;
  successRate: number;
  tags: string[];
}

export function SkillLibrary() {
  const [skills, setSkills] = useState<SkillInfo[]>([]);

  useEffect(() => {
    const refresh = async () => {
      try {
        const res = await fetch('/api/v1/skills');
        if (res.ok) setSkills(await res.json());
      } catch { /* 忽略 */ }
    };
    refresh();
    const timer = setInterval(refresh, 10000);
    return () => clearInterval(timer);
  }, []);

  const verifySkill = async (id: string) => {
    await fetch(`/api/v1/skills/${id}/verify`, { method: 'POST' });
  };

  const deprecateSkill = async (id: string) => {
    await fetch(`/api/v1/skills/${id}/deprecate`, { method: 'POST' });
  };

  const statusBadge = (status: string) => {
    const colors: Record<string, string> = {
      active: '#22c55e', verified: '#3b82f6', draft: '#f59e0b', deprecated: '#6b7280',
    };
    return <span style={{ color: colors[status] || '#000', fontWeight: 'bold' }}>{status}</span>;
  };

  return (
    <div style={{ padding: 24 }}>
      <h1>📚 技能库</h1>
      <p style={{ color: '#6b7280' }}>共 {skills.length} 个技能</p>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(300px, 1fr))', gap: 16 }}>
        {skills.map(s => (
          <div key={s.id} style={{ border: '1px solid #e5e7eb', borderRadius: 8, padding: 16 }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <h3 style={{ margin: 0 }}>{s.name}</h3>
              {statusBadge(s.status)}
            </div>
            <p style={{ color: '#6b7280', fontSize: 14 }}>{s.description}</p>
            <div style={{ fontSize: 13, color: '#374151' }}>
              <span>v{s.version}</span> · <span>调用 {s.usageCount} 次</span> · <span>成功率 {(s.successRate * 100).toFixed(0)}%</span>
            </div>
            {s.tags.length > 0 && (
              <div style={{ marginTop: 8 }}>
                {s.tags.map(t => (
                  <span key={t} style={{ background: '#eff6ff', color: '#1d4ed8', padding: '2px 8px', borderRadius: 12, fontSize: 12, marginRight: 4 }}>{t}</span>
                ))}
              </div>
            )}
            <div style={{ marginTop: 12, display: 'flex', gap: 8 }}>
              <button onClick={() => verifySkill(s.id)} style={{ cursor: 'pointer' }}>验证</button>
              {s.status === 'active' && (
                <button onClick={() => deprecateSkill(s.id)} style={{ cursor: 'pointer', color: '#ef4444' }}>停用</button>
              )}
            </div>
          </div>
        ))}
      </div>

      {skills.length === 0 && <p style={{ color: '#6b7280' }}>暂无技能 — Agent 运行中会自动习得</p>}
    </div>
  );
}

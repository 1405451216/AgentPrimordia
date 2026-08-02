/**
 * v3.6 Studio Web UI — Realtime Console
 *
 * 会话状态可视化 + 音频波形 + 打断控制
 */
import { useState, useEffect, useRef } from 'react';

interface SessionInfo {
  id: string;
  state: string;
  createdAt: string;
}

interface RealtimeEvent {
  type: string;
  sessionId: string;
  timestamp: string;
}

export function RealtimeConsole() {
  const [sessions, setSessions] = useState<SessionInfo[]>([]);
  const [events, setEvents] = useState<RealtimeEvent[]>([]);
  const eventsEndRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const refresh = async () => {
      try {
        const [sessRes, evtRes] = await Promise.all([
          fetch('/api/v1/realtime/sessions'),
          fetch('/api/v1/realtime/events?limit=50'),
        ]);
        if (sessRes.ok) setSessions(await sessRes.json());
        if (evtRes.ok) setEvents(await evtRes.json());
      } catch { /* 忽略 */ }
    };
    refresh();
    const timer = setInterval(refresh, 2000);
    return () => clearInterval(timer);
  }, []);

  useEffect(() => {
    eventsEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [events]);

  const bargeIn = async (sessionId: string) => {
    await fetch(`/api/v1/realtime/sessions/${sessionId}/barge-in`, { method: 'POST' });
  };

  const stateIcon = (state: string) => {
    switch (state) {
      case 'idle': return '⏸️';
      case 'listening': return '🎙️';
      case 'thinking': return '🧠';
      case 'speaking': return '🔊';
      default: return '❓';
    }
  };

  return (
    <div style={{ padding: 24 }}>
      <h1>🎧 实时会话控制台</h1>

      <div style={{ display: 'flex', gap: 24 }}>
        {/* 会话列表 */}
        <div style={{ flex: 1 }}>
          <h3>活跃会话 ({sessions.length})</h3>
          {sessions.map(s => (
            <div key={s.id} style={{ border: '1px solid #e5e7eb', borderRadius: 8, padding: 12, marginBottom: 8, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <div>
                <span style={{ fontSize: 20 }}>{stateIcon(s.state)}</span>
                <span style={{ marginLeft: 8, fontWeight: 'bold' }}>{s.id}</span>
                <span style={{ marginLeft: 8, color: '#6b7280' }}>{s.state}</span>
              </div>
              {s.state === 'speaking' && (
                <button onClick={() => bargeIn(s.id)} style={{ background: '#ef4444', color: '#fff', border: 'none', borderRadius: 4, padding: '4px 12px', cursor: 'pointer' }}>
                  ⚡ 打断
                </button>
              )}
            </div>
          ))}
          {sessions.length === 0 && <p style={{ color: '#6b7280' }}>暂无活跃会话</p>}
        </div>

        {/* 事件流 */}
        <div style={{ flex: 1 }}>
          <h3>事件流</h3>
          <div style={{ background: '#1e293b', borderRadius: 8, padding: 12, height: 400, overflow: 'auto', fontFamily: 'monospace', fontSize: 12 }}>
            {events.map((e, i) => (
              <div key={i} style={{ color: '#e2e8f0', marginBottom: 4 }}>
                <span style={{ color: '#64748b' }}>{new Date(e.timestamp).toLocaleTimeString()}</span>
                {' '}
                <span style={{ color: '#38bdf8' }}>[{e.type}]</span>
                {' '}
                <span>{e.sessionId}</span>
              </div>
            ))}
            <div ref={eventsEndRef} />
          </div>
        </div>
      </div>
    </div>
  );
}

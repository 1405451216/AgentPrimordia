/**
 * CollaborationReplay — 协作过程回放（T3-4）。
 *
 * 将消息序列按时间线逐步播放，支持播放/暂停/单步/进度跳转，
 * 复用 MessageFlow 渲染当前可见消息。适用于复盘与演示。
 */

import { useState, useEffect, useRef, useCallback } from 'react';
import type { ReactElement, CSSProperties } from 'react';
import type { CollaborationAgent, CollaborationMessage } from './CollaborationView.js';
import { MessageFlow } from './MessageFlow.js';

export interface CollaborationReplayProps {
  messages: CollaborationMessage[];
  agents?: CollaborationAgent[];
  /** 每步间隔（毫秒），默认 800 */
  intervalMs?: number;
}

export function CollaborationReplay({
  messages,
  agents = [],
  intervalMs = 800,
}: CollaborationReplayProps): ReactElement {
  const [current, setCurrent] = useState(0);
  const [playing, setPlaying] = useState(false);
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const stop = useCallback(() => {
    if (timerRef.current !== null) {
      clearInterval(timerRef.current);
      timerRef.current = null;
    }
    setPlaying(false);
  }, []);

  useEffect(() => {
    if (!playing) return undefined;
    timerRef.current = setInterval(() => {
      setCurrent((c) => {
        if (c >= messages.length - 1) {
          stop();
          return c;
        }
        return c + 1;
      });
    }, intervalMs);
    return () => {
      if (timerRef.current !== null) clearInterval(timerRef.current);
    };
  }, [playing, intervalMs, messages.length, stop]);

  const toggle = () => {
    if (playing) {
      stop();
    } else {
      if (current >= messages.length - 1) setCurrent(0);
      setPlaying(true);
    }
  };

  const step = (dir: 1 | -1) => {
    stop();
    setCurrent((c) => Math.min(messages.length - 1, Math.max(0, c + dir)));
  };

  const visible = messages.slice(0, current + 1);
  const total = messages.length;

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', fontFamily: 'system-ui, sans-serif' }}>
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 8,
          padding: 10,
          borderBottom: '1px solid #e2e8f0',
          background: '#fff',
        }}
      >
        <button onClick={() => step(-1)} disabled={current <= 0} style={btnStyle}>
          ◀ 上一步
        </button>
        <button onClick={toggle} style={{ ...btnStyle, background: '#6366f1', color: '#fff' }}>
          {playing ? '⏸ 暂停' : '▶ 播放'}
        </button>
        <button onClick={() => step(1)} disabled={current >= total - 1} style={btnStyle}>
          下一步 ▶
        </button>
        <div style={{ marginLeft: 'auto', fontSize: 12, color: '#64748b' }}>
          {total === 0 ? '0 / 0' : `${current + 1} / ${total}`}
        </div>
      </div>
      <div style={{ flex: 1, overflow: 'auto', padding: 16 }}>
        {total === 0 ? (
          <div style={{ color: '#94a3b8', fontSize: 13, textAlign: 'center', padding: 24 }}>
            暂无可回放的消息
          </div>
        ) : (
          <MessageFlow messages={visible} agents={agents} />
        )}
      </div>
    </div>
  );
}

const btnStyle: CSSProperties = {
  padding: '6px 12px',
  borderRadius: 6,
  border: '1px solid #cbd5e1',
  background: '#fff',
  fontSize: 12,
  cursor: 'pointer',
};

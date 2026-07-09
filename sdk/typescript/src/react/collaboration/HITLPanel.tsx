/**
 * HITLPanel — 人类介入（Human-in-the-Loop）审批面板（T3-4）。
 *
 * 展示待审批请求，提供通过 / 拒绝按钮。无待审批时显示空闲状态。
 */

import type { ReactElement } from 'react';
import type { ApprovalRequest } from './CollaborationView.js';

export interface HITLPanelProps {
  approvals: ApprovalRequest[];
  onApprove?: (id: string) => void | Promise<void>;
  onReject?: (id: string) => void | Promise<void>;
}

export function HITLPanel({ approvals, onApprove, onReject }: HITLPanelProps): ReactElement {
  return (
    <footer
      style={{
        borderTop: '1px solid #e2e8f0',
        background: '#fff',
        padding: 12,
        maxHeight: 220,
        overflowY: 'auto',
      }}
    >
      <div style={{ fontSize: 13, fontWeight: 700, color: '#0f172a', marginBottom: 8 }}>
        人类介入 · 待审批（{approvals.length}）
      </div>
      {approvals.length === 0 ? (
        <div style={{ fontSize: 12, color: '#94a3b8' }}>无待审批事项</div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
          {approvals.map((a) => (
            <div
              key={a.id}
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 10,
                padding: '8px 10px',
                borderRadius: 8,
                border: '1px solid #fde68a',
                background: '#fffbeb',
              }}
            >
              <div style={{ flex: 1 }}>
                <div style={{ fontSize: 13, color: '#0f172a' }}>{a.title}</div>
                {a.detail && (
                  <div style={{ fontSize: 12, color: '#64748b' }}>{a.detail}</div>
                )}
              </div>
              <button
                onClick={() => onApprove?.(a.id)}
                style={{
                  padding: '6px 12px',
                  borderRadius: 6,
                  border: 'none',
                  background: '#22c55e',
                  color: '#fff',
                  fontSize: 12,
                  cursor: 'pointer',
                }}
              >
                通过
              </button>
              <button
                onClick={() => onReject?.(a.id)}
                style={{
                  padding: '6px 12px',
                  borderRadius: 6,
                  border: 'none',
                  background: '#ef4444',
                  color: '#fff',
                  fontSize: 12,
                  cursor: 'pointer',
                }}
              >
                拒绝
              </button>
            </div>
          ))}
        </div>
      )}
    </footer>
  );
}

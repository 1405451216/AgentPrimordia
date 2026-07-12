/**
 * AgentPanelServer �� React Server Component �� Agent ���
 *
 * �ڷ������� fetch Agent ��Ϣ����Ⱦ������ͻ��� JavaScript��
 * ������ Next.js App Router / �κ� RSC ����ʱ��
 *
 * ʹ�÷�ʽ��
 *   // app/agent/[id]/page.tsx (Server Component)
 *   import { AgentPanelServer } from '@agentprimordia/sdk/react';
 *   export default async function Page({ params }) {
 *     return <AgentPanelServer agentId={params.id} />;
 *   }
 *
 * ���Ҫ�㣺
 * - �첽 Server Component����Ȼ���� Suspense
 * - �������κοͻ��� hooks
 * - ��ͨ�� props �Զ��� API �˵�
 */

import type { ReactElement } from 'react';
import type { AgentInfo } from '../playground/index.js';

/** AgentPanelServer ���� */
export interface AgentPanelServerProps {
  /** Agent ID */
  agentId: string;
  /** API ����·�� */
  apiBase?: string;
}

/**
 * AgentView �� �ڲ�չʾ���
 */
function AgentView({ agent, error }: { agent: AgentInfo | null; error?: string }): ReactElement {
  if (error) {
    return <div role="alert" style={{ color: '#dc2626', padding: 16 }}>Agent Error: {error}</div>;
  }
  if (!agent) {
    return <div aria-busy="true" style={{ padding: 16 }}>Loading agent...</div>;
  }
  return (
    <div className="ap-agent-panel" style={{ padding: 16, borderRadius: 8, border: '1px solid #e5e7eb' }}>
      <header style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
        <h3 style={{ margin: 0 }}>{agent.id}</h3>
        <span className={"badge badge-" + (agent.status ?? 'idle')} style={{ padding: '2px 8px', borderRadius: 4, background: '#f3f4f6' }}>
          {agent.status ?? 'idle'}
        </span>
        {agent.model && <span className="model-tag" style={{ fontSize: 12, color: '#6b7280' }}>{agent.model}</span>}
      </header>
    </div>
  );
}

/**
 * AgentPanelServer �� RSC �� Agent ���
 *
 * �ڷ������� fetch Agent ��Ϣ��ֱ����ȾΪ��̬ HTML��
 */
export async function AgentPanelServer({
  agentId,
  apiBase = 'http://localhost:8080',
}: AgentPanelServerProps): Promise<ReactElement> {
  let agent: AgentInfo | null = null;
  let error: string | undefined;

  try {
    const resp = await fetch(apiBase + '/api/playground/agents/' + encodeURIComponent(agentId));
    if (!resp.ok) throw new Error('HTTP ' + resp.status);
    agent = (await resp.json()) as AgentInfo;
  } catch (e) {
    error = e instanceof Error ? e.message : String(e);
  }

  return <AgentView agent={agent} error={error} />;
}

export default AgentPanelServer;

// AgentPanel.tsx - React Server Component
import type { ReactNode } from 'react';
import type { AgentInfo } from '../playground/index.js';

export interface AgentPanelProps {
  agentId: string;
  apiBase?: string;
}

interface AgentViewProps {
  agent: AgentInfo | null;
  error?: string;
}

function AgentView({ agent, error }: AgentViewProps): ReactNode {
  if (error) {
    return <div role="alert" style={{ color: "#dc2626" }}>Agent Error: {error}</div>;
  }
  if (!agent) {
    return <div aria-busy="true">Loading agent...</div>;
  }
  return (
    <div className="ap-agent-panel">
      <header>
        <h3>{agent.id}</h3>
        <span className={"badge badge-" + agent.status}>{agent.status}</span>
        <span className="model-tag">{agent.model}</span>
      </header>
    </div>
  );
}

export async function AgentPanel({ agentId, apiBase = "http://localhost:8080" }: AgentPanelProps): Promise<ReactNode> {
  let agent: AgentInfo | null = null;
  let error: string | undefined;
  try {
    const resp = await fetch(apiBase + "/api/playground/agents/" + encodeURIComponent(agentId));
    if (!resp.ok) throw new Error("HTTP " + resp.status);
    agent = await resp.json() as AgentInfo;
  } catch (e) {
    error = e instanceof Error ? e.message : String(e);
  }
  return <AgentView agent={agent} error={error} />;
}

export function StreamingOutput({ stream }: { stream: ReadableStream<Uint8Array> }): ReactNode {
  return (
    <div className="ap-streaming-output" data-stream="sse" aria-live="polite">
      <span className="sr-only">Streaming agent response...</span>
    </div>
  );
}

export default AgentPanel;

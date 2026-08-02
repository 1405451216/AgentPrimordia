// A2A Open Interop — TypeScript 对等实现（v3.5）
// Mirrors Go internal/agent/a2a/interop_*.go

export interface OpenCapabilities {
  streaming: boolean;
  pushNotifications?: boolean;
  stateTransitionHistory?: boolean;
}

export interface OpenSkillDecl {
  id: string;
  name: string;
  description: string;
  tags?: string[];
  examples?: string[];
}

export interface OpenAgentCard {
  name: string;
  description: string;
  url: string;
  version: string;
  capabilities: OpenCapabilities;
  skills?: OpenSkillDecl[];
  defaultInputModes?: string[];
  defaultOutputModes?: string[];
}

export interface OpenPart {
  type: 'text' | 'file' | 'data';
  text?: string;
  data?: Record<string, unknown>;
}

export interface OpenMessage {
  role: string;
  parts: OpenPart[];
  metadata?: Record<string, unknown>;
}

export function newTextMessage(role: string, text: string): OpenMessage {
  return { role, parts: [{ type: 'text', text }] };
}

export function textContent(m: OpenMessage): string {
  return m.parts.find(p => p.type === 'text')?.text ?? '';
}

export type OpenTaskState =
  | 'submitted' | 'working' | 'input-required'
  | 'completed' | 'canceled' | 'failed';

export function isTerminalState(s: OpenTaskState): boolean {
  return s === 'completed' || s === 'canceled' || s === 'failed';
}

export interface OpenTaskStatus {
  state: OpenTaskState;
  message?: OpenMessage;
  timestamp: string;
}

export interface OpenTask {
  id: string;
  contextId?: string;
  status: OpenTaskStatus;
  messages?: OpenMessage[];
}

export interface OpenError {
  code: number;
  message: string;
  data?: unknown;
}

// 标准错误码
export const OpenErr = {
  ParseError: -32700,
  InvalidRequest: -32600,
  MethodNotFound: -32601,
  InvalidParams: -32602,
  Internal: -32603,
  TaskNotFound: -32001,
  TaskAlreadyCanceled: -32002,
} as const;

// 开放协议客户端
export class OpenInteropClient {
  constructor(private baseURL: string) {}

  async fetchAgentCard(): Promise<OpenAgentCard> {
    const res = await fetch(`${this.baseURL}/.well-known/agent.json`);
    if (!res.ok) throw new Error(`agent card ${res.status}`);
    return res.json() as Promise<OpenAgentCard>;
  }

  private async rpc<T>(method: string, params: unknown): Promise<T> {
    const res = await fetch(`${this.baseURL}/a2a/v1`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ jsonrpc: '2.0', method, id: 1, params }),
    });
    const body = await res.json() as { result?: T; error?: OpenError };
    if (body.error) throw body.error;
    return body.result as T;
  }

  sendTask(message: OpenMessage): Promise<OpenTask> {
    return this.rpc<OpenTask>('tasks/send', { message });
  }

  getTask(taskId: string): Promise<OpenTask> {
    return this.rpc<OpenTask>('tasks/get', { taskId });
  }

  cancelTask(taskId: string): Promise<OpenTask> {
    return this.rpc<OpenTask>('tasks/cancel', { taskId });
  }
}

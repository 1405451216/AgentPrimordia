/**
 * Playground 类型与辅助组件定义。
 */

export interface PlaygroundConfig {
  apiBase: string;
  defaultModel: string;
}

export interface AgentConfig {
  name: string;
  model?: string;
  systemPrompt?: string;
  tools?: string[];
  maxTurns?: number;
}

export interface AgentInfo {
  id: string;
  model: string;
  status: string;
}

export interface ChatResponse {
  response: string;
  tokens: number;
}

export interface AgentStats {
  turnCount: number;
  totalTokens: number;
}

export interface TokenEvent {
  type: 'token';
  content: string;
}

export interface ToolCallEvent {
  type: 'tool_call';
  tool: string;
  args: unknown;
}

export interface ErrorEvent {
  type: 'error';
  message: string;
}

export interface DoneEvent {
  type: 'done';
}

export type StreamEvent =
  | TokenEvent
  | ToolCallEvent
  | ErrorEvent
  | DoneEvent;

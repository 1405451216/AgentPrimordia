/**
 * Playground 客户端 — AgentPrimordia Playground 的 TypeScript SDK。
 *
 * 提供与 AgentPrimordia Playground HTTP API 交互的高级封装，包括：
 * - Agent 生命周期管理（创建/删除/列表）
 * - 同步 / 流式对话
 * - SSE 事件订阅
 * - Agent 运行统计
 *
 * 内部使用 fetch + ReadableStream 实现 SSE 客户端，内置超时与自动重连。
 */

import type {
  PlaygroundConfig,
  AgentConfig,
  AgentInfo,
  ChatResponse,
  AgentStats,
  StreamEvent,
} from './components.js';

export type {
  PlaygroundConfig, AgentConfig, AgentInfo, ChatResponse, AgentStats, StreamEvent,
  TokenEvent, ToolCallEvent, ErrorEvent, DoneEvent,
} from './components.js';

/** 默认请求超时（毫秒） */
const DEFAULT_TIMEOUT_MS = 30_000;
/** SSE 重连基础延迟（毫秒） */
const RECONNECT_BASE_MS = 1_000;
/** 最大重连次数 */
const MAX_RECONNECT_ATTEMPTS = 3;

/**
 * Playground 客户端，封装与 Playground API 的所有交互。
 *
 * 用法：
 *   const pg = new PlaygroundClient({ apiBase: "http://localhost:8080", defaultModel: "gpt-4" });
 *   const agent = await pg.createAgent({ name: "assistant" });
 *   const reply = await pg.chat(agent.id, "Hello");
 *   console.log(reply.response);
 */
export class PlaygroundClient {
  private readonly apiBase: string;
  private readonly defaultModel: string;
  private readonly timeoutMs: number;

  constructor(config: PlaygroundConfig, opts?: { timeoutMs?: number }) {
    this.apiBase = config.apiBase.replace(/[\\/]+$/, '');
    this.defaultModel = config.defaultModel;
    this.timeoutMs = opts?.timeoutMs ?? DEFAULT_TIMEOUT_MS;
  }

  // ===== Agent 生命周期 =====

  /** 创建一个新的 Agent 实例 */
  async createAgent(config: AgentConfig): Promise<AgentInfo> {
    return this.post<AgentInfo>('/api/playground/agents', {
      name: config.name,
      model: config.model ?? this.defaultModel,
      system_prompt: config.systemPrompt,
      tools: config.tools,
      max_turns: config.maxTurns,
    });
  }

  /** 删除指定 Agent */
  async deleteAgent(agentID: string): Promise<void> {
    await this.request<void>('DELETE', `/api/playground/agents/${encodeURIComponent(agentID)}`);
  }

  /** 列出全部 Agent */
  async listAgents(): Promise<AgentInfo[]> {
    return this.get<AgentInfo[]>('/api/playground/agents');
  }

  /** 查询单个 Agent 详情 */
  async getAgent(agentID: string): Promise<AgentInfo> {
    return this.get<AgentInfo>(`/api/playground/agents/${encodeURIComponent(agentID)}`);
  }

  // ===== 对话 =====

  /** 同步对话，返回完整回复 */
  async chat(agentID: string, message: string): Promise<ChatResponse> {
    return this.post<ChatResponse>(`/api/playground/agents/${encodeURIComponent(agentID)}/chat`, {
      message,
    });
  }

  /**
   * 流式对话，返回 AsyncGenerator 产出 StreamEvent。
   * 使用 ReadableStream 消费 SSE。
   */
  async *streamChat(agentID: string, message: string): AsyncGenerator<StreamEvent> {
    const url = `${this.apiBase}/api/playground/agents/${encodeURIComponent(agentID)}/stream`;
    const resp = await fetch(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Accept: 'text/event-stream' },
      body: JSON.stringify({ message }),
      signal: AbortSignal.timeout(this.timeoutMs),
    });

    if (!resp.ok) {
      const text = await resp.text().catch(() => '');
      throw new Error(`Playground stream HTTP ${resp.status}: ${text}`);
    }
    if (!resp.body) throw new Error('Response body is null');

    yield* parseSSE(resp.body);
  }

  /** 获取 Agent 运行统计数据 */
  async getStats(agentID: string): Promise<AgentStats> {
    return this.get<AgentStats>(`/api/playground/agents/${encodeURIComponent(agentID)}/stats`);
  }
  /**
   * 订阅 Agent 所有事件（SSE）。
   * 用于实时监听 tool_call / error / done 等事件。
   */
  async *streamEvents(agentID: string): AsyncGenerator<StreamEvent> {
    const url = `${this.apiBase}/api/playground/agents/${encodeURIComponent(agentID)}/events`;
    const resp = await fetch(url, {
      method: 'GET',
      headers: { Accept: 'text/event-stream' },
      signal: AbortSignal.timeout(this.timeoutMs),
    });

    if (!resp.ok) {
      const text = await resp.text().catch(() => '');
      throw new Error(`Playground events HTTP ${resp.status}: ${text}`);
    }
    if (!resp.body) throw new Error('Response body is null');

    yield* parseSSE(resp.body);
  }

  // ===== 内部 HTTP 方法 =====

  private async get<T>(path: string): Promise<T> {
    return this.request<T>('GET', path);
  }

  private async post<T>(path: string, body: unknown): Promise<T> {
    return this.request<T>('POST', path, body);
  }

  private async request<T>(method: string, path: string, body?: unknown): Promise<T> {
    const headers: Record<string, string> = { 'Content-Type': 'application/json' };
    const init: RequestInit = { method, headers };
    if (body !== undefined) init.body = JSON.stringify(body);
    init.signal = AbortSignal.timeout(this.timeoutMs);

    let lastError: Error | undefined;
    for (let attempt = 0; attempt <= MAX_RECONNECT_ATTEMPTS; attempt++) {
      try {
        const resp = await fetch(this.apiBase + path, init);
        if (!resp.ok) {
          const text = await resp.text().catch(() => '');
          throw new Error(`Playground API HTTP ${resp.status}: ${text}`);
        }
        if (resp.status === 204) return undefined as T;
        return (await resp.json()) as T;
      } catch (err) {
        lastError = err instanceof Error ? err : new Error(String(err));
        if (respErrorIsHTTP(err)) throw lastError;
        if (attempt < MAX_RECONNECT_ATTEMPTS) {
          const delay = RECONNECT_BASE_MS * 2 ** attempt;
          await new Promise((r) => setTimeout(r, delay));
        }
      }
    }
    throw lastError;
  }
}



// ===== SSE 解析 =====

/**
 * 解析 SSE ReadableStream，产出 StreamEvent。
 *
 * SSE 格式：
 *   event: token
 *   data: {"content":"hello"}
 *
 * 本函数逐行读取 buffer，按 double-newline 切分 event，
 * 再从 event 行中提取 event 字段与 data 字段，解析为 StreamEvent。
 */
async function* parseSSE(stream: ReadableStream<Uint8Array>): AsyncGenerator<StreamEvent> {
  const reader = stream.getReader();
  const decoder = new TextDecoder();
  let buffer = '';

  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });
      const blocks = buffer.split('\n\n');
      buffer = blocks.pop() ?? '';

      for (const block of blocks) {
        const event = parseSSENode(block);
        if (event) yield event;
      }
    }
  } finally {
    reader.releaseLock();
  }

  if (buffer.trim()) {
    const event = parseSSENode(buffer);
    if (event) yield event;
  }
}

/** 解析单条 SSE event 块，返回 StreamEvent 或 null */
function parseSSENode(block: string): StreamEvent | null {
  let eventType = 'message';
  const dataLines: string[] = [];

  for (const rawLine of block.split('\n')) {
    const line = rawLine.trimEnd();
    if (line.startsWith('event:')) {
      eventType = line.slice(6).trim();
    } else if (line.startsWith('data:')) {
      dataLines.push(line.slice(5).trim());
    }
  }

  if (dataLines.length === 0) return null;
  const dataStr = dataLines.join('\n');
  if (dataStr === '[DONE]') return { type: 'done' };

  let payload: unknown;
  try {
    payload = JSON.parse(dataStr);
  } catch {
    // 非 JSON 数据跳过
    return null;
  }

  switch (eventType) {
    case 'token':
      return { type: 'token', content: (payload as { content: string }).content ?? String(payload) };
    case 'tool_call':
      return { type: 'tool_call', tool: (payload as { tool: string }).tool, args: (payload as { args?: unknown }).args ?? null };
    case 'error':
      return { type: 'error', message: (payload as { message: string }).message ?? String(payload) };
    case 'done':
      return { type: 'done' };
    default:
      if (payload && typeof payload === 'object' && 'type' in payload) {
        return payload as StreamEvent;
      }
      return null;
  }
}

/** 判断异常是否为 HTTP 错误（不应重试） */
function respErrorIsHTTP(err: unknown): boolean {
  if (err instanceof Error) {
    return /HTTP \d{3}/.test(err.message);
  }
  return false;
}

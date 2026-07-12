/**
 * Playground CLI 前端 鈥?多 Agent 会话管理器。
 *
 * 提供与 AgentPrimordia Playground API 交互的会话层封装：
 * - PlaygroundSession: 单个 Agent 的对话会话，支持 SSE 流式、abort、统计
 * - PlaygroundManager: 多 Agent 生命周期管理（创建、列表）
 *
 * 与已有 PlaygroundClient（高级封装）互补，本模块聚焦 CLI 场景下的
 * 轻量会话管理与批量操作。
 */

/** Playground 中的 Agent 状态 */
export interface PlaygroundAgent {
  id: string;
  name: string;
  model: string;
  status: 'connecting' | 'idle' | 'thinking' | 'error';
  turnCount: number;
  totalTokens: number;
  lastActive?: Date;
}

/** 会话中的消息 */
export interface PlaygroundMessage {
  role: 'user' | 'assistant' | 'system';
  content: string;
  timestamp: Date;
  tokens?: number;
}

/**
 * 单 Agent 会话。
 *
 * 一个 PlaygroundSession 对应一个 Agent 实体的完整对话历史。
 * 支持：
 * - 同步 chat（返回完整响应）
 * - SSE 流式 chat（逐 token 回调）
 * - AbortController 中断
 * - 本地历史管理与 JSON 导出
 * - 远程统计刷新
 */
export class PlaygroundSession {
  agent: PlaygroundAgent;
  messages: PlaygroundMessage[] = [];
  private abortController: AbortController | null = null;

  constructor(agent: PlaygroundAgent) {
    this.agent = agent;
  }

  /**
   * 发送一条用户消息并获取 Agent 回复。
   *
   * 流程：
   * 1. 将用户消息追加到 history
   * 2. POST 到远端 /chat
   * 3. 根据 Content-Type 判断 SSE 流式或 JSON 同步
   * 4. 将 assistant 回复追加到 history
   *
   * @param message 用户输入
   * @param onToken 流式模式下的逐 token 回调（可选）
   * @returns 完整回复文本
   */
  async send(message: string, onToken?: (token: string) => void): Promise<string> {
    this.agent.status = 'thinking';
    this.messages.push({ role: 'user', content: message, timestamp: new Date() });

    this.abortController = new AbortController();
    let response = '';

    try {
      const resp = await fetch(`${this.apiBase()}/chat`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ agent_id: this.agent.id, message }),
        signal: this.abortController.signal,
      });

      if (!resp.ok) throw new Error(`HTTP ${resp.status}`);

      if (resp.headers.get('content-type')?.includes('text/event-stream')) {
        // SSE 流式解析
        const reader = resp.body?.getReader();
        if (reader) {
          const decoder = new TextDecoder();
          while (true) {
            const { done, value } = await reader.read();
            if (done) break;
            const text = decoder.decode(value, { stream: true });
            for (const line of text.split('\n')) {
              if (line.startsWith('data: ')) {
                const chunk = line.substring(6);
                if (chunk === '[DONE]') break;
                response += chunk;
                onToken?.(chunk);
              }
            }
          }
        }
      } else {
        // JSON 同步响应
        const data = await resp.json();
        response = data.response || data.content || '';
      }

      this.agent.turnCount++;
      this.messages.push({ role: 'assistant', content: response, timestamp: new Date() });
      this.agent.status = 'idle';
      return response;
    } catch (e: any) {
      if (e.name === 'AbortError') {
        this.agent.status = 'idle';
        return response + ' [aborted]';
      }
      this.agent.status = 'error';
      throw e;
    }
  }

  /**
   * 刷新 Agent 运行统计数据（turnCount / totalTokens）。
   * 仅当 Agent 处于 idle 状态时执行。
   */
  async streamStats(): Promise<void> {
    if (this.agent.status !== 'idle') return;
    try {
      const resp = await fetch(`${this.apiBase()}/stats?agent_id=${this.agent.id}`);
      if (resp.ok) {
        const stats = await resp.json();
        this.agent.turnCount = stats.turn_count ?? stats.turnCount ?? this.agent.turnCount;
        this.agent.totalTokens = stats.total_tokens ?? stats.totalTokens ?? this.agent.totalTokens;
      }
    } catch {
      // 统计加载失败不影响主流程
    }
  }

  /** 中断当前请求 */
  abort() {
    this.abortController?.abort();
  }

  /** 清空对话历史 */
  clearHistory() {
    this.messages = [];
  }

  /** 导出会话为 JSON 字符串（包含 agent 信息与完整 history）*/
  exportAsJSON(): string {
    return JSON.stringify(
      { agent: this.agent, messages: this.messages },
      null,
      2
    );
  }

  private apiBase(): string {
    return process.env.PLAYGROUND_API || 'http://localhost:3000/api/playground/agent';
  }
}

/**
 * 多 Agent 管理器。
 *
 * 提供批量 Agent 创建、列表查询能力，内部维护 session 映射。
 */
export class PlaygroundManager {
  sessions = new Map<string, PlaygroundSession>();

  /**
   * 创建一个新的 Agent 并建立会话。
   *
   * @param config Agent 配置（name, model, tools）
   * @returns 新 Agent 的 ID
   */
  async createAgent(config: { name: string; model: string; tools?: string[] }): Promise<string> {
    const resp = await fetch(`${this.apiBase()}/create`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(config),
    });
    if (!resp.ok) {
      throw new Error(`Failed to create agent: HTTP ${resp.status}`);
    }
    const data = await resp.json();
    const agent: PlaygroundAgent = {
      id: data.id,
      name: config.name,
      model: config.model,
      status: 'idle',
      turnCount: 0,
      totalTokens: 0,
    };
    this.sessions.set(agent.id, new PlaygroundSession(agent));
    return agent.id;
  }

  /**
   * 列出远端全部 Agent。
   */
  async listAgents(): Promise<PlaygroundAgent[]> {
    const resp = await fetch(`${this.apiBase()}/list`);
    if (!resp.ok) {
      throw new Error(`Failed to list agents: HTTP ${resp.status}`);
    }
    const data = await resp.json();
    return data.agents || [];
  }

  /** 通过 ID 获取本地缓存的 session（如有）*/
  getSession(agentId: string): PlaygroundSession | undefined {
    return this.sessions.get(agentId);
  }

  private apiBase(): string {
    return `${process.env.PLAYGROUND_API || 'http://localhost:3000/api/playground'}`;
  }
}
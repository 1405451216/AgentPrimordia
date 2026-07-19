/**
 * AgentPrimordia Studio REST API 客户端。
 *
 * 职责：
 * 1. 封装对 Studio 后端的基础 CRUD 操作（runs / templates / agents）
 * 2. 为 ChatPanel / RunHistory / StatusBar 提供统一数据源
 *
 * 纯逻辑层：不依赖 vscode API，可在 Node 环境直接测试。
 */

/** 单次 Agent 运行记录 */
export interface Run {
  id: string;
  template: string;
  agent: string;
  message: string;
  turns: number;
  tokens: number;
  cost: number;
  status: 'pending' | 'running' | 'done' | 'error';
  startedAt: number;
  endedAt: number | null;
  error?: string;
}

/** Agent 模板概要 */
export interface TemplateSummary {
  name: string;
  description: string;
  agent: string;
}

/** 分页列表响应 */
export interface Paginated<T> {
  items: T[];
  total: number;
}

/** SSE 流事件（与 Studio /runs/:id/stream 接口对齐） */
export interface StreamEvent {
  type: 'token' | 'thought' | 'action' | 'observation' | 'done' | 'error';
  text?: string;
  tool?: string;
  args?: unknown;
}

/** 客户端错误 */
export class StudioApiError extends Error {
  constructor(
    message: string,
    public readonly status: number,
  ) {
    super(message);
    this.name = 'StudioApiError';
  }
}

/** Studio REST API 客户端 */
export class StudioApi {
  constructor(
    private readonly baseUrl: string,
    private readonly apiKey: string,
  ) {}

  /** 公共 fetch 封装：自动注入认证头与错误处理 */
  private async request<T>(path: string, init: RequestInit = {}): Promise<T> {
    const url = `${this.baseUrl.replace(/\/+$/, '')}${path}`;
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      ...(init.headers as Record<string, string> | undefined),
    };
    if (this.apiKey) headers['Authorization'] = `Bearer ${this.apiKey}`;

    const res = await fetch(url, { ...init, headers });
    if (!res.ok) {
      const body = await res.text().catch(() => '');
      throw new StudioApiError(
        `Studio API ${res.status}: ${body || res.statusText}`,
        res.status,
      );
    }
    return (await res.json()) as T;
  }

  /** 查询最近运行记录 */
  async getRuns(limit = 20): Promise<Paginated<Run>> {
    return this.request<Paginated<Run>>(`/api/runs?limit=${limit}`);
  }

  /** 查询单条运行详情 */
  async getRun(id: string): Promise<Run> {
    return this.request<Run>(`/api/runs/${encodeURIComponent(id)}`);
  }

  /** 列出可用模板 */
  async getTemplates(): Promise<Paginated<TemplateSummary>> {
    return this.request<Paginated<TemplateSummary>>('/api/templates');
  }

  /** 启动一次 Agent 运行（非流式入口） */
  async startRun(template: string, message: string): Promise<Run> {
    return this.request<Run>('/api/runs', {
      method: 'POST',
      body: JSON.stringify({ template, message }),
    });
  }

  /**
   * 订阅 SSE 流，返回 Response 以便调用方通过 ReadableStream 读取。
   * 失败时抛出 StudioApiError。
   */
  async streamRun(id: string): Promise<Response> {
    const url = `${this.baseUrl.replace(/\/+$/, '')}/api/runs/${encodeURIComponent(id)}/stream`;
    const headers: Record<string, string> = { Accept: 'text/event-stream' };
    if (this.apiKey) headers['Authorization'] = `Bearer ${this.apiKey}`;
    const res = await fetch(url, { headers });
    if (!res.ok) {
      throw new StudioApiError(`Stream ${res.status}: ${res.statusText}`, res.status);
    }
    return res;
  }
}

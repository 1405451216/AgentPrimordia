// distributed.ts — 分布式 Agent 执行
// 包含 Agent 发现、Token 认证、分布式通信
// Mirrors Go internal/agent/discovery.go + discovery_auth.go

import * as crypto from 'node:crypto';

// ===== Agent 信息 =====

export interface AgentInfo {
  /** Agent 唯一标识 */
  id: string;
  /** Agent 名称 */
  name: string;
  /** Agent 地址 */
  address: string;
  /** 能力标签（含角色） */
  capabilities: string[];
  /** 元数据 */
  metadata?: Record<string, string>;
  /** 最后心跳时间 */
  lastHeartbeat?: Date;
  /** Agent 状态 */
  status?: 'online' | 'offline' | 'busy';
}

// ===== Agent 身份 =====

export interface AgentIdentity {
  id: string;
  name: string;
  roles: string[];
  metadata?: Record<string, string>;
}

// ===== 发现接口 =====

export interface Discovery {
  /** 注册 Agent */
  register(info: AgentInfo): Promise<void>;
  /** 注销 Agent */
  unregister(agentId: string): Promise<void>;
  /** 发现 Agent */
  discover(agentId: string): Promise<AgentInfo | null>;
  /** 列出所有 Agent */
  listAgents(): Promise<AgentInfo[]>;
  /** 发送心跳 */
  heartbeat(agentId: string): Promise<void>;
}

// ===== 本地发现实现 =====

export class LocalDiscovery implements Discovery {
  private agents: Map<string, AgentInfo> = new Map();
  private heartbeatTimers: Map<string, NodeJS.Timeout> = new Map();
  private heartbeatTimeout: number;

  constructor(heartbeatTimeoutMs: number = 30000) {
    this.heartbeatTimeout = heartbeatTimeoutMs;
  }

  async register(info: AgentInfo): Promise<void> {
    const now = new Date();
    this.agents.set(info.id, {
      ...info,
      lastHeartbeat: now,
      status: 'online',
    });

    // 启动心跳监控
    this.startHeartbeatMonitor(info.id);
  }

  async unregister(agentId: string): Promise<void> {
    this.agents.delete(agentId);
    this.stopHeartbeatMonitor(agentId);
  }

  async discover(agentId: string): Promise<AgentInfo | null> {
    return this.agents.get(agentId) ?? null;
  }

  async listAgents(): Promise<AgentInfo[]> {
    return Array.from(this.agents.values());
  }

  async heartbeat(agentId: string): Promise<void> {
    const agent = this.agents.get(agentId);
    if (agent) {
      this.agents.set(agentId, {
        ...agent,
        lastHeartbeat: new Date(),
        status: 'online',
      });
    }
  }

  /** 按角色列出 Agent */
  listAgentsByRole(role: string): AgentInfo[] {
    return Array.from(this.agents.values()).filter((a) =>
      a.capabilities.includes(role),
    );
  }

  /** 关闭发现服务 */
  close(): void {
    for (const [id] of this.heartbeatTimers) {
      this.stopHeartbeatMonitor(id);
    }
    this.agents.clear();
  }

  private startHeartbeatMonitor(agentId: string): void {
    const timer = setInterval(() => {
      const agent = this.agents.get(agentId);
      if (agent && agent.lastHeartbeat) {
        const elapsed = Date.now() - agent.lastHeartbeat.getTime();
        if (elapsed > this.heartbeatTimeout) {
          agent.status = 'offline';
          this.agents.set(agentId, agent);
        }
      }
    }, this.heartbeatTimeout / 2);

    this.heartbeatTimers.set(agentId, timer);
  }

  private stopHeartbeatMonitor(agentId: string): void {
    const timer = this.heartbeatTimers.get(agentId);
    if (timer) {
      clearInterval(timer);
      this.heartbeatTimers.delete(agentId);
    }
  }
}

// ===== HTTP 发现客户端 =====

export class HTTPDiscoveryClient implements Discovery {
  private baseURL: string;
  private apiKey?: string;

  constructor(baseURL: string, apiKey?: string) {
    this.baseURL = baseURL.replace(/\/$/, '');
    this.apiKey = apiKey;
  }

  async register(info: AgentInfo): Promise<void> {
    const resp = await this.fetch('/agents', {
      method: 'POST',
      body: JSON.stringify(info),
    });
    if (!resp.ok) {
      throw new Error(`register failed: ${resp.status} ${await resp.text()}`);
    }
  }

  async unregister(agentId: string): Promise<void> {
    const resp = await this.fetch(`/agents/${encodeURIComponent(agentId)}`, {
      method: 'DELETE',
    });
    if (!resp.ok) {
      throw new Error(`unregister failed: ${resp.status} ${await resp.text()}`);
    }
  }

  async discover(agentId: string): Promise<AgentInfo | null> {
    const resp = await this.fetch(`/agents/${encodeURIComponent(agentId)}`);
    if (resp.status === 404) return null;
    if (!resp.ok) {
      throw new Error(`discover failed: ${resp.status} ${await resp.text()}`);
    }
    return resp.json();
  }

  async listAgents(): Promise<AgentInfo[]> {
    const resp = await this.fetch('/agents');
    if (!resp.ok) {
      throw new Error(`list failed: ${resp.status} ${await resp.text()}`);
    }
    return resp.json();
  }

  async heartbeat(agentId: string): Promise<void> {
    const resp = await this.fetch(
      `/agents/${encodeURIComponent(agentId)}/heartbeat`,
      { method: 'POST' },
    );
    if (!resp.ok) {
      throw new Error(`heartbeat failed: ${resp.status} ${await resp.text()}`);
    }
  }

  private async fetch(path: string, init?: RequestInit): Promise<Response> {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      ...(init?.headers as Record<string, string>),
    };
    if (this.apiKey) {
      headers['Authorization'] = `Bearer ${this.apiKey}`;
    }
    return fetch(`${this.baseURL}${path}`, {
      ...init,
      headers,
    });
  }
}

// ===== Token 认证器 =====

export class TokenAuthenticator {
  private secret: Buffer;

  constructor(secret: string) {
    this.secret = Buffer.from(secret);
  }

  /** 为 Agent 身份生成签名 token */
  generateToken(identity: AgentIdentity): string {
    const payload = Buffer.from(JSON.stringify(identity));
    const hmac = crypto.createHmac('sha256', this.secret);
    hmac.update(payload);
    const signature = hmac.digest();
    return `${payload.toString('base64url')}.${signature.toString('base64url')}`;
  }

  /** 验证 token 并返回 Agent 身份 */
  authenticate(token: string): AgentIdentity {
    const parts = token.split('.', 2);
    if (parts.length !== 2) {
      throw new Error('token 格式无效');
    }

    const payload = Buffer.from(parts[0], 'base64url');
    const signature = Buffer.from(parts[1], 'base64url');

    // 验证签名
    const hmac = crypto.createHmac('sha256', this.secret);
    hmac.update(payload);
    const expected = hmac.digest();

    if (!crypto.timingSafeEqual(signature, expected)) {
      throw new Error('token 签名无效');
    }

    return JSON.parse(payload.toString());
  }
}

// ===== 带认证的发现服务 =====

/** 带认证的发现服务（不实现 Discovery 接口，因为 register/unregister 需要额外 token 参数） */
export class AuthenticatedDiscovery {
  private inner: Discovery;
  private auth: TokenAuthenticator;
  private agentTokens: Map<string, string> = new Map();

  constructor(inner: Discovery, auth: TokenAuthenticator) {
    this.inner = inner;
    this.auth = auth;
  }

  async register(info: AgentInfo, token: string): Promise<void> {
    const identity = this.auth.authenticate(token);

    if (identity.id !== info.id) {
      throw new Error('token 身份与注册信息不匹配');
    }

    // 将 identity 的角色同步到 info.capabilities
    if (info.capabilities.length === 0 && identity.roles.length > 0) {
      info.capabilities = identity.roles;
    }

    await this.inner.register(info);
    this.agentTokens.set(info.id, token);
  }

  async discover(agentId: string): Promise<AgentInfo | null> {
    return this.inner.discover(agentId);
  }

  async listAgents(): Promise<AgentInfo[]> {
    return this.inner.listAgents();
  }

  async unregister(agentId: string, token: string): Promise<void> {
    this.auth.authenticate(token);
    this.agentTokens.delete(agentId);
    return this.inner.unregister(agentId);
  }

  async heartbeat(agentId: string): Promise<void> {
    return this.inner.heartbeat(agentId);
  }

  /** 按角色列出 Agent */
  listAgentsByRole(role: string): Promise<AgentInfo[]> {
    return this.inner.listAgents().then((agents) =>
      agents.filter((a) => a.capabilities.includes(role)),
    );
  }
}

// ===== 分布式消息总线 =====

export type BusMessageType =
  | 'task_request'
  | 'task_response'
  | 'heartbeat'
  | 'event'
  | 'broadcast';

export interface BusMessage {
  id: string;
  from: string;
  to: string;
  type: BusMessageType;
  content: string;
  timestamp: Date;
  metadata?: Record<string, string>;
}

// ===== TCP 传输（简化实现：使用 HTTP 作为传输层） =====

export interface Transport {
  /** 发送消息 */
  send(address: string, message: BusMessage): Promise<void>;
  /** 接收消息 */
  onMessage(handler: (message: BusMessage) => void): void;
  /** 关闭传输 */
  close(): void;
}

export class DistributedHTTPTransport implements Transport {
  private handlers: Array<(message: BusMessage) => void> = [];

  async send(address: string, message: BusMessage): Promise<void> {
    const resp = await fetch(`${address.replace(/\/$/, '')}/bus/message`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(message),
    });
    if (!resp.ok) {
      throw new Error(`send failed: ${resp.status} ${await resp.text()}`);
    }
  }

  onMessage(handler: (message: BusMessage) => void): void {
    this.handlers.push(handler);
  }

  /** 触发消息处理（由 HTTP 服务器调用） */
  async handleMessage(message: BusMessage): Promise<void> {
    for (const handler of this.handlers) {
      handler(message);
    }
  }

  close(): void {
    this.handlers = [];
  }
}

// ===== 分布式 Agent 协调器 =====

export interface DistributedConfig {
  /** Agent ID */
  agentId: string;
  /** 发现服务 */
  discovery: Discovery;
  /** 传输层 */
  transport: Transport;
  /** 心跳间隔（毫秒） */
  heartbeatIntervalMs?: number;
}

export class DistributedAgent {
  private config: DistributedConfig;
  private heartbeatTimer?: NodeJS.Timeout;
  private running: boolean = false;

  constructor(config: DistributedConfig) {
    this.config = {
      heartbeatIntervalMs: 10000,
      ...config,
    };
  }

  /** 启动分布式 Agent */
  async start(address: string, capabilities: string[] = []): Promise<void> {
    await this.config.discovery.register({
      id: this.config.agentId,
      name: this.config.agentId,
      address,
      capabilities,
      status: 'online',
    });

    // 启动心跳
    this.running = true;
    this.heartbeatTimer = setInterval(async () => {
      if (!this.running) return;
      try {
        await this.config.discovery.heartbeat(this.config.agentId);
      } catch {
        // 心跳失败，忽略
      }
    }, this.config.heartbeatIntervalMs);
  }

  /** 发送任务到其他 Agent */
  async sendTask(targetAgentId: string, content: string): Promise<void> {
    const target = await this.config.discovery.discover(targetAgentId);
    if (!target) {
      throw new Error(`Agent ${targetAgentId} not found`);
    }

    const message: BusMessage = {
      id: crypto.randomUUID(),
      from: this.config.agentId,
      to: targetAgentId,
      type: 'task_request',
      content,
      timestamp: new Date(),
    };

    await this.config.transport.send(target.address, message);
  }

  /** 广播消息到所有 Agent */
  async broadcast(content: string): Promise<void> {
    const agents = await this.config.discovery.listAgents();
    const message: BusMessage = {
      id: crypto.randomUUID(),
      from: this.config.agentId,
      to: '*',
      type: 'broadcast',
      content,
      timestamp: new Date(),
    };

    const results = await Promise.allSettled(
      agents
        .filter((a) => a.id !== this.config.agentId && a.status === 'online')
        .map((a) => this.config.transport.send(a.address, message)),
    );

    const failures = results.filter((r) => r.status === 'rejected');
    if (failures.length > 0) {
      console.warn(`Broadcast: ${failures.length} agents unreachable`);
    }
  }

  /** 监听消息 */
  onMessage(handler: (message: BusMessage) => void): void {
    this.config.transport.onMessage(handler);
  }

  /** 停止分布式 Agent */
  async stop(): Promise<void> {
    this.running = false;
    if (this.heartbeatTimer) {
      clearInterval(this.heartbeatTimer);
    }
    try {
      await this.config.discovery.unregister(this.config.agentId);
    } catch {
      // 忽略注销错误
    }
    this.config.transport.close();
  }
}
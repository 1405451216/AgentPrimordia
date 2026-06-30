/**
 * 可视化监控面板 — 实时 Agent 状态/Token/工具热力图。
 *
 * 提供两个核心组件：
 * 1. AgentMonitor: 收集 Agent 运行时的实时指标
 * 2. DashboardServer: HTTP 服务器提供实时 SSE 推送 + 静态 HTML 面板
 *
 * 监控维度：
 * - Agent 状态：running / completed / error
 * - Token 使用：prompt / completion / total
 * - 工具调用热力图：哪个工具被调用最频繁、延迟最高
 * - 轮次分布：每个 Agent 的 ReAct 轮次分布
 * - 延迟分解：LLM 延迟 vs 工具延迟
 *
 * 使用方式：
 *   const monitor = new AgentMonitor();
 *   monitor.attach(agent); // 自动收集指标
 *   const server = new DashboardServer(monitor);
 *   server.listen(3000); // 打开 http://localhost:3000
 */

import type { ReActAgent, StreamEvent } from './react-loop.js';
import type { RunOptions } from './react-loop.js';
import type { Response } from '../types.js';
import * as http from 'http';

// ===== 类型定义 =====

/** Agent 实时状态 */
export interface AgentState {
  name: string;
  status: 'idle' | 'running' | 'completed' | 'error';
  currentTurn: number;
  maxTurns: number;
  startTime?: number;
  endTime?: number;
  lastError?: string;
}

/** Token 使用统计 */
export interface TokenUsage {
  agentName: string;
  promptTokens: number;
  completionTokens: number;
  totalTokens: number;
  estimatedCost: number;
}

/** 工具调用统计 */
export interface ToolCallStat {
  toolName: string;
  callCount: number;
  errorCount: number;
  totalLatencyMs: number;
  avgLatencyMs: number;
  maxLatencyMs: number;
  lastCalledAt: string;
}

/** 单次运行记录 */
export interface RunRecord {
  agentName: string;
  sessionId: string;
  input: string;
  response: Response;
  duration: number;
  timestamp: string;
}

/** 面板快照 */
export interface DashboardSnapshot {
  timestamp: string;
  agents: AgentState[];
  tokenUsage: TokenUsage[];
  toolStats: ToolCallStat[];
  recentRuns: RunRecord[];
  totalRuns: number;
  totalErrors: number;
  avgDuration: number;
  avgTurns: number;
}

// ===== Agent 监控器 =====

/**
 * Agent 运行监控器。
 *
 * 通过 Hook 机制注入 Agent，自动收集运行时指标。
 * 不侵入 Agent 主循环，对性能影响极小。
 */
export class AgentMonitor {
  private agents: Map<string, AgentState> = new Map();
  private tokenUsage: Map<string, TokenUsage> = new Map();
  private toolStats: Map<string, ToolCallStat> = new Map();
  private runRecords: RunRecord[] = [];
  private listeners: Set<(snapshot: DashboardSnapshot) => void> = new Set();
  private maxRecords: number;

  constructor(maxRecords: number = 100) {
    this.maxRecords = maxRecords;
  }

  /** 已附加的 Agent 集合 — 防止重复 attach 导致嵌套包装 */
  private attachedAgents: WeakSet<ReActAgent> = new WeakSet();

  /** 附加到 Agent，自动注册 hooks */
  attach(agent: ReActAgent): void {
    // 防止重复 attach 导致嵌套 monkey-patch
    if (this.attachedAgents.has(agent)) return;
    this.attachedAgents.add(agent);

    const name = agent.name;
    this.agents.set(name, {
      name,
      status: 'idle',
      currentTurn: 0,
      maxTurns: 10,
    });

    // 通过包装 run 方法收集指标
    const originalRun = agent.run.bind(agent);
    agent.run = async (input: string, options?: unknown): Promise<Response> => {
      const state = this.agents.get(name)!;
      state.status = 'running';
      state.startTime = Date.now();
      state.currentTurn = 0;
      this.notify();

      try {
        const response = await originalRun(input, options as never);
        state.status = 'completed';
        state.endTime = Date.now();
        state.currentTurn = response.metrics.totalTurns;

        // 记录运行
        this.runRecords.push({
          agentName: name,
          sessionId: '',
          input: input.slice(0, 200),
          response,
          duration: response.metrics.duration,
          timestamp: new Date().toISOString(),
        });
        if (this.runRecords.length > this.maxRecords) {
          this.runRecords.shift();
        }

        this.notify();
        return response;
      } catch (err) {
        state.status = 'error';
        state.endTime = Date.now();
        state.lastError = err instanceof Error ? err.message : String(err);
        this.notify();
        throw err;
      }
    };

    // 同样包装 streamEvents 方法，确保流式模式也能被监控
    const originalStream = agent.streamEvents.bind(agent);
    const monitor = this;
    agent.streamEvents = async function* (input: string, options?: RunOptions): AsyncIterable<StreamEvent> {
      const state = monitor.agents.get(name);
      if (state) {
        state.status = 'running';
        state.startTime = Date.now();
        state.currentTurn = 0;
        monitor.notify();
      }

      try {
        let lastResponse: Response | null = null;
        for await (const event of originalStream(input, options)) {
          if (event.type === 'done' && event.response) {
            lastResponse = event.response;
          }
          yield event;
        }

        const st = monitor.agents.get(name);
        if (st && lastResponse) {
          st.status = 'completed';
          st.endTime = Date.now();
          st.currentTurn = lastResponse.metrics.totalTurns;

          monitor.runRecords.push({
            agentName: name,
            sessionId: '',
            input: input.slice(0, 200),
            response: lastResponse,
            duration: lastResponse.metrics.duration,
            timestamp: new Date().toISOString(),
          });
          if (monitor.runRecords.length > monitor.maxRecords) {
            monitor.runRecords.shift();
          }
          monitor.notify();
        }
      } catch (err) {
        const st = monitor.agents.get(name);
        if (st) {
          st.status = 'error';
          st.endTime = Date.now();
          st.lastError = err instanceof Error ? err.message : String(err);
          monitor.notify();
        }
        throw err;
      }
    };
  }

  /** 手动记录工具调用 */
  recordToolCall(
    agentName: string,
    toolName: string,
    latencyMs: number,
    isError: boolean,
  ): void {
    let stat = this.toolStats.get(toolName);
    if (!stat) {
      stat = {
        toolName,
        callCount: 0,
        errorCount: 0,
        totalLatencyMs: 0,
        avgLatencyMs: 0,
        maxLatencyMs: 0,
        lastCalledAt: new Date().toISOString(),
      };
      this.toolStats.set(toolName, stat);
    }

    stat.callCount++;
    if (isError) stat.errorCount++;
    stat.totalLatencyMs += latencyMs;
    stat.avgLatencyMs = stat.totalLatencyMs / stat.callCount;
    stat.maxLatencyMs = Math.max(stat.maxLatencyMs, latencyMs);
    stat.lastCalledAt = new Date().toISOString();
  }

  /** 手动记录 Token 使用 */
  recordTokenUsage(
    agentName: string,
    promptTokens: number,
    completionTokens: number,
    costPer1K: number = 0.002,
  ): void {
    let usage = this.tokenUsage.get(agentName);
    if (!usage) {
      usage = {
        agentName,
        promptTokens: 0,
        completionTokens: 0,
        totalTokens: 0,
        estimatedCost: 0,
      };
      this.tokenUsage.set(agentName, usage);
    }

    usage.promptTokens += promptTokens;
    usage.completionTokens += completionTokens;
    usage.totalTokens = usage.promptTokens + usage.completionTokens;
    usage.estimatedCost += (usage.totalTokens / 1000) * costPer1K;
  }

  /** 获取当前快照 */
  getSnapshot(): DashboardSnapshot {
    const agents = Array.from(this.agents.values());
    const tokenUsage = Array.from(this.tokenUsage.values());
    const toolStats = Array.from(this.toolStats.values());
    const recentRuns = this.runRecords.slice(-20);

    const totalRuns = this.runRecords.length;
    const totalErrors = this.runRecords.filter(
      (r) => r.response.content.startsWith('Agent error'),
    ).length;
    const avgDuration =
      totalRuns > 0
        ? this.runRecords.reduce((s, r) => s + r.duration, 0) / totalRuns
        : 0;
    const avgTurns =
      totalRuns > 0
        ? this.runRecords.reduce((s, r) => s + r.response.metrics.totalTurns, 0) / totalRuns
        : 0;

    return {
      timestamp: new Date().toISOString(),
      agents,
      tokenUsage,
      toolStats,
      recentRuns,
      totalRuns,
      totalErrors,
      avgDuration,
      avgTurns,
    };
  }

  /** 注册变更监听器 */
  onChange(listener: (snapshot: DashboardSnapshot) => void): () => void {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }

  /** 通知所有监听器 */
  private notify(): void {
    const snapshot = this.getSnapshot();
    for (const listener of this.listeners) {
      try {
        listener(snapshot);
      } catch {
        // 监听器异常不影响主流程
      }
    }
  }
}

// ===== Dashboard HTTP 服务器 =====

/**
 * 监控面板 HTTP 服务器。
 *
 * 提供：
 * - GET /: 静态 HTML 面板
 * - GET /api/snapshot: JSON 快照
 * - GET /api/stream: SSE 实时推送
 */
export class DashboardServer {
  private monitor: AgentMonitor;
  private server?: http.Server;
  private sseClients: Set<http.ServerResponse> = new Set();

  constructor(monitor: AgentMonitor) {
    this.monitor = monitor;
  }

  listen(port: number = 3000): void {
    this.server = http.createServer((req, res) => {
      const url = req.url ?? '/';

      if (url === '/' || url === '/index.html') {
        res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
        res.end(this.getHTML());
        return;
      }

      if (url === '/api/snapshot') {
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify(this.monitor.getSnapshot(), null, 2));
        return;
      }

      if (url === '/api/stream') {
        res.writeHead(200, {
          'Content-Type': 'text/event-stream',
          'Cache-Control': 'no-cache',
          Connection: 'keep-alive',
        });
        this.sseClients.add(res);

        // 立即推送一次快照
        res.write(`data: ${JSON.stringify(this.monitor.getSnapshot())}\n\n`);

        // 注册变更监听
        const unsubscribe = this.monitor.onChange((snapshot) => {
          res.write(`data: ${JSON.stringify(snapshot)}\n\n`);
        });

        req.on('close', () => {
          this.sseClients.delete(res);
          unsubscribe();
        });
        return;
      }

      res.writeHead(404);
      res.end('Not Found');
    });

    this.server.listen(port, () => {
      // 服务器启动成功
    });
  }

  close(): void {
    if (this.server) {
      this.server.close();
      this.server = undefined;
    }
    this.sseClients.clear();
  }

  /** 生成 HTML 面板 */
  private getHTML(): string {
    return `<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>AgentPrimordia 监控面板</title>
  <style>
    * { margin: 0; padding: 0; box-sizing: border-box; }
    body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; background: #0f172a; color: #e2e8f0; }
    .header { background: #1e293b; padding: 1rem 2rem; border-bottom: 1px solid #334155; }
    .header h1 { font-size: 1.5rem; color: #38bdf8; }
    .container { padding: 2rem; display: grid; grid-template-columns: repeat(auto-fit, minmax(400px, 1fr)); gap: 1.5rem; }
    .card { background: #1e293b; border-radius: 12px; padding: 1.5rem; border: 1px solid #334155; }
    .card h2 { font-size: 1.1rem; margin-bottom: 1rem; color: #38bdf8; }
    .stat-grid { display: grid; grid-template-columns: repeat(2, 1fr); gap: 1rem; }
    .stat { background: #0f172a; padding: 1rem; border-radius: 8px; }
    .stat .label { font-size: 0.8rem; color: #94a3b8; }
    .stat .value { font-size: 1.5rem; font-weight: bold; color: #f1f5f9; }
    .agent-row { display: flex; justify-content: space-between; padding: 0.5rem 0; border-bottom: 1px solid #334155; }
    .status-badge { padding: 2px 8px; border-radius: 4px; font-size: 0.75rem; }
    .status-running { background: #f59e0b; color: #000; }
    .status-completed { background: #22c55e; color: #000; }
    .status-error { background: #ef4444; color: #fff; }
    .status-idle { background: #64748b; color: #fff; }
    .tool-bar { display: flex; align-items: center; gap: 0.5rem; margin: 0.25rem 0; }
    .tool-bar .name { width: 120px; font-size: 0.85rem; }
    .tool-bar .bar { flex: 1; height: 8px; background: #334155; border-radius: 4px; overflow: hidden; }
    .tool-bar .fill { height: 100%; background: #38bdf8; transition: width 0.3s; }
    .tool-bar .count { width: 40px; text-align: right; font-size: 0.85rem; color: #94a3b8; }
    #updated { text-align: right; font-size: 0.75rem; color: #64748b; margin-top: 1rem; }
  </style>
</head>
<body>
  <div class="header">
    <h1>AgentPrimordia 监控面板</h1>
  </div>
  <div class="container">
    <div class="card">
      <h2>总览</h2>
      <div class="stat-grid">
        <div class="stat"><div class="label">总运行次数</div><div class="value" id="totalRuns">-</div></div>
        <div class="stat"><div class="label">错误次数</div><div class="value" id="totalErrors">-</div></div>
        <div class="stat"><div class="label">平均耗时</div><div class="value" id="avgDuration">-</div></div>
        <div class="stat"><div class="label">平均轮次</div><div class="value" id="avgTurns">-</div></div>
      </div>
    </div>
    <div class="card">
      <h2>Agent 状态</h2>
      <div id="agents"></div>
    </div>
    <div class="card">
      <h2>Token 使用</h2>
      <div id="tokens"></div>
    </div>
    <div class="card">
      <h2>工具调用热力图</h2>
      <div id="tools"></div>
    </div>
  </div>
  <div id="updated">等待数据...</div>
  <script>
    const es = new EventSource('/api/stream');
    es.onmessage = (e) => {
      const data = JSON.parse(e.data);
      document.getElementById('totalRuns').textContent = data.totalRuns;
      document.getElementById('totalErrors').textContent = data.totalErrors;
      document.getElementById('avgDuration').textContent = Math.round(data.avgDuration) + 'ms';
      document.getElementById('avgTurns').textContent = data.avgTurns.toFixed(1);

      // Agent 状态
      const agentsHtml = data.agents.map(a => 
        '<div class="agent-row"><span>' + a.name + '</span><span class="status-badge status-' + a.status + '">' + a.status + '</span></div>'
      ).join('');
      document.getElementById('agents').innerHTML = agentsHtml;

      // Token 使用
      const tokensHtml = data.tokenUsage.map(t =>
        '<div class="agent-row"><span>' + t.agentName + '</span><span>' + t.totalTokens + ' ($' + t.estimatedCost.toFixed(4) + ')</span></div>'
      ).join('');
      document.getElementById('tokens').innerHTML = tokensHtml;

      // 工具热力图
      const maxCount = Math.max(...data.toolStats.map(t => t.callCount), 1);
      const toolsHtml = data.toolStats.map(t =>
        '<div class="tool-bar"><span class="name">' + t.toolName + '</span><div class="bar"><div class="fill" style="width:' + (t.callCount/maxCount*100) + '%"></div></div><span class="count">' + t.callCount + '</span></div>'
      ).join('');
      document.getElementById('tools').innerHTML = toolsHtml;

      document.getElementById('updated').textContent = '最后更新: ' + new Date().toLocaleTimeString();
    };
  </script>
</body>
</html>`;
  }
}

# Playground 快速开始指南

AgentPrimordia Playground 是一个交互式多 Agent 对话环境，支持通过 Web UI 或 CLI 创建、管理和调试 Agent。

---

## 快速开始

### 前置条件

- Node.js >= 18
- Docker & Docker Compose（用于部署 Playground 服务端）

### 1. 启动 Playground 服务

```bash
cd sdk/typescript/playground
docker compose up -d
```

服务默认监听 `http://localhost:8080`。

### 2. 使用 TypeScript 客户端

```typescript
import { PlaygroundClient } from '@agentprimordia/sdk';

const pg = new PlaygroundClient({
  apiBase: 'http://localhost:8080',
  defaultModel: 'gpt-4',
});

// 创建 Agent
const agent = await pg.createAgent({
  name: 'my-assistant',
  systemPrompt: '你是一个有帮助的助手。',
  maxTurns: 10,
});

// 同步对话
const reply = await pg.chat(agent.id, '你好，请介绍一下自己');
console.log(reply.response);

// 流式对话
for await (const event of pg.streamChat(agent.id, '写一首诗')) {
  if (event.type === 'token') process.stdout.write(event.content);
  if (event.type === 'done') console.log('\n--- 完成 ---');
}

// 查看统计
const stats = await pg.getStats(agent.id);
console.log(`Turns: ${stats.turnCount}, Tokens: ${stats.totalTokens}`);

// 清理
await pg.deleteAgent(agent.id);
```

### 3. 使用 CLI 会话管理器

```typescript
import { PlaygroundSession, PlaygroundManager } from '@agentprimordia/sdk';

const manager = new PlaygroundManager('http://localhost:8080', 'gpt-4');
const session = await manager.createSession('code-helper', {
  systemPrompt: '你是代码助手',
  tools: ['filesystem', 'shell'],
});

// 流式对话（带回调）
await session.chat('帮我写一个排序算法', {
  onToken: (token) => process.stdout.write(token),
  onToolCall: (tool, args) => console.log(`\n[Tool] ${tool}`, args),
});

// 导出历史
const history = session.exportJSON();
```

---

## API 参考

### PlaygroundClient

| 方法 | 说明 | 返回值 |
|------|------|--------|
| `createAgent(config)` | 创建 Agent | `Promise<AgentInfo>` |
| `deleteAgent(id)` | 删除 Agent | `Promise<void>` |
| `listAgents()` | 列出所有 Agent | `Promise<AgentInfo[]>` |
| `getAgent(id)` | 获取 Agent 详情 | `Promise<AgentInfo>` |
| `chat(id, message)` | 同步对话 | `Promise<ChatResponse>` |
| `streamChat(id, message)` | 流式对话（SSE） | `AsyncGenerator<StreamEvent>` |
| `streamEvents(id)` | 订阅 Agent 事件 | `AsyncGenerator<StreamEvent>` |
| `getStats(id)` | 获取运行统计 | `Promise<AgentStats>` |

### StreamEvent 类型

```typescript
type StreamEvent =
  | { type: 'token'; content: string }      // 逐 token 输出
  | { type: 'tool_call'; tool: string; args: unknown }  // 工具调用
  | { type: 'error'; message: string }      // 错误
  | { type: 'done' };                       // 完成
```

### PlaygroundSession

| 方法/属性 | 说明 |
|-----------|------|
| `chat(message, opts?)` | 发送消息（支持 onToken/onToolCall 回调） |
| `abort()` | 中断当前请求 |
| `exportJSON()` | 导出对话历史为 JSON |
| `refreshStats()` | 刷新远程统计 |
| `messages` | 本地消息历史 |
| `agent` | 关联的 Agent 信息 |

---

## HTTP API 端点

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/playground/agents` | 创建 Agent |
| GET | `/api/playground/agents` | 列出 Agent |
| GET | `/api/playground/agents/:id` | Agent 详情 |
| DELETE | `/api/playground/agents/:id` | 删除 Agent |
| POST | `/api/playground/agents/:id/chat` | 同步对话 |
| POST | `/api/playground/agents/:id/stream` | 流式对话（SSE） |
| GET | `/api/playground/agents/:id/events` | 事件订阅（SSE） |
| GET | `/api/playground/agents/:id/stats` | 运行统计 |

---

## 部署指南

### Docker Compose（推荐）

```bash
cd sdk/typescript/playground
docker compose up -d
```

环境变量：

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `AP_PLAYGROUND_PORT` | `8080` | 服务监听端口 |
| `AP_LLM_PROVIDER` | `openai` | LLM 提供商 |
| `AP_LLM_MODEL` | `gpt-4` | 默认模型 |
| `AP_LLM_API_KEY` | — | LLM API Key（必填） |
| `AP_MAX_AGENTS` | `50` | 最大并发 Agent 数 |
| `AP_LOG_LEVEL` | `info` | 日志级别 |

### 手动构建

```bash
# 构建镜像
docker build -t ap-playground ./playground

# 运行
docker run -d -p 8080:8080 -e AP_LLM_API_KEY=sk-xxx ap-playground
```

### 生产部署建议

1. 使用反向代理（Nginx/Caddy）终结 TLS
2. 设置 `AP_MAX_AGENTS` 限制并发，防止资源耗尽
3. 配置健康检查端点 `GET /healthz`
4. 使用 Docker 资源限制（`--memory`, `--cpus`）
5. 日志输出到集中式日志系统（ELK/Loki）

---

## 故障排查

| 问题 | 解决方案 |
|------|---------|
| 连接超时 | 检查 `apiBase` 是否正确，服务是否启动 |
| HTTP 401 | 检查 `AP_LLM_API_KEY` 是否配置 |
| SSE 断开 | 客户端内置 3 次自动重连，检查网络稳定性 |
| Agent 创建失败 | 检查是否超过 `AP_MAX_AGENTS` 限制 |

# CodeCast 子 Agent 系统设计规格

> 日期: 2026-05-22
> 状态: 已确认
> 方案: B（完整 Agent Loop）

## 1. 概述

CodeCast 子 Agent 系统允许主 Agent 将复杂任务拆分为多个独立子任务，每个子任务由一个独立的 Agent Loop（LLM 调用 → 工具执行 → 观察结果 → 循环）在后台并发执行。这能显著提升多文件编辑、并行搜索、批量处理等场景的效率。

系统支持两种触发模式：显式模式（用户在 UI 中可见并行任务卡片）和隐式模式（主 Agent 内部加速，对用户透明）。

## 2. 核心架构

```
┌─────────────────────────────────────────────────┐
│                  Frontend (React)                 │
│  ┌──────────────┐  ┌─────────────────────────┐  │
│  │ 聊天区内联卡片  │  │  侧边栏 Agents 面板      │  │
│  └──────────────┘  └─────────────────────────┘  │
│         ▲ Wails Events (agent:progress)          │
├─────────────────────────────────────────────────┤
│                  Backend (Go)                     │
│  ┌───────────────────────────────────────────┐  │
│  │           AgentPool (Semaphore=10)          │  │
│  │  ┌─────────┐ ┌─────────┐ ┌─────────┐     │  │
│  │  │SubAgent1│ │SubAgent2│ │SubAgent3│ ... │  │
│  │  └────┬────┘ └────┬────┘ └────┬────┘     │  │
│  │       │            │            │           │  │
│  │  ┌────▼────────────▼────────────▼────────┐ │  │
│  │  │        Tool Executor Layer             │ │  │
│  │  │  read_file | write_file | edit_file    │ │  │
│  │  │  run_command | search | web_fetch      │ │  │
│  │  └───────────────────────────────────────┘ │  │
│  └───────────────────────────────────────────┘  │
│  ┌───────────────────────────────────────────┐  │
│  │         Persistence Layer                   │  │
│  │  ~/.codecast/agents/{session}/{agent}.json  │  │
│  └───────────────────────────────────────────┘  │
├─────────────────────────────────────────────────┤
│              DeepSeek API (同模型)                │
└─────────────────────────────────────────────────┘
```

## 3. 数据结构

### 3.1 Go 后端

```go
// SubAgent 代表一个独立执行的子代理
type SubAgent struct {
    ID          string            `json:"id"`
    SessionID   string            `json:"session_id"`
    ParentMsgID string            `json:"parent_msg_id"`
    Title       string            `json:"title"`
    Prompt      string            `json:"prompt"`
    FilesScope  []string          `json:"files_scope"`
    Status      AgentStatus       `json:"status"`
    Messages    []AgentMessage    `json:"messages"`
    Result      string            `json:"result"`
    Error       string            `json:"error,omitempty"`
    TurnCount   int               `json:"turn_count"`
    MaxTurns    int               `json:"max_turns"`
    CreatedAt   time.Time         `json:"created_at"`
    UpdatedAt   time.Time         `json:"updated_at"`
    Mode        AgentMode         `json:"mode"`
}

type AgentStatus string
const (
    AgentStatusQueued    AgentStatus = "queued"
    AgentStatusRunning   AgentStatus = "running"
    AgentStatusCompleted AgentStatus = "completed"
    AgentStatusFailed    AgentStatus = "failed"
    AgentStatusCancelled AgentStatus = "cancelled"
)

type AgentMode string
const (
    AgentModeExplicit AgentMode = "explicit"  // 用户可见
    AgentModeImplicit AgentMode = "implicit"  // 对用户透明
)

// AgentMessage 是子 Agent 的对话消息
type AgentMessage struct {
    Role       string      `json:"role"`
    Content    string      `json:"content"`
    ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
    ToolResult *ToolResult `json:"tool_result,omitempty"`
}

type ToolCall struct {
    ID       string `json:"id"`
    Name     string `json:"name"`
    Args     string `json:"args"`  // JSON string
}

type ToolResult struct {
    ToolCallID string `json:"tool_call_id"`
    Content    string `json:"content"`
    IsError    bool   `json:"is_error"`
}

// AgentPool 管理并发执行
type AgentPool struct {
    mu        sync.Mutex
    agents    map[string]*SubAgent
    semaphore chan struct{}        // capacity = 10
    queue     []*SubAgent
    ctx       context.Context
    cancel    context.CancelFunc
    app       *App
}

// AgentEvent 发送给前端的进度事件
type AgentEvent struct {
    AgentID   string      `json:"agent_id"`
    Type      string      `json:"type"`  // "status" | "progress" | "tool_use" | "result"
    Status    AgentStatus `json:"status,omitempty"`
    Turn      int         `json:"turn,omitempty"`
    MaxTurns  int         `json:"max_turns,omitempty"`
    ToolName  string      `json:"tool_name,omitempty"`
    Message   string      `json:"message,omitempty"`
}
```

### 3.2 前端 TypeScript

```typescript
interface SubAgent {
  id: string;
  sessionId: string;
  parentMsgId: string;
  title: string;
  status: 'queued' | 'running' | 'completed' | 'failed' | 'cancelled';
  mode: 'explicit' | 'implicit';
  turn: number;
  maxTurns: number;
  result?: string;
  error?: string;
  lastToolName?: string;
  createdAt: string;
  updatedAt: string;
}

interface AgentEvent {
  agent_id: string;
  type: 'status' | 'progress' | 'tool_use' | 'result';
  status?: SubAgent['status'];
  turn?: number;
  max_turns?: number;
  tool_name?: string;
  message?: string;
}
```

## 4. 主 Agent 调度接口

主 Agent 通过 `dispatch_agents` 工具触发子任务。该工具作为 system prompt 中的工具描述注入：

```json
{
  "name": "dispatch_agents",
  "description": "将任务拆分为多个子任务并行执行。每个子任务由独立 Agent 完成。适用于：多文件同时编辑、并行搜索多个代码库、批量文件处理等场景。",
  "parameters": {
    "type": "object",
    "properties": {
      "tasks": {
        "type": "array",
        "items": {
          "type": "object",
          "properties": {
            "title": { "type": "string", "description": "子任务标题" },
            "prompt": { "type": "string", "description": "子任务详细指令" },
            "files_scope": {
              "type": "array",
              "items": { "type": "string" },
              "description": "该子任务可写入的文件路径列表（避免冲突）"
            }
          },
          "required": ["title", "prompt"]
        }
      },
      "mode": {
        "type": "string",
        "enum": ["explicit", "implicit"],
        "description": "explicit=用户可见并行卡片, implicit=后台静默执行"
      }
    },
    "required": ["tasks"]
  }
}
```

后端实现为 Wails binding 方法 `App.DispatchAgents(tasksJSON string) ([]string, error)`，返回各 agent ID 列表。

## 5. Agent 执行引擎

每个 SubAgent 在独立 goroutine 中运行完整的 Agent Loop：

```
初始化 → [LLM调用(含工具描述)] → 解析响应
    ├─ 响应含 tool_calls → 执行工具 → 观察结果 → 追加消息 → 循环
    ├─ 响应为纯文本（无 tool_calls） → 任务完成
    └─ 超过 maxTurns 或出错 → 标记失败
```

### 5.1 引擎关键行为

- 每次 LLM 调用使用与主 Agent 相同的模型和 API Key
- System prompt 精简为任务导向（不含交互风格、个性化等内容）
- 每次工具执行后自动保存状态到磁盘（持久化）
- 通过 Wails EventsEmit 向前端推送进度事件
- 支持取消（context 传播）

### 5.2 子 Agent System Prompt 模板

```
你是一个专注的代码执行助手。你的任务是：

{task_prompt}

你可以使用以下工具完成任务：
- read_file: 读取文件内容
- write_file: 创建或覆盖文件
- edit_file: 搜索替换编辑文件
- run_command: 执行 shell 命令
- search: 搜索文件内容或文件名
- web_fetch: 获取网页内容

规则：
1. 直接开始工作，不要询问用户
2. 每步只做一件事，确认结果后再继续
3. 完成后用一句话总结你做了什么
4. 如果遇到无法解决的问题，说明原因后停止

{files_scope_constraint}
```

### 5.3 Tool Calling 协议

使用 DeepSeek API 的 function calling 格式：

```json
{
  "model": "deepseek-v4-flash",
  "messages": [...],
  "tools": [
    {
      "type": "function",
      "function": {
        "name": "read_file",
        "description": "读取指定路径的文件内容",
        "parameters": {
          "type": "object",
          "properties": {
            "path": { "type": "string", "description": "文件绝对路径" }
          },
          "required": ["path"]
        }
      }
    }
  ]
}
```

响应中 `choices[0].message.tool_calls` 非空时，执行工具并将结果作为 `role: "tool"` 消息追加，然后继续调用 LLM。

## 6. 工具层实现

### 6.1 工具清单

| 工具 | 描述 | 参数 | 冲突控制 |
|------|------|------|----------|
| `read_file` | 读取文件内容 | `path` | 无限制 |
| `write_file` | 写入完整文件 | `path`, `content` | 受 FilesScope 约束 |
| `edit_file` | 搜索替换编辑 | `path`, `old_string`, `new_string` | 受 FilesScope 约束 |
| `run_command` | 执行 shell 命令 | `command`, `workdir`(可选) | 工作目录隔离 |
| `search` | grep/glob 搜索 | `pattern`, `path`(可选), `type`(可选) | 无限制 |
| `web_fetch` | 获取网页内容 | `url` | 无限制 |

### 6.2 FilesScope 约束

```go
func (agent *SubAgent) canWriteFile(path string) bool {
    if len(agent.FilesScope) == 0 {
        return true  // 无限制
    }
    absPath := filepath.Clean(path)
    for _, scope := range agent.FilesScope {
        scopeAbs := filepath.Clean(scope)
        // 精确匹配或目录前缀匹配
        if absPath == scopeAbs || strings.HasPrefix(absPath, scopeAbs+string(filepath.Separator)) {
            return true
        }
    }
    return false
}
```

## 7. 并发池（AgentPool）

### 7.1 核心实现

```go
func NewAgentPool(app *App, maxConcurrency int) *AgentPool {
    ctx, cancel := context.WithCancel(context.Background())
    return &AgentPool{
        agents:    make(map[string]*SubAgent),
        semaphore: make(chan struct{}, maxConcurrency),
        queue:     make([]*SubAgent, 0),
        ctx:       ctx,
        cancel:    cancel,
        app:       app,
    }
}

func (pool *AgentPool) Submit(agent *SubAgent) {
    pool.mu.Lock()
    pool.agents[agent.ID] = agent
    agent.Status = AgentStatusQueued
    pool.mu.Unlock()

    pool.emitEvent(agent, "status")

    go func() {
        select {
        case pool.semaphore <- struct{}{}:
            // 获得信号量，开始执行
            defer func() { <-pool.semaphore }()
            pool.runAgentLoop(agent)
        case <-pool.ctx.Done():
            // pool 被关闭
            agent.Status = AgentStatusCancelled
            pool.emitEvent(agent, "status")
        }
    }()
}

func (pool *AgentPool) Cancel(agentID string) {
    pool.mu.Lock()
    agent, exists := pool.agents[agentID]
    pool.mu.Unlock()
    if exists && agent.Status == AgentStatusRunning {
        agent.Status = AgentStatusCancelled
        // 通过 agent 内部的 context cancel 传播
    }
}

func (pool *AgentPool) Shutdown() {
    pool.cancel()
    // 等待所有 agent 完成（最多 10 秒）
}
```

### 7.2 事件推送

```go
func (pool *AgentPool) emitEvent(agent *SubAgent, eventType string) {
    event := AgentEvent{
        AgentID:  agent.ID,
        Type:     eventType,
        Status:   agent.Status,
        Turn:     agent.TurnCount,
        MaxTurns: agent.MaxTurns,
    }
    wailsRuntime.EventsEmit(pool.app.ctx, "agent:event", event)
}
```

## 8. 持久化

### 8.1 存储路径

```
~/.codecast/agents/
  └── {session_id}/
      ├── {agent_id_1}.json
      ├── {agent_id_2}.json
      └── ...
```

### 8.2 保存时机

每个 turn 完成后（工具执行完、观察结果追加到 Messages 后）自动保存。保存为完整的 SubAgent 结构体 JSON。

### 8.3 恢复策略

应用启动时不自动恢复未完成任务（避免意外副作用）。用户可在 Agents 面板中查看历史记录，手动重试失败的任务。

## 9. 前端 UI

### 9.1 Zustand Store (`useAgentStore.ts`)

```typescript
interface AgentStore {
  agents: Map<string, SubAgent>;
  addAgent: (agent: SubAgent) => void;
  updateAgent: (id: string, updates: Partial<SubAgent>) => void;
  removeAgent: (id: string) => void;
  getAgentsBySession: (sessionId: string) => SubAgent[];
  cancelAgent: (id: string) => Promise<void>;
}
```

### 9.2 聊天区内联卡片 (`AgentCard.tsx`)

在聊天消息流中，当检测到 `dispatch_agents` 工具调用时，渲染一个任务组卡片。卡片显示各子任务的实时状态，支持展开查看详情和取消操作。

卡片状态映射：
- `queued` → 灰色等待图标
- `running` → 蓝色旋转动画 + 当前 turn/maxTurns + 最近工具名
- `completed` → 绿色勾号 + 结果摘要
- `failed` → 红色叉号 + 错误信息
- `cancelled` → 灰色删除线

隐式模式行为：当 `mode === 'implicit'` 时，聊天区不渲染内联卡片。子任务仅在侧边栏 Agents 面板中可见（折叠在"后台任务"分组下）。主 Agent 收到结果后直接整合到回复中，用户感知不到子 Agent 的存在。

### 9.3 侧边栏 Agents 面板 (`AgentsPanel.tsx`)

新增侧边栏面板入口（图标：机器人/线程），展示：
- 当前 session 活跃的子任务（实时进度）
- 历史子任务列表（按时间倒序）
- 筛选：按状态、按 session
- 展开详情：完整对话记录、工具调用历史

### 9.4 类型扩展

`ActivePanel` 类型新增 `'agents'` 选项。

## 10. 完整文件变更清单

### 后端新增

| 文件 | 职责 |
|------|------|
| `agent.go` | SubAgent、AgentPool、AgentEvent 类型定义 + Pool 实现 |
| `agent_engine.go` | Agent Loop 执行引擎（LLM 调用 + 响应解析 + 循环控制） |
| `agent_tools.go` | 6 个工具的具体实现（read_file, write_file, edit_file, run_command, search, web_fetch） |
| `agent_persist.go` | JSON 持久化（Save/Load/List） |

### 后端修改

| 文件 | 变更 |
|------|------|
| `main.go` | App 添加 `agentPool *AgentPool` 字段，startup 中初始化，shutdown 中优雅关闭 |
| `session.go` | 添加 `DispatchAgents`、`GetAgents`、`GetAgent`、`CancelAgent` Wails binding 方法 |
| `prompts.go` | coding 模式 system prompt 添加 dispatch_agents 工具描述段落 |

### 前端新增

| 文件 | 职责 |
|------|------|
| `store/useAgentStore.ts` | Agent 状态管理 + 事件监听初始化 |
| `components/AgentCard.tsx` | 内联任务卡片组件 |
| `components/AgentsPanel.tsx` | 侧边栏面板组件 |
| `styles/agents.css` | Agent 相关样式 |

### 前端修改

| 文件 | 变更 |
|------|------|
| `store/types.ts` | 添加 SubAgent、AgentEvent 接口；ActivePanel 添加 `'agents'` |
| `components/MessagesView.tsx` | 识别 dispatch_agents 消息并渲染 AgentCard |
| `components/Sidebar.tsx` | 添加 Agents 面板图标入口 |
| `styles/index.css` | 引入 agents.css |

## 11. 非功能需求

- 单个子 Agent 最大 50 turns，超出后标记为 failed 并返回已有结果
- 工具执行超时：shell 命令 5 分钟，文件操作 30 秒，web_fetch 60 秒
- 消息历史上限：每个 agent 不超过 100 条消息（含工具调用/结果），超出时截断早期非 system 消息
- 优雅关闭：App shutdown 时取消所有运行中的 agent，等待最多 10 秒
- 磁盘清理：超过 7 天的已完成 agent 持久化文件自动清理

## 12. 未来扩展（不在本次实现范围）

- 子 Agent 间通信（message passing）
- 自动文件冲突检测与合并
- Agent 执行结果的自动汇总摘要
- 用户自定义工具注入子 Agent
- 子 Agent 支持不同模型（如复杂任务用 pro、简单任务用 flash）

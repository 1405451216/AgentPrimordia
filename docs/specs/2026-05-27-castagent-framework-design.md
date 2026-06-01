# AgentPrimordia Framework - 设计规格书

> **名称**: AgentPrimordia（原智/元代理）
> **缩写**: `ap` / `prim`
> **Slogan**: "The Primordial Agent Framework for Go"
> 日期: 2026-05-27
> 状态: ✅ 已批准
> 版本: v0.1-draft
> 协议: Apache-2.0

## 1. 项目概述

### 1.1 目标

将 CodeCast 应用的核心技术栈提取为**独立的通用 Agent 开发框架**，定位为 **"Go 语言的 LangChain/LangGraph"**。

### 1.2 核心价值主张

```
AgentPrimordia = 轻量 + 并发原生 + 简单
```

| 特性 | 描述 |
|------|------|
| **🚀 轻量级** | 无重 Runtime，纯 Go 实现，编译成单二进制 |
| **🔀 并发原生** | Goroutine + Channel，非线程池模拟 |
| **🧩 极简 API** | 5 行代码跑通 Hello Agent |
| **🔌 多模型支持** | OpenAI/DeepSeek/Anthropic/Ollama 统一接口 |
| **🛡️ 生产就绪** | 容错、安全沙箱、可观测性内置 |

### 1.3 差异化竞争

| 对比维度 | CastAgent | LangChain (Python) | CrewAI (Python) | AutoGen (Python) |
|----------|-----------|---------------------|------------------|-------------------|
| **语言** | Go ✅ | Python | Python | Python |
| **并发模型** | Goroutine 原生 | asyncio | threading | async |
| **内存占用** | ~20MB | ~200MB+ | ~150MB | ~180MB |
| **部署形态** | 单二进制 | pip install | pip install | pip install |
| **学习曲线** | 30 分钟 | 6+ 小时 | 2 小时 | 4 小时 |
| **多 Agent** | AgentPool 原生 | LangGraph | Crew | GroupChat |
| **类型安全** | 编译期检查 | 运行时 | 运行时 | 运行时 |

---

## 2. 设计哲学（3 大原则）

### 2.1 并发原生 (Concurrency-Native)

```go
// 不是用线程池模拟并发，而是用 Go 原生 goroutine + channel
// CodeCast 的 AgentPool 设计已被验证（10 并发子 Agent）

agent := cast.NewAgent(config)
result, _ := agent.Run(ctx, task,
    cast.WithParallelism(10),       // 控制并发度
    cast.WithTimeout(5*time.Minute), // 超时控制
)
```

**核心优势：**
- 零额外开销（Go runtime 已优化）
- 天然支持百万级并发
- 优雅的取消和超时机制

### 2.2 透明可调试 (Transparent & Debuggable)

```go
// 不封装黑盒，每一步都可观测
agent := cast.NewAgent(cast.AgentConfig{
    Hooks: cast.Hooks{
        OnReasoning: func(ctx context.Context, thought *Thought) {
            log.Printf("思考: %s", thought.Content)
        },
        OnToolUse: func(ctx context.Context, call *ToolCall) {
            log.Printf("工具: %s(%s)", call.Name, call.Args)
        },
    },
})
```

**设计原则：**
- Prompt 可完全自定义
- 每个 API 调用可追踪
- 决策过程通过 Hooks 暴露

### 2.3 渐进式复杂度 (Progressive Complexity)

```
Level 1: 5 行代码 → Hello Agent
Level 2: + 工具 + 记忆 → 实用 Agent
Level 3: + 多 Agent 协作 → 复杂系统
Level 4: + 容错/监控/分布式 → 生产级
```

---

## 3. 系统架构

### 3.1 分层架构图

```
┌─────────────────────────────────────────────────────────────────────┐
│                        CastAgent Framework                          │
│                                                                     │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │                    Application Layer                         │   │
│  │         (用户代码: 定义 Agent / Tools / Workflow)            │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                              ↕                                       │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │                      Orchestration Layer                     │   │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐    │   │
│  │  │ ReActLoop│  │ AgentPool│  │ Pipeline │  │ Workflow │    │   │
│  │  │ (单Agent)│  │ (多Agent)│  │ (顺序)   │  │ (DAG)    │    │   │
│  │  └──────────┘  └──────────┘  └──────────┘  └──────────┘    │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                              ↕                                       │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │                       Capability Layer                       │   │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐    │   │
│  │  │ Tool Sys │  │ Memory   │  │ LLM Abst.│  │ Knowledge│    │   │
│  │  │ Registry │  │ Store    │  │ Provider │  │ Base/RAG │    │   │
│  │  └──────────┘  └──────────┘  └──────────┘  └──────────┘    │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                              ↕                                       │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │                     Infrastructure Layer                      │   │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐    │   │
│  │  │ Event Bus│  │Persistenc│  │ Security  │  │ Observab.│    │   │
│  │  │ Channel  │  │ Checkpt  │  │ Sandbox   │  │ Tracing  │    │   │
│  │  └──────────┘  └──────────┘  └──────────┘  └──────────┘    │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                                                                     │
│  ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ 多语言绑定 ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─     │
│  ┌─────────┐  ┌─────────┐  ┌─────────┐                             │
│  │ Go SDK  │  │ TS SDK  │  │ Py SDK  │                             │
│  │ (原生)  │  │(Wasm/FFI)│  │(cgo/PyO3)│                            │
│  └─────────┘  └─────────┘  └─────────┘                             │
└─────────────────────────────────────────────────────────────────────┘
```

### 3.2 核心模块清单

| 模块 | 包路径 | 职责 | 代码来源 |
|------|--------|------|----------|
| **Agent Core** | `cast/agent` | ReAct Loop、生命周期管理 | CodeCast `agent_engine.go` |
| **AgentPool** | `cast/pool` | 多 Agent 并发调度 | CodeCast `agent.go` (AgentPool) |
| **Tool System** | `cast/tools` | 工具注册、执行、权限 | CodeCast `agent_tools.go` |
| **Memory Store** | `cast/memory` | 记忆存储、检索、摘要 | CodeCast `memory.go` |
| **LLM Abstraction** | `cast/llm` | 统一 LLM 接口、Provider | CodeCast `context.go` |
| **Event Bus** | `cast/events` | 事件发布订阅 | Wails Events 抽象 |
| **Persistence** | `cast/persist` | Checkpoint、状态持久化 | CodeCast `agent_persist.go` |
| **Security** | `cast/security` | 沙箱、权限控制 | CodeCast `sandbox.go`, `command_filter.go` |

---

## 4. 核心 API 设计

### 4.1 Agent 接口

```go
package cast

// Agent 是所有 Agent 的基础接口
type Agent interface {
    // 执行主入口
    Run(ctx context.Context, input Message) (*Response, error)
    
    // 生命周期管理
    Start(ctx context.Context) error
    Stop() error
    Pause() error
    Resume() error
    
    // 状态查询
    Status() AgentStatus
    Stats() AgentStats
}

// Message 表示一次交互的消息
type Message struct {
    Role      Role       `json:"role"`       // user / assistant / system / tool
    Content   string     `json:"content"`
    ToolCalls []ToolCall `json:"tool_calls,omitempty"`
    Metadata  Metadata   `json:"metadata,omitempty"`
}

// Response 表示执行结果
type Response struct {
    Content    string     `json:"content"`
    ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
    Usage      Usage      `json:"usage"`
    Metrics    Metrics    `json:"metrics"`
    Error      error      `json:"error,omitempty"`
}
```

### 4.2 ReActAgent 配置

```go
package cast

// ReActConfig 配置 ReAct 类型的 Agent
type ReActConfig struct {
    // 基础配置
    Name        string       `json:"name"`
    SystemPrompt string      `json:"system_prompt"`
    
    // 能力配置
    Model       LLMProvider  `json:"-"`           // LLM 提供者
    Toolkit     *ToolRegistry `json:"-"`           // 工具集
    Memory      MemoryStore   `json:"-"`           // 记忆系统
    
    // 行为配置
    MaxTurns    int           `json:"max_turns"`    // 最大轮次 (默认 50)
    Temperature float64       `json:"temperature"`  // 温度参数
    
    // 高级特性
    Hooks       Hooks         `json:"-"`           // 钩子系统
    Security    *SecurityCfg  `json:"-"`           // 安全配置
    Persistence *PersistCfg   `json:"-"`           // 持久化配置
}

// NewReActAgent 创建 ReAct Agent（主要入口）
func NewReActAgent(cfg ReActConfig) *ReActAgent
```

### 4.3 AgentPool 接口

```go
package cast

// Pool 管理 Agent 的并发执行
type Pool interface {
    // Dispatch 分发多个子任务并行执行
    Dispatch(ctx context.Context, tasks []TaskConfig) ([]*TaskResult, error)
    
    // Submit 提交单个任务（异步）
    Submit(ctx context.Context, task TaskConfig) (*Future, error)
    
    // Cancel 取消指定任务
    Cancel(taskID string) error
    
    // CancelAll 取消所有任务
    CancelAll() error
    
    // Stats 获取池统计信息
    Stats() PoolStats
}

// TaskConfig 定义子任务
type TaskConfig struct {
    Title      string   `json:"title"`
    Prompt     string   `json:"prompt"`
    Tools      []Tool   `json:"tools,omitempty"`
    FilesScope []string `json:"files_scope,omitempty"`
    MaxTurns   int      `json:"max_turns,omitempty"`
}

// TaskResult 子任务结果
type TaskResult struct {
    TaskID   string    `json:"task_id"`
    Task     TaskConfig `json:"task"`
    Response *Response `json:"response"`
    Error    error     `json:"error,omitempty"`
    Duration time.Duration `json:"duration"`
}
```

### 4.4 Tool 接口

```go
package tools

// Tool 是工具的基础接口
type Tool interface {
    // Name 返回工具名称（用于 Function Calling）
    Name() string
    
    // Description 返回工具描述（提供给 LLM）
    Description() string
    
    // Parameters 返回 JSON Schema 格式的参数定义
    Parameters() jsonschema.Schema
    
    // Execute 执行工具调用
    Execute(ctx context.Context, args json.RawMessage) (*ToolResult, error)
}

// ToolResult 工具执行结果
type ToolResult struct {
    Content string `json:"content"`
    IsError bool   `json:"is_error"`
}

// ToolRegistry 工具注册表
type Registry struct {
    tools       map[string]Tool
    permissions PermissionManager
}

func (r *Registry) Register(tool Tool) error
func (r *Registry) Get(name string) (Tool, bool)
func (r *Registry) List() []Tool
func (r *Registry) Definitions() []FunctionDefinition  // 用于 LLM Function Calling
```

### 4.5 LLM Provider 接口

```go
package llm

// Provider 是 LLM 提供者的统一接口
type Provider interface {
    // Complete 同步完成
    Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)
    
    // Stream 流式完成
    Stream(ctx context.Context, req *CompletionRequest) (<-chan Chunk, error)
    
    // CallTools 带 Function Calling 的调用
    CallTools(ctx context.Context, req *ToolCallRequest) (*ToolCallResponse, error)
    
    // Embeddings 文本向量化（可选）
    Embeddings(ctx context.Context, texts []string) ([][]float32, error)
    
    // Info 模型信息
    Info() ModelInfo
}

// 内置 Provider 实现
func NewOpenAIProvider(cfg Config) (Provider, error)
func NewDeepSeekProvider(cfg Config) (Provider, error)
func NewAnthropicProvider(cfg Config) (Provider, error)
func NewOllamaProvider(cfg Config) (Provider, error)

// ResilientProvider 容错 Provider（主备切换）
func NewResilientProvider(cfg ResilientConfig) Provider
```

### 4.6 Memory Store 接口

```go
package memory

// Store 记忆存储接口
type Store interface {
    Add(ctx context.Context, episode Episode) error
    Search(ctx context.Context, query string, opts SearchOptions) ([]Episode, error)
    GetBySession(ctx context.Context, sessionID string) ([]Episode, error)
    Summarize(ctx context.Context, sessionID string) (*Summary, error)
    Delete(ctx context.Context, sessionID string) error
}

// 实现
func NewSQLiteStore(path string) (Store, error)          // FTS5 全文检索
func NewVectorStore(cfg VectorConfig) (Store, error)     // 向量语义搜索
func NewHybridStore(cfg HybridConfig) (Store, error)     // 混合模式
```

---

## 5. 使用示例

### 5.1 Level 1: 最简示例

```go
package main

import (
    "context"
    "fmt"
    "time"
    
    "github.com/castagent/castagent-go"
    "github.com/castagent/castagent-go/llm"
    "github.com/castagent/castagent-go/tools"
)

func main() {
    agent := castagent.NewReActAgent(castagent.ReActConfig{
        Name:    "Assistant",
        Model:   llm.NewDeepSeekProvider(&llm.Config{APIKey: "sk-xxx"}),
        Tools:   []castagent.Tool{tools.WebFetch{}},
    })
    
    resp, err := agent.Run(context.Background(),
        castagent.UserMessage("查询 Go 1.22 新特性"),
        castagent.WithTimeout(2*time.Minute),
    )
    
    fmt.Println(resp.Content)
}
```

### 5.2 Level 2: 多 Agent 协作

```go
pool := castagent.NewPool(castagent.PoolConfig{
    MaxConcurrency: 5,
})

results, _ := pool.Dispatch(context.Background(), []castagent.TaskConfig{
    {Title: "代码审查", Prompt: "检查 src/ 目录的代码质量"},
    {Title: "文档生成", Prompt: "生成 API 文档"},
    {Title: "测试用例", Prompt: "为核心模块写单元测试"},
})
```

### 5.3 Level 3: 生产级完整示例

```go
agent := castagent.NewReActAgent(castagent.ReActConfig{
    Name:    "ProductionAgent",
    Model:   llm.NewResilientProvider(llm.ResilientConfig{
        Primary:  llm.NewOpenAIProvider(...),
        Fallback: llm.NewDeepSeekProvider(...),
        Retry:    llm.RetryConfig{MaxRetries: 3},
    }),
    Memory: memory.NewHybridMemory(memory.HybridConfig{
        ShortTermSize: 10,
        LongTermStore: memory.NewSQLiteMemory("./data/memory.db"),
    }),
    Hooks: castagent.Hooks{
        OnReasoning: func(ctx context.Context, t *Thought) { ... },
        OnToolUse:   func(ctx context.Context, tc *ToolCall) { ... },
        OnError:     func(ctx context.Context, err error) { ... },
    },
    Security: &SecurityCfg{
        SandboxMode: true,
        AllowedTools: []string{"read_file", "search"},
        BlockedPaths: []string{".env", ".ssh"},
    },
})

resp, err := agent.Run(ctx, msg,
    castagent.WithCheckpoint("./checkpoints/"),
    castagent.WithStreaming(func(chunk string) { fmt.Print(chunk) }),
)
```

---

## 6. 多语言绑定策略

### 6.1 TypeScript SDK

```typescript
// @castagent/sdk - 基于 Go WASM 或 FFI

import { CastAgent, DeepSeekProvider, WebFetchTool } from '@castagent/sdk';

const agent = new CastAgent({
  name: 'TS Assistant',
  model: new DeepSeekProvider({ apiKey: process.env.API_KEY! }),
  tools: [new WebFetchTool()],
});

const response = await agent.run('Hello');

// Stream 支持
for await (const chunk of agent.stream('Write code')) {
  process.stdout.write(chunk);
}
```

**实现方式选择：**
- 方案 A: **Wasm 编译**（推荐）- Go 代码编译为 WASM，TS 直接调用
- 方案 B: **cgo + NAPI** - 性能更好但构建复杂
- 方案 C: **HTTP gRPC Gateway** - 松耦合但延迟高

### 6.2 Python SDK

```python
# castagent - 基于 cgo 或 PyO3

from castagent import CastAgent, DeepSeekProvider, WebFetchTool

agent = CastAgent(
    name="Python Assistant",
    model=DeepSeekProvider(api_key="sk-xxx"),
    tools=[WebFetchTool()]
)

response = agent.run("Hello")
print(response.content)
```

**实现方式选择：**
- 方案 A: **PyO3 + Rust bridge**（推荐）- 性能好，生态成熟
- 方案 B: **cgo + ctypes** - 直接但类型映射复杂
- 方案 C: **HTTP REST API** - 最简单但有网络开销

---

## 7. 从 CodeCast 提取的资产映射

| CodeCast 组件 | 框架模块 | 改造内容 |
|---------------|----------|----------|
| `agent_engine.go` | `cast/agent` | 移除 IDE 特定逻辑，抽象为通用 ReActLoop |
| `agent.go` (AgentPool) | `cast/pool` | 通用化，移除 Wails 依赖 |
| `agent_tools.go` | `cast/tools` | 抽象 Tool 接口，保留内置实现 |
| `memory.go` | `cast/memory` | 增强：添加向量记忆支持 |
| `context.go` | `cast/llm` | 扩展为多 Provider 支持 |
| `agent_persist.go` | `cast/persist` | 通用化 Checkpoint 机制 |
| `sandbox.go` + `command_filter.go` | `cast/security` | 抽象为可配置的安全策略 |

---

## 8. 项目结构（新仓库）

```
castagent/
├── cmd/
│   └── example/              # 示例应用
│       ├── hello/             # Level 1 示例
│       ├── multi-agent/       # Level 2 示例
│       └── production/        # Level 3 示例
├── internal/
│   ├── agent/                 # Agent Core 实现
│   │   ├── react_loop.go      # ReAct 循环引擎
│   │   ├── lifecycle.go       # 生命周期管理
│   │   └── hooks.go           # 钩子系统
│   ├── pool/                  # AgentPool 实现
│   │   ├── dispatcher.go      # 任务分发
│   │   ├── semaphore.go       # 并发控制
│   │   └── events.go          # 事件系统
│   ├── tools/                 # 工具系统
│   │   ├── registry.go        # 注册表
│   │   ├── executor.go        # 执行器
│   │   ├── builtin/           # 内置工具
│   │   │   ├── filesystem.go
│   │   │   ├── shell.go
│   │   │   ├── web.go
│   │   │   └── http.go
│   │   └── permission.go      # 权限控制
│   ├── memory/                # 记忆系统
│   │   ├── sqlite.go          # SQLite FTS5
│   │   ├── vector.go          # 向量数据库
│   │   └── hybrid.go          # 混合模式
│   ├── llm/                   # LLM 抽象层
│   │   ├── provider.go        # 接口定义
│   │   ├── openai.go
│   │   ├── deepseek.go
│   │   ├── anthropic.go
│   │   ├── ollama.go
│   │   └── resilient.go       # 容错包装
│   ├── persist/               # 持久化
│   │   ├── checkpoint.go
│   │   └── state.go
│   ├── security/              # 安全
│   │   ├── sandbox.go
│   │   └── acl.go
│   └── events/                # 事件总线
│       ├── bus.go
│       └── types.go
├── pkg/                       # 公共 API（用户导入）
│   ├── castagent.go           # 主入口
│   ├── agent.go               # Agent 接口
│   ├── pool.go                # Pool 接口
│   ├── tools.go               # Tool 接口
│   ├── memory.go              # Memory 接口
│   └── llm.go                 # LLM 接口
├── sdk/
│   ├── typescript/            # TS SDK
│   └── python/                # Python SDK
├── test/                      # 测试
│   ├── unit/                  # 单元测试
│   ├── integration/           # 集成测试
│   └── benchmark/             # 性能基准测试
├── docs/                      # 文档
│   ├── getting-started.md
│   ├── api-reference.md
│   ├── examples/
│   └── architecture.md
├── go.mod
├── go.sum
├── Makefile
├── LICENSE                   # Apache-2.0 or MIT
└── README.md
```

---

## 9. 路线图（MVP → Production）

### Phase 0: 基础设施（Week 1-2）
- [ ] 初始化仓库结构
- [ ] 定义核心接口（Agent, Tool, Memory, LLM）
- [ ] 实现 ReAct Loop 引擎
- [ ] 实现 OpenAI + DeepSeek Provider

### Phase 1: MVP（Week 3-4）
- [ ] AgentPool 并发调度
- [ ] 内置工具集（FileSystem, Shell, Web）
- [ ] SQLite Memory Store
- [ ] 3 个完整示例（Hello/Multi/Production）

### Phase 2: 增强特性（Week 5-6）
- [ ] Hook 系统（可观测性）
- [ ] Security Sandbox
- [ ] Checkpoint 持久化
- [ ] Resilient Provider（容错）
- [ ] Vector Memory（可选）

### Phase 3: 生态扩展（Week 7-8）
- [ ] TypeScript SDK（Wasm）
- [ ] CLI 工具
- [ ] 文档完善
- [ ] Benchmark 套件
- [ ] v0.1.0 发布

---

## 10. 成功指标

| 指标 | MVP 目标 | Production 目标 |
|------|----------|-----------------|
| **Hello Agent 代码量** | < 10 行 | < 5 行 |
| **单 Agent 内存占用** | < 30MB | < 20MB |
| **并发 Agent 数量** | 100 | 10000+ |
| **ReAct Loop 延迟（P99）** | < 500ms | < 200ms |
| **测试覆盖率** | > 80% | > 90% |
| **文档完整度** | Getting Started | Full API Reference |
| **GitHub Stars** | 100 | 1000+ |

---

## 11. 开源协议建议

| 协议 | 适用场景 |
|------|----------|
| **Apache-2.0**（推荐） | 企业友好，允许商业使用和修改 |
| MIT | 更宽松，适合最大化传播 |
| GPL v3 | 强制开源衍生作品 |

**建议**: Apache-2.0（与 AgentScope、LangChain 一致，企业采用门槛最低）

---

## 12. 待决策事项

在开始实施前，需要确认以下问题：

1. **框架名称**: CastAgent / GoAgent / 其他？
2. **仓库位置**: GitHub Organization 名称？
3. **开源协议**: Apache-2.0 / MIT？
4. **TypeScript SDK 优先级**: Phase 2 还是 Phase 3？
5. **是否需要 CLI 工具**?

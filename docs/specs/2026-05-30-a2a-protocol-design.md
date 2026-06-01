# A2A 协议实现设计规格

> 版本: v0.1 | 日期: 2026-05-30 | 状态: 已批准

## 1. 背景与目标

### 1.1 问题陈述

AgentPrimordia (AP) 当前具备基础的跨进程 Agent 通信能力（TCPTransport/HTTPTransport + Discovery），但缺乏标准化的 Agent 间协作协议。这导致：

- **无法与外部 A2A 生态互通**：LangGraph、CrewAI、AutoGen 等 A2A 兼容系统无法直接调用 AP Agent
- **任务生命周期不完整**：现有 BusMessage 是无状态消息投递，缺少 Task 追踪、状态机、产物交换
- **流式推送缺失**：长任务无法实时向调用方推送中间状态

### 1.2 设计目标

| 目标 | 描述 |
|------|------|
| **标准对齐** | 遵循 A2A Protocol v0.2+ 核心子集 (Google/Linux Foundation) |
| **内部协作** | 支持 AP 内部多 Agent 间任务委派和结果回传 |
| **异步流式** | 支持 SSE 流式状态推送 + Webhook 回调 |
| **安全认证** | Task 级别的 API Key / Bearer Token 认证 + 权限校验 |
| **渐进扩展** | 在现有 Transport/Discovery 基础上升级，非推翻重写 |

### 1.3 范围边界

**包含**:
- AgentCard 能力发现
- Task 完整生命周期 (7 状态)
- JSON-RPC 2.0 消息编解码
- SSE 流式事件推送
- 基础认证 (API Key / Bearer Token)
- 多模态 Parts (Text/File/Data)

**不含** (留待后续):
- 签名 AgentCard (v1.0 特性)
- 多租户支持
- Push Notification (Webhook) 回调
- mTLS 双向认证
- 工作流编排扩展 (ACP 对齐)

---

## 2. 架构设计

### 2.1 模块划分

```
agentprimordia/internal/agent/
├── a2a/                              # 新增 A2A 模块
│   ├── types.go                      # 核心数据类型
│   │   ├── AgentCard                 # Agent 能力声明
│   │   ├── Task                      # 任务实体
│   │   ├── TaskStatus                # 任务状态详情
│   │   ├── TaskState                 # 状态枚举与转换
│   │   ├── Message                   # A2A 消息
│   │   ├── Part 接口及实现            # TextPart/FilePart/DataPart
│   │   └── Artifact                  # 任务产出物
│   ├── jsonrpc.go                    # JSON-RPC 2.0 编解码
│   ├── server.go                     # A2A Server (HTTP Handler)
│   ├── client.go                     # A2A Client (调用远端 Agent)
│   ├── task_manager.go               # Task 生命周期管理器
│   ├── sse.go                        # SSE 流式推送
│   ├── auth.go                       # 认证接口与内置实现
│   ├── discovery_adapter.go          # Discovery → AgentCard 适配
│   ├── conversion.go                 # BusMessage ↔ A2AMessage 转换
│   └── a2a_test.go                   # 测试入口
│
├── transport.go                      # 现有，不变
├── http_transport.go                 # 升级: 复用为 A2A HTTP 后端
├── tcp_transport.go                  # 不变 (内部快速路径保留)
├── discovery.go                      # 扩展: 增加 GetAgentCard 方法
└── bus.go                            # 现有，不变
```

### 2.2 层次架构

```
┌─────────────────────────────────────────────┐
│         应用层 (Agent/Pool/Orchestration)      │
├─────────────────────────────────────────────┤
│              A2A 层                           │
│   client.go · server.go · task_manager.go     │
├─────────────────────────────────────────────┤
│     JSON-RPC 2.0 编解码 + SSE + Auth 中间件    │
├─────────────────────────────────────────────┤
│     Transport (HTTP/TCP) + Discovery          │
│     (复用 http_transport / discovery)          │
└─────────────────────────────────────────────┘
```

### 2.3 设计原则

1. **接口优先**: `A2AServer` / `A2AClient` / `TaskManager` / `Authenticator` 均通过接口定义
2. **并发安全**: 共享状态用 sync.RWMutex / channel 保护
3. **零外部依赖**: 仅使用 Go 标准库 (encoding/json, net/http, context)
4. **中文注释**: 代码注释使用中文

---

## 3. 核心数据模型

### 3.1 AgentCard (Agent 能力声明)

对应 A2A 规范的 `/.well-known/agent-card.json` 端点。

```go
// AgentCard A2A Agent 能力声明
type AgentCard struct {
    Protocol        string            `json:"protocol"`         // 固定 "a2a"
    AgentID         string            `json:"agent_id"`         // 全局唯一 ID
    Name            string            `json:"name"`             // 显示名称
    Description     string            `json:"description"`      // 能力描述
    
    Capabilities    AgentCapabilities `json:"capabilities"`     // 能力声明
    Endpoints       AgentEndpoints    `json:"endpoints"`        // 端点地址
    SecuritySchemes []SecurityScheme  `json:"security_schemes"` // 认证方式列表
    Skills          []AgentSkill      `json:"skills,omitempty"` // 技能列表
    Metadata        map[string]string `json:"metadata,omitempty"`
}

type AgentCapabilities struct {
    InputModes  []string `json:"input_modes"`   // 支持的输入类型
    OutputModes []string `json:"output_modes"`  // 支持的输出类型
    Streaming   bool     `json:"streaming"`     // 是否支持 SSE 流式
}

type AgentEndpoints struct {
    BaseURL       string `json:"base_url"`        // 基础 URL
    TaskSend      string `json:"task_send"`       // POST 路径
    TaskGet       string `json:"task_get"`        // GET 路径
    TaskCancel    string `json:"task_cancel"`     // POST 路径
    TaskSubscribe string `json:"task_subscribe"`  // GET SSE 路径
    AgentCardURL  string `json:"agent_card_url"`  // AgentCard URL
}

type SecurityScheme struct {
    Scheme AuthType `json:"scheme"`              // 认证类型
    In     string   `json:"in"`                  // "header" | "query"
    Name   string   `json:"name"`                // Header 名称
    Scopes []string `json:"scopes,omitempty"`     // 权限范围
}

type AgentSkill struct {
    ID          string `json:"id"`
    Name        string `json:"name"`
    Description string `json:"description,omitempty"`
    InputModes  []string `json:"input_modes,omitempty"`
    OutputModes []string `json:"output_modes,omitempty"`
}
```

### 3.2 Task & TaskStatus

Task 是 A2A 的核心原语，具有完整的生命周期状态机。

```go
// TaskState 任务状态枚举
type TaskState string

const (
    TaskSubmitted     TaskState = "submitted"      // 已提交，等待处理
    TaskWorking       TaskState = "working"        // 执行中
    TaskInputRequired TaskState = "input-required" // 需要人工输入
    TaskCompleted     TaskState = "completed"      // 成功完成
    TaskFailed        TaskState = "failed"         // 执行失败
    TaskCanceled      TaskState = "canceled"       // 已取消
    TaskRejected      TaskState = "rejected"       // 被拒绝
)

// Task A2A 任务实体
type Task struct {
    ID        string      `json:"id"`                    // UUID
    SessionID string      `json:"session_id,omitempty"`  // 会话 ID
    State     TaskState   `json:"state"`                 // 当前状态
    Message   *A2AMessage `json:"message"`               // 初始请求消息

    Status    *TaskStatus `json:"status,omitempty"`      // 执行状态详情
    Artifacts []Artifact  `json:"artifacts,omitempty"`   // 产出物列表

    CreatedAt time.Time  `json:"created_at"`
    UpdatedAt time.Time  `json:"updated_at"`
    ExpiresAt time.Time  `json:"expires_at,omitempty"`  // 过期时间

    mu sync.RWMutex
}

// TaskStatus 任务执行状态详情
type TaskStatus struct {
    State         TaskState   `json:"state"`
    ErrorMessage  string      `json:"error_message,omitempty"`
    StreamMessage *A2AMessage `json:"stream_message,omitempty"`
}
```

**状态机转换规则**:

```
                    ┌─────────────┐
                    │  submitted  │
                    └──────┬──────┘
                           │
              ┌────────────┼────────────┐
              ↓            ↓            ↓
        ┌──────────┐ ┌──────────┐ ┌──────────┐
        │ working  │ │ rejected │ │ canceled │
        └────┬─────┘ └──────────┘ └──────────┘
             │
    ┌────────┼────────┬──────────┐
    ↓        ↓        ↓          ↓
┌────────┐ ┌──────┐ ┌──────┐ ┌───────────────┐
│completed│ │failed│ │canceled│ │ input-required│→ working → ...
└────────┘ └──────┘ └──────┘ └───────────────┘
```

**合法转换表**:

| 当前状态 | 可转换到 |
|----------|----------|
| submitted | working, rejected, canceled |
| working | completed, failed, canceled, input-required |
| input-required | working, canceled |
| completed | - (终态) |
| failed | - (终态) |
| canceled | - (终态) |
| rejected | - (终态) |

### 3.3 Message & Part (多模态消息)

```go
// A2AMessage A2A 消息
type A2AMessage struct {
    Role      string `json:"role"`               // "user" | "agent"
    Parts     []Part `json:"parts"`              // 内容块列表
    MessageID string `json:"message_id,omitempty"`
    ParentID  string `json:"parent_id,omitempty"` // 父消息 ID
}

// Part 内容块联合类型接口
type Part interface {
    Type() string
}

// TextPart 文本内容
type TextPart struct {
    Text string `json:"text"`
}
func (t TextPart) Type() string { return "text" }

// FilePart 文件内容 (支持内联 base64 或外部 URI)
type FilePart struct {
    File     *FileWithBytes `json:"file,omitempty"`
    FileURI  *FileWithURI   `json:"file_uri,omitempty"`
    MimeType string         `json:"mimetype"`
    Filename string         `json:"filename,omitempty"`
}
func (f FilePart) Type() string { return "file" }

// DataPart 结构化数据
type DataPart struct {
    Data json.RawMessage `json:"data"`
}
func (d DataPart) Type() string { return "data" }

// FileWithBytes 内联文件 (base64 编码)
type FileWithBytes struct {
    Name     string `json:"name"`
    MimeType string `json:"mime_type"`
    Bytes    string `json:"bytes"` // base64 编码
}

// FileWithURI 外部文件引用
type FileWithURI struct {
    URI      string `json:"uri"`
    MimeType string `json:"mime_type"`
}

// Artifact 任务产出物
type Artifact struct {
    ArtifactID string    `json:"artifact_id"`
    MimeType   string    `json:"mimetype"`
    Bytes      []byte    `json:"bytes,omitempty"` // 内联数据
    URI        string    `json:"uri,omitempty"`    // 外部引用
    CreatedAt  time.Time `json:"created_at"`
}
```

### 3.4 JSON-RPC 信封

```go
// JSONRPCRequest JSON-RPC 2.0 请求
type JSONRPCRequest struct {
    JSONRPC string          `json:"jsonrpc"` // 固定 "2.0"
    ID      interface{}     `json:"id,omitempty"`
    Method  string          `json:"method"`
    Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse JSON-RPC 2.0 响应
type JSONRPCResponse struct {
    JSONRPC string          `json:"jsonrpc"`
    ID      interface{}     `json:"id,omitempty"`
    Result  json.RawMessage `json:"result,omitempty"`
    Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCError JSON-RPC 错误对象
type JSONRPCError struct {
    Code    int    `json:"code"`
    Message string `json:"message"`
    Data    string `json:"data,omitempty"`
}
```

**标准错误码**:

| Code | 含义 |
|------|------|
| -32700 | 解析错误 |
| -32600 | 无效请求 |
| -32601 | 方法不存在 |
| -32602 | 参数无效 |
| -32603 | 内部错误 |
| -32000 | 任务不存在 |
| -32001 | 任务状态冲突 |
| -32002 | 认证失败 |
| -32003 | 权限不足 |

---

## 4. API 设计

### 4.1 HTTP 路由表

| Method | Path | JSON-RPC Method | 说明 |
|--------|------|-----------------|------|
| POST | `/a2a` | tasks/send | 创建并执行任务 |
| GET | `/a2a/tasks/{id}` | tasks/get | 查询任务状态 |
| POST | `/a2a/tasks/{id}/cancel` | tasks/cancel | 取消任务 |
| POST | `/a2a/tasks/{id}/event` | tasks/sendEvent | 发送事件 |
| GET | `/a2a/tasks/{id}/events` | - | SSE 流式订阅 |
| GET | `/.well-known/agent-card.json` | agent/card | 获取能力声明 |

### 4.2 核心接口定义

```go
// A2AServer A2A 服务端接口
type A2AServer interface {
    Start(addr string) error
    Close() error
    AgentCard() *AgentCard
    TaskManager() TaskManager
}

// A2AClient A2A 客户端接口
type A2AClient interface {
    GetAgentCard(ctx context.Context, baseURL string) (*AgentCard, error)
    SendTask(ctx context.Context, task *Task) (*Task, error)
    SendTaskAsync(ctx context.Context, task *Task) (string, error)
    GetTask(ctx context.Context, taskID string) (*Task, error)
    CancelTask(ctx context.Context, taskID string) error
    SubscribeTask(ctx context.Context, taskID string) (<-chan *TaskEvent, error)
    SendEvent(ctx context.Context, taskID string, event *TaskEvent) error
}

// TaskManager 任务管理器接口
type TaskManager interface {
    Create(task *Task) (*Task, error)
    Get(taskID string) (*Task, error)
    Update(taskID string, state TaskState, status *TaskStatus) error
    AddArtifact(taskID string, artifact Artifact) error
    Cancel(taskID string) error
    Subscribe(taskID string) <-chan *TaskEvent
    Unsubscribe(taskID string, ch <-chan *TaskEvent)
    List(filter TaskFilter) []*Task
    Cleanup()
}

// Authenticator 认证器接口
type Authenticator interface {
    Authenticate(r *http.Request) (*Principal, error)
}
```

### 4.3 请求/响应示例

**tasks/request 创建任务**:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tasks/send",
  "params": {
    "task": {
      "id": "task-001",
      "message": {
        "role": "user",
        "parts": [
          {"type": "text", "text": "分析销售数据并生成报告"}
        ]
      }
    }
  }
}
```

**成功响应 (completed)**:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "task": {
      "id": "task-001",
      "state": "completed",
      "status": {"state": "completed"},
      "artifacts": [
        {
          "artifact_id": "report-001",
          "mimetype": "application/pdf",
          "uri": "https://storage.example.com/reports/report-001.pdf"
        }
      ],
      "message": {
        "role": "agent",
        "parts": [{"type": "text", "text": "报告已生成"}]
      }
    }
  }
}
```

---

## 5. SSE 流式推送

### 5.1 事件类型

```go
// TaskEventType SSE 事件类型
type TaskEventType string

const (
    EventStateChange TaskEventType = "state_change"   // 状态变更
    EventMessage     TaskEventType = "message"        // 新消息
    EventArtifact    TaskEventType = "artifact"       // 新产物
    EventError       TaskEventType = "error"          // 错误
    EventCanceled    TaskEventType = "canceled"       // 取消通知
)

// TaskEvent SSE 事件
type TaskEvent struct {
    Type      TaskEventType `json:"type"`
    TaskID    string        `json:"task_id"`
    Timestamp time.Time     `json:"timestamp"`

    State    *TaskState    `json:"state,omitempty"`
    Message  *A2AMessage   `json:"message,omitempty"`
    Artifact *Artifact     `json:"artifact,omitempty"`
    Error    string        `json:"error,omitempty"`
}
```

### 5.2 SSE 事件流示例

```
GET /a2a/tasks/task-001/events

HTTP/1.1 200 OK
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive

data: {"type":"state_change","task_id":"task-001","state":"working","timestamp":"..."}

data: {"type":"message","task_id":"task-001","message":{"role":"agent","parts":[{"type":"text","text":"正在分析数据..."}]},"timestamp":"..."}

data: {"type":"artifact","task_id":"task-001","artifact":{"artifact_id":"chart-001","mimetype":"image/png","uri":"..."},"timestamp":"..."}

data: {"type":"state_change","task_id":"task-001","state":"completed","timestamp":"..."}
```

### 5.3 实现要点

- 使用 `http.Flusher` 接口确保实时推送
- 每个 Task 维护一个 subscriber map (`map[string][]chan *TaskEvent`)
- 连接断开时自动清理 subscriber
- 设置合理的超时和心跳机制

---

## 6. 安全认证

### 6.1 分层安全模型

```
┌──────────────────────────────────────┐
│         Transport 层                  │
│   TLS 1.2+ (可选 mTLS)               │
├──────────────────────────────────────┤
│         A2A 认证层                    │
│   API Key / Bearer Token             │
├──────────────────────────────────────┤
│         Task 级授权                   │
│   Session 绑定 + Scope 校验           │
└──────────────────────────────────────┘
```

### 6.2 认证方式

```go
// AuthType 认证类型
type AuthType string

const (
    AuthNone   AuthType = "none"     // 无认证 (开发用)
    AuthAPIKey AuthType = "api_key"  // API Key
    AuthBearer AuthType = "bearer"   // JWT/OAuth2 Bearer
    AuthMTLS   AuthType = "mtls"     // 双向 TLS (预留)
)

// Principal 已认证主体信息
type Principal struct {
    ID       string            `json:"id"`
    Roles    []string          `json:"roles"`
    Scopes   []string          `json:"scopes"`
    Metadata map[string]string `json:"metadata,omitempty"`
}
```

### 6.3 内置认证器

| 实现类 | 说明 | 使用场景 |
|--------|------|----------|
| `NoopAuthenticator` | 跳过所有认证检查 | 开发/测试环境 |
| `APIKeyAuthenticator` | 校验 `X-API-Key` Header | 内部服务调用 |
| `BearerTokenAuthenticator` | 校验 JWT `Authorization: Bearer` | 生产环境 |

### 6.4 认证流程

```
HTTP Request
    ↓
TLS 握手 (如启用)
    ↓
Auth Middleware
    ↓ 从 Header 提取凭证
    ↓ 调用 Authenticator.Authenticate()
    ↓ 成功 → Principal 注入 Context
    ↓ 失败 → 返回 JSON-RPC Error (-32002)
    ↓
Handler 执行业务逻辑
    ↓ 从 Context 读取 Principal
    ↓ Scope 校验 (可选)
```

---

## 7. 与现有代码集成

### 7.1 Discovery 扩展

在现有 `Discovery` 接口新增 `GetAgentCard` 方法:

```go
type Discovery interface {
    Register(ctx context.Context, info *AgentInfo) error
    Unregister(ctx context.Context, agentID string) error
    Discover(ctx context.Context, agentID string) (*AgentInfo, error)
    ListAgents(ctx context.Context) ([]*AgentInfo, error)
    Heartbeat(ctx context.Context, agentID string) error
    Close() error

    // 新增: 获取 A2A AgentCard
    GetAgentCard(ctx context.Context, agentID string) (*AgentCard, error)
}
```

`LocalDiscovery` 实现: 将 `AgentInfo` 映射为 `AgentCard` 格式。

### 7.2 HTTPTransport 复用

`A2AServerImpl` 内部持有 `*HTTPTransport`，在其上注册 A2A 路由:

```go
type A2AServerImpl struct {
    http    *HTTPTransport
    taskMgr *TaskManager
    agent   Agent
    card    *AgentCard
    auth    Authenticator
    logger  *slog.Logger
}
```

需要为 `HTTPTransport` 增加动态路由注册能力 (或直接操作底层 `http.ServeMux`)。

### 7.3 消息转换桥

提供 `BusMessage` ↔ `A2AMessage` 的双向转换函数，位于 `conversion.go`:

- `BusMessageToA2A(*BusMessage) *A2AMessage`
- `A2AToBusMessage(*A2AMessage) *BusMessage`
- `ExtractTextFromParts([]Part) string` — 从 Parts 中提取纯文本

---

## 8. 测试策略

### 8.1 测试矩阵

| 测试文件 | 覆盖范围 | 测试方法 |
|----------|----------|----------|
| `types_test.go` | 数据模型序列化/反序列化 | 单元测试 |
| `jsonrpc_test.go` | JSON-RPC 编解码、错误处理 | 表驱动测试 |
| `task_manager_test.go` | 状态机转换、并发安全 | 并发测试 |
| `server_test.go` | HTTP Handler 集成 | httptest.Server |
| `client_test.go` | 远端调用流程 | Mock Server |
| `sse_test.go` | SSE 流式推送 | httptest + channel 验证 |
| `auth_test.go` | 认证中间件 | 表驱动测试 |
| `discovery_adapter_test.go` | Discovery 适配 | Mock Discovery |
| `conversion_test.go` | 消息双向转换 | 单元测试 |
| `integration_test.go` | Client↔Server 全链路 | httptest 端到端 |

### 8.2 关键测试场景

1. **Task 完整生命周期**: submit → working → completed
2. **Task 取消流程**: submit → working → canceled
3. **Input-Required 流程**: submit → working → input-required → working → completed
4. **SSE 事件顺序验证**: 确保状态变更和消息顺序一致
5. **认证失败场景**: 无凭证/无效凭证/过期凭证
6. **并发安全性**: 多 goroutine 同时创建/查询 Task
7. **消息转换完整性**: BusMessage ↔ A2AMessage 双向无损转换
8. **超时清理**: 过期 Task 自动清理

### 8.3 Mock 策略

- **LLM**: 使用已有的 `MockLLM`
- **远端 Server**: 使用 `httptest.NewServer`
- **Authenticator**: 使用 `NoopAuthenticator` 或自定义 mock

---

## 9. 实施计划

### Phase 1: 核心骨架 (预计代码量 ~800 行)

1. `types.go` — 所有数据类型定义
2. `jsonrpc.go` — JSON-RPC 2.0 编解码
3. `task_manager.go` — Task 存储与状态机
4. `auth.go` — 认证接口 + Noop/APIKey 实现
5. 对应测试文件

### Phase 2: Server & Client (预计代码量 ~500 行)

6. `server.go` — A2AServerImpl + HTTP Handler
7. `client.go` — A2AClientImpl + 远端调用
8. `sse.go` — SSE 推送实现
9. `discovery_adapter.go` — Discovery 扩展
10. `conversion.go` — 消息转换桥
11. 集成测试

### Phase 3: 集成与优化 (预计代码量 ~200 行)

12. 与 Orchestration/Pool 层集成
13. 性能基准测试
14. 文档补充

---

## 10. 风险与缓解

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| JSON-RPC 规范理解偏差 | 外部兼容性问题 | 严格参考官方 spec；增加兼容性测试 |
| SSE 在高并发下的内存压力 | 泄漏风险 | 设置 subscriber 上限 + 超时清理 |
| Task 状态机复杂度 | 边界条件 bug | 充分的单元测试覆盖所有转换路径 |
| 与现有 Transport 耦合 | 改动范围扩大 | 通过接口隔离，最小化侵入 |

---

## 附录 A: 参考

- [A2A Protocol Specification v0.2.5](https://a2a-protocol.org/v0.2.5/specification/)
- [A2A GitHub Repository](https://github.com/agentic-ai/a2a)
- [JSON-RPC 2.0 Specification](https://www.jsonrpc.org/specification)
- [Server-Sent Events (W3C)](https://html.spec.whatwg.org/multipage/server-sent-events.html)

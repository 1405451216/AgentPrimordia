# A2A 协议实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 AgentPrimordia 中实现 A2A Protocol v0.2+ 核心子集，支持 AgentCard 发现、Task 生命周期管理、JSON-RPC 通信、SSE 流式推送和基础认证。

**Architecture:** 新增 `internal/agent/a2a/` 模块，包含 types/jsonrpc/server/client/task_manager/sse/auth/discovery_adapter/conversion 共 10 个文件。底层复用现有 HTTPTransport 和 Discovery，通过接口解耦。遵循 TDD 流程：先写测试（Red），再实现（Green），最后重构。

**Tech Stack:** Go 1.22+ 标准库 (encoding/json, net/http, context, sync), 无外部依赖

**设计规格:** [docs/specs/2026-05-30-a2a-protocol-design.md](../specs/2026-05-30-a2a-protocol-design.md)

---

## 文件结构总览

```
internal/agent/a2a/                    # 新增目录
├── types.go                           # AgentCard / Task / TaskStatus / Message / Part / Artifact
├── jsonrpc.go                         # JSON-RPC 2.0 Request/Response 编解码 + 错误码
├── task_manager.go                    # Task 存储与状态机管理器
├── auth.go                            # Authenticator 接口 + Noop/APIKey/Bearer 实现
├── sse.go                             # SSE 事件推送 (TaskEvent 类型定义也在此)
├── server.go                          # A2AServer 接口 + A2AServerImpl 实现
├── client.go                          # A2AClient 接口 + A2AClientImpl 实现
├── discovery_adapter.go               # Discovery.GetAgentCard 扩展适配
├── conversion.go                      # BusMessage <-> A2AMessage 双向转换
└── a2a_test.go                        # 集成测试入口

internal/agent/                        # 现有文件修改
├── discovery.go                       # 扩展 Discovery 接口 + LocalDiscovery 实现
└── http_transport.go                  # 增加动态路由注册能力
```

---

### Task 1: 核心数据类型定义

**Files:** Create `internal/agent/a2a/types.go`, Test: `internal/agent/a2a/types_test.go`

- [ ] **Step 1: 编写 AgentCard 序列化测试**
  验证 AgentCard 的 JSON Marshal/Unmarshal 往返一致性，包括 Capabilities(Streaming)、Endpoints、SecuritySchemes 字段

- [ ] **Step 2: 运行测试确认失败**
  Run: `go test ./internal/agent/a2a/ -run TestAgentCard -v`
  Expected: FAIL (类型不存在)

- [ ] **Step 3: 实现核心数据类型**
  定义以下全部类型:
  - AuthType 枚举 (None/APIKey/Bearer/MTLS)
  - SecurityScheme, AgentSkill, AgentCapabilities, AgentEndpoints, AgentCard
  - TaskState 枚举 (7个状态) + IsValidTransition() + IsTerminal()
  - TaskStatus, Task 结构体
  - Part 接口 + TextPart/FilePart(FileWithBytes+FileWithURI)/DataPart
  - A2AMessage (含自定义 JSON marshal 处理 Part 接口)
  - Artifact 结构体

- [ ] **Step 4: 补充更多类型测试并运行**
  测试覆盖: TaskState 转换合法性表(合法/非法/终态)、Part 多态序列化、Artifact 字段完整性
  Run: `go test ./internal/agent/a2a/ -v`
  Expected: 全部 PASS

- [ ] **Step 5: 提交**
  `git commit -m "feat(a2a): 定义核心数据类型 AgentCard/Task/Message/Part/Artifact"`

---

### Task 2: JSON-RPC 2.0 编解码层

**Files:** Create `internal/agent/a2a/jsonrpc.go`, Test: `internal/agent/a2a/jsonrpc_test.go`

- [ ] **Step 1: 编写 JSON-RPC 编解码测试**
  - TestJSONRPCRequest_Marshal: 验证 jsonrpc/version/method/params 字段正确序列化
  - TestJSONRPCResponse_Error: 验证错误响应包含 code/message
  - TestJSONRPCResponse_Success: 验证成功响应不含 error 字段
  - TestJSONRPC_UnmarshalRequest: 验证反版本校验(非2.0返回错误)
  - TestJSONRPC_InvalidRequest: 验证畸形请求处理

- [ ] **Step 2: 运行测试确认失败**

- [ ] **Step 3: 实现 JSON-RPC 编解码**
  定义 JSONRPCRequest/JSONRPCResponse/JSONRPCError 结构体
  实现自定义 UnmarshalJSON (版本校验)
  定义标准错误码常量 (-32700 ~ -32003)
  实现构造函数: NewJSONRPCResult/NewJSONRPCError/NewMethodNotFoundError/NewParamsError/NewAuthFailedError/NewTaskNotFoundError/NewTaskConflictError/NewInternalError

- [ ] **Step 4: 运行测试确认通过**
  Run: `go test ./internal/agent/a2a/ -run TestJSONRPC -v`

- [ ] **Step 5: 提交**
  `git commit -m "feat(a2a): 实现 JSON-RPC 2.0 编解码与标准错误码"`

---

### Task 3: SSE 事件系统 + 认证接口

**Files:** Create `internal/agent/a2a/sse.go` + `internal/agent/a2a/auth.go`, 对应 _test.go

- [ ] **Step 1: 编写 SSE 事件和认证测试**
  SSE 测试:
  - TestTaskEvent_Marshal: StateChange/Message/Artifact 三种事件的 JSON 序列化
  - TestSSEFormat: 验证 FormatSSEEvent 输出格式 (`data: {...}\n\n`)
  认证测试:
  - TestNoopAuthenticator_Passes: 始终返回 Principal
  - TestAPIKeyAuthenticator_ValidKey/MissingHeader/InvalidKey: 三种场景
  - TestBearerTokenAuthenticator_ValidToken: Bearer 格式解析
  - TestPrincipal_HasScope/HasRole: 权限检查方法

- [ ] **Step 2: 运行测试确认失败**

- [ ] **Step 3: 实现 SSE 事件系统和认证**
  sse.go:
  - TaskEventType 常量 (5种事件类型)
  - TaskEvent 结构体 (Type/TaskID/Timestamp/State/Message/Artifact/Error)
  - FormatSSEEvent() 函数
  auth.go:
  - Principal 结构体 + HasScope()/HasRole() 方法
  - Authenticator 接口
  - NoopAuthenticator 实现
  - APIKeyAuthenticator 实现 (map[string]string 存储 key->principalID)
  - BearerTokenAuthenticator 实现 (注入 Validator 函数)

- [ ] **Step 4: 运行测试确认通过**
  Run: `go test ./internal/agent/a2a/ -run 'TestTaskEvent|TestSSE|Test.*Auth|TestPrincipal' -v`

- [ ] **Step 5: 提交**
  `git commit -m "feat(a2a): 实现 SSE 事件系统和可插拔认证中间件"`

---

### Task 4: TaskManager 任务管理器

**Files:** Create `internal/agent/a2a/task_manager.go`, Test: `internal/agent/a2a/task_manager_test.go`

- [ ] **Step 1: 编写 TaskManager 测试**
  - TestTaskManager_CreateAndGet: 创建后可查询
  - TestTaskManager_StateTransition: submitted→working→completed 合法链
  - TestTaskManager_InvalidTransition: submitted→completed 非法(跳过working)
  - TestTaskManager_TerminalStateNoTransition: completed→working 应失败
  - TestTaskManager_Cancel: working→canceled
  - TestTaskManager_AddArtifact: 添加产物到 working 任务
  - TestTaskManager_GetNotFound: 不存在返回错误
  - TestTaskManager_ConcurrentAccess: 10 goroutine 并发创建，List 验证数量
  - TestTaskManager_SubscribeAndPublish: 订阅后 Update 触发事件接收

- [ ] **Step 2: 运行测试确认失败**

- [ ] **Step 3: 实现 TaskManager**
  - TaskManager 接口定义 (Create/Get/Update/AddArtifact/Cancel/Subscribe/Unsubscribe/List/Cleanup)
  - TaskFilter 过滤结构体
  - TaskManagerImpl 实现:
    - map[string]*Task 存储 + sync.RWMutex 保护
    - Create: 深拷贝存储 + 时间戳初始化
    - Update: IsValidTransition 校验 + publishEvent 通知订阅者
    - AddArtifact: 追加产物 + 发布 Artifact 事件
    - Subscribe/Unsubscribe: subscriber map 管理 (map[taskID]map[chan]struct{})
    - List: 按时间倒序 + State 过滤 + Limit 截断
    - Cleanup: 清空所有数据
    - deepCopyTask 辅助函数 (深拷贝 Task 含 Message/Status/Artifacts)

- [ ] **Step 4: 运行测试确认通过**
  Run: `go test ./internal/agent/a2a/ -run TestTaskManager -v`

- [ ] **Step 5: 提交**
  `git commit -m "feat(a2a): 实现 TaskManager 任务生命周期管理器"`

---

### Task 5: A2A Server 实现

**Files:** Create `internal/agent/a2a/server.go`, Test: `internal/agent/a2a/server_test.go`. Modify: `http_transport.go`

- [ ] **Step 1: 扩展 HTTPTransport 支持动态路由注册**
  在 http_transport.go 中添加 RegisterRoute(pattern, handler) 方法，允许外部模块向 ServeMux 注册路由

- [ ] **Step 2: 编写 A2AServer 测试**
  - TestA2AServer_AgentCardEndpoint: GET /.well-known/agent-card.json 返回正确 JSON
  - TestA2AServer_SendTask: POST /a2a tasks/send 创建任务并异步执行
  - TestA2AServer_GetTask: tasks/get 查询已创建任务
  - TestA2AServer_GetTaskNotFound: 不存在任务返回 -32000 错误
  - TestA2AServer_CancelTask: tasks/cancel 取消 working 状态任务
  - TestA2AServer_InvalidMethod: 未知方法返回 -32601
  - TestA2AServer_SSESubscribe: GET /tasks/{id}/events 返回 text/event-stream 并推送事件
  - TestA2AServer_AuthFailure: 无效凭证返回 -32002 错误
  使用 httptest.Server + newMockAgent (立即返回的Agent) + newSlowMockAgent (保持working的Agent)

- [ ] **Step 3: 运行测试确认失败**

- [ ] **Step 4: 实现 A2AServer**
  - A2AServer 接口 (Start/Close/AgentCard/TaskManager)
  - A2AServerImpl 结构体 (taskMgr/agent/card/auth/server/mu/started/logger/testAddr)
  - NewA2AServer(agent, card, auth) 构造函数
  - handler() 返回 http.Handler (注册 /a2a, /.well-known/agent-card.json, /a2a/tasks/)
  - Start(addr): 启动 http.Server
  - Close(): Shutdown + Cleanup
  - handleA2A(): 认证 → 解析 JSON-RPC → dispatch
  - dispatch(): 方法路由 (tasks/send/tasks/get/tasks/cancel/tasks/sendEvent/agent/card)
  - handleSendTask: Create → Update(working) → goroutine executeTask
  - executeTask: agent.Run(ctx, input) → Update(completed/failed) + publishEvent
  - handleGetTask/handleCancelTaskRPC/handleSendEvent/handleGetAgentCard: 各方法处理器
  - handleSSESubscribe: text/event-stream header → Subscribe → 循环 Flush
  - handleAgentCard: 返回 card JSON
  - 辅助: setPrincipal/GetPrincipal (context.Value), writeJSONRPCError, unmarshalParams, extractTaskIDFromPath, messageToInput, ExtractTextFromParts

- [ ] **Step 5: 运行测试确认通过**
  Run: `go test ./internal/agent/a2a/ -run TestA2AServer -v`

- [ ] **Step 6: 提交**
  `git commit -m "feat(a2a): 实现 A2A Server (JSON-RPC Handler + SSE)"`

---

### Task 6: A2A Client 实现

**Files:** Create `internal/agent/a2a/client.go`, Test: `internal/agent/a2a/client_test.go`

- [ ] **Step 1: 编写 A2AClient 测试**
  使用 httptest.NewServer 模拟远端 A2A Server:
  - TestClient_GetAgentCard: 获取远端 Agent 能力声明
  - TestClient_SendTask: 发送同步任务并等待完成
  - TestClient_SendTaskAsync: 异步发送仅返回 taskID
  - TestClient_GetTask: 查询任务状态
  - TestClient_CancelTask: 取消远端任务
  - TestClient_SubscribeTask: SSE 订阅远端任务事件流
  - TestClient_SendEvent: 发送 input-required 事件的输入
  - TestClient_NetworkError: 远端不可达时返回错误

- [ ] **Step 2: 运行测试确认失败**

- [ ] **Step 3: 实现 A2AClient**
  - A2AClient 接口 (GetAgentCard/SendTask/SendTaskAsync/GetTask/CancelTask/SubscribeTask/SendEvent)
  - A2AClientImpl 结构体 (baseURL/client/apiKey)
  - NewA2AClient(baseURL, apiKey) 构造函数
  - GetAgentCard: GET {baseURL}/.well-known/agent-card.json → 解码 AgentCard
  - SendTask: POST {baseURL}/a2a (tasks/send) → 解码 Task 结果
  - SendTaskAsync: 同上但不等待完整结果
  - GetTask: POST {baseURL}/a2a (tasks/get) → 解码 Task
  - CancelTask: POST {baseURL}/a2a (tasks/cancel)
  - SubscribeTask: GET {baseURL}/a2a/tasks/{id}/events → 解析 SSE 行 → chan *TaskEvent
  - SendEvent: POST {baseURL}/a2a (tasks/sendEvent)
  - 内部辅助: callRPC(method, params) 封装 JSON-RPC 调用流程

- [ ] **Step 4: 运行测试确认通过**
  Run: `go test ./internal/agent/a2a/ -run TestClient -v`

- [ ] **Step 5: 提交**
  `git commit -m "feat(a2a): 实现 A2A Client (远端 Agent 调用)"`

---

### Task 7: Discovery 适配器 + 消息转换桥

**Files:** Create `internal/agent/a2a/discovery_adapter.go`, `internal/agent/a2a/conversion.go`, 对应 _test.go. Modify: `internal/agent/discovery.go`

- [ ] **Step 1: 编写适配器和转换测试**
  discovery_adapter_test.go:
  - TestDiscoveryAdapter_GetAgentCard: LocalDiscovery.GetAgentCard 将 AgentInfo 映射为 AgentCard
  - TestDiscoveryAdapter_AgentCardFields: 验证 Capabilities/Endpoints/Protocol 字段正确填充
  conversion_test.go:
  - TestBusMessageToA2A: BusMessage → A2AMessage (Content→TextPart, Source→Role)
  - TestA2AToBusMessage: A2AMessage → BusMessage (Parts→Content)
  - TestExtractTextFromParts: TextPart/FilePart/DataPart 的文本提取
  - TestConversion_RoundTrip: 双向转换后关键信息不丢失

- [ ] **Step 2: 运行测试确认失败**

- [ ] **Step 3: 实现 Discovery 适配和消息转换**
  discovery_adapter.go:
  - 在 Discovery 接口增加 GetAgentCard(ctx, agentID) (*AgentCard, error) 方法
  - LocalDiscovery 实现: Discover(agentID) → AgentInfo → AgentCard 映射
  conversion.go:
  - BusMessageToA2A(*BusMessage) *A2AMessage
  - A2AToBusMessage(*A2AMessage) *BusMessage
  - ExtractTextFromParts([]Part) string — 遍历 Parts 提取 TextPart.Text
  - PartsToMessage(parts []Part) Message — A2AMessage.Parts → agent.Message

- [ ] **Step 4: 运行测试确认通过**
  Run: `go test ./internal/agent/a2a/ -run 'TestDiscoveryAdapter|TestConversion' -v`

- [ ] **Step 5: 提交**
  `git commit -m "feat(a2a): 实现 Discovery 适配器和消息转换桥"`

---

### Task 8: 集成测试与端到端验证

**Files:** Create `internal/agent/a2a/integration_test.go`

- [ ] **Step 1: 编写集成测试**
  - TestIntegration_ClientServerRoundTrip: Client→Server 完整请求-响应周期
    - 启动 A2AServer (httptest)
    - 用 A2AClient 连接
    - SendTask → GetTask(验证state=completed) → CancelTask(已完成的任务取消应失败)
  - TestIntegration_MultiAgentDiscovery: 通过 Discovery 发现 Agent 后调用
    - LocalDiscovery.Register → GetAgentCard → A2AClient.SendTask
  - TestIntegration_SSEFullLifecycle: SSE 事件完整顺序验证
    - SendTask → Subscribe → 收集所有事件 → 验证顺序: state_change(working) → message → state_change(completed)
  - TestIntegration_AuthFlow: API Key 认证全流程
    - 有 Key: 正常调用; 无 Key/错 Key: 返回 -32002

- [ ] **Step 2: 运行集成测试**
  Run: `go test ./internal/agent/a2a/ -run TestIntegration -v`

- [ ] **Step 3: 运行全量测试确保无回归**
  Run: `go test ./internal/agent/a2a/ -v && go test ./internal/agent/ -v`
  Expected: 全部 PASS (包括现有测试不受影响)

- [ ] **Step 4: 提交**
  `git commit -m "test(a2a): 添加端到端集成测试"`

---

### Task 9: 最终验证与清理

- [ ] **Step 1: 运行整个项目测试套件**
  Run: `cd agentprimordia && go test ./... -count=1`
  Expected: 所有包 PASS, 无编译错误

- [ ] **Step 2: 检查代码覆盖率**
  Run: `go test ./internal/agent/a2a/ -coverprofile=a2a_cover.out && go tool cover -func=a2a_cover.out`
  目标: 核心模块覆盖率 > 80%

- [ ] **Step 3: 更新设计文档引用**
  确认 docs/specs/2026-05-30-a2a-protocol-design.md 中的代码示例与实际一致

- [ ] **Step 4: 最终提交**
  `git commit -m "feat(a2a): 完成 A2A 协议核心子集实现"`

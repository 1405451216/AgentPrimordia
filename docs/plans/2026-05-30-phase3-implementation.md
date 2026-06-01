# Phase 3: 高级编排与分布式能力 — 实施计划

> **日期**: 2026-05-30
> **状态**: Ready to Execute
> **前置条件**: Phase 2 (Tasks 18-31) 已完成，全量编译通过，~300+ tests 全通过
> **后续**: Phase 4 (Tasks 44-55) 差距补齐与生产就绪

---

## 总览

| # | Task | 子阶段 | 模块 | 来源 | 复杂度 | 预估测试数 |
|:-:|------|:------:|------|:----:|:------:|:----------:|
| 32 | Agent 评估框架 | 3-A | agent/ | 新功能 | ⭐⭐⭐ | ~10 |
| 33 | 插件化工具系统 | 3-A | tools/ | 架构 | ⭐⭐⭐ | ~10 |
| 34 | TCP 传输层增强 | 3-B | agent/transport | 新功能 | ⭐⭐⭐ | ~8 |
| 35 | Discovery 服务认证 | 3-B | agent/discovery | 新功能 | ⭐⭐ | ~7 |
| 36 | 分布式集成验证 | 3-B | integration | 新功能 | ⭐⭐ | ~5 |
| 37 | Ollama Provider 完善 | 3-C | llm/ | 新功能 | ⭐⭐ | ~6 |
| 38 | Admin 面板增强 | 3-C | admin/ | 增强 | ⭐⭐ | ~6 |
| 39 | Memory SummaryStrategy | 3-C | memory/ | 增强 | ⭐⭐ | ~7 |
| 40 | 工作流可视化 | 3-C | orchestration/ | 新功能 | ⭐⭐ | ~5 |
| 41 | MCP 协议增强 | 3-C | tools/ | 增强 | ⭐⭐ | ~6 |

**预估新增**: ~8 个文件，~2500 行代码，**~70 个新测试**

---

## 子阶段 3-A: 评估框架 + 插件系统（P0）

### Task 32: Agent 评估框架

**来源**: 生产级 Agent 需要质量评估能力
**目标**: 支持对 Agent 输出质量、工具使用效率、推理路径进行自动化评估

#### 文件清单

| 操作 | 文件路径 | 说明 |
|------|---------|------|
| 新增 | `internal/agent/eval.go` | 评估框架核心 |
| 新增 | `internal/agent/eval_test.go` | 测试 |

#### 实现步骤

1. **定义评估接口**
   ```go
   type Evaluator interface {
       Evaluate(ctx context.Context, input EvalInput) (*EvalResult, error)
   }

   type EvalInput struct {
       Task        string
       AgentOutput *Response
       Expected    string
       Metadata    map[string]any
   }

   type EvalResult struct {
       Score       float64
       Passed      bool
       Criteria    []CriterionResult
   }

   type CriterionResult struct {
       Name    string
       Score   float64
       Passed  bool
       Reason  string
   }
   ```

2. **内置评估器**
   - `ExactMatchEvaluator`: 精确匹配评估
   - `ContainsEvaluator`: 包含关键词评估
   - `LLMEvaluator`: 使用 LLM 作为评判者（LLM-as-Judge）
   - `ToolUsageEvaluator`: 工具使用效率评估
   - `CompositeEvaluator`: 组合多个评估器

3. **评估运行器**
   ```go
   type EvalRunner struct {
       agent     Agent
       evaluators []Evaluator
   }

   func (r *EvalRunner) RunSuite(ctx context.Context, cases []EvalCase) (*EvalSuiteResult, error)
   ```

4. **评估报告**
   - 生成结构化评估报告
   - 支持通过率、平均分、各维度分析

#### 测试用例清单

```
TestExactMatchEvaluator_Match
TestExactMatchEvaluator_NoMatch
TestContainsEvaluator_Found
TestContainsEvaluator_NotFound
TestContainsEvaluator_MultipleKeywords
TestLLMEvaluator_Success
TestLLMEvaluator_Error
TestToolUsageEvaluator_CorrectTool
TestToolUsageEvaluator_WrongTool
TestCompositeEvaluator_AllPass
```

---

### Task 33: 插件化工具系统

**来源**: 架构需求 — 工具应支持动态加载和第三方扩展
**目标**: 支持工具的动态注册、发现和加载，提供插件接口

#### 文件清单

| 操作 | 文件路径 | 说明 |
|------|---------|------|
| 新增 | `internal/tools/plugin.go` | 插件接口和加载器 |
| 新增 | `internal/tools/plugin_test.go` | 测试 |
| 修改 | `internal/tools/registry.go` | 支持动态注册和分类 |

#### 实现步骤

1. **定义插件接口**
   ```go
   type ToolPlugin interface {
       Name() string
       Version() string
       Tools() []Tool
       Init(config map[string]any) error
       Close() error
   }
   ```

2. **插件加载器**
   ```go
   type PluginLoader struct {
       registry *Registry
       plugins  map[string]ToolPlugin
   }

   func (l *PluginLoader) Load(plugin ToolPlugin) error
   func (l *PluginLoader) Unload(name string) error
   func (l *PluginLoader) List() []PluginInfo
   ```

3. **内置插件示例**
   - `BuiltinPlugin`: 封装现有内置工具
   - `APIPlugin`: 封装 API 工具
   - `DataPlugin`: 封装数据工具

4. **Registry 增强**
   - `RegisterPlugin(plugin ToolPlugin)` 批量注册
   - `ToolsByCategory()` 按分类返回工具
   - `ToolCategories()` 返回所有分类

#### 测试用例清单

```
TestPluginLoader_Load
TestPluginLoader_LoadDuplicate
TestPluginLoader_Unload
TestPluginLoader_UnloadNonExistent
TestPluginLoader_List
TestBuiltinPlugin_Init
TestBuiltinPlugin_Tools
TestRegistry_RegisterPlugin
TestRegistry_ToolsByCategory
TestRegistry_ToolCategories
```

---

## 子阶段 3-B: 分布式能力增强（P1）

### Task 34: TCP 传输层增强

**来源**: 已有 tcp_transport.go 基础实现，需增强可靠性
**目标**: 增加连接池、重连机制、消息确认和流量控制

#### 文件清单

| 操作 | 文件路径 | 说明 |
|------|---------|------|
| 修改 | `internal/agent/tcp_transport.go` | 增强实现 |
| 修改 | `internal/agent/tcp_transport_test.go` | 补充测试 |

#### 实现步骤

1. **连接池**
   - 维护到多个 Agent 的 TCP 连接池
   - 自动重连断开的连接
   - 连接健康检查

2. **消息确认机制**
   - 发送方等待 ACK
   - 超时重发
   - 消息 ID 去重

3. **流量控制**
   - 背压机制：接收方缓冲区满时通知发送方暂停
   - 批量发送优化

4. **TLS 支持**
   - 可选 TLS 加密传输
   - 证书验证

#### 测试用例清单

```
TestTCPTransport_ConnectionPool
TestTCPTransport_AutoReconnect
TestTCPTransport_MessageAck
TestTCPTransport_MessageAck_Timeout
TestTCPTransport_Backpressure
TestTCPTransport_BatchSend
TestTCPTransport_TLS
TestTCPTransport_LargeMessage
```

---

### Task 35: Discovery 服务认证

**来源**: 已有 discovery.go 基础实现，需增加安全认证
**目标**: Agent 注册和发现时支持认证和授权

#### 文件清单

| 操作 | 文件路径 | 说明 |
|------|---------|------|
| 修改 | `internal/agent/discovery.go` | 增加认证机制 |
| 修改 | `internal/agent/discovery_test.go` | 补充测试 |

#### 实现步骤

1. **认证接口**
   ```go
   type Authenticator interface {
       Authenticate(token string) (*AgentIdentity, error)
       GenerateToken(identity *AgentIdentity) (string, error)
   }

   type AgentIdentity struct {
       ID       string
       Name     string
       Roles    []string
       Metadata map[string]string
   }
   ```

2. **内置认证器**
   - `TokenAuthenticator`: 基于 HMAC-SHA256 的 Token 认证
   - `NoopAuthenticator`: 无认证（开发模式）

3. **Discovery 集成**
   - Register 时验证 Token
   - Discover 时返回已认证的 Agent 列表
   - 支持角色过滤

#### 测试用例清单

```
TestTokenAuthenticator_Authenticate_Success
TestTokenAuthenticator_Authenticate_InvalidToken
TestTokenAuthenticator_GenerateToken
TestNoopAuthenticator
TestDiscovery_RegisterWithAuth
TestDiscovery_DiscoverWithRoleFilter
TestDiscovery_UnauthorizedRegister
```

---

### Task 36: 分布式集成验证

**来源**: 需要端到端验证分布式 Agent 通信
**目标**: 验证 TCP/HTTP 传输层 + Discovery + 认证的完整链路

#### 文件清单

| 操作 | 文件路径 | 说明 |
|------|---------|------|
| 新增 | `internal/agent/distributed_test.go` | 分布式集成测试 |

#### 实现步骤

1. **测试场景**
   - 两节点 TCP 通信 + Discovery 注册发现
   - HTTP 传输 + Token 认证
   - 多节点广播消息
   - 节点故障恢复

2. **测试工具**
   - `startTestNode(ctx, config)` 启动测试节点
   - `waitForDiscovery(node, agentID)` 等待发现
   - `assertMessageReceived(ch, msg)` 断言消息接收

#### 测试用例清单

```
TestDistributed_TwoNodeTCP
TestDistributed_HTTPWithAuth
TestDistributed_MultiNodeBroadcast
TestDistributed_NodeFailureRecovery
TestDistributed_DiscoveryAndCommunicate
```

---

## 子阶段 3-C: 能力增强（P2）

### Task 37: Ollama Provider 完善

**来源**: 已有 ollama_provider.go 骨架，需完善实现
**目标**: 完整实现 Ollama 本地模型 Provider

#### 文件清单

| 操作 | 文件路径 | 说明 |
|------|---------|------|
| 修改 | `internal/llm/ollama_provider.go` | 完善实现 |
| 新增 | `internal/llm/ollama_test.go` | 测试 |

#### 实现步骤

1. **Complete 方法**
   - POST `/api/chat` 调用 Ollama Chat API
   - 解析 JSON 响应

2. **Stream 方法**
   - POST `/api/chat` with `stream: true`
   - 逐行解析 NDJSON 响应

3. **CallTools 方法**
   - Ollama 支持 function calling 的模型（如 llama3.1+）
   - 降级处理：不支持时返回空

4. **Embeddings 方法**
   - POST `/api/embeddings`
   - 返回嵌入向量

5. **模型列表**
   - GET `/api/tags` 获取可用模型

#### 测试用例清单

```
TestOllamaProvider_Complete_Success
TestOllamaProvider_Complete_Error
TestOllamaProvider_Stream_Basic
TestOllamaProvider_CallTools_Supported
TestOllamaProvider_CallTools_Unsupported
TestOllamaProvider_Embeddings
```

---

### Task 38: Admin 面板增强

**来源**: 已有 admin 基础实现，需增强功能
**目标**: 增加工具管理、工作流监控、实时日志

#### 文件清单

| 操作 | 文件路径 | 说明 |
|------|---------|------|
| 修改 | `internal/admin/handler.go` | 增强端点 |
| 修改 | `internal/admin/handler_test.go` | 补充测试 |

#### 实现步骤

1. **工具管理端点**
   - `GET /api/tools` 列出所有注册工具
   - `GET /api/tools/:name` 查看工具详情
   - `POST /api/tools/:name/execute` 执行工具

2. **工作流监控端点**
   - `GET /api/workflows` 列出工作流
   - `GET /api/workflows/:id` 查看工作流状态
   - `GET /api/workflows/:id/metrics` 工作流指标

3. **实时日志**
   - `GET /api/logs/stream` SSE 实时日志流
   - 订阅事件总线，推送 Agent 生命周期事件

#### 测试用例清单

```
TestAdminHandler_ListTools
TestAdminHandler_GetTool
TestAdminHandler_ExecuteTool
TestAdminHandler_ListWorkflows
TestAdminHandler_GetWorkflow
TestAdminHandler_LogStream
```

---

### Task 39: Memory SummaryStrategy

**来源**: CodeCast `memory.go` 的摘要策略，当前 Summarizer 仅支持 LLM 摘要
**目标**: 支持多种摘要策略（LLM/规则/混合），可配置选择

#### 文件清单

| 操作 | 文件路径 | 说明 |
|------|---------|------|
| 修改 | `internal/memory/summarizer.go` | 增加策略模式 |
| 修改 | `internal/memory/summarizer_test.go` | 补充测试 |

#### 实现步骤

1. **定义摘要策略接口**
   ```go
   type SummaryStrategy interface {
       Summarize(ctx context.Context, content string) (*SummaryResult, error)
   }

   type SummaryResult struct {
       Summary    string
       Topics     []string
       Importance float64
   }
   ```

2. **内置策略**
   - `LLMSummaryStrategy`: 使用 LLM 生成摘要（已有）
   - `RuleSummaryStrategy`: 基于规则的摘要（截取前N字+关键词提取）
   - `HybridSummaryStrategy`: 规则快速摘要 + LLM 异步精炼
   - `NoopSummaryStrategy`: 不生成摘要

3. **Summarizer 重构**
   - `Summarizer` 接受 `SummaryStrategy` 参数
   - 默认使用 `HybridSummaryStrategy`
   - 支持运行时切换策略

#### 测试用例清单

```
TestLLMSummaryStrategy
TestRuleSummaryStrategy_ShortContent
TestRuleSummaryStrategy_LongContent
TestRuleSummaryStrategy_KeywordExtraction
TestHybridSummaryStrategy_QuickSummary
TestHybridSummaryStrategy_RefinedSummary
TestNoopSummaryStrategy
TestSummarizer_WithStrategy
```

---

### Task 40: 工作流可视化

**来源**: 工作流引擎需要可视化调试能力
**目标**: 生成工作流 DAG 的 Mermaid/Graphviz 图形描述

#### 文件清单

| 操作 | 文件路径 | 说明 |
|------|---------|------|
| 新增 | `internal/orchestration/visualize.go` | 可视化生成器 |
| 新增 | `internal/orchestration/visualize_test.go` | 测试 |

#### 实现步骤

1. **Mermaid 生成器**
   ```go
   func (w *Workflow) ToMermaid() string
   ```
   - 遍历节点和转换，生成 Mermaid flowchart
   - 支持条件分支（菱形节点）
   - 支持并行节点（子图）

2. **Graphviz DOT 生成器**
   ```go
   func (w *Workflow) ToDot() string
   ```
   - 生成 DOT 格式图形描述

3. **执行路径高亮**
   ```go
   func (w *Workflow) ToMermaidWithExecution(result *WorkflowResult) string
   ```
   - 高亮实际执行路径
   - 标记失败节点

#### 测试用例清单

```
TestWorkflow_ToMermaid_Linear
TestWorkflow_ToMermaid_Conditional
TestWorkflow_ToMermaid_Parallel
TestWorkflow_ToMermaid_StateMachine
TestWorkflow_ToDot_Linear
```

---

### Task 41: MCP 协议增强

**来源**: 已有 mcp.go 基础实现，需增强协议支持
**目标**: 支持 MCP 协议的完整生命周期和更多传输方式

#### 文件清单

| 操作 | 文件路径 | 说明 |
|------|---------|------|
| 修改 | `internal/tools/mcp.go` | 增强 MCP 实现 |
| 修改 | `internal/tools/mcp_test.go` | 补充测试 |

#### 实现步骤

1. **MCP 生命周期**
   - `initialize` / `initialized` 握手
   - `shutdown` 优雅关闭
   - 心跳保活

2. **SSE 传输**
   - 除 stdio 外支持 SSE 传输
   - HTTP POST 发送请求 + SSE 接收响应

3. **资源订阅**
   - `resources/subscribe` / `resources/unsubscribe`
   - 资源变更通知

4. **Prompt 模板**
   - `prompts/list` / `prompts/get`
   - 支持 MCP 服务器提供的 Prompt 模板

#### 测试用例清单

```
TestMCP_Initialize_Handshake
TestMCP_Shutdown_Graceful
TestMCP_SSETransport
TestMCP_ResourceSubscribe
TestMCP_ResourceNotification
TestMCP_PromptList
```

---

## 执行顺序

```
P0 (必须): Task 32 → 33
P1 (重要): Task 34 → 35 → 36
P2 (增强): Task 37 → 38 → 39 → 40 → 41
```

## 依赖关系

```
Task 32 (评估框架) ← 独立
Task 33 (插件系统) ← 独立
Task 34 (TCP增强) ← Task 28 (HTTP传输层, Phase 2)
Task 35 (Discovery认证) ← Task 34
Task 36 (分布式验证) ← Task 34 + 35
Task 37 (Ollama) ← 独立
Task 38 (Admin增强) ← Task 31 (Admin面板, Phase 2)
Task 39 (SummaryStrategy) ← Task 22 (Summarizer, Phase 2)
Task 40 (工作流可视化) ← 独立
Task 41 (MCP增强) ← 独立
```

## 验收标准

1. 所有新功能有对应测试，测试覆盖率 ≥ 80%
2. `go build ./...` 零错误
3. `go test -count=1 ./...` 全部通过
4. 新增公共 API 通过 `pkg/` 导出
5. 示例代码可编译运行（Demo 模式）

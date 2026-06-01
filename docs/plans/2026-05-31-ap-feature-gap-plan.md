# AP 功能补齐实施方案

> 日期: 2026-05-31 | 状态: 待批准
> 对标: LangGraph / OpenAI Agents SDK / Semantic Kernel / CrewAI / AutoGen

## 0. 总览

本方案覆盖 8 个功能点，分 4 个 Phase 实施。每个 Phase 内的 Task 可并行开发，Phase 间有依赖关系。

```
Phase 1 — 基础增强（结构化输出 + Agent-as-Tool）
Phase 2 — 核心交互（人机协作 + 多模态集成）
Phase 3 — 运维可观测（Token/成本追踪 + 分布式追踪）
Phase 4 — 智能优化（语义缓存 + 上下文压缩）
```

**依赖关系：**
- Phase 1 无外部依赖，可立即开始
- Phase 2 的多模态集成依赖 Phase 1 的结构化输出（Message 扩展）
- Phase 3 无外部依赖，可与 Phase 1/2 并行
- Phase 4 的语义缓存依赖 Phase 3 的 Token 追踪（缓存命中需记录节省量）

**技术约束：**
- Go 1.22+ 标准库，仅允许 `modernc.org/sqlite`
- TDD 强制：先写测试再实现
- 接口优先：所有新功能通过接口解耦
- 并发安全：共享状态用 sync 保护
- 中文注释

---

## Phase 1: 基础增强

### Task 1: 结构化输出（Structured Output）

**目标：** 让 LLM 强制输出符合 JSON Schema 的结构化数据，支持 OpenAI `response_format`、Anthropic `tool_choice`、Gemini `responseMimeType` 三种主流实现方式。

**文件结构：**
```
internal/llm/
├── structured.go          # 结构化输出核心类型与接口
├── structured_test.go     # 单元测试
├── openai_provider.go     # 修改：增加 ResponseFormat 支持
├── anthropic_provider.go  # 修改：增加 tool_choice 强制模式
├── gemini_provider.go     # 修改：增加 responseMimeType 支持
├── types.go               # 修改：CompletionRequest 增加 ResponseSchema
```

**核心设计：**

```go
// internal/llm/structured.go

// ResponseFormat LLM 响应格式控制
type ResponseFormat struct {
    Type       string         `json:"type"`                  // "text" | "json_object" | "json_schema"
    JSONSchema *SchemaDef     `json:"json_schema,omitempty"`  // 当 type=json_schema 时必填
}

// SchemaDef JSON Schema 定义
type SchemaDef struct {
    Name        string         `json:"name"`
    Description string         `json:"description,omitempty"`
    Schema      map[string]any `json:"schema"`               // 标准 JSON Schema 对象
    Strict      bool           `json:"strict,omitempty"`      // 严格模式（OpenAI 专属）
}

// StructuredExtractor 结构化提取器
type StructuredExtractor struct {
    provider Provider
    model    string
}

// Extract 从自然语言输入中提取结构化数据
// prompt 引导 LLM 输出，schema 约束输出格式
func (e *StructuredExtractor) Extract(ctx context.Context, prompt string, schema *SchemaDef) (json.RawMessage, error)

// ExtractInto 提取并反序列化到目标类型
func ExtractInto[T any](e *StructuredExtractor, ctx context.Context, prompt string, schema *SchemaDef) (*T, error)
```

**CompletionRequest 扩展：**

```go
type CompletionRequest struct {
    Messages       []ChatMessage  `json:"messages"`
    Model          string         `json:"model,omitempty"`
    Temperature    *float64       `json:"temperature,omitempty"`
    MaxTokens      int            `json:"max_tokens,omitempty"`
    Stream         bool           `json:"stream,omitempty"`
    ResponseFormat *ResponseFormat `json:"response_format,omitempty"` // 新增
}
```

**Provider 层适配：**
- OpenAI: 映射为 `response_format: {type: "json_schema", json_schema: {...}}`
- Anthropic: 通过 `tool_choice: {type: "tool", name: "structured_output"}` + 单工具注入实现
- Gemini: 映射为 `generationConfig: {responseMimeType: "application/json", responseSchema: {...}}`
- 其他 Provider: 降级为 Prompt 注入模式（在 system prompt 中追加 JSON Schema 要求）

**TDD 步骤：**

- [ ] Step 1: 编写 `TestResponseFormat_Marshal` 验证 ResponseFormat 序列化
- [ ] Step 2: 编写 `TestStructuredExtractor_Extract` 用 MockLLM 验证提取流程
- [ ] Step 3: 编写 `TestExtractInto_Generic` 验证泛型反序列化
- [ ] Step 4: 运行测试确认失败
- [ ] Step 5: 实现 `structured.go` 核心逻辑
- [ ] Step 6: 修改 `openai_provider.go` 增加 ResponseFormat 请求字段映射
- [ ] Step 7: 修改 `anthropic_provider.go` 增加工具注入模式
- [ ] Step 8: 修改 `gemini_provider.go` 增加 responseMimeType 映射
- [ ] Step 9: 运行全部测试确认通过
- [ ] Step 10: 提交 `feat(llm): 结构化输出 Structured Output`

---

### Task 2: Agent-as-Tool 模式

**目标：** 将 Agent 包装为 Tool，使 ReAct Loop 中的 Agent 可以动态调用子 Agent，实现嵌套 Agent 组合。

**文件结构：**
```
internal/agent/
├── agent_tool.go          # AgentTool 适配器
├── agent_tool_test.go     # 单元测试
internal/tools/
├── types.go               # 无修改（已有 Tool 接口）
```

**核心设计：**

```go
// internal/agent/agent_tool.go

// AgentTool 将 Agent 适配为 Tool 接口
// 使一个 Agent 可以在 ReAct Loop 中作为工具调用另一个 Agent
type AgentTool struct {
    agent      Agent
    desc       string
    paramSchema json.RawMessage
}

// AgentToolConfig AgentTool 配置
type AgentToolConfig struct {
    Description  string          // 工具描述，默认使用 Agent 名称
    ParamSchema  json.RawMessage // 输入参数 JSON Schema
    MaxSubTurns  int             // 子 Agent 最大轮数，默认 10
    PassContext  bool            // 是否将父 Agent 上下文传递给子 Agent
}

// NewAgentTool 创建 Agent-as-Tool 适配器
func NewAgentTool(agent Agent, opts ...AgentToolConfig) *AgentTool

// Name 实现 Tool 接口
func (t *AgentTool) Name() string

// Description 实现 Tool 接口
func (t *AgentTool) Description() string

// Parameters 实现 Tool 接口
func (t *AgentTool) Parameters() json.RawMessage

// Execute 实现 Tool 接口 — 调用子 Agent 并返回结果
func (t *AgentTool) Execute(ctx context.Context, args json.RawMessage) (*tools.Result, error)
```

**Execute 流程：**
1. 从 `args` 中解析 `input` 字段作为子 Agent 输入
2. 调用 `agent.Run(ctx, UserMessage(input))`
3. 将 `Response.Content` 包装为 `tools.Result` 返回
4. 如果子 Agent 返回错误，包装为 `tools.NewErrorResult`

**TDD 步骤：**

- [ ] Step 1: 编写 `TestAgentTool_ImplementsTool` 验证接口实现
- [ ] Step 2: 编写 `TestAgentTool_Execute` 用 MockAgent 验证调用流程
- [ ] Step 3: 编写 `TestAgentTool_ExecuteError` 验证错误处理
- [ ] Step 4: 编写 `TestAgentTool_RegistryIntegration` 验证可注册到 Registry
- [ ] Step 5: 运行测试确认失败
- [ ] Step 6: 实现 `agent_tool.go`
- [ ] Step 7: 运行全部测试确认通过
- [ ] Step 8: 提交 `feat(agent): Agent-as-Tool 适配器`

---

## Phase 2: 核心交互

### Task 3: Human-in-the-Loop（人机协作）

**目标：** 在 ReAct Loop 中支持中断点，Agent 遇到不确定决策时暂停等待人类输入，人类确认后恢复执行。

**文件结构：**
```
internal/agent/
├── hitl.go                # 人机协作核心类型
├── hitl_test.go           # 单元测试
├── react_loop.go          # 修改：增加中断点检测和恢复逻辑
├── lifecycle.go           # 修改：增加 WaitingForInput 状态
├── types.go               # 修改：增加 WaitingForInput AgentStatus
```

**核心设计：**

```go
// internal/agent/hitl.go

// InterruptReason 中断原因
type InterruptReason string

const (
    InterruptToolConfirm   InterruptReason = "tool_confirm"    // 工具执行前需确认
    InterruptDecisionPoint InterruptReason = "decision_point"  // 决策点需人类判断
    InterruptBudgetExceed  InterruptReason = "budget_exceed"   // 预算超限需确认
    InterruptCustom        InterruptReason = "custom"          // 自定义中断
)

// InterruptPoint 中断点配置
type InterruptPoint struct {
    Type     InterruptReason // 中断类型
    ToolName string          // 当 type=tool_confirm 时，指定工具名（空=所有工具）
    Message  string          // 中断时展示给人类的消息
}

// InterruptRequest 中断请求（Agent 发出）
type InterruptRequest struct {
    Reason  InterruptReason `json:"reason"`
    Message string          `json:"message"`
    Data    map[string]any  `json:"data,omitempty"`
    Turn    int             `json:"turn"`
}

// HumanResponse 人类响应
type HumanResponse struct {
    Approved bool           `json:"approved"`           // true=继续，false=取消
    Input    string         `json:"input,omitempty"`    // 人类补充的输入
    Modified map[string]any `json:"modified,omitempty"` // 人类修改的参数
}

// HITLConfig 人机协作配置
type HITLConfig struct {
    // InterruptPoints 中断点列表，Agent 在这些点暂停
    InterruptPoints []InterruptPoint

    // HumanInputChan 人类输入通道
    // Agent 暂停后从此通道等待人类响应
    HumanInputChan <-chan *HumanResponse

    // OnInterrupt 中断回调（可选），用于通知外部系统
    OnInterrupt func(req *InterruptRequest)

    // AutoApproveTools 自动批准的工具列表（不触发中断）
    AutoApproveTools []string
}

// HITLManager 人机协作管理器
type HITLManager struct {
    config    HITLConfig
    pending   *InterruptRequest
    responseCh chan *HumanResponse
    mu        sync.RWMutex
}

// ShouldInterrupt 判断当前操作是否需要中断
func (m *HITLManager) ShouldInterrupt(toolName string, reason InterruptReason) bool

// RequestInterrupt 发起中断请求，阻塞等待人类响应
func (m *HITLManager) RequestInterrupt(ctx context.Context, req *InterruptRequest) (*HumanResponse, error)

// Resume 恢复 Agent 执行（外部调用）
func (m *HITLManager) Resume(response *HumanResponse)
```

**AgentStatus 扩展：**

```go
const (
    // ... 现有状态 ...
    StatusWaitingForInput AgentStatus = "waiting_for_input" // 新增：等待人类输入
)
```

**ReAct Loop 集成点：**

1. **工具执行前**：`HookBeforeTool` → 检查 `HITLManager.ShouldInterrupt(toolName, InterruptToolConfirm)`
2. **决策点**：在 `thought.Content` 包含特定标记时触发 `InterruptDecisionPoint`
3. **恢复**：Agent 进入 `StatusWaitingForInput` → 外部调用 `HITLManager.Resume()` → Agent 继续

**TDD 步骤：**

- [ ] Step 1: 编写 `TestHITLManager_ShouldInterrupt` 验证中断判断逻辑
- [ ] Step 2: 编写 `TestHITLManager_RequestInterrupt` 验证中断-恢复流程
- [ ] Step 3: 编写 `TestHITLManager_AutoApprove` 验证自动批准
- [ ] Step 4: 编写 `TestReActLoop_WithToolConfirm` 集成测试：工具确认中断
- [ ] Step 5: 编写 `TestReActLoop_WithHumanReject` 集成测试：人类拒绝后取消
- [ ] Step 6: 运行测试确认失败
- [ ] Step 7: 实现 `hitl.go`
- [ ] Step 8: 修改 `lifecycle.go` 增加 `StatusWaitingForInput` 状态
- [ ] Step 9: 修改 `react_loop.go` 在工具执行前插入中断检测
- [ ] Step 10: 运行全部测试确认通过
- [ ] Step 11: 提交 `feat(agent): Human-in-the-Loop 人机协作`

---

### Task 4: 多模态集成到 ReAct Loop

**目标：** 将已有的多模态 Provider 能力打通到 ReAct Loop，使 Agent 可以接收和产出图片/音频等多模态内容。

**文件结构：**
```
internal/agent/
├── types.go               # 修改：Message 支持 ContentParts
├── multimodal.go          # 新增：多模态消息适配器
├── multimodal_test.go     # 新增：单元测试
├── react_loop.go          # 修改：历史构建时处理 ContentParts
internal/llm/
├── multimodal_types.go    # 修改：增加 FromAgentParts 转换函数
```

**核心设计：**

```go
// internal/agent/types.go — Message 扩展

// ContentPart 消息内容片段（多模态）
type ContentPart struct {
    Type     string `json:"type"`                // "text" | "image_url" | "image_b64" | "audio" | "video"
    Text     string `json:"text,omitempty"`       // Type=text 时的文本
    URL      string `json:"url,omitempty"`        // Type=image_url 时的 URL
    Data     string `json:"data,omitempty"`       // Base64 数据
    MIME     string `json:"mime,omitempty"`       // MIME 类型
    Detail   string `json:"detail,omitempty"`     // 图片细节级别
}

// Message 扩展
type Message struct {
    Role         Role          `json:"role"`
    Content      string        `json:"content"`                   // 纯文本内容（向后兼容）
    ContentParts []ContentPart `json:"content_parts,omitempty"`   // 多模态内容（新增）
    ToolCalls    []ToolCall    `json:"tool_calls,omitempty"`
    Metadata     Metadata      `json:"metadata,omitempty"`
}

// HasMultimodal 判断消息是否包含多模态内容
func (m *Message) HasMultimodal() bool

// TextContent 提取纯文本内容（兼容旧逻辑）
func (m *Message) TextContent() string
```

```go
// internal/agent/multimodal.go

// MultimodalAdapter 多模态消息适配器
// 将 agent.ContentPart 转换为 llm.MultimodalContent
type MultimodalAdapter struct{}

// ToLLMContents 将 agent ContentParts 转换为 llm.MultimodalContent 列表
func (a *MultimodalAdapter) ToLLMContents(parts []ContentPart) []*llm.MultimodalContent

// FromLLMResponse 将 llm 响应转换为 agent Message
func (a *MultimodalAdapter) FromLLMResponse(content string) Message

// UserMultimodalMessage 创建多模态用户消息
func UserMultimodalMessage(parts ...ContentPart) Message

// UserImageMessage 便捷函数：创建图片用户消息
func UserImageMessage(text, imageURL string) Message
```

**ReAct Loop 修改点：**

1. 历史构建时：检查 `msg.HasMultimodal()`，如果为 true 则使用 `MultimodalAdapter` 转换为 `ChatMessageExt` 格式
2. LLM 调用时：如果历史包含多模态，使用 `ProviderExt.CompleteMultimodal()` 代替 `Provider.Complete()`
3. Provider 接口检测：通过 `InfoExt()` 判断 Provider 是否支持多模态

**向后兼容：**
- `Message.Content` 保留，纯文本场景无变化
- `ContentParts` 为空时走原有 `Provider.Complete()` 路径
- `ContentParts` 非空时走 `CompleteMultimodal()` 路径

**TDD 步骤：**

- [ ] Step 1: 编写 `TestContentPart_Marshal` 验证 ContentPart 序列化
- [ ] Step 2: 编写 `TestMessage_HasMultimodal` 验证多模态判断
- [ ] Step 3: 编写 `TestMultimodalAdapter_ToLLMContents` 验证格式转换
- [ ] Step 4: 编写 `TestUserMultimodalMessage` 验证便捷构造
- [ ] Step 5: 编写 `TestReActLoop_MultimodalInput` 集成测试
- [ ] Step 6: 运行测试确认失败
- [ ] Step 7: 修改 `types.go` 增加 ContentParts 字段和方法
- [ ] Step 8: 实现 `multimodal.go` 适配器
- [ ] Step 9: 修改 `react_loop.go` 历史构建逻辑
- [ ] Step 10: 运行全部测试确认通过
- [ ] Step 11: 提交 `feat(agent): 多模态集成到 ReAct Loop`

---

## Phase 3: 运维可观测

### Task 5: Token 计数与成本追踪

**目标：** 聚合 LLM Usage 数据，追踪成本，支持预算控制和成本归因。

**文件结构：**
```
internal/agent/
├── cost_tracker.go        # 成本追踪器
├── cost_tracker_test.go   # 单元测试
├── react_loop.go          # 修改：记录每轮 Usage
internal/metrics/
├── metrics.go             # 修改：增加 Token 追踪方法
internal/llm/
├── pricing.go             # 模型定价表
├── pricing_test.go        # 定价测试
```

**核心设计：**

```go
// internal/llm/pricing.go

// ModelPricing 模型定价
type ModelPricing struct {
    Model              string  `json:"model"`
    Provider           string  `json:"provider"`
    PromptPricePer1M   float64 `json:"prompt_price_per_1m"`   // 输入每百万 Token 价格（USD）
    CompletionPricePer1M float64 `json:"completion_price_per_1m"` // 输出每百万 Token 价格（USD）
}

// DefaultPricingTable 默认定价表
func DefaultPricingTable() map[string]ModelPricing

// EstimateCost 估算单次调用成本
func EstimateCost(model string, usage Usage, table map[string]ModelPricing) float64
```

```go
// internal/agent/cost_tracker.go

// CostRecord 单次成本记录
type CostRecord struct {
    Model           string    `json:"model"`
    PromptTokens    int       `json:"prompt_tokens"`
    CompletionTokens int      `json:"completion_tokens"`
    TotalTokens     int       `json:"total_tokens"`
    CostUSD         float64   `json:"cost_usd"`
    Timestamp       time.Time `json:"timestamp"`
    SessionID       string    `json:"session_id"`
    AgentName       string    `json:"agent_name"`
}

// BudgetConfig 预算配置
type BudgetConfig struct {
    MaxTotalCostUSD   float64 // 总成本上限
    MaxTokensPerCall  int     // 单次调用 Token 上限
    MaxTokensPerSession int   // 单会话 Token 上限
    OnBudgetExceed    func(summary *CostSummary) // 超预算回调
}

// CostSummary 成本汇总
type CostSummary struct {
    TotalCostUSD      float64 `json:"total_cost_usd"`
    TotalPromptTokens int64   `json:"total_prompt_tokens"`
    TotalCompTokens   int64   `json:"total_completion_tokens"`
    TotalTokens       int64   `json:"total_tokens"`
    CallCount         int     `json:"call_count"`
    ByModel           map[string]*ModelCost `json:"by_model"`
}

// ModelCost 单模型成本
type ModelCost struct {
    CostUSD  float64 `json:"cost_usd"`
    Calls    int     `json:"calls"`
    Tokens   int64   `json:"tokens"`
}

// CostTracker 成本追踪器
type CostTracker struct {
    pricing map[string]ModelPricing
    budget  *BudgetConfig
    records []CostRecord
    mu      sync.RWMutex
}

// NewCostTracker 创建成本追踪器
func NewCostTracker(pricing map[string]ModelPricing, budget *BudgetConfig) *CostTracker

// Record 记录一次 LLM 调用的 Usage
func (t *CostTracker) Record(model, sessionID, agentName string, usage llm.Usage) error

// Summary 返回成本汇总
func (t *CostTracker) Summary() *CostSummary

// CheckBudget 检查是否超出预算
func (t *CostTracker) CheckBudget() bool

// Reset 重置追踪器
func (t *CostTracker) Reset()
```

**ReAct Loop 集成：**

1. `ReActConfig` 增加 `CostTracker *CostTracker`
2. 每次 LLM 调用后，将 `Usage` 传递给 `CostTracker.Record()`
3. 每轮开始前检查 `CostTracker.CheckBudget()`，超预算则中断

**MetricsRecorder 扩展：**

```go
type MetricsRecorder interface {
    RecordLLMCall(duration time.Duration, err error)
    RecordToolCall(duration time.Duration, err error)
    RecordTurn(duration time.Duration)
    RecordTokenUsage(model string, promptTokens, completionTokens int) // 新增
    IncActiveAgents()
    DecActiveAgents()
}
```

**TDD 步骤：**

- [ ] Step 1: 编写 `TestModelPricing_EstimateCost` 验证成本估算
- [ ] Step 2: 编写 `TestCostTracker_Record` 验证记录逻辑
- [ ] Step 3: 编写 `TestCostTracker_Summary` 验证汇总计算
- [ ] Step 4: 编写 `TestCostTracker_BudgetExceed` 验证预算控制
- [ ] Step 5: 编写 `TestCostTracker_ConcurrentRecord` 验证并发安全
- [ ] Step 6: 运行测试确认失败
- [ ] Step 7: 实现 `pricing.go`
- [ ] Step 8: 实现 `cost_tracker.go`
- [ ] Step 9: 修改 `react_loop.go` 集成 CostTracker
- [ ] Step 10: 运行全部测试确认通过
- [ ] Step 11: 提交 `feat(agent): Token 计数与成本追踪`

---

### Task 6: 分布式追踪（Observability）

**目标：** 定义 Trace/Span 接口，在 Hooks 中埋点，默认提供日志实现，可选集成 OpenTelemetry。

**文件结构：**
```
internal/agent/
├── trace.go               # 追踪接口与默认实现
├── trace_test.go          # 单元测试
├── hooks.go               # 修改：在关键 Hook 点埋入 Trace
├── react_loop.go          # 修改：创建 RootSpan
```

**核心设计：**

```go
// internal/agent/trace.go

// SpanKind Span 类型
type SpanKind string

const (
    SpanKindInternal SpanKind = "internal"
    SpanKindClient   SpanKind = "client"   // 出站调用（如 LLM）
    SpanKindServer   SpanKind = "server"   // 入站调用
)

// SpanContext Span 上下文，用于跨服务传播
type SpanContext struct {
    TraceID string `json:"trace_id"`
    SpanID  string `json:"span_id"`
}

// Span 追踪 Span 接口
type Span interface {
    // SetName 设置 Span 名称
    SetName(name string)
    // SetAttribute 设置属性
    SetAttribute(key string, value any)
    // SetError 标记错误
    SetError(err error)
    // End 结束 Span
    End()
    // Context 返回 Span 上下文
    Context() SpanContext
}

// Tracer 追踪器接口
type Tracer interface {
    // StartSpan 创建 Span
    StartSpan(ctx context.Context, name string, kind SpanKind, opts ...SpanOption) (context.Context, Span)
    // Extract 从载体中提取 SpanContext
    Extract(carrier map[string]string) SpanContext
    // Inject 将 SpanContext 注入载体
    Inject(ctx context.Context, carrier map[string]string)
}

// SpanOption Span 创建选项
type SpanOption func(*spanConfig)

type spanConfig struct {
    parent SpanContext
    attrs  map[string]any
}

// WithParent 设置父 Span
func WithParent(parent SpanContext) SpanOption

// WithAttributes 设置初始属性
func WithAttributes(attrs map[string]any) SpanOption

// ===== 默认日志实现 =====

// LoggingTracer 基于 slog 的追踪器（零依赖默认实现）
type LoggingTracer struct {
    logger *slog.Logger
}

func NewLoggingTracer(logger *slog.Logger) *LoggingTracer

// NoopTracer 空追踪器（性能敏感场景）
type NoopTracer struct{}

func NewNoopTracer() *NoopTracer
```

**ReAct Loop 埋点位置：**

| 位置 | Span 名称 | 属性 |
|------|-----------|------|
| Run 开始 | `agent.run` | agent_name, session_id |
| LLM 调用 | `agent.llm.call` | model, prompt_tokens |
| LLM 流式 | `agent.llm.stream` | model |
| 工具执行 | `agent.tool.execute` | tool_name, duration |
| RAG 检索 | `agent.rag.search` | query, top_k, result_count |
| Run 结束 | `agent.run` (End) | status, total_turns, duration |

**Hook 集成方式：**

```go
// ReActConfig 扩展
type ReActConfig struct {
    // ... 现有字段 ...
    Tracer Tracer // 新增
}
```

在 `reactLoopEngine` 入口创建 RootSpan，在各 Hook 点创建子 Span。

**TDD 步骤：**

- [ ] Step 1: 编写 `TestLoggingTracer_StartSpan` 验证 Span 创建
- [ ] Step 2: 编写 `TestSpan_SetAttribute` 验证属性设置
- [ ] Step 3: 编写 `TestSpan_SetError` 验证错误标记
- [ ] Step 4: 编写 `TestSpan_End` 验证 Span 结束日志输出
- [ ] Step 5: 编写 `TestNoopTracer_NoOutput` 验证空实现无副作用
- [ ] Step 6: 编写 `TestTracer_InjectExtract` 验证上下文传播
- [ ] Step 7: 运行测试确认失败
- [ ] Step 8: 实现 `trace.go`
- [ ] Step 9: 修改 `react_loop.go` 集成 Tracer 埋点
- [ ] Step 10: 运行全部测试确认通过
- [ ] Step 11: 提交 `feat(agent): 分布式追踪 Tracer 接口与日志实现`

---

## Phase 4: 智能优化

### Task 7: 语义缓存（Semantic Caching）

**目标：** 相似查询命中缓存时直接返回结果，避免重复 LLM 调用，降低成本和延迟。

**文件结构：**
```
internal/llm/
├── cache.go               # 语义缓存接口与实现
├── cache_test.go          # 单元测试
├── resilient.go           # 修改：Provider 层注入缓存
```

**核心设计：**

```go
// internal/llm/cache.go

// CacheEntry 缓存条目
type CacheEntry struct {
    Key       string             `json:"key"`
    Query     string             `json:"query"`
    Response  *CompletionResponse `json:"response"`
    CreatedAt time.Time          `json:"created_at"`
    HitCount  int                `json:"hit_count"`
    Model     string             `json:"model"`
}

// CacheStats 缓存统计
type CacheStats struct {
    TotalQueries  int64   `json:"total_queries"`
    CacheHits     int64   `json:"cache_hits"`
    CacheMisses   int64   `json:"cache_misses"`
    HitRate       float64 `json:"hit_rate"`
    EntryCount    int     `json:"entry_count"`
    TokensSaved   int64   `json:"tokens_saved"`
    CostSavedUSD  float64 `json:"cost_saved_usd"`
}

// LLMCache LLM 缓存接口
type LLMCache interface {
    // Get 查找缓存，similarity 为最低相似度阈值（0-1）
    Get(ctx context.Context, query string, similarity float32) (*CompletionResponse, bool)
    // Set 写入缓存
    Set(ctx context.Context, query string, resp *CompletionResponse) error
    // Stats 返回缓存统计
    Stats() CacheStats
    // Clear 清空缓存
    Clear()
}

// InMemoryCache 内存缓存（基于向量相似度）
type InMemoryCache struct {
    entries  []*CacheEntry
    vectors  [][]float32
    embedder EmbeddingFunc
    mu       sync.RWMutex
    maxSize  int
    minScore float32
}

// EmbeddingFunc 文本向量化函数
type EmbeddingFunc func(ctx context.Context, text string) ([]float32, error)

// NewInMemoryCache 创建内存缓存
// embedder 用于将查询文本转为向量，maxSize 为最大缓存条目数
func NewInMemoryCache(embedder EmbeddingFunc, maxSize int, minScore float32) *InMemoryCache

// CachedProvider 带 LLM 缓存的 Provider 装饰器
type CachedProvider struct {
    inner  Provider
    cache  LLMCache
    minScore float32
}

// NewCachedProvider 创建带缓存的 Provider
func NewCachedProvider(inner Provider, cache LLMCache, minScore float32) *CachedProvider

// Complete 实现 Provider 接口 — 先查缓存，未命中再调用内部 Provider
func (p *CachedProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)
```

**CachedProvider.Complete 流程：**
1. 将 `req.Messages` 最后一条用户消息提取为 query
2. 调用 `cache.Get(ctx, query, p.minScore)` 查找缓存
3. 命中：直接返回缓存响应，记录 `TokensSaved`
4. 未命中：调用 `inner.Complete()`，将结果写入缓存

**TDD 步骤：**

- [ ] Step 1: 编写 `TestInMemoryCache_SetGet` 验证精确匹配缓存
- [ ] Step 2: 编写 `TestInMemoryCache_SemanticMatch` 验证语义相似匹配
- [ ] Step 3: 编写 `TestInMemoryCache_Eviction` 验证 LRU 淘汰
- [ ] Step 4: 编写 `TestCachedProvider_CacheHit` 验证缓存命中不调用 LLM
- [ ] Step 5: 编写 `TestCachedProvider_CacheMiss` 验证缓存未命中正常调用
- [ ] Step 6: 编写 `TestCacheStats` 验证统计计算
- [ ] Step 7: 运行测试确认失败
- [ ] Step 8: 实现 `cache.go`
- [ ] Step 9: 运行全部测试确认通过
- [ ] Step 10: 提交 `feat(llm): 语义缓存 Semantic Cache`

---

### Task 8: 上下文智能压缩

**目标：** 替代简单的尾部截断，使用 LLM 驱动的摘要压缩策略，保留关键上下文的同时控制 Token 数量。

**文件结构：**
```
internal/agent/
├── context_window.go      # 修改：增加 CompressStrategy
├── context_compress.go    # 新增：智能压缩策略
├── context_compress_test.go # 新增：单元测试
```

**核心设计：**

```go
// internal/agent/context_compress.go

// CompressConfig 压缩配置
type CompressConfig struct {
    // MaxTokens 压缩后最大 Token 数（估算）
    MaxTokens int
    // SummaryModel 用于摘要的 LLM Provider
    SummaryModel llm.Provider
    // KeepSystemMessages 是否保留所有系统消息
    KeepSystemMessages bool
    // KeepRecentN 保留最近 N 条消息不压缩
    KeepRecentN int
    // CompressRatio 压缩比例（0.3 = 保留 30% 的 Token）
    CompressRatio float64
}

// CompressStrategy 智能压缩策略
type CompressStrategy struct {
    config  CompressConfig
    logger  *slog.Logger
}

// NewCompressStrategy 创建压缩策略
func NewCompressStrategy(config CompressConfig) *CompressStrategy

// Trim 实现 ContextWindowStrategy 接口
func (s *CompressStrategy) Trim(messages []Message, maxMessages int) []Message

// compressOldMessages 压缩旧消息为摘要
func (s *CompressStrategy) compressOldMessages(ctx context.Context, old []Message) (string, error)

// estimateTokens 估算消息的 Token 数（简单启发式：1 Token ≈ 4 字符）
func estimateTokens(messages []Message) int
```

**Trim 算法：**

```
输入: messages, maxMessages
1. 分离: systemMsgs + recentMsgs + oldMsgs
   - systemMsgs: 所有 Role=System 的消息
   - recentMsgs: 最后 KeepRecentN 条消息
   - oldMsgs: 中间的消息
2. 如果 len(messages) <= maxMessages，直接返回
3. 对 oldMsgs 调用 compressOldMessages() 生成摘要
4. 构造摘要消息: SystemMessage("[对话摘要]\n" + summary)
5. 返回: systemMsgs + [摘要消息] + recentMsgs
```

**compressOldMessages 流程：**
1. 将 oldMsgs 格式化为 "User: xxx\nAssistant: xxx" 文本
2. 构造摘要 Prompt："请将以下对话历史压缩为简洁摘要，保留关键信息、决策和结论：\n\n"
3. 调用 `SummaryModel.Complete()` 生成摘要
4. 如果 SummaryModel 为 nil，降级为简单截断（取 oldMsgs 的首尾各 1 条）

**TDD 步骤：**

- [ ] Step 1: 编写 `TestEstimateTokens` 验证 Token 估算
- [ ] Step 2: 编写 `TestCompressStrategy_TrimShort` 验证短消息不压缩
- [ ] Step 3: 编写 `TestCompressStrategy_TrimWithSummary` 验证 LLM 摘要压缩
- [ ] Step 4: 编写 `TestCompressStrategy_TrimFallback` 验证无 LLM 时降级
- [ ] Step 5: 编写 `TestCompressStrategy_KeepSystem` 验证系统消息保留
- [ ] Step 6: 编写 `TestCompressStrategy_KeepRecent` 验证近期消息保留
- [ ] Step 7: 运行测试确认失败
- [ ] Step 8: 实现 `context_compress.go`
- [ ] Step 9: 运行全部测试确认通过
- [ ] Step 10: 提交 `feat(agent): 上下文智能压缩 CompressStrategy`

---

## 实施时间线

```
Week 1: Phase 1 — Task 1 (结构化输出) + Task 2 (Agent-as-Tool)  [可并行]
Week 2: Phase 2 — Task 3 (人机协作) + Task 4 (多模态集成)        [可并行]
Week 3: Phase 3 — Task 5 (成本追踪) + Task 6 (分布式追踪)        [可与 Phase 2 并行]
Week 4: Phase 4 — Task 7 (语义缓存) + Task 8 (上下文压缩)        [依赖 Phase 3]
```

## 风险与缓解

| 风险 | 缓解措施 |
|------|----------|
| 结构化输出 Provider 适配复杂 | 先实现 OpenAI，其他 Provider 降级为 Prompt 注入 |
| HITL 阻塞 ReAct Loop | 使用 channel 异步等待，不阻塞 goroutine |
| 多模态 Message 向后兼容 | ContentParts 为空时走原路径，零侵入 |
| 语义缓存 Embedding 依赖 | 可选依赖，无 Embedder 时降级为精确匹配 |
| 上下文压缩 LLM 调用成本 | 压缩调用使用最便宜的模型，且只在窗口满时触发 |
| Tracer 接口过度设计 | 先实现 LoggingTracer，OTel 留为扩展点 |

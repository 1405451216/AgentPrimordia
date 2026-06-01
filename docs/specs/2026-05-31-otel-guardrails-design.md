# OpenTelemetry 桥接适配层 + Guardrails 安全护栏 设计规格

> 版本: v0.1 | 日期: 2026-05-31 | 状态: 已批准

## 1. 背景与目标

### 1.1 问题陈述

AgentPrimordia (AP) 在功能覆盖度上已对齐第一梯队 Agent 开发框架，但在两个维度存在差距：

- **可观测性深度**：AP 有自建的 Span/Tracer 体系和 Prometheus 格式 Metrics，但无法与 OpenTelemetry 生态（Jaeger/Zipkin/Grafana Tempo/OTel Collector）对接，缺少 W3C TraceContext 跨服务传播能力
- **内容安全护栏**：AP 有沙箱（Sandbox/ACL）控制 Agent 的文件和命令权限，但缺少 LLM 输入输出的内容级安全控制（PII 泄露、敏感词、Prompt 注入等）

### 1.2 设计目标

| 目标 | 描述 |
|------|------|
| **OTel 生态对接** | 通过桥接适配层，将 AP 的 Trace/Metrics 数据导出到 OTel 后端 |
| **零依赖备选** | 提供 OTLP HTTP 导出器，不引入 OTel SDK 即可对接 OTel Collector |
| **W3C 传播** | SpanContext 支持 TraceFlags/TraceState，实现跨服务 Trace 传播 |
| **内容安全** | PII 检测、敏感词过滤、Prompt 注入检测、话题约束、输出安全检查 |
| **脱敏策略** | 支持拒绝/脱敏放行/标记审计三种处理方式 |
| **零侵入** | 默认行为不变，OTel 和 Guardrails 均为可选增强 |

### 1.3 范围边界

**包含**:
- OTel SDK 桥接适配器（构建标签隔离）
- OTLP HTTP/JSON 导出器（零依赖）
- SpanContext W3C TraceContext 扩展
- Tracer 接口化升级
- Guardrail 接口 + 规则引擎
- 5 类内置规则：PII / 敏感词 / Prompt 注入 / 话题约束 / 输出安全
- 脱敏处理器（Masker）
- HookPhase 执行阶段保障

**不含** (留待后续):
- OTLP gRPC 导出
- OTel Logs 桥接
- Guardrails 的 LLM 辅助检测（用 LLM 判断是否为注入）
- 对话流程控制（NeMo Guardrails 风格的话题转换图）
- 自定义 Guardrail 规则的热重载

---

## 2. 架构设计

### 2.1 模块划分

```
internal/
├── otel/                    # OpenTelemetry 桥接模块
│   ├── provider.go          # TelemetryProvider 统一入口
│   ├── provider_test.go
│   ├── bridge.go            # +build otel — OTel SDK 桥接
│   ├── bridge_nootel.go     # +build !otel — Noop 降级
│   ├── bridge_test.go
│   ├── metrics.go           # +build otel — OTel Metrics 桥接
│   ├── metrics_nootel.go    # +build !otel — Noop 降级
│   ├── metrics_test.go
│   ├── otlp_exporter.go     # OTLP HTTP 导出（零依赖，始终可用）
│   ├── otlp_exporter_test.go
│   └── doc.go
│
├── guardrails/              # 安全护栏模块
│   ├── guardrail.go         # Guardrail 接口 + GuardrailResult
│   ├── guardrail_test.go
│   ├── engine.go            # GuardrailEngine 规则引擎
│   ├── engine_test.go
│   ├── pii.go               # PII 检测规则
│   ├── pii_test.go
│   ├── sensitive.go         # 敏感词过滤规则
│   ├── sensitive_test.go
│   ├── injection.go         # Prompt 注入检测规则
│   ├── injection_test.go
│   ├── topic.go             # 话题约束规则
│   ├── topic_test.go
│   ├── output.go            # 输出安全检查规则
│   ├── output_test.go
│   ├── masker.go            # 脱敏处理器
│   ├── masker_test.go
│   └── doc.go
```

### 2.2 依赖关系

```
                    ┌──────────────┐
                    │   pkg/otel   │  公共 API re-export
                    └──────┬───────┘
                           │
                    ┌──────┴───────┐
                    │ internal/otel│  桥接适配层
                    └──────┬───────┘
                           │ 依赖
              ┌────────────┼────────────┐
              │            │            │
     ┌────────┴──┐  ┌──────┴─────┐  ┌──┴──────────┐
     │agent/trace│  │metrics/    │  │go.opentelemetry│
     │ (Span等)  │  │(AgentMetrics)│  │.io/otel (可选) │
     └───────────┘  └────────────┘  └───────────────┘

                    ┌──────────────────┐
                    │  pkg/guardrails  │  公共 API re-export
                    └────────┬─────────┘
                             │
                    ┌────────┴─────────┐
                    │ internal/guardrails│  规则引擎
                    └────────┬─────────┘
                             │ 依赖
              ┌──────────────┼──────────────┐
              │              │              │
     ┌────────┴──────┐ ┌────┴─────┐ ┌──────┴──────┐
     │ agent/hooks   │ │security/ │ │  内置规则    │
     │ (HookPhase)   │ │(ACL等)   │ │(PII/敏感词)  │
     └───────────────┘ └──────────┘ └─────────────┘
```

---

## 3. OpenTelemetry 桥接适配层

### 3.1 构建标签隔离

OTel SDK 是可选依赖，通过 Go 构建标签隔离：

| 文件 | 构建标签 | 说明 |
|------|---------|------|
| `bridge.go` | `//go:build otel` | OTel SDK 桥接实现 |
| `bridge_nootel.go` | `//go:build !otel` | Noop 降级，默认编译 |
| `metrics.go` | `//go:build otel` | OTel Metrics 桥接 |
| `metrics_nootel.go` | `//go:build !otel` | Noop 降级 |
| `otlp_exporter.go` | 无 | 纯标准库，始终可用 |
| `provider.go` | 无 | 统一入口，条件调用 |

编译方式：
- `go build` → 零外部依赖，OTLP HTTP 可用
- `go build -tags otel` → 引入 OTel SDK，完整桥接

### 3.2 Tracer 接口化

将 AP 的 `LoggingTracer` 从具体类型升级为接口，支持多种实现替换：

```go
// Tracer 追踪器接口
type Tracer interface {
    Start(name string, kind SpanKind, opts ...SpanOption) Span
}

// TracerDebug 调试扩展接口
type TracerDebug interface {
    Tracer
    Reset()
    String() string
}
```

实现关系：
- `LoggingTracer` → 实现 `Tracer` + `TracerDebug`（现有行为不变）
- `OTelBridgeTracer` → 实现 `Tracer`，内部委托给 OTel SDK
- `NoopTracer` → 实现 `Tracer`，零开销

ReAct Loop 中的 `Tracer` 字段类型从 `*LoggingTracer` 改为 `Tracer` 接口。需要调试输出时通过类型断言访问 `TracerDebug`。

### 3.3 SpanContext W3C 扩展

```go
type SpanContext struct {
    TraceID    string            `json:"trace_id"`
    SpanID     string            `json:"span_id"`
    TraceFlags byte              `json:"trace_flags"`  // 0x01 = sampled
    TraceState map[string]string `json:"trace_state,omitempty"` // W3C TraceState
    Remote     bool              `json:"remote"`       // 是否来自远程服务
}
```

新增方法：
- `ToW3CTraceParent() string` — 生成 `traceparent` Header 值
- `FromW3CTraceParent(s string) (SpanContext, error)` — 解析 `traceparent` Header
- `WithTraceState(key, value string) SpanContext` — 追加 TraceState 键值

### 3.4 OTel SDK 桥接（bridge.go，+build otel）

```go
// OTelBridgeTracer 将 AP 的 Trace 操作桥接到 OTel SDK
type OTelBridgeTracer struct {
    provider trace.TracerProvider
    tracer   trace.Tracer
    logger   *slog.Logger
}

func NewOTelBridgeTracer(provider trace.TracerProvider, name string) *OTelBridgeTracer

// OTelBridgeSpan 同时实现 AP Span 接口，内部委托 OTel SDK Span
type OTelBridgeSpan struct {
    otelSpan trace.Span
    context  SpanContext
    ended    bool
    mu       sync.RWMutex
}
```

桥接映射：

| AP Span 操作 | OTel SDK 操作 |
|-------------|--------------|
| `SetName(name)` | `span.SetName(name)` |
| `SetAttribute(k, v)` | `span.SetAttributes(attribute.Any(k, v))` |
| `SetStatus(OK, desc)` | `span.SetStatus(codes.Ok, desc)` |
| `SetStatus(Error, desc)` | `span.SetStatus(codes.Error, desc)` |
| `SpanContext()` | 从 `span.SpanContext()` 转换为 AP `SpanContext` |
| `End()` | `span.End()` |

### 3.5 OTLP HTTP 导出器（otlp_exporter.go，零依赖）

纯标准库实现 OTLP/HTTP JSON 协议，直接发送到 OTel Collector：

```go
// OTLPExporter 通过 HTTP/JSON 发送 OTLP 数据到 OTel Collector
type OTLPExporter struct {
    endpoint    string       // 默认 http://localhost:4318
    headers     map[string]string
    httpClient  *http.Client
    batchSize   int          // 默认 512
    flushInterval time.Duration // 默认 5s
    // 内部缓冲和批量发送
}

// OTLPExporter 实现 TelemetryExporter 接口
// 同时支持 Trace 和 Metrics 数据导出
```

导出路径：
- Traces → `POST /v1/traces`（OTLP/JSON）
- Metrics → `POST /v1/metrics`（OTLP/JSON）

数据转换：
- AP `LoggingSpan` → OTLP `Span` JSON
- AP `MetricsSnapshot` → OTLP `Metric` JSON

### 3.6 Metrics 桥接

两种模式：

**快照导出模式**（默认）：
- 定期调用 `AgentMetrics.Snapshot()` 转换为 OTel Metrics 数据
- 复用现有 `MetricsExporter` 的定时机制

**实时双写模式**（+build otel）：
- `OTelMetricsBridge` 实现 `MetricsRecorder` 接口
- 每次 `RecordLLMCall`/`RecordToolCall` 同时写入 OTel Meter
- 更精确但引入 OTel SDK 依赖

### 3.7 TelemetryProvider 统一入口

```go
// TelemetryProvider 统一管理 Trace + Metrics 配置
type TelemetryProvider struct {
    tracer   Tracer
    exporter TelemetryExporter
    config   TelemetryConfig
}

type TelemetryConfig struct {
    // Trace 配置
    TraceEnabled   bool
    TraceEndpoint  string   // OTel Collector 地址
    TraceServiceName string // 服务名，默认 "agentprimordia"

    // Metrics 配置
    MetricsEnabled   bool
    MetricsEndpoint  string
    MetricsInterval  time.Duration // 导出间隔，默认 15s

    // 模式选择
    UseOTelSDK  bool   // true=桥接 OTel SDK, false=OTLP HTTP
    OTelHeaders map[string]string // 认证 Header
}

// NewTelemetryProvider 创建遥测提供者
func NewTelemetryProvider(config TelemetryConfig) (*TelemetryProvider, error)

// Tracer 返回配置好的 Tracer
func (p *TelemetryProvider) Tracer() Tracer

// Exporter 返回配置好的 TelemetryExporter
func (p *TelemetryProvider) Exporter() TelemetryExporter

// Shutdown 优雅关闭
func (p *TelemetryProvider) Shutdown(ctx context.Context) error
```

### 3.8 pkg/otel 公共 API

```go
// pkg/otel.go
type TelemetryProvider = otel.TelemetryProvider
type TelemetryConfig = otel.TelemetryConfig
var NewTelemetryProvider = otel.NewTelemetryProvider
```

---

## 4. Guardrails 安全护栏

### 4.1 核心接口

```go
// GuardrailAction 护栏动作
type GuardrailAction string

const (
    ActionAllow   GuardrailAction = "allow"   // 放行
    ActionMask    GuardrailAction = "mask"    // 脱敏后放行
    ActionReject  GuardrailAction = "reject"  // 拒绝
    ActionFlag    GuardrailAction = "flag"    // 标记但放行（审计）
)

// GuardrailResult 护栏检查结果
type GuardrailResult struct {
    Action     GuardrailAction
    MaskedText string    // Action=Mask 时的脱敏文本
    Reason     string    // 触发原因说明
    Confidence float64   // 0.0-1.0 置信度
    RuleName   string    // 触发的规则名
    Details    map[string]any // 额外信息（如匹配位置）
}

// Guardrail 护栏规则接口
type Guardrail interface {
    // Name 规则名称
    Name() string
    // CheckInput 检查输入文本（LLM 调用前）
    CheckInput(ctx context.Context, text string) (*GuardrailResult, error)
    // CheckOutput 检查输出文本（LLM 调用后）
    CheckOutput(ctx context.Context, text string) (*GuardrailResult, error)
}
```

### 4.2 GuardrailEngine 规则引擎

```go
// GuardrailEngine 护栏规则引擎
type GuardrailEngine struct {
    inputRules  []Guardrail
    outputRules []Guardrail
    config      GuardrailConfig
    mu          sync.RWMutex
}

type GuardrailConfig struct {
    // OnInputViolation 输入违规时的默认动作（可被规则覆盖）
    OnInputViolation GuardrailAction
    // OnOutputViolation 输出违规时的默认动作
    OnOutputViolation GuardrailAction
    // ConfidenceThreshold 置信度阈值，低于此值不触发
    ConfidenceThreshold float64 // 默认 0.7
    // AuditLogger 审计日志记录器
    AuditLogger AuditLogger
}

// AuditLogger 审计日志接口
type AuditLogger interface {
    LogViolation(ctx context.Context, result *GuardrailResult, direction string)
}

// NewGuardrailEngine 创建规则引擎
func NewGuardrailEngine(config GuardrailConfig) *GuardrailEngine

// AddInputRule 添加输入检查规则
func (e *GuardrailEngine) AddInputRule(rule Guardrail)

// AddOutputRule 添加输出检查规则
func (e *GuardrailEngine) AddOutputRule(rule Guardrail)

// CheckInput 执行所有输入规则
func (e *GuardrailEngine) CheckInput(ctx context.Context, text string) (*GuardrailResult, error)

// CheckOutput 执行所有输出规则
func (e *GuardrailEngine) CheckOutput(ctx context.Context, text string) (*GuardrailResult, error)
```

执行逻辑：
- 规则按添加顺序依次执行
- 任何规则返回 `ActionReject` → 立即中断，返回拒绝结果
- 规则返回 `ActionMask` → 替换文本，继续执行后续规则
- 规则返回 `ActionFlag` → 记录审计日志，继续执行
- 所有规则通过 → 返回 `ActionAllow`

### 4.3 HookPhase 执行阶段

扩展 `HookManager` 支持 Phase 概念：

```go
type HookPhase int

const (
    PhaseValidation   HookPhase = iota // 护栏阶段：Guardrails 固定在此
    PhasePreProcessing                 // 预处理：日志、指标
    PhaseExecution                     // 执行：业务逻辑
    PhasePostProcessing                // 后处理：通知、缓存
)
```

修改 `Hook.Register` 方法：

```go
// RegisterInPhase 在指定阶段注册钩子
func (m *HookManager) RegisterInPhase(phase HookPhase, point HookPoint, fn HookFunc)

// Register 保持向后兼容，默认注册到 PhaseExecution
func (m *HookManager) Register(point HookPoint, fn HookFunc)
```

`Fire` 执行顺序：PhaseValidation → PhasePreProcessing → PhaseExecution → PhasePostProcessing。PhaseValidation 中任何 Hook 返回错误即中断。

### 4.4 Guardrails 与 Hook 的集成

```go
// GuardrailHook 将 GuardrailEngine 注册为 Hook
type GuardrailHook struct {
    engine *GuardrailEngine
}

// NewGuardrailHook 创建护栏 Hook
func NewGuardrailHook(engine *GuardrailEngine) *GuardrailHook

// Register 注册到 HookManager
func (h *GuardrailHook) Register(hooks *HookManager) {
    // 输入护栏：在 LLM 调用前检查
    hooks.RegisterInPhase(PhaseValidation, HookBeforeLLM, h.checkInput)
    // 输出护栏：在 LLM 调用后检查
    hooks.RegisterInPhase(PhaseValidation, HookAfterLLM, h.checkOutput)
}

func (h *GuardrailHook) checkInput(ctx context.Context, hctx *HookContext) error {
    if hctx.Message == nil {
        return nil
    }
    result, err := h.engine.CheckInput(ctx, hctx.Message.Content)
    if err != nil {
        return err
    }
    switch result.Action {
    case ActionReject:
        return fmt.Errorf("输入被护栏拒绝: %s (规则: %s)", result.Reason, result.RuleName)
    case ActionMask:
        hctx.Message.Content = result.MaskedText
    case ActionFlag:
        // 审计日志已由引擎记录
    }
    return nil
}
```

### 4.5 内置规则

#### 4.5.1 PII 检测（pii.go）

检测类型及默认策略：

| PII 类型 | 正则模式 | 默认动作 | 默认置信度 |
|---------|---------|---------|-----------|
| 手机号 | `1[3-9]\d{9}` | Mask | 0.9 |
| 身份证号 | `[1-9]\d{5}(19|20)\d{2}[01]\d[0123]\d\d{3}[\dXx]` | Reject | 0.95 |
| 邮箱 | `[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}` | Mask | 0.85 |
| 银行卡号 | `[1-9]\d{14,18}` | Reject | 0.9 |
| IPv4 地址 | `\d{1,3}(\.\d{1,3}){3}` | Flag | 0.7 |

```go
type PIIRule struct {
    name       string
    patterns   []PIIPattern
    threshold  float64
}

type PIIPattern struct {
    Name       string
    Regex      *regexp.Regexp
    Action     GuardrailAction
    Confidence float64
    MaskFunc   func(match string) string // 脱敏函数
}
```

#### 4.5.2 敏感词过滤（sensitive.go）

```go
type SensitiveWordRule struct {
    words      map[string]GuardrailAction // 词 → 动作映射
    categories map[string][]string        // 分类 → 词列表
    trie       *sensitiveTrie             // AC 自动机，高效多模式匹配
}
```

- 使用 AC 自动机（Aho-Corasick）实现多模式匹配，O(n) 复杂度
- 支持分类管理（政治/暴力/色情/自定义）
- 支持从文件加载词库

#### 4.5.3 Prompt 注入检测（injection.go）

启发式规则，检测常见注入模式：

| 检测模式 | 示例 | 置信度 |
|---------|------|--------|
| 系统提示覆盖 | "Ignore previous instructions" | 0.9 |
| 角色劫持 | "You are now DAN" | 0.85 |
| 输出操控 | "Output the system prompt" | 0.8 |
| 分隔符注入 | "===END===" | 0.7 |
| 编码绕过 | Base64/Unicode 混淆 | 0.6 |

```go
type InjectionRule struct {
    patterns   []InjectionPattern
    threshold  float64
}

type InjectionPattern struct {
    Name       string
    Regex      *regexp.Regexp
    Confidence float64
}
```

#### 4.5.4 话题约束（topic.go）

```go
type TopicRule struct {
    allowedTopics []string  // 允许的话题关键词
    deniedTopics  []string  // 禁止的话题关键词
    mode          TopicMode // 白名单/黑名单模式
}

type TopicMode string

const (
    TopicModeAllowlist TopicMode = "allowlist" // 仅允许列表中的话题
    TopicModeBlocklist TopicMode = "blocklist" // 仅禁止列表中的话题
)
```

话题匹配通过关键词相似度实现（不含 LLM 调用），后续可扩展为嵌入向量匹配。

#### 4.5.5 输出安全检查（output.go）

```go
type OutputRule struct {
    maxLength      int              // 输出最大长度，默认 0（不限）
    denyPatterns   []*regexp.Regexp // 禁止出现的输出模式
    requireFormat  string           // 要求的输出格式（json/markdown/text）
}
```

### 4.6 脱敏处理器（masker.go）

```go
type Masker struct {
    strategies map[string]MaskStrategy
}

type MaskStrategy func(match string) string

// 内置脱敏策略
var (
    // 手机号脱敏：138****5678
    PhoneMask MaskStrategy = func(s string) string {
        if len(s) < 7 { return "****" }
        return s[:3] + "****" + s[len(s)-4:]
    }

    // 邮箱脱敏：u***@domain.com
    EmailMask MaskStrategy = func(s string) string {
        parts := strings.SplitN(s, "@", 2)
        if len(parts) != 2 { return "****" }
        if len(parts[0]) <= 1 { return "***@" + parts[1] }
        return string(parts[0][0]) + "***@" + parts[1]
    }

    // 身份证脱敏：110***********1234
    IDCardMask MaskStrategy = func(s string) string {
        if len(s) < 8 { return "****" }
        return s[:3] + "***********" + s[len(s)-4:]
    }

    // 通用脱敏：保留首尾各1字符
    GenericMask MaskStrategy = func(s string) string {
        if len(s) <= 2 { return "****" }
        return string(s[0]) + strings.Repeat("*", len(s)-2) + string(s[len(s)-1])
    }
)
```

### 4.7 pkg/guardrails 公共 API

```go
// pkg/guardrails.go
type Guardrail = guardrails.Guardrail
type GuardrailResult = guardrails.GuardrailResult
type GuardrailAction = guardrails.GuardrailAction
type GuardrailEngine = guardrails.GuardrailEngine
type GuardrailConfig = guardrails.GuardrailConfig
type GuardrailHook = guardrails.GuardrailHook
type PIIRule = guardrails.PIIRule
type SensitiveWordRule = guardrails.SensitiveWordRule
type InjectionRule = guardrails.InjectionRule
type TopicRule = guardrails.TopicRule
type OutputRule = guardrails.OutputRule
type Masker = guardrails.Masker

const (
    ActionAllow  = guardrails.ActionAllow
    ActionMask   = guardrails.ActionMask
    ActionReject = guardrails.ActionReject
    ActionFlag   = guardrails.ActionFlag
)

var (
    NewGuardrailEngine = guardrails.NewGuardrailEngine
    NewGuardrailHook   = guardrails.NewGuardrailHook
    NewPIIRule         = guardrails.NewPIIRule
    NewSensitiveWordRule = guardrails.NewSensitiveWordRule
    NewInjectionRule   = guardrails.NewInjectionRule
    NewTopicRule       = guardrails.NewTopicRule
    NewOutputRule      = guardrails.NewOutputRule
    NewMasker          = guardrails.NewMasker
)
```

---

## 5. 使用示例

### 5.1 OpenTelemetry 使用

```go
// 方式一：OTLP HTTP 导出（零依赖）
provider, _ := ap.NewTelemetryProvider(ap.TelemetryConfig{
    TraceEnabled:    true,
    TraceEndpoint:   "http://localhost:4318",
    MetricsEnabled:  true,
    MetricsEndpoint: "http://localhost:4318",
    UseOTelSDK:      false,
})
defer provider.Shutdown(ctx)

// 方式二：OTel SDK 桥接（需 go build -tags otel）
provider, _ := ap.NewTelemetryProvider(ap.TelemetryConfig{
    TraceEnabled:    true,
    TraceEndpoint:   "http://localhost:4318",
    UseOTelSDK:      true,
    TraceServiceName: "my-agent-service",
})

// 在 Agent 配置中使用
agent := ap.NewAgent(ap.WithTracer(provider.Tracer()))
```

### 5.2 Guardrails 使用

```go
// 创建规则引擎
engine := ap.NewGuardrailEngine(ap.GuardrailConfig{
    OnInputViolation:    ap.ActionReject,
    OnOutputViolation:   ap.ActionMask,
    ConfidenceThreshold: 0.7,
})

// 添加规则
engine.AddInputRule(ap.NewPIIRule(ap.PIIDefaultConfig()))
engine.AddInputRule(ap.NewInjectionRule(ap.InjectionDefaultConfig()))
engine.AddInputRule(ap.NewSensitiveWordRule().WithCategory("politics"))
engine.AddInputRule(ap.NewTopicRule().WithBlocklist("赌博", "毒品"))
engine.AddOutputRule(ap.NewPIIRule(ap.PIIDefaultConfig()))
engine.AddOutputRule(ap.NewOutputRule().WithMaxLength(10000))

// 注册为 Hook
guardrailHook := ap.NewGuardrailHook(engine)
guardrailHook.Register(hookManager)

// Agent 运行时自动执行护栏检查
// - HookBeforeLLM → 输入检查
// - HookAfterLLM → 输出检查
```

---

## 6. 测试策略

### 6.1 OTel 模块测试

| 测试文件 | 覆盖内容 |
|---------|---------|
| `bridge_test.go` | OTelBridgeTracer/OTelBridgeSpan 的桥接映射正确性 |
| `otlp_exporter_test.go` | OTLP JSON 编码正确性、HTTP 发送、批量缓冲、重试 |
| `metrics_test.go` | MetricsSnapshot → OTLP Metric 转换正确性 |
| `provider_test.go` | TelemetryProvider 配置组合、生命周期管理 |

OTLP Exporter 测试使用 `httptest.Server` 模拟 OTel Collector，不需要真实网络。

### 6.2 Guardrails 模块测试

| 测试文件 | 覆盖内容 |
|---------|---------|
| `guardrail_test.go` | GuardrailResult 各 Action 的行为正确性 |
| `engine_test.go` | 规则执行顺序、中断逻辑、Mask 链式替换、Flag 审计 |
| `pii_test.go` | 各 PII 类型的正则匹配、脱敏输出、置信度阈值 |
| `sensitive_test.go` | AC 自动机匹配、分类管理、词库加载 |
| `injection_test.go` | 各注入模式的检测、误报率控制 |
| `topic_test.go` | 白名单/黑名单模式、关键词匹配 |
| `output_test.go` | 长度限制、格式检查、禁止模式 |
| `masker_test.go` | 各内置脱敏策略的输出正确性 |

---

## 7. 与现有模块的变更

### 7.1 agent/trace.go 变更

- `SpanContext` 增加 `TraceFlags`/`TraceState`/`Remote` 字段
- 新增 `ToW3CTraceParent()`/`FromW3CTraceParent()` 方法
- 新增 `Tracer` 接口和 `TracerDebug` 扩展接口
- `LoggingTracer` 保持实现 `Tracer` + `TracerDebug`

### 7.2 agent/hooks.go 变更

- 新增 `HookPhase` 类型和常量
- `Hook` 结构增加 `Phase HookPhase` 字段
- `HookManager.RegisterInPhase()` 方法
- `HookManager.Fire()` 按 Phase 顺序执行
- 现有 `Register()` 调用默认注册到 `PhaseExecution`，向后兼容

### 7.3 agent/react_loop.go 变更

- `ReActConfig.Tracer` 字段类型从 `*LoggingTracer` 改为 `Tracer` 接口
- 调试输出通过类型断言 `tracer.(TracerDebug)` 访问

**迁移策略**（向后兼容）：
- `ReActConfig.Tracer` 字段为 `Tracer` 接口类型，`nil` 值等价于 `NoopTracer`
- 现有代码中 `NewLoggingTracer()` 返回值自动满足 `Tracer` 接口，无需修改调用方
- 新增 `WithTracer(tracer Tracer) ReActOption` 选项函数
- `ReActConfig` 中 `Tracer` 为 nil 时，内部自动创建 `LoggingTracer`，行为与当前完全一致

### 7.4 pkg/ 变更

- 新增 `pkg/otel.go`：re-export OTel 公共类型
- 新增 `pkg/guardrails.go`：re-export Guardrails 公共类型

### 7.5 不变的模块

- `internal/metrics/` — 不修改，OTel 桥接层通过 `TelemetryExporter` 接口适配
- `internal/security/` — 不修改，Guardrails 是内容级安全，与 ACL/Sandbox 互补
- `internal/llm/` — 不修改，Guardrails 通过 Hook 拦截，不侵入 LLM 调用链

# OTel 桥接适配层 + Guardrails 安全护栏 实施计划

> 日期: 2026-05-31 | 状态: 待执行 | 设计文档: `docs/specs/2026-05-31-otel-guardrails-design.md`

**Goal:** 为 AgentPrimordia 添加 OpenTelemetry 可观测性桥接和内容安全护栏，补齐与第一梯队框架的差距。

**Architecture:** OTel 模块通过构建标签隔离（`//go:build otel`），默认零依赖提供 OTLP HTTP 导出，可选引入 OTel SDK 做完整桥接。Guardrails 模块通过 HookPhase 机制集成到 Hook 体系，在 Validation 阶段强制执行，支持拒绝/脱敏/标记三种动作。

**Tech Stack:** Go 1.22+ 标准库；可选 `go.opentelemetry.io/otel`（构建标签隔离）

---

## 总览

| Task | 内容 | 依赖 | 预估时间 |
|------|------|------|---------|
| 1 | SpanContext W3C 扩展 | 无 | 1h |
| 2 | Tracer 接口化 | Task 1 | 1.5h |
| 3 | HookPhase 执行阶段 | 无 | 2h |
| 4 | OTLP HTTP 导出器 | 无 | 2h |
| 5 | OTel SDK 桥接（构建标签隔离） | Task 2, 4 | 2h |
| 6 | TelemetryProvider 统一入口 | Task 4, 5 | 1.5h |
| 7 | Guardrail 核心接口和引擎 | Task 3 | 2h |
| 8 | PII 检测规则 | Task 7 | 1.5h |
| 9 | 敏感词过滤规则（Trie） | Task 7 | 2h |
| 10 | Prompt 注入检测规则 | Task 7 | 1.5h |
| 11 | 话题约束规则 | Task 7 | 1h |
| 12 | 输出安全检查规则 | Task 7 | 1h |
| 13 | 脱敏处理器 | Task 8, 9 | 1h |
| 14 | GuardrailHook 集成 | Task 3, 7 | 1.5h |
| 15 | pkg/ 公共 API re-export | Task 6, 14 | 0.5h |
| 16 | 集成测试 + 全量回归 | 全部 | 2h |

**总计预估: ~21.5h**

---

## 文件结构

### 新建文件

| 文件 | 职责 |
|------|------|
| `internal/agent/tracer.go` | Tracer 接口 + TracerDebug 扩展接口 + NoopTracer |
| `internal/agent/tracer_test.go` | Tracer 接口测试 |
| `internal/otel/doc.go` | 包文档 |
| `internal/otel/otlp_exporter.go` | OTLP HTTP/JSON 导出器（零依赖） |
| `internal/otel/otlp_exporter_test.go` | OTLP 编码和发送测试 |
| `internal/otel/bridge.go` | `//go:build otel` — OTelBridgeTracer/Span |
| `internal/otel/bridge_nootel.go` | `//go:build !otel` — Noop 降级 |
| `internal/otel/bridge_test.go` | 桥接映射正确性测试 |
| `internal/otel/metrics.go` | `//go:build otel` — OTel Metrics 桥接 |
| `internal/otel/metrics_nootel.go` | `//go:build !otel` — Noop 降级 |
| `internal/otel/provider.go` | TelemetryProvider 统一入口 + TelemetryConfig |
| `internal/otel/provider_test.go` | Provider 配置组合和生命周期测试 |
| `internal/guardrails/doc.go` | 包文档 |
| `internal/guardrails/guardrail.go` | Guardrail 接口 + GuardrailResult + GuardrailAction |
| `internal/guardrails/guardrail_test.go` | 接口行为测试 |
| `internal/guardrails/engine.go` | GuardrailEngine 规则引擎 |
| `internal/guardrails/engine_test.go` | 引擎执行顺序和中断逻辑测试 |
| `internal/guardrails/pii.go` | PII 检测规则 |
| `internal/guardrails/pii_test.go` | PII 检测和脱敏测试 |
| `internal/guardrails/sensitive.go` | 敏感词过滤规则（Trie 多模式匹配） |
| `internal/guardrails/sensitive_test.go` | Trie 匹配测试 |
| `internal/guardrails/injection.go` | Prompt 注入检测规则 |
| `internal/guardrails/injection_test.go` | 注入检测测试 |
| `internal/guardrails/topic.go` | 话题约束规则 |
| `internal/guardrails/topic_test.go` | 话题约束测试 |
| `internal/guardrails/output.go` | 输出安全检查规则 |
| `internal/guardrails/output_test.go` | 输出检查测试 |
| `internal/guardrails/masker.go` | 脱敏处理器 |
| `internal/guardrails/masker_test.go` | 脱敏策略测试 |
| `internal/guardrails/hook.go` | GuardrailHook — 将引擎注册为 Hook |
| `internal/guardrails/hook_test.go` | Hook 集成测试 |
| `pkg/otel.go` | OTel 公共 API re-export |
| `pkg/guardrails.go` | Guardrails 公共 API re-export |

### 修改文件

| 文件 | 变更内容 |
|------|---------|
| `internal/agent/trace.go` | SpanContext 增加 TraceFlags/TraceState/Remote；新增 W3C 方法 |
| `internal/agent/hooks.go` | 新增 HookPhase 类型常量；Hook 增加 Phase 字段；RegisterInPhase 方法；Fire 按 Phase 排序 |
| `internal/agent/react_loop.go` | Tracer 字段类型从 `*LoggingTracer` 改为 `Tracer` 接口 |

---

## Task 1: SpanContext W3C 扩展 [1h]

**Files:** Modify `internal/agent/trace.go`, `internal/agent/trace_test.go`

- [ ] **Step 1: 写失败测试** — 在 `trace_test.go` 末尾追加 `TestSpanContext_W3CTraceParent`、`TestSpanContext_FromW3CTraceParent`、`TestSpanContext_FromW3CTraceParent_Invalid`、`TestSpanContext_WithTraceState`、`TestSpanContext_ToW3CTraceParent_Unsampled`

- [ ] **Step 2: 运行测试确认失败** — `go test ./internal/agent/ -run TestSpanContext_W3C -v`

- [ ] **Step 3: 实现** — SpanContext 增加 `TraceFlags byte`、`TraceState map[string]string`、`Remote bool` 字段；新增 `ToW3CTraceParent()`、`FromW3CTraceParent(s string)`、`WithTraceState(key, value string)` 方法

- [ ] **Step 4: 运行测试确认通过** — `go test ./internal/agent/ -run TestSpanContext -v`

- [ ] **Step 5: 全量回归** — `go test ./internal/agent/ -v`（新增字段零值不影响现有逻辑）

- [ ] **Step 6: 提交** — `feat: extend SpanContext with W3C TraceContext support`

---

## Task 2: Tracer 接口化 [1.5h]

**Files:** Create `internal/agent/tracer.go`, `internal/agent/tracer_test.go`; Modify `internal/agent/react_loop.go`

- [ ] **Step 1: 写失败测试** — `TestTracerInterface_LoggingTracer`、`TestTracerInterface_NoopTracer`、`TestTracerDebug_LoggingTracer`、`TestNoopTracer_DoesNotPanic`

- [ ] **Step 2: 运行测试确认失败**

- [ ] **Step 3: 创建 Tracer 接口和 NoopTracer** — `tracer.go` 定义 `Tracer` 接口（`Start` 方法）、`TracerDebug` 扩展接口（`Reset`/`String`）、`NoopTracer` 实现

- [ ] **Step 4: 修改 ReActConfig.Tracer 类型** — `*LoggingTracer` → `Tracer`

- [ ] **Step 5: 运行测试确认通过**

- [ ] **Step 6: 提交** — `feat: abstract Tracer interface with NoopTracer implementation`

---

## Task 3: HookPhase 执行阶段 [2h]

**Files:** Modify `internal/agent/hooks.go`, `internal/agent/hooks_test.go`

- [ ] **Step 1: 写失败测试** — `TestHookPhase_ValidationStopsExecution`、`TestHookPhase_ExecutionOrder`、`TestHookPhase_RegisterDefaultIsExecution`、`TestHookPhase_MultipleInSamePhase`

- [ ] **Step 2: 运行测试确认失败**

- [ ] **Step 3: 实现** — 新增 `HookPhase` 类型（`PhaseValidation`/`PhasePreProcessing`/`PhaseExecution`/`PhasePostProcessing`）；Hook 结构增加 `Phase` 字段；新增 `RegisterInPhase`/`RegisterConditionalInPhase` 方法；修改 `Fire` 按 Phase 排序执行；现有 `RegisterConditional` 默认使用 `PhaseExecution`

- [ ] **Step 4: 运行测试确认通过**

- [ ] **Step 5: 全量回归** — 现有 Hook 注册默认 PhaseExecution，行为不变

- [ ] **Step 6: 提交** — `feat: add HookPhase for validation-first hook execution`

---

## Task 4: OTLP HTTP 导出器 [2h]

**Files:** Create `internal/otel/doc.go`, `internal/otel/otlp_exporter.go`, `internal/otel/otlp_exporter_test.go`

- [ ] **Step 1: 创建包文档** — `doc.go`

- [ ] **Step 2: 写失败测试** — `TestOTLPExporter_ExportTraces`（httptest 验证 JSON 结构）、`TestOTLPExporter_ExportMetrics`、`TestOTLPExporter_RetryOnFailure`（5xx 重试）、`TestOTLPExporter_NoRetryOnClientError`（4xx 不重试）

- [ ] **Step 3: 运行测试确认失败**

- [ ] **Step 4: 实现** — `OTLPConfig`（Endpoint/Headers/MaxRetry/HTTPTimeout）、`OTLPExporter`（ExportTraces/ExportMetrics/Close）、`send` 方法（重试+退避）、`buildTracePayload`/`buildMetricsPayload`（JSON 编码）

- [ ] **Step 5: 运行测试确认通过**

- [ ] **Step 6: 提交** — `feat: add OTLP HTTP exporter with zero-dependency`

---

## Task 5: OTel SDK 桥接（构建标签隔离）[2h]

**Files:** Create `internal/otel/bridge_nootel.go`, `internal/otel/bridge.go`, `internal/otel/metrics_nootel.go`, `internal/otel/metrics.go`, `internal/otel/bridge_test.go`

- [ ] **Step 1: 创建 Noop 降级文件** — `bridge_nootel.go`（`//go:build !otel`，返回 NoopTracer）、`metrics_nootel.go`（`//go:build !otel`，noopMetricsBridge）

- [ ] **Step 2: 创建 OTel SDK 桥接文件** — `bridge.go`（`//go:build otel`，OTelBridgeTracer/Span，映射 AP SpanKind→OTel SpanKind，AP SpanContext→OTel SpanContext）、`metrics.go`（`//go:build otel`，otelMetricsBridge）

- [ ] **Step 3: 创建桥接测试** — `bridge_test.go`（默认构建验证 Noop 行为）

- [ ] **Step 4: 验证默认构建** — `go test ./internal/otel/ -v`（不引入 OTel SDK）

- [ ] **Step 5: 提交** — `feat: add OTel SDK bridge with build tag isolation`

---

## Task 6: TelemetryProvider 统一入口 [1.5h]

**Files:** Create `internal/otel/provider.go`, `internal/otel/provider_test.go`

- [ ] **Step 1: 写失败测试** — `TestNewTelemetryProvider_DefaultConfig`、`TestNewTelemetryProvider_WithOTLP`、`TestTelemetryProvider_TracerCreatesSpans`、`TestTelemetryProvider_DefaultServiceName`

- [ ] **Step 2: 运行测试确认失败**

- [ ] **Step 3: 实现** — `TelemetryConfig`（TraceEnabled/TraceEndpoint/TraceServiceName/MetricsEnabled/MetricsEndpoint/MetricsInterval/UseOTelSDK/OTelHeaders）、`TelemetryProvider`（Tracer/Exporter/StartMetricsExport/Shutdown）；默认使用 LoggingTracer + OTLPExporter；UseOTelSDK 时使用 OTelBridgeTracer

- [ ] **Step 4: 运行测试确认通过**

- [ ] **Step 5: 提交** — `feat: add TelemetryProvider unified entry point`

---

## Task 7: Guardrail 核心接口和引擎 [2h]

**Files:** Create `internal/guardrails/doc.go`, `internal/guardrails/guardrail.go`, `internal/guardrails/guardrail_test.go`, `internal/guardrails/engine.go`, `internal/guardrails/engine_test.go`

- [ ] **Step 1: 创建包文档** — `doc.go`

- [ ] **Step 2: 写 Guardrail 接口失败测试** — `TestGuardrailAction_Constants`、`TestGuardrailInterface`（mockGuardrail 实现）

- [ ] **Step 3: 写引擎失败测试** — `TestEngine_AllowWhenNoRules`、`TestEngine_RejectStopsExecution`、`TestEngine_MaskReplacesText`、`TestEngine_FlagContinuesExecution`、`TestEngine_ConfidenceThreshold`、`TestEngine_ErrorPropagation`、`TestEngine_OutputRules`、`TestEngine_AuditLogger`

- [ ] **Step 4: 运行测试确认失败**

- [ ] **Step 5: 实现 Guardrail 接口** — `GuardrailAction`（allow/mask/reject/flag）、`GuardrailResult`（Action/MaskedText/Reason/Confidence/RuleName/Details）、`Guardrail` 接口（Name/CheckInput/CheckOutput）、`AuditLogger` 接口

- [ ] **Step 6: 实现 GuardrailEngine** — `GuardrailConfig`（OnInputViolation/OnOutputViolation/ConfidenceThreshold/AuditLogger）、`GuardrailEngine`（AddInputRule/AddOutputRule/CheckInput/CheckOutput/check）；check 方法按顺序执行规则，Reject 立即返回，Mask 替换文本继续，Flag 记录继续，低于置信度阈值跳过

- [ ] **Step 7: 运行测试确认通过**

- [ ] **Step 8: 提交** — `feat: add Guardrail interface and engine with mask/reject/flag actions`

---

## Task 8: PII 检测规则 [1.5h]

**Files:** Create `internal/guardrails/pii.go`, `internal/guardrails/pii_test.go`

- [ ] **Step 1: 写失败测试** — `TestPIIRule_PhoneNumber`（Mask）、`TestPIIRule_IDCard`（Reject）、`TestPIIRule_Email`（Mask）、`TestPIIRule_NoPII`（Allow）、`TestPIIRule_BankCard`（Reject）、`TestPIIRule_IPv4`（Flag）

- [ ] **Step 2: 运行测试确认失败**

- [ ] **Step 3: 实现** — `PIIPattern`（Name/Regex/Action/Confidence/MaskFunc）、`PIIConfig`、`PIIDefaultConfig()`（手机号/身份证/邮箱/银行卡/IPv4 五种模式）、`PIIRule`（check 方法遍历模式，Reject 立即返回，Mask 替换文本）、脱敏函数（phoneMask/idCardMask/emailMask/genericMask）

- [ ] **Step 4: 运行测试确认通过**

- [ ] **Step 5: 提交** — `feat: add PII detection rule with phone/idcard/email/bankcard/ipv4 patterns`

---

## Task 9: 敏感词过滤规则（Trie 多模式匹配）[2h]

**Files:** Create `internal/guardrails/sensitive.go`, `internal/guardrails/sensitive_test.go`

- [ ] **Step 1: 写失败测试** — `TestSensitiveWordRule_BasicMatch`（Reject）、`TestSensitiveWordRule_NoMatch`（Allow）、`TestSensitiveWordRule_FlagAction`、`TestSensitiveWordRule_MaskAction`、`TestSensitiveWordRule_Category`（按类别批量添加）

- [ ] **Step 2: 运行测试确认失败**

- [ ] **Step 3: 实现** — `sensitiveTrie`（Trie 树，支持中文 rune 匹配）、`sensitiveTrieNode`（children/isEnd/word/action）、`search` 方法（扫描文本找所有匹配，返回最高优先级动作）、`SensitiveWordRule`（AddWord/AddCategory/CheckInput/CheckOutput）；Mask 动作将匹配词替换为等长星号

- [ ] **Step 4: 运行测试确认通过**

- [ ] **Step 5: 提交** — `feat: add sensitive word filter with Trie multi-pattern matching`

---

## Task 10: Prompt 注入检测规则 [1.5h]

**Files:** Create `internal/guardrails/injection.go`, `internal/guardrails/injection_test.go`

- [ ] **Step 1: 写失败测试** — `TestInjectionRule_SystemPromptLeak`（检测 "ignore previous instructions" 等模式）、`TestInjectionRule_RoleManipulation`（检测 "you are now" 等角色操控）、`TestInjectionRule_CodeInjection`（检测代码注入模式）、`TestInjectionRule_NormalInput`（正常输入允许通过）、`TestInjectionRule_CustomPattern`（自定义注入模式）

- [ ] **Step 2: 运行测试确认失败**

- [ ] **Step 3: 实现** — `InjectionConfig`（Patterns/Threshold/Action）、`InjectionDefaultConfig()`（预定义常见注入模式：system prompt leak/role manipulation/code injection/data exfiltration）、`InjectionRule`（CheckInput 检测注入，CheckOutput 允许通过）；使用正则匹配 + 置信度评分

- [ ] **Step 4: 运行测试确认通过**

- [ ] **Step 5: 提交** — `feat: add prompt injection detection rule`

---

## Task 11: 话题约束规则 [1h]

**Files:** Create `internal/guardrails/topic.go`, `internal/guardrails/topic_test.go`

- [ ] **Step 1: 写失败测试** — `TestTopicRule_AllowedTopic`（允许的话题通过）、`TestTopicRule_DisallowedTopic`（禁止的话题拒绝）、`TestTopicRule_WhitelistMode`（白名单模式，仅允许列表内话题）、`TestTopicRule_EmptyConfig`（空配置允许所有）

- [ ] **Step 2: 运行测试确认失败**

- [ ] **Step 3: 实现** — `TopicConfig`（AllowedTopics/DisallowedTopics/Mode/Action）、`TopicRule`（CheckInput 检查话题约束，CheckOutput 允许通过）；支持黑名单模式（默认）和白名单模式；话题匹配使用关键词匹配

- [ ] **Step 4: 运行测试确认通过**

- [ ] **Step 5: 提交** — `feat: add topic constraint rule`

---

## Task 12: 输出安全检查规则 [1h]

**Files:** Create `internal/guardrails/output.go`, `internal/guardrails/output_test.go`

- [ ] **Step 1: 写失败测试** — `TestOutputRule_SafeOutput`（安全输出通过）、`TestOutputRule_HarmfulContent`（有害内容拒绝）、`TestOutputRule_PIIInOutput`（输出中 PII 脱敏）、`TestOutputRule_CustomCheck`（自定义检查函数）

- [ ] **Step 2: 运行测试确认失败**

- [ ] **Step 3: 实现** — `OutputConfig`（MaxLength/ForbiddenPatterns/CustomChecks/Action）、`OutputRule`（CheckInput 允许通过，CheckOutput 执行检查）；支持长度限制、禁止模式、自定义检查函数

- [ ] **Step 4: 运行测试确认通过**

- [ ] **Step 5: 提交** — `feat: add output safety check rule`

---

## Task 13: 脱敏处理器 [1h]

**Files:** Create `internal/guardrails/masker.go`, `internal/guardrails/masker_test.go`

- [ ] **Step 1: 写失败测试** — `TestMasker_PartialMask`（部分脱敏）、`TestMasker_FullMask`（完全脱敏）、`TestMasker_CustomReplacement`（自定义替换字符）、`TestMasker_PreserveLength`（保持长度）、`TestMasker_MultipleMatches`（多处匹配）

- [ ] **Step 2: 运行测试确认失败**

- [ ] **Step 3: 实现** — `MaskerConfig`（ReplacementChar/PreserveLength/KeepPrefix/KeepSuffix）、`Masker`（Mask 方法，支持部分脱敏/完全脱敏/自定义替换）；作为 PII/敏感词规则的共享脱敏工具

- [ ] **Step 4: 运行测试确认通过**

- [ ] **Step 5: 提交** — `feat: add text masker with configurable masking strategies`

---

## Task 14: GuardrailHook 集成 [1.5h]

**Files:** Create `internal/guardrails/hook.go`, `internal/guardrails/hook_test.go`

- [ ] **Step 1: 写失败测试** — `TestGuardrailHook_RegistersInValidationPhase`（验证注册到 PhaseValidation）、`TestGuardrailHook_InputBlocked`（输入被拒绝时阻止 LLM 调用）、`TestGuardrailHook_InputMasked`（输入被脱敏时修改 HookContext）、`TestGuardrailHook_OutputBlocked`（输出被拒绝时阻止返回）、`TestGuardrailHook_IntegrationWithHookManager`（与 HookManager 集成测试）

- [ ] **Step 2: 运行测试确认失败**

- [ ] **Step 3: 实现** — `GuardrailHook`（包装 GuardrailEngine，实现 HookFunc 签名）、`NewGuardrailHook`（创建并返回 HookFunc + Phase）；BeforeLLM 调用 CheckInput，AfterLLM 调用 CheckOutput；拒绝时返回错误，脱敏时修改 HookContext.Input

- [ ] **Step 4: 运行测试确认通过**

- [ ] **Step 5: 提交** — `feat: add GuardrailHook integrating engine into Hook system`

---

## Task 15: pkg/ 公共 API re-export [0.5h]

**Files:** Create `pkg/otel.go`, `pkg/guardrails.go`

- [ ] **Step 1: 创建 OTel re-export** — `pkg/otel.go`：re-export `TelemetryProvider`、`TelemetryConfig`、`OTLPConfig`、`NewTelemetryProvider`

- [ ] **Step 2: 创建 Guardrails re-export** — `pkg/guardrails.go`：re-export `GuardrailEngine`、`GuardrailConfig`、`GuardrailResult`、`GuardrailAction`、`NewGuardrailEngine`、`NewPIIRule`、`NewSensitiveWordRule`、`NewInjectionRule`、`NewTopicRule`、`NewOutputRule`、`NewGuardrailHook`

- [ ] **Step 3: 验证编译** — `go build ./pkg/...`

- [ ] **Step 4: 提交** — `feat: add pkg re-exports for OTel and Guardrails`

---

## Task 16: 集成测试 + 全量回归 [2h]

- [ ] **Step 1: OTel 集成测试** — 验证 TelemetryProvider → LoggingTracer → OTLPExporter 完整链路

- [ ] **Step 2: Guardrails 集成测试** — 验证 GuardrailEngine + 多规则 + GuardrailHook + HookManager 完整链路

- [ ] **Step 3: 全量回归** — `go test ./...` 确保所有测试通过

- [ ] **Step 4: 构建标签验证** — `go build -tags otel ./...` 确认 OTel SDK 构建通过

- [ ] **Step 5: 提交** — `test: add integration tests for OTel and Guardrails`

---

## 依赖关系图

```
Task 1 (SpanContext W3C) ──┐
                            ├── Task 2 (Tracer 接口化) ──┐
Task 3 (HookPhase) ────────┤                             ├── Task 5 (OTel SDK 桥接) ── Task 6 (Provider)
Task 4 (OTLP 导出器) ──────┘                             │
                            │                             │
Task 3 (HookPhase) ────────┼── Task 7 (Guardrail 引擎) ──┤
                            │                             │
                            ├── Task 8 (PII) ─────────────┤
                            ├── Task 9 (敏感词) ──────────┤
                            ├── Task 10 (注入检测) ───────┤
                            ├── Task 11 (话题约束) ───────┤
                            ├── Task 12 (输出检查) ───────┤
                            ├── Task 13 (脱敏处理器) ─────┤
                            │                             │
Task 3 + Task 7 ───────────┴── Task 14 (Hook 集成) ──────┤
                                                        │
Task 6 + Task 14 ────────────────────────────────────── Task 15 (pkg re-export)
                                                        │
全部 ─────────────────────────────────────────────────── Task 16 (集成测试)
```

## 关键设计决策

1. **构建标签隔离**：OTel SDK 桥接使用 `//go:build otel` 隔离，默认构建零外部依赖
2. **HookPhase 优先级**：Guardrails 固定在 PhaseValidation 阶段执行，确保安全检查优先于业务逻辑
3. **Trie 多模式匹配**：敏感词过滤使用 Trie 树实现 O(n) 复杂度的多模式匹配（n 为文本长度）
4. **置信度阈值**：所有规则支持置信度评分，低于阈值的检测结果被过滤
5. **审计日志**：GuardrailEngine 支持 AuditLogger 接口，记录所有违规事件

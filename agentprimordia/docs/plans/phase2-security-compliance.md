# 阶段二：安全合规闭环实施计划（2-3 周）

> **状态：已完成 ✅**（7/7 Task 全部完成；guardrail/audit/security 三层防护已落地并测试通过）
> **创建日期：2026-07-05**
> **前置文档**：`docs/plans/2026-06-22-long-term-vision.md`（长期愿景 Phase 8）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

## 目标

完成长期愿景 Phase 8（安全与合规）的剩余工作：PII 输出端覆盖、审计日志全事件覆盖、权限继承体系、合规报告自动生成，使 AgentPrimordia 满足企业级安全合规要求。

## 当前状态盘点

| 组件 | 状态 | 说明 |
|------|------|------|
| PII 检测器 (`guardrail/pii.go`) | ✅ 基础完成 | 支持 Email/Phone/IDCard/CreditCard/IPAddress 5 种类型 |
| PII 脱敏规则 (`guardrail/pii_rule.go`) | ✅ 完成 | 实现 Rule 接口，集成到 Guardrail Engine |
| 审计日志 (`audit/logger.go`) | ✅ 基础完成 | Event/QueryFilter/ComplianceReport/ExportJSON |
| Guardrail Engine (`guardrail/engine.go`) | ✅ 完成 | 支持 Input/Output 检查点、规则链、优先级 |
| 注入防护 (`guardrail/injection_rule.go`) | ✅ 完成 | Prompt injection 检测 |
| 安全沙箱 (`security/sandbox.go`) | ✅ 完成 | 命令白名单 + 路径限制 |
| 权限继承 | ⬜ 未开始 | 需统一 `pkg/security.go` 接口 |
| 合规报告 API | ⬜ 未开始 | 依赖审计日志全事件覆盖 |

---

## Phase 2A：PII 输出端覆盖 + 模式扩展（第 1-3 天）

### Task 1: PII 输出端自动拦截

**问题**：当前 PII 脱敏仅在 `CheckPointInput` 生效，LLM 响应输出端的 PII 未被拦截。

**Files:**
- Modify: `internal/guardrail/pii_rule.go`
- Modify: `internal/agent/react_loop_core.go`（在 LLM 响应后插入 output 检查点）
- Create: `internal/guardrail/pii_output_test.go`

- [ ] **Step 1: 编写输出端 PII 拦截测试**

```go
// internal/guardrail/pii_output_test.go
func TestPIIRule_OutputCheckpoint(t *testing.T) {
    rule := NewSanitizeRule(SanitizeConfig{ReplaceWith: "[REDACTED]"})
    
    // LLM 响应中包含 PII
    output := "用户邮箱是 zhangsan@example.com，电话 13812345678"
    result, err := rule.Check(output, CheckPointOutput)
    
    if err != nil {
        t.Fatalf("检查失败: %v", err)
    }
    if result.Action != ActionSanitize {
        t.Errorf("Action = %v, 期望 ActionSanitize", result.Action)
    }
    if strings.Contains(result.Sanitized, "zhangsan@example.com") {
        t.Error("输出端 PII 未被脱敏")
    }
}
```

- [ ] **Step 2: 确保 PII Rule 在 Output 检查点生效**

检查 `pii_rule.go` 的 `Check` 方法是否已支持 `CheckPointOutput`。如果 `Check` 不区分检查点，则天然支持。如果需要在 output 端使用不同策略（如仅记录不拦截），添加 `CheckPoint` 参数判断。

- [ ] **Step 3: 在 ReAct Loop 中插入 output 检查点**

在 `react_loop_core.go` 中，LLM 响应返回后、写入消息历史前，调用 Guardrail Engine 的 output 检查：

```go
// LLM 响应后
if a.guardrail != nil {
    result, err := a.guardrail.Check(response.Content, CheckPointOutput)
    if err != nil {
        return err
    }
    if result.Action == ActionSanitize {
        response.Content = result.Sanitized
    }
    if result.Action == ActionBlock {
        return fmt.Errorf("output blocked by guardrail: %s", result.Message)
    }
}
```

- [ ] **Step 4: 验证**

```bash
go test -race -count=1 ./internal/guardrail/ -run TestPII
go test -race -count=1 ./internal/agent/ -run TestGuardrail
```

---

### Task 2: PII 模式扩展

**问题**：当前仅支持 5 种 PII 类型，缺少护照号、银行账号、社保号等。

**Files:**
- Modify: `internal/guardrail/pii.go`

- [ ] **Step 1: 添加新 PII 类型**

```go
const (
    // 现有
    Email      PIIType = "email"
    Phone      PIIType = "phone"
    IDCard     PIIType = "id_card"
    CreditCard PIIType = "credit_card"
    IPAddress  PIIType = "ip_address"
    // 新增
    Passport      PIIType = "passport"       // 护照号（中国：E+8位数字；美国：9位数字）
    BankAccount   PIIType = "bank_account"    // 银行账号（16-19位，特定前缀）
    SSN           PIIType = "ssn"            // 美国社保号（XXX-XX-XXXX）
    APIKey        PIIType = "api_key"         // API 密钥（sk-/pk-/AKIA 前缀）
    JWT           PIIType = "jwt"            // JWT 令牌（三段式 base64）
)
```

- [ ] **Step 2: 编写正则模式**

```go
{typ: Passport, regex: regexp.MustCompile(`(?:E\d{8})|(?:[A-Z]{2}\d{6})`)},
{typ: BankAccount, regex: regexp.MustCompile(`\b[1-9]\d{15,18}\b`)},
{typ: SSN, regex: regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`)},
{typ: APIKey, regex: regexp.MustCompile(`(?:sk-[a-zA-Z0-9]{20,})|(?:pk_[a-zA-Z0-9]{20,})|(?:AKIA[A-Z0-9]{16})`)},
{typ: JWT, regex: regexp.MustCompile(`eyJ[a-zA-Z0-9_-]+\.eyJ[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+`)},
```

- [ ] **Step 3: 编写测试**

```go
func TestPIIDetector_Passport(t *testing.T) { /* ... */ }
func TestPIIDetector_BankAccount(t *testing.T) { /* ... */ }
func TestPIIDetector_SSN(t *testing.T) { /* ... */ }
func TestPIIDetector_APIKey(t *testing.T) { /* ... */ }
func TestPIIDetector_JWT(t *testing.T) { /* ... */ }
```

- [ ] **Step 4: 验证**

```bash
go test -v ./internal/guardrail/ -run TestPII
```

---

## Phase 2B：审计日志全事件覆盖（第 4-8 天）

### Task 3: 审计事件类型定义

**问题**：当前审计 `Event.Action` 为自由字符串，缺乏标准化事件类型枚举。

**Files:**
- Create: `internal/audit/event_types.go`
- Create: `internal/audit/event_types_test.go`

- [ ] **Step 1: 定义标准审计事件类型**

```go
// internal/audit/event_types.go
package audit

// AuditAction 标准审计操作类型
type AuditAction string

const (
    // Agent 生命周期
    ActionAgentStart    AuditAction = "agent.start"
    ActionAgentStop     AuditAction = "agent.stop"
    ActionAgentPanic    AuditAction = "agent.panic"
    ActionAgentResume   AuditAction = "agent.resume"
    
    // LLM 调用
    ActionLLMCall       AuditAction = "llm.call"
    ActionLLMStream     AuditAction = "llm.stream"
    ActionLLMError      AuditAction = "llm.error"
    
    // 工具调用
    ActionToolCall      AuditAction = "tool.call"
    ActionToolResult    AuditAction = "tool.result"
    ActionToolError     AuditAction = "tool.error"
    ActionToolDenied    AuditAction = "tool.denied"
    
    // 文件操作
    ActionFileRead      AuditAction = "file.read"
    ActionFileWrite     AuditAction = "file.write"
    ActionFileDelete    AuditAction = "file.delete"
    
    // 网络请求
    ActionHTTPRequest   AuditAction = "http.request"
    
    // 权限变更
    ActionPermissionGrant  AuditAction = "permission.grant"
    ActionPermissionRevoke AuditAction = "permission.revoke"
    
    // 配置变更
    ActionConfigChange  AuditAction = "config.change"
    
    // Guardrail 拦截
    ActionGuardrailBlock   AuditAction = "guardrail.block"
    ActionGuardrailSanitize AuditAction = "guardrail.sanitize"
)
```

- [ ] **Step 2: 验证**

```bash
go test -v ./internal/audit/ -run TestEventTypes
```

---

### Task 4: 审计日志集成到 ReAct Loop

**问题**：ReAct Loop 的 LLM 调用、工具调用、Guardrail 拦截未写入审计日志。

**Files:**
- Modify: `internal/agent/react_loop_core.go`
- Modify: `internal/agent/react_loop_tools.go`
- Modify: `internal/agent/react_lifecycle.go`

- [ ] **Step 1: 在 Agent 中注入 AuditLogger**

```go
type ReActAgent struct {
    // ... 现有字段
    auditLogger *audit.Logger // 可选，nil 表示不记录审计日志
}

func (a *ReActAgent) WithAuditLogger(l *audit.Logger) *ReActAgent {
    a.auditLogger = l
    return a
}
```

- [ ] **Step 2: 在关键路径写入审计事件**

在 `react_loop_core.go` 的 LLM 调用后：
```go
if a.auditLogger != nil {
    a.auditLogger.Log(ctx, audit.Event{
        Actor:    a.name,
        Action:   string(audit.ActionLLMCall),
        Resource: req.Model,
        Details: map[string]any{
            "prompt_tokens": resp.Usage.PromptTokens,
            "completion_tokens": resp.Usage.CompletionTokens,
        },
        Result: "success",
    })
}
```

在 `react_loop_tools.go` 的工具执行后：
```go
if a.auditLogger != nil {
    a.auditLogger.Log(ctx, audit.Event{
        Actor:    a.name,
        Action:   string(audit.ActionToolCall),
        Resource: tc.Name,
        Details: map[string]any{
            "duration_ms": duration.Milliseconds(),
        },
        Result: "success",
    })
}
```

- [ ] **Step 3: 异步写入（避免阻塞主循环）**

```go
// 使用 buffered channel 异步写入审计日志
type AsyncAuditLogger struct {
    logger *audit.Logger
    ch     chan audit.Event
    done   chan struct{}
}

func NewAsyncAuditLogger(logger *audit.Logger, bufSize int) *AsyncAuditLogger {
    l := &AsyncAuditLogger{
        logger: logger,
        ch:     make(chan audit.Event, bufSize),
        done:   make(chan struct{}),
    }
    go l.process()
    return l
}

func (l *AsyncAuditLogger) Log(ctx context.Context, event audit.Event) error {
    select {
    case l.ch <- event:
        return nil
    default:
        // 缓冲区满时降级为同步写入
        return l.logger.Log(ctx, event)
    }
}
```

- [ ] **Step 4: 编写集成测试**

```go
func TestReActLoop_AuditLogWritten(t *testing.T) {
    output := &audit.MemoryOutput{}
    logger := audit.NewLogger(audit.LoggerConfig{Output: output})
    
    agent := NewReActAgent(/* ... */).WithAuditLogger(logger)
    agent.Run(ctx, "test prompt")
    
    events := output.Events()
    // 验证 LLM 调用被记录
    // 验证工具调用被记录
}
```

- [ ] **Step 5: 验证**

```bash
go test -race -count=1 ./internal/agent/ -run TestAudit
go test -race -count=1 ./internal/audit/
```

---

### Task 5: 审计日志 HTTP 查询接口

**Files:**
- Create: `internal/audit/http_handler.go`
- Create: `internal/audit/http_handler_test.go`

- [ ] **Step 1: 实现 HTTP handler**

```go
// GET /audit/events?actor=xxx&action=xxx&limit=100
// GET /audit/report?start=2026-01-01&end=2026-12-31
type HTTPHandler struct {
    logger *Logger
}

func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    switch r.URL.Path {
    case "/audit/events":
        h.handleQuery(w, r)
    case "/audit/report":
        h.handleReport(w, r)
    }
}
```

- [ ] **Step 2: 在 pkg/ 中导出**

```go
// pkg/audit.go
type AuditHTTPHandler = audit.HTTPHandler
var NewAuditHTTPHandler = audit.NewHTTPHandler
```

- [ ] **Step 3: 验证**

```bash
go test -v ./internal/audit/ -run TestHTTP
```

---

## Phase 2C：权限继承体系（第 9-12 天）

### Task 6: 统一权限接口

**问题**：当前 `security/sandbox.go` 和 `tools/executor.go` 各有权限逻辑，缺乏统一的权限继承链。

**Files:**
- Create: `internal/security/permission.go`
- Create: `internal/security/permission_test.go`
- Modify: `pkg/security.go`（导出公共 API）

- [ ] **Step 1: 定义权限接口**

```go
// internal/security/permission.go
package security

// Permission 权限接口，所有权限系统实现此接口
type Permission interface {
    // Allow 检查是否允许操作指定资源
    Allow(agentID string, resource string) bool
    // Grant 授予权限
    Grant(agentID string, resource string, perm PermissionLevel) error
    // Revoke 撤销权限
    Revoke(agentID string, resource string) error
    // Inherit 从父 Agent 继承权限
    Inherit(parentAgentID, childAgentID string) error
}

// PermissionLevel 权限级别
type PermissionLevel int

const (
    PermNone   PermissionLevel = iota // 无权限
    PermRead                           // 只读
    PermWrite                          // 读写
    PermExecute                        // 执行
    PermAdmin                          // 管理员
)
```

- [ ] **Step 2: 实现 RBAC + Scope 组合权限管理器**

```go
type PermissionManager struct {
    mu       sync.RWMutex
    roles    map[string]*Role          // agentID → Role
    scopes   map[string]ScopePolicy    // agentID → ScopePolicy
    children map[string][]string       // parentAgentID → []childAgentID
}

type Role struct {
    AgentID string
    Level   PermissionLevel
    Resources []string // 允许访问的资源模式
}
```

- [ ] **Step 3: 实现权限继承**

```go
// Inherit 让子 Agent 继承父 Agent 的权限（可收窄不可放大）
func (pm *PermissionManager) Inherit(parentID, childID string) error {
    pm.mu.Lock()
    defer pm.mu.Unlock()
    
    parentRole, ok := pm.roles[parentID]
    if !ok {
        return fmt.Errorf("parent agent %q not found", parentID)
    }
    
    // 子 Agent 继承父 Agent 权限（级别不超过父 Agent）
    childRole := &Role{
        AgentID:   childID,
        Level:     parentRole.Level, // 继承相同级别
        Resources: parentRole.Resources, // 继承相同资源
    }
    pm.roles[childID] = childRole
    pm.children[parentID] = append(pm.children[parentID], childID)
    
    return nil
}
```

- [ ] **Step 4: 编写测试**

```go
func TestPermission_Inherit(t *testing.T) { /* ... */ }
func TestPermission_Inherit_CannotEscalate(t *testing.T) { /* ... */ }
func TestPermission_Revoke(t *testing.T) { /* ... */ }
func TestPermission_ChildCannotExceedParent(t *testing.T) { /* ... */ }
```

- [ ] **Step 5: 验证**

```bash
go test -race -count=1 ./internal/security/
```

---

## Phase 2D：合规报告自动生成（第 13-15 天）

### Task 7: 合规报告模板

**Files:**
- Create: `internal/audit/report_templates.go`
- Create: `internal/audit/report_templates_test.go`

- [ ] **Step 1: 定义报告模板**

```go
// internal/audit/report_templates.go
package audit

// ReportTemplate 报告模板类型
type ReportTemplate string

const (
    TemplateSOC2  ReportTemplate = "soc2"
    TemplateGDPR  ReportTemplate = "gdpr"
    TemplateCustom ReportTemplate = "custom"
)

// ReportConfig 报告配置
type ReportConfig struct {
    Template ReportTemplate
    Start    time.Time
    End      time.Time
    Actors   []string // 筛选特定 Actor，空表示全部
}

// GenerateComplianceReport 生成符合指定模板的合规报告
func (l *Logger) GenerateComplianceReport(ctx context.Context, cfg ReportConfig) (*ComplianceReport, error) {
    switch cfg.Template {
    case TemplateSOC2:
        return l.generateSOC2Report(ctx, cfg)
    case TemplateGDPR:
        return l.generateGDPRReport(ctx, cfg)
    default:
        return l.GenerateReport(ctx, cfg.Start, cfg.End)
    }
}
```

- [ ] **Step 2: SOC2 报告生成**

```go
func (l *Logger) generateSOC2Report(ctx context.Context, cfg ReportConfig) (*ComplianceReport, error) {
    events, err := l.config.Output.Query(QueryFilter{
        Start: cfg.Start,
        End:   cfg.End,
    })
    if err != nil {
        return nil, err
    }
    
    report := &ComplianceReport{
        Period:      PeriodStats{Start: cfg.Start, End: cfg.End},
        TotalEvents: len(events),
        ActorStats:  make(map[string]ActorStats),
        ActionStats: make(map[string]int),
    }
    
    // SOC2 关注：访问控制、操作审计、异常检测
    // 统计被拒绝的操作
    // 统计异常事件（panic、error、guardrail block）
    // 按 Actor 分组统计
    
    return report, nil
}
```

- [ ] **Step 3: GDPR 报告生成**

```go
func (l *Logger) generateGDPRReport(ctx context.Context, cfg ReportConfig) (*ComplianceReport, error) {
    // GDPR 关注：个人数据处理、PII 脱敏记录、数据主体权利
    // 统计 PII 脱敏事件
    // 统计个人数据访问记录
    // 生成数据保留策略执行情况
    
    return report, nil
}
```

- [ ] **Step 4: 编写测试**

```go
func TestGenerateComplianceReport_SOC2(t *testing.T) { /* ... */ }
func TestGenerateComplianceReport_GDPR(t *testing.T) { /* ... */ }
```

- [ ] **Step 5: 在 HTTP handler 中暴露报告接口**

```go
// GET /audit/report?template=soc2&start=...&end=...
```

- [ ] **Step 6: 验证**

```bash
go test -race -count=1 ./internal/audit/
```

---

## 验收标准

1. `go build ./...` 和 `go vet ./...` 零错误
2. `go test -race -count=1 ./...` 全部通过
3. PII 检测器支持 10 种类型（原 5 + 新 5）
4. LLM 响应输出端 PII 被自动拦截和脱敏
5. ReAct Loop 的 LLM 调用、工具调用、Guardrail 拦截均写入审计日志
6. 审计日志支持异步写入，不阻塞主循环
7. 审计日志支持 HTTP 查询接口（`/audit/events`、`/audit/report`）
8. 权限管理器支持继承链，子 Agent 权限不超过父 Agent
9. 合规报告支持 SOC2 和 GDPR 模板
10. 覆盖率：`internal/guardrail` ≥90%、`internal/audit` ≥85%、`internal/security` ≥90%

## 预期成果

| 指标 | 当前 | 目标 |
|------|------|------|
| PII 类型覆盖 | 5 种 | 10 种 |
| 审计事件覆盖 | 基础 | 全生命周期（Agent/LLM/Tool/File/Guardrail） |
| 权限模型 | 独立 | 继承链 + RBAC + Scope 组合 |
| 合规报告 | 基础统计 | SOC2 + GDPR 模板 |
| 审计写入方式 | 同步 | 异步（buffered channel） |

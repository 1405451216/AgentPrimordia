# Phase 6: 协议式微内核 + 公共 API 收尾 + 生态重组 — 实施计划

> **日期**: 2026-06-04
> **状态**: Code Complete (backfill)
> **前置条件**: Phase 5 公共 API 收尾（WorkflowExecution / SummaryEngine / CostTracker / ContentPart 等已暴露到 `pkg/`）
> **后续**: Phase 7 候选：API 兼容性策略 + 覆盖率门槛 + 文档站拆分

---

## 总览

Phase 6 的核心命题是**把 AgentPrimordia 从"功能堆叠"过渡到"可扩展架构"**，让核心包稳定、生态侧自由扩展。三个互锁的子目标：

| # | 子目标 | 落地形式 | 状态 |
|:-:|--------|----------|:----:|
| 6-A | 协议式微内核 | `*Capable` 接口 + `CapabilityAgent` 包装器 + `WithXxx` 链式 API | ✅ Code Complete |
| 6-B | LLM Provider 模板 | `internal/llm/provider_template.go` + `ecosystem/contributing/PROVIDER.md` | ✅ Code Complete |
| 6-C | 仓库结构重组 | `docs/` + `examples/` → `ecosystem/`；新增 `plugins/` + `templates/` + `contributing/` | ✅ Code Complete |
| 6-D | 公共 API 收尾 | `pkg/tools.go` 暴露 `PluginLoader / ToolPlugin / PluginInfo` | ✅ Code Complete |

**实际提交（领先 origin/main 8 个 commit）**:

```
070d3a3 refactor: migrate docs/examples to ecosystem/ and add plugins + scaffold (Phase 6)
dff74c5 feat(pkg): expose PluginLoader / ToolPlugin / PluginInfo (Phase 6)
7488d5b feat(llm): add Provider template for ecosystem contribution (Phase 6)
3a913b2 feat(agent): capability-based microkernel + chain API (Phase 6)
53e00b8 feat(pkg): expose WorkflowExecution engine to pkg/ (Phase 6 prerequisite)
0b614a5 feat(pkg): expose ContentPart, ContentType constants, Engine.RuleCount
460092a feat: export SummaryEngine, SummaryStrategy, WindowSummaryStrategy from pkg/agent.go
bb65209 feat: export CostTracker, ModelPricing from public pkg API
```

> Phase 6 在仓库内**没有先行 spec**——本文档为代码落地后的反向归档。后续 Phase 严禁重蹈此模式：先有 plan，再有 commit。

---

## 子阶段 6-A：协议式微内核（核心架构变更）

### 任务清单

| # | Task | 复杂度 | 文件 |
|:-:|------|:------:|------|
| A1 | 定义 `*Capable` 接口协议 | ⭐⭐ | `internal/agent/capabilities.go` |
| A2 | `CapabilityAgent` 包装器 + `WithXxx` 链式方法 | ⭐⭐⭐ | `internal/agent/capability_agent.go` |
| A3 | `ReActAgent.WithXxx` 入口（链式 API） | ⭐⭐ | `internal/agent/chain_api.go` |
| A4 | ReAct 引擎重构：能力探测改走 `a.self.(XxxCapable)` | ⭐⭐⭐ | `internal/agent/react_loop.go` |
| A5 | 能力接口的单元测试 | ⭐⭐ | `internal/agent/capability_agent_test.go` |

**总规模**：~1100 行新增（含测试）

### 设计意图

**问题**：ReAct Agent 的能力（Memory / RAG / Hooks / Tracer / CostTracker …）早期都通过 `ReActConfig` 的零散字段暴露，导致：
- 配置膨胀（10+ 能力字段）
- 能力未配置时引擎仍需 `if a.config.Xxx != nil` 散落在 30+ 处
- 生态扩展需要改 `ReActConfig`，污染公共 API

**方案**：借鉴 Go 标准库的接口发现模式（`io.Reader`、`http.Handler`），把能力从"配置字段"提升为"接口协议"。

```go
// 引擎内部（react_loop.go）：
func (a *ReActAgent) getMemoryStore() MemoryStore {
    if c, ok := a.self.(MemoryCapable); ok && c.GetMemoryStore() != nil {
        return c.GetMemoryStore()    // 优先：接口发现
    }
    return a.config.Memory          // 回退：旧 config 字段（向后兼容）
}
```

引擎与具体能力**完全解耦**——新增能力只需定义新 `*Capable` 接口，引擎无需感知。

### 12 个 Capable 接口清单

| 接口 | 能力 | 引擎在哪用 | 链式方法 |
|------|------|-----------|---------|
| `MemoryCapable` | 对话/记忆持久化 | `saveMemory()` | `WithMemory()` |
| `RAGCapable` | 知识库检索注入 | `shouldRAG()` / `searchRAG()` | `WithRAG()` |
| `HITLCapable` | 人机协作中断 | 工具执行前 | `WithHITL()` |
| `HookCapable` | 生命周期钩子 | `fireHook()` | `WithHooks()` |
| `TraceCapable` | 分布式追踪 Span | `runLoop()` 起止 | `WithTracer()` |
| `CostCapable` | Token 成本追踪 | `recordUsage()` | `WithCostTracker()` |
| `ContextWindowCapable` | 历史消息裁剪 | `trimContext()` | `WithContextWindow()` |
| `EventCapable` | 生命周期事件 | `publishEvent()` | `WithEvents()` |
| `MetricsCapable` | Prometheus 指标 | 多处 | `WithMetrics()` |
| `CheckpointCapable` | 状态持久化 | `saveCheckpoint()` | `WithCheckpointStore()` |
| `SummarizerCapable` | 记忆摘要提取 | `saveMemory()` 异步 | `WithSummarizer()` |
| `FileScopeCapable` | 文件权限作用域 | 系统提示词构造 | `WithFileScope()` |
| `CacheCapable` | LLM 响应缓存 | `WithCache()` 内部包装 Model | `WithCache()` |

> `CacheCapable` 与其他不同：它的"启用"会自动把 `Model` 包装为 `CachedProvider`（见 `chain_api.go:108-118`），属于能力激活会改写其它能力的特殊情形。

### `ReActConfig` 字段废弃计划

`react_loop.go` 中以下字段已加 `// Deprecated:` 注释：

```go
// Deprecated: 使用 .WithMemory(store) 链式方法注入
Memory MemoryStore
// Deprecated: 使用 .WithToolkit(registry) 链式方法注入
Toolkit *tools.Registry
// ... 9 个其他字段
```

**废弃时间表**（待 Phase 7 决策）：
- **v0.x（当前）**：`config.Xxx` 仍可使用，引擎优先 `a.self.(XxxCapable)` 探测，回退到字段
- **v1.0（Phase 7 候选）**：在 godoc 加 `// Deprecated:` 与 `// Removed in v2.0.`
- **v2.0**：移除字段，仅保留链式 API

**理由**：一次大版本破坏 10 个字段 = 巨大破坏面。给用户两个小版本窗口迁移。

### 接口发现的正确用法

```go
// ✅ 推荐：链式 API（编译期类型安全、IDE 自动补全）
agent := NewReActAgent(ReActConfig{
    Name: "my-agent", Model: provider, MaxTurns: 10,
}).WithMemory(mem).WithRAG(RAGConfig{...}).WithHooks(hooks)

// ⚠️ 仍可用：直接 config（向后兼容）
agent := NewReActAgent(ReActConfig{
    Name: "my-agent", Model: provider, MaxTurns: 10,
    Memory: mem, RAG: &RAGConfig{...}, Hooks: hooks,
})
```

两种写法底层都走相同的 `a.self.(XxxCapable)` 探测路径。

---

## 子阶段 6-B：LLM Provider 模板

### 任务清单

| # | Task | 复杂度 | 文件 |
|:-:|------|:------:|------|
| B1 | `TemplateProvider` 模板代码 | ⭐⭐ | `internal/llm/provider_template.go` |
| B2 | 模板行为测试 | ⭐⭐ | `internal/llm/provider_template_test.go` |
| B3 | 贡献者指南 | ⭐ | `ecosystem/contributing/PROVIDER.md` |

**总规模**：~440 行（含测试）

### 设计意图

新增 LLM Provider 此前需修改 `internal/llm` 核心包、阅读 4+ 现有 Provider 的 HTTP/JSON/SSE 实现细节。模板把"样板"封装，贡献者只需复制 + 替换 `TODO`：

```
1. cp provider_template.go {your_provider}_provider.go
2. 全局替换 "template" / "Template" → 你的 provider 名称
3. 实现 Complete() / Stream() / CallTools() / Info()
4. 删除 TODO 注释
5. go test -run TestTemplate ./internal/llm/
```

模板封装了**所有可复用部分**：
- `Config` 字段标准化（APIKey / BaseURL / Model / Timeout）
- `ResolveModel` / `ResolveTemperature` 助手
- `doRequest()` 助手：marshal → POST → 状态码检查 → 错误解析 → 限流读取
- `ModelInfo` 返回结构

贡献者只需实现**4 个核心方法**。

### 风险

1. **模板是空实现**。`TemplateProvider.Complete()` 等返回 `fmt.Errorf("TODO: 未实现")`——任何人**误把 `TemplateProvider` 当真 Provider 用**会运行时 panic（错误信息明确，但不优雅）。
   - **Phase 7 行动**：在 `NewTemplateProvider` 启动时 panic，或加 `// DO NOT USE — TEMPLATE ONLY` 包级警告。

2. **未覆盖的特殊 Provider 形态**：
   - Anthropic（自定义 SSE / 工具调用格式）
   - Gemini（多模态专用结构）
   - Ollama（本地 HTTP、无 Auth）
   - 模板假设 **OpenAI 兼容 HTTP + Bearer Auth**，对其他形态需重写 `doRequest()`

3. **测试覆盖的是模板结构而非真实交互**。`provider_template_test.go` 259 行，主要是注册/导出/方法签名断言，不验证 HTTP 协议。

---

## 子阶段 6-C：仓库结构重组

### 任务清单

| # | Task | 复杂度 | 说明 |
|:-:|------|:------:|------|
| C1 | `docs/*` → `ecosystem/docs/*` 迁移 | ⭐ | 11 个文件（Git rename） |
| C2 | `examples/go/*` → `ecosystem/examples/*` 迁移 | ⭐ | 13 个目录（Git rename） |
| C3 | `ecosystem/plugins/` 6 个插件 + 注册表 | ⭐⭐ | email / git / http / json / kv / sql |
| C4 | `ecosystem/templates/` 3 个脚手架 | ⭐ | basic / with-tools / multi-agent |
| C5 | `ecosystem/contributing/` 指南 | ⭐ | PLUGIN.md / PROVIDER.md |
| C6 | `cmd/ap/scaffold/main.go` | ⭐ | `ap init` 实现 |
| C7 | `Makefile` 路径更新 | ⭐ | `run-hello` / `run-multi` / `run-production` |

**总规模**：47 个新增文件 + 26 个迁移文件

### 重组后的目录角色

```
agentprimordia/
├── cmd/           ← CLI 入口（不变）
├── internal/      ← 核心实现（不变）
├── operator/      ← K8s Operator（不变）
├── pkg/           ← 公共 API（不变）
├── bench/         ← 基准测试（不变）
├── sdk/           ← 多语言 SDK（不变）
├── deploy/        ← 部署配置（不变）
├── test/          ← 集成测试（不变）
└── ecosystem/     ← 生态聚合（新增）
    ├── docs/      ← 11 篇文档 + CODE_WIKI
    ├── examples/  ← 20 个示例（含 chain-* 新例）
    ├── plugins/   ← 6 个开箱即用工具插件
    ├── templates/ ← 3 个 `ap init` 脚手架
    └── contributing/  ← PLUGIN.md + PROVIDER.md
```

**`internal/` vs `ecosystem/` 边界**：
- `internal/` 改动 = breaking change
- `ecosystem/` 改动 = 自由添加，按需 SemVer

### 6 个插件的状态

| 插件 | 工具数 | 测试 | 备注 |
|------|:------:|:----:|------|
| `email` | — | ❌ | 仅有 `plugin.go`，无测试、无 `plugin_test.go` |
| `git` | — | ❌ | 同上 |
| `http` | 1 (`http_client`) | ❌ | 需实际查看是否实现 |
| `json` | — | ❌ | 同上 |
| `kv` | — | ✅ | `plugin_test.go` 存在 |
| `sql` | 1 (`sqlite_processor`) | ❌ | — |

> **Phase 7 待办**：6 个插件中只有 1 个有测试。需补齐 `email` / `git` / `http` / `json` / `sql` 的单元测试。

### `ap init` 脚手架流程

```
ap init my-agent
    ↓
读 ecosystem/templates/<selected>/
    ↓
替换占位符 {{.Name}} / {{.Module}} / {{.Date}}
    ↓
写入 my-agent/main.go + go.mod
```

> 当前 `cmd/ap/scaffold/main.go` 仅 28 行——**实际复制/模板渲染逻辑**可能尚未实现或较简陋，Phase 7 需要 e2e 验证。

---

## 子阶段 6-D：公共 API 收尾

### 任务清单

| # | Task | 复杂度 | 文件 |
|:-:|------|:------:|------|
| D1 | 暴露 `PluginLoader` / `NewPluginLoader` | ⭐ | `pkg/tools.go` |
| D2 | 暴露 `ToolPlugin` / `PluginInfo` 类型别名 | ⭐ | `pkg/tools.go` |

**总规模**：11 行新增

### 暴露策略（待 Phase 7 决策）

当前 8 个"暴露到 pkg/"提交都遵循：
```go
// pkg/tools.go
NewPluginLoader = tools.NewPluginLoader
type ToolPlugin = tools.ToolPlugin
type PluginLoader = tools.PluginLoader
type PluginInfo = tools.PluginInfo
```

**问题**：
- 没有 godoc 标注稳定性（`// Stable:` / `// Experimental:` / `// Deprecated:`）
- 没有 `doc.go` 总览，告知用户 `pkg/` 哪些是 stable、哪些会变
- 移除周期、API 兼容承诺均无书面规定

**Phase 7 候选**：见 §后续工作 §1。

---

## 模块边界更新

### 旧 AGENTS.md 描述（已不准）

```
internal/
├── agent/      — ReActLoop 引擎核心（不依赖 pool/memory）
├── pool/       — 多Agent调度（依赖 agent, tools）
├── tools/      — 工具系统（独立模块）
├── memory/     — 记忆存储（独立模块）
├── llm/        — LLM抽象层（最底层，被 agent 依赖）
└── concurrency/— 并发原语（FileLock 等，被 pool/tools 依赖）
```

### 实际依赖图（重构后）

```
        ┌────────────────────────────────────────┐
        │           agent/  (顶层)               │
        │   引用 llm, memory, persist, tools    │
        └────┬───────┬───────┬───────────┬──────┘
             │       │       │           │
        ┌────▼─┐ ┌───▼──┐ ┌──▼───┐ ┌────▼────┐
        │ llm  │ │memory│ │persist│ │  tools  │
        └──────┘ └──────┘ └───────┘ └────┬────┘
                                          │
                                     ┌────▼────┐
                                     │  pool   │
                                     └─────────┘
```

`agent/` 实际上是**顶层模块**（依赖其它 4 个），而不是 AGENTS.md 描述的"不依赖 pool/memory 的引擎核心"。

**Phase 7 行动**：
- 选项 A：改 AGENTS.md 明确 `agent/` 处于顶层，可依赖 llm/memory/persist/tools
- 选项 B：把 `*Config` 类（含 `RAGConfig` / `HITLConfig` / `*Capable` 接口）下移到 `internal/agent/configs/`，让 `agent/core/` 保持不依赖其它包
- 推荐 A（侵入小、忠实反映现状）

---

## 验证结果

### 构建

| 命令 | 结果 |
|------|------|
| `go build ./...` | ✅ 通过（0 错误 0 警告） |
| `go vet ./...` | ✅ 通过 |
| `go test ./internal/agent/ ./internal/llm/ ./pkg/...` | ✅ 全部通过（17s / 45s / 0.06s） |
| `go test -race ./...` | ⚠️ 跳过（Windows 环境无 gcc，CGO 不可用） |

### 测试覆盖率

未在 Phase 6 引入覆盖率门槛。建议 Phase 7 加 `Makefile` 的 `test-cover` 目标，门槛设为：
- 核心包（`internal/agent/` / `internal/llm/` / `internal/pool/`）：行覆盖 ≥ 80%
- 整体：行覆盖 ≥ 70%

### 文件规模

| 模块 | 新增 | 迁移 | 删除 |
|------|:----:|:----:|:----:|
| `internal/agent/` | +1100 行 | — | — |
| `internal/llm/` | +440 行 | — | — |
| `pkg/` | +11 行 | — | — |
| `ecosystem/` | +47 文件 | — | — |
| `docs/` / `examples/go/` | — | -26 文件 | — |

总计 4 个 commit，约 1500+ 行新代码，~30 个新测试用例。

---

## 风险与债务

### 高优先级（Phase 7 必须解决）

1. **公共 API 无稳定性标注**。`pkg/` 全部 export 既无 `// Stable:` 也无 `// Experimental:`，用户拿到的 API 等于没有承诺。
2. **`ReActConfig` 字段废弃时间表无书面记录**。本文档 §子阶段 6-A 给出了建议，但**未在代码 godoc 中体现**。需在每个 `Deprecated:` 字段上加 `// Removed in v2.0.`
3. **`TemplateProvider` 误用风险**。`NewTemplateProvider` 不拒绝调用——任何人 copy-paste 后忘了改 TODO，运行时炸出 `TemplateProvider.Complete: TODO: 未实现`。
4. **6 个插件仅 1 个有测试**。`email` / `git` / `http` / `json` / `sql` 无单元测试。
5. **`ap init` 脚手架未 e2e 验证**。`cmd/ap/scaffold/main.go` 仅 28 行，模板渲染/文件复制可能未完成或简陋。

### 中优先级

6. **AGENTS.md 模块边界描述与代码不符**（§模块边界更新）。
7. **README 链接**指向已迁移的 `docs/api-reference.md` 等路径，需改为 `ecosystem/docs/...`。
8. **废弃字段与链式 API 长期双轨**。两个小版本窗口内并存，维护负担。

### 低优先级

9. **`ecosystem/` 子目录在 Go module 路径上的处理**。`agentprimordia/ecosystem/plugins/http` 等是合法 import path，但和核心包视觉上无差别，可能让人误以为是 core。建议在 `ecosystem/README.md` 显式声明 import 约定。
10. **新 `ap init` 与旧 `ap init` 命令兼容**。`cmd/ap/scaffold/main.go` 是新文件，但 `cmd/ap/` 旧 init 是否还存在？未排查。

---

## 后续工作候选（Phase 7+）

1. **公共 API SemVer 策略**
   - `pkg/doc.go` 总览
   - 每个 export 加 `// Stable:` / `// Experimental:` / `// Deprecated:` 标注
   - CHANGELOG.md 规范
   - 版本号策略（v0.x 允许小幅 breaking，v1.0 起 SemVer 严格）

2. **覆盖率门槛 + CI**
   - Makefile `test-cover` 目标
   - GitHub Actions 跑覆盖率检查
   - 核心包 ≥ 80%、整体 ≥ 70% 门槛

3. **测试补齐**
   - 6 个插件补单元测试
   - `TemplateProvider` 模板使用文档 e2e 测试
   - `ap init` 脚手架生成代码的可编译性测试

4. **文档站拆分**
   - `ecosystem/docs/` 拆为独立站点（`ap-docs/` 子项目 + Hugo/Docusaurus）
   - 现在 README + ecosystem/docs 双层结构，搜索引擎收录差

5. **示例工程化**
   - `ecosystem/examples/*` 改为独立 go.mod（每个示例独立可运行）
   - 现在的结构是 monorepo 子目录，`go run` 仍可，但 `go.mod` 解析缓慢

6. **废弃字段治理**
   - 给 `ReActConfig` 中 9 个 `Deprecated:` 字段加 `// Removed in v2.0.`
   - v0.x 末期（至少 2 个小版本）后升级为 panic
   - v2.0 移除

---

## 反思：Phase 6 暴露的过程问题

> **本节是元层面的，不属于实施内容**

1. **代码先行、文档后补是高风险节奏**。Phase 6 的 4 个 commit 没有 plan 文档，事后反推才能写成本文件。如果 1 个月后有人来 review，会陷入"代码已是既定事实"的窘境。
2. **提交粒度过细时失去节奏感**。8 个 commit 里有 5 个是"expose X to pkg/"——这些本可以合并为 1 个 `feat(pkg): stabilize public API surface` 提交。
3. **PR/Plan → Code → Test → Doc 的顺序未被强制**。`AGENTS.md` 提到 `docs/plans/2026-05-27-agentprimordia-implementation.md`，但没有要求每个 Phase 必须有 spec。
4. **废弃标记缺少"何时移除"**。`// Deprecated:` 没有 `// Removed in v2.0.`，用户不知道能用到什么时候。

**Phase 7 建议流程改进**：
- 任何新 Phase 必须先有 `docs/plans/YYYY-MM-DD-phaseN-implementation.md`
- 提交 message 引用 plan 中的 Task 编号（如 `feat(agent): A2 capability interface (Phase 7/Task-3)`）
- 任何 `// Deprecated:` 标注必须包含 `// Removed in vX.Y.`

---

## 后记:Phase 6.5 治理收尾 (2026-06-05)

> 本节在 Phase 6 完成后由治理工作追加，记录**计划文档与代码不一致**的 3 处发现。

### 实际发生:Phase 6.5 9 个治理点

Phase 6 计划文档列出的 5 高 + 3 中 + 2 低优先级债务,2026-06-04 治理收尾为 9 个提交落地。

| # | 点 | 提交 | 文档预期 | 实际发现 |
|:-:|----|------|---------|---------|
| 1 | pkg/ API 稳定性 | `5e8557a` | 19 文件 | ✅ 一致 |
| 2 | ReActConfig 字段废弃 | `9391f32` | 9 字段 | ⚠️ 实际 14 字段 |
| 3 | TemplateProvider 误用防护 | `deb543c` | 启动期 panic | ✅ 落地 |
| 4 | 6 插件补测试 | `7d09fd6` | 5 插件缺测试 | ⚠️ 实际仅 4 插件缺(email 250 行已有) |
| 5 | ap init 实现 + e2e | `4e0daf5` | 28 行简陋 | ⚠️ 实际 init.go 已完整,scaffold/main.go 是孤儿模板源 |
| 6 | AGENTS.md 可见性 | `2da7726` | 仓库外 .gitignore | ✅ 选 CONTRIBUTING.md 同步方案 |
| 7 | 迁移指南 | (合并到 #2) | docs/migration/ | ✅ 与 #2 同提交 |
| 8 | ecosystem/ import 约定 | `a881cc1` | ecosystem/README.md | ✅ 新建 |
| 9 | ap init 新旧命令排查 | `9a2aa9e` | 排查冲突 | ⚠️ 实际无冲突,但发现 go.mod 路径错 |

### 计划文档与代码不一致的 3 处

#### 1. 14 vs 9 字段 (点 2)

计划文档 §风险与债务 §2 写 "9 个 `Deprecated:` 字段",实际 react_loop.go 中有 14 个:
- 计划漏数了: `EventPublisher / Metrics / ContextWindow / Hooks / Lifecycle / Logger / HITL` 等
- `Lifecycle` 与 `Logger` 未标 Deprecated(它们是"默认"而非"能力"),所以"可废弃"的是 14 个
- 提交 `9391f32` 已正确处理 14 个,迁移指南同步更新

教训: **数字类估计需用 `grep | wc -l` 实际计数**,不靠目测。

#### 2. 5 vs 4 插件 (点 4)

计划文档 §风险与债务 §4 写 "6 插件仅 1 个有测试",实际:
- `email` (250 行) 在 phase6 重构提交 `070d3a3` 已包含测试
- `kv` (356 行) 同样
- 真正缺测试的是 4 个:git / http / json / sql

教训: **做风险评估前先 `ls ecosystem/plugins/*/plugin_test.go`**,不靠 commit message 印象。

#### 3. 28 行简陋 vs 完整 (点 5 / 9)

计划文档 §风险与债务 §5 写 "`cmd/ap/scaffold/main.go` 仅 28 行,模板渲染/文件复制可能未完成或简陋",实际:
- `cmd/ap/scaffold/main.go` 是**嵌入资源**(被 `//go:embed scaffold/*` 包含)
- 真正的脚手架命令是 `cmd/ap/init.go` (runInit 函数)
- init.go 已实现模板复制 + 变量替换 + .ap.yaml 生成 + 3 个模板

但**e2e 测试发现真问题**: 生成的项目**没有 go.mod**, `go build` 直接失败。
- 修复 (点 5): 加 go.mod 生成
- 二次发现 (点 9): replace 路径 `../agentprimordia` 错(应 `..`)
- 二次发现 (点 9): `scaffold/main.go` 孤儿文件被 embed 包含(字典序巧合避免 bug)

教训: **风险评估不能只读代码表面,必须实际跑一遍端到端**。

### 治理总规模

- **9 个 commit**, 全部 Phase 6.5.x 标记
- **代码变动**: ~700 行 godoc/测试/配置
- **新文档**: 4 个 (SemVer spec / migration guide / ecosystem README / examples README)
- **删除**: 1 个孤儿文件
- **实际工时**: 一天
- **总提交数**: 18 个 commit 领先 origin/main (10 个 Phase 6.5 治理 + 8 个原 Phase 6 实施)

### 流程改进的"被采纳"情况

Phase 6 反思建议的 3 项流程改进:
- ✅ **先 plan 后 commit**: Phase 6.5 实施时**没有 plan 文档**(再次违反)→ Phase 7 改正
- ✅ **`// Deprecated:` 必含 `// Removed in vX.Y.`**: Phase 6.5.2 落实
- ⚠️ **commit message 引用 Task 编号**: 仅 Phase 6.5 标记,未细化到 Task 级

**仍待改进**: Phase 7 计划文档 (7.x) 落地后,看是否能严格执行"plan 先行"。


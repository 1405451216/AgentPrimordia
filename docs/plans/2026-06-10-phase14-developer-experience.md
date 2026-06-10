# Phase 14: 开发者体验优化 — 实施计划

> **日期**: 2026-06-10
> **状态**: ✅ 已完成
> **前置条件**: Phase 6 协议式微内核 + Phase 7 公共 API 收尾
> **目标**: 消除「从零到第一个 Agent」的高频摩擦，提升日常开发体验

---

## 背景

站在专业 Agent 开发者视角，对 AP 框架进行全量 DX 审查后，识别出 3 个 P0（高频摩擦）问题和 4 个 P1（中频摩擦）问题。本 Phase 聚焦 P0，力争一个 commit 解决最大的痛点。

## 子阶段清单

| # | 子阶段 | 复杂度 | 描述 |
|:-:|--------|:------:|------|
| 14-A | `testutil` 包：MockProvider + NewTestAgent() | ⭐ | 提供开箱即用的测试用 LLM |
| 14-B | `NewAgent()` 简化入口 | ⭐⭐ | 消除 ReActConfig 14 个废弃字段的视野污染 |
| 14-C | `WithRAGMemory(mem, provider)` 自动适配 | ⭐ | 从 6 步降到 2 步 |
| 14-D | 示例清理 + 死代码清理 | ⭐ | 更新所有示例使用新 API，移除废弃适配器引用 |

> **设计原则**：只做减法不做加法。不引入新概念，不要求用户学习新 API。每个改动要么减少代码行数，要么减少认知负担。

---

## 14-A：testutil 包

### 当前状态

`cmd/example/demo/demo_llm.go` 已有 `DemoLLM`，功能非常完备（预设响应序列、工具调用、延迟、错误注入、调用计数）。但它位于 `cmd/example/demo/`（示例目录），不属于公共 API，外部开发者无法 `import`。

所有示例和内部测试都直接使用 `demo.NewDemoLLM()`，而生态示例（`ecosystem/examples/*`）则各自手写了不同类型的 MockLLM。

### 方案

**不写新 MockLLM**。将现有的 `DemoLLM` 从 `cmd/example/demo/` 搬迁到新建的 `testutil/` 包，作为 `MockProvider`，并添加 `NewTestAgent()` 快捷构造器。原有的 `cmd/example/demo/` 改为薄包装（type alias 指向 testutil）。

### 文件清单

| 文件 | 操作 | 描述 |
|------|:----:|------|
| `testutil/provider.go` | **新建** | `MockProvider`（当前 DemoLLM 的搬迁）+ `NewTestAgent()` |
| `cmd/example/demo/demo_llm.go` | **修改** | 改为 `type DemoLLM = testutil.MockProvider` 别名（向后兼容） |
| `testutil/provider_test.go` | **新建** | MockProvider 单元测试 |

### MockProvider API

```go
package testutil

import "agentprimordia/internal/llm"

type MockProvider struct { ... } // 原名 DemoLLM

// NewMockProvider 创建预设响应的 mock LLM
func NewMockProvider(responses ...string) *MockProvider

// WithToolCalls 设置工具调用响应序列
func (m *MockProvider) WithToolCalls(calls ...llm.FunctionCall) *MockProvider

// WithDelay 设置每次调用的延迟
func (m *MockProvider) WithDelay(d time.Duration) *MockProvider

// WithError 设置错误注入
func (m *MockProvider) WithError(err error) *MockProvider

// CallCount 返回 Complete + CallTools 的总调用次数
func (m *MockProvider) CallCount() int

// ===== NewTestAgent 快捷构造器 =====

// TestAgent 是预配置了 MockProvider 的测试用 Agent
// 等价于传统 NewReActAgent + 设置 Model 为 MockProvider
type TestAgent = agent.CapabilityAgent

// TestAgentConfig 是 NewTestAgent 的配置
type TestAgentConfig struct {
    Name         string
    SystemPrompt string
    Responses    []string
    MaxTurns     int
}

// NewTestAgent 创建一个带 MockProvider 的测试用 Agent
func NewTestAgent(cfg TestAgentConfig) *TestAgent
```

### 验收标准

- `go test ./testutil/...` 通过
- 所有现有测试仍通过（`DemoLLM` 别名向后兼容）
- `testutil.NewTestAgent()` 不依赖任何外部包

---

## 14-B：NewAgent() 简化入口

### 当前状态

`ReActConfig` 结构（[react_loop.go:57-155](file:///e:/codecast(3)/codecast/AgentPrimordia/agentprimordia/internal/agent/react_loop.go#L57-L155)）包含 20+ 字段，其中 14 个标记 `Deprecated`。新开发者在 IDE 中看到 70% 的字段被划了删除线，困惑"到底该填哪些"。

虽然链式 API（`WithMemory`/`WithRAG`/…）已存在，但默认构造方式仍是填充 struct 字段，与链式 API 并存造成双路径混淆。

### 方案

**不拆 `ReActConfig`**（避免破坏性变更）。新增一个精简入口函数 `NewAgent()`：

```go
// NewAgent 是创建 Agent 的推荐入口，只暴露核心必填字段。
// 能力（Memory/RAG/Hooks/Tracer等）通过链式 API 注入。
func NewAgent(name, systemPrompt string, model llm.Provider, opts ...AgentOption) *CapabilityAgent
```

其中 `AgentOption` 是函数式选项（`WithMaxTurns()`/`WithTemperature()`/`WithSessionID()`），只覆盖核心可选参数。能力注入仍然用链式 API（`agent.WithMemory(mem).WithRAG(rag)` 等）。

### 文件清单

| 文件 | 操作 | 描述 |
|------|:----:|------|
| `internal/agent/new_agent.go` | **新建** | `NewAgent()` + `AgentOption` + `WithMaxTurns/WithTemperature/WithSessionID` |
| `internal/agent/new_agent_test.go` | **新建** | NewAgent 单元测试 |

### API 对比

```go
// 旧（14 个被划删除线的字段暴露在视野中）：
agent := ap.NewReActAgent(ap.ReActConfig{
    Name:         "my-bot",
    SystemPrompt: "you are helpful",
    Model:        provider,
    MaxTurns:     10,
    Temperature:  0.7,
})

// 新（只有核心参数，Deprecated 字段不可见）：
agent := ap.NewAgent("my-bot", "you are helpful", provider,
    ap.WithMaxTurns(10),
    ap.WithTemperature(0.7),
).WithMemory(mem).WithRAG(rag)
```

### 验收标准

- `NewAgent` 不暴露任何 `ReActConfig` 内部字段
- 与现有 `NewReActAgent` 功能等价（ChainAPI 兼容）
- 所有现有测试无需修改

---

## 14-C：WithRAG 自动适配

### 当前状态

启用 RAG 需要用户手动组装 6 个对象：

```
NewSQLiteStore → NewVectorStore → NewEmbeddingAdapter → NewRAGStore → NewRAGProviderAdapter → WithRAG(RAGConfig{Provider: ...})
```

### 方案

在 `CapabilityAgent` 上新增一个重载的 `WithRAGMemory()` 方法，接受 `memory.Memory` + `llm.Provider`（或已适配的 `memory.EmbeddingProvider`），内部自动完成 RAGStore 创建和适配。

```go
func (a *ReActAgent) WithRAGMemory(mem memory.Memory, emb memory.EmbeddingProvider) *CapabilityAgent
```

### 文件清单

| 文件 | 操作 | 描述 |
|------|:----:|------|
| `internal/agent/chain_api.go` | **修改** | 新增 `WithRAGMemory()` 方法 |
| `internal/agent/chain_api_test.go` | **修改** | 新增测试 |

### 验收标准

- 原有 `WithRAG(RAGConfig{...})` 仍可用
- `WithRAGMemory(mem, emb)` 等价于手动 6 步组装
- 单元测试覆盖

---

## 14-D：示例清理 + 死代码清理

### 清理项

| # | 文件 | 操作 | 描述 |
|:-:|------|:----:|------|
| 1 | `ecosystem/examples/basic/main.go` | **修改** | 用 `NewAgent()` 替代 `NewReActAgent()`；MockLLM 替换为 `testutil.NewMockProvider` |
| 2 | `ecosystem/examples/with-tools/main.go` | **修改** | `NewMemoryAdapter(memory)` → `memory`（无操作适配器）；`NewAgent` + 链式 API |
| 3 | `ecosystem/examples/multi-agent/main.go` | **修改** | 手写 MockLLM → `testutil.NewMockProvider`；`NewAgent` |
| 4 | `ecosystem/examples/simple/main.go` | **修改** | `agent.NewReActAgent(agent.ReActConfig{...})` → `ap.NewAgent(...)` |
| 5 | `ecosystem/examples/chain-api/` | **修改** | 如果存在，同步更新 |
| 6 | `ecosystem/examples/memory-backends/` | **修改** | DemoLLM → testutil.NewMockProvider |
| 7 | `cmd/ap/scaffold/basic/main.go` | **修改** | 模板中的 DemoLLM → testutil.NewMockProvider（脚手架模板） |
| 8 | 所有 `ecosystem/examples/*/main.go` | **检查** | 检查是否还有手写 MockLLM，如有则替换 |
| 9 | `pkg/adapters.go` | **检查** | `NewMemoryAdapter` 和 `NewMetricsAdapter` 标记 Deprecated 的注释中更新说明 |

### 验收标准

- 所有示例编译通过、运行通过
- 任何示例中不再出现「手写 MockLLM 实现 Provider 接口」
- `go test ./...` 零失败

---

## 兼容性承诺

- `NewReActAgent(ReActConfig{...})` **不删除、不标记 Deprecated**，仅新增推荐入口
- `DemoLLM` 保持向后兼容（type alias 指向 testutil）
- 所有现有测试代码不受影响

---

## 风险与债务

| 风险 | 缓解 |
|------|------|
| `NewAgent` 与 `NewReActAgent` 并存可能造成新的混淆 | 在 `NewAgent` 文档注明"推荐入口"，`NewReActAgent` 文档注明"低级 API，当前仍可用" |
| `MockProvider` 从 `cmd/example/demo` 移到 `testutil` 后，`demo` 包变成空壳 | 保留 `demo_llm.go` 中的 type alias，已有引用不受影响 |

---

## 实际结果

- `testutil/` 新增 16 个测试
- `internal/agent/` 新增 6 个测试
- 7 个示例文件简化
- 8 个文档文件同步
- 总计 +1719 / -549 行，34 个文件

---

## 验证

- `go build ./...` ✅ 零错误
- `go vet ./...` ✅ 零错误
- `go test ./...` ✅ 全包通过

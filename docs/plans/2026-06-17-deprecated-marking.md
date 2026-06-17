# Deprecated 标记实施计划

> **日期**: 2026-06-17
> **状态**: Approved
> **版本**: 待定（用户选择"先做 Deprecated 再说"）
> **前置**: v0.7.0 API 稳定化已完成（commit 767c7ba）
> **后续阻塞**: v1.0.0（ReActConfig 字段 panic if non-nil）

## 1. 背景与动机

v0.7.0 API 稳定化已完成，引入了 `NewAgent` + Functional Options 作为推荐入口。
按照废弃时间表（v0.7.0 设计文档 §6），下一步是正式标记旧 API 为 Deprecated，
让 IDE 和 `go vet` 在用户使用旧 API 时产生编译期 warning。

## 2. 当前状态审计

### 已完成 ✅
- `ReActConfig` 的 14 个能力字段已标记 `// Deprecated:` + `// Removed in v2.0.`
  - Toolkit, Memory, EventPublisher, Metrics, ContextWindow, CheckpointStore,
    RAG, Hooks, Summarizer, FileScope, HITL, CostTracker, Tracer, Cache
- `pkg/agent.go` 的 `ReActConfig` 类型别名有 Stability 标注和迁移指南指针

### 未完成 ❌
- `internal/agent.NewReActAgent` 函数未标记 `// Deprecated:`
- `pkg/agent.go` 的 `NewReActAgent` 变量未标记 `// Deprecated:`

## 3. 实施任务

### Task 1: 标记 NewReActAgent 为 Deprecated

**文件**:
- `agentprimordia/internal/agent/react_loop.go` — `NewReActAgent` 函数
- `agentprimordia/pkg/agent.go` — `NewReActAgent` 变量别名

**修改内容**:

`internal/agent/react_loop.go`:
```go
// NewReActAgent creates a new ReAct-based agent
//
// Deprecated: 使用 NewAgent 代替。NewReActAgent 暴露了 14 个已废弃的 ReActConfig 字段，
// 容易导致误用。NewAgent 通过 Functional Options 注入能力，构造后不可变。
// NewReActAgent 将在 v2.0.0 移除。
// 迁移指南: ecosystem/docs/migration/v0-deprecations.md
func NewReActAgent(cfg ReActConfig) *ReActAgent {
```

`pkg/agent.go`:
```go
var (
    // NewReActAgent 创建基于 ReAct 循环的 Agent 实例
    //
    // Deprecated: 使用 NewAgent 代替。NewReActAgent 暴露了 14 个已废弃的 ReActConfig 字段，
    // 容易导致误用。NewAgent 通过 Functional Options 注入能力，构造后不可变。
    // NewReActAgent 将在 v2.0.0 移除。
    // 迁移指南: ecosystem/docs/migration/v0-deprecations.md
    NewReActAgent = agent.NewReActAgent
```

### Task 2: 验证编译期 warning 生效

运行 `go vet` 确认 Deprecated 标记被识别。在测试文件中临时使用 NewReActAgent，
确认 `go vet` 产生 `deprecated` warning。

### Task 3: 更新 CHANGELOG

在 CHANGELOG.md 的 `[Unreleased]` 节添加 Deprecated 标记记录。

## 4. 验收标准

1. `go build ./...` 零错误
2. `go vet ./...` 无警告（项目内已无 NewReActAgent 直接调用）
3. `go test ./...` 全部通过
4. `internal/agent.NewReActAgent` 有 `// Deprecated:` godoc 标记
5. `pkg/agent.go` 的 `NewReActAgent` 有 `// Deprecated:` godoc 标记
6. CHANGELOG 已更新

## 5. 风险

- **低风险**: 仅添加注释，不改变任何运行时行为
- **向后兼容**: NewReActAgent 仍可用，只是产生编译期 warning

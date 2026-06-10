# Phase 15: 接口与体验精细化 — 实施计划

> **日期**: 2026-06-10
> **状态**: 执行中
> **前置条件**: Phase 14 DX 优化 + Phase 14 文档同步已推送
> **目标**: 解决「写完原型开始深耕」阶段体验问题，消除二次开发摩擦

---

## 背景

Phase 14 解决了「从零到第一个 Agent」的高频摩擦。但留下来 4 个深耕阶段的摩擦：

1. 每个 Capability 都要写一遍适配器样板代码 → 泛型抽出来统一做
2. 跨包 5 层接口关系不清晰 → 概念文档
3. 多轮对话需要开发者手动管理会话上下文 → 便利层
4. 流式输出消费方式过时 → 迭代器模式

## 子阶段清单

| # | 子阶段 | 复杂度 | 描述 |
|:-:|--------|:------:|------|
| 15-A | 概念文档：跨包接口图 | ⭐ | 绘制 5 层接口关系图，写 1 页决策说明 |
| 15-B | 通用适配工具：泛化抽离 | ⭐⭐ | 统一适配函数代替散落在每个 WithXxx 中的样板 |
| 15-C | Session 会话管理便利层 | ⭐⭐ | `agent.NewSession(agent, memory)` + `sess.Ask(msg)` |
| 15-D | 流式输出迭代器适配 | ⭐ | Go 1.23 `iter.Seq2[Chunk, error]` 风格流式消费 |

---

## 15-A：概念文档：跨包接口图

### 当前问题

新开发者遇到 `memory.Memory` vs `agent.MemoryStore` vs `llm.Embedder` vs `memory.EmbeddingProvider` vs `agent.RAGProvider` 会困惑：

> 我该怎么从 `X` 得到 `Y`？到底要不要包一层？

### 方案

新增 `docs/concepts/interface-graph.md`，包含：

1. 模块边界回顾（`internal/` 依赖图 → 这张图已经有了）
2. 每个接口「是给谁用的、怎么来、怎么去」
3. 「我手头有 X，需要 Y 怎么办」决策表
4. 适配原则：框架自动适配，开发者不需要手工写

### 文件清单

| 文件 | 操作 | 描述 |
|------|:----:|------|
| `docs/concepts/interface-graph.md` | **新建** | 接口关系图 + 决策表 |

### 验收标准

- 新开发者看了以后能说出「每个接口在哪个模块负责什么」
- 能直接回答「需要 RAGProvider，手头有 Memory + LLM，怎么做」

---

## 15-B：通用适配工具：泛化抽离

### 当前问题

每个 `WithXxx` 方法都有类似的模式：

```go
// 接受 interface X，需要 *Y 类型，要检查 nil → 类型断言 → new Y(x)
// 散落在 WithMemory/WithRAG/WithEvents/... 每个地方都写一遍
```

### 方案

新增 `internal/agent/adapt.go`，泛型实现通用适配：

```go
// TryAdapt 尝试将 cap interface{} 适配为 T。
// 如果 cap 已经是 *T，直接返回 cap。
// 如果 cap 已经是 T（值类型），返回 &cap。
// 如果 cap 是 nil，返回 nil（表示不配置该能力）。
// 如果都不匹配，返回 ErrIncompatibleCapability。
func TryAdapt[T any](cap interface{}) (*T, error)
```

然后把 `WithMemory`/`WithRAG`/`WithEvents`/`WithMetrics` 里面的样板代码替换为 `TryAdapt`。

### 文件清单

| 文件 | 操作 | 描述 |
|------|:----:|------|
| `internal/agent/adapt.go` | **新建** | `TryAdapt` 泛型函数 |
| `internal/agent/adapt_test.go` | **新建** | 单元测试覆盖空值/直接指针/值类型 场景 |
| `internal/agent/capability_agent.go` | **修改** | 替换各个 WithXxx 的适配样板为 `TryAdapt` |

### 验收标准

- 现有行为完全不变
- 行数减少 ≈ 20 行（多个 WithXxx 去掉样板）
- `go test` 全部通过

---

## 15-C：Session 会话管理便利层

### 当前问题

多轮对话需要开发者：

1. 手动把消息历史拼接到 Memory
2. 手动维护 SessionID
3. 每次 `agent.Run` 要传新的 `UserMessage` 进去

```go
// 当前
resp1, err := agent.Run(ctx, ap.UserMessage("first"))
resp2, err := agent.Run(ctx, ap.UserMessage("second"))  // 漏了上下文！
```

### 方案

新增 `Session` 结构体：

```go
// Session 维护多轮对话上下文，自动追加历史到记忆。
type Session struct {
    agent     *CapabilityAgent
    memory    memory.Memory
    sessionID string
    lastTurn  int
}

// NewSession 创建新会话。
// 如果 mem == nil，使用 agent 已配置的内存。
func NewSession(agent *CapabilityAgent, mem memory.Memory, opts ...SessionOption) *Session

// Ask 发送一轮用户消息，返回响应，自动追加历史。
func (s *Session) Ask(ctx context.Context, userMessage string) (*ap.Response, error)

// LastResponse 返回上一轮响应。
func (s *Session) LastResponse() *ap.Response

// TurnCount 返回已完成轮次。
func (s *Session) TurnCount() int

// Reset 重置会话（不清空底层记忆）。
func (s *Session) Reset()
```

### 文件清单

| 文件 | 操作 | 描述 |
|------|:----:|------|
| `internal/agent/session.go` | **新建** | `Session` 结构体 + 方法 |
| `internal/agent/session_test.go` | **新建** | 单元测试 |
| `pkg/agent.go` | **修改** | 导出 `Session` + `NewSession` |

### 验收标准

- 不依赖额外状态，直接复用 agent 已配置的内存能力
- 示例：用 3 行代码搞定 3 轮对话
- 向后兼容：现有用法不受影响

---

## 15-D：流式输出迭代器适配

### 当前问题

现在流式输出返回 `<-chan Chunk`：

```go
ch, err := agent.Stream(ctx, msg)
for chunk := range ch {
    // ...
}
```

Go 1.23 推荐 `iter.Seq2[Chunk, error]`：

```go
for chunk, err := range agent.StreamSeq(ctx, msg) {
    // ... 写法更一致，错误处理更自然
}
```

### 方案

在 `ReActAgent` 上新增 `StreamSeq` 方法，适配为 `iter.Seq2`：

```go
// StreamSeq 返回流式输出迭代器（Go 1.23+ 风格）。
// 用法:
//
//  for chunk, err := range agent.StreamSeq(ctx, ap.UserMessage("hi")) {
//      if err != nil { return err }
//      fmt.Print(chunk.Content)
//  }
func (a *ReActAgent) StreamSeq(ctx context.Context, msg ap.Message) iter.Seq2[ap.Chunk, error]
```

原有 `Stream` 方法（返回 channel）仍然保留。

### 文件清单

| 文件 | 操作 | 描述 |
|------|:----:|------|
| `internal/agent/react_loop.go` | **修改** | 新增 `StreamSeq` 方法 |
| `pkg/agent.go` | **修改** | 导出 `ap.Chunk`（已导出，这里只确保兼容） |

### 验收标准

- 原有 `Stream` 仍然工作
- 新增 `StreamSeq` 不改变现有算法，仅做迭代器适配
- 兼容性：Go < 1.23 也能编译，不导入 `iter` 除非 Go 版本支持

---

## 兼容性承诺

- 所有原有 API **不删除、不改变签名**
- 仅新增 API + 重构内部样板代码
- 重构后功能等价，原有测试全部通过

---

## 工作量估计

| # | 耗时 |
|:-:|------:|
| 15-A | 0.5 天 |
| 15-B | 0.5 天 |
| 15-C | 1 天 |
| 15-D | 0.5 天 |
| **合计** | **~2.5 天** |

---

## 风险与债务

| 风险 | 缓解 |
|------|------|
| `TryAdapt` 泛型是否能处理所有场景 | 覆盖空值/指针/值三种场景，完全覆盖现在用例 |
| Go 版本兼容性 | `iter` 包从 Go 1.23 引入，用 build tag 条件编译 |
| Session 实现是否会和 Memory 重复维护上下文 | Session 只做上下文拼接，不存储完整内容，完整内容仍然存在 Memory |

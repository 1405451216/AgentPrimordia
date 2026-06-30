# v0 → v1 迁移指南:ReActConfig 字段废弃

> **状态**: Active
> **生效版本**: v0.6.0 起
> **目标读者**: AgentPrimordia 框架使用者

本指南解释 `ReActConfig` 中 14 个字段的废弃原因、迁移路径和时间表。

## 为什么废弃?

`ReActConfig` 在 Phase 5 / Phase 6 演进过程中,字段膨胀到 16 个,出现以下问题:

1. **配置膨胀** — 大部分用户用不到这些字段,但每次 `NewReActAgent(ReActConfig{...})` 都要面对
2. **接口发现不一致** — 引擎需要 `if a.config.Memory != nil { ... }` 散落在 30+ 处
3. **可读性差** — 一个 Config 字面量无法体现"哪些能力被启用"
4. **生态扩展难** — 新能力要么污染 Config,要么绕过 Config 单独注入

Phase 6 引入**协议式微内核**(`*Capable` 接口 + `WithXxx` 链式 API)解决了上述问题,旧 Config 字段进入废弃流程。

## 废弃时间表

| 版本 | 行为 |
|------|------|
| **v0.6.0** | 字段加 `// Deprecated:` 标注,godoc 显示替代方案 |
| **v0.7.0** | 升级为编译期 warning(`go vet` 可检测) |
| **v0.8.0** | 字段仍可用,链式 API 为推荐入口 |
| **v1.0.0** (当前) | 若字段非 nil,运行时打印 `slog.Warn` 警告并自动回退到链式 API(不 panic) |
| **v2.0.0** (未来) | **字段已移除** — 编译期阻止误用 |

> **v1.0.0 行为变更说明**: 原计划在 v1.0.0 对废弃字段 `panic`,经生产用户反馈后改为
> `slog.Warn` 警告 + 优雅回退。理由: panic 会导致生产进程崩溃,不符合框架"生产优先"的承诺。
> 链式 API 注入的能力会自动覆盖 Config 字段,因此即便用户未迁移,功能仍能正常工作。
>
> `Lifecycle` 与 `Logger` **不在移除范围**(它们是"默认值"而非"能力",链式 API 不适合)。

## 14 个废弃字段映射表

| 字段 | 类型 | 替代链式方法 |
|------|------|------------|
| `Toolkit` | `*tools.Registry` | `.WithToolkit(reg)` |
| `Memory` | `MemoryStore` | `.WithMemory(store)` |
| `EventPublisher` | `EventPublisher` | `.WithEvents(ep)` |
| `Metrics` | `MetricsRecorder` | `.WithMetrics(rec)` |
| `ContextWindow` | `ContextWindowStrategy` | `.WithContextWindow(cw)` |
| `CheckpointStore` | `persist.CheckpointStore` | `.WithCheckpointStore(cs)` |
| `RAG` | `*RAGConfig` | `.WithRAG(cfg)` |
| `Hooks` | `Hooks` | `.WithHooks(h)` |
| `Summarizer` | `memory.SummaryExtractor` | `.WithSummarizer(s)` |
| `FileScope` | `[]string` | `.WithFileScope(scopes)` |
| `HITL` | `*HITLConfig` | `.WithHITL(cfg)` |
| `CostTracker` | `*CostTracker` | `.WithCostTracker(ct)` |
| `Tracer` | `Tracer` | `.WithTracer(t)` |
| `Cache` | `llm.LLMCache` | `.WithCache(cache)` |

## 迁移示例

### Before (v0.5.x 写法,仍可用)

```go
agent := ap.NewReActAgent(ap.ReActConfig{
    Name:         "my-agent",
    SystemPrompt: "你是一个智能助手",
    Model:        provider,
    MaxTurns:     10,
    Memory:       mem,
    RAG:          &ap.RAGConfig{...},
    Hooks:        hooks,
    Metrics:      metrics,
    Tracer:       tracer,
    CostTracker:  costTracker,
    FileScope:    []string{"/data"},
    Cache:        cache,
})
```

### After (v0.6.0+ 推荐)

```go
agent := ap.NewAgent("my-agent", "你是一个智能助手", provider,
    ap.WithMaxTurns(10),
).WithMemory(mem).
    WithRAG(ap.RAGConfig{...}).
    WithHooks(hooks).
    WithMetrics(metrics).
    WithTracer(tracer).
    WithCostTracker(costTracker).
    WithFileScope([]string{"/data"}).
    WithCache(cache)
```

### After v0.8.0+（v0.7.0 新增，**更推荐**）

> v0.7.0 起 `ap.NewAgent()` 替代 `ap.NewReActAgent(ReActConfig{...})` 作为推荐入口。
> 优势：不暴露 14 个 Deprecated 字段视野污染，函数式选项语义清晰。

```go
agent := ap.NewAgent("my-agent", "你是一个智能助手", provider,
    ap.WithMaxTurns(10),
).WithMemory(mem).
    WithRAG(ap.RAGConfig{...}).
    WithHooks(hooks)
```

### RAG 简化（v0.7.0+）

> v0.7.0 起启用 RAG 不再需要 6 步手动组装，使用 `WithRAGMemory` 一步完成：

```go
// 旧（6 步）
adapter := ap.NewEmbeddingAdapter(provider, 1536)
ragStore := ap.NewRAGStore(memory, adapter)
ragProvider := ap.NewRAGProviderAdapter(ragStore)
agent := ap.NewReActAgent(ap.ReActConfig{Model: provider}).WithRAG(ap.RAGConfig{Provider: ragProvider})

// 新（2 步）
agent := ap.NewAgent("my-agent", "你是一个智能助手", provider).
    WithMemory(memory).
    WithRAGMemory(memory, provider)  // 自动完成 VectorStore + RAGStore + 适配器
```

### 混合写法(过渡期)

```go
// 推荐入口用 ap.NewAgent, 必填字段也用 NewAgent 的参数，可选能力全部用链式
agent := ap.NewAgent("my-agent", "你是一个智能助手", provider,
    ap.WithMaxTurns(10),
).WithMemory(mem).WithRAG(...)

// 旧 ReActConfig 仍可使用（v2.0.0 移除前一直可用）
agent := ap.NewReActAgent(ap.ReActConfig{
    Name: "my-agent", SystemPrompt: "你是一个智能助手", Model: provider, MaxTurns: 10,
    // Lifecycle / Logger 仍可放 Config 里(非废弃)
}).WithMemory(mem).WithRAG(...)
```

## 检测当前代码中的废弃用法

```bash
# 找到所有直接赋值给废弃字段的位置
grep -rn "Memory:" --include="*.go" ./  | grep "ReActConfig"
grep -rn "RAG:"     --include="*.go" ./ | grep "ReActConfig"
# ... 类似 14 个字段
```

或者用 IDE 静态检查(Go 1.19+ 支持 `// Deprecated:` 标注的悬停提示)。

## 引擎如何发现能力?

引擎通过 `a.self.(XxxCapable)` 类型断言发现链式 API 注入的能力。
Config 字段作为**回退路径**仍然有效,但优先级低于接口发现:

```go
// react_loop.go (引擎内部)
func (a *ReActAgent) getMemoryStore() MemoryStore {
    // 1. 优先：链式 API 注入的 CapabilityAgent
    if c, ok := a.self.(MemoryCapable); ok && c.GetMemoryStore() != nil {
        return c.GetMemoryStore()
    }
    // 2. 回退：旧 Config 字段
    return a.config.Memory
}
```

**含义**: 即便你混用两种写法,链式 API 注入的会**覆盖** Config 字段。
v1.0.0 起会打印 `slog.Warn` 警告提示迁移,v2.0.0 起 Config 字段会被移除,届时仅链式 API 生效。

## FAQ

**Q: 我必须立刻迁移吗?**
A: 不必。v0.6.0 - v0.8.0 期间 Config 字段完全可用,只是 godoc 标注 `Deprecated`。v1.0.0 起会打印运行时警告但功能不受影响。

**Q: 链式 API 有什么好处?**
A: 类型安全(编译期检查每个能力)、IDE 自动补全(`agent.With...` 显示所有能力)、接口发现统一(协议式微内核)。

**Q: 我可以只迁移一部分字段吗?**
A: 可以。每个能力独立,迁移一个不影响其他。但建议一次性全部迁移,避免 v1.0.0 升级时出现大量运行时警告。

**Q: `Lifecycle` 和 `Logger` 为什么不废弃?**
A: 它们是默认值(默认自动创建 / 默认 `slog.Default()`),而非"用户注入的能力"。链式 API 不适合表达"自动"语义,Config 字段保留。

**Q: 我自定义的 `WithXxx` 扩展能工作吗?**
A: 可以。`CapabilityAgent` 包装 `ReActAgent` 并实现所有 `*Capable` 接口。你可以基于 `CapabilityAgent` 进一步包装添加自定义能力(只要新接口被引擎识别)。

**Q: 引擎会在 v1.0.0 panic 吗?**
A: 不会。原计划在 v1.0.0 panic,经生产用户反馈后已改为 `slog.Warn` 警告 + 优雅回退。
v1.0.0 起若检测到废弃字段非 nil,会打印类似以下警告并自动使用链式 API 路径:
```
WARN ReActConfig.Memory is deprecated; use .WithMemory(store) instead. See: docs/migration/v0-deprecations.md
```
功能不受影响,但建议尽早迁移以消除警告。

## 反馈

如有迁移困难或提案,请在 GitHub Issues 标注 `migration`。
我们承诺在 v0.7.0 之前根据反馈调整时间表。

# 跨包接口关系图

> **维护日期**: 2026-06-10  
> **适用版本**: v0.8.0+  

## 1. 模块边界回顾

```
┌──────────────────────────────────────────────────┐
│            agent/  (顶层 — Agent 运行时)          │
│   agent.Agent, agent.MemoryStore, agent.RAGProvider │
│   agent.Tracer, agent.Hooks, agent.EventPublisher  │
│   agent.MetricsRecorder, agent.ContextWindowStrategy│
└────┬───────┬───────┬───────────┬──────┬───────────┘
     │       │       │           │      │
┌────▼─┐ ┌───▼──┐ ┌──▼───┐ ┌────▼────┐ │
│ llm  │ │memory│ │persist│ │  tools  │ │
│      │ │      │ │       │ │         │ │
│ Provider │Memory│ │Check- │ │Registry │ │
│ Embedder │RAGStore│point  │ │Plugin   │ │
│ LLMCache│Vector- │Store  │ │         │ │
└──────┘ │Store  │ └───────┘ └────┬────┘ │
         └──────┘                 │       │
                             ┌────▼────┐  │
                             │  pool   │  │
                             └─────────┘  │
                                          │
    ┌─────────────────────────────────────┘
    │  ecosystem/     (独立域，不 import internal/)
    │  plugins/* — 通过 tools.Plugin 协议解耦
    └─────────────────────────────────────
```

## 2. 关键接口映射表

当你有 **X** (来自 Y 包) 而需要 **Z** (agent 包) 的时候，看这张表：

| 你有什么 (X) | 在哪 | 你要什么 (Z) | 在哪 | 怎么做 |
|:---|:---|:---|:---|:---|
| `memory.Memory` | `internal/memory` | `agent.MemoryStore` | `internal/agent` | 直接用：`memory.Memory` 实现 `agent.MemoryStore`（同接口） |
| `memory.RAGStore` | `internal/memory` | `agent.RAGProvider` | `internal/agent` | 用 `WithRAGMemory(mem, emb)`（Phase 14 新增，自动适配）<br>或手动：`NewRAGProviderAdapter(ragStore)` |
| `llm.Provider` | `internal/llm` | `memory.EmbeddingProvider` | `internal/memory` | 用 `NewEmbeddingAdapter(llmProvider, 1536)`（仅当 LLM 支持 Embeddings） |
| `llm.Provider` | `internal/llm` | `agent.Model`（直接设到 Config.Model） | `internal/agent` | 直接用，`llm.Provider` 就是 `agent.Model` 的类型 |
| `llm.LLMCache` | `internal/llm` | 缓存能力 | `agent.WithCache(cache)` | 直接用 `ap.WithCache(myCache)`，框架自动包装 |
| `persist.CheckpointStore` | `internal/persist` | 检查点能力 | `agent.WithCheckpointStore(cs)` | 直接用 `ap.WithCheckpointStore(myStore)` |
| `tools.Registry` | `internal/tools` | 工具注册 | `agent.WithToolkit(registry)` | 直接用 `ap.WithToolkit(myRegistry)` |
| `memory.SummaryExtractor` | `internal/memory` | 摘要能力 | `agent.WithSummarizer(ext)` | 直接用 `ap.WithSummarizer(myExt)` |
| events.Bus / 自定义 | 任何 | `agent.EventPublisher` | `agent.WithEvents(pub)` | 实现 `agent.EventPublisher` 接口，传入即可 |

## 3. 不需要适配的情况（90% 场景）

Phase 14 以后，以下能力注入是**零适配**的：

```go
agent := ap.NewAgent("bot", "prompt", provider, ap.WithMaxTurns(10)).
    WithMemory(mem).                // ✅ memory.Memory 直接是 agent.MemoryStore
    WithToolkit(registry).          // ✅ tools.Registry 直接用
    WithHooks(hooks).               // ✅ agent.Hooks 直接用
    WithTracer(tracer).             // ✅ agent.Tracer 直接用
    WithCostTracker(ct).            // ✅ *agent.CostTracker 直接用
    WithContextWindow(cw).          // ✅ agent.ContextWindowStrategy 直接用
    WithEvents(pub).                // ✅ agent.EventPublisher 直接用
    WithMetrics(recorder).          // ✅ agent.MetricsRecorder 直接用
    WithCheckpointStore(cs).        // ✅ persist.CheckpointStore 直接用
    WithSummarizer(ext).            // ✅ memory.SummaryExtractor 直接用
    WithFileScope(scopes).          // ✅ []string 直接用
    WithCache(cache).               // ✅ llm.LLMCache 直接用（自动包装）
    WithHITL(cfg).                  // ✅ HITLConfig 直接用
```

## 4. 需要适配的唯一情况：RAG

RAG 是将 memory 层的 `RAGStore` 喂给 agent 层的 `RAGProvider`——两个不同包的接口。

**推荐方式（Phase 14 起）**：

```go
agent.WithRAGMemory(memoryStore, llmProvider) // 2 步：读 + 配
```

**手动方式（需要精细控制时）**：

```go
// 6 步
emb := memory.NewEmbeddingAdapter(provider, 1536)    // 1. 创建 Embedding 适配器
ragStore := memory.NewRAGStore(mem, emb)             // 2. 创建 RAG 存储
ragProvider := agent.NewRAGProviderAdapter(ragStore) // 3. 创建 RAG 提供者适配器
ragCfg := agent.RAGConfig{                            // 4. 配置
    Provider:  ragProvider,
    Mode:      agent.RAGModeAuto,
    TopK:      5,
    MinScore:  0.3,
}
agent.WithRAG(ragCfg)                                // 5. 注入
```

## 5. 接口定义清单

| 接口名 | 包 | 方法数 | 用途 |
|:---|:---|:---:|:---|
| `Agent` | `agent` | 4 | Run, StreamRun, Stop, Stats |
| `MemoryStore` | `agent` | 3 | Remember, Recall, Forget |
| `RAGProvider` | `agent` | 1 | Search |
| `Tracer` | `agent` | 3 | StartSpan, EndSpan, AddEvent |
| `Hooks` | `agent` | 5 | Before/After Turn, Before/After Tool, Before/After LLM |
| `EventPublisher` | `agent` | 1 | Publish |
| `MetricsRecorder` | `agent` | 3 | IncCounter, ObserveHistogram, Gauge |
| `ContextWindowStrategy` | `agent` | 1 | Trim |
| `Memory` | `memory` | 5 | Store, Recall, Forget, Search, Close |
| `EmbeddingProvider` | `memory` | 1 | Embed |
| `SummaryExtractor` | `memory` | 1 | ExtractSummary |
| `CheckpointStore` | `persist` | 3 | Save, Load, Delete |
| `Provider` | `llm` | 4 | Complete, Stream, CallTools, Info |
| `Embedder` | `llm` | 1 | Embeddings |
| `LLMCache` | `llm` | 2 | Get, Set |
| `Registry` | `tools` | 5 | Register, Unregister, Get, List, Validate |

## 6. 总结原则

1. **同接口**：agent 和 memory 中名称近似的接口（MemoryStore/Memory）实际上是同一个接口在各层的影子——存在重复是因为 agent 层只暴露需要的部分方法（依赖倒置原则）。在 90% 的场景中你可以直接用同一个实现。

2. **桥接**：RAG 需要桥接是因为 agent 的 `RAGProvider` 接口语义和 memory 的 `RAGStore` 不同。桥接由框架在 `WithRAGMemory()` 中自动完成。

3. **扩展**：自定义能力需要实现接口 → 传给对应的 `WithXxx` 方法。只需要实现对应接口即可，不需要适配。
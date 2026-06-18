# RAG 检索增强

RAG（Retrieval-Augmented Generation）让 Agent 在回答前先从知识库检索相关文档，再基于检索结果生成答案，减少幻觉并支持私有知识。

## 三种记忆模式

| 模式 | 说明 | 适用场景 |
|------|------|----------|
| **Memory** | 原始对话记录 | 短期上下文 |
| **FTS5** | SQLite 全文检索 | 关键词搜索 |
| **Vector** | 语义向量检索 | 语义相似搜索 |
| **Hybrid RAG** | 向量 + 全文混合检索 | 生产推荐 |

## 快速开始

```go
// 创建 embedding provider
emb := memory.NewEmbeddingAdapter(openaiProvider)

// 一步完成 RAG 配置
agent := NewReActAgent(cfg).
    WithRAGMemory(mem, emb)
```

等价于手动配置：

```go
ragStore := memory.NewRAGStore(mem, emb)
cfg := agent.RAGConfig{
    Provider: &agent.RAGProviderAdapter{Store: ragStore},
    Mode:     agent.RAGModeAuto,
    TopK:     5,
    MinScore: 0.3,
}
agent := NewReActAgent(reactCfg).WithRAG(cfg)
```

## 添加文档

```go
doc := &memory.Document{
    ID:      "doc-001",
    Content: "AgentPrimordia 是 Go 语言 AI Agent 框架...",
    Metadata: map[string]string{"source": "docs"},
}
_ = ragStore.AddDocument(ctx, doc)
```

## RAG 模式

```go
const (
    RAGModeAuto    RAGMode = "auto"    // 自动判断
    RAGModeAlways  RAGMode = "always"  // 每轮都检索
    RAGModeManual  RAGMode = "manual"  // 仅工具触发
    RAGModeDisable RAGMode = "disable" // 关闭
)
```

## 与工具系统结合

```go
toolkit.Register(agent.NewRAGTool(ragStore))
```

Agent 可以主动调用 `rag_search` 工具查询知识库。

## 下一步

- 阅读 [RAG Agent Cookbook](../../ecosystem/docs/cookbook/rag-agent.md)
- 查看 [记忆系统](memory.md)

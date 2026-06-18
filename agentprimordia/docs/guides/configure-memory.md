# 配置记忆

本指南介绍如何配置和使用 AgentPrimordia 的记忆系统。

## 记忆层级选择

```
┌──────────────────────────────────────────┐
│  需要语义搜索？                           │
│  ├─ 是 → 使用 Vector Store + RAG        │
│  └─ 否 → 需要全文搜索？                  │
│          ├─ 是 → 使用 SQLite FTS5        │
│          └─ 否 → 使用简单 KV 存储        │
└──────────────────────────────────────────┘
```

## SQLite 记忆

### 基础配置

```go
import "agentprimordia.dev/agentprimordia/pkg/memory"

mem, err := memory.NewSQLiteMemory(memory.SQLiteConfig{
    Path: "./data/memory.db",
    FTS5: true,   // 启用全文搜索
    WAL:  true,   // 启用 WAL 模式（并发读写）
})
if err != nil {
    log.Fatal(err)
}
defer mem.Close()
```

### 存储与检索

```go
ctx := context.Background()

// 存储
err := mem.Store(ctx, "user:1:name", "Alice")
err = mem.Store(ctx, "user:1:email", "alice@example.com")

// 检索
value, err := mem.Load(ctx, "user:1:name")
// value = "Alice"

// 删除
err = mem.Delete(ctx, "user:1:email")
```

### 全文搜索

```go
// 存储多条记忆
mem.Store(ctx, "doc:1", "Agent 架构设计文档")
mem.Store(ctx, "doc:2", "Agent 性能优化指南")
mem.Store(ctx, "doc:3", "Python 编程入门")

// 搜索
results, err := mem.Search(ctx, "Agent", 10)
// results 包含 doc:1 和 doc:2
```

### 批量操作

```go
items := []memory.MemoryItem{
    {Key: "key1", Value: "value1"},
    {Key: "key2", Value: "value2"},
    {Key: "key3", Value: "value3"},
}

err := mem.BatchStore(ctx, items)
```

### 事务

```go
err := mem.Transaction(ctx, func(tx memory.Transaction) error {
    tx.Store(ctx, "key1", "value1")
    tx.Store(ctx, "key2", "value2")
    // 如果这里返回错误，所有操作回滚
    return nil
})
```

## 向量存储

### 基础配置

```go
vectorStore := memory.NewVectorStore(memory.VectorConfig{
    Path:       "./data/vectors.db",
    Dimensions: 1536,  // 嵌入维度
    Index:      "hnsw", // 索引类型
})
defer vectorStore.Close()
```

### HNSW 索引配置

```go
vectorStore := memory.NewVectorStore(memory.VectorConfig{
    Path:       "./data/vectors.db",
    Dimensions: 1536,
    Index:      "hnsw",
    HNSWConfig: memory.HNSWConfig{
        M:              16,   // 每个节点最大连接数
        EFConstruction: 200,  // 构建时搜索范围
        EFSearch:       100,  // 查询时搜索范围
    },
})
```

### 存储向量

```go
// 使用 OpenAI Embedding
embedding := []float32{0.1, 0.2, ...}  // 1536 维

err := vectorStore.Store(ctx, "doc:1", embedding, map[string]string{
    "title":   "Agent 架构设计",
    "content": "本文介绍了 Agent 的核心架构...",
    "author":  "Alice",
})
```

### 语义搜索

```go
// 查询向量
queryEmbedding := []float32{0.15, 0.25, ...}

results, err := vectorStore.Search(ctx, queryEmbedding, 5)
for _, r := range results {
    fmt.Printf("ID: %s, Score: %f, Metadata: %v\n", r.ID, r.Score, r.Metadata)
}
```

## RAG Pipeline

### 基础配置

```go
rag := memory.NewRAGPipeline(memory.RAGConfig{
    Memory:      mem,
    VectorStore: vectorStore,
    Embedder:    embedder,  // 嵌入模型
    TopK:        5,         // 检索 Top-K 条
})
```

### 添加文档

```go
err := rag.AddDocument(ctx, memory.Document{
    ID:      "doc:1",
    Title:   "Agent 最佳实践",
    Content: "设计 Agent 时应该遵循以下原则...",
    Tags:    []string{"agent", "best-practice"},
})
```

### RAG 查询

```go
answer, err := rag.Query(ctx, "如何设计高可用的 Agent 系统？")
// answer 包含基于检索文档生成的答案
```

### 自定义 Prompt

```go
rag := memory.NewRAGPipeline(memory.RAGConfig{
    // ...
    SystemPrompt: "你是一个技术专家。基于以下参考资料回答问题。如果资料中没有相关信息，请明确说明。",
})
```

## 记忆清理

### 自动清理

```go
mem := memory.NewSQLiteMemory(memory.SQLiteConfig{
    Path: "./data/memory.db",
}).WithCleanup(memory.CleanupConfig{
    MaxAge:              30 * 24 * time.Hour,  // 30 天过期
    MaxSize:             10000,                 // 最多 10000 条
    ImportanceThreshold: 0.3,                   // 重要性阈值
    CleanupInterval:     1 * time.Hour,         // 每小时清理一次
})
```

### 手动清理

```go
// 清理过期记忆
err := mem.Cleanup(ctx)

// 清理指定标签的记忆
err := mem.CleanupByTags(ctx, []string{"temp", "cache"})
```

## 记忆重要性

```go
// 存储高重要性记忆（不会被自动清理）
mem.Store(ctx, "user:1:preferences", prefs, memory.WithImportance(0.9))

// 存储低重要性记忆（优先被清理）
mem.Store(ctx, "session:temp:data", data, memory.WithImportance(0.1))
```

## 记忆标签

```go
// 存储带标签的记忆
mem.Store(ctx, "doc:1", content, memory.WithTags([]string{"ai", "agent"}))

// 按标签搜索
results, err := mem.SearchByTags(ctx, []string{"ai"}, 10)

// 按标签删除
err = mem.DeleteByTags(ctx, []string{"temp"})
```

## 会话记忆

```go
sessionMem := memory.NewSessionMemory()

// 存储会话上下文
sessionMem.Store(ctx, "conversation:1", []memory.Message{
    {Role: "user", Content: "你好"},
    {Role: "assistant", Content: "你好！有什么可以帮助你的？"},
    {Role: "user", Content: "今天天气怎么样？"},
})

// 获取会话历史
messages, err := sessionMem.GetConversation(ctx, "conversation:1")
```

## 性能优化

### 缓存

```go
mem := memory.NewSQLiteMemory(config).
    WithCache(memory.NewLRUCache(1000))  // 缓存 1000 条热点数据
```

### 索引

```go
// 创建自定义索引
mem.CreateIndex(ctx, "idx_user_id", "user_id")
mem.CreateIndex(ctx, "idx_created_at", "created_at")
```

### 批量操作

```go
// 批量存储（比单条存储快 10-100 倍）
items := make([]memory.MemoryItem, 1000)
// ... 填充 items
mem.BatchStore(ctx, items)
```

### 连接池

```go
mem := memory.NewSQLiteMemory(memory.SQLiteConfig{
    Path:         "./data/memory.db",
    MaxOpenConns: 10,
    MaxIdleConns: 5,
})
```

## 监控

### 指标收集

```go
mem := memory.NewSQLiteMemory(config).
    WithMetrics(metricsCollector)

// 收集的指标：
// - memory_store_total
// - memory_load_total
// - memory_search_duration
// - memory_size
```

### 健康检查

```go
err := mem.HealthCheck(ctx)
if err != nil {
    log.Printf("记忆系统不健康: %v", err)
}
```

## 最佳实践

1. **选择合适的存储层**：结构化数据用 SQLite，语义搜索用 Vector Store
2. **启用 WAL 模式**：提高并发读写性能
3. **设置清理策略**：防止记忆无限增长
4. **使用标签分类**：便于检索和管理
5. **批量操作**：减少数据库往返
6. **启用缓存**：热点数据缓存减少查询
7. **监控性能**：定期检查查询耗时和存储大小

## 下一步

- 查看 [RAG 示例](../examples/rag.md) 了解实际应用
- 阅读 [记忆 API](../api/memory.md) 了解完整接口定义
- 学习 [性能优化](../advanced/performance.md) 了解更多优化技巧

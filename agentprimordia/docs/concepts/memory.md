# 记忆系统

AgentPrimordia 的三层记忆架构提供了从短期会话到长期知识的完整记忆解决方案。

## 三层架构

```
┌─────────────────────────────────────────┐
│  Layer 3: RAG Pipeline                  │
│  - 检索增强生成                         │
│  - 语义搜索                             │
│  - 知识库集成                           │
└─────────────────────────────────────────┘
                    ▲
┌─────────────────────────────────────────┐
│  Layer 2: Vector Store                  │
│  - 向量嵌入                             │
│  - 相似度检索                           │
│  - HNSW 索引                            │
└─────────────────────────────────────────┘
                    ▲
┌─────────────────────────────────────────┐
│  Layer 1: SQLite FTS5                   │
│  - 全文搜索                             │
│  - 结构化存储                           │
│  - 事务支持                             │
└─────────────────────────────────────────┘
```

## Memory 接口

所有记忆实现都遵循统一接口：

```go
type Memory interface {
    // Store 存储记忆
    Store(ctx context.Context, key string, value interface{}) error
    
    // Load 加载记忆
    Load(ctx context.Context, key string) (interface{}, error)
    
    // Delete 删除记忆
    Delete(ctx context.Context, key string) error
    
    // Search 搜索记忆
    Search(ctx context.Context, query string, limit int) ([]MemoryItem, error)
    
    // Close 关闭记忆存储
    Close() error
}
```

## SQLite FTS5 记忆

基于 SQLite 的全文搜索记忆，适合结构化数据：

```go
// 创建 SQLite 记忆
memory := memory.NewSQLiteMemory(memory.SQLiteConfig{
    Path:     "./data/memory.db",
    FTS5:     true,  // 启用全文搜索
    WAL:      true,  // 启用 WAL 模式
})

// 存储记忆
memory.Store(ctx, "user:1:name", "Alice")
memory.Store(ctx, "user:1:preferences", map[string]string{
    "theme": "dark",
    "language": "zh",
})

// 搜索记忆
results, _ := memory.Search(ctx, "Alice", 10)
```

### 特性

- **全文搜索**：基于 FTS5 的高效文本搜索
- **事务支持**：原子性批量操作
- **WAL 模式**：并发读写不阻塞
- **持久化**：数据保存在磁盘

## Vector Store

向量存储支持语义搜索：

```go
// 创建向量存储
vectorStore := memory.NewVectorStore(memory.VectorConfig{
    Path:       "./data/vectors.db",
    Dimensions: 1536,  // OpenAI embeddings 维度
    Index:      "hnsw", // HNSW 索引
})

// 存储向量
embedding := []float32{0.1, 0.2, ...}  // 1536 维
vectorStore.Store(ctx, "doc:1", embedding, map[string]string{
    "title": "Agent 架构设计",
    "content": "本文介绍了...",
})

// 语义搜索
queryEmbedding := []float32{0.15, 0.25, ...}
results, _ := vectorStore.Search(ctx, queryEmbedding, 5)
```

### HNSW 索引

Hierarchical Navigable Small World 索引提供快速的近似最近邻搜索：

```go
vectorStore := memory.NewVectorStore(memory.VectorConfig{
    Index: "hnsw",
    HNSWConfig: memory.HNSWConfig{
        M:              16,   // 每个节点的最大连接数
        EFConstruction: 200,  // 构建时的搜索范围
        EFSearch:       100,  // 搜索时的搜索范围
    },
})
```

## RAG Pipeline

检索增强生成将记忆检索与 LLM 生成结合：

```go
// 创建 RAG 管道
rag := memory.NewRAGPipeline(memory.RAGConfig{
    Memory:      sqliteMemory,
    VectorStore: vectorStore,
    Embedder:    openaiEmbedder,
    TopK:        5,
})

// RAG 查询
answer, err := rag.Query(ctx, "如何设计一个高可用的 Agent 系统？")
```

### 工作流程

```
1. 用户提问
   ↓
2. 将问题转换为向量嵌入
   ↓
3. 在向量存储中检索相关文档
   ↓
4. 将检索到的文档作为上下文
   ↓
5. LLM 基于上下文生成答案
   ↓
6. 返回答案给用户
```

## 记忆类型

### 会话记忆

短期记忆，用于保持当前会话的上下文：

```go
sessionMemory := memory.NewSessionMemory()

// 存储会话上下文
sessionMemory.Store(ctx, "conversation:1", []Message{
    {Role: "user", Content: "你好"},
    {Role: "assistant", Content: "你好！有什么可以帮助你的？"},
})
```

### 长期记忆

持久化记忆，用于存储重要信息：

```go
longTermMemory := memory.NewSQLiteMemory(...)

// 存储用户偏好
longTermMemory.Store(ctx, "user:1:preferences", preferences)

// 下次会话时加载
prefs, _ := longTermMemory.Load(ctx, "user:1:preferences")
```

### 知识记忆

结构化知识库，用于 RAG：

```go
knowledgeBase := memory.NewKnowledgeBase(vectorStore)

// 添加文档
knowledgeBase.AddDocument(ctx, Document{
    ID:      "doc:1",
    Title:   "Agent 最佳实践",
    Content: "设计 Agent 时应该...",
    Tags:    []string{"agent", "best-practice"},
})
```

## 记忆清理

自动清理过期或不重要的记忆：

```go
memory := memory.NewSQLiteMemory(...).
    WithCleanup(memory.CleanupConfig{
        MaxAge:      30 * 24 * time.Hour,  // 30 天过期
        MaxSize:     10000,                 // 最多 10000 条
        ImportanceThreshold: 0.3,           // 重要性阈值
    })
```

## 记忆重要性

为记忆分配重要性权重，影响清理和检索优先级：

```go
memory.Store(ctx, "key", "value", memory.WithImportance(0.9))
```

## 记忆标签

为记忆添加标签，支持分类检索：

```go
memory.Store(ctx, "doc:1", content, memory.WithTags([]string{"ai", "agent"}))

// 按标签搜索
results, _ := memory.SearchByTags(ctx, []string{"ai"}, 10)
```

## 性能优化

### 批量操作

批量存储和检索减少数据库往返：

```go
items := []MemoryItem{
    {Key: "key1", Value: "value1"},
    {Key: "key2", Value: "value2"},
    {Key: "key3", Value: "value3"},
}
memory.BatchStore(ctx, items)
```

### 缓存

热点数据缓存减少数据库查询：

```go
memory := memory.NewSQLiteMemory(...).
    WithCache(memory.NewLRUCache(1000))  // 缓存 1000 条
```

### 索引优化

为常用查询字段创建索引：

```go
memory.CreateIndex(ctx, "idx_user_id", "user_id")
```

## 最佳实践

1. **选择合适的记忆层**：结构化数据用 SQLite，语义搜索用 Vector Store
2. **设置合理的清理策略**：防止记忆无限增长
3. **使用标签分类**：便于检索和管理
4. **批量操作**：减少数据库往返
5. **监控性能**：定期检查查询耗时和索引大小

## 下一步

- 学习如何 [配置记忆](../guides/configure-memory.md)
- 查看 [RAG 示例](../examples/rag.md) 了解实际应用
- 阅读 [API 参考](../api/memory.md) 了解详细接口定义

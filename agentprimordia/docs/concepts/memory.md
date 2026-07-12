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

所有记忆实现都遵循统一复合接口，由 7 个子接口组成：

```go
type Memory interface {
    MemoryReader      // Get, GetBatch, Search, List, Count, Stats
    MemoryWriter      // Add, AddBatch, Delete, DeleteBatch, UpdateSummary, SetImportance
    MemorySearcher    // SearchAdvanced, SearchByTag, GetImportant, GetTimeline
    MemoryLifecycle   // Close, CleanupExpired, ClearAll
    MemoryExporter    // ExportMemories, ImportMemories
    MemoryQuery       // GetMemoriesByTag, GetMemoriesBySession, GetImportantMemories, GetMemoryTimeline
    MemoryToolUse     // RecordToolUse
}
```

### Episode 结构

记忆的基本单元是 `Episode`：

```go
type Episode struct {
    ID         string            `json:"id"`
    SessionID  string            `json:"session_id"`
    Role       string            `json:"role"`               // user / assistant / system / tool
    Content    string            `json:"content"`
    Summary    string            `json:"summary,omitempty"`
    Topics     string            `json:"topics,omitempty"`
    Importance float64           `json:"importance,omitempty"`
    Metadata   map[string]string `json:"metadata,omitempty"`
    CreatedAt  string            `json:"created_at"`
}
```

## SQLite FTS5 记忆

基于 SQLite 的全文搜索记忆，适合结构化数据：

```go
// 创建 SQLite 记忆
mem, _ := memory.NewSQLiteStore("./data/memory.db")
defer mem.Close()

// 启用 WAL 模式（可选）
mem = mem.WithWAL()

// 存储记忆
mem.Add(ctx, &memory.Episode{
    SessionID: "session-1",
    Role:      "user",
    Content:   "你好",
    Metadata:  map[string]string{"source": "chat"},
})

// 全文搜索
results, _ := mem.Search(ctx, "你好", &memory.SearchOptions{Limit: 10})
```

### 特性

- **全文搜索**：基于 FTS5 的高效文本搜索
- **批量操作**：`AddBatch` / `DeleteBatch` / `GetBatch` 减少数据库往返
- **WAL 模式**：并发读写不阻塞
- **自动清理**：`WithCleanup(maxAgeDays)` 自动清理过期记忆
- **持久化**：数据保存在磁盘

## Vector Store

向量存储支持语义搜索：

```go
// 创建向量存储
vectorStore := memory.NewVectorStore(1536)  // 1536 维（OpenAI embeddings）

// 存储向量
embedding := []float32{0.1, 0.2, ...}  // 1536 维
vectorStore.Add(ctx, "doc:1", embedding, map[string]string{
    "title":   "Agent 架构设计",
    "content": "本文介绍了...",
})

// 语义搜索
queryEmbedding := []float32{0.15, 0.25, ...}
results, _ := vectorStore.Search(ctx, queryEmbedding, 5)
```

### HNSW 索引

Hierarchical Navigable Small World 索引提供快速的近似最近邻搜索：

```go
vectorStore := memory.NewVectorStoreWithHNSW(1536, memory.HNSWConfig{
    M:              16,   // 每个节点的最大连接数
    EFConstruction: 200,  // 构建时的搜索范围
    EFSearch:       100,  // 搜索时的搜索范围
})
```

## RAG Pipeline

检索增强生成将记忆检索与 LLM 生成结合：

```go
// 创建 RAG Store
ragStore := memory.NewRAGStore(mem, embedder)

// 混合检索：关键词 + 语义双通道
results, _ := ragStore.HybridSearch(ctx, "Go 并发模型", 5)
// 自动融合 FTS 和向量搜索结果，按相关度排序
```

### RRF 融合模式

v0.8.0 新增 Reciprocal Rank Fusion（RRF）混合检索算法：

```go
// 使用 RRF 融合模式创建
ragStore := memory.NewRAGStoreWithFusionConfig(mem, embedder, memory.FusionRRF)

// 运行时切换融合模式
ragStore.SetFusionConfig(memory.RAGFusionConfig{
    FusionMode: memory.FusionRRF,
    RRFK:       60,
})
```

### 工作流程

```
1. 用户提问
   ↓
2. 将问题转换为向量嵌入
   ↓
3. 在向量存储中检索相关文档
   ↓
4. 同时在 FTS5 中检索关键词
   ↓
5. RRF 融合两路结果
   ↓
6. 将检索到的文档作为上下文
   ↓
7. LLM 基于上下文生成答案
   ↓
8. 返回答案给用户
```

## 记忆重要性

为记忆分配重要性权重，影响清理和检索优先级：

```go
mem.Add(ctx, &memory.Episode{
    SessionID:  "session-1",
    Role:       "user",
    Content:    "用户偏好：深色模式",
    Importance: 0.9,  // 高重要性
})

// 按重要性检索
important, _ := mem.GetImportant(ctx, 0.7, 10)
```

## 记忆标签

通过 `Topics` 字段为记忆添加标签，支持分类检索：

```go
mem.Add(ctx, &memory.Episode{
    SessionID: "session-1",
    Role:      "user",
    Content:   "Agent 架构设计文档",
    Topics:    "ai,agent,best-practice",
})

// 按标签搜索
results, _ := mem.SearchByTag(ctx, "ai", &memory.SearchOptions{Limit: 10})
```

## 记忆清理

自动清理过期或不重要的记忆：

```go
mem, _ := memory.NewSQLiteStore("./data/memory.db")

// 启用自动清理（30 天过期）
mem = mem.WithCleanup(30)

// 手动清理
deleted, _ := mem.CleanupExpired(ctx, 30)  // 清理 30 天前的记忆
```

## 外部向量库

### QdrantProvider

```go
provider, _ := memory.NewQdrantProvider(memory.QdrantConfig{
    Host:   "localhost",
    Port:   6333,
    APIKey: "optional-api-key",
})
```

### MilvusProvider

```go
provider, _ := memory.NewMilvusProvider(memory.MilvusConfig{
    Host:   "localhost",
    Port:   19530,
})
```

## Vector DB 选型

| 规模 | 推荐 | 原因 |
|------|------|------|
| <100K 文档 | InMemory | 零依赖 |
| 100K-1M | Qdrant | Go REST 客户端，性能优 |
| >1M | Milvus | 分布式，水平扩展 |
| 已有 PostgreSQL | pgvector | 不引入新基础设施 |

## 最佳实践

1. **选择合适的记忆层**：结构化数据用 SQLite，语义搜索用 Vector Store
2. **设置合理的清理策略**：防止记忆无限增长
3. **使用标签分类**：便于检索和管理
4. **批量操作**：使用 `AddBatch` 减少数据库往返
5. **监控性能**：定期检查查询耗时和索引大小

## 下一步

- 学习如何 [配置记忆](../guides/configure-memory.md)
- 查看 [RAG 示例](../cookbook/rag-agent.md) 了解实际应用
- 阅读 [API 参考](../api/memory.md) 了解详细接口定义

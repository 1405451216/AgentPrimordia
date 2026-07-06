# 记忆系统指南

> 让 Agent 跨会话保持上下文与知识。

## 记忆后端

### InMemory（内存模式）

适用于测试和临时对话：

```go
mem := ap.NewInMemoryMemory()
```

### SQLite（单机持久化）

适用于生产环境单机部署：

```go
mem, _ := ap.NewSQLiteMemory("./data/memory.db")
```

自动建表、自动迁移、支持数百万条对话历史。

### Vector（向量检索）

适用于知识库 / RAG 场景：

```go
mem, _ := ap.NewVectorMemory(ap.VectorConfig{
    DBPath:   "./data/vectors.db",
    Embedder: ap.NewOpenAIEmbedder(), // 或本地 ONNX 嵌入模型
    Dim:      1536,                    // 嵌入向量维度
})
```

### pgvector（企业级）

适用于高并发、大数据量：

```go
mem, _ := ap.NewPgVectorMemory(ap.PgConfig{
    DSN:      "postgres://...",
    TableName: "embeddings",
    Dim:      1536,
})
```

## 写入与检索

```go
// 写入对话
mem.Add(ctx, &ap.Episode{
    Role:    "user",
    Content: "什么是 RAG？",
    Metadata: map[string]any{"session_id": "123"},
})

// 语义搜索
results, _ := mem.Search(ctx, "检索增强生成", 5) // top-5

// 关键词搜索
results, _ := mem.KeywordSearch(ctx, "RAG", 5)
```

## 记忆压缩

对话过长时自动摘要压缩：

```yaml
memory:
  backend: sqlite
  compression:
    enabled: true
    max_episodes: 50     # 超过 50 条自动摘要
    summarize_turns: 10  # 每 10 轮摘要一次
```

## 多租户 Memory

通过租户 ID 前缀自动隔离：

```go
// 在 middleware 中注入 tenant
ctx = multitenant.WithTenant(ctx, "tenant-a")
mem.Add(ctx, episode) // 写入 tenant-a:xxx
mem.Search(ctx, query, 5) // 只返回 tenant-a 的数据
```

## 数据保留

```yaml
memory:
  retention_days: 30            # 30 天后自动清理
  max_size_mb: 1024             # 最大 1GB
  cleanup_interval_hours: 24    # 每天清理一次
```

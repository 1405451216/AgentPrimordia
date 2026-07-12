# 配置记忆

本指南介绍如何配置和使用 AgentPrimordia 的记忆系统。

## 记忆层级选择

```
┌──────────────────────────────────────────┐
│  需要语义搜索？                           │
│  ├─ 是 → 使用 Vector Store + RAG        │
│  └─ 否 → 需要全文搜索？                  │
│          ├─ 是 → 使用 SQLite FTS5        │
│          └─ 否 → 使用简单内存存储        │
└──────────────────────────────────────────┘
```

## 内存记忆（开发/测试）

最简单的记忆后端，无需外部依赖：

=== "Go"

    ```go
    memory, _ := ap.WithInMemory()
    defer memory.Close()

    agent, err := ap.NewAgent("assistant", "你是助手", provider,
        ap.WithMaxTurns(10),
        ap.WithMemory(memory),
    )
    ```

=== "TypeScript"

    ```typescript
    import { InMemoryStore } from '@agentprimordia/sdk';

    const memory = new InMemoryStore();

    const agent = new ReActAgent({
      name: 'assistant',
      model: provider,
      toolkit: new ToolRegistry(),
      maxTurns: 10,
      memory,
    });
    ```

## SQLite 记忆（生产推荐）

### 基础配置

=== "Go"

    ```go
    memory, _ := ap.NewSQLiteStore("./data/memory.db")
    defer memory.Close()

    // 可选：启用 WAL 模式（并发读写）
    memory = memory.WithWAL()

    // 可选：启用自动清理（30 天过期）
    memory = memory.WithCleanup(30)

    agent, err := ap.NewAgent("assistant", "你是助手", provider,
        ap.WithMaxTurns(10),
        ap.WithMemory(memory),
    )
    ```

=== "TypeScript"

    ```typescript
    import { SqliteStore } from '@agentprimordia/sdk';
    // 需要先安装：npm install better-sqlite3

    const memory = new SqliteStore({
      dbPath: './data/memory.db',
      wal: true,  // 启用 WAL 模式
    });

    const agent = new ReActAgent({
      name: 'assistant',
      model: provider,
      toolkit: new ToolRegistry(),
      maxTurns: 10,
      memory,
    });
    ```

### 存储与检索

=== "Go"

    ```go
    ctx := context.Background()

    // 存储记忆（使用 Episode 结构）
    mem.Add(ctx, &memory.Episode{
        SessionID: "session-1",
        Role:      "user",
        Content:   "用户名是 Alice",
        Metadata:  map[string]string{"source": "chat"},
    })

    // 按ID获取
    episode, _ := mem.Get(ctx, "episode-id")

    // 全文搜索
    results, _ := mem.Search(ctx, "Alice", &memory.SearchOptions{Limit: 10})

    // 按会话列出
    episodes, _ := mem.List(ctx, &memory.ListOptions{
        SessionID: "session-1",
        Limit:     20,
        OrderBy:   "created_at",
    })
    ```

=== "TypeScript"

    ```typescript
    // 存储记忆
    await memory.add({
      sessionId: 'session-1',
      role: 'user',
      content: '用户名是 Alice',
    });

    // 全文搜索
    const results = await memory.search('Alice', { limit: 10 });
    ```

## 向量存储

用于语义搜索和 RAG 管道：

=== "Go"

    ```go
    // 创建向量存储（指定维度）
    vectorStore := memory.NewVectorStore(1536)

    // 或创建带 HNSW 索引的向量存储（推荐生产使用）
    vectorStore = memory.NewVectorStoreWithHNSW(1536, memory.HNSWConfig{
        M:              16,
        EFConstruction: 200,
        EFSearch:       100,
    })

    // 存储向量
    embedding := []float32{0.1, 0.2, ...}
    vectorStore.Add(ctx, "doc:1", embedding, map[string]string{
        "title":   "Agent 架构设计",
        "content": "本文介绍了 Agent 的核心架构...",
    })

    // 语义搜索
    queryEmbedding := []float32{0.15, 0.25, ...}
    results, _ := vectorStore.Search(ctx, queryEmbedding, 5)

    // 删除
    vectorStore.Delete(ctx, "doc:1")
    ```

=== "TypeScript"

    ```typescript
    import { VectorStore } from '@agentprimordia/sdk';

    // 创建向量存储
    const vectorStore = new VectorStore(1536);

    // 存储向量
    const embedding = [0.1, 0.2, /* ... */];
    await vectorStore.add('doc:1', embedding, {
      title: 'Agent 架构设计',
      content: '本文介绍了 Agent 的核心架构...',
    });

    // 语义搜索
    const results = await vectorStore.search(queryEmbedding, 5);
    ```

## RAG Pipeline

### 基础配置

=== "Go"

    使用 `WithRAGMemory()` 一步完成 RAG 组装（v1.0+ 推荐）：

    ```go
    embedder := ap.NewEmbeddingAdapter(provider, 1536)

    agent, err := ap.NewAgent("assistant", "你是助手", provider,
        ap.WithMaxTurns(10),
        ap.WithMemory(mem),
        ap.WithRAGMemory(mem, embedder),
    )
    ```

    手动配置 RAG：

    ```go
    embedder := ap.NewEmbeddingAdapter(provider, 1536)
    ragStore := ap.NewRAGStore(mem, embedder)

    agent, err := ap.NewAgent("assistant", "你是助手", provider,
        ap.WithMaxTurns(10),
        ap.WithRAG(ap.RAGConfig{
            Provider: ragStore,
            Mode:     ap.RAGModeAuto, // 每轮自动注入
            TopK:     5,
        }),
    )
    ```

### RRF 融合模式（生产推荐）

```go
// 创建带 RRF 融合的 RAG Store
ragStore := memory.NewRAGStoreWithFusionConfig(mem, embedder, memory.RAGFusionConfig{
    FusionMode:    memory.FusionRRF,
    RRFK:          60,
    OverFetchSize: 5,
})

// 运行时切换融合模式
ragStore.SetFusionConfig(memory.RAGFusionConfig{
    FusionMode: memory.FusionLinear,
})
```

=== "TypeScript"

    ```typescript
    import { RAGStore } from '@agentprimordia/sdk';

    const ragStore = new RAGStore({
      memory: sqliteStore,
      embedder: embedder,
      fusionMode: 'rrf',
    });

    const agent = new ReActAgent({
      name: 'assistant',
      model: provider,
      maxTurns: 10,
      memory: ragStore,
    });
    ```

## 记忆重要性

为记忆分配重要性权重，影响清理和检索优先级：

```go
mem.Add(ctx, &memory.Episode{
    SessionID:  "session-1",
    Role:       "user",
    Content:    "用户偏好：深色模式",
    Importance: 0.9,
})

// 更新重要性
mem.SetImportance(ctx, "episode-id", 0.8)

// 按重要性检索
important, _ := mem.GetImportant(ctx, 0.7, 10)
```

## 记忆清理

```go
// 自动清理（在创建时配置）
mem, _ := ap.NewSQLiteStore("./data/memory.db")
mem = mem.WithCleanup(30) // 30 天过期

// 手动清理
deleted, _ := mem.CleanupExpired(ctx, 30)
fmt.Printf("清理了 %d 条过期记忆\n", deleted)

// 清空指定会话
mem.ClearAll(ctx, "session-1")
```

## 批量操作

```go
// 批量写入
episodes := []*memory.Episode{
    {SessionID: "s1", Role: "user", Content: "消息1"},
    {SessionID: "s1", Role: "user", Content: "消息2"},
    {SessionID: "s1", Role: "user", Content: "消息3"},
}
mem.AddBatch(ctx, episodes)

// 批量获取
results, _ := mem.GetBatch(ctx, []string{"id1", "id2", "id3"})

// 批量删除
mem.DeleteBatch(ctx, []string{"id1", "id2"})
```

## 导入导出

```go
// 导出
data, _ := mem.ExportMemories(ctx, "session-1", "json")
os.WriteFile("export.json", data, 0644)

// 导入
data, _ := os.ReadFile("export.json")
count, _ := mem.ImportMemories(ctx, data, "json")
fmt.Printf("导入了 %d 条记忆\n", count)
```

## 最佳实践

1. **生产环境用 SQLite + WAL**：`mem.WithWAL()` 并发读写不阻塞
2. **设置清理策略**：`mem.WithCleanup(30)` 防止记忆无限增长
3. **RAG 用 RRF 融合**：`FusionRRF` 对量纲差异鲁棒
4. **批量操作**：使用 `AddBatch` 减少数据库往返
5. **重要性评分**：为关键信息设置高重要性，优先检索

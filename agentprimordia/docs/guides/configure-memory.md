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

    agent := ap.NewAgent("assistant", "你是助手", provider,
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
    memory, _ := ap.NewSQLiteMemory(memory.SQLiteConfig{
        Path: "./data/memory.db",
        FTS5: true,  // 启用全文搜索
        WAL:  true,  // 启用 WAL 模式（并发读写）
    })
    defer memory.Close()

    agent := ap.NewAgent("assistant", "你是助手", provider,
        ap.WithMemory(memory),
    )
    ```

=== "TypeScript"

    ```typescript
    import { SqliteStore } from '@agentprimordia/sdk';
    // 需要先安装：npm install better-sqlite3

    const memory = new SqliteStore({
      path: './data/memory.db',
      fts5: true,  // 启用全文搜索
      wal:  true,  // 启用 WAL 模式
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

    // 存储记忆
    memory.Store(ctx, "user:1:name", "Alice")
    memory.Store(ctx, "user:1:email", "alice@example.com")

    // 检索
    value, _ := memory.Load(ctx, "user:1:name")
    // value = "Alice"

    // 全文搜索
    results, _ := memory.Search(ctx, "Agent", 10)
    ```

=== "TypeScript"

    ```typescript
    // 存储记忆
    await memory.store('user:1:name', 'Alice');
    await memory.store('user:1:email', 'alice@example.com');

    // 检索
    const value = await memory.load('user:1:name');
    // value = "Alice"

    // 全文搜索
    const results = await memory.search('Agent', 10);
    ```

## 向量存储

用于语义搜索和 RAG 管道：

=== "Go"

    ```go
    // 创建向量存储
    vectorStore := memory.NewVectorStore(memory.VectorConfig{
        Dimensions: 1536,  // 嵌入维度
        Index:      "hnsw",
        HNSWConfig: memory.HNSWConfig{
            M:              16,
            EFConstruction: 200,
            EFSearch:       100,
        },
    })
    defer vectorStore.Close()

    // 存储向量
    embedding := []float32{0.1, 0.2, ...}
    vectorStore.Store(ctx, "doc:1", embedding, map[string]string{
        "title":   "Agent 架构设计",
        "content": "本文介绍了 Agent 的核心架构...",
    })

    // 语义搜索
    results, _ := vectorStore.Search(ctx, queryEmbedding, 5)
    ```

=== "TypeScript"

    ```typescript
    import { VectorStore, HNSW } from '@agentprimordia/sdk';

    // 创建向量存储
    const vectorStore = new VectorStore({
      dimensions: 1536,
      index: new HNSW({ m: 16, efConstruction: 200, efSearch: 100 }),
    });

    // 存储向量
    const embedding = [0.1, 0.2, /* ... */];
    await vectorStore.store('doc:1', embedding, {
      title: 'Agent 架构设计',
      content: '本文介绍了 Agent 的核心架构...',
    });

    // 语义搜索
    const results = await vectorStore.search(queryEmbedding, 5);
    ```

## RAG Pipeline

### 基础配置

=== "Go"

    使用 `WithRAGMemory()` 一步完成 RAG 组装（v0.8.0+ 推荐）：

    ```go
    agent := ap.NewAgent("assistant", "你是助手", provider,
        ap.WithMaxTurns(10),
    ).WithRAGMemory(ragStore)  // 自动组装 EmbeddingAdapter + RAGStore + RAGProvider
    ```

    手动配置 RAG：

    ```go
    ragStore := memory.NewRAGStore(mem, vectorStore, embedder)

    // 支持 RRF 融合模式（推荐生产使用）
    ragStore = memory.NewRAGStoreWithFusionConfig(
        mem, vectorStore, embedder,
        memory.RAGFusionConfig{
            FusionMode:    memory.FusionRRF,
            RRFK:          60,
            OverFetchSize: 5,
        },
    )

    agent := ap.NewAgent("assistant", "你是助手", provider,
        ap.WithMaxTurns(10),
        ap.WithRAG(ap.RAGConfig{
            Provider: ragStore,
            Mode:     ap.RAGModeAuto, // 每轮自动注入
            TopK:     5,
        }),
    )
    ```

=== "TypeScript"

    ```typescript
    import { RAGStore } from '@agentprimordia/sdk';

    const ragStore = new RAGStore({
      memory: sqliteStore,
      vectorStore,
      embedder,
      topK: 5,
    });

    const agent = new ReActAgent({
      name: 'assistant',
      model: provider,
      toolkit: new ToolRegistry(),
      maxTurns: 10,
      ragStore,
    });
    ```

### 添加文档

=== "Go"

    ```go
    err := ragStore.AddDocument(ctx, memory.Document{
        ID:      "doc:1",
        Title:   "Agent 最佳实践",
        Content: "设计 Agent 时应该遵循以下原则...",
        Tags:    []string{"agent", "best-practice"},
    })
    ```

=== "TypeScript"

    ```typescript
    await ragStore.addDocument({
      id: 'doc:1',
      title: 'Agent 最佳实践',
      content: '设计 Agent 时应该遵循以下原则...',
      tags: ['agent', 'best-practice'],
    });
    ```

### RRF 融合模式

RAG 混合检索支持两种融合模式（Go SDK 独有特性）：

| 模式 | 说明 | 适用场景 |
|------|------|----------|
| `FusionLinear` | 线性加权融合（默认） | 向量和 FTS 分数量纲一致 |
| `FusionRRF` | Reciprocal Rank Fusion | 生产推荐，对量纲差异鲁棒 |

```go
// 运行时切换融合模式
ragStore.SetFusionConfig(memory.RAGFusionConfig{
    FusionMode: memory.FusionRRF,
    RRFK:       60,
})
```

## 记忆清理

### 自动清理

=== "Go"

    ```go
    // 使用默认清理配置（30 天过期、24 小时间隔）
    cleanupCfg := ap.DefaultCleanupConfig()

    // 或自定义
    memory.StartAutoCleanup(ctx, memory.CleanupConfig{
        MaxAge:          30 * 24 * time.Hour,
        CleanupInterval: 1 * time.Hour,
    })
    ```

=== "TypeScript"

    ```typescript
    // 自动清理（默认 30 天过期、每小时清理）
    memory.startAutoCleanup({
      maxAge: 30 * 24 * 60 * 60 * 1000, // 30 天（毫秒）
      cleanupInterval: 60 * 60 * 1000,   // 1 小时
    });
    ```

## 性能优化

### 启用 WAL 模式

=== "Go"

    ```go
    memory, _ := ap.NewSQLiteMemory(memory.SQLiteConfig{
        Path: "./data/memory.db",
        WAL:  true,  // 并发读写性能提升
    })
    ```

=== "TypeScript"

    ```typescript
    const memory = new SqliteStore({
      path: './data/memory.db',
      wal: true,  // 并发读写性能提升
    });
    ```

### 批量操作

=== "Go"

    ```go
    items := make([]memory.MemoryItem, 1000)
    // 填充 items...
    memory.BatchStore(ctx, items) // 比单条存储快 10-100x
    ```

=== "TypeScript"

    ```typescript
    const items = Array.from({ length: 1000 }, (_, i) => ({
      key: `key${i}`,
      value: `value${i}`,
    }));
    await memory.batchStore(items); // 比单条存储快 10-100x
    ```

## 最佳实践

1. **选择合适的存储层**：开发用 InMemory，生产用 SQLite，语义搜索用 Vector Store
2. **启用 WAL 模式**：提高并发读写性能
3. **设置清理策略**：防止记忆无限增长
4. **使用标签分类**：便于检索和管理
5. **批量操作**：减少数据库往返
6. **RAG 使用 RRF 融合**：生产环境推荐 `FusionRRF` 模式

## 下一步

- 查看 [RAG 概念](../concepts/rag.md) 了解检索增强生成原理
- 阅读 [记忆 API](../api/memory.md) 了解完整接口定义
- 学习 [性能优化](../advanced/performance.md) 了解更多优化技巧

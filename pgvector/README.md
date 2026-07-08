# pgvector Provider for AgentPrimordia

[PostgreSQL](https://www.postgresql.org/) + [pgvector](https://github.com/pgvector/pgvector) 向量存储提供者。

## 功能

- 向量 CRUD（增删改查）
- KNN 搜索（余弦距离 / L2 距离 / 内积）
- JSONB 元数据存储
- 多租户命名空间隔离
- HNSW 与 IVFFlat 索引支持

## 前置条件

1. PostgreSQL 13+ 已安装
2. 启用 pgvector 扩展：`CREATE EXTENSION IF NOT EXISTS vector;`
3. 连接字符串：`postgres://user:password@host:5432/dbname`

## 使用方式

```go
import "agentprimordia/pgvector"

cfg := pgvector.Config{
    ConnString:    "postgres://localhost:5432/ap",
    TableName:     "agent_memory",
    Dimensions:    1536,     // OpenAI text-embedding-3-large
    Distance:      pgvector.CosineDistance,
    IndexType:     pgvector.HNSWIndex,
    MaxConnections: 10,
}

store, err := pgvector.New(context.Background(), cfg)
if err != nil { log.Fatal(err) }
defer store.Close()

// 添加向量
err = store.Add(ctx, "ep_1", []float32{0.1, 0.2, ...}, map[string]string{"tenant": "t1"})

// 搜索
results, err := store.Search(ctx, queryVec, 10, map[string]string{"tenant": "t1"})
```

## 与 memory.RAGStore 集成

```go
import "agentprimordia/internal/memory"

// 替换默认向量存储
vecStore := memory.NewVectorStoreWithHNSW(1536, memory.HNSWConfig{...})
// pgVector 作为底层存储
rag := memory.NewRAGStore(mem, embedder)
```

## 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `PGVECTOR_CONN_STRING` | — | PostgreSQL 连接字符串 |
| `PGVECTOR_TABLE` | `agent_vectors` | 表名 |
| `PGVECTOR_DIMENSIONS` | `16` | 向量维度 |

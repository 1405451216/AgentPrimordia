# Memory API 参考

> `package memory` — 记忆存储接口与实现。

## Memory 接口

```go
type Memory interface {
    Add(ctx context.Context, episode *Episode) error
    Get(ctx context.Context, id string) (*Episode, error)
    Search(ctx context.Context, query string, limit int) ([]*Episode, error)
    Delete(ctx context.Context, id string) error
    List(ctx context.Context, opts ListOptions) ([]*Episode, error)
    Close() error
}
```

## Episode 类型

```go
type Episode struct {
    ID        string
    Role      string         // system / user / assistant / tool
    Content   string
    Metadata  map[string]any // 租户 ID、session ID、来源等
    Embedding []float32      // 向量（可选，用于 Vector backend）
    CreatedAt time.Time
}
```

## 后端实现

### InMemory

```go
func NewInMemoryMemory() *InMemory
```

适用于测试、无状态场景。数据仅存进程内存。

### SQLite

```go
func NewSQLiteMemory(path string) (*SQLite, error)
```

单机持久化，自动建表迁移，支持数百万条记录。

### Vector

```go
type VectorConfig struct {
    DBPath   string
    Dim      int       // 嵌入向量维度
    IndexType IndexType // BruteForce / HNSW / IVFFlat
    Embedder Embedder  // 嵌入模型
}

func NewVectorMemory(cfg VectorConfig) (*Vector, error)
```

语义检索后端，支持 HNSW 索引加速。

## 检索选项

```go
type SearchOptions struct {
    Limit     int
    MinScore  float64            // 最低相似度阈值
    Filters   map[string]any     // 元数据过滤（如 tenant_id）
    Since     time.Time          // 时间范围
    TenantID  string             // 多租户隔离（可选）
}

// 使用示例
results, _ := mem.Search(ctx, "RAG 是什么", 5)
results, _ := mem.Search(ctx, "配置方法", 5, memory.WithTenant("tenant-a"))
results, _ := mem.Search(ctx, "最新对话", 10, memory.WithSince(time.Now().Add(-24*time.Hour)))
```

## 嵌入接口

```go
type Embedder interface {
    Embed(ctx context.Context, texts []string) ([][]float32, error)
    Dim() int
}

// 内置实现
func NewOpenAIEmbedder() Embedder           // OpenAI text-embedding-3-small
func NewONNXEmbedder(modelPath string) Embedder  // 本地 ONNX 嵌入
```

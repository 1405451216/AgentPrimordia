# Memory API

记忆系统 API 参考文档。

## Memory 接口

```go
type Memory interface {
    // Store 存储记忆
    Store(ctx context.Context, key string, value interface{}, opts ...StoreOption) error
    
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

## MemoryItem 结构

```go
type MemoryItem struct {
    // Key 键
    Key string
    
    // Value 值
    Value interface{}
    
    // Tags 标签
    Tags []string
    
    // Importance 重要性（0-1）
    Importance float64
    
    // CreatedAt 创建时间
    CreatedAt time.Time
    
    // UpdatedAt 更新时间
    UpdatedAt time.Time
    
    // Metadata 元数据
    Metadata map[string]interface{}
}
```

## StoreOption

存储选项：

```go
type StoreOption func(*StoreConfig)

func WithTags(tags []string) StoreOption
func WithImportance(importance float64) StoreOption
func WithMetadata(metadata map[string]interface{}) StoreOption
```

**示例：**
```go
mem.Store(ctx, "key", "value",
    memory.WithTags([]string{"tag1", "tag2"}),
    memory.WithImportance(0.9),
    memory.WithMetadata(map[string]interface{}{
        "source": "user",
    }),
)
```

## SQLiteMemory

SQLite 记忆实现：

### NewSQLiteMemory

```go
func NewSQLiteMemory(config SQLiteConfig) (*SQLiteMemory, error)
```

**SQLiteConfig 结构：**
```go
type SQLiteConfig struct {
    // Path 数据库路径
    Path string
    
    // FTS5 启用全文搜索
    FTS5 bool
    
    // WAL 启用 WAL 模式
    WAL bool
    
    // MaxOpenConns 最大打开连接数
    MaxOpenConns int
    
    // MaxIdleConns 最大空闲连接数
    MaxIdleConns int
}
```

**示例：**
```go
mem, err := memory.NewSQLiteMemory(memory.SQLiteConfig{
    Path:         "./data/memory.db",
    FTS5:         true,
    WAL:          true,
    MaxOpenConns: 10,
    MaxIdleConns: 5,
})
```

### WithCleanup

配置自动清理：

```go
func (m *SQLiteMemory) WithCleanup(config CleanupConfig) *SQLiteMemory
```

**CleanupConfig 结构：**
```go
type CleanupConfig struct {
    // MaxAge 最大年龄
    MaxAge time.Duration
    
    // MaxSize 最大数量
    MaxSize int
    
    // ImportanceThreshold 重要性阈值
    ImportanceThreshold float64
    
    // CleanupInterval 清理间隔
    CleanupInterval time.Duration
}
```

**示例：**
```go
mem := memory.NewSQLiteMemory(config).
    WithCleanup(memory.CleanupConfig{
        MaxAge:              30 * 24 * time.Hour,
        MaxSize:             10000,
        ImportanceThreshold: 0.3,
        CleanupInterval:     1 * time.Hour,
    })
```

### WithCache

配置缓存：

```go
func (m *SQLiteMemory) WithCache(cache Cache) *SQLiteMemory
```

**示例：**
```go
mem := memory.NewSQLiteMemory(config).
    WithCache(memory.NewLRUCache(1000))
```

### BatchStore

批量存储：

```go
func (m *SQLiteMemory) BatchStore(ctx context.Context, items []MemoryItem) error
```

**示例：**
```go
items := []memory.MemoryItem{
    {Key: "key1", Value: "value1"},
    {Key: "key2", Value: "value2"},
}
err := mem.BatchStore(ctx, items)
```

### Transaction

事务操作：

```go
func (m *SQLiteMemory) Transaction(ctx context.Context, fn func(Transaction) error) error
```

**示例：**
```go
err := mem.Transaction(ctx, func(tx memory.Transaction) error {
    tx.Store(ctx, "key1", "value1")
    tx.Store(ctx, "key2", "value2")
    return nil
})
```

### SearchByTags

按标签搜索：

```go
func (m *SQLiteMemory) SearchByTags(ctx context.Context, tags []string, limit int) ([]MemoryItem, error)
```

**示例：**
```go
items, err := mem.SearchByTags(ctx, []string{"ai", "agent"}, 10)
```

### Cleanup

手动清理：

```go
func (m *SQLiteMemory) Cleanup(ctx context.Context) error
```

**示例：**
```go
err := mem.Cleanup(ctx)
```

### CreateIndex

创建索引：

```go
func (m *SQLiteMemory) CreateIndex(ctx context.Context, name string, column string) error
```

**示例：**
```go
err := mem.CreateIndex(ctx, "idx_user_id", "user_id")
```

## VectorStore

向量存储：

### NewVectorStore

```go
func NewVectorStore(config VectorConfig) *VectorStore
```

**VectorConfig 结构：**
```go
type VectorConfig struct {
    // Path 数据库路径
    Path string
    
    // Dimensions 向量维度
    Dimensions int
    
    // Index 索引类型: "flat", "hnsw"
    Index string
    
    // HNSWConfig HNSW 索引配置
    HNSWConfig HNSWConfig
}

type HNSWConfig struct {
    // M 每个节点最大连接数
    M int
    
    // EFConstruction 构建时搜索范围
    EFConstruction int
    
    // EFSearch 查询时搜索范围
    EFSearch int
}
```

**示例：**
```go
vs := memory.NewVectorStore(memory.VectorConfig{
    Path:       "./data/vectors.db",
    Dimensions: 1536,
    Index:      "hnsw",
    HNSWConfig: memory.HNSWConfig{
        M:              16,
        EFConstruction: 200,
        EFSearch:       100,
    },
})
```

### Store

存储向量：

```go
func (vs *VectorStore) Store(ctx context.Context, id string, embedding []float32, metadata map[string]string) error
```

**示例：**
```go
err := vs.Store(ctx, "doc:1", embedding, map[string]string{
    "title":   "文档标题",
    "content": "文档内容",
})
```

### Search

语义搜索：

```go
func (vs *VectorStore) Search(ctx context.Context, query []float32, limit int) ([]VectorItem, error)
```

**VectorItem 结构：**
```go
type VectorItem struct {
    ID       string
    Score    float64
    Metadata map[string]string
}
```

**示例：**
```go
results, err := vs.Search(ctx, queryEmbedding, 5)
for _, r := range results {
    fmt.Printf("ID: %s, Score: %f\n", r.ID, r.Score)
}
```

## RAGPipeline

RAG 管道：

### NewRAGPipeline

```go
func NewRAGPipeline(config RAGConfig) *RAGPipeline
```

**RAGConfig 结构：**
```go
type RAGConfig struct {
    // Memory 记忆存储
    Memory Memory
    
    // VectorStore 向量存储
    VectorStore *VectorStore
    
    // Embedder 嵌入模型
    Embedder llm.Embedder
    
    // TopK 检索数量
    TopK int
    
    // SystemPrompt 系统提示
    SystemPrompt string
}
```

**示例：**
```go
rag := memory.NewRAGPipeline(memory.RAGConfig{
    Memory:      sqliteMem,
    VectorStore: vectorStore,
    Embedder:    openaiEmbedder,
    TopK:        5,
    SystemPrompt: "基于以下参考资料回答问题。",
})
```

### AddDocument

添加文档：

```go
func (r *RAGPipeline) AddDocument(ctx context.Context, doc Document) error
```

**Document 结构：**
```go
type Document struct {
    ID      string
    Title   string
    Content string
    Tags    []string
}
```

**示例：**
```go
err := rag.AddDocument(ctx, memory.Document{
    ID:      "doc:1",
    Title:   "Agent 架构设计",
    Content: "本文介绍了 Agent 的核心架构...",
    Tags:    []string{"agent", "architecture"},
})
```

### Query

RAG 查询：

```go
func (r *RAGPipeline) Query(ctx context.Context, question string) (string, error)
```

**示例：**
```go
answer, err := rag.Query(ctx, "如何设计高可用的 Agent 系统？")
```

## SessionMemory

会话记忆：

### NewSessionMemory

```go
func NewSessionMemory() *SessionMemory
```

**示例：**
```go
sessionMem := memory.NewSessionMemory()
```

### Store

存储会话：

```go
func (sm *SessionMemory) Store(ctx context.Context, sessionID string, messages []Message) error
```

**Message 结构：**
```go
type Message struct {
    Role    string
    Content string
}
```

**示例：**
```go
err := sessionMem.Store(ctx, "conversation:1", []memory.Message{
    {Role: "user", Content: "你好"},
    {Role: "assistant", Content: "你好！"},
})
```

### GetConversation

获取会话历史：

```go
func (sm *SessionMemory) GetConversation(ctx context.Context, sessionID string) ([]Message, error)
```

**示例：**
```go
messages, err := sessionMem.GetConversation(ctx, "conversation:1")
```

## Cache 接口

```go
type Cache interface {
    // Get 获取缓存
    Get(key string) (interface{}, bool)
    
    // Set 设置缓存
    Set(key string, value interface{})
    
    // Delete 删除缓存
    Delete(key string)
    
    // Clear 清空缓存
    Clear()
}
```

### NewLRUCache

```go
func NewLRUCache(size int) Cache
```

**示例：**
```go
cache := memory.NewLRUCache(1000)
```

## 错误定义

```go
var (
    // ErrKeyNotFound 键未找到
    ErrKeyNotFound = errors.New("key not found")
    
    // ErrDatabaseClosed 数据库已关闭
    ErrDatabaseClosed = errors.New("database closed")
    
    // ErrInvalidDimension 维度无效
    ErrInvalidDimension = errors.New("invalid dimension")
    
    // ErrIndexNotReady 索引未就绪
    ErrIndexNotReady = errors.New("index not ready")
)
```

## 完整示例

```go
package main

import (
    "context"
    "fmt"
    "log"
    
    "agentprimordia.dev/agentprimordia/pkg/memory"
)

func main() {
    // 创建 SQLite 记忆
    mem, err := memory.NewSQLiteMemory(memory.SQLiteConfig{
        Path: "./data/memory.db",
        FTS5: true,
        WAL:  true,
    })
    if err != nil {
        log.Fatal(err)
    }
    defer mem.Close()
    
    ctx := context.Background()
    
    // 存储记忆
    err = mem.Store(ctx, "user:1:name", "Alice",
        memory.WithTags([]string{"user", "name"}),
        memory.WithImportance(0.9),
    )
    if err != nil {
        log.Fatal(err)
    }
    
    // 加载记忆
    value, err := mem.Load(ctx, "user:1:name")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("姓名: %v\n", value)
    
    // 搜索记忆
    items, err := mem.Search(ctx, "Alice", 10)
    if err != nil {
        log.Fatal(err)
    }
    for _, item := range items {
        fmt.Printf("找到: %s = %v\n", item.Key, item.Value)
    }
}
```

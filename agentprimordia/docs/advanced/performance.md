# 性能优化

本指南介绍如何优化 AgentPrimordia 应用的性能。

## LLM 优化

### 使用 ResilientProvider

内置重试、熔断和降级，提高可用性：

```go
resilient := llm.NewResilientProvider(baseProvider, llm.ResilientConfig{
    MaxRetries:     3,
    RetryDelay:     time.Second,
    CircuitBreaker: true,
    FallbackProvider: fallbackLLM,
})
```

### 缓存响应

对相同输入缓存 LLM 响应：

```go
cachedLLM := llm.NewCachedProvider(baseProvider, llm.CacheConfig{
    MaxSize: 1000,
    TTL:     1 * time.Hour,
})
```

### 流式输出

减少首字延迟：

```go
req := llm.Request{
    Messages: messages,
    Stream:   true,
}

stream, err := provider.CompleteStream(ctx, req)
for chunk := range stream {
    fmt.Print(chunk.Content)
}
```

### 批量嵌入

批量处理减少 API 调用：

```go
texts := []string{"文本1", "文本2", "文本3"}
embeddings, err := embedder.EmbedBatch(ctx, texts)
```

## 记忆优化

### 启用 WAL 模式

提高并发读写性能：

```go
mem := memory.NewSQLiteMemory(memory.SQLiteConfig{
    Path: "./data/memory.db",
    WAL:  true,
})
```

### 使用缓存

热点数据缓存减少数据库查询：

```go
mem := memory.NewSQLiteMemory(config).
    WithCache(memory.NewLRUCache(10000))
```

### 批量操作

批量存储减少数据库往返：

```go
items := make([]memory.MemoryItem, 1000)
// 填充 items
err := mem.BatchStore(ctx, items)
```

### 优化索引

为常用查询字段创建索引：

```go
mem.CreateIndex(ctx, "idx_user_id", "user_id")
mem.CreateIndex(ctx, "idx_created_at", "created_at")
```

### HNSW 索引优化

调整 HNSW 参数平衡性能和准确率：

```go
vectorStore := memory.NewVectorStore(memory.VectorConfig{
    Index: "hnsw",
    HNSWConfig: memory.HNSWConfig{
        M:              16,   // 增加 M 提高准确率，但增加内存
        EFConstruction: 200,  // 增加提高构建质量
        EFSearch:       100,  // 增加提高搜索准确率
    },
})
```

## Agent 优化

### 控制迭代次数

防止无限循环：

```go
agent := agent.NewAgent(llm, tools).
    WithMaxIterations(10)
```

### 设置超时

防止长时间阻塞：

```go
agent := agent.NewAgent(llm, tools).
    WithTimeout(60 * time.Second)
```

### 并行工具调用

独立工具并行执行：

```go
// Agent 自动并行执行多个工具调用
actions := []string{"tool1", "tool2", "tool3"}
results := agent.ActParallel(ctx, actions)
```

### 减少上下文长度

精简消息历史：

```go
// 只保留最近 N 条消息
messages := messages[len(messages)-10:]
```

## 编排优化

### 控制并发数

防止资源耗尽：

```go
orch := orchestration.NewParallelOrchestrator(agents).
    WithMaxConcurrency(10)
```

### DAG 并行执行

自动识别可并行的节点：

```go
dag := orchestration.NewDAGOrchestrator().
    WithMaxConcurrency(10)
```

### 避免重复执行

缓存中间结果：

```go
// 使用共享状态存储中间结果
state := orchestration.NewSharedState()
state.Set("cached_result", result)
```

## 工具优化

### 超时控制

长时间运行的工具支持超时：

```go
func (t *MyTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
    ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
    defer cancel()
    // ...
}
```

### 连接池

复用 HTTP 连接：

```go
client := &http.Client{
    Transport: &http.Transport{
        MaxIdleConns:        100,
        MaxIdleConnsPerHost: 10,
        IdleConnTimeout:     90 * time.Second,
    },
}
```

### 批量处理

批量处理减少调用次数：

```go
// 不推荐：循环调用
for _, item := range items {
    process(item)
}

// 推荐：批量处理
processBatch(items)
```

## 监控与调优

### Prometheus 指标

```go
var (
    agentDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "agent_request_duration_seconds",
            Help:    "Duration of agent requests",
            Buckets: prometheus.DefBuckets,
        },
        []string{"method"},
    )
    
    llmTokens = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "llm_tokens_total",
            Help: "Total number of LLM tokens used",
        },
        []string{"provider", "type"},
    )
)
```

### pprof 性能分析

```go
import _ "net/http/pprof"

// 启动 pprof
go func() {
    http.ListenAndServe(":6060", nil)
}()
```

访问 `http://localhost:6060/debug/pprof/` 查看性能数据。

### 内存分析

```bash
# 获取内存 profile
go tool pprof http://localhost:6060/debug/pprof/heap

# 查看内存分配
(pprof) top 10
(pprof) web
```

### CPU 分析

```bash
# 获取 CPU profile（30秒）
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30

# 查看 CPU 使用
(pprof) top 10
(pprof) web
```

## 基准测试

### 编写基准测试

```go
func BenchmarkAgentRun(b *testing.B) {
    agent := setupAgent()
    ctx := context.Background()
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        agent.Run(ctx, "测试输入")
    }
}
```

### 运行基准测试

```bash
go test -bench=. -benchmem -benchtime=10s
```

### 比较基准测试

```bash
# 保存基准结果
go test -bench=. -benchmem > old.txt

# 修改代码后再次测试
go test -bench=. -benchmem > new.txt

# 比较结果
benchstat old.txt new.txt
```

## 性能检查清单

### 启动时

- [ ] 启用 WAL 模式
- [ ] 配置连接池
- [ ] 设置合理的缓存大小
- [ ] 配置 ResilientProvider

### 运行时

- [ ] 监控内存使用
- [ ] 监控 CPU 使用
- [ ] 监控 LLM token 使用
- [ ] 监控数据库查询耗时

### 定期维护

- [ ] 清理过期记忆
- [ ] 重建索引
- [ ] 分析性能瓶颈
- [ ] 更新依赖

## 常见性能问题

### 1. LLM 调用慢

**原因：**
- 网络延迟
- 模型过大
- 输入过长

**解决方案：**
- 使用更快的模型（如 GPT-3.5）
- 减少输入长度
- 使用缓存
- 使用流式输出

### 2. 内存泄漏

**原因：**
- Agent 未正确关闭
- 缓存无限增长
- 数据库连接未释放

**解决方案：**
- 确保调用 Close 方法
- 设置缓存大小限制
- 使用连接池

### 3. 数据库锁

**原因：**
- 并发写入冲突
- 长时间事务

**解决方案：**
- 启用 WAL 模式
- 减少事务持续时间
- 优化查询

### 4. 工具执行超时

**原因：**
- 外部服务慢
- 网络问题

**解决方案：**
- 设置合理超时
- 实现重试机制
- 使用熔断器

## 下一步

- 阅读 [安全最佳实践](security.md) 了解安全加固
- 学习 [自定义 Provider](custom-provider.md) 优化 LLM 调用
- 查看 [部署到生产](../guides/deployment.md) 了解生产环境配置

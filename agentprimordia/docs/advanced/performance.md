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

项目内置 pprof 端点注册（`internal/health/pprof.go`），推荐使用：

```go
mux := http.NewServeMux()
// 注册健康检查和业务路由
hc := ap.NewHealthChecker()
mux.Handle("/healthz", hc)

// 注册 pprof 端点（所有标准 profile 类型）
ap.RegisterPProf(mux)

// 生产环境仅监听 localhost，避免暴露进程内部信息
go http.ListenAndServe("127.0.0.1:6060", mux)
```

访问 `http://localhost:6060/debug/pprof/` 查看性能数据。

也可使用独立 Handler：
```go
go http.ListenAndServe("127.0.0.1:6060", ap.PProfHandler())
```

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

## Go 运行时调优（GC）

Go 的并发标记-清除垃圾回收器可通过环境变量调优。在生产环境中，
合理的 GC 配置可显著降低内存使用和延迟尾延迟。

### GOGC：控制 GC 触发频率

`GOGC` 控制堆增长率触发阈值（默认 100，表示堆翻倍时触发 GC）：

| GOGC | 效果 | 适用场景 |
|------|------|--------|
| 50 | 更频繁 GC，内存占用低，CPU 开销高 | 内存受限环境 |
| 100 | 默认值，平衡内存与 CPU | 通用场景 |
| 200 | 更少 GC，内存占用高，CPU 开销低 | 延迟敏感、内存充裕 |
| off | 禁用 GC（不推荐生产使用） | 短命批处理任务 |

```bash
# 降低内存占用（更频繁 GC）
GOGC=50 ./ap

# 降低 CPU 开销（更少 GC）
GOGC=200 ./ap
```

### GOMEMLIMIT：设置软内存上限

> Go 1.19+ 引入。与 `GOGC` 互补，设置 Go 运行时的软内存上限。

`GOMEMLIMIT` 是软限制——运行时会尽量在不超过此值的情况下运行 GC，
但在内存压力下可能超过（不像 cgroup 的硬限制会 OOM Kill）。

```bash
# 限制堆内存为 2 GiB
GOMEMLIMIT=2GiB ./ap

# 与 GOGC 配合使用
GOMEMLIMIT=4GiB GOGC=200 ./ap
```

### 容器环境推荐配置

在 Docker / Kubernetes 中部署时，建议根据容器内存限制设置 `GOMEMLIMIT`：

```yaml
# docker-compose.yml
services:
  ap:
    environment:
      - GOMEMLIMIT=1500MiB  # 容器限制 2GiB 的 75%
      - GOGC=100              # 默认值
    deploy:
      resources:
        limits:
          memory: 2G
```

```yaml
# Kubernetes Deployment
spec:
  containers:
  - name: ap
    env:
    - name: GOMEMLIMIT
      value: "1500MiB"
    resources:
      limits:
        memory: "2Gi"
```

> **经验法则**：`GOMEMLIMIT` 设为容器内存限制的 75-80%，预留空间给
> 非 Go 堆内存（goroutine 栈、CGO、mmap 等）。

### runtime.SetMemoryLimit（代码内设置）

也可在代码中通过 `debug.SetMemoryLimit` 设置：

```go
import "runtime/debug"

func init() {
    // 设置 2 GiB 软内存上限
    debug.SetMemoryLimit(2 * 1024 * 1024 * 1024)
    // 等价于 GOMEMLIMIT=2GiB
}
```

### GC 调优验证

通过 pprof 的 heap 端点验证 GC 效果：

```bash
# 查看堆分配
go tool pprof http://localhost:6060/debug/pprof/heap

# 查看 GC 统计
curl http://localhost:6060/debug/pprof/heap?gc=1 | go tool pprof -text
```

使用 `GODEBUG=gctrace=1` 观察 GC 日志：

```bash
# 打印每次 GC 的时间、暂停时间和堆大小
GODEBUG=gctrace=1 ./ap 2>gc.log
```

输出示例：
```
gc 1 @0.045s 1%: 0.013+0.36+0.022 ms clock, 0.10+0.17/0.30/0.65+0.18 ms cpu, 4->4->2 MB, 5 MB goal, 0 MB stacks, 0 MB globals, 8 P
gc 2 @0.082s 1%: 0.004+0.27+0.016 ms clock, ...
```

关键字段：
- `4->4->2 MB`：GC 前堆大小 -> GC 后存活堆大小 -> 当前堆大小
- `5 MB goal`：下次 GC 触发的堆目标（受 GOGC 控制）
- `8 P`：逻辑处理器数量（GOMAXPROCS）

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
- [ ] 设置 GOMEMLIMIT（容器内存限制的 75-80%）
- [ ] 评估 GOGC 调整（延迟敏感场景考虑 GOGC=200）
- [ ] 注册 pprof 端点（仅 localhost）

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

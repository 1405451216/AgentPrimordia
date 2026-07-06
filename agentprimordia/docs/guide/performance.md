# 性能调优指南

> 让 AgentPrimordia 支撑高并发、低延迟的生产场景。

## 性能基线

在标准测试环境（AWS c5.2x4large, Go 1.26）：

| 指标 | 基线值 |
|------|--------|
| Agent 启动（冷启动） | ~5ms |
| Agent 启动（热启动） | <1ms |
| ReAct 单步（无工具调用） | 取决于 LLM 延迟 |
| ReAct 单步（带工具调用） | LLM 延迟 + ~0.5ms 工具调度 |
| Memory 检索（SQLite, 100 万条） | <2ms P99 |
| Memory 检索（pgvector, HNSW） | <5ms P99 |
| Pool 调度 1000 QPS | <1ms P99 |

## LLM 调用优化

### LLM 缓存

相同 prefix 的请求自动缓存：

```go
llmCfg := ap.LLMConfig{
    CacheEnabled: true,
    CacheBackend:  ap.RedisCache("localhost:6379"), // 或 ap.InMemoryCache()
    CacheTTL:      1 * time.Hour,
}
```

命中率通常可达 30-60%（system prompt 相同）。

### 模型路由

根据 prompt 长度等特征自动选择模型：

```go
router := ap.NewModelRouter(ap.RouterConfig{
    Rules: []ap.RouteRule{
        {MaxTokens: 1000, Model: "gpt-4o-mini"},    // 短任务用便宜模型
        {MinTokens: 1000, Model: "gpt-4o"},          // 长任务用强模型
    },
    Fallback: "gpt-4o",
})
```

## Memory 性能

### HNSW 索引

向量检索加速（150x 暴力搜索）：

```go
mem, _ := ap.NewVectorMemory(ap.VectorConfig{
    IndexType: ap.IndexHNSW,
    HNSWM:     16,       // 每层连接数
    HNSWEfConstruction: 200,
    HNSWEfSearch: 50,
})
```

### 批量写入

高吞吐场景使用批量写入：

```go
writer := mem.BatchWriter(ap.BatchConfig{
    Size:     100,           // 攒够 100 条写入
    FlushInterval: 1 * time.Second, // 或 1s 刷盘
})
writer.Write(episode)
```

## Pool 调优

```go
pool := ap.NewPool(ap.PoolConfig{
    MaxConcurrent: 100,        // 最大并发 Agent 数
    QueueSize:     10000,      // 任务队列深度
    WorkerIdleTTL: 5 * time.Minute, // 空闲 worker 存活时间
})
```

### 自动扩缩

```yaml
pool:
  min_workers: 2
  max_workers: 50
  scale_up_queue_depth: 100   # 队列超过 100 扩容
  scale_down_idle_seconds: 300 # 空闲 5 分钟缩容
```

## OpenTelemetry 追踪

全链路追踪定位瓶颈：

```go
import "agentprimordia/internal/otel"

tp, _ := otel.NewTracerProvider(ctx, otel.Config{
    ServiceName: "ap-agent",
    Endpoint:    "otel-collector:4317",
    SampleRate:  0.1,   // 10% 采样
})
```

Grafana 可直观看到：
- ReAct 每步耗时
- LLM 调用延迟分布
- 工具执行延迟分布
- Memory 检索延迟

## 性能分析

内置 pprof：

```
curl http://localhost:8080/debug/pprof/profile?seconds=30 > cpu.prof
go tool pprof cpu.prof

curl http://localhost:8080/debug/pprof/heap > heap.prof
go tool pprof heap.prof
```

## 常见瓶颈

| 现象 | 原因 | 解决 |
|------|------|------|
| 尾延迟高 | LLM 流式 chunk 延迟 | 启用首 token 缓存 |
| 内存增长 | Memory 未清理 | 开启 TTL 自动清理 |
| CPU 高 | JSON 序列化 | 启用 jsoniter / zerolog |
| 调度延迟 | Worker 不足 | 增加 MaxConcurrent 或扩容 |

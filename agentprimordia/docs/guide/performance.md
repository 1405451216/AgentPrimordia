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

使用 `CachedProvider` 包装 LLM Provider，对相同输入缓存响应：

```go
// 方式一：指纹精确匹配缓存（无需 Embedding）
cache := ap.NewFingerprintCache(10000, time.Hour)

// 方式二：语义相似度缓存（需提供向量化函数，相似度 >= 0.8 视为命中）
// cache := ap.NewInMemoryCache(embedFn, 10000, 0.8)

// 可选：混合缓存，先精确匹配再语义匹配
// hybrid, err := ap.NewHybridCache(fingerprint, semanticCache)

cached, err := ap.NewCachedProvider(provider, cache, 0.8)
if err != nil {
    log.Fatal(err)
}

agent, err := ap.NewAgent("assistant", "你是助手", cached, ap.WithMaxTurns(10))
```

命中率通常可达 30-60%（system prompt 相同）。

### 模型路由

框架内置成本感知模型路由器（`ModelRouter`），根据消息复杂度、上下文长度、
是否需要工具等指标，从多个注册的 Provider 中选择最优模型。路由策略为
`RouteStrategy` 枚举，取值：

- `StrategyCostFirst` — 优先选择成本最低的可用模型
- `StrategyQualityFirst` — 优先选择能力最强的模型（按 ComplexityLimit 倒序）
- `StrategyBalanced` — 综合评分（成本 + 能力）

```go
router := NewModelRouter(StrategyBalanced)
router.Register(ModelRouteConfig{
    Name:            "gpt-4o-mini",
    Provider:        miniProvider,
    CostPer1K:       0.00015,
    ComplexityLimit: 0.5,     // 可处理的复杂度上限 [0, 1]
    MaxContext:      128000,
    SupportsTools:   true,
})
router.Register(ModelRouteConfig{
    Name:            "gpt-4o",
    Provider:        strongProvider,
    ComplexityLimit: 1.0,
})
router.SetFallback("gpt-4o") // 兜底模型
```

> 注：`ModelRouter` 目前由框架 LLM 抽象层（`internal/llm`）实现，
> pkg 公共 API 尚未提供导出别名，以上为真实 API 形态，仅供框架内部
> 与扩展开发参考。

## Memory 性能

### HNSW 索引

向量检索加速（相比暴力搜索有数量级提升）。创建带 HNSW 索引的向量存储：

```go
store := ap.NewVectorStoreWithHNSW(1536, ap.HNSWConfig{
    MaxConnections: 16,  // 每层最大连接数 M（默认 16）
    EfConstruction: 200, // 构建时搜索范围（默认 200）
    EfSearch:       50,  // 查询时搜索范围（默认 50）
    Dimensions:     1536,
})
```

也可单独创建 HNSW 索引使用：

```go
index := ap.NewHNSWIndex(ap.HNSWConfig{
    MaxConnections: 16,
    EfConstruction: 200,
    EfSearch:       50,
    Dimensions:     1536,
})
```

## Pool 调优

```go
pool := ap.NewPool(ap.PoolConfig{
    MaxConcurrency:   100,            // 最大并发 Agent 数
    Timeout:          5 * time.Minute, // 单任务超时
    MaxRetainedTasks: 1000,           // 保留的已完成任务数上限（防长期运行内存泄漏）
})
pool.SetModel(provider)
defer pool.Close()
```

### 自动扩缩

自动扩缩容通过 `PoolConfig.AutoScaler` 字段（`*AutoScalerConfig`）启用，
关键参数：`MinConcurrency` / `MaxConcurrency`（并发度上下限）、
`ScaleUpThreshold`（利用率超过此值扩容，默认 0.8）、
`ScaleDownThreshold`（利用率低于此值缩容，默认 0.2）、
`CoolDownPeriod`（扩缩容冷却期）、`CheckInterval`（检查间隔）。

## OpenTelemetry 追踪

全链路追踪定位瓶颈。通过 pkg 公共 API 创建遥测提供者（`TelemetryConfig`
支持 `ServiceName` / `ServiceVersion` / `OTLPEndpoint` / `OTLPHeaders` /
`ExportInterval` / `EnableTraces` / `EnableMetrics`）：

```go
m := ap.NewMetrics()

tp, err := ap.NewTelemetryProvider(ap.TelemetryConfig{
    ServiceName:   "ap-agent",
    OTLPEndpoint:  "otel-collector:4317",
    EnableTraces:  true,
    EnableMetrics: true,
}, m)
if err != nil {
    log.Fatal(err)
}
defer tp.Shutdown()

// WithTelemetry 将 Tracer 与 Metrics 一次性注入 Agent
agent, err := ap.NewAgent("assistant", "你是助手", provider, ap.WithTelemetry(tp))
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

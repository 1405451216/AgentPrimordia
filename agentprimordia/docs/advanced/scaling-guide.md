# 性能扩展与瓶颈缓解指南

> 本文档说明 AgentPrimordia 的已知结构性性能瓶颈及其缓解路径。

## 瓶颈概览

| # | 瓶颈 | 影响场景 | 严重度 | 缓解方案 |
|---|------|----------|--------|----------|
| 1 | HNSW 全内存索引 | >1M 向量时内存压力 | 中 | pgvector/Qdrant/Milvus 外部后端 |
| 2 | SQLite 单写并发 | 高并发写入（>100 TPS） | 中 | WAL 模式 + Redis/etcd 分布式检查点 |
| 3 | Agent 单实例串行 Run | 单 Agent 吞吐量上限 | 低 | 设计使然，多实例 + Pool 为标准姿势 |

## 1. HNSW 内存压力

### 问题

内置 HNSW 索引（`internal/memory/hnsw.go`）将所有向量和图结构保存在内存中。
每个 1536 维 float32 向量占 ~6KB，加上 HNSW 图结构（M=16 时每节点约 256B），
1M 向量约需 **~6.5GB** 纯内存。

### 缓解

```
数据量 < 100K  →  内置 HNSW（默认，零配置）
数据量 100K-1M →  pgvector（PostgreSQL 扩展）
数据量 > 1M    →  Qdrant / Milvus（专用向量数据库）
```

详见 [向量存储选型指南](vector-store-selection.md)。

切换后端只需替换 `VectorStore` 接口实现，RAG 管道和 Agent 代码无需修改。

## 2. SQLite 单写并发

### 问题

SQLite 采用单写者模型（即使 WAL 模式下也仅允许一个写事务）。
当多个 Agent 同时写入记忆存储时，写操作会串行化。

### 缓解

**短期**（已内置）：
- WAL 模式（`journal_mode=WAL`）：读写并发，写-写串行
- `busy_timeout` 配置：避免立即 SQLITE_BUSY
- 批量写入：合并多条记忆为单事务

**中期**（已就绪）：
- Redis 检查点后端（`internal/persist/redis_checkpoint.go`，build tag `redis`）
- etcd 检查点后端（`internal/persist/etcd_checkpoint.go`，build tag `etcd`）

**长期**：
- 分片策略：按 Agent/Session 分配独立 SQLite 文件
- 写入队列：异步批量提交，减少锁竞争

### 配置示例

```go
// WAL 模式（默认已启用）
store, _ := memory.NewSqliteStore(memory.SqliteConfig{
    Path:       "agent_memory.db",
    WALMode:    true,
    BusyTimeout: 5 * time.Second,
})

// 高并发场景：使用 Redis 检查点
// go build -tags redis
checkpoint, _ := persist.NewRedisCheckpoint(persist.RedisConfig{
    Addr: "localhost:6379",
})
```

## 3. Agent 单实例串行

### 问题

`ReActAgent.Run()` 是同步阻塞调用，单个 Agent 实例同一时刻只能处理一个任务。
这是 **设计使然**（ReAct 循环需要维护对话上下文状态），而非缺陷。

### 标准姿势：Pool 多实例

```go
pool := ap.NewPool(ap.PoolConfig{
    MaxConcurrency: 10,        // 10 个 Agent 并行
    AgentFactory: func() ap.Agent {
        return ap.NewAgent(ap.AgentConfig{
            Name:  "worker",
            Model: "gpt-4",
        })
    },
})

// 分发任务
for _, task := range tasks {
    pool.Dispatch(ap.TaskConfig{
        ID:      task.ID,
        Message: task.Content,
    })
}
```

### 性能参考

| 模式 | 吞吐量 | 适用场景 |
|------|--------|----------|
| 单 Agent 串行 | ~1 req/s（受 LLM 延迟限制） | 交互式对话 |
| Pool 10 并发 | ~10 req/s | 批量任务 |
| Pool 50 并发 | ~50 req/s | 高吞吐后端 |
| 多节点集群 | 线性扩展 | 超大规模 |

### 进阶：集群水平扩展

当单机 Pool 无法满足时，使用 v3.1 集群能力：

```go
// etcd 服务发现 + gRPC 消息总线
cluster, _ := ap.NewCluster(ap.ClusterConfig{
    NodeID:   "node-1",
    EtcdEndpoints: []string{"localhost:2379"},
})
```

## 性能监控

使用内置 Prometheus 指标监控瓶颈：

```go
// 注册指标
metrics := ap.NewPrometheusMetrics()

// 关键指标
// ap_pool_running_tasks    — 当前运行任务数
// ap_pool_queued_tasks     — 排队任务数
// ap_pool_dropped_events   — 背压丢弃事件数
// ap_agent_run_duration    — Agent 运行耗时
// ap_memory_search_latency — 记忆搜索延迟
```

Grafana 仪表盘（`deploy/grafana/`）提供开箱即用的可视化面板。

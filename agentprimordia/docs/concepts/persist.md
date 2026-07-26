# 状态持久化（Persist）

Persist 模块提供 Agent 状态的检查点（Checkpoint）保存与恢复能力，支持分布式场景下的锁协调和 Leader 选举。

## 核心组件

### CheckpointStore

检查点存储接口，支持 Save / Load / List / Delete 操作：

```go
type CheckpointStore interface {
    Save(ctx context.Context, state *AgentState) error
    Load(ctx context.Context, agentID string) (*AgentState, error)
    List(ctx context.Context, sessionID string) ([]*AgentState, error)
    Delete(ctx context.Context, agentID string) error
}
```

内置实现：
- **SQLiteCheckpointStore**：基于 SQLite 的本地持久化（纯 Go，无 CGO）
- **InMemoryCheckpointStore**：内存实现，用于测试
- **DistributedCheckpointStore**：分布式包装器，结合 Coordinator 实现跨节点写锁

### Coordinator（分布式协调）

分布式锁接口，用于多节点间的写互斥：

```go
type Coordinator interface {
    Acquire(ctx context.Context, key string) (Lease, error)
    Release(ctx context.Context, lease Lease) error
    Owner(ctx context.Context, key string) (string, error)
}
```

内置实现：
- **MemoryCoordinator**：内存实现（单进程/测试）
- **FSCoordinator**：基于共享文件系统的原子锁（NFS/云盘）
- **EtcdCoordinator**：基于 etcd 的强一致锁（需 `etcd` build tag）
- **RedisCoordinator**：基于 Redis 的分布式锁（需 `redis` build tag）

### LeaderElector（Leader 选举）

基于 Coordinator 的 Leader 选举器，支持心跳续约、自动降级和可观测性指标：

```go
le := persist.NewLeaderElector(coord, "node-a", config)
le.OnEvent(func(e persist.LeaderEvent) { /* 状态变更回调 */ })
le.Start(ctx)
```

## 使用示例

```go
store, _ := persist.NewSQLiteCheckpointStore("checkpoints.db")
coord := persist.NewMemoryCoordinator("node-1", 30*time.Second)
dist := persist.NewDistributedCheckpointStore(store, coord, "node-1")

// 保存检查点
dist.Save(ctx, &persist.AgentState{
    AgentID:   "agent-1",
    SessionID: "sess-1",
    Status:    "running",
})

// 恢复
state, err := dist.Load(ctx, "agent-1")
```

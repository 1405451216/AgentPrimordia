# 检查点持久化

Persist 模块提供 Agent 状态检查点保存与恢复能力，支持断点续跑和容错。

## 核心接口

```go
type CheckpointStore interface {
    Save(ctx context.Context, state *AgentState) error
    Load(ctx context.Context, agentID string) (*AgentState, error)
    List(ctx context.Context, sessionID string) ([]*AgentState, error)
    Delete(ctx context.Context, agentID string) error
}
```

## 数据结构

```go
type AgentState struct {
    AgentID   string
    SessionID string
    Status    string
    Messages  []CheckpointMessage
    TurnCount int
    Metrics   CheckpointMetrics
    SavedAt   time.Time
}
```

## 快速开始

```go
store := persist.NewSQLiteCheckpointStore("./data/checkpoints.db")

agent := NewReActAgent(cfg).WithCheckpointStore(store)

// 自动保存：每轮结束后引擎自动调用 Save
// 手动恢复：
resp, err := agent.ResumeFromCheckpoint(ctx)
```

## SQLite 检查点存储

```go
store, err := persist.NewSQLiteCheckpointStore("./checkpoints.db")
if err != nil {
    panic(err)
}
defer store.Close()
```

## 使用场景

- 长任务断点续跑
- 服务重启后恢复会话
- 审计与调试回放

## 下一步

- 查看 [Admin API](advanced/admin-api.md) 获取检查点管理端点
- 了解 [Pool 调度](pool.md)

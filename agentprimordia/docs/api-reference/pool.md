# Pool API 参考

> `package pool` — 多 Agent 并发调度池。

## 类型定义

```go
type Pool struct{}

type PoolConfig struct {
    MaxConcurrency    int              `json:"max_concurrency"`             // 最大并发数
    Timeout           time.Duration    `json:"timeout"`                     // 超时时间
    RetryPolicy       RetryPolicy      `json:"retry_policy,omitempty"`      // 重试策略
    MaxRetainedTasks  int              `json:"max_retained_tasks,omitempty"` // 保留已完成任务数上限（0=不清理）
    DefaultAgent      ReActAgentConfig `json:"default_agent"`               // 默认 Agent 配置
    AutoScaler        *AutoScalerConfig `json:"auto_scaler,omitempty"`      // 自动扩缩容配置
}

type RetryPolicy struct {
    MaxRetries      int           `json:"max_retries"`
    Backoff         time.Duration `json:"backoff"`
    RetryableErrors []string      `json:"retryable_errors"`
}

type ReActAgentConfig struct {
    SystemPrompt string  `json:"system_prompt"`
    MaxTurns     int     `json:"max_turns"`
    Temperature  float64 `json:"temperature"`
}

type TaskConfig struct {
    ID         string            `json:"id"`
    Title      string            `json:"title"`
    Prompt     string            `json:"prompt"`
    SessionID  string            `json:"session_id,omitempty"`
    Tools      []tools.Tool      `json:"tools,omitempty"`
    FilesScope []string          `json:"files_scope,omitempty"`
    MaxTurns   int               `json:"max_turns,omitempty"`
    Metadata   map[string]string `json:"metadata,omitempty"`
}

type TaskResult struct {
    TaskID   string           `json:"task_id"`
    Task     TaskConfig       `json:"task"`
    Response *agent.Response  `json:"response,omitempty"`
    Error    error            `json:"error,omitempty"`
    Duration time.Duration    `json:"duration"`
    Status   PoolTaskStatus   `json:"status"`
}

type PoolTaskStatus string

const (
    PoolTaskQueued    PoolTaskStatus = "queued"
    PoolTaskRunning   PoolTaskStatus = "running"
    PoolTaskCompleted PoolTaskStatus = "completed"
    PoolTaskFailed    PoolTaskStatus = "failed"
    PoolTaskCancelled PoolTaskStatus = "cancelled"
)
```

## 构造函数

```go
func NewPool(config PoolConfig) *Pool
```

## 主要方法

| 方法 | 说明 |
|------|------|
| `Dispatch(ctx, tasks []TaskConfig) ([]*TaskResult, error)` | 批量分发任务 |
| `GetTask(taskID string) (TaskResult, bool)` | 获取任务状态 |
| `GetTasksBySession(sessionID string) []TaskResult` | 按会话查询任务 |
| `CancelBySession(sessionID string) error` | 取消会话下所有任务 |
| `Cancel(taskID string) error` | 取消单个任务 |
| `CancelAll()` | 取消所有任务 |
| `ListTasks() []TaskResult` | 列出所有任务 |
| `ListAgents() map[string]agent.AgentStats` | 列出 Agent 统计 |
| `SetAgentFactory(factory AgentFactory)` | 设置自定义 Agent 工厂 |
| `SetModel(provider llm.Provider)` | 设置默认 LLM Provider |
| `SetToolkit(registry *tools.Registry)` | 设置默认工具注册表 |
| `Stats() PoolStats` | 获取 Pool 统计 |
| `EventChannel() <-chan PoolEvent` | 订阅 Pool 事件 |
| `GracefulShutdown(ctx) error` | 优雅关闭 |
| `Close()` | 关闭 Pool |

## 统计信息

```go
type PoolStats struct {
    TotalTasks        int `json:"total_tasks"`
    CompletedTasks    int `json:"completed_tasks"`
    FailedTasks       int `json:"failed_tasks"`
    RunningTasks      int `json:"running_tasks"`
    QueuedTasks       int `json:"queued_tasks"`
    MaxConcurrency    int `json:"max_concurrency"`
    ActiveConcurrency int `json:"active_concurrency"`
}
```

## PoolEvent

```go
type PoolEvent struct {
    Type      string    `json:"type"`      // task_queued / task_started / task_completed / task_failed
    TaskID    string    `json:"task_id"`
    Timestamp time.Time `json:"timestamp"`
    Data      any       `json:"data,omitempty"`
}
```

## 租户配额

```go
// 构造租户注册表：factory 按租户 ID 返回配额；factory 为 nil 时所有租户用 defaultQ
tenantReg := pool.NewTenantRegistry(
    func(tenantID string) (pool.TenantQuota, error) {
        return pool.TenantQuota{MaxConcurrency: 10, MaxTasksPerMinute: 100}, nil
    },
    pool.TenantQuota{MaxConcurrency: 5, MaxTasksPerMinute: 60}, // 默认配额
)
pool.EnableTenantRegistry(tenantReg)

// 为租户提交任务
result, err := pool.SubmitForTenant(ctx, "tenant-a", taskConfig)

// 获取租户令牌（手动获取，返回 release 函数）
release, err := pool.AcquireForTenant("tenant-a")
defer release()

// 查看租户统计
snapshots := pool.TenantStats()
```

## 自动扩缩

经 `PoolConfig.AutoScaler`（`*AutoScalerConfig`）启用：

```go
type AutoScalerConfig struct {
    MinConcurrency      int           // 最小并发
    MaxConcurrency      int           // 最大并发
    ScaleUpThreshold    float64       // 扩容阈值（默认 0.8）
    ScaleDownThreshold  float64       // 缩容阈值（默认 0.2）
    CoolDownPeriod      time.Duration // 扩缩冷却期
    CheckInterval       time.Duration // 检查间隔
}
```

**示例：**

```go
// 注：AutoScalerConfig 目前仅在 internal/pool 定义（pkg 尚未导出别名），
// 需要在构造 PoolConfig 时直接内联设置；字段与默认值如上表所示。
pool := ap.NewPool(ap.PoolConfig{
    MaxConcurrency: 10,
    // AutoScaler: <internal/pool.AutoScalerConfig{MinConcurrency: 2, MaxConcurrency: 32}>
})
```

## 调度流程

```
Dispatch(tasks) → 为每个 task 启动 goroutine
    ↓
executeTask():
    1. semaphore <- struct{}{} (获取信号量)
    2. createAgentForTask() (工厂或默认)
    3. agent.Run(ctx, prompt)
    4. 失败时检查 RetryPolicy
    5. 释放信号量
    6. 更新 Stats
```

## 完整示例

=== "Go"

    ```go
    pool := ap.NewPool(ap.PoolConfig{
        MaxConcurrency: 5,
        Timeout:        60 * time.Second,
        DefaultAgent: ap.ReActAgentConfig{
            SystemPrompt: "你是任务处理助手",
            MaxTurns:     10,
        },
    })
    defer pool.Close()

    pool.SetModel(provider)

    results, err := pool.Dispatch(ctx, []ap.TaskConfig{
        {ID: "task-1", Title: "代码分析", Prompt: "分析 main.go"},
        {ID: "task-2", Title: "运行测试", Prompt: "执行 go test"},
        {ID: "task-3", Title: "生成文档", Prompt: "生成 API 文档"},
    })

    for _, r := range results {
        if r.Error != nil {
            log.Printf("%s 失败: %v", r.TaskID, r.Error)
        } else {
            log.Printf("%s 完成: %s", r.TaskID, r.Response.Content)
        }
    }
    ```

=== "TypeScript"

    ```typescript
    import { AgentPool } from '@agentprimordia/sdk';

    const pool = new AgentPool({
      maxConcurrency: 5,
      timeout: 60000,
      defaultAgent: {
        systemPrompt: '你是任务处理助手',
        maxTurns: 10,
      },
    });

    pool.setModel(provider);

    const results = await pool.dispatch([
      { id: 'task-1', title: '代码分析', prompt: '分析 main.go' },
      { id: 'task-2', title: '运行测试', prompt: '执行 go test' },
    ]);

    for (const r of results) {
      if (r.error) {
        console.log(`${r.taskId} 失败: ${r.error}`);
      } else {
        console.log(`${r.taskId} 完成: ${r.response?.content}`);
      }
    }
    ```

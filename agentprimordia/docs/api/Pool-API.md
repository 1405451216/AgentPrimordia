# Pool API

多 Agent 并发调度 API 参考文档。

## Pool

```go
func NewPool(config PoolConfig) *Pool
```

创建多 Agent 调度池，实现信号量并发控制、会话隔离和重试策略。

## PoolConfig

```go
type PoolConfig struct {
    MaxConcurrency   int              `json:"max_concurrency"`           // 最大并发数
    Timeout          time.Duration    `json:"timeout"`                   // 超时时间
    RetryPolicy      RetryPolicy      `json:"retry_policy,omitempty"`    // 重试策略
    MaxRetainedTasks int              `json:"max_retained_tasks,omitempty"` // 保留已完成任务数上限（0=不清理）
    DefaultAgent     ReActAgentConfig `json:"default_agent"`             // 默认 Agent 配置
    AutoScaler       *AutoScalerConfig `json:"auto_scaler,omitempty"`    // 自动扩缩容配置
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
```

**示例：**

```go
pool := ap.NewPool(ap.PoolConfig{
    MaxConcurrency:   5,
    Timeout:          60 * time.Second,
    MaxRetainedTasks: 1000,
    DefaultAgent: ap.ReActAgentConfig{
        SystemPrompt: "你是任务处理助手",
        MaxTurns:     10,
    },
})
defer pool.Close()

pool.SetModel(provider)
```

## TaskConfig

```go
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
```

## TaskResult

```go
type TaskResult struct {
    TaskID   string          `json:"task_id"`
    Task     TaskConfig      `json:"task"`
    Response *agent.Response `json:"response,omitempty"`
    Error    error           `json:"error,omitempty"`
    Duration time.Duration   `json:"duration"`
    Status   PoolTaskStatus  `json:"status"`
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

## PoolStats

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

## 主要方法

| 方法 | 说明 |
|------|------|
| `Dispatch(ctx, tasks []TaskConfig) ([]TaskResult, error)` | 批量分发任务 |
| `WaitForTask(ctx, taskID string) (*TaskResult, error)` | 等待单个任务结果 |
| `GetTask(taskID string) (*TaskResult, bool)` | 获取任务状态 |
| `GetTasksBySession(sessionID string) []*TaskResult` | 按会话查询任务 |
| `CancelBySession(sessionID string) int` | 取消会话下所有任务 |
| `Subscribe() <-chan PoolEvent` | 订阅 Pool 事件 |
| `SetAgentFactory(factory AgentFactory)` | 设置自定义 Agent 工厂 |
| `SetModel(provider llm.Provider)` | 设置默认 LLM Provider |
| `Stats() PoolStats` | 获取 Pool 统计 |
| `Close()` | 关闭 Pool |

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

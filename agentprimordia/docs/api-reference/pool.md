# Pool API 参考

> `package ap` — Agent 调度池与并发管理。

## 类型定义

```go
type Pool struct{}

type PoolConfig struct {
    MaxConcurrent       int           // 最大并发 Agent 数
    QueueSize           int           // 任务队列深度
    WorkerIdleTTL       time.Duration // 空闲 worker 存活时间
    TaskTimeout         time.Duration // 单任务超时
    TenantRegistry      *multitenant.TenantRegistry  // 可选
}

type TaskConfig struct {
    AgentName   string
    Prompt      string
    TenantID    string     // 可选（多租户场景）
    Timeout     time.Duration
    Priority    int        // 优先级（越高越优先）
    Callback    func(*TaskResult)  // 异步回调
}

type TaskResult struct {
    TaskID   string
    Response *agent.Response
    Error    error
    Duration time.Time
}
```

## 构造函数

```go
func NewPool(cfg PoolConfig) *Pool
```

## 方法

```go
// Submit 提交任务，返回 task ID（异步）
func (p *Pool) Submit(cfg TaskConfig) (string, error)

// SubmitSync 提交并等待结果（同步）
func (p *Pool) SubmitSync(ctx context.Context, cfg TaskConfig) (*TaskResult, error)

// Register 注册 Agent 模板
func (p *Pool) Register(name string, cfg ap.AgentConfig)

// Acquire 获取 worker（租户场景使用）
func (p *Pool) Acquire(ctx context.Context) (release func(), error)

// Stats 返回 Pool 实时统计
func (p *Pool) Stats() PoolStats

// Start / Stop 启停 Pool
func (p *Pool) Start(ctx context.Context) error
func (p *Pool) Stop() error
```

## 统计信息

```go
type PoolStats struct {
    ActiveWorkers   int64
    QueuedTasks     int64
    TotalProcessed  int64
    TotalErrors     int64
    AvgTaskDuration time.Duration
    PerAgentStats   map[string]AgentStats
}
```

## 租户配额

```go
// 启用多租户配额
tenantReg := multitenant.NewTenantRegistry()
tenantReg.Add("tenant-a", multitenant.Quota{MaxConcurrency: 10, MaxTasksPerMinute: 100})
pool.EnableTenantRegistry(tenantReg)
```

## 自动扩缩

```go
type AutoscalerConfig struct {
    MinWorkers          int
    MaxWorkers          int
    ScaleUpThreshold    int   // 队列深度阈值
    ScaleDownIdleSeconds int  // 空闲秒数
}

func (p *Pool) EnableAutoscaler(cfg AutoscalerConfig)
```

## 示例

```go
pool := ap.NewPool(ap.PoolConfig{
    MaxConcurrent: 10,
    QueueSize:     1000,
})

// 注册模板
pool.Register("chatbot", ap.AgentConfig{Name: "chatbot", SystemPrompt: "你是客服"})

// 同步提交
result, _ := pool.SubmitSync(ctx, ap.TaskConfig{
    AgentName: "chatbot",
    Prompt:    "如何退货？",
})
fmt.Println(result.Response.Content)
```

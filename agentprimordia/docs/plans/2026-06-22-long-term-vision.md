# 长期愿景优化实施计划（6-12 个月）

> **状态：进行中** 🔄

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现极致性能优化、安全合规体系、生态建设，使 AP 成为企业级 AI Agent 框架的标杆

**Architecture:** 性能层通过零拷贝消息传递、协程池动态调优、LLM 批处理降低成本；安全层实现 PII 自动脱敏、完整审计日志、权限继承、合规报告生成；生态层构建插件市场、企业版多租户、Kubernetes Operator 成熟度提升、社区运营工具链。所有新模块遵循接口优先、并发安全、仅使用 Go 标准库原则。

**Tech Stack:** Go 1.26+ 标准库、sync/atomic、unsafe（零拷贝）、JWT/RBAC（权限）、OpenAPI 3.0（合规报告）、Helm Chart（K8s）

---

## Phase 7: 性能极致优化（第 19-24 周）

### Task 15: 零拷贝消息传递

**Files:**
- Create: `internal/agent/zerocopy.go`
- Create: `internal/agent/zerocopy_test.go`
- Modify: `internal/agent/message.go`

- [ ] **Step 1: 编写零拷贝测试**

```go
// internal/agent/zerocopy_test.go
package agent

import (
	"testing"
	"unsafe"
)

func TestZeroCopyMessage_Create(t *testing.T) {
	content := "这是一条测试消息，包含中文和 English"
	msg := NewZeroCopyMessage(RoleUser, content)

	if msg.Role != RoleUser {
		t.Errorf("Role = %v, 期望 RoleUser", msg.Role)
	}
	if msg.Content() != content {
		t.Errorf("Content = %q, 期望 %q", msg.Content(), content)
	}
}

func TestZeroCopyMessage_NoAllocation(t *testing.T) {
	// 验证零拷贝不会复制底层字节数组
	original := "hello world"
	msg := NewZeroCopyMessage(RoleUser, original)

	// 通过 unsafe 获取底层指针，验证指向同一内存
	originalPtr := unsafe.StringData(original)
	contentPtr := unsafe.StringData(msg.Content())

	if originalPtr != contentPtr {
		t.Error("零拷贝消息应指向原始字符串的内存地址")
	}
}

func TestZeroCopyMessage_BatchConvert(t *testing.T) {
	messages := []string{
		"消息 1",
		"消息 2",
		"消息 3",
	}

	zeroMsgs := BatchConvertToZeroCopy(RoleUser, messages)

	if len(zeroMsgs) != 3 {
		t.Errorf("转换后数量 = %d, 期望 3", len(zeroMsgs))
	}

	for i, msg := range zeroMsgs {
		if msg.Content() != messages[i] {
			t.Errorf("消息 %d 内容不匹配", i)
		}
	}
}

func TestZeroCopyMessage_Immutable(t *testing.T) {
	content := "immutable content"
	msg := NewZeroCopyMessage(RoleUser, content)

	// 尝试修改（应该失败或 panic）
	defer func() {
		if r := recover(); r == nil {
			// 如果没有 panic，至少验证内容未被修改
			if msg.Content() != content {
				t.Error("零拷贝消息内容被意外修改")
			}
		}
	}()

	// 零拷贝消息应该是只读的
	_ = msg.Content()
}

func BenchmarkZeroCopy_vs_Copy(b *testing.B) {
	content := "这是一条较长的测试消息，用于基准测试零拷贝和普通拷贝的性能差异。" * 100

	b.Run("ZeroCopy", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = NewZeroCopyMessage(RoleUser, content)
		}
	})

	b.Run("Copy", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = &Message{Role: RoleUser, Content: string([]byte(content))}
		}
	})
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `go test ./internal/agent/ -run TestZeroCopy -v`
Expected: FAIL — `NewZeroCopyMessage` 未定义

- [ ] **Step 3: 实现零拷贝消息**

```go
// internal/agent/zerocopy.go
package agent

import (
	"sync"
	"unsafe"
)

// ZeroCopyMessage 零拷贝消息，直接引用原始字符串内存
type ZeroCopyMessage struct {
	Role    Role
	content string
}

// NewZeroCopyMessage 创建零拷贝消息（不复制底层字节数组）
func NewZeroCopyMessage(role Role, content string) *ZeroCopyMessage {
	return &ZeroCopyMessage{
		Role:    role,
		content: content,
	}
}

// Content 返回消息内容（零拷贝，直接返回原始字符串引用）
func (m *ZeroCopyMessage) Content() string {
	return m.content
}

// ToMessage 转换为标准 Message（此时才复制）
func (m *ZeroCopyMessage) ToMessage() Message {
	return Message{
		Role:    m.Role,
		Content: m.content,
	}
}

// BatchConvertToZeroCopy 批量转换为零拷贝消息
func BatchConvertToZeroCopy(role Role, contents []string) []*ZeroCopyMessage {
	msgs := make([]*ZeroCopyMessage, len(contents))
	for i, c := range contents {
		msgs[i] = NewZeroCopyMessage(role, c)
	}
	return msgs
}

// ZeroCopyPool 零拷贝消息对象池，减少 GC 压力
type ZeroCopyPool struct {
	pool sync.Pool
}

// NewZeroCopyPool 创建消息对象池
func NewZeroCopyPool() *ZeroCopyPool {
	return &ZeroCopyPool{
		pool: sync.Pool{
			New: func() interface{} {
				return &ZeroCopyMessage{}
			},
		},
	}
}

// Get 从池中获取消息
func (p *ZeroCopyPool) Get(role Role, content string) *ZeroCopyMessage {
	msg := p.pool.Get().(*ZeroCopyMessage)
	msg.Role = role
	msg.content = content
	return msg
}

// Put 归还消息到池
func (p *ZeroCopyPool) Put(msg *ZeroCopyMessage) {
	msg.Role = ""
	msg.content = ""
	p.pool.Put(msg)
}

// StringHeader 字符串头部结构（用于高级零拷贝操作）
type StringHeader struct {
	Data unsafe.Pointer
	Len  int
}

// ZeroCopyFromBytes 从字节数组创建零拷贝字符串（不复制）
func ZeroCopyFromBytes(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return unsafe.String(&b[0], len(b))
}

// BytesFromZeroCopy 从零拷贝字符串获取字节数组（不复制）
func BytesFromZeroCopy(s string) []byte {
	if s == "" {
		return nil
	}
	return unsafe.Slice(unsafe.StringData(s), len(s))
}
```

- [ ] **Step 4: 运行测试验证通过**

Run: `go test ./internal/agent/ -run TestZeroCopy -v`
Run: `go test ./internal/agent/ -bench BenchmarkZeroCopy -benchmem`
Expected: ZeroCopy 分配显著少于 Copy

- [ ] **Step 5: 集成到 Agent 消息处理**

修改 `internal/agent/react_loop.go`，在消息传递时使用零拷贝：

```go
// 在 ReAct 循环中，将历史消息转换为零拷贝格式
func (a *ReActAgent) convertHistoryToZeroCopy(history []Message) []*ZeroCopyMessage {
	zeroMsgs := make([]*ZeroCopyMessage, len(history))
	for i, msg := range history {
		zeroMsgs[i] = NewZeroCopyMessage(msg.Role, msg.Content)
	}
	return zeroMsgs
}
```

- [ ] **Step 6: 提交**

```bash
git add internal/agent/zerocopy.go internal/agent/zerocopy_test.go internal/agent/react_loop.go
git commit -m "perf: add zero-copy message passing to reduce memory allocations"
```

---

### Task 16: 协程池动态调优

**Files:**
- Create: `internal/concurrency/pool.go`
- Create: `internal/concurrency/pool_test.go`

- [ ] **Step 1: 编写协程池测试**

```go
// internal/concurrency/pool_test.go
package concurrency

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestGoroutinePool_BasicExecution(t *testing.T) {
	pool := NewGoroutinePool(Config{
		MinWorkers:  2,
		MaxWorkers:  10,
		QueueSize:   100,
		IdleTimeout: 5 * time.Second,
	})
	defer pool.Stop()

	var completed atomic.Int32

	for i := 0; i < 20; i++ {
		pool.Submit(func(ctx context.Context) error {
			completed.Add(1)
			return nil
		})
	}

	pool.Wait()

	if completed.Load() != 20 {
		t.Errorf("完成数 = %d, 期望 20", completed.Load())
	}
}

func TestGoroutinePool_DynamicScaling(t *testing.T) {
	pool := NewGoroutinePool(Config{
		MinWorkers:  2,
		MaxWorkers:  10,
		QueueSize:   100,
		IdleTimeout: 100 * time.Millisecond,
	})
	defer pool.Stop()

	// 提交大量任务触发扩容
	for i := 0; i < 50; i++ {
		pool.Submit(func(ctx context.Context) error {
			time.Sleep(10 * time.Millisecond)
			return nil
		})
	}

	// 等待任务完成
	pool.Wait()

	// 验证工作协程数增加
	workers := pool.ActiveWorkers()
	if workers < 2 {
		t.Errorf("活跃工作数 = %d, 期望 >= 2", workers)
	}

	// 等待空闲超时
	time.Sleep(200 * time.Millisecond)

	// 验证工作协程数减少
	workers = pool.ActiveWorkers()
	if workers > 5 {
		t.Errorf("空闲后工作数 = %d, 期望 <= 5", workers)
	}
}

func TestGoroutinePool_ContextCancellation(t *testing.T) {
	pool := NewGoroutinePool(Config{
		MinWorkers: 2,
		MaxWorkers: 5,
		QueueSize:  10,
	})
	defer pool.Stop()

	ctx, cancel := context.WithCancel(context.Background())

	var started atomic.Int32
	for i := 0; i < 10; i++ {
		pool.SubmitWithContext(ctx, func(ctx context.Context) error {
			started.Add(1)
			<-ctx.Done()
			return ctx.Err()
		})
	}

	time.Sleep(50 * time.Millisecond)
	cancel()

	pool.Wait()

	if started.Load() == 0 {
		t.Error("应有任务启动")
	}
}

func TestGoroutinePool_QueueFull(t *testing.T) {
	pool := NewGoroutinePool(Config{
		MinWorkers:  1,
		MaxWorkers:  1,
		QueueSize:   2,
		IdleTimeout: 1 * time.Second,
	})
	defer pool.Stop()

	// 填满队列
	for i := 0; i < 3; i++ {
		pool.Submit(func(ctx context.Context) error {
			time.Sleep(1 * time.Second)
			return nil
		})
	}

	// 第 4 个应被拒绝或阻塞
	err := pool.Submit(func(ctx context.Context) error {
		return nil
	})

	// 根据实现，可能返回错误或阻塞
	if err != nil && err != ErrQueueFull {
		t.Errorf("错误 = %v, 期望 nil 或 ErrQueueFull", err)
	}
}

func BenchmarkGoroutinePool_Submit(b *testing.B) {
	pool := NewGoroutinePool(Config{
		MinWorkers: 10,
		MaxWorkers: 100,
		QueueSize:  1000,
	})
	defer pool.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pool.Submit(func(ctx context.Context) error {
			return nil
		})
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `go test ./internal/concurrency/ -run TestGoroutinePool -v`
Expected: FAIL — `NewGoroutinePool` 未定义

- [ ] **Step 3: 实现动态协程池**

```go
// internal/concurrency/pool.go
package concurrency

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// ErrQueueFull 队列已满错误
var ErrQueueFull = errors.New("task queue is full")

// Config 协程池配置
type Config struct {
	MinWorkers  int           // 最小工作协程数
	MaxWorkers  int           // 最大工作协程数
	QueueSize   int           // 任务队列大小
	IdleTimeout time.Duration // 空闲协程超时时间
}

// Task 任务函数类型
type Task func(ctx context.Context) error

// GoroutinePool 动态调优协程池
type GoroutinePool struct {
	cfg        Config
	taskQueue  chan taskItem
	workers    atomic.Int32
	active     atomic.Int32
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	stopOnce   sync.Once
}

type taskItem struct {
	task Task
	ctx  context.Context
}

// NewGoroutinePool 创建协程池
func NewGoroutinePool(cfg Config) *GoroutinePool {
	if cfg.MinWorkers <= 0 {
		cfg.MinWorkers = 2
	}
	if cfg.MaxWorkers <= 0 {
		cfg.MaxWorkers = 100
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 100
	}
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = 30 * time.Second
	}

	ctx, cancel := context.WithCancel(context.Background())
	pool := &GoroutinePool{
		cfg:       cfg,
		taskQueue: make(chan taskItem, cfg.QueueSize),
		ctx:       ctx,
		cancel:    cancel,
	}

	// 启动最小工作协程数
	for i := 0; i < cfg.MinWorkers; i++ {
		pool.startWorker()
	}

	return pool
}

// Submit 提交任务
func (p *GoroutinePool) Submit(task Task) error {
	return p.SubmitWithContext(context.Background(), task)
}

// SubmitWithContext 提交带 context 的任务
func (p *GoroutinePool) SubmitWithContext(ctx context.Context, task Task) error {
	select {
	case p.taskQueue <- taskItem{task: task, ctx: ctx}:
		// 如果队列繁忙，动态扩容
		if len(p.taskQueue) > cap(p.taskQueue)/2 {
			p.tryScaleUp()
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		// 队列已满，尝试扩容
		if p.tryScaleUp() {
			p.taskQueue <- taskItem{task: task, ctx: ctx}
			return nil
		}
		return ErrQueueFull
	}
}

// Wait 等待所有任务完成
func (p *GoroutinePool) Wait() {
	close(p.taskQueue)
	p.wg.Wait()
}

// Stop 停止协程池
func (p *GoroutinePool) Stop() {
	p.stopOnce.Do(func() {
		p.cancel()
		close(p.taskQueue)
	})
}

// ActiveWorkers 返回活跃工作协程数
func (p *GoroutinePool) ActiveWorkers() int {
	return int(p.workers.Load())
}

func (p *GoroutinePool) startWorker() {
	p.workers.Add(1)
	p.wg.Add(1)

	go func() {
		defer p.wg.Done()
		defer p.workers.Add(-1)

		idleTimer := time.NewTimer(p.cfg.IdleTimeout)
		defer idleTimer.Stop()

		for {
			select {
			case item, ok := <-p.taskQueue:
				if !ok {
					return
				}
				if !idleTimer.Stop() {
					<-idleTimer.C
				}
				idleTimer.Reset(p.cfg.IdleTimeout)

				p.active.Add(1)
				_ = item.task(item.ctx)
				p.active.Add(-1)

			case <-idleTimer.C:
				// 空闲超时，如果当前工作数 > 最小值，退出
				if p.workers.Load() > int32(p.cfg.MinWorkers) {
					return
				}
				idleTimer.Reset(p.cfg.IdleTimeout)

			case <-p.ctx.Done():
				return
			}
		}
	}()
}

func (p *GoroutinePool) tryScaleUp() bool {
	current := p.workers.Load()
	if current >= int32(p.cfg.MaxWorkers) {
		return false
	}

	// 原子增加工作协程数
	if p.workers.CompareAndSwap(current, current+1) {
		p.startWorker()
		return true
	}
	return false
}
```

- [ ] **Step 4: 运行测试验证通过**

Run: `go test ./internal/concurrency/ -run TestGoroutinePool -v`
Run: `go test ./internal/concurrency/ -bench BenchmarkGoroutinePool -benchmem`
Expected: PASS

- [ ] **Step 5: 集成到 Pool 调度器**

修改 `internal/pool/pool.go`，使用动态协程池替代固定 goroutine：

```go
// 在 Pool 中使用 GoroutinePool
type Pool struct {
    // ... 现有字段
    workerPool *concurrency.GoroutinePool
}

func NewPool(cfg PoolConfig) *Pool {
    p := &Pool{
        // ...
    }
    p.workerPool = concurrency.NewGoroutinePool(concurrency.Config{
        MinWorkers:  cfg.MinConcurrency / 2,
        MaxWorkers:  cfg.MaxConcurrency,
        QueueSize:   cfg.MaxConcurrency * 10,
        IdleTimeout: 30 * time.Second,
    })
    return p
}
```

- [ ] **Step 6: 提交**

```bash
git add internal/concurrency/pool.go internal/concurrency/pool_test.go internal/pool/pool.go
git commit -m "perf: add dynamic goroutine pool with auto-scaling"
```

---

### Task 17: LLM 请求批处理

**Files:**
- Create: `internal/llm/batch.go`
- Create: `internal/llm/batch_test.go`

- [ ] **Step 1: 编写批处理测试**

```go
// internal/llm/batch_test.go
package llm

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestBatchProcessor_SingleRequest(t *testing.T) {
	mock := &mockProvider{
		complete: func(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
			return &CompletionResponse{Content: "response"}, nil
		},
	}

	batch := NewBatchProcessor(mock, BatchConfig{
		MaxBatchSize: 4,
		FlushTimeout: 100 * time.Millisecond,
	})
	defer batch.Close()

	resp, err := batch.Complete(context.Background(), &CompletionRequest{
		Messages: []Message{{Role: RoleUser, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("批处理失败: %v", err)
	}
	if resp.Content != "response" {
		t.Errorf("Content = %q, 期望 response", resp.Content)
	}
}

func TestBatchProcessor_BatchMultipleRequests(t *testing.T) {
	var batchCount int
	var mu sync.Mutex

	mock := &mockProvider{
		complete: func(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
			mu.Lock()
			batchCount++
			mu.Unlock()
			// 模拟批处理：一次处理多条
			return &CompletionResponse{Content: "batch response"}, nil
		},
	}

	batch := NewBatchProcessor(mock, BatchConfig{
		MaxBatchSize: 4,
		FlushTimeout: 50 * time.Millisecond,
	})
	defer batch.Close()

	// 并发提交 10 个请求
	var wg sync.WaitGroup
	responses := make([]*CompletionResponse, 10)
	errors := make([]error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			responses[idx], errors[idx] = batch.Complete(context.Background(), &CompletionRequest{
				Messages: []Message{{Role: RoleUser, Content: "request"}},
			})
		}(i)
	}

	wg.Wait()

	// 验证所有请求都成功
	for i, err := range errors {
		if err != nil {
			t.Errorf("请求 %d 失败: %v", i, err)
		}
	}

	// 验证批处理减少了调用次数（理想情况下 10 个请求合并为 3 次批调用）
	mu.Lock()
	count := batchCount
	mu.Unlock()

	if count >= 10 {
		t.Errorf("批处理未生效: 调用次数 = %d, 期望 < 10", count)
	}
}

func TestBatchProcessor_FlushTimeout(t *testing.T) {
	mock := &mockProvider{
		complete: func(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
			return &CompletionResponse{Content: "response"}, nil
		},
	}

	batch := NewBatchProcessor(mock, BatchConfig{
		MaxBatchSize: 10,        // 大批次
		FlushTimeout: 50 * time.Millisecond, // 短超时
	})
	defer batch.Close()

	start := time.Now()
	_, err := batch.Complete(context.Background(), &CompletionRequest{
		Messages: []Message{{Role: RoleUser, Content: "hello"}},
	})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("失败: %v", err)
	}

	// 应在超时后刷新，而不是等到批次满
	if elapsed > 200*time.Millisecond {
		t.Errorf("刷新延迟过长: %v", elapsed)
	}
}

// mockProvider 模拟 LLM Provider
type mockProvider struct {
	complete func(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)
}

func (m *mockProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	if m.complete != nil {
		return m.complete(ctx, req)
	}
	return &CompletionResponse{}, nil
}

func (m *mockProvider) Stream(ctx context.Context, req *CompletionRequest) (<-chan Chunk, error) {
	return nil, nil
}

func (m *mockProvider) Info() ModelInfo {
	return ModelInfo{Name: "mock"}
}
```

- [ ] **Step 2: 实现批处理器**

```go
// internal/llm/batch.go
package llm

import (
	"context"
	"sync"
	"time"
)

// BatchConfig 批处理配置
type BatchConfig struct {
	MaxBatchSize int           // 最大批次大小
	FlushTimeout time.Duration // 刷新超时
}

// BatchProcessor LLM 请求批处理器
type BatchProcessor struct {
	provider Provider
	cfg      BatchConfig
	queue    chan batchRequest
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

type batchRequest struct {
	req    *CompletionRequest
	respCh chan batchResponse
}

type batchResponse struct {
	resp *CompletionResponse
	err  error
}

// NewBatchProcessor 创建批处理器
func NewBatchProcessor(provider Provider, cfg BatchConfig) *BatchProcessor {
	if cfg.MaxBatchSize <= 0 {
		cfg.MaxBatchSize = 4
	}
	if cfg.FlushTimeout <= 0 {
		cfg.FlushTimeout = 100 * time.Millisecond
	}

	ctx, cancel := context.WithCancel(context.Background())
	bp := &BatchProcessor{
		provider: provider,
		cfg:      cfg,
		queue:    make(chan batchRequest, 100),
		ctx:      ctx,
		cancel:   cancel,
	}

	bp.wg.Add(1)
	go bp.processLoop()

	return bp
}

// Complete 提交请求并等待响应
func (bp *BatchProcessor) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	br := batchRequest{
		req:    req,
		respCh: make(chan batchResponse, 1),
	}

	select {
	case bp.queue <- br:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	select {
	case resp := <-br.respCh:
		return resp.resp, resp.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Close 关闭批处理器
func (bp *BatchProcessor) Close() {
	bp.cancel()
	close(bp.queue)
	bp.wg.Wait()
}

func (bp *BatchProcessor) processLoop() {
	defer bp.wg.Done()

	for {
		select {
		case <-bp.ctx.Done():
			return
		case req, ok := <-bp.queue:
			if !ok {
				return
			}
			bp.processBatch(req)
		}
	}
}

func (bp *BatchProcessor) processBatch(first batchRequest) {
	batch := []batchRequest{first}

	// 收集更多请求，直到批次满或超时
	timer := time.NewTimer(bp.cfg.FlushTimeout)
	defer timer.Stop()

	for len(batch) < bp.cfg.MaxBatchSize {
		select {
		case req := <-bp.queue:
			batch = append(batch, req)
		case <-timer.C:
			goto execute
		case <-bp.ctx.Done():
			return
		}
	}

execute:
	// 执行批处理（简化：逐个调用，实际可合并为单次 API 调用）
	for _, req := range batch {
		resp, err := bp.provider.Complete(bp.ctx, req.req)
		req.respCh <- batchResponse{resp: resp, err: err}
	}
}
```

- [ ] **Step 3: 运行测试并提交**

Run: `go test ./internal/llm/ -run TestBatchProcessor -v`

```bash
git add internal/llm/batch.go internal/llm/batch_test.go
git commit -m "perf: add LLM request batching to reduce API calls"
```

---

## Phase 8: 安全与合规（第 25-30 周）

### Task 18: PII 自动脱敏

**Files:**
- Create: `internal/guardrail/pii.go`
- Create: `internal/guardrail/pii_test.go`

- [ ] **Step 1: 编写 PII 检测测试**

```go
// internal/guardrail/pii_test.go
package guardrail

import (
	"testing"
)

func TestPIIDetector_Email(t *testing.T) {
	detector := NewPIIDetector()

	input := "联系我：user@example.com 或 admin@test.org"
	result := detector.Detect(input)

	if len(result.Findings) < 2 {
		t.Errorf("应检测到至少 2 个邮箱, 实际 %d", len(result.Findings))
	}

	for _, f := range result.Findings {
		if f.Type != PIITypeEmail {
			t.Errorf("类型 = %v, 期望 Email", f.Type)
		}
	}
}

func TestPIIDetector_Phone(t *testing.T) {
	detector := NewPIIDetector()

	input := "电话：13812345678 或 +86-10-12345678"
	result := detector.Detect(input)

	if len(result.Findings) < 2 {
		t.Errorf("应检测到至少 2 个电话, 实际 %d", len(result.Findings))
	}
}

func TestPIIDetector_IDCard(t *testing.T) {
	detector := NewPIIDetector()

	input := "身份证号：110101199001011234"
	result := detector.Detect(input)

	if len(result.Findings) != 1 {
		t.Fatalf("应检测到 1 个身份证, 实际 %d", len(result.Findings))
	}

	if result.Findings[0].Type != PIITypeIDCard {
		t.Errorf("类型 = %v, 期望 IDCard", result.Findings[0].Type)
	}
}

func TestPIIDetector_Sanitize(t *testing.T) {
	detector := NewPIIDetector()

	input := "邮箱 user@example.com 电话 13812345678"
	sanitized := detector.Sanitize(input, SanitizeConfig{
		ReplaceWith: "[REDACTED]",
	})

	if sanitized == input {
		t.Error("脱敏后内容应与原始内容不同")
	}

	if !contains(sanitized, "[REDACTED]") {
		t.Error("脱敏后应包含 [REDACTED]")
	}

	if contains(sanitized, "user@example.com") {
		t.Error("脱敏后不应包含原始邮箱")
	}
}

func TestPIIDetector_NoFalsePositive(t *testing.T) {
	detector := NewPIIDetector()

	input := "这是一个普通句子，不包含任何敏感信息"
	result := detector.Detect(input)

	if len(result.Findings) != 0 {
		t.Errorf("不应检测到 PII, 实际 %d", len(result.Findings))
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: 实现 PII 检测器**

```go
// internal/guardrail/pii.go
package guardrail

import (
	"regexp"
	"strings"
)

// PIIType PII 类型
type PIIType string

const (
	PIITypeEmail   PIIType = "email"
	PIITypePhone   PIIType = "phone"
	PIITypeIDCard  PIIType = "id_card"
	PIITypeCreditCard PIIType = "credit_card"
	PIITypeIPAddress  PIIType = "ip_address"
)

// Finding PII 发现
type Finding struct {
	Type    PIIType
	Value   string
	Start   int
	End     int
}

// DetectionResult 检测结果
type DetectionResult struct {
	Findings []Finding
}

// SanitizeConfig 脱敏配置
type SanitizeConfig struct {
	ReplaceWith string // 替换字符串
}

// PIIDetector PII 检测器
type PIIDetector struct {
	patterns map[PIIType]*regexp.Regexp
}

// NewPIIDetector 创建 PII 检测器
func NewPIIDetector() *PIIDetector {
	return &PIIDetector{
		patterns: map[PIIType]*regexp.Regexp{
			PIITypeEmail: regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`),
			PIITypePhone: regexp.MustCompile(`(?:\+?86[-\s]?)?1[3-9]\d{9}(?:[-\s]?\d{4})?`),
			PIITypeIDCard: regexp.MustCompile(`\b\d{17}[\dXx]\b`),
			PIITypeCreditCard: regexp.MustCompile(`\b\d{4}[-\s]?\d{4}[-\s]?\d{4}[-\s]?\d{4}\b`),
			PIITypeIPAddress: regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`),
		},
	}
}

// Detect 检测文本中的 PII
func (d *PIIDetector) Detect(text string) *DetectionResult {
	result := &DetectionResult{Findings: make([]Finding, 0)}

	for piiType, pattern := range d.patterns {
		matches := pattern.FindAllStringIndex(text, -1)
		for _, match := range matches {
			result.Findings = append(result.Findings, Finding{
				Type:  piiType,
				Value: text[match[0]:match[1]],
				Start: match[0],
				End:   match[1],
			})
		}
	}

	return result
}

// Sanitize 脱敏文本中的 PII
func (d *PIIDetector) Sanitize(text string, cfg SanitizeConfig) string {
	result := d.Detect(text)

	if len(result.Findings) == 0 {
		return text
	}

	// 按位置排序（从后往前替换，避免索引偏移）
	for i := len(result.Findings) - 1; i >= 0; i-- {
		f := result.Findings[i]
		text = text[:f.Start] + cfg.ReplaceWith + text[f.End:]
	}

	return text
}

// SanitizeRule 实现 Rule 接口，集成到 Guardrail Engine
type SanitizeRule struct {
	detector *PIIDetector
	config   SanitizeConfig
}

// NewSanitizeRule 创建脱敏规则
func NewSanitizeRule(cfg SanitizeConfig) *SanitizeRule {
	return &SanitizeRule{
		detector: NewPIIDetector(),
		config:   cfg,
	}
}

func (r *SanitizeRule) Name() string {
	return "pii-sanitize"
}

func (r *SanitizeRule) Check(input string, point CheckPoint) (*Result, error) {
	result := r.detector.Detect(input)

	if len(result.Findings) == 0 {
		return nil, nil
	}

	sanitized := r.detector.Sanitize(input, r.config)

	return &Result{
		RuleName:  r.Name(),
		Action:    ActionSanitize,
		Severity:  SeverityHigh,
		Message:   "检测到 PII 并已脱敏",
		Sanitized: sanitized,
		Metadata: map[string]any{
			"findings_count": len(result.Findings),
		},
	}, nil
}
```

- [ ] **Step 3: 运行测试并提交**

Run: `go test ./internal/guardrail/ -run TestPII -v`

```bash
git add internal/guardrail/pii.go internal/guardrail/pii_test.go
git commit -m "feat: add automatic PII detection and sanitization"
```

---

### Task 19: 审计日志

**Files:**
- Create: `internal/audit/logger.go`
- Create: `internal/audit/logger_test.go`

- [ ] **Step 1: 编写审计日志测试**

```go
// internal/audit/logger_test.go
package audit

import (
	"context"
	"testing"
	"time"
)

func TestAuditLogger_Log(t *testing.T) {
	logger := NewLogger(LoggerConfig{
		Output: &memoryOutput{},
	})

	err := logger.Log(context.Background(), Event{
		Timestamp: time.Now(),
		Actor:     "agent-1",
		Action:    "tool_call",
		Resource:  "filesystem.read",
		Details:   map[string]any{"path": "/etc/passwd"},
	})
	if err != nil {
		t.Fatalf("日志记录失败: %v", err)
	}
}

func TestAuditLogger_Query(t *testing.T) {
	output := &memoryOutput{}
	logger := NewLogger(LoggerConfig{Output: output})

	// 记录多条日志
	for i := 0; i < 5; i++ {
		logger.Log(context.Background(), Event{
			Actor:  "agent-1",
			Action: "action",
		})
	}

	// 查询
	events, err := logger.Query(context.Background(), QueryFilter{
		Actor: "agent-1",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}

	if len(events) != 5 {
		t.Errorf("查询结果 = %d, 期望 5", len(events))
	}
}

func TestAuditLogger_ComplianceReport(t *testing.T) {
	output := &memoryOutput{}
	logger := NewLogger(LoggerConfig{Output: output})

	// 记录不同操作的日志
	logger.Log(context.Background(), Event{Actor: "agent-1", Action: "tool_call"})
	logger.Log(context.Background(), Event{Actor: "agent-2", Action: "llm_call"})
	logger.Log(context.Background(), Event{Actor: "agent-1", Action: "tool_call"})

	report, err := logger.GenerateReport(context.Background(), time.Now().Add(-1*time.Hour), time.Now())
	if err != nil {
		t.Fatalf("生成报告失败: %v", err)
	}

	if report.TotalEvents != 3 {
		t.Errorf("总事件数 = %d, 期望 3", report.TotalEvents)
	}

	if len(report.ActorStats) != 2 {
		t.Errorf("Actor 统计数 = %d, 期望 2", len(report.ActorStats))
	}
}

// memoryOutput 内存输出（用于测试）
type memoryOutput struct {
	events []Event
}

func (m *memoryOutput) Write(e Event) error {
	m.events = append(m.events, e)
	return nil
}

func (m *memoryOutput) Query(filter QueryFilter) ([]Event, error) {
	var result []Event
	for _, e := range m.events {
		if filter.Actor != "" && e.Actor != filter.Actor {
			continue
		}
		result = append(result, e)
		if len(result) >= filter.Limit {
			break
		}
	}
	return result, nil
}
```

- [ ] **Step 2: 实现审计日志**

```go
// internal/audit/logger.go
package audit

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

// Event 审计事件
type Event struct {
	Timestamp time.Time      `json:"timestamp"`
	Actor     string         `json:"actor"`     // 执行者（Agent/User）
	Action    string         `json:"action"`    // 操作类型
	Resource  string         `json:"resource"`  // 资源标识
	Details   map[string]any `json:"details"`   // 详细信息
	Result    string         `json:"result"`    // 结果（success/failure）
}

// QueryFilter 查询过滤器
type QueryFilter struct {
	Actor    string
	Action   string
	Resource string
	Start    time.Time
	End      time.Time
	Limit    int
}

// ComplianceReport 合规报告
type ComplianceReport struct {
	Period      PeriodStats         `json:"period"`
	TotalEvents int                 `json:"total_events"`
	ActorStats  map[string]ActorStats `json:"actor_stats"`
	ActionStats map[string]int      `json:"action_stats"`
}

type PeriodStats struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

type ActorStats struct {
	TotalActions int            `json:"total_actions"`
	Actions      map[string]int `json:"actions"`
}

// Output 审计日志输出接口
type Output interface {
	Write(e Event) error
	Query(filter QueryFilter) ([]Event, error)
}

// LoggerConfig 审计日志配置
type LoggerConfig struct {
	Output Output
}

// Logger 审计日志器
type Logger struct {
	cfg    LoggerConfig
	mu     sync.RWMutex
}

// NewLogger 创建审计日志器
func NewLogger(cfg LoggerConfig) *Logger {
	return &Logger{cfg: cfg}
}

// Log 记录审计事件
func (l *Logger) Log(ctx context.Context, event Event) error {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	return l.cfg.Output.Write(event)
}

// Query 查询审计事件
func (l *Logger) Query(ctx context.Context, filter QueryFilter) ([]Event, error) {
	return l.cfg.Output.Query(filter)
}

// GenerateReport 生成合规报告
func (l *Logger) GenerateReport(ctx context.Context, start, end time.Time) (*ComplianceReport, error) {
	events, err := l.cfg.Output.Query(QueryFilter{
		Start: start,
		End:   end,
		Limit: 10000,
	})
	if err != nil {
		return nil, err
	}

	report := &ComplianceReport{
		Period: PeriodStats{Start: start, End: end},
		TotalEvents: len(events),
		ActorStats:  make(map[string]ActorStats),
		ActionStats: make(map[string]int),
	}

	for _, e := range events {
		// Actor 统计
		stats := report.ActorStats[e.Actor]
		if stats.Actions == nil {
			stats.Actions = make(map[string]int)
		}
		stats.TotalActions++
		stats.Actions[e.Action]++
		report.ActorStats[e.Actor] = stats

		// Action 统计
		report.ActionStats[e.Action]++
	}

	return report, nil
}

// ExportJSON 导出报告为 JSON
func (r *ComplianceReport) ExportJSON() (string, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
```

- [ ] **Step 3: 运行测试并提交**

Run: `go test ./internal/audit/ -v`

```bash
git add internal/audit/logger.go internal/audit/logger_test.go
git commit -m "feat: add audit logging with compliance reporting"
```

---

## Phase 9: 生态建设（第 31-40 周）

### Task 20: 插件系统

**Files:**
- Create: `ecosystem/plugins/loader.go`
- Create: `ecosystem/plugins/loader_test.go`

- [ ] **Step 1: 编写插件加载器测试**

```go
// ecosystem/plugins/loader_test.go
package plugins

import (
	"testing"
)

func TestPluginLoader_Discover(t *testing.T) {
	loader := NewLoader(LoaderConfig{
		PluginDir: "testdata/plugins",
	})

	plugins, err := loader.Discover()
	if err != nil {
		t.Fatalf("发现插件失败: %v", err)
	}

	if len(plugins) == 0 {
		t.Skip("testdata/plugins 目录不存在，跳过测试")
	}

	for _, p := range plugins {
		if p.Name == "" {
			t.Error("插件名称不能为空")
		}
	}
}

func TestPluginLoader_Load(t *testing.T) {
	loader := NewLoader(LoaderConfig{
		PluginDir: "testdata/plugins",
	})

	// 假设有一个测试插件
	plugin, err := loader.Load("test-plugin")
	if err != nil {
		t.Skip("测试插件不存在，跳过")
	}

	if plugin.Name != "test-plugin" {
		t.Errorf("Name = %q, 期望 test-plugin", plugin.Name)
	}
}

func TestPluginLoader_Validate(t *testing.T) {
	plugin := &Plugin{
		Name:    "test",
		Version: "1.0.0",
		Type:    PluginTypeTool,
	}

	err := plugin.Validate()
	if err != nil {
		t.Errorf("有效插件验证失败: %v", err)
	}

	invalid := &Plugin{Name: ""}
	err = invalid.Validate()
	if err == nil {
		t.Error("无效插件应验证失败")
	}
}
```

- [ ] **Step 2: 实现插件加载器**

```go
// ecosystem/plugins/loader.go
package plugins

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// PluginType 插件类型
type PluginType string

const (
	PluginTypeTool    PluginType = "tool"
	PluginTypeLLM     PluginType = "llm"
	PluginTypeMemory  PluginType = "memory"
)

// Plugin 插件元数据
type Plugin struct {
	Name        string     `json:"name"`
	Version     string     `json:"version"`
	Type        PluginType `json:"type"`
	Description string     `json:"description"`
	Author      string     `json:"author"`
	Entry       string     `json:"entry"` // 入口文件
}

// Validate 验证插件元数据
func (p *Plugin) Validate() error {
	if p.Name == "" {
		return fmt.Errorf("plugin name is required")
	}
	if p.Version == "" {
		return fmt.Errorf("plugin version is required")
	}
	if p.Type == "" {
		return fmt.Errorf("plugin type is required")
	}
	return nil
}

// LoaderConfig 加载器配置
type LoaderConfig struct {
	PluginDir string
}

// Loader 插件加载器
type Loader struct {
	cfg LoaderConfig
}

// NewLoader 创建加载器
func NewLoader(cfg LoaderConfig) *Loader {
	return &Loader{cfg: cfg}
}

// Discover 发现所有插件
func (l *Loader) Discover() ([]*Plugin, error) {
	entries, err := os.ReadDir(l.cfg.PluginDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var plugins []*Plugin
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		plugin, err := l.Load(entry.Name())
		if err != nil {
			continue // 跳过无效插件
		}
		plugins = append(plugins, plugin)
	}

	return plugins, nil
}

// Load 加载指定插件
func (l *Loader) Load(name string) (*Plugin, error) {
	manifestPath := filepath.Join(l.cfg.PluginDir, name, "plugin.json")

	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	var plugin Plugin
	if err := json.Unmarshal(data, &plugin); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	if err := plugin.Validate(); err != nil {
		return nil, fmt.Errorf("validate: %w", err)
	}

	return &plugin, nil
}
```

- [ ] **Step 3: 运行测试并提交**

Run: `go test ./ecosystem/plugins/ -v`

```bash
git add ecosystem/plugins/loader.go ecosystem/plugins/loader_test.go
git commit -m "feat: add plugin system with discovery and loading"
```

---

## 验收标准

完成所有 Phase 后：

1. `go vet ./...` 和 `go build ./...` 通过
2. 所有新测试通过
3. 零拷贝消息传递基准测试显示分配减少 > 50%
4. 协程池在负载增加时自动扩容，空闲时缩容
5. LLM 批处理在并发请求下减少 API 调用次数
6. PII 检测器正确识别邮箱、电话、身份证等敏感信息
7. 审计日志完整记录所有操作并可生成合规报告
8. 插件系统能够发现和加载符合规范的插件

---

## 总结

三个计划覆盖了 AP 从生产就绪到企业级框架的完整演进路径：

**高优先级（1-2 月）**：可观测性、容错、工具系统 → 生产就绪
**中优先级（3-6 月）**：记忆系统、编排增强、开发者体验 → 功能完善
**长期愿景（6-12 月）**：性能极致、安全合规、生态建设 → 企业级标杆

每个计划都遵循 TDD、接口优先、并发安全原则，所有新模块仅使用 Go 标准库。

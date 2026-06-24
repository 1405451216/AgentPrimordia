# 高优先级优化实施计划（1-2 个月）

> **状态：已完成** ✅
> **完成日期：2026-06-22**

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 完善可观测性体系、增强容错与韧性、增强工具系统，使 AP 达到生产就绪状态

**Architecture:** 在现有 internal/otel、internal/metrics、internal/guardrail、internal/persist 基础上扩展，新增 Prometheus HTTP 端点、健康检查、断路器、重试策略、工具沙箱和缓存层。所有新模块仅使用 Go 标准库，遵循接口优先和并发安全原则。

**Tech Stack:** Go 1.26+ 标准库（net/http、sync、context、log/slog）、Prometheus text format、W3C Trace Context

---

## Phase 1: 可观测性体系完善（第 1-2 周）

### Task 1: Prometheus /metrics HTTP 端点

**Files:**
- Create: `internal/metrics/handler.go`
- Create: `internal/metrics/handler_test.go`
- Modify: `pkg/metrics.go`（导出新类型）

- [ ] **Step 1: 编写 Handler 测试**

```go
// internal/metrics/handler_test.go
package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMetricsHandler_ContentType(t *testing.T) {
	m := NewMetrics()
	h := NewHandler(m)

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Errorf("Content-Type = %q, 期望包含 text/plain", ct)
	}
}

func TestMetricsHandler_ContainsCounters(t *testing.T) {
	m := NewMetrics()
	m.RecordLLMCall(100*time.Millisecond, nil)
	m.RecordToolCall(50*time.Millisecond, nil)

	h := NewHandler(m)
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "ap_llm_total_calls 1") {
		t.Error("缺少 ap_llm_total_calls 计数器")
	}
	if !strings.Contains(body, "ap_tool_total_calls 1") {
		t.Error("缺少 ap_tool_total_calls 计数器")
	}
}

func TestMetricsHandler_HistogramBuckets(t *testing.T) {
	m := NewMetrics()
	m.RecordLLMCall(150*time.Millisecond, nil)

	h := NewHandler(m)
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "ap_llm_latency_ms_bucket") {
		t.Error("缺少 histogram bucket 数据")
	}
}

func TestMetricsHandler_MethodNotAllowed(t *testing.T) {
	m := NewMetrics()
	h := NewHandler(m)

	req := httptest.NewRequest("POST", "/metrics", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST 状态码 = %d, 期望 %d", w.Code, http.StatusMethodNotAllowed)
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `go test ./internal/metrics/ -run TestMetricsHandler -v`
Expected: FAIL — `NewHandler` 未定义

- [ ] **Step 3: 实现 Handler**

```go
// internal/metrics/handler.go
package metrics

import "net/http"

// Handler 提供 Prometheus 兼容的 /metrics HTTP 端点
type Handler struct {
	metrics *AgentMetrics
}

// NewHandler 创建 metrics HTTP handler
func NewHandler(m *AgentMetrics) *Handler {
	return &Handler{metrics: m}
}

// ServeHTTP 实现 http.Handler 接口
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(h.metrics.String()))
}
```

- [ ] **Step 4: 运行测试验证通过**

Run: `go test ./internal/metrics/ -run TestMetricsHandler -v`
Expected: PASS

- [ ] **Step 5: 在 pkg/ 中导出**

在 `pkg/metrics.go` 中添加：

```go
// MetricsHandler 提供 Prometheus /metrics 端点
type MetricsHandler = metrics.Handler

// NewMetricsHandler 创建 metrics HTTP handler
var NewMetricsHandler = metrics.NewHandler
```

- [ ] **Step 6: 提交**

```bash
git add internal/metrics/handler.go internal/metrics/handler_test.go pkg/metrics.go
git commit -m "feat: add Prometheus /metrics HTTP handler"
```

---

### Task 2: 健康检查端点 /healthz + /readyz

**Files:**
- Create: `internal/health/health.go`
- Create: `internal/health/health_test.go`
- Create: `pkg/health.go`

- [ ] **Step 1: 编写健康检查测试**

```go
// internal/health/health_test.go
package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mockChecker 模拟健康检查器
type mockChecker struct {
	healthy bool
	name    string
}

func (m *mockChecker) Name() string { return m.name }
func (m *mockChecker) Check(ctx context.Context) error {
	if !m.healthy {
		return context.DeadlineExceeded
	}
	return nil
}

func TestHealthz_AllHealthy(t *testing.T) {
	h := NewChecker()
	h.Register(&mockChecker{healthy: true, name: "db"})
	h.Register(&mockChecker{healthy: true, name: "llm"})

	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("状态码 = %d, 期望 %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"status":"ok"`) {
		t.Errorf("响应体 = %q, 期望包含 ok", body)
	}
}

func TestHealthz_OneUnhealthy(t *testing.T) {
	h := NewChecker()
	h.Register(&mockChecker{healthy: true, name: "db"})
	h.Register(&mockChecker{healthy: false, name: "llm"})

	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("状态码 = %d, 期望 %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestReadyz_BeforeReady(t *testing.T) {
	h := NewChecker()
	// 未调用 SetReady()

	req := httptest.NewRequest("GET", "/readyz", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("状态码 = %d, 期望 %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestReadyz_AfterReady(t *testing.T) {
	h := NewChecker()
	h.SetReady()

	req := httptest.NewRequest("GET", "/readyz", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("状态码 = %d, 期望 %d", w.Code, http.StatusOK)
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `go test ./internal/health/ -v`
Expected: FAIL — 包不存在

- [ ] **Step 3: 实现健康检查**

```go
// internal/health/health.go
package health

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// Checker 健康检查接口
type Checker interface {
	Name() string
	Check(ctx context.Context) error
}

// HealthChecker 聚合多个健康检查器
type HealthChecker struct {
	mu       sync.RWMutex
	checkers []Checker
	ready    atomic.Bool
}

// NewChecker 创建健康检查器
func NewChecker() *HealthChecker {
	return &HealthChecker{
		checkers: make([]Checker, 0),
	}
}

// Register 注册健康检查器
func (h *HealthChecker) Register(c Checker) {
	h.mu.Lock()
	h.checkers = append(h.checkers, c)
	h.mu.Unlock()
}

// SetReady 标记服务就绪
func (h *HealthChecker) SetReady() {
	h.ready.Store(true)
}

// ServeHTTP 处理 /healthz 和 /readyz 请求
func (h *HealthChecker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/healthz":
		h.handleHealthz(w, r)
	case "/readyz":
		h.handleReadyz(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *HealthChecker) handleHealthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	h.mu.RLock()
	checkers := make([]Checker, len(h.checkers))
	copy(checkers, h.checkers)
	h.mu.RUnlock()

	type componentStatus struct {
		Name   string `json:"name"`
		Status string `json:"status"`
		Error  string `json:"error,omitempty"`
	}

	statuses := make([]componentStatus, 0, len(checkers))
	allHealthy := true

	for _, c := range checkers {
		cs := componentStatus{Name: c.Name(), Status: "ok"}
		if err := c.Check(ctx); err != nil {
			cs.Status = "error"
			cs.Error = err.Error()
			allHealthy = false
		}
		statuses = append(statuses, cs)
	}

	resp := map[string]any{
		"status":     "ok",
		"components": statuses,
	}
	if !allHealthy {
		resp["status"] = "degraded"
	}

	w.Header().Set("Content-Type", "application/json")
	if !allHealthy {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	json.NewEncoder(w).Encode(resp)
}

func (h *HealthChecker) handleReadyz(w http.ResponseWriter, r *http.Request) {
	if !h.ready.Load() {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"status":"not ready"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ready"}`))
}
```

- [ ] **Step 4: 运行测试验证通过**

Run: `go test ./internal/health/ -v`
Expected: PASS

- [ ] **Step 5: 在 pkg/ 中导出**

```go
// pkg/health.go
package ap

import "agentprimordia/internal/health"

// HealthChecker 聚合健康检查器
type HealthChecker = health.HealthChecker

// HealthCheckable 健康检查接口
type HealthCheckable = health.Checker

// NewHealthChecker 创建健康检查器
var NewHealthChecker = health.NewChecker
```

- [ ] **Step 6: 提交**

```bash
git add internal/health/ pkg/health.go
git commit -m "feat: add /healthz and /readyz health check endpoints"
```

---

### Task 3: 结构化日志标准

**Files:**
- Create: `internal/logger/logger.go`
- Create: `internal/logger/logger_test.go`
- Create: `pkg/logger.go`

- [ ] **Step 1: 编写日志测试**

```go
// internal/logger/logger_test.go
package logger

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestNewLogger_JSONFormat(t *testing.T) {
	var buf bytes.Buffer
	l := New(&Config{
		Level:  LevelInfo,
		Format: FormatJSON,
		Output: &buf,
	})

	l.Info("test message", "key", "value")

	output := buf.String()
	var m map[string]any
	if err := json.Unmarshal([]byte(output), &m); err != nil {
		t.Fatalf("输出不是有效 JSON: %v\n输出: %s", err, output)
	}

	if m["msg"] != "test message" {
		t.Errorf("msg = %v, 期望 test message", m["msg"])
	}
	if m["key"] != "value" {
		t.Errorf("key = %v, 期望 value", m["key"])
	}
}

func TestNewLogger_TextFormat(t *testing.T) {
	var buf bytes.Buffer
	l := New(&Config{
		Level:  LevelInfo,
		Format: FormatText,
		Output: &buf,
	})

	l.Info("test message")

	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Errorf("输出 = %q, 期望包含 test message", output)
	}
}

func TestNewLogger_LevelFilter(t *testing.T) {
	var buf bytes.Buffer
	l := New(&Config{
		Level:  LevelWarn,
		Format: FormatJSON,
		Output: &buf,
	})

	l.Info("should be filtered")
	l.Warn("should appear")

	output := buf.String()
	if strings.Contains(output, "should be filtered") {
		t.Error("Info 日志未被过滤")
	}
	if !strings.Contains(output, "should appear") {
		t.Error("Warn 日志被错误过滤")
	}
}

func TestNewLogger_WithAgentContext(t *testing.T) {
	var buf bytes.Buffer
	l := New(&Config{
		Level:  LevelInfo,
		Format: FormatJSON,
		Output: &buf,
	})

	agentL := l.WithAgent("my-agent", "session-123")
	agentL.Info("agent action")

	var m map[string]any
	json.Unmarshal(buf.Bytes(), &m)

	if m["agent_name"] != "my-agent" {
		t.Errorf("agent_name = %v, 期望 my-agent", m["agent_name"])
	}
	if m["session_id"] != "session-123" {
		t.Errorf("session_id = %v, 期望 session-123", m["session_id"])
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `go test ./internal/logger/ -v`
Expected: FAIL — 包不存在

- [ ] **Step 3: 实现结构化日志**

```go
// internal/logger/logger.go
package logger

import (
	"io"
	"log/slog"
	"os"
)

// Level 日志级别
type Level = slog.Level

const (
	LevelDebug Level = slog.LevelDebug
	LevelInfo  Level = slog.LevelInfo
	LevelWarn  Level = slog.LevelWarn
	LevelError Level = slog.LevelError
)

// Format 日志格式
type Format string

const (
	FormatJSON Format = "json"
	FormatText Format = "text"
)

// Config 日志配置
type Config struct {
	Level  Level
	Format Format
	Output io.Writer
}

// Logger 结构化日志器
type Logger struct {
	*slog.Logger
}

// New 创建结构化日志器
func New(cfg *Config) *Logger {
	if cfg == nil {
		cfg = &Config{Level: LevelInfo, Format: FormatJSON, Output: os.Stdout}
	}
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}

	opts := &slog.HandlerOptions{Level: cfg.Level}

	var handler slog.Handler
	switch cfg.Format {
	case FormatText:
		handler = slog.NewTextHandler(cfg.Output, opts)
	default:
		handler = slog.NewJSONHandler(cfg.Output, opts)
	}

	return &Logger{Logger: slog.New(handler)}
}

// WithAgent 返回带 Agent 上下文的日志器
func (l *Logger) WithAgent(agentName, sessionID string) *Logger {
	return &Logger{
		Logger: l.Logger.With("agent_name", agentName, "session_id", sessionID),
	}
}

// WithComponent 返回带组件名的日志器
func (l *Logger) WithComponent(component string) *Logger {
	return &Logger{
		Logger: l.Logger.With("component", component),
	}
}
```

- [ ] **Step 4: 运行测试验证通过**

Run: `go test ./internal/logger/ -v`
Expected: PASS

- [ ] **Step 5: 在 pkg/ 中导出并提交**

```go
// pkg/logger.go
package ap

import "agentprimordia/internal/logger"

type Logger = logger.Logger
type LogConfig = logger.Config

var NewLogger = logger.New

const (
	LogLevelDebug = logger.LevelDebug
	LogLevelInfo  = logger.LevelInfo
	LogLevelWarn  = logger.LevelWarn
	LogLevelError = logger.LevelError

	LogFormatJSON = logger.FormatJSON
	LogFormatText = logger.FormatText
)
```

```bash
git add internal/logger/ pkg/logger.go
git commit -m "feat: add structured logging with JSON/text format support"
```

---

## Phase 2: 容错与韧性增强（第 3-4 周）

### Task 4: 断路器模式

**Files:**
- Create: `internal/resilience/circuit_breaker.go`
- Create: `internal/resilience/circuit_breaker_test.go`
- Create: `pkg/circuit_breaker.go`

- [ ] **Step 1: 编写断路器测试**

```go
// internal/resilience/circuit_breaker_test.go
package resilience

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCircuitBreaker_ClosedState(t *testing.T) {
	cb := NewCircuitBreaker(Config{
		FailureThreshold: 3,
		Timeout:          1 * time.Second,
	})

	// 成功调用不应触发断路
	for i := 0; i < 5; i++ {
		err := cb.Execute(context.Background(), func(ctx context.Context) error {
			return nil
		})
		if err != nil {
			t.Fatalf("成功调用返回错误: %v", err)
		}
	}

	if cb.State() != StateClosed {
		t.Errorf("状态 = %v, 期望 Closed", cb.State())
	}
}

func TestCircuitBreaker_OpenAfterFailures(t *testing.T) {
	cb := NewCircuitBreaker(Config{
		FailureThreshold: 3,
		Timeout:          1 * time.Second,
	})

	testErr := errors.New("test error")

	// 连续失败 3 次
	for i := 0; i < 3; i++ {
		cb.Execute(context.Background(), func(ctx context.Context) error {
			return testErr
		})
	}

	if cb.State() != StateOpen {
		t.Errorf("状态 = %v, 期望 Open", cb.State())
	}

	// 断路后调用应立即返回错误
	err := cb.Execute(context.Background(), func(ctx context.Context) error {
		return nil
	})
	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("错误 = %v, 期望 ErrCircuitOpen", err)
	}
}

func TestCircuitBreaker_HalfOpenAfterTimeout(t *testing.T) {
	cb := NewCircuitBreaker(Config{
		FailureThreshold: 2,
		Timeout:          50 * time.Millisecond,
	})

	// 触发断路
	for i := 0; i < 2; i++ {
		cb.Execute(context.Background(), func(ctx context.Context) error {
			return errors.New("fail")
		})
	}

	if cb.State() != StateOpen {
		t.Fatalf("状态应为 Open")
	}

	// 等待超时
	time.Sleep(60 * time.Millisecond)

	if cb.State() != StateHalfOpen {
		t.Errorf("状态 = %v, 期望 HalfOpen", cb.State())
	}

	// 半开状态成功调用应关闭断路器
	err := cb.Execute(context.Background(), func(ctx context.Context) error {
		return nil
	})
	if err != nil {
		t.Fatalf("半开成功调用返回错误: %v", err)
	}

	if cb.State() != StateClosed {
		t.Errorf("状态 = %v, 期望 Closed", cb.State())
	}
}

func TestCircuitBreaker_Fallback(t *testing.T) {
	cb := NewCircuitBreaker(Config{
		FailureThreshold: 1,
		Timeout:          1 * time.Second,
	})

	// 触发断路
	cb.Execute(context.Background(), func(ctx context.Context) error {
		return errors.New("fail")
	})

	// 使用 fallback
	fallbackCalled := false
	err := cb.ExecuteWithFallback(context.Background(),
		func(ctx context.Context) error {
			return errors.New("primary fail")
		},
		func(ctx context.Context, err error) error {
			fallbackCalled = true
			return nil
		},
	)

	if err != nil {
		t.Fatalf("fallback 后仍返回错误: %v", err)
	}
	if !fallbackCalled {
		t.Error("fallback 未被调用")
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `go test ./internal/resilience/ -run TestCircuitBreaker -v`
Expected: FAIL — 包不存在

- [ ] **Step 3: 实现断路器**

```go
// internal/resilience/circuit_breaker.go
package resilience

import (
	"context"
	"errors"
	"sync"
	"time"
)

// State 断路器状态
type State int

const (
	StateClosed   State = iota // 正常
	StateOpen                  // 断路
	StateHalfOpen              // 半开（试探）
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// ErrCircuitOpen 断路器打开时返回的错误
var ErrCircuitOpen = errors.New("circuit breaker is open")

// Config 断路器配置
type Config struct {
	FailureThreshold int           // 触发断路的连续失败次数
	Timeout          time.Duration // 从 Open 到 HalfOpen 的等待时间
}

// CircuitBreaker 断路器实现
type CircuitBreaker struct {
	cfg     Config
	mu      sync.RWMutex
	state   State
	failures int
	lastFail time.Time
}

// NewCircuitBreaker 创建断路器
func NewCircuitBreaker(cfg Config) *CircuitBreaker {
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = 5
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	return &CircuitBreaker{
		cfg:   cfg,
		state: StateClosed,
	}
}

// State 返回当前状态
func (cb *CircuitBreaker) State() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	// 检查是否应从 Open 转为 HalfOpen
	if cb.state == StateOpen && time.Since(cb.lastFail) > cb.cfg.Timeout {
		return StateHalfOpen
	}
	return cb.state
}

// Execute 通过断路器执行函数
func (cb *CircuitBreaker) Execute(ctx context.Context, fn func(ctx context.Context) error) error {
	state := cb.State()

	if state == StateOpen {
		return ErrCircuitOpen
	}

	err := fn(ctx)

	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.failures++
		cb.lastFail = time.Now()
		if cb.failures >= cb.cfg.FailureThreshold {
			cb.state = StateOpen
		}
		return err
	}

	// 成功：重置计数
	cb.failures = 0
	if state == StateHalfOpen {
		cb.state = StateClosed
	}
	return nil
}

// ExecuteWithFallback 带降级回调的执行
func (cb *CircuitBreaker) ExecuteWithFallback(
	ctx context.Context,
	fn func(ctx context.Context) error,
	fallback func(ctx context.Context, err error) error,
) error {
	err := cb.Execute(ctx, fn)
	if err != nil {
		return fallback(ctx, err)
	}
	return nil
}

// Reset 手动重置断路器
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	cb.state = StateClosed
	cb.failures = 0
	cb.mu.Unlock()
}
```

- [ ] **Step 4: 运行测试验证通过**

Run: `go test ./internal/resilience/ -run TestCircuitBreaker -v`
Expected: PASS

- [ ] **Step 5: 在 pkg/ 中导出并提交**

```go
// pkg/circuit_breaker.go
package ap

import "agentprimordia/internal/resilience"

type CircuitBreaker = resilience.CircuitBreaker
type CircuitBreakerConfig = resilience.Config
type CircuitBreakerState = resilience.State

var NewCircuitBreaker = resilience.NewCircuitBreaker

var ErrCircuitOpen = resilience.ErrCircuitOpen

const (
	CircuitClosed   = resilience.StateClosed
	CircuitOpen     = resilience.StateOpen
	CircuitHalfOpen = resilience.StateHalfOpen
)
```

```bash
git add internal/resilience/circuit_breaker.go internal/resilience/circuit_breaker_test.go pkg/circuit_breaker.go
git commit -m "feat: add circuit breaker for LLM provider failover"
```

---

### Task 5: 重试策略（指数退避 + 抖动）

**Files:**
- Create: `internal/resilience/retry.go`
- Create: `internal/resilience/retry_test.go`

- [ ] **Step 1: 编写重试测试**

```go
// internal/resilience/retry_test.go
package resilience

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRetry_SuccessOnFirstAttempt(t *testing.T) {
	r := NewRetrier(Config{
		MaxAttempts: 3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     1 * time.Second,
	})

	calls := 0
	err := r.Do(context.Background(), func(ctx context.Context) error {
		calls++
		return nil
	})

	if err != nil {
		t.Fatalf("期望成功, 得到错误: %v", err)
	}
	if calls != 1 {
		t.Errorf("调用次数 = %d, 期望 1", calls)
	}
}

func TestRetry_SuccessAfterRetries(t *testing.T) {
	r := NewRetrier(Config{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
	})

	calls := 0
	err := r.Do(context.Background(), func(ctx context.Context) error {
		calls++
		if calls < 3 {
			return errors.New("transient error")
		}
		return nil
	})

	if err != nil {
		t.Fatalf("期望第 3 次成功, 得到错误: %v", err)
	}
	if calls != 3 {
		t.Errorf("调用次数 = %d, 期望 3", calls)
	}
}

func TestRetry_AllAttemptsFail(t *testing.T) {
	r := NewRetrier(Config{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
	})

	testErr := errors.New("persistent error")
	calls := 0
	err := r.Do(context.Background(), func(ctx context.Context) error {
		calls++
		return testErr
	})

	if !errors.Is(err, testErr) {
		t.Errorf("错误 = %v, 期望 %v", err, testErr)
	}
	if calls != 3 {
		t.Errorf("调用次数 = %d, 期望 3", calls)
	}
}

func TestRetry_NonRetryableError(t *testing.T) {
	r := NewRetrier(Config{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
	})

	authErr := NewNonRetryableError(errors.New("auth failed"))
	calls := 0
	err := r.Do(context.Background(), func(ctx context.Context) error {
		calls++
		return authErr
	})

	if calls != 1 {
		t.Errorf("不可重试错误应只调用 1 次, 实际 %d 次", calls)
	}
	if !errors.Is(err, authErr) {
		t.Errorf("错误 = %v, 期望 %v", err, authErr)
	}
}

func TestRetry_ContextCancelled(t *testing.T) {
	r := NewRetrier(Config{
		MaxAttempts:  10,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     1 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := r.Do(ctx, func(ctx context.Context) error {
		return errors.New("always fail")
	})

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("错误 = %v, 期望 DeadlineExceeded", err)
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `go test ./internal/resilience/ -run TestRetry -v`
Expected: FAIL — `NewRetrier` 未定义

- [ ] **Step 3: 实现重试策略**

```go
// internal/resilience/retry.go
package resilience

import (
	"context"
	"errors"
	"math/rand"
	"time"
)

// NonRetryableError 标记不可重试的错误
type NonRetryableError struct {
	err error
}

func (e *NonRetryableError) Error() string { return e.err.Error() }
func (e *NonRetryableError) Unwrap() error { return e.err }

// NewNonRetryableError 包装为不可重试错误
func NewNonRetryableError(err error) error {
	return &NonRetryableError{err: err}
}

// RetryConfig 重试配置
type RetryConfig struct {
	MaxAttempts  int           // 最大尝试次数（含首次）
	InitialDelay time.Duration // 首次重试延迟
	MaxDelay     time.Duration // 最大延迟
	Multiplier   float64       // 退避乘数（默认 2.0）
}

// Retrier 重试执行器
type Retrier struct {
	cfg RetryConfig
}

// NewRetrier 创建重试器
func NewRetrier(cfg RetryConfig) *Retrier {
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 3
	}
	if cfg.InitialDelay <= 0 {
		cfg.InitialDelay = 100 * time.Millisecond
	}
	if cfg.MaxDelay <= 0 {
		cfg.MaxDelay = 30 * time.Second
	}
	if cfg.Multiplier <= 0 {
		cfg.Multiplier = 2.0
	}
	return &Retrier{cfg: cfg}
}

// Do 执行函数，失败时按指数退避重试
func (r *Retrier) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	var lastErr error

	for attempt := 0; attempt < r.cfg.MaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		lastErr = fn(ctx)
		if lastErr == nil {
			return nil
		}

		// 检查是否为不可重试错误
		var nre *NonRetryableError
		if errors.As(lastErr, &nre) {
			return lastErr
		}

		// 最后一次尝试失败不需要等待
		if attempt == r.cfg.MaxAttempts-1 {
			break
		}

		// 指数退避 + 抖动
		delay := r.calculateDelay(attempt)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}

	return lastErr
}

// calculateDelay 计算第 n 次重试的延迟（指数退避 + 全抖动）
func (r *Retrier) calculateDelay(attempt int) time.Duration {
	delay := float64(r.cfg.InitialDelay)
	for i := 0; i < attempt; i++ {
		delay *= r.cfg.Multiplier
	}
	if delay > float64(r.cfg.MaxDelay) {
		delay = float64(r.cfg.MaxDelay)
	}

	// 全抖动：在 [0, delay] 中随机选择
	jitter := rand.Float64() * delay
	return time.Duration(jitter)
}
```

- [ ] **Step 4: 运行测试验证通过并提交**

Run: `go test ./internal/resilience/ -run TestRetry -v`
Expected: PASS

```bash
git add internal/resilience/retry.go internal/resilience/retry_test.go
git commit -m "feat: add retry with exponential backoff and jitter"
```

---

### Task 6: 优雅关闭

**Files:**
- Create: `internal/pool/graceful_shutdown.go`
- Create: `internal/pool/graceful_shutdown_test.go`

- [ ] **Step 1: 编写优雅关闭测试**

```go
// internal/pool/graceful_shutdown_test.go
package pool

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestGracefulShutdown_WaitForRunningTasks(t *testing.T) {
	p := NewPool(PoolConfig{MaxConcurrency: 2})
	defer p.Close()

	var completed atomic.Int32

	// 提交长时间任务
	for i := 0; i < 2; i++ {
		p.Dispatch(context.Background(), TaskConfig{
			ID:    "task-1",
			Title: "long task",
			Prompt: "working",
			OnComplete: func(result any) {
				completed.Add(1)
			},
		})
	}

	// 等待任务开始
	time.Sleep(50 * time.Millisecond)

	// 优雅关闭，设置超时
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := p.GracefulShutdown(ctx)
	if err != nil {
		t.Fatalf("优雅关闭失败: %v", err)
	}

	// 验证任务已完成
	if completed.Load() < 1 {
		t.Error("至少应有 1 个任务在关闭前完成")
	}
}

func TestGracefulShutdown_Timeout(t *testing.T) {
	p := NewPool(PoolConfig{MaxConcurrency: 1})

	// 提交永远不会完成的任务
	p.Dispatch(context.Background(), TaskConfig{
		ID:     "stuck-task",
		Title:  "stuck",
		Prompt: "never finish",
	})

	time.Sleep(50 * time.Millisecond)

	// 短超时
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := p.GracefulShutdown(ctx)
	if err == nil {
		t.Error("超时应返回错误")
	}
}
```

- [ ] **Step 2: 实现优雅关闭**

在 `internal/pool/pool.go` 中添加 `GracefulShutdown` 方法：

```go
// GracefulShutdown 优雅关闭：停止接受新任务，等待正在执行的任务完成
func (p *Pool) GracefulShutdown(ctx context.Context) error {
	// 标记为关闭状态，拒绝新任务
	p.shutdown.Store(true)

	// 等待正在执行的任务完成
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if p.activeTasks.Load() == 0 {
				return nil
			}
		}
	}
}
```

- [ ] **Step 3: 运行测试并提交**

Run: `go test ./internal/pool/ -run TestGracefulShutdown -v`

```bash
git add internal/pool/graceful_shutdown.go internal/pool/graceful_shutdown_test.go
git commit -m "feat: add graceful shutdown with timeout for pool"
```

---

## Phase 3: 工具系统增强（第 5-6 周）

### Task 7: 工具执行超时

**Files:**
- Modify: `internal/tools/executor.go`
- Create: `internal/tools/executor_timeout_test.go`

- [ ] **Step 1: 编写超时测试**

```go
// internal/tools/executor_timeout_test.go
package tools

import (
	"context"
	"testing"
	"time"
)

func TestExecutor_ToolTimeout(t *testing.T) {
	registry := NewRegistry()
	slowTool := &mockTool{
		name: "slow",
		execute: func(ctx context.Context, args map[string]any) (any, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(5 * time.Second):
				return "done", nil
			}
		},
	}
	registry.Register(slowTool)

	exec := NewExecutor(registry, ExecutorConfig{
		DefaultTimeout: 100 * time.Millisecond,
	})

	ctx := context.Background()
	_, err := exec.Execute(ctx, "slow", map[string]any{})

	if err == nil {
		t.Fatal("期望超时错误")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("错误 = %v, 期望 DeadlineExceeded", err)
	}
}
```

- [ ] **Step 2: 在 Executor 中添加超时支持**

修改 `internal/tools/executor.go`，在 `Execute` 方法中注入超时 context：

```go
// Execute 执行工具，自动应用超时
func (e *Executor) Execute(ctx context.Context, name string, args map[string]any) (any, error) {
	tool := e.registry.Get(name)
	if tool == nil {
		return nil, fmt.Errorf("tool not found: %s", name)
	}

	timeout := e.cfg.DefaultTimeout
	if perTool, ok := e.cfg.PerToolTimeout[name]; ok {
		timeout = perTool
	}

	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	return tool.Execute(ctx, args)
}
```

- [ ] **Step 3: 运行测试并提交**

Run: `go test ./internal/tools/ -run TestExecutor_ToolTimeout -v`

```bash
git add internal/tools/executor.go internal/tools/executor_timeout_test.go
git commit -m "feat: add tool execution timeout support"
```

---

### Task 8: 工具结果缓存

**Files:**
- Create: `internal/tools/cache.go`
- Create: `internal/tools/cache_test.go`

- [ ] **Step 1: 编写缓存测试**

```go
// internal/tools/cache_test.go
package tools

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestToolCache_Hit(t *testing.T) {
	cache := NewResultCache(Config{TTL: 1 * time.Second, MaxEntries: 100})

	calls := 0
	executor := func(ctx context.Context, args map[string]any) (any, error) {
		calls++
		return "result", nil
	}

	// 第一次调用：miss
	result, err := cache.GetOrExecute(context.Background(), "tool1", map[string]any{"key": "val"}, executor)
	if err != nil {
		t.Fatal(err)
	}
	if result != "result" {
		t.Errorf("result = %v, 期望 result", result)
	}
	if calls != 1 {
		t.Errorf("calls = %d, 期望 1", calls)
	}

	// 第二次调用：hit
	result, err = cache.GetOrExecute(context.Background(), "tool1", map[string]any{"key": "val"}, executor)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Errorf("缓存命中后 calls = %d, 期望仍为 1", calls)
	}
}

func TestToolCache_TTLExpiry(t *testing.T) {
	cache := NewResultCache(Config{TTL: 50 * time.Millisecond, MaxEntries: 100})

	calls := atomic.Int32{}
	executor := func(ctx context.Context, args map[string]any) (any, error) {
		calls.Add(1)
		return "result", nil
	}

	cache.GetOrExecute(context.Background(), "tool1", nil, executor)
	time.Sleep(60 * time.Millisecond)
	cache.GetOrExecute(context.Background(), "tool1", nil, executor)

	if calls.Load() != 2 {
		t.Errorf("TTL 过期后 calls = %d, 期望 2", calls.Load())
	}
}
```

- [ ] **Step 2: 实现工具缓存**

```go
// internal/tools/cache.go
package tools

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// CacheConfig 缓存配置
type CacheConfig struct {
	TTL        time.Duration
	MaxEntries int
}

// ResultCache 工具结果缓存
type ResultCache struct {
	cfg     CacheConfig
	mu      sync.RWMutex
	entries map[string]*cacheEntry
}

type cacheEntry struct {
	value     any
	expiresAt time.Time
}

// NewResultCache 创建工具结果缓存
func NewResultCache(cfg CacheConfig) *ResultCache {
	if cfg.TTL <= 0 {
		cfg.TTL = 5 * time.Minute
	}
	if cfg.MaxEntries <= 0 {
		cfg.MaxEntries = 1000
	}
	return &ResultCache{
		cfg:     cfg,
		entries: make(map[string]*cacheEntry),
	}
}

// GetOrExecute 从缓存获取或执行函数
func (c *ResultCache) GetOrExecute(
	ctx context.Context,
	toolName string,
	args map[string]any,
	executor func(ctx context.Context, args map[string]any) (any, error),
) (any, error) {
	key := c.makeKey(toolName, args)

	// 尝试读取缓存
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()

	if ok && time.Now().Before(entry.expiresAt) {
		return entry.value, nil
	}

	// 执行函数
	result, err := executor(ctx, args)
	if err != nil {
		return nil, err
	}

	// 写入缓存（带 LRU 淘汰）
	c.mu.Lock()
	if len(c.entries) >= c.cfg.MaxEntries {
		c.evictOldest()
	}
	c.entries[key] = &cacheEntry{
		value:     result,
		expiresAt: time.Now().Add(c.cfg.TTL),
	}
	c.mu.Unlock()

	return result, nil
}

func (c *ResultCache) makeKey(toolName string, args map[string]any) string {
	argsJSON, _ := json.Marshal(args)
	hash := sha256.Sum256(argsJSON)
	return fmt.Sprintf("%s:%x", toolName, hash[:8])
}

func (c *ResultCache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	first := true
	for k, v := range c.entries {
		if first || v.expiresAt.Before(oldestTime) {
			oldestKey = k
			oldestTime = v.expiresAt
			first = false
		}
	}
	if oldestKey != "" {
		delete(c.entries, oldestKey)
	}
}
```

- [ ] **Step 3: 运行测试并提交**

Run: `go test ./internal/tools/ -run TestToolCache -v`

```bash
git add internal/tools/cache.go internal/tools/cache_test.go
git commit -m "feat: add tool result caching with TTL and LRU eviction"
```

---

## 验收标准

完成所有 Phase 后：

1. `go vet ./...` 通过
2. `go build ./...` 通过
3. 所有新测试通过：`go test ./internal/metrics/ ./internal/health/ ./internal/logger/ ./internal/resilience/ ./internal/tools/ -v`
4. Prometheus 端点可访问并返回正确格式
5. 健康检查端点正确反映组件状态
6. 断路器在连续失败后正确断路
7. 重试策略正确执行指数退避
8. 工具超时和缓存正常工作

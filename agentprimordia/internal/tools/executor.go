package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"agentprimordia/internal/concurrency"
)

const (
	defaultToolTimeout     = 30 * time.Second
	defaultBatchConcurrency = 10
)

// FunctionCall represents a request to execute a tool function
type FunctionCall struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Args string `json:"args"`
}

// Executor handles tool execution with logging, timing, and error handling
type Executor struct {
	registry    *Registry
	logger      *log.Logger
	timeout     time.Duration
	scopePolicy ScopePolicy
	scopeAgent  string
	fileLock    *concurrency.FileLockManager
}

// NewExecutor creates a new tool executor
func NewExecutor(registry *Registry) *Executor {
	return &Executor{
		registry: registry,
		logger:   log.Default(),
		timeout:  defaultToolTimeout,
	}
}

// WithTimeout sets the execution timeout for all tools
func (e *Executor) WithTimeout(d time.Duration) *Executor {
	e.timeout = d
	return e
}

// WithScopePolicy 注入权限策略，agentID 标识当前 Agent
// 执行工具前会检查 Agent 是否有权限操作指定资源
func (e *Executor) WithScopePolicy(policy ScopePolicy, agentID string) *Executor {
	e.scopePolicy = policy
	e.scopeAgent = agentID
	return e
}

// WithFileLock 注入文件锁管理器
// 文件写入/编辑操作会自动获取和释放文件锁
func (e *Executor) WithFileLock(fl *concurrency.FileLockManager) *Executor {
	e.fileLock = fl
	return e
}

// Execute runs a tool call by name
func (e *Executor) Execute(ctx context.Context, tc *FunctionCall) (*Result, error) {
	start := time.Now()

	e.logger.Printf("[TOOL] Executing: %s(%s)", tc.Name, tc.Args)

	tool, exists := e.registry.Get(tc.Name)
	if !exists {
		return NewErrorResult(fmt.Sprintf("tool not found: %s", tc.Name)), ErrToolNotFound
	}

	// ScopePolicy 权限检查：从参数中提取 path 资源路径
	if e.scopePolicy != nil {
		resource := extractPathFromArgs(tc.Args)
		if resource != "" && !e.scopePolicy.Allow(e.scopeAgent, resource) {
			deniedErr := NewScopeDeniedError(e.scopeAgent, resource)
			return NewErrorResult(deniedErr.Error()), deniedErr
		}
	}

	if perm, ok := e.registry.GetPermission(tc.Name); ok {
		if perm.RequireConfirmation {
			e.logger.Printf("[TOOL] Tool %s requires confirmation", tc.Name)
		}
	}

	var args json.RawMessage = json.RawMessage(tc.Args)

	// 权限检查：需要确认的工具必须通过确认回调
	if perm, ok := e.registry.GetPermission(tc.Name); ok {
		if perm.RequireConfirmation {
			if perm.ConfirmFunc != nil {
				if !perm.ConfirmFunc(tc.Name, args) {
					return NewErrorResult(fmt.Sprintf("tool %s requires confirmation and was denied", tc.Name)), ErrConfirmDenied
				}
			} else {
				// 没有确认回调时默认拒绝
				return NewErrorResult(fmt.Sprintf("tool %s requires confirmation but no confirmation handler is registered", tc.Name)), ErrConfirmDenied
			}
		}
	}

	execCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	result, err := tool.Execute(execCtx, args)

	duration := time.Since(start)

	if err != nil {
		e.logger.Printf("[TOOL] Error in %s (%v): %v", tc.Name, duration, err)
		if result == nil {
			result = NewErrorResult(err.Error())
		}
		return result, err
	}

	e.logger.Printf("[TOOL] %s completed in %v", tc.Name, duration)

	if result.Metadata == nil {
		result.Metadata = make(map[string]any)
	}
	result.Metadata["duration_ms"] = duration.Milliseconds()
	result.Metadata["tool_name"] = tc.Name

	return result, nil
}

// ExecuteBatch executes multiple tool calls concurrently
func (e *Executor) ExecuteBatch(ctx context.Context, calls []*FunctionCall) ([]*Result, error) {
	if len(calls) == 0 {
		return nil, nil
	}

	results := make([]*Result, len(calls))
	errs := make([]error, len(calls))

	// 限制并发数，避免资源耗尽
	sem := make(chan struct{}, defaultBatchConcurrency)

	var wg sync.WaitGroup
	for i, tc := range calls {
		if tc == nil {
			errs[i] = fmt.Errorf("tool call at index %d is nil", i)
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, call *FunctionCall) {
			defer wg.Done()
			defer func() { <-sem }()
			result, err := e.Execute(ctx, call)
			results[idx] = result
			errs[idx] = err
		}(i, tc)
	}
	wg.Wait()

	var firstErr error
	for _, err := range errs {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return results, firstErr
}

// extractPathFromArgs 从工具调用参数中提取 path 字段
// 用于 ScopePolicy 权限检查
func extractPathFromArgs(args string) string {
	var params map[string]json.RawMessage
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return ""
	}
	// 尝试多个常见的路径参数名
	pathKeys := []string{"path", "file_path", "target_dir", "workdir", "directory", "output_path"}
	for _, key := range pathKeys {
		if raw, ok := params[key]; ok {
			var val string
			if json.Unmarshal(raw, &val) == nil && val != "" {
				return val
			}
		}
	}
	return ""
}

package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"regexp"
	"sync"
	"time"

	"agentprimordia/internal/concurrency"
)

const (
	defaultToolTimeout      = 30 * time.Second
	defaultBatchConcurrency = 10
)

// FunctionCall represents a request to execute a tool function
type FunctionCall struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Args string `json:"args"`
}

// Executor handles tool execution with logging, timing, and error handling
// perf-v5 Task 20：内部新增 slogLogger 字段（结构化日志）；保留 logger *log.Logger 兼容旧 API
type Executor struct {
	registry    *Registry
	logger      *log.Logger
	slogger     *slog.Logger // perf-v5 Task 20：结构化日志（slog），优先使用
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
		slogger:  slog.Default(), // perf-v5 Task 20：默认 slog
		timeout:  defaultToolTimeout,
	}
}

// WithSlogLogger 注入自定义 *slog.Logger（perf-v5 Task 20）
func (e *Executor) WithSlogLogger(l *slog.Logger) *Executor {
	e.slogger = l
	return e
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

	// perf-v5 Task 20：对参数做脱敏后再记录，避免 password/token 等敏感字段泄漏到日志
	e.logger.Printf("[TOOL] Executing: %s(args_len=%d)", tc.Name, len(tc.Args))
	if e.slogger != nil {
		e.slogger.Debug("tool executing",
			"tool", tc.Name,
			"args_len", len(tc.Args),
			"args_preview", redactSensitiveArgs(tc.Args),
		)
	}

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

	// perf-v5 Task 1：工具 panic recover，避免任意工具 panic 杀死整个 agent 进程
	result, err := e.safeExecute(execCtx, tool, args)

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

// safeExecute 包装工具调用并捕获 panic（perf-v5 Task 1）
// 任意工具 panic 转为 error 返回，避免杀死 agent 进程
func (e *Executor) safeExecute(ctx context.Context, tool Tool, args json.RawMessage) (result *Result, err error) {
	defer func() {
		if r := recover(); r != nil {
			e.logger.Printf("[TOOL] panic recovered in %s: %v", tool.Name(), r)
			if e.slogger != nil {
				e.slogger.Error("tool panic recovered", "tool", tool.Name(), "panic", r)
			}
			result = NewErrorResult(fmt.Sprintf("tool %s panic: %v", tool.Name(), r))
			err = fmt.Errorf("tool %s panic: %v", tool.Name(), r)
		}
	}()
	return tool.Execute(ctx, args)
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
			// perf-v5 Task 1：goroutine 顶层 panic recover
			defer func() {
				if r := recover(); r != nil {
					e.logger.Printf("[TOOL] panic in batch goroutine: %v", r)
					if e.slogger != nil {
						e.slogger.Error("tool batch panic", "panic", r)
					}
					results[idx] = NewErrorResult(fmt.Sprintf("tool panic: %v", r))
					errs[idx] = fmt.Errorf("tool panic: %v", r)
				}
			}()
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
// 优化（Task 9）：使用 json.Decoder 替代 Unmarshal，按需查找常见路径字段；
// 第一个匹配字段后立即返回，减少 JSON 解析开销。
func extractPathFromArgs(args string) string {
	if args == "" {
		return ""
	}
	dec := json.NewDecoder(bytes.NewReader([]byte(args)))
	dec.UseNumber()
	// 流式解析：直到找到第一个 path 字段
	for {
		tok, err := dec.Token()
		if err != nil {
			return ""
		}
		// 在对象开始时进入键值对循环
		if delim, ok := tok.(json.Delim); ok && delim == '{' {
			// 优化：不再解析整个 map，而是逐个键查找
			for dec.More() {
				// 读取 key
				keyTok, err := dec.Token()
				if err != nil {
					return ""
				}
				key, ok := keyTok.(string)
				if !ok {
					// 跳过 value
					var skip json.RawMessage
					if err := dec.Decode(&skip); err != nil {
						return ""
					}
					continue
				}
				// 检查是否为常见路径字段
				if isPathKey(key) {
					var val string
					if err := dec.Decode(&val); err == nil && val != "" {
						return val
					}
					// value 不是 string，继续扫描
					var skip json.RawMessage
					_ = dec.Decode(&skip)
					continue
				}
				// 跳过 value
				var skip json.RawMessage
				if err := dec.Decode(&skip); err != nil {
					return ""
				}
			}
			return ""
		}
		// 跳过非对象起始 token
	}
}

// isPathKey 判断 key 是否为常见的路径参数名
func isPathKey(key string) bool {
	switch key {
	case "path", "file_path", "target_dir", "workdir", "directory", "output_path":
		return true
	}
	return false
}

// redactSensitiveArgs 扫描 JSON 参数，将敏感字段值替换为 "***REDACTED***"
// 返回脱敏后的 JSON 字符串（截断到 256 字符）
// perf-v5 Task 20：避免 password / token / api_key 等敏感字段泄漏到日志
//
// 实现说明：使用正则匹配常见 flat JSON 的 "key":"value" 模式。
// 对嵌套对象的深度不在本函数覆盖范围（只脱敏顶层 key），
// 复杂场景可改用 json.Decoder + 递归遍历。
func redactSensitiveArgs(args string) string {
	if args == "" {
		return ""
	}
	// 截断：避免极长 args 拖慢日志
	if len(args) > 1024 {
		args = args[:1024] + "...(truncated)"
	}

	redacted := args
	sensitiveKeys := []string{
		"password", "passwd", "secret", "token", "api_key", "apikey",
		"access_key", "secret_key", "authorization", "auth", "credential",
		"private_key", "session_token", "cookie",
	}
	for _, key := range sensitiveKeys {
		// 匹配 "key":"value" 或 "key": "value" 模式（大小写不敏感）
		// 用 ReplaceAllStringFunc 保留原 key 的大小写
		pattern := regexp.MustCompile(`(?i)("` + regexp.QuoteMeta(key) + `"\s*:\s*)"[^"]*"`)
		redacted = pattern.ReplaceAllStringFunc(redacted, func(match string) string {
			// match 形如 "PASSWORD":"hunter2" → 保留 "PASSWORD": 部分
			submatch := pattern.FindStringSubmatch(match)
			if len(submatch) >= 2 {
				return submatch[1] + `"***REDACTED***"`
			}
			return match
		})
	}
	if len(redacted) > 256 {
		return redacted[:256] + "...(truncated)"
	}
	return redacted
}

// react_llm.go — LLM 调用 + 工具执行
// 包含工具执行、LLM 调用重试、错误处理等底层操作
package agent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"agentprimordia/internal/llm"
	"agentprimordia/internal/tools"
)

// ErrMaxTurnsExceeded is returned when the agent exceeds max turn limit
var ErrMaxTurnsExceeded = errors.New("max turns exceeded")

var ErrBudgetExceeded = errors.New("budget exceeded")

var ErrNoToolkit = errors.New("no toolkit configured")

// executeTool runs a single tool call
func (a *ReActAgent) executeTool(ctx context.Context, tc ToolCall) (*ToolResult, error) {
	if a.config.Toolkit == nil {
		return &ToolResult{
			ToolCallID: tc.ID,
			Content:    "error: no toolkit configured",
			IsError:    true,
		}, ErrNoToolkit
	}

	executor := tools.NewExecutor(a.config.Toolkit)
	fc := tools.FunctionCall{
		ID:   tc.ID,
		Name: tc.Name,
		Args: tc.Args,
	}

	result, err := executor.Execute(ctx, &fc)
	if err != nil {
		return &ToolResult{
			ToolCallID: tc.ID,
			Content:    err.Error(),
			IsError:    true,
		}, err
	}

	return &ToolResult{
		ToolCallID: tc.ID,
		Content:    result.Content,
		IsError:    result.IsError,
	}, nil
}

// callToolsWithRetry calls LLM with function calling support, with retry on transient errors
func (a *ReActAgent) callToolsWithRetry(ctx context.Context, messages []llm.ChatMessage, toolDefs []map[string]any) (*llm.ToolCallResponse, error) {
	definitions := make([]llm.ToolDefinition, 0, len(toolDefs))
	for _, def := range toolDefs {
		fn, ok := def["function"].(map[string]any)
		if !ok {
			continue
		}
		typ, _ := def["type"].(string)
		name, _ := fn["name"].(string)
		desc, _ := fn["description"].(string)
		params, _ := fn["parameters"].(map[string]any)
		if name == "" {
			continue
		}
		definitions = append(definitions, llm.ToolDefinition{
			Type: typ,
			Function: llm.FunctionDefinition{
				Name:        name,
				Description: desc,
				Parameters:  params,
			},
		})
	}

	req := &llm.ToolCallRequest{
		Messages: messages,
		Tools:    definitions,
	}

	const maxRetries = 2
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		resp, err := a.config.Model.CallTools(ctx, req)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		a.logger.Warn("CallTools 失败，将重试", "attempt", attempt+1, "error", err)
	}
	return nil, fmt.Errorf("callTools failed after %d retries: %w", maxRetries, lastErr)
}

// completeWithRetry calls LLM for simple completion, with retry on transient errors
func (a *ReActAgent) completeWithRetry(ctx context.Context, messages []llm.ChatMessage) (*llm.CompletionResponse, error) {
	req := &llm.CompletionRequest{
		Messages:    messages,
		Temperature: llm.Float64Ptr(a.config.Temperature),
	}

	const maxRetries = 2
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		resp, err := a.config.Model.Complete(ctx, req)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		a.logger.Warn("Complete 失败，将重试", "attempt", attempt+1, "error", err)
	}
	return nil, fmt.Errorf("complete failed after %d retries: %w", maxRetries, lastErr)
}

// handleOnError triggers error hook
func (a *ReActAgent) handleOnError(_ context.Context, err error) {
	_ = a.fireHook(HookOnError, &HookContext{Error: err})
}

// react_reasoning.go — LLM 推理阶段
// 包含同步推理和流式推理两种模式的实现
package agent

import (
	"context"
	"strings"
	"time"

	"agentprimordia/internal/llm"
)

// syncReasoning 非流式推理阶段
// 优化（Task 2.5）：toolDefs 一次性转换为 []llm.ToolDefinition 并在内部复用，
// 避免每轮 LLM 调用都进行 map 反解。
func (a *ReActAgent) syncReasoning(ctx context.Context, llmMessages []llm.ChatMessage, toolDefs []llm.ToolDefinition, llmStart time.Time) (Thought, error) {
	var thought Thought

	if len(toolDefs) > 0 {
		resp, err := a.callToolsWithRetry(ctx, llmMessages, toolDefs)
		if err != nil {
			a.recordLLM(time.Since(llmStart), err)
			return Thought{}, err
		}
		if len(resp.ToolCalls) == 0 && resp.Content == "" {
			completeResp, completeErr := a.completeWithRetry(ctx, llmMessages)
			if completeErr != nil {
				a.recordLLM(time.Since(llmStart), completeErr)
				return Thought{}, completeErr
			}
			thought = Thought{Content: completeResp.Content, Usage: completeResp.Usage}
		} else {
			thought = Thought{
				Content:   resp.Content,
				ToolCalls: convertToToolCalls(resp.ToolCalls),
				Usage:     resp.Usage,
			}
		}
		a.recordLLM(time.Since(llmStart), nil)
	} else {
		resp, err := a.completeWithRetry(ctx, llmMessages)
		if err != nil {
			a.recordLLM(time.Since(llmStart), err)
			return Thought{}, err
		}
		thought = Thought{Content: resp.Content, Usage: resp.Usage}
		a.recordLLM(time.Since(llmStart), nil)
	}

	return thought, nil
}

// streamReasoning 流式推理阶段
// 先尝试 Stream 接口，失败则回退到非流式调用。
// 优化（Task 1.5 / Task 2.5）：
//   - 流式拼接改用 strings.Builder，避免 O(n^2) 的字符串拼接
//   - toolDefs 一次性转换，复用传入的 []llm.ToolDefinition，不再重复 tk.Definitions()
func (a *ReActAgent) streamReasoning(ctx context.Context, cfg loopConfig, llmMessages []llm.ChatMessage, toolDefs []llm.ToolDefinition, llmStart time.Time) (Thought, error) {
	streamCh, streamErr := a.config.Model.Stream(ctx, &llm.CompletionRequest{
		Messages:    llmMessages,
		Temperature: llm.Float64Ptr(a.config.Temperature),
	})

	if streamErr == nil {
		// 优化（Task 1.5）：使用 strings.Builder 拼接流式内容，O(n) 复杂度
		var contentBuilder strings.Builder
		// 预分配容量以减少 realloc（典型 4K 流式响应）
		contentBuilder.Grow(4096)

		for chunk := range streamCh {
			if ctx.Err() != nil {
				_ = a.lifecycle.SetStatus(StatusCancelled)
				a.emitStream(cfg, StreamEvent{Type: StreamEventError, Content: ctx.Err().Error()})
				return Thought{}, ctx.Err()
			}
			if chunk.Content != "" {
				contentBuilder.WriteString(chunk.Content)
				a.emitStream(cfg, StreamEvent{Type: StreamEventToken, Content: chunk.Content})
			}
			if chunk.Done {
				break
			}
		}
		thought := Thought{Content: contentBuilder.String()}

		// 优化（Task 2.5）：直接复用外层传入的 toolDefs，不再调用 tk.Definitions()
		if len(toolDefs) > 0 {
			resp, err := a.callToolsWithRetry(ctx, llmMessages, toolDefs)
			if err == nil && len(resp.ToolCalls) > 0 {
				thought = Thought{
					Content:   resp.Content,
					ToolCalls: convertToToolCalls(resp.ToolCalls),
				}
			}
		}

		a.recordLLM(time.Since(llmStart), nil)
		return thought, nil
	}

	// Fallback: 非流式调用
	if len(toolDefs) > 0 {
		resp, err := a.callToolsWithRetry(ctx, llmMessages, toolDefs)
		if err != nil {
			a.recordLLM(time.Since(llmStart), err)
			_ = a.lifecycle.SetStatus(StatusFailed)
			a.emitStream(cfg, StreamEvent{Type: StreamEventError, Content: err.Error()})
			return Thought{}, err
		}
		if len(resp.ToolCalls) == 0 && resp.Content == "" {
			completeResp, completeErr := a.completeWithRetry(ctx, llmMessages)
			if completeErr != nil {
				a.recordLLM(time.Since(llmStart), completeErr)
				_ = a.lifecycle.SetStatus(StatusFailed)
				a.emitStream(cfg, StreamEvent{Type: StreamEventError, Content: completeErr.Error()})
				return Thought{}, completeErr
			}
			a.emitStream(cfg, StreamEvent{Type: StreamEventThought, Content: completeResp.Content})
			a.recordLLM(time.Since(llmStart), nil)
			return Thought{Content: completeResp.Content}, nil
		}
		a.emitStream(cfg, StreamEvent{Type: StreamEventThought, Content: resp.Content})
		a.recordLLM(time.Since(llmStart), nil)
		return Thought{
			Content:   resp.Content,
			ToolCalls: convertToToolCalls(resp.ToolCalls),
		}, nil
	}

	resp, err := a.completeWithRetry(ctx, llmMessages)
	if err != nil {
		a.recordLLM(time.Since(llmStart), err)
		_ = a.lifecycle.SetStatus(StatusFailed)
		a.emitStream(cfg, StreamEvent{Type: StreamEventError, Content: err.Error()})
		return Thought{}, err
	}
	a.emitStream(cfg, StreamEvent{Type: StreamEventThought, Content: resp.Content})
	a.recordLLM(time.Since(llmStart), nil)
	return Thought{Content: resp.Content}, nil
}

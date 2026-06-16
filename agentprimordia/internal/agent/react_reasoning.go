// react_reasoning.go — LLM 推理阶段
// 包含同步推理和流式推理两种模式的实现
package agent

import (
	"context"
	"time"

	"agentprimordia/internal/llm"
)

// syncReasoning 非流式推理阶段
func (a *ReActAgent) syncReasoning(ctx context.Context, llmMessages []llm.ChatMessage, toolDefs []map[string]any, llmStart time.Time) (Thought, error) {
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
// 先尝试 Stream 接口，失败则回退到非流式调用
func (a *ReActAgent) streamReasoning(ctx context.Context, cfg loopConfig, llmMessages []llm.ChatMessage, toolDefs []map[string]any, llmStart time.Time) Thought {
	streamCh, streamErr := a.config.Model.Stream(ctx, &llm.CompletionRequest{
		Messages:    llmMessages,
		Temperature: llm.Float64Ptr(a.config.Temperature),
	})

	if streamErr == nil {
		var fullContent string
		for chunk := range streamCh {
			if ctx.Err() != nil {
				_ = a.lifecycle.SetStatus(StatusCancelled)
				a.emitStream(cfg, StreamEvent{Type: StreamEventError, Content: ctx.Err().Error()})
				return Thought{}
			}
			if chunk.Content != "" {
				fullContent += chunk.Content
				a.emitStream(cfg, StreamEvent{Type: StreamEventToken, Content: chunk.Content})
			}
			if chunk.Done {
				break
			}
		}
		thought := Thought{Content: fullContent}

		if a.config.Toolkit != nil {
			td := a.config.Toolkit.Definitions()
			if len(td) > 0 {
				resp, err := a.callToolsWithRetry(ctx, llmMessages, td)
				if err == nil && len(resp.ToolCalls) > 0 {
					thought = Thought{
						Content:   resp.Content,
						ToolCalls: convertToToolCalls(resp.ToolCalls),
					}
				}
			}
		}

		a.recordLLM(time.Since(llmStart), nil)
		return thought
	}

	// Fallback: 非流式调用
	if len(toolDefs) > 0 {
		resp, err := a.callToolsWithRetry(ctx, llmMessages, toolDefs)
		if err != nil {
			a.recordLLM(time.Since(llmStart), err)
			_ = a.lifecycle.SetStatus(StatusFailed)
			a.emitStream(cfg, StreamEvent{Type: StreamEventError, Content: err.Error()})
			return Thought{}
		}
		if len(resp.ToolCalls) == 0 && resp.Content == "" {
			completeResp, completeErr := a.completeWithRetry(ctx, llmMessages)
			if completeErr != nil {
				a.recordLLM(time.Since(llmStart), completeErr)
				_ = a.lifecycle.SetStatus(StatusFailed)
				a.emitStream(cfg, StreamEvent{Type: StreamEventError, Content: completeErr.Error()})
				return Thought{}
			}
			a.emitStream(cfg, StreamEvent{Type: StreamEventThought, Content: completeResp.Content})
			a.recordLLM(time.Since(llmStart), nil)
			return Thought{Content: completeResp.Content}
		}
		a.emitStream(cfg, StreamEvent{Type: StreamEventThought, Content: resp.Content})
		a.recordLLM(time.Since(llmStart), nil)
		return Thought{
			Content:   resp.Content,
			ToolCalls: convertToToolCalls(resp.ToolCalls),
		}
	}

	resp, err := a.completeWithRetry(ctx, llmMessages)
	if err != nil {
		a.recordLLM(time.Since(llmStart), err)
		_ = a.lifecycle.SetStatus(StatusFailed)
		a.emitStream(cfg, StreamEvent{Type: StreamEventError, Content: err.Error()})
		return Thought{}
	}
	a.emitStream(cfg, StreamEvent{Type: StreamEventThought, Content: resp.Content})
	a.recordLLM(time.Since(llmStart), nil)
	return Thought{Content: resp.Content}
}

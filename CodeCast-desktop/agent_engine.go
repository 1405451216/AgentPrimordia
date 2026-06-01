package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ==================== LLM Response ====================

// LLMResponse represents the parsed response from the LLM API
type LLMResponse struct {
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// ==================== System Prompt ====================

// agentSystemPrompt generates the system prompt for sub-agents
func agentSystemPrompt(taskPrompt string, filesScope []string) string {
	var sb strings.Builder

	sb.WriteString("你是一个代码助手子代理（Sub-Agent），负责独立执行分配给你的任务。\n\n")
	sb.WriteString("## 任务\n\n")
	sb.WriteString(taskPrompt)
	sb.WriteString("\n\n")

	sb.WriteString("## 规则\n\n")
	sb.WriteString("1. 专注于分配给你的任务，不要偏离主题。\n")
	sb.WriteString("2. 使用提供的工具完成任务，每次只调用必要的工具。\n")
	sb.WriteString("3. 完成任务后，输出简洁的结果摘要。\n")
	sb.WriteString("4. 如果遇到无法解决的错误，清晰地描述问题并停止。\n")
	sb.WriteString("5. 不要请求用户输入，你需要独立完成任务。\n")

	if len(filesScope) > 0 {
		sb.WriteString("\n## 文件范围限制\n\n")
		sb.WriteString("你只能操作以下文件或目录：\n")
		for _, f := range filesScope {
			sb.WriteString("- ")
			sb.WriteString(f)
			sb.WriteString("\n")
		}
		sb.WriteString("\n不要修改范围之外的文件。\n")
	}

	return sb.String()
}

// ==================== Tool Definitions ====================

// agentToolDefinitions returns the tool definitions for sub-agent function calling
func agentToolDefinitions() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "read_file",
				"description": "读取指定路径的文件内容",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "文件的绝对路径",
						},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "write_file",
				"description": "将内容写入指定路径的文件（覆盖已有内容或创建新文件）",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "文件的绝对路径",
						},
						"content": map[string]interface{}{
							"type":        "string",
							"description": "要写入的文件内容",
						},
					},
					"required": []string{"path", "content"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "edit_file",
				"description": "对文件进行搜索替换编辑",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "文件的绝对路径",
						},
						"old_string": map[string]interface{}{
							"type":        "string",
							"description": "要替换的原始文本（必须精确匹配）",
						},
						"new_string": map[string]interface{}{
							"type":        "string",
							"description": "替换后的新文本",
						},
					},
					"required": []string{"path", "old_string", "new_string"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "run_command",
				"description": "在系统 shell 中执行命令",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"command": map[string]interface{}{
							"type":        "string",
							"description": "要执行的命令",
						},
						"working_dir": map[string]interface{}{
							"type":        "string",
							"description": "命令的工作目录（可选）",
						},
					},
					"required": []string{"command"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "search",
				"description": "在文件中搜索匹配的文本或正则表达式",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"pattern": map[string]interface{}{
							"type":        "string",
							"description": "搜索的正则表达式模式",
						},
						"path": map[string]interface{}{
							"type":        "string",
							"description": "搜索的目录或文件路径",
						},
						"include": map[string]interface{}{
							"type":        "string",
							"description": "文件匹配模式（如 *.go）",
						},
					},
					"required": []string{"pattern"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "web_fetch",
				"description": "获取 URL 的网页内容",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"url": map[string]interface{}{
							"type":        "string",
							"description": "要获取内容的 URL",
						},
					},
					"required": []string{"url"},
				},
			},
		},
	}
}

// ==================== Agent Loop ====================

// runAgentLoop is the core execution loop for a sub-agent
func (pool *AgentPool) runAgentLoop(agent *SubAgent) {
	// Set up agent context
	agentCtx, agentCancel := context.WithCancel(pool.ctx)
	agent.ctx = agentCtx
	agent.cancel = agentCancel
	defer agentCancel()

	// Mark as running
	pool.mu.Lock()
	agent.Status = AgentStatusRunning
	agent.UpdatedAt = time.Now()
	pool.mu.Unlock()
	pool.emitEvent(agent, "status")

	// Initialize messages with system prompt and user message
	systemPrompt := agentSystemPrompt(agent.Prompt, agent.FilesScope)
	agent.Messages = []AgentMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: "请开始执行任务。"},
	}

	// Get API credentials
	pool.app.mu.Lock()
	creds, credErr := pool.app.resolveCredentialsLocked("")
	pool.app.mu.Unlock()
	if credErr != nil {
		pool.mu.Lock()
		agent.Status = AgentStatusFailed
		agent.Error = credErr.Error()
		agent.UpdatedAt = time.Now()
		pool.mu.Unlock()
		pool.emitEvent(agent, "status")
		return
	}
	apiKey := creds.APIKey
	apiURL := creds.APIURL

	// Main execution loop
	for {
		// Check for cancellation
		select {
		case <-agentCtx.Done():
			pool.mu.Lock()
			agent.Status = AgentStatusCancelled
			agent.UpdatedAt = time.Now()
			pool.mu.Unlock()
			pool.emitEvent(agent, "status")
			return
		default:
		}

		// Check max turns
		if agent.TurnCount >= agent.MaxTurns {
			pool.mu.Lock()
			agent.Status = AgentStatusFailed
			agent.Error = fmt.Sprintf("超出最大轮次限制 (%d)", agent.MaxTurns)
			agent.UpdatedAt = time.Now()
			pool.mu.Unlock()
			pool.emitEvent(agent, "status")
			return
		}

		// Call LLM
		resp, err := pool.callLLM(agentCtx, agent, apiKey, apiURL)
		if err != nil {
			// Check if it's a context cancellation
			if agentCtx.Err() != nil {
				pool.mu.Lock()
				agent.Status = AgentStatusCancelled
				agent.UpdatedAt = time.Now()
				pool.mu.Unlock()
				pool.emitEvent(agent, "status")
				return
			}
			pool.mu.Lock()
			agent.Status = AgentStatusFailed
			agent.Error = fmt.Sprintf("LLM 调用失败: %v", err)
			agent.UpdatedAt = time.Now()
			pool.mu.Unlock()
			pool.emitEvent(agent, "status")
			return
		}

		agent.TurnCount++
		agent.UpdatedAt = time.Now()
		pool.emitEvent(agent, "progress")

		// If no tool calls, the agent is done
		if len(resp.ToolCalls) == 0 {
			// Add assistant message
			agent.Messages = append(agent.Messages, AgentMessage{
				Role:    "assistant",
				Content: resp.Content,
			})

			pool.mu.Lock()
			agent.Status = AgentStatusCompleted
			agent.Result = resp.Content
			agent.UpdatedAt = time.Now()
			pool.mu.Unlock()
			pool.emitEvent(agent, "result")
			saveAgentState(agent)
			return
		}

		// Add assistant message with tool calls
		agent.Messages = append(agent.Messages, AgentMessage{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		// Execute each tool call
		for _, tc := range resp.ToolCalls {
			pool.emitEvent(agent, "tool_use")

			result := pool.executeTool(agent, tc)

			// Add tool result message
			agent.Messages = append(agent.Messages, AgentMessage{
				Role:       "tool",
				ToolResult: &result,
			})
		}

		// Persist state
		saveAgentState(agent)

		// Trim messages if too long (keep system + last N messages)
		if len(agent.Messages) > 100 {
			// Keep system message (first) and the last 80 messages
			trimmed := make([]AgentMessage, 0, 81)
			trimmed = append(trimmed, agent.Messages[0]) // system prompt
			trimmed = append(trimmed, agent.Messages[len(agent.Messages)-80:]...)
			agent.Messages = trimmed
		}
	}
}

// ==================== Helper ====================

// getLastAssistantContent returns the content of the last assistant message
func getLastAssistantContent(messages []AgentMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" && messages[i].Content != "" {
			return messages[i].Content
		}
	}
	return ""
}

// ==================== LLM API Call ====================

// callLLM calls the LLM API with the agent's message history
func (pool *AgentPool) callLLM(ctx context.Context, agent *SubAgent, apiKey string, apiURL string) (*LLMResponse, error) {
	// Build API messages array
	apiMessages := make([]map[string]interface{}, 0, len(agent.Messages))

	for _, msg := range agent.Messages {
		switch msg.Role {
		case "system":
			apiMessages = append(apiMessages, map[string]interface{}{
				"role":    "system",
				"content": msg.Content,
			})
		case "user":
			apiMessages = append(apiMessages, map[string]interface{}{
				"role":    "user",
				"content": msg.Content,
			})
		case "assistant":
			assistantMsg := map[string]interface{}{
				"role":    "assistant",
				"content": msg.Content,
			}
			if len(msg.ToolCalls) > 0 {
				toolCalls := make([]map[string]interface{}, len(msg.ToolCalls))
				for i, tc := range msg.ToolCalls {
					toolCalls[i] = map[string]interface{}{
						"id":   tc.ID,
						"type": "function",
						"function": map[string]interface{}{
							"name":      tc.Name,
							"arguments": tc.Args,
						},
					}
				}
				assistantMsg["tool_calls"] = toolCalls
			}
			apiMessages = append(apiMessages, assistantMsg)
		case "tool":
			if msg.ToolResult != nil {
				apiMessages = append(apiMessages, map[string]interface{}{
					"role":         "tool",
					"tool_call_id": msg.ToolResult.ToolCallID,
					"content":      msg.ToolResult.Content,
				})
			}
		}
	}

	// Build request body
	reqBody := map[string]interface{}{
		"model":      pool.app.llmConfig.Model,
		"messages":   apiMessages,
		"tools":      agentToolDefinitions(),
		"max_tokens": 4096,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", apiURL+"/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	// Execute request
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	// Read response with size limit
	limitedReader := io.LimitReader(resp.Body, MaxResponseSize)
	respBody, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API 返回错误 (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	// Parse response
	var apiResp struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}

	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if apiResp.Error != nil {
		return nil, fmt.Errorf("API 错误: %s (%s)", apiResp.Error.Message, apiResp.Error.Type)
	}

	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("API 返回空 choices")
	}

	// Build LLMResponse
	choice := apiResp.Choices[0]
	result := &LLMResponse{
		Content: choice.Message.Content,
	}

	if len(choice.Message.ToolCalls) > 0 {
		result.ToolCalls = make([]ToolCall, len(choice.Message.ToolCalls))
		for i, tc := range choice.Message.ToolCalls {
			result.ToolCalls[i] = ToolCall{
				ID:   tc.ID,
				Name: tc.Function.Name,
				Args: tc.Function.Arguments,
			}
		}
	}

	return result, nil
}

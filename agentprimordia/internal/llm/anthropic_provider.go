package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

const (
	anthropicDefaultBaseURL   = "https://api.anthropic.com"
	anthropicAPIVersion       = "2023-06-01"
	defaultAnthropicMaxContext = 200000
	defaultAnthropicMaxTokens  = 4096
)

// AnthropicProvider 实现 Claude 系列模型调用
type AnthropicProvider struct {
	config Config
	client *http.Client
}

// NewAnthropicProvider 创建 Anthropic Claude Provider
func NewAnthropicProvider(cfg Config) (*AnthropicProvider, error) {
	if cfg.APIKey == "" {
		return nil, ErrAPIKeyRequired
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = anthropicDefaultBaseURL
	}
	cfg.BaseURL = strings.TrimRight(baseURL, "/")

	if cfg.Model == "" {
		cfg.Model = "claude-sonnet-4-20250514"
	}

	return &AnthropicProvider{
		config: cfg,
		client: &http.Client{Timeout: defaultTimeout},
	}, nil
}

func (p *AnthropicProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	model := p.resolveModel(req)
	messages, systemMsg := p.convertMessages(req.Messages)

	body := map[string]any{
		"model":      model,
		"messages":   messages,
		"max_tokens": p.resolveMaxTokens(req),
	}
	if systemMsg != "" {
		body["system"] = systemMsg
	}
	if temp := ResolveTemperature(req.Temperature, p.config.Temperature); temp != nil {
		body["temperature"] = *temp
	}

	// 结构化输出：Anthropic 通过 tool_choice + 单工具注入实现
	if req.ResponseFormat != nil {
		p.injectStructuredOutput(body, req.ResponseFormat)
	}

	raw, err := p.doRequest(ctx, "/v1/messages", body)
	if err != nil {
		return nil, err
	}

	var resp anthropicMessagesResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrResponseParseFailed, err)
	}

	content := ""
	if len(resp.Content) > 0 {
		content = resp.Content[0].Text
	}

	return &CompletionResponse{
		ID:    resp.ID,
		Model: model,
		Content: content,
		Role:    "assistant",
		Usage: Usage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		},
	}, nil
}

func (p *AnthropicProvider) Stream(ctx context.Context, req *CompletionRequest) (<-chan Chunk, error) {
	model := p.resolveModel(req)
	messages, systemMsg := p.convertMessages(req.Messages)

	body := map[string]any{
		"model":      model,
		"messages":   messages,
		"max_tokens": p.resolveMaxTokens(req),
		"stream":     true,
	}
	if systemMsg != "" {
		body["system"] = systemMsg
	}
	if temp := ResolveTemperature(req.Temperature, p.config.Temperature); temp != nil {
		body["temperature"] = *temp
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.config.BaseURL+"/v1/messages", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.setHeaders(httpReq)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
		return nil, fmt.Errorf("Anthropic API returned HTTP %d: %s", resp.StatusCode, respBody)
	}

	ch := make(chan Chunk, 32)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
			}

			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")

			var sseEvt anthropicSSEEvent
			if err := json.Unmarshal([]byte(data), &sseEvt); err != nil {
				continue
			}

			var chunk Chunk
			switch sseEvt.Type {
			case "content_block_delta":
				if sseEvt.Delta != nil {
					chunk = Chunk{Content: sseEvt.Delta.Text}
				}
			case "message_delta":
				if sseEvt.Usage != nil {
					chunk = Chunk{
						Usage: &Usage{
							CompletionTokens: sseEvt.Usage.OutputTokens,
						},
					}
				}
			case "message_stop":
				chunk = Chunk{Done: true}
			case "error":
				chunk = Chunk{Done: true}
			}

			select {
			case ch <- chunk:
			case <-ctx.Done():
				return
			}

			if chunk.Done {
				return
			}
		}
		if err := scanner.Err(); err != nil {
			slog.Warn("Anthropic 流式读取错误", "error", err)
		}
		select {
		case ch <- Chunk{Done: true}:
		case <-ctx.Done():
		}
	}()

	return ch, nil
}

func (p *AnthropicProvider) CallTools(ctx context.Context, req *ToolCallRequest) (*ToolCallResponse, error) {
	model := p.config.Model
	if req.Model != "" {
		model = req.Model
	}
	messages, systemMsg := p.convertMessages(req.Messages)

	tools := make([]anthropicTool, len(req.Tools))
	for i, t := range req.Tools {
		tools[i] = anthropicTool{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			InputSchema: t.Function.Parameters,
		}
	}

	body := map[string]any{
		"model":      model,
		"messages":   messages,
		"max_tokens": p.resolveMaxTokens(nil),
		"tools":      tools,
	}
	if systemMsg != "" {
		body["system"] = systemMsg
	}

	raw, err := p.doRequest(ctx, "/v1/messages", body)
	if err != nil {
		return nil, err
	}

	var resp anthropicMessagesResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrResponseParseFailed, err)
	}

	result := &ToolCallResponse{
		Usage: Usage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		},
	}

	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			result.Content = block.Text
		case "tool_use":
			result.ToolCalls = append(result.ToolCalls, FunctionCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: block.Input,
			})
		}
	}

	return result, nil
}

func (p *AnthropicProvider) Info() ModelInfo {
	contextSizes := map[string]int{
		"claude-sonnet-4-20250514":   200000,
		"claude-3-5-sonnet-20241022": 200000,
		"claude-3-5-haiku-20241022":  200000,
		"claude-3-opus-20240229":     200000,
		"claude-3-haiku-20240307":    200000,
	}
	maxCtx := defaultAnthropicMaxContext
	if c, ok := contextSizes[p.config.Model]; ok {
		maxCtx = c
	}
	return ModelInfo{
		Name:              p.config.Model,
		Provider:          "anthropic",
		MaxContext:        maxCtx,
		SupportsTools:     true,
		SupportsStreaming: true,
	}
}

// ===== 内部方法 =====

func (p *AnthropicProvider) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.config.APIKey)
	req.Header.Set("anthropic-version", anthropicAPIVersion)
	req.Header.Set("User-Agent", userAgent)
}

func (p *AnthropicProvider) doRequest(ctx context.Context, path string, body any) (json.RawMessage, error) {
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.config.BaseURL+path, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.setHeaders(httpReq)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	limitedReader := io.LimitReader(resp.Body, maxResponseSize)
	respBody, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error *APIError `json:"error"`
		}
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != nil {
			return nil, errResp.Error
		}
		return nil, fmt.Errorf("Anthropic API returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// convertMessages 将通用 ChatMessage 转换为 Anthropic 格式
// Anthropic 要求 system 消息不在 messages 数组中，而是单独的 system 参数
func (p *AnthropicProvider) convertMessages(msgs []ChatMessage) ([]map[string]any, string) {
	var systemMsg string
	var result []map[string]any

	for _, m := range msgs {
		if m.Role == "system" {
			systemMsg = m.Content
			continue
		}
		result = append(result, map[string]any{
			"role":    m.Role,
			"content": m.Content,
		})
	}

	if len(result) == 0 {
		result = []map[string]any{}
	}
	return result, systemMsg
}

func (p *AnthropicProvider) resolveModel(req *CompletionRequest) string {
	if req != nil && req.Model != "" {
		return req.Model
	}
	return p.config.Model
}

func (p *AnthropicProvider) resolveMaxTokens(req *CompletionRequest) int {
	if req != nil && req.MaxTokens > 0 {
		return req.MaxTokens
	}
	if p.config.MaxTokens > 0 {
		return p.config.MaxTokens
	}
	return defaultAnthropicMaxTokens
}

// injectStructuredOutput 将结构化输出要求注入 Anthropic 请求体
// Anthropic 不支持 response_format，通过 tool_choice + 单工具注入实现等效效果
func (p *AnthropicProvider) injectStructuredOutput(body map[string]any, rf *ResponseFormat) {
	if rf.JSONSchema == nil {
		return
	}

	schemaName := rf.JSONSchema.Name
	if schemaName == "" {
		schemaName = "structured_output"
	}

	tool := anthropicTool{
		Name:        schemaName,
		Description: rf.JSONSchema.Description,
		InputSchema: rf.JSONSchema.Schema,
	}

	existingTools, _ := body["tools"].([]anthropicTool)
	allTools := append(existingTools, tool)
	body["tools"] = allTools
	body["tool_choice"] = map[string]any{
		"type": "tool",
		"name": schemaName,
	}
}

// ===== Anthropic API 响应类型 =====

type anthropicMessagesResponse struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Role    string `json:"role"`
	Content []struct {
		Type  string `json:"type"`
		Text  string `json:"text,omitempty"`
		ID    string `json:"id,omitempty"`
		Name  string `json:"name,omitempty"`
		Input string `json:"input,omitempty"`
	} `json:"content"`
	Model string `json:"model"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	StopReason string `json:"stop_reason,omitempty"`
}

type anthropicTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

type anthropicSSEEvent struct {
	Type  string `json:"type"`
	Delta *struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"delta,omitempty"`
	Usage *struct {
		OutputTokens int `json:"output_tokens"`
	} `json:"usage,omitempty"`
}

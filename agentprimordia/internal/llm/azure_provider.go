package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	// Azure 默认配置
	azureDefaultBaseURL    = "https://resource.openai.azure.com"
	azureDefaultAPIVersion = "2024-02-15-preview"
	azureDefaultTimeout    = 120 * time.Second
)

var (
	ErrAzureDeploymentRequired = errors.New("azure deployment name is required")
	ErrAzureResourceRequired   = errors.New("azure resource name or base URL is required")
)

// AzureConfig 是 Azure OpenAI 的专有配置
type AzureConfig struct {
	// ResourceName Azure OpenAI 资源名称 (如 "my-resource")
	// 将用于构建 URL: https://{resource}.openai.azure.com
	ResourceName string `json:"resource_name"`

	// DeploymentName Azure 部署名称 (替代 OpenAI 的 model 参数)
	DeploymentName string `json:"deployment_name"`

	// APIVersion Azure API 版本 (默认 "2024-02-15-preview")
	APIVersion string `json:"api_version"`

	// EmbeddingDeploymentName Embedding 模型的部署名称
	// 如果为空，则使用 DeploymentName
	EmbeddingDeploymentName string `json:"embedding_deployment_name"`

	// APIKey Azure OpenAI API Key
	APIKey string `json:"api_key"`

	// BaseURL 自定义 Base URL（优先级高于 ResourceName）
	BaseURL string `json:"base_url,omitempty"`

	// Temperature 生成温度
	Temperature float64 `json:"temperature,omitempty"`

	// MaxTokens 最大生成 token 数
	MaxTokens int `json:"max_tokens,omitempty"`
}

// AzureOpenAIProvider 实现了 Azure OpenAI 的 Provider 接口
type AzureOpenAIProvider struct {
	config AzureConfig
	client *http.Client
}

// NewAzureOpenAIProvider 创建 Azure OpenAI Provider
func NewAzureOpenAIProvider(cfg AzureConfig) (*AzureOpenAIProvider, error) {
	if cfg.APIKey == "" {
		return nil, ErrAPIKeyRequired
	}

	if cfg.DeploymentName == "" {
		return nil, ErrAzureDeploymentRequired
	}

	// 构建 Base URL
	baseURL := cfg.BaseURL
	if baseURL == "" {
		if cfg.ResourceName == "" {
			return nil, ErrAzureResourceRequired
		}
		baseURL = fmt.Sprintf("https://%s.openai.azure.com", cfg.ResourceName)
	}
	cfg.BaseURL = strings.TrimRight(baseURL, "/")

	if cfg.APIVersion == "" {
		cfg.APIVersion = azureDefaultAPIVersion
	}

	return &AzureOpenAIProvider{
		config: cfg,
		client: &http.Client{Timeout: azureDefaultTimeout},
	}, nil
}

// Complete 执行补全请求
func (p *AzureOpenAIProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	body := map[string]any{
		"messages": p.buildMessages(req.Messages),
	}
	if temp := ResolveTemperature(req.Temperature, p.config.Temperature); temp != nil {
		body["temperature"] = *temp
	}
	if req.MaxTokens > 0 {
		body["max_tokens"] = req.MaxTokens
	} else if p.config.MaxTokens > 0 {
		body["max_tokens"] = p.config.MaxTokens
	}

	path := fmt.Sprintf("/openai/deployments/%s/chat/completions", p.config.DeploymentName)
	raw, err := p.doRequest(ctx, path, body)
	if err != nil {
		return nil, err
	}

	var resp openaiChatResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrResponseParseFailed, err)
	}
	if resp.Error != nil {
		return nil, resp.Error
	}
	if len(resp.Choices) == 0 {
		return nil, ErrEmptyResponse
	}

	choice := resp.Choices[0]
	return &CompletionResponse{
		ID:      resp.ID,
		Model:   p.config.DeploymentName,
		Content: choice.Message.Content,
		Role:    choice.Message.Role,
		Usage: Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
	}, nil
}

// Stream 执行流式补全请求
func (p *AzureOpenAIProvider) Stream(ctx context.Context, req *CompletionRequest) (<-chan Chunk, error) {
	body := map[string]any{
		"messages": p.buildMessages(req.Messages),
		"stream":   true,
	}
	if temp := ResolveTemperature(req.Temperature, p.config.Temperature); temp != nil {
		body["temperature"] = *temp
	}
	if req.MaxTokens > 0 {
		body["max_tokens"] = req.MaxTokens
	} else if p.config.MaxTokens > 0 {
		body["max_tokens"] = p.config.MaxTokens
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	path := fmt.Sprintf("/openai/deployments/%s/chat/completions", p.config.DeploymentName)
	url := p.buildURL(path)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
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
		limitedReader := io.LimitReader(resp.Body, maxResponseSize)
		respBody, _ := io.ReadAll(limitedReader)
		var apiErr APIError
		if json.Unmarshal(respBody, &apiErr) == nil && apiErr.Message != "" {
			return nil, &apiErr
		}
		return nil, fmt.Errorf("Azure API returned HTTP %d: %s", resp.StatusCode, respBody)
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
			if data == "[DONE]" {
				select {
				case ch <- Chunk{Done: true}:
				case <-ctx.Done():
				}
				return
			}

			var sseResp openaiChatResponse
			if err := json.Unmarshal([]byte(data), &sseResp); err != nil {
				continue
			}
			if sseResp.Error != nil {
				continue
			}
			if len(sseResp.Choices) == 0 {
				continue
			}

			chunk := Chunk{
				Content: sseResp.Choices[0].Delta.Content,
			}
			if sseResp.Choices[0].FinishReason == "stop" {
				chunk.Done = true
				chunk.Usage = &Usage{
					PromptTokens:     sseResp.Usage.PromptTokens,
					CompletionTokens: sseResp.Usage.CompletionTokens,
					TotalTokens:      sseResp.Usage.TotalTokens,
				}
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
			slog.Warn("Azure 流式读取错误", "error", err)
		}
		select {
		case ch <- Chunk{Done: true}:
		case <-ctx.Done():
		}
	}()

	return ch, nil
}

// CallTools 执行工具调用请求
func (p *AzureOpenAIProvider) CallTools(ctx context.Context, req *ToolCallRequest) (*ToolCallResponse, error) {
	tools := make([]map[string]any, len(req.Tools))
	for i, t := range req.Tools {
		tools[i] = map[string]any{
			"type": t.Type,
			"function": map[string]any{
				"name":        t.Function.Name,
				"description": t.Function.Description,
				"parameters":  t.Function.Parameters,
			},
		}
	}

	body := map[string]any{
		"messages": p.buildMessages(req.Messages),
		"tools":    tools,
	}
	if p.config.Temperature > 0 {
		body["temperature"] = p.config.Temperature
	}
	if p.config.MaxTokens > 0 {
		body["max_tokens"] = p.config.MaxTokens
	}

	path := fmt.Sprintf("/openai/deployments/%s/chat/completions", p.config.DeploymentName)
	raw, err := p.doRequest(ctx, path, body)
	if err != nil {
		return nil, err
	}

	var resp openaiChatResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrResponseParseFailed, err)
	}
	if resp.Error != nil {
		return nil, resp.Error
	}
	if len(resp.Choices) == 0 {
		return nil, ErrEmptyResponse
	}

	choice := resp.Choices[0]
	result := &ToolCallResponse{
		Content: choice.Message.Content,
		Usage: Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
	}

	if len(choice.Message.ToolCalls) > 0 {
		result.ToolCalls = make([]FunctionCall, len(choice.Message.ToolCalls))
		for i, tc := range choice.Message.ToolCalls {
			result.ToolCalls[i] = FunctionCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			}
		}
	}

	return result, nil
}

// Embeddings 执行向量嵌入请求
func (p *AzureOpenAIProvider) Embeddings(ctx context.Context, texts []string) ([][]float32, error) {
	deployment := p.config.EmbeddingDeploymentName
	if deployment == "" {
		deployment = p.config.DeploymentName
	}

	body := map[string]any{
		"input": texts,
	}

	path := fmt.Sprintf("/openai/deployments/%s/embeddings", deployment)
	raw, err := p.doRequest(ctx, path, body)
	if err != nil {
		return nil, err
	}

	var resp openaiEmbedResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrResponseParseFailed, err)
	}

	result := make([][]float32, len(resp.Data))
	for i, d := range resp.Data {
		result[i] = d.Embedding
	}
	return result, nil
}

// Info 返回模型信息
func (p *AzureOpenAIProvider) Info() ModelInfo {
	maxCtx := 128000
	// 根据部署名称推断上下文窗口大小
	deployment := strings.ToLower(p.config.DeploymentName)
	if strings.Contains(deployment, "gpt-4o") {
		maxCtx = 128000
	} else if strings.Contains(deployment, "gpt-4-32k") {
		maxCtx = 32768
	} else if strings.Contains(deployment, "gpt-4") {
		maxCtx = 8192
	} else if strings.Contains(deployment, "gpt-35-turbo-16k") {
		maxCtx = 16384
	} else if strings.Contains(deployment, "gpt-35") || strings.Contains(deployment, "gpt-3.5") {
		maxCtx = 4096
	}

	return ModelInfo{
		Name:              p.config.DeploymentName,
		Provider:          "azure-openai",
		MaxContext:        maxCtx,
		SupportsTools:     true,
		SupportsStreaming: true,
	}
}

// buildURL 构建完整的 Azure OpenAI 请求 URL
func (p *AzureOpenAIProvider) buildURL(path string) string {
	return fmt.Sprintf("%s%s?api-version=%s", p.config.BaseURL, path, p.config.APIVersion)
}

// setHeaders 设置 Azure OpenAI 请求头
func (p *AzureOpenAIProvider) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api-key", p.config.APIKey) // Azure 使用 api-key 而非 Bearer
	req.Header.Set("User-Agent", userAgent)
}

// doRequest 执行 HTTP 请求
func (p *AzureOpenAIProvider) doRequest(ctx context.Context, path string, body any) (json.RawMessage, error) {
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := p.buildURL(path)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
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
		return nil, fmt.Errorf("Azure API returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// buildMessages 构建消息列表
func (p *AzureOpenAIProvider) buildMessages(msgs []ChatMessage) []map[string]any {
	return BuildOpenAIMessages(msgs)
}

// AzureConfigFromEnv 从环境变量加载 Azure OpenAI 配置
func AzureConfigFromEnv(prefix string) AzureConfig {
	if prefix == "" {
		prefix = "AP_AZURE"
	}

	cfg := AzureConfig{
		ResourceName:            os.Getenv(prefix + "_RESOURCE_NAME"),
		DeploymentName:          os.Getenv(prefix + "_DEPLOYMENT_NAME"),
		APIVersion:              os.Getenv(prefix + "_API_VERSION"),
		EmbeddingDeploymentName: os.Getenv(prefix + "_EMBEDDING_DEPLOYMENT_NAME"),
		APIKey:                  os.Getenv(prefix + "_API_KEY"),
		BaseURL:                 os.Getenv(prefix + "_BASE_URL"),
		Temperature:             envFloat(prefix+"_TEMPERATURE", 0),
		MaxTokens:               envInt(prefix+"_MAX_TOKENS", 0),
	}

	if cfg.BaseURL == "" && cfg.ResourceName == "" {
		slog.Warn("Azure 配置缺少 Endpoint：BASE_URL 和 RESOURCE_NAME 均为空")
	}

	return cfg
}

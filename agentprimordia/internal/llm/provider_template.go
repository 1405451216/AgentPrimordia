package llm

// ProviderTemplate 是新 Provider 的模板代码
// 复制此文件并重命名为 {provider}_provider.go，然后替换所有 TODO
//
// 使用步骤：
//  1. cp provider_template.go {provider}_provider.go
//  2. 全局替换 "template" / "Template" 为你的 provider 名称
//  3. 实现 Complete() 方法
//  4. 实现 Stream() 方法
//  5. 实现 CallTools() 方法
//  6. 实现 Info() 方法
//  7. 删除此注释块
//  8. 运行测试：go test -run TestTemplate ./internal/llm/

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	// templateDefaultBaseURL 模板 Provider 默认 API 地址
	// TODO: 替换为实际的 API 地址
	templateDefaultBaseURL = "https://api.example.com/v1"
	// defaultTemplateMaxContext 默认最大上下文窗口
	// TODO: 替换为实际的上下文窗口大小
	defaultTemplateMaxContext = 8192
	// defaultTemplateMaxTokens 默认最大输出 Token 数
	// TODO: 替换为实际的默认值
	defaultTemplateMaxTokens = 4096
)

// TemplateProvider 模板 Provider 实现
// TODO: 重命名为 XxxProvider（如 DeepSeekProvider）
type TemplateProvider struct {
	config Config
	client *http.Client // 使用包内已定义的 http.Client（来自 net/http）
}

// NewTemplateProvider 创建模板 Provider
// TODO: 重命名为 NewXxxProvider
func NewTemplateProvider(cfg Config) (*TemplateProvider, error) {
	// 校验 API Key
	if cfg.APIKey == "" {
		return nil, ErrAPIKeyRequired
	}

	// 设置默认 BaseURL
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = templateDefaultBaseURL
	}
	cfg.BaseURL = strings.TrimRight(baseURL, "/")

	// 设置默认模型
	if cfg.Model == "" {
		cfg.Model = "template-default-model" // TODO: 替换为实际的默认模型名称
	}

	return &TemplateProvider{
		config: cfg,
		client: &http.Client{Timeout: defaultTimeout},
	}, nil
}

// Complete 执行非流式补全
func (p *TemplateProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	// TODO: 实现 API 调用
	// 1. 使用 ResolveModel 解析模型名称
	// 2. 使用 ResolveTemperature 解析温度参数
	// 3. 构建请求体（参考 openai_provider.go 的 Complete 方法）
	// 4. 发送 HTTP 请求（使用 p.doRequest 辅助方法）
	// 5. 解析响应
	// 6. 返回 CompletionResponse
	return nil, fmt.Errorf("TemplateProvider.Complete: TODO: 未实现")
}

// Stream 执行流式补全
func (p *TemplateProvider) Stream(ctx context.Context, req *CompletionRequest) (<-chan Chunk, error) {
	// TODO: 实现流式 API 调用
	// 1. 使用 ResolveModel 解析模型名称
	// 2. 构建请求体（stream: true）
	// 3. 发送 HTTP 请求
	// 4. 检查响应状态码
	// 5. 创建 buffered channel: ch := make(chan Chunk, 32)
	// 6. 在 goroutine 中：
	//    a. 使用 bufio.Scanner 读取 SSE 流
	//    b. 解析每个 data: 行
	//    c. 发送 Chunk 到 channel
	//    d. 监听 ctx.Done() 支持取消
	//    e. 流结束时发送 Chunk{Done: true}
	//    f. close(ch) 和 resp.Body.Close()
	// 参考 openai_provider.go 的 Stream 方法
	return nil, fmt.Errorf("TemplateProvider.Stream: TODO: 未实现")
}

// CallTools 执行工具调用
func (p *TemplateProvider) CallTools(ctx context.Context, req *ToolCallRequest) (*ToolCallResponse, error) {
	// TODO: 实现工具调用
	// 大多数 Provider 的工具调用与 Complete 共享同一 API
	// 区别在于请求中包含 tools 定义，响应中包含 tool_calls
	//
	// 如果 Provider 不支持工具调用，返回 ErrNotSupported：
	//   return nil, ErrNotSupported
	//
	// OpenAI 兼容 API 可复用 BuildOpenAIMessages 构建消息
	// 参考 openai_provider.go 的 CallTools 方法
	return nil, fmt.Errorf("TemplateProvider.CallTools: TODO: 未实现")
}

// Info 返回模型信息
func (p *TemplateProvider) Info() ModelInfo {
	// TODO: 返回正确的模型信息
	return ModelInfo{
		Name:              p.config.Model,
		Provider:          "template", // TODO: 替换为 provider 标识（小写英文）
		MaxContext:        defaultTemplateMaxContext,
		SupportsTools:     true,  // TODO: 根据实际情况设置
		SupportsStreaming: true,  // TODO: 根据实际情况设置
	}
}

// resolveModel 解析模型名称
func (p *TemplateProvider) resolveModel(model string) string {
	return ResolveModel(model, p.config.Model)
}

// doRequest 发送 HTTP 请求
// TODO: 根据目标 API 的认证方式调整请求头
func (p *TemplateProvider) doRequest(ctx context.Context, endpoint string, body any) ([]byte, error) {
	url := fmt.Sprintf("%s%s", p.config.BaseURL, endpoint)

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// TODO: 根据目标 API 调整认证方式
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.config.APIKey))
	req.Header.Set("User-Agent", userAgent)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		limitedReader := io.LimitReader(resp.Body, maxResponseSize)
		respBody, _ := io.ReadAll(limitedReader)
		// TODO: 根据目标 API 的错误格式调整解析逻辑
		var apiErr struct {
			Error *struct {
				Message string `json:"message"`
				Type    string `json:"type"`
				Code    string `json:"code"`
			} `json:"error"`
		}
		if json.Unmarshal(respBody, &apiErr) == nil && apiErr.Error != nil {
			return nil, fmt.Errorf("API returned HTTP %d: [%s] %s",
				resp.StatusCode, apiErr.Error.Code, apiErr.Error.Message)
		}
		return nil, fmt.Errorf("API returned HTTP %d: %s", resp.StatusCode, respBody)
	}

	limitedReader := io.LimitReader(resp.Body, maxResponseSize)
	responseData, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return responseData, nil
}

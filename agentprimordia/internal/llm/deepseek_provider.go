package llm

import (
	"context"
	"fmt"
	"strings"
)

const (
	deepseekDefaultBaseURL    = "https://api.deepseek.com/v1"
	defaultDeepSeekMaxContext = 64000
	defaultDeepSeekMaxTokens  = 4096
)

// DeepSeekProvider DeepSeek 大模型 Provider
//
// DeepSeek 提供 OpenAI 兼容的 API 接口，支持 deepseek-chat（通用对话）
// 和 deepseek-reasoner（推理模型）。本 Provider 在 OpenAIProvider 基础上
// 封装了 DeepSeek 特有的默认配置，用户无需手动设置 BaseURL。
//
// 使用示例：
//
//	provider, err := llm.NewDeepSeekProvider(llm.Config{
//	    APIKey: "sk-xxx",
//	    Model:  "deepseek-chat", // 或 "deepseek-reasoner"
//	})
type DeepSeekProvider struct {
	*OpenAIProvider
	config Config
}

// NewDeepSeekProvider 创建 DeepSeek Provider
//
// 如果未指定 BaseURL，自动使用 "https://api.deepseek.com/v1"。
// 如果未指定 Model，默认使用 "deepseek-chat"。
func NewDeepSeekProvider(cfg Config) (*DeepSeekProvider, error) {
	if cfg.APIKey == "" {
		return nil, ErrAPIKeyRequired
	}

	if cfg.BaseURL == "" {
		cfg.BaseURL = deepseekDefaultBaseURL
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")

	if cfg.Model == "" {
		cfg.Model = "deepseek-chat"
	}

	inner, err := NewOpenAIProvider(cfg)
	if err != nil {
		return nil, fmt.Errorf("deepseek provider init failed: %w", err)
	}

	return &DeepSeekProvider{
		OpenAIProvider: inner,
		config:         cfg,
	}, nil
}

// Info 返回 DeepSeek 模型信息
func (p *DeepSeekProvider) Info() ModelInfo {
	return ModelInfo{
		Name:              p.config.Model,
		Provider:          "deepseek",
		MaxContext:        defaultDeepSeekMaxContext,
		SupportsTools:     true,
		SupportsStreaming: true,
	}
}

// Complete 重写 Complete 以确保使用 DeepSeek 默认配置
func (p *DeepSeekProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	if req.Model == "" {
		req.Model = p.config.Model
	}
	return p.OpenAIProvider.Complete(ctx, req)
}

// Stream 重写 Stream 以确保使用 DeepSeek 默认配置
func (p *DeepSeekProvider) Stream(ctx context.Context, req *CompletionRequest) (<-chan Chunk, error) {
	if req.Model == "" {
		req.Model = p.config.Model
	}
	return p.OpenAIProvider.Stream(ctx, req)
}

// CallTools 重写 CallTools 以确保使用 DeepSeek 默认配置
func (p *DeepSeekProvider) CallTools(ctx context.Context, req *ToolCallRequest) (*ToolCallResponse, error) {
	if req.Model == "" {
		req.Model = p.config.Model
	}
	return p.OpenAIProvider.CallTools(ctx, req)
}

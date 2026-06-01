package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var ErrUnsupportedMultimodalProvider = errors.New("unsupported multimodal provider type")

// multimodalCompleter 多模态补全内部接口
type multimodalCompleter interface {
	CompleteMultimodal(ctx context.Context, req *CompletionRequestExt) (*CompletionResponse, error)
}

// multimodalStreamer 多模态流式内部接口
type multimodalStreamer interface {
	StreamMultimodal(ctx context.Context, req *CompletionRequestExt) (<-chan Chunk, error)
}

// multimodalInfoExtProvider 扩展模型信息内部接口
type multimodalInfoExtProvider interface {
	InfoExt() ModelInfoExt
}

// MultimodalCapability 多模态能力标记
type MultimodalCapability int

const (
	CapText   MultimodalCapability = 1 << iota // 文本
	CapVision                                  // 视觉（图片理解）
	CapAudio                                   // 音频理解
	CapVideo                                   // 视频理解
)

// HasCapability 检查是否具备指定能力
func (c MultimodalCapability) HasCapability(cap MultimodalCapability) bool {
	return c&cap != 0
}

// capabilitiesFromInfoExt 从 ModelInfoExt 推导多模态能力标记
func capabilitiesFromInfoExt(info ModelInfoExt) MultimodalCapability {
	caps := CapText
	if info.SupportsVision {
		caps |= CapVision
	}
	if info.SupportsAudio {
		caps |= CapAudio
	}
	if info.SupportsVideo {
		caps |= CapVideo
	}
	return caps
}

// MultimodalProvider 多模态 LLM 提供者接口
//
// 扩展标准 Provider 接口，支持图片/音频/视频等多模态输入。
// 使用方只需依赖此接口即可统一调用不同提供商的多模态能力。
type MultimodalProvider interface {
	Provider

	CompleteMultimodal(ctx context.Context, req *CompletionRequestExt) (*CompletionResponse, error)
	StreamMultimodal(ctx context.Context, req *CompletionRequestExt) (<-chan Chunk, error)
	Capabilities() MultimodalCapability
	ModelInfoExt() ModelInfoExt
}

// MultimodalAdapter 将现有独立多模态 Provider 适配为统一 MultimodalProvider 接口的适配器
type MultimodalAdapter struct {
	provider any
	caps     MultimodalCapability
	infoExt  func() ModelInfoExt
}

// NewMultimodalAdapter 创建多模态适配器
//
// 支持任何实现了 multimodalCompleter 和 Provider 接口的底层 Provider。
// 如果底层 Provider 实现了 multimodalInfoExtProvider，则自动推导能力标记。
func NewMultimodalAdapter(provider any) (*MultimodalAdapter, error) {
	if provider == nil {
		return nil, ErrUnsupportedMultimodalProvider
	}
	if _, ok := provider.(multimodalCompleter); !ok {
		return nil, ErrUnsupportedMultimodalProvider
	}
	if _, ok := provider.(Provider); !ok {
		return nil, ErrUnsupportedMultimodalProvider
	}

	var caps MultimodalCapability
	var infoExtFn func() ModelInfoExt

	if ep, ok := provider.(multimodalInfoExtProvider); ok {
		infoExtFn = ep.InfoExt
		caps = capabilitiesFromInfoExt(ep.InfoExt())
	} else {
		caps = CapText
	}

	return &MultimodalAdapter{
		provider: provider,
		caps:     caps,
		infoExt:  infoExtFn,
	}, nil
}

// Complete 标准文本补全（委托给底层 Provider）
func (a *MultimodalAdapter) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	if p, ok := a.provider.(Provider); ok {
		return p.Complete(ctx, req)
	}
	return nil, ErrUnsupportedMultimodalProvider
}

// Stream 标准流式补全（委托给底层 Provider）
func (a *MultimodalAdapter) Stream(ctx context.Context, req *CompletionRequest) (<-chan Chunk, error) {
	if p, ok := a.provider.(Provider); ok {
		return p.Stream(ctx, req)
	}
	return nil, ErrUnsupportedMultimodalProvider
}

// CallTools 工具调用（委托给底层 Provider）
func (a *MultimodalAdapter) CallTools(ctx context.Context, req *ToolCallRequest) (*ToolCallResponse, error) {
	if p, ok := a.provider.(Provider); ok {
		return p.CallTools(ctx, req)
	}
	return nil, ErrUnsupportedMultimodalProvider
}

// Embeddings 文本嵌入（通过类型断言委托给底层 Provider）
func (a *MultimodalAdapter) Embeddings(ctx context.Context, texts []string) ([][]float32, error) {
	if embedder, ok := a.provider.(Embedder); ok {
		return embedder.Embeddings(ctx, texts)
	}
	return nil, ErrNotSupported
}

// Info 返回基础模型信息
func (a *MultimodalAdapter) Info() ModelInfo {
	if a.infoExt != nil {
		ext := a.infoExt()
		return ext.ModelInfo
	}
	if p, ok := a.provider.(Provider); ok {
		return p.Info()
	}
	return ModelInfo{}
}

// CompleteMultimodal 多模态补全
func (a *MultimodalAdapter) CompleteMultimodal(ctx context.Context, req *CompletionRequestExt) (*CompletionResponse, error) {
	if c, ok := a.provider.(multimodalCompleter); ok {
		return c.CompleteMultimodal(ctx, req)
	}
	return nil, ErrUnsupportedMultimodalProvider
}

// StreamMultimodal 多模态流式输出
func (a *MultimodalAdapter) StreamMultimodal(ctx context.Context, req *CompletionRequestExt) (<-chan Chunk, error) {
	if s, ok := a.provider.(multimodalStreamer); ok {
		return s.StreamMultimodal(ctx, req)
	}
	return nil, ErrUnsupportedMultimodalProvider
}

// Capabilities 返回多模态能力标记
func (a *MultimodalAdapter) Capabilities() MultimodalCapability {
	return a.caps
}

// ModelInfoExt 返回扩展模型信息
func (a *MultimodalAdapter) ModelInfoExt() ModelInfoExt {
	if a.infoExt != nil {
		return a.infoExt()
	}
	return ModelInfoExt{}
}

// As 尝试将适配器转换为具体的底层 Provider 类型
func (a *MultimodalAdapter) As(target any) bool {
	switch p := a.provider.(type) {
	case *OpenAIMultimodalProvider:
		if ptr, ok := target.(**OpenAIMultimodalProvider); ok {
			*ptr = p
			return true
		}
	case *AnthropicVisionProvider:
		if ptr, ok := target.(**AnthropicVisionProvider); ok {
			*ptr = p
			return true
		}
	case *GeminiMultimodalProvider:
		if ptr, ok := target.(**GeminiMultimodalProvider); ok {
			*ptr = p
			return true
		}
	}
	return false
}

// SupportsVision 是否支持视觉能力
func (a *MultimodalAdapter) SupportsVision() bool {
	return a.caps.HasCapability(CapVision)
}

// SupportsAudio 是否支持音频能力
func (a *MultimodalAdapter) SupportsAudio() bool {
	return a.caps.HasCapability(CapAudio)
}

// AutoFallback 自动降级策略：
// 如果请求包含多模态内容但 Provider 不支持，自动降级为纯文本请求
func (a *MultimodalAdapter) AutoFallback(ctx context.Context, req *CompletionRequestExt) (*CompletionResponse, error) {
	if !req.HasMultimodalContent() {
		return a.Complete(ctx, req.ToCompletionRequest())
	}
	return a.CompleteMultimodal(ctx, req)
}

// NewMultimodalProvider 根据配置自动创建多模态 Provider
//
// 根据 Config.BaseURL 或 Config.Model 自动识别提供商类型，
// 返回统一的 MultimodalProvider 接口。使用方无需关心底层差异。
//
// 识别规则：
//   - BaseURL 包含 "openai" 或 Model 以 "gpt-4o"/"gpt-4-turbo" 开头 → OpenAI
//   - BaseURL 包含 "anthropic" 或 Model 以 "claude" 开头 → Anthropic
//   - BaseURL 包含 "gemini" 或 Model 以 "gemini" 开头 → Gemini
//   - 其他 → 尝试 OpenAI 兼容格式
func NewMultimodalProvider(cfg Config) (MultimodalProvider, error) {
	providerType := detectMultimodalProviderType(cfg)

	var raw any
	var err error

	switch providerType {
	case "openai":
		raw, err = NewOpenAIMultimodalProvider(cfg)
	case "anthropic":
		raw, err = NewAnthropicVisionProvider(cfg)
	case "gemini":
		raw, err = NewGeminiMultimodalProvider(cfg)
	default:
		raw, err = NewOpenAIMultimodalProvider(cfg)
	}

	if err != nil {
		return nil, fmt.Errorf("创建多模态 Provider 失败: %w", err)
	}

	return NewMultimodalAdapter(raw)
}

func detectMultimodalProviderType(cfg Config) string {
	baseURL := cfg.BaseURL
	model := cfg.Model

	lowerBaseURL := strings.ToLower(baseURL)
	lowerModel := strings.ToLower(model)

	if strings.Contains(lowerBaseURL, "anthropic") || strings.HasPrefix(lowerModel, "claude") {
		return "anthropic"
	}
	if strings.Contains(lowerBaseURL, "gemini") || strings.HasPrefix(lowerModel, "gemini") ||
		strings.Contains(lowerBaseURL, "generativelanguage") {
		return "gemini"
	}
	if strings.Contains(lowerBaseURL, "openai") || strings.HasPrefix(lowerModel, "gpt-4o") ||
		strings.HasPrefix(lowerModel, "gpt-4-turbo") {
		return "openai"
	}

	return "openai"
}

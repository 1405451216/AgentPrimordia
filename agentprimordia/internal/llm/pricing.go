package llm

import "sync"

const pricePerMillion = 1_000_000

var (
	defaultPricingTable     map[string]ModelPricing
	defaultPricingTableOnce sync.Once
)

// ModelPricing 模型定价
type ModelPricing struct {
	Model                string  `json:"model"`
	Provider             string  `json:"provider"`
	PromptPricePer1M     float64 `json:"prompt_price_per_1m"`
	CompletionPricePer1M float64 `json:"completion_price_per_1m"`
}

// DefaultPricingTable 默认定价表（主流模型 2025 年公开价格）
func DefaultPricingTable() map[string]ModelPricing {
	defaultPricingTableOnce.Do(func() {
		defaultPricingTable = map[string]ModelPricing{
		"gpt-4o": {
			Model:                "gpt-4o",
			Provider:             "openai",
			PromptPricePer1M:     2.5,
			CompletionPricePer1M: 10.0,
		},
		"gpt-4o-mini": {
			Model:                "gpt-4o-mini",
			Provider:             "openai",
			PromptPricePer1M:     0.15,
			CompletionPricePer1M: 0.6,
		},
		"gpt-4-turbo": {
			Model:                "gpt-4-turbo",
			Provider:             "openai",
			PromptPricePer1M:     10.0,
			CompletionPricePer1M: 30.0,
		},
		"gpt-3.5-turbo": {
			Model:                "gpt-3.5-turbo",
			Provider:             "openai",
			PromptPricePer1M:     0.5,
			CompletionPricePer1M: 1.5,
		},
		"claude-3-5-sonnet-20241022": {
			Model:                "claude-3-5-sonnet-20241022",
			Provider:             "anthropic",
			PromptPricePer1M:     3.0,
			CompletionPricePer1M: 15.0,
		},
		"claude-3-haiku-20240307": {
			Model:                "claude-3-haiku-20240307",
			Provider:             "anthropic",
			PromptPricePer1M:     0.25,
			CompletionPricePer1M: 1.25,
		},
		"claude-3-opus-20240229": {
			Model:                "claude-3-opus-20240229",
			Provider:             "anthropic",
			PromptPricePer1M:     15.0,
			CompletionPricePer1M: 75.0,
		},
		"gemini-2.0-flash": {
			Model:                "gemini-2.0-flash",
			Provider:             "google",
			PromptPricePer1M:     0.1,
			CompletionPricePer1M: 0.4,
		},
		"gemini-1.5-pro": {
			Model:                "gemini-1.5-pro",
			Provider:             "google",
			PromptPricePer1M:     1.25,
			CompletionPricePer1M: 5.0,
		},
		"deepseek-chat": {
			Model:                "deepseek-chat",
			Provider:             "deepseek",
			PromptPricePer1M:     0.14,
			CompletionPricePer1M: 0.28,
		},
		"qwen-turbo": {
			Model:                "qwen-turbo",
			Provider:             "alibaba",
			PromptPricePer1M:     0.3,
			CompletionPricePer1M: 0.6,
		},
	}
	})
	return defaultPricingTable
}

// EstimateCost 估算单次调用成本（USD）
func EstimateCost(model string, usage Usage, table map[string]ModelPricing) float64 {
	if table == nil {
		return 0
	}
	p, ok := table[model]
	if !ok {
		return 0
	}
	promptCost := p.PromptPricePer1M * float64(usage.PromptTokens) / pricePerMillion
	completionCost := p.CompletionPricePer1M * float64(usage.CompletionTokens) / pricePerMillion
	return promptCost + completionCost
}

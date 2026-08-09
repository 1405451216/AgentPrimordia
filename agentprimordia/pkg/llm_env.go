// llm_env.go — 环境变量驱动的真实 LLM Provider 装配（v4.1 真实接线）
//
// ProviderFromEnv 读取 AP_LLM_* 环境变量并装配真实 Provider，
// 供示例应用与 llm_bench 在 CI（默认 mock）与真实评测（设置环境变量）间切换。
package ap

import (
	"fmt"
	"os"
	"strings"
)

// ProviderFromEnv 从环境变量装配真实 LLM Provider。
//
// 环境变量（与 llm.ConfigFromEnv 同前缀）：
//   - AP_LLM_PROVIDER: openai（默认）| anthropic | gemini | ollama | qwen | glm
//   - AP_LLM_MODEL: 模型名（必填，未设置返回错误）
//   - AP_LLM_API_KEY: API Key（本地服务如 Ollama 可留空）
//   - AP_LLM_BASE_URL: 自定义端点（OpenAI 兼容服务/本地网关）
//
// 未设置 AP_LLM_MODEL 时返回错误（区分「未配置」与「配置错误」）。
func ProviderFromEnv() (Provider, error) {
	providerName := strings.ToLower(os.Getenv("AP_LLM_PROVIDER"))
	if providerName == "" {
		providerName = "openai"
	}
	cfg := ConfigFromEnv("AP_LLM")
	if cfg.Model == "" {
		return nil, fmt.Errorf("AP_LLM_MODEL 未设置：真实 LLM 需要模型名（示例：AP_LLM_MODEL=gpt-4o-mini）；不设置环境变量时示例保持 mock 模式")
	}

	var (
		p   Provider
		err error
	)
	switch providerName {
	case "openai":
		p, err = NewOpenAIProvider(cfg)
	case "anthropic":
		p, err = NewAnthropicProvider(cfg)
	case "gemini":
		p, err = NewGeminiProvider(cfg)
	case "ollama":
		p, err = NewOllamaProvider(cfg)
	case "qwen":
		p, err = NewQwenProvider(cfg)
	case "glm":
		p, err = NewGLMProvider(cfg)
	default:
		return nil, fmt.Errorf("未知 AP_LLM_PROVIDER %q（支持 openai|anthropic|gemini|ollama|qwen|glm）", providerName)
	}
	if err != nil {
		return nil, fmt.Errorf("装配 %s Provider 失败: %w", providerName, err)
	}
	return p, nil
}

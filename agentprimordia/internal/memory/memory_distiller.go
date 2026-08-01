package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// LLMClient 蒸馏所需的 LLM 能力（最小接口）。
// 采用与 memory 包解耦的字符串签名，便于用 mock 测试，
// 也避免 internal/memory → internal/llm 的跨模块依赖。
type LLMClient interface {
	Complete(ctx context.Context, prompt string) (string, error)
}

// distillOutput LLM 返回的期望 JSON 结构。
type distillOutput struct {
	Patterns []struct {
		Pattern     string  `json:"pattern"`
		Description string  `json:"description"`
		SuccessRate float64 `json:"success_rate"`
	} `json:"patterns"`
	Facts []struct {
		Key        string  `json:"key"`
		Value      string  `json:"value"`
		Confidence float64 `json:"confidence"`
	} `json:"facts"`
}

// MemoryDistiller 从 Episodic（MemoryStore）蒸馏到 Semantic。
type MemoryDistiller struct {
	episodicStore Memory
	semantic      *SemanticMemory
	llm           LLMClient
}

// NewMemoryDistiller 创建蒸馏器。
func NewMemoryDistiller(store Memory, semantic *SemanticMemory, client LLMClient) *MemoryDistiller {
	return &MemoryDistiller{
		episodicStore: store,
		semantic:      semantic,
		llm:           client,
	}
}

// Distill 从历史对话提取结构化知识，写入 SemanticMemory。
// 行为：
//   - 空会话直接返回 nil（无操作）；
//   - LLM 调用失败返回 error；
//   - LLM 输出无法解析为 JSON 时优雅降级（不写入，不 panic）。
func (d *MemoryDistiller) Distill(ctx context.Context, sessionID string) error {
	episodes, err := d.episodicStore.List(ctx, &ListOptions{SessionID: sessionID})
	if err != nil {
		return fmt.Errorf("distill: list episodes: %w", err)
	}
	if len(episodes) == 0 {
		return nil
	}

	prompt := d.buildPrompt(episodes)
	resp, err := d.llm.Complete(ctx, prompt)
	if err != nil {
		return fmt.Errorf("distill: llm complete: %w", err)
	}

	patterns, facts := parseDistillResult(resp)
	for _, p := range patterns {
		d.semantic.AddPattern(ctx, p)
	}
	for _, f := range facts {
		d.semantic.AddFact(ctx, f)
	}
	return nil
}

// buildPrompt 把若干 episode 拼成蒸馏提示。
func (d *MemoryDistiller) buildPrompt(episodes []*Episode) string {
	var b strings.Builder
	b.WriteString("从以下对话历史中提取结构化知识，严格以 JSON 返回，格式：\n")
	b.WriteString(`{"patterns":[{"pattern":"tool名/场景","description":"何时使用","success_rate":0.0}],`)
	b.WriteString(`"facts":[{"key":"事实名","value":"事实内容","confidence":0.0}]}`)
	b.WriteString("\n\n对话历史：\n")
	for _, ep := range episodes {
		fmt.Fprintf(&b, "[%s] %s\n", ep.Role, ep.Content)
	}
	return b.String()
}

// parseDistillResult 从 LLM 输出中提取首个 JSON 对象并解析。
// 支持 LLM 用 ```json 代码块包裹的情况。解析失败返回空切片（降级）。
func parseDistillResult(raw string) ([]Pattern, []Fact) {
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return nil, nil
	}
	jsonStr := raw[start : end+1]

	var out distillOutput
	if err := json.Unmarshal([]byte(jsonStr), &out); err != nil {
		return nil, nil
	}

	patterns := make([]Pattern, 0, len(out.Patterns))
	for _, p := range out.Patterns {
		patterns = append(patterns, Pattern{
			Pattern:     p.Pattern,
			Description: p.Description,
			SuccessRate: p.SuccessRate,
		})
	}

	facts := make([]Fact, 0, len(out.Facts))
	for _, f := range out.Facts {
		source := "distilled"
		facts = append(facts, Fact{
			Key:        f.Key,
			Value:      f.Value,
			Confidence: f.Confidence,
			Source:     source,
		})
	}
	return patterns, facts
}

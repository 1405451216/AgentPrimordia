// llm_distiller.go — LLM 驱动的知识蒸馏器（V3.1 Phase 1 生产实现）
//
// 替代 v3.0 中 KnowledgeDistiller 的字符串匹配提取方式，
// 使用 LLM 从 Agent 交互中提取结构化知识。
//
// 核心能力：
//   - LLM 提取管道：用 LLM 从交互中提取结构化知识（事实/技能/偏好/模式）
//   - 知识压缩：将提取的知识压缩为 KnowledgeItem 并存入 SemanticMemory
//   - 能力评估：用 LLM 自动生成能力测试用例
package learning

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// LLMProvider LLM 提供者接口（解耦 internal/llm）
//
// 由调用方注入具体的 LLM Provider 实现。
type LLMProvider interface {
	// Complete 发送补全请求，返回 LLM 响应内容
	Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}

// LLMDistillerConfig LLM 蒸馏器配置
type LLMDistillerConfig struct {
	// Provider LLM 提供者
	Provider LLMProvider
	// MaxBatchSize 单次蒸馏最大交互数（默认 5）
	MaxBatchSize int
	// MinConfidence 最低置信度阈值（低于此值的知识不存储，默认 0.5）
	MinConfidence float64
	// EnableCapabilityEval 是否启用 LLM 能力评估（默认 true）
	EnableCapabilityEval bool
	// Logger 日志器
	Logger *slog.Logger
}

// LLMDistiller LLM 驱动的知识蒸馏器
//
// 使用 LLM 从 Agent 交互中提取结构化知识，替代简单的字符串匹配。
// 提取的知识包括：事实、技能、偏好、模式。
type LLMDistiller struct {
	provider      LLMProvider
	logger        *slog.Logger
	maxBatchSize  int
	minConfidence float64
	enableEval    bool

	mu    sync.RWMutex
	store map[string]*KnowledgeItem
	stats LLMDistillerStats
}

// LLMDistillerStats LLM 蒸馏统计
type LLMDistillerStats struct {
	TotalInteractions int64     `json:"total_interactions"`
	TotalDistilled    int64     `json:"total_distilled"`
	TotalLLMCalls     int64     `json:"total_llm_calls"`
	TotalErrors       int64     `json:"total_errors"`
	LastDistillTime   time.Time `json:"last_distill_time"`
}

// LLMExtractionResult LLM 提取结果（JSON 格式）
type LLMExtractionResult struct {
	Knowledge []LLMKnowledgeEntry `json:"knowledge"`
	Summary   string              `json:"summary"`
}

// LLMKnowledgeEntry LLM 提取的单条知识
type LLMKnowledgeEntry struct {
	Category   string  `json:"category"`   // "fact"/"skill"/"preference"/"pattern"
	Pattern    string  `json:"pattern"`    // 知识内容
	Context    string  `json:"context"`    // 适用上下文
	Confidence float64 `json:"confidence"` // 置信度 0-1
}

// LLMCapabilityTest LLM 生成的能力测试用例
type LLMCapabilityTest struct {
	CapabilityName string `json:"capability_name"`
	TestInput      string `json:"test_input"`
	ExpectedOutput string `json:"expected_output"`
	Difficulty     string `json:"difficulty"` // "easy"/"medium"/"hard"
}

// NewLLMDistiller 创建 LLM 驱动的知识蒸馏器
func NewLLMDistiller(cfg LLMDistillerConfig) *LLMDistiller {
	if cfg.MaxBatchSize <= 0 {
		cfg.MaxBatchSize = 5
	}
	if cfg.MinConfidence <= 0 {
		cfg.MinConfidence = 0.5
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	return &LLMDistiller{
		provider:      cfg.Provider,
		logger:        cfg.Logger,
		maxBatchSize:  cfg.MaxBatchSize,
		minConfidence: cfg.MinConfidence,
		enableEval:    cfg.EnableCapabilityEval,
		store:         make(map[string]*KnowledgeItem),
	}
}

// DistillWithLLM 使用 LLM 从交互中蒸馏知识
//
// 流程：
// 1. 将交互格式化为 LLM 提示
// 2. 调用 LLM 提取结构化知识
// 3. 解析 LLM 响应为 KnowledgeItem
// 4. 过滤低置信度知识
// 5. 存入知识库
func (d *LLMDistiller) DistillWithLLM(ctx context.Context, interactions []Interaction) ([]KnowledgeItem, error) {
	if len(interactions) == 0 {
		return nil, nil
	}

	// 限制批次大小
	if len(interactions) > d.maxBatchSize {
		interactions = interactions[:d.maxBatchSize]
	}

	d.mu.Lock()
	d.stats.TotalInteractions += int64(len(interactions))
	d.mu.Unlock()

	// 构造 LLM 提示
	systemPrompt := d.buildDistillSystemPrompt()
	userPrompt := d.buildDistillUserPrompt(interactions)

	// 调用 LLM
	d.mu.Lock()
	d.stats.TotalLLMCalls++
	d.mu.Unlock()

	response, err := d.provider.Complete(ctx, systemPrompt, userPrompt)
	if err != nil {
		d.mu.Lock()
		d.stats.TotalErrors++
		d.mu.Unlock()
		return nil, fmt.Errorf("llm_distiller: LLM call failed: %w", err)
	}

	// 解析 LLM 响应
	extraction, err := d.parseExtraction(response)
	if err != nil {
		d.mu.Lock()
		d.stats.TotalErrors++
		d.mu.Unlock()
		return nil, fmt.Errorf("llm_distiller: parse extraction: %w", err)
	}

	// 转换为 KnowledgeItem 并过滤
	var items []KnowledgeItem
	sourceID := ""
	if len(interactions) > 0 {
		sourceID = interactions[0].ID
	}

	for i, entry := range extraction.Knowledge {
		if entry.Confidence < d.minConfidence {
			continue
		}

		item := KnowledgeItem{
			ID:         fmt.Sprintf("llm_%s_%d", sourceID, i),
			Category:   entry.Category,
			Pattern:    entry.Pattern,
			Context:    entry.Context,
			Confidence: entry.Confidence,
			Source:     sourceID,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}
		items = append(items, item)
	}

	// 存入知识库
	d.mu.Lock()
	for _, item := range items {
		d.store[item.ID] = &item
	}
	d.stats.TotalDistilled += int64(len(items))
	d.stats.LastDistillTime = time.Now()
	d.mu.Unlock()

	d.logger.Info("LLM 知识蒸馏完成",
		"interactions", len(interactions),
		"extracted", len(extraction.Knowledge),
		"stored", len(items),
		"summary", extraction.Summary,
	)

	return items, nil
}

// GenerateCapabilityTests 使用 LLM 生成能力测试用例
//
// 根据指定的能力名称和描述，让 LLM 自动生成测试用例。
func (d *LLMDistiller) GenerateCapabilityTests(ctx context.Context, capabilityName, description string, count int) ([]LLMCapabilityTest, error) {
	if count <= 0 {
		count = 3
	}

	systemPrompt := `你是一个 Agent 能力评估专家。根据给定的能力描述，生成测试用例来评估 Agent 在该能力上的表现。
以 JSON 数组格式返回，每个元素包含：
- capability_name: 能力名称
- test_input: 测试输入（用户问题/指令）
- expected_output: 期望输出描述
- difficulty: 难度（easy/medium/hard）

仅返回 JSON 数组，不要其他内容。`

	userPrompt := fmt.Sprintf(`能力名称：%s
能力描述：%s
请生成 %d 个测试用例。`, capabilityName, description, count)

	d.mu.Lock()
	d.stats.TotalLLMCalls++
	d.mu.Unlock()

	response, err := d.provider.Complete(ctx, systemPrompt, userPrompt)
	if err != nil {
		d.mu.Lock()
		d.stats.TotalErrors++
		d.mu.Unlock()
		return nil, fmt.Errorf("llm_distiller: generate tests: %w", err)
	}

	// 解析测试用例
	tests, err := d.parseCapabilityTests(response)
	if err != nil {
		return nil, fmt.Errorf("llm_distiller: parse tests: %w", err)
	}

	d.logger.Info("LLM 能力测试生成完成",
		"capability", capabilityName,
		"tests", len(tests),
	)

	return tests, nil
}

// GetKnowledge 获取知识项
func (d *LLMDistiller) GetKnowledge(id string) (*KnowledgeItem, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	item, exists := d.store[id]
	if !exists {
		return nil, false
	}
	cp := *item
	return &cp, true
}

// SearchKnowledge 搜索知识库
func (d *LLMDistiller) SearchKnowledge(category, query string) []KnowledgeItem {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var results []KnowledgeItem
	for _, item := range d.store {
		if category != "" && item.Category != category {
			continue
		}
		if query != "" {
			queryLower := strings.ToLower(query)
			if !strings.Contains(strings.ToLower(item.Pattern), queryLower) &&
				!strings.Contains(strings.ToLower(item.Context), queryLower) {
				continue
			}
		}
		results = append(results, *item)
	}
	return results
}

// GetStats 获取蒸馏统计
func (d *LLMDistiller) GetStats() LLMDistillerStats {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.stats
}

// ===== 内部方法 =====

// buildDistillSystemPrompt 构造蒸馏系统提示
func (d *LLMDistiller) buildDistillSystemPrompt() string {
	return `你是一个知识提取专家。从 Agent 与用户的交互中提取结构化知识。

分析交互内容，提取以下类型的知识：
- fact: 事实性知识（定义、规则、数据）
- skill: 技能性知识（如何做某事的方法/步骤）
- preference: 用户偏好（喜欢/不喜欢的响应风格）
- pattern: 行为模式（重复出现的交互模式）

以 JSON 格式返回，结构如下：
{
  "knowledge": [
    {
      "category": "fact|skill|preference|pattern",
      "pattern": "提取的知识内容",
      "context": "适用上下文",
      "confidence": 0.0-1.0
    }
  ],
  "summary": "本次提取的简要总结"
}

注意：
- confidence 应基于知识的确定性和交互的成功程度
- 仅提取有价值的、可复用的知识
- 不要提取过于泛化或无意义的知识
- 仅返回 JSON，不要其他内容`
}

// buildDistillUserPrompt 构造蒸馏用户提示
func (d *LLMDistiller) buildDistillUserPrompt(interactions []Interaction) string {
	var sb strings.Builder
	sb.WriteString("请从以下 Agent 交互中提取知识：\n\n")

	for i, inter := range interactions {
		sb.WriteString(fmt.Sprintf("--- 交互 %d ---\n", i+1))
		sb.WriteString(fmt.Sprintf("用户输入: %s\n", inter.UserInput))
		sb.WriteString(fmt.Sprintf("Agent 输出: %s\n", inter.AgentOutput))
		if inter.Feedback != "" {
			sb.WriteString(fmt.Sprintf("用户反馈: %s\n", inter.Feedback))
		}
		sb.WriteString(fmt.Sprintf("是否成功: %v\n", inter.Success))
		sb.WriteString("\n")
	}

	return sb.String()
}

// parseExtraction 解析 LLM 提取结果
func (d *LLMDistiller) parseExtraction(response string) (*LLMExtractionResult, error) {
	// 尝试提取 JSON（LLM 可能返回额外文本）
	jsonStr := extractJSON(response)
	if jsonStr == "" {
		return nil, fmt.Errorf("no JSON found in LLM response")
	}

	var result LLMExtractionResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("unmarshal extraction: %w", err)
	}

	return &result, nil
}

// parseCapabilityTests 解析 LLM 生成的能力测试
func (d *LLMDistiller) parseCapabilityTests(response string) ([]LLMCapabilityTest, error) {
	jsonStr := extractJSON(response)
	if jsonStr == "" {
		return nil, fmt.Errorf("no JSON found in LLM response")
	}

	var tests []LLMCapabilityTest
	if err := json.Unmarshal([]byte(jsonStr), &tests); err != nil {
		return nil, fmt.Errorf("unmarshal tests: %w", err)
	}

	return tests, nil
}

// extractJSON 从 LLM 响应中提取 JSON 字符串
//
// 处理 LLM 可能返回的 markdown 代码块包裹：
//
//	```json
//	{...}
//	```
func extractJSON(response string) string {
	response = strings.TrimSpace(response)
	if response == "" {
		return ""
	}

	// 尝试从 markdown 代码块中提取（支持块前有文本）
	if idx := strings.Index(response, "```"); idx >= 0 {
		afterFence := response[idx+3:]
		// 跳过语言标识行（如 "json\n"）
		if nlIdx := strings.Index(afterFence, "\n"); nlIdx >= 0 {
			afterFence = afterFence[nlIdx+1:]
		}
		// 找到结束 fence
		if endIdx := strings.Index(afterFence, "```"); endIdx >= 0 {
			candidate := strings.TrimSpace(afterFence[:endIdx])
			if strings.HasPrefix(candidate, "{") || strings.HasPrefix(candidate, "[") {
				return candidate
			}
		}
	}

	// 直接以 JSON 开头
	if strings.HasPrefix(response, "{") || strings.HasPrefix(response, "[") {
		// 找到匹配的结束括号
		return balanceJSON(response)
	}

	// 从文本中查找 JSON 起始
	startObj := strings.Index(response, "{")
	startArr := strings.Index(response, "[")

	start := -1
	if startObj >= 0 && (startArr < 0 || startObj < startArr) {
		start = startObj
	} else if startArr >= 0 {
		start = startArr
	}

	if start >= 0 {
		return balanceJSON(response[start:])
	}

	return ""
}

// balanceJSON 找到 JSON 字符串的平衡结束位置
func balanceJSON(s string) string {
	if len(s) == 0 {
		return ""
	}

	open := s[0]
	var close byte
	if open == '{' {
		close = '}'
	} else if open == '[' {
		close = ']'
	} else {
		return s
	}

	depth := 0
	inString := false
	escaped := false

	for i := 0; i < len(s); i++ {
		ch := s[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && inString {
			escaped = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if ch == open {
			depth++
		} else if ch == close {
			depth--
			if depth == 0 {
				return s[:i+1]
			}
		}
	}

	// 未找到平衡，返回原字符串
	return s
}

// Package learning 提供 Agent 自适应学习与知识蒸馏能力。
//
// v3.0 方向3：Agent 自适应学习 + 知识蒸馏
//
// 核心组件：
//   - KnowledgeDistiller：从交互中提取知识→压缩→存入语义记忆
//   - CapabilityEvolver：Agent 能力评估→弱项识别→自动改进
//   - FeedbackLearner：人类反馈→偏好模型→行为调整
package learning

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ===== 知识蒸馏 =====

// Interaction 一次 Agent 交互
type Interaction struct {
	ID          string            `json:"id"`
	UserInput   string            `json:"user_input"`
	AgentOutput string            `json:"agent_output"`
	Feedback    string            `json:"feedback,omitempty"` // 可选的人类反馈
	Success     bool              `json:"success"`
	Timestamp   time.Time         `json:"timestamp"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// KnowledgeItem 蒸馏出的知识项
type KnowledgeItem struct {
	ID          string `json:"id"`
	Category    string `json:"category"`    // "fact"/"skill"/"preference"/"pattern"
	Pattern     string `json:"pattern"`     // 提取的模式/规则
	Context     string `json:"context"`    // 适用上下文
	Confidence  float64 `json:"confidence"` // 置信度 0-1
	Source      string `json:"source"`     // 来源交互 ID
	TimesUsed   int     `json:"times_used"`
	TimesCorrect int   `json:"times_correct"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// KnowledgeDistiller 知识蒸馏器
type KnowledgeDistiller struct {
	logger *slog.Logger
	mu     sync.RWMutex
	store  map[string]*KnowledgeItem // 知识库
	stats  DistillerStats
}

// DistillerStats 蒸馏统计
type DistillerStats struct {
	TotalInteractions  atomic.Int64
	TotalDistilled     atomic.Int64
	TotalKnowledgeItems atomic.Int64
}

// NewKnowledgeDistiller 创建知识蒸馏器
func NewKnowledgeDistiller() *KnowledgeDistiller {
	return &KnowledgeDistiller{
		logger: slog.Default(),
		store:  make(map[string]*KnowledgeItem),
	}
}

// WithLogger 设置日志器
func (d *KnowledgeDistiller) WithLogger(logger *slog.Logger) *KnowledgeDistiller {
	d.logger = logger
	return d
}

// Distill 从一次交互中蒸馏知识
func (d *KnowledgeDistiller) Distill(ctx context.Context, interaction Interaction) ([]KnowledgeItem, error) {
	d.stats.TotalInteractions.Add(1)

	var items []KnowledgeItem

	// 1. 提取事实类知识
	if facts := d.extractFacts(interaction); len(facts) > 0 {
		items = append(items, facts...)
	}

	// 2. 提取模式类知识
	if patterns := d.extractPatterns(interaction); len(patterns) > 0 {
		items = append(items, patterns...)
	}

	// 3. 从反馈中提取偏好
	if interaction.Feedback != "" {
		if prefs := d.extractPreferences(interaction); len(prefs) > 0 {
			items = append(items, prefs...)
		}
	}

	// 4. 存储到知识库
	d.mu.Lock()
	for _, item := range items {
		d.store[item.ID] = &item
		d.stats.TotalKnowledgeItems.Add(1)
	}
	d.mu.Unlock()

	d.stats.TotalDistilled.Add(int64(len(items)))
	d.logger.Info("知识蒸馏完成",
		"interaction", interaction.ID,
		"items", len(items),
	)

	return items, nil
}

// extractFacts 从交互中提取事实类知识
func (d *KnowledgeDistiller) extractFacts(inter Interaction) []KnowledgeItem {
	var items []KnowledgeItem

	// 简化版：检查 Agent 输出中是否包含确定的事实
	// 实际实现可以使用 NLP 或 LLM 来提取
	factIndicators := []string{"is", "are", "was", "were", "means", "equals", "refers to"}

	sentences := splitSentences(inter.AgentOutput)
	for _, sentence := range sentences {
		sentenceLower := strings.ToLower(sentence)
		for _, indicator := range factIndicators {
			if strings.Contains(sentenceLower, " "+indicator+" ") {
				item := KnowledgeItem{
					ID:         fmt.Sprintf("fact_%s_%d", inter.ID, len(items)),
					Category:   "fact",
					Pattern:    sentence,
					Confidence: 0.7, // 基础置信度
					Source:     inter.ID,
					CreatedAt:  time.Now(),
					UpdatedAt:  time.Now(),
				}
				if inter.Success {
					item.Confidence = 0.8
				}
				if inter.Feedback != "" {
					item.Confidence = 0.9
				}
				items = append(items, item)
				break
			}
		}
	}

	return items
}

// extractPatterns 从交互中提取模式类知识
func (d *KnowledgeDistiller) extractPatterns(inter Interaction) []KnowledgeItem {
	var items []KnowledgeItem

	// 检查是否有重复的操作模式
	// 简化版：检查用户输入的模式
	if strings.Contains(strings.ToLower(inter.UserInput), "how") ||
		strings.Contains(strings.ToLower(inter.UserInput), "what") {
		item := KnowledgeItem{
			ID:         fmt.Sprintf("pattern_%s", inter.ID),
			Category:   "pattern",
			Pattern:    "question_answering",
			Context:    "user asks a question",
			Confidence: 0.6,
			Source:     inter.ID,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}
		if inter.Success {
			item.TimesCorrect = 1
		}
		items = append(items, item)
	}

	return items
}

// extractPreferences 从反馈中提取偏好
func (d *KnowledgeDistiller) extractPreferences(inter Interaction) []KnowledgeItem {
	var items []KnowledgeItem

	feedbackLower := strings.ToLower(inter.Feedback)

	// 正面反馈
	if strings.Contains(feedbackLower, "good") ||
		strings.Contains(feedbackLower, "great") ||
		strings.Contains(feedbackLower, "correct") ||
		strings.Contains(feedbackLower, "right") {
		items = append(items, KnowledgeItem{
			ID:         fmt.Sprintf("pref_pos_%s", inter.ID),
			Category:   "preference",
			Pattern:    "user prefers this type of response",
			Confidence: 0.85,
			Source:     inter.ID,
			TimesCorrect: 1,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		})
	}

	// 负面反馈
	if strings.Contains(feedbackLower, "bad") ||
		strings.Contains(feedbackLower, "wrong") ||
		strings.Contains(feedbackLower, "incorrect") ||
		strings.Contains(feedbackLower, "no") {
		items = append(items, KnowledgeItem{
			ID:         fmt.Sprintf("pref_neg_%s", inter.ID),
			Category:   "preference",
			Pattern:    "user dislikes this type of response",
			Confidence: 0.85,
			Source:     inter.ID,
			TimesCorrect: 0,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		})
	}

	return items
}

// GetKnowledge 获取知识项
func (d *KnowledgeDistiller) GetKnowledge(id string) (*KnowledgeItem, bool) {
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
func (d *KnowledgeDistiller) SearchKnowledge(category, query string) []KnowledgeItem {
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
func (d *KnowledgeDistiller) GetStats() DistillerStats {
	return d.stats
}

// splitSentences 分割句子
func splitSentences(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	// 按句号/问号/感叹号分割
	var sentences []string
	start := 0
	for i, ch := range text {
		if ch == '.' || ch == '!' || ch == '?' {
			sentence := strings.TrimSpace(text[start : i+1])
			if sentence != "" {
				sentences = append(sentences, sentence)
			}
			start = i + 1
		}
	}
	if start < len(text) {
		remaining := strings.TrimSpace(text[start:])
		if remaining != "" {
			sentences = append(sentences, remaining)
		}
	}
	return sentences
}

// ===== 能力进化 =====

// Capability 能力定义
type Capability struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Score       float64 `json:"score"`       // 当前评分 0-1
	TimesTested int     `json:"times_tested"`
	TimesPassed int     `json:"times_passed"`
}

// CapabilityEvolver 能力进化器
type CapabilityEvolver struct {
	mu       sync.RWMutex
	capabilities map[string]*Capability
	logger   *slog.Logger
}

// NewCapabilityEvolver 创建能力进化器
func NewCapabilityEvolver() *CapabilityEvolver {
	return &CapabilityEvolver{
		capabilities: make(map[string]*Capability),
		logger:       slog.Default(),
	}
}

// Register 注册一个能力
func (e *CapabilityEvolver) Register(cap Capability) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.capabilities[cap.Name] = &cap
}

// Evaluate 评估一个能力
func (e *CapabilityEvolver) Evaluate(capName string, passed bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	cap, exists := e.capabilities[capName]
	if !exists {
		return fmt.Errorf("learning: capability %q not registered", capName)
	}

	cap.TimesTested++
	if passed {
		cap.TimesPassed++
	}

	// 更新评分（加权移动平均）
	total := float64(cap.TimesTested)
	passRate := float64(cap.TimesPassed) / total
	// 平滑评分：新评分 = 旧评分 * 0.7 + 通过率 * 0.3
	cap.Score = cap.Score*0.7 + passRate*0.3

	e.logger.Info("能力评估",
		"capability", capName,
		"passed", passed,
		"score", cap.Score,
		"tests", cap.TimesTested,
	)

	return nil
}

// GetWeaknesses 获取弱项能力（评分低于阈值）
func (e *CapabilityEvolver) GetWeaknesses(threshold float64) []Capability {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var results []Capability
	for _, cap := range e.capabilities {
		if cap.Score < threshold {
			results = append(results, *cap)
		}
	}
	return results
}

// GetCapability 获取能力信息
func (e *CapabilityEvolver) GetCapability(name string) (*Capability, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	cap, exists := e.capabilities[name]
	if !exists {
		return nil, false
	}
	cp := *cap
	return &cp, true
}

// ListCapabilities 列出所有能力
func (e *CapabilityEvolver) ListCapabilities() []Capability {
	e.mu.RLock()
	defer e.mu.RUnlock()
	result := make([]Capability, 0, len(e.capabilities))
	for _, cap := range e.capabilities {
		result = append(result, *cap)
	}
	return result
}

// ===== 反馈学习 =====

// FeedbackEntry 反馈条目
type FeedbackEntry struct {
	ID          string `json:"id"`
	UserInput   string `json:"user_input"`
	AgentOutput string `json:"agent_output"`
	Feedback    string `json:"feedback"`
	Rating      int    `json:"rating"` // -1 负面, 0 中性, 1 正面
	Timestamp   time.Time `json:"timestamp"`
}

// PreferenceModel 偏好模型
type PreferenceModel struct {
	PositivePatterns []string `json:"positive_patterns"`
	NegativePatterns []string `json:"negative_patterns"`
	TotalFeedback    int      `json:"total_feedback"`
}

// FeedbackLearner 反馈学习器
type FeedbackLearner struct {
	mu           sync.RWMutex
	feedback     []FeedbackEntry
	preferences  PreferenceModel
	logger       *slog.Logger
}

// NewFeedbackLearner 创建反馈学习器
func NewFeedbackLearner() *FeedbackLearner {
	return &FeedbackLearner{
		logger: slog.Default(),
	}
}

// RecordFeedback 记录反馈
func (f *FeedbackLearner) RecordFeedback(entry FeedbackEntry) error {
	if entry.ID == "" {
		entry.ID = fmt.Sprintf("fb_%d", time.Now().UnixNano())
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	f.feedback = append(f.feedback, entry)
	f.preferences.TotalFeedback++

	// 分析反馈内容
	feedbackLower := strings.ToLower(entry.Feedback)

	if entry.Rating > 0 || strings.Contains(feedbackLower, "good") ||
		strings.Contains(feedbackLower, "great") ||
		strings.Contains(feedbackLower, "correct") {
		// 正面模式
		pattern := extractPattern(entry.AgentOutput)
		if pattern != "" && !contains(f.preferences.PositivePatterns, pattern) {
			f.preferences.PositivePatterns = append(f.preferences.PositivePatterns, pattern)
		}
	}

	if entry.Rating < 0 || strings.Contains(feedbackLower, "bad") ||
		strings.Contains(feedbackLower, "wrong") ||
		strings.Contains(feedbackLower, "incorrect") {
		// 负面模式
		pattern := extractPattern(entry.AgentOutput)
		if pattern != "" && !contains(f.preferences.NegativePatterns, pattern) {
			f.preferences.NegativePatterns = append(f.preferences.NegativePatterns, pattern)
		}
	}

	f.logger.Info("反馈记录",
		"rating", entry.Rating,
		"total", f.preferences.TotalFeedback,
	)

	return nil
}

// GetPreferences 获取偏好模型
func (f *FeedbackLearner) GetPreferences() PreferenceModel {
	f.mu.RLock()
	defer f.mu.RUnlock()
	cp := f.preferences
	return cp
}

// ShouldPrefer 判断是否应偏好某种输出模式
func (f *FeedbackLearner) ShouldPrefer(output string) float64 {
	f.mu.RLock()
	defer f.mu.RUnlock()

	outputLower := strings.ToLower(output)
	posScore := 0.0
	negScore := 0.0

	for _, pattern := range f.preferences.PositivePatterns {
		if strings.Contains(outputLower, strings.ToLower(pattern)) {
			posScore += 1.0
		}
	}

	for _, pattern := range f.preferences.NegativePatterns {
		if strings.Contains(outputLower, strings.ToLower(pattern)) {
			negScore += 1.0
		}
	}

	total := posScore + negScore
	if total == 0 {
		return 0.5 // 中性
	}
	return posScore / total
}

// GetFeedbackHistory 获取反馈历史
func (f *FeedbackLearner) GetFeedbackHistory() []FeedbackEntry {
	f.mu.RLock()
	defer f.mu.RUnlock()
	result := make([]FeedbackEntry, len(f.feedback))
	copy(result, f.feedback)
	return result
}

// extractPattern 从输出中提取模式
func extractPattern(output string) string {
	// 简化版：提取第一句话作为模式
	sentences := splitSentences(output)
	if len(sentences) == 0 {
		return ""
	}
	first := sentences[0]
	if len(first) > 100 {
		first = first[:100]
	}
	return first
}

// contains 检查字符串是否在切片中
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
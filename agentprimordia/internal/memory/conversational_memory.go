package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	maxSummaryLen  = 500
	maxKeyPointLen = 100
	maxKeyPoints   = 5
)

// ConversationalMemory 是对话记忆系统，支持窗口管理和摘要压缩
type ConversationalMemory struct {
	mu             sync.RWMutex
	messages       []*Message
	summary        string
	maxMessages    int
	summaryTrigger int // 触发摘要的消息数量
	compressor     SummaryCompressor
	metadata       map[string]string
	lastUpdated    time.Time
	totalMessages  int // 总消息数（包括被压缩的）
}

// Message 表示一条对话消息
type Message struct {
	Role       string            `json:"role"`                  // user, assistant, system, tool
	Content    string            `json:"content"`               // 消息内容
	Timestamp  time.Time         `json:"timestamp"`             // 时间戳
	Metadata   map[string]string `json:"metadata,omitempty"`    // 元数据
	TokenCount int               `json:"token_count,omitempty"` // token 数量估算
}

// SummaryCompressor 是摘要压缩器接口
type SummaryCompressor interface {
	Compress(ctx context.Context, messages []*Message, existingSummary string) (string, error)
}

// ConversationalMemoryConfig 是配置
type ConversationalMemoryConfig struct {
	MaxMessages    int               `json:"max_messages"`    // 窗口最大消息数（默认 50）
	SummaryTrigger int               `json:"summary_trigger"` // 触发摘要的消息数（默认 40）
	Compressor     SummaryCompressor `json:"-"`               // 自定义压缩器
	InitialSummary string            `json:"initial_summary"` // 初始摘要
	Metadata       map[string]string `json:"metadata"`        // 元数据
}

// NewConversationalMemory 创建新的对话记忆
func NewConversationalMemory(config ConversationalMemoryConfig) *ConversationalMemory {
	if config.MaxMessages <= 0 {
		config.MaxMessages = 50
	}
	if config.SummaryTrigger <= 0 || config.SummaryTrigger >= config.MaxMessages {
		config.SummaryTrigger = config.MaxMessages * 80 / 100
	}
	if config.Compressor == nil {
		config.Compressor = &DefaultCompressor{}
	}

	return &ConversationalMemory{
		messages:       make([]*Message, 0),
		summary:        config.InitialSummary,
		maxMessages:    config.MaxMessages,
		summaryTrigger: config.SummaryTrigger,
		compressor:     config.Compressor,
		metadata:       config.Metadata,
		lastUpdated:    time.Now(),
	}
}

// AddMessage 添加消息到记忆中
func (m *ConversationalMemory) AddMessage(ctx context.Context, role, content string, metadata map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	msg := &Message{
		Role:       role,
		Content:    content,
		Timestamp:  time.Now(),
		Metadata:   metadata,
		TokenCount: estimateTokens(content),
	}

	m.messages = append(m.messages, msg)
	m.totalMessages++
	m.lastUpdated = time.Now()

	// 检查是否需要触发摘要压缩
	if len(m.messages) > m.summaryTrigger {
		if err := m.compressAndSummarizeLocked(ctx); err != nil {
			return fmt.Errorf("compress summary error: %w", err)
		}
	}

	// 如果超过最大消息数，移除最旧的消息
	if len(m.messages) > m.maxMessages {
		m.messages = m.messages[len(m.messages)-m.maxMessages:]
	}

	return nil
}

func (m *ConversationalMemory) summarySystemMessage() *Message {
	if m.summary == "" {
		return nil
	}
	return &Message{
		Role:    "system",
		Content: fmt.Sprintf("[Previous conversation summary]\n%s", m.summary),
	}
}

// getMessagesLocked 获取消息列表（调用方必须持有锁）
func (m *ConversationalMemory) getMessagesLocked() []*Message {
	result := make([]*Message, 0, len(m.messages)+1)

	if msg := m.summarySystemMessage(); msg != nil {
		msg.Timestamp = m.lastUpdated
		result = append(result, msg)
	}

	result = append(result, m.messages...)
	return result
}

// GetMessages 获取当前窗口内的所有消息（包含摘要作为系统消息）
func (m *ConversationalMemory) GetMessages() []*Message {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.getMessagesLocked()
}

// GetRecentMessages 获取最近的 N 条消息
func (m *ConversationalMemory) GetRecentMessages(n int) []*Message {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if n <= 0 || n >= len(m.messages) {
		return m.getMessagesLocked()
	}

	recent := m.messages[len(m.messages)-n:]
	result := make([]*Message, 0, len(recent)+1)

	if msg := m.summarySystemMessage(); msg != nil {
		result = append(result, msg)
	}

	result = append(result, recent...)
	return result
}

// GetSummary 获取当前摘要
func (m *ConversationalMemory) GetSummary() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.summary
}

// Clear 清空所有消息和摘要
func (m *ConversationalMemory) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.messages = make([]*Message, 0)
	m.summary = ""
	m.totalMessages = 0
	m.lastUpdated = time.Now()
}

// GetMessageCount 获取当前消息数
func (m *ConversationalMemory) GetMessageCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.messages)
}

// GetTotalMessageCount 获取总消息数（包括被压缩的）
func (m *ConversationalMemory) GetTotalMessageCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.totalMessages
}

// getStatsLocked 获取统计信息（调用方必须持有锁）
func (m *ConversationalMemory) getStatsLocked() map[string]any {
	totalTokens := 0
	for _, msg := range m.messages {
		totalTokens += msg.TokenCount
	}

	userMsgCount := 0
	assistantMsgCount := 0
	for _, msg := range m.messages {
		switch msg.Role {
		case "user":
			userMsgCount++
		case "assistant":
			assistantMsgCount++
		}
	}

	return map[string]any{
		"current_messages":   len(m.messages),
		"total_messages":     m.totalMessages,
		"summary_length":     len(m.summary),
		"estimated_tokens":   totalTokens,
		"user_messages":      userMsgCount,
		"assistant_messages": assistantMsgCount,
		"last_updated":       m.lastUpdated.Format(time.RFC3339),
		"compression_count":  m.totalMessages - len(m.messages),
	}
}

// GetStats 获取统计信息
func (m *ConversationalMemory) GetStats() map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.getStatsLocked()
}

// Export 导出为 JSON
func (m *ConversationalMemory) Export() ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	data := map[string]any{
		"messages":       m.messages,
		"summary":        m.summary,
		"total_messages": m.totalMessages,
		"stats":          m.getStatsLocked(),
		"exported_at":    time.Now().Format(time.RFC3339),
	}

	return json.MarshalIndent(data, "", "  ")
}

// Import 从 JSON 导入
func (m *ConversationalMemory) Import(data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var imported struct {
		Messages      []*Message `json:"messages"`
		Summary       string     `json:"summary"`
		TotalMessages int        `json:"total_messages"`
	}

	if err := json.Unmarshal(data, &imported); err != nil {
		return fmt.Errorf("unmarshal error: %w", err)
	}

	m.messages = imported.Messages
	m.summary = imported.Summary
	m.totalMessages = imported.TotalMessages
	m.lastUpdated = time.Now()

	return nil
}

// compressAndSummarizeLocked 压缩并生成摘要（必须在锁内调用）
func (m *ConversationalMemory) compressAndSummarizeLocked(ctx context.Context) error {
	if len(m.messages) < 2 {
		return nil
	}

	// 取前半部分进行压缩
	splitPoint := len(m.messages) / 2
	oldMessages := m.messages[:splitPoint]

	newSummary, err := m.compressor.Compress(ctx, oldMessages, m.summary)
	if err != nil {
		return err
	}

	// 更新摘要并保留后半部分消息
	m.summary = newSummary
	m.messages = m.messages[splitPoint:]

	return nil
}

// ===== 默认压缩器 =====

// DefaultCompressor 是基于规则的简单摘要压缩器
type DefaultCompressor struct{}

func (c *DefaultCompressor) Compress(_ context.Context, messages []*Message, existingSummary string) (string, error) {
	var parts []string

	if existingSummary != "" {
		parts = append(parts, existingSummary)
	}

	var userTopics []string
	var assistantResponses []string

	for _, msg := range messages {
		switch msg.Role {
		case "user":
			extractKeyPoints(msg.Content, &userTopics)
		case "assistant":
			extractKeyPoints(msg.Content, &assistantResponses)
		}
	}

	if len(userTopics) > 0 {
		parts = append(parts, fmt.Sprintf("User discussed: %s", strings.Join(userTopics, "; ")))
	}
	if len(assistantResponses) > 0 {
		parts = append(parts, fmt.Sprintf("Assistant provided: %s", strings.Join(assistantResponses, "; ")))
	}

	summary := strings.Join(parts, "\n")
	if len(summary) > maxSummaryLen {
		summary = summary[:maxSummaryLen-3] + "..."
	}

	return summary, nil
}

// extractKeyPoints 从文本中提取关键点（简化版）
func extractKeyPoints(text string, points *[]string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}

	// 提取第一句或前100个字符
	if len(text) > maxKeyPointLen {
		text = text[:maxKeyPointLen-3] + "..."
	}

	*points = append(*points, text)
	if len(*points) > maxKeyPoints {
		*points = (*points)[len(*points)-maxKeyPoints:]
	}
}

// ===== LLM 压缩器（可扩展）=====

// LLMCompressFunc 是 LLM 压缩函数类型，用于解耦 memory 和 llm 包
// 输入: 需要压缩的文本内容, 最大输出字符数
// 输出: 压缩后的摘要文本, 错误信息
type LLMCompressFunc func(ctx context.Context, content string, maxChars int) (string, error)

// LLMCompressor 使用 LLM 进行智能摘要
type LLMCompressor struct {
	PromptTemplate string          // 用于生成摘要的 Prompt 模板
	MaxSummaryLen  int             // 摘要最大长度（默认 500）
	CompressFn     LLMCompressFunc // 可选的 LLM 压缩函数，为 nil 时回退到规则压缩
}

func (c *LLMCompressor) Compress(ctx context.Context, messages []*Message, existingSummary string) (string, error) {
	maxLen := c.MaxSummaryLen
	if maxLen <= 0 {
		maxLen = maxSummaryLen
	}

	content := buildCompressContent(messages, existingSummary)

	if c.CompressFn != nil {
		summary, err := c.CompressFn(ctx, content, maxLen)
		if err == nil && summary != "" {
			return summary, nil
		}
	}

	defaultCompressor := &DefaultCompressor{}
	return defaultCompressor.Compress(ctx, messages, existingSummary)
}

// buildCompressContent 构建需要压缩的完整文本内容
func buildCompressContent(messages []*Message, existingSummary string) string {
	var parts []string

	if existingSummary != "" {
		parts = append(parts, "[已有摘要]\n"+existingSummary)
	}

	parts = append(parts, "[待压缩对话]")
	for _, msg := range messages {
		parts = append(parts, fmt.Sprintf("[%s] %s", msg.Role, msg.Content))
	}

	return strings.Join(parts, "\n\n")
}

// estimateTokens 估算 token 数量（粗略估算：1 token ≈ 4 字符 for 中文，≈ 4 words for English）
func estimateTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	runeCount := len([]rune(text))
	wordCount := len(strings.Fields(text))

	estimate := runeCount / 4
	if wordCount > estimate {
		estimate = wordCount
	}

	return max(1, estimate)
}

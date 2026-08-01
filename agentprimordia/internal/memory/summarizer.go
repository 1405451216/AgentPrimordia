package memory

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

const defaultSummarizerRetries = 1

// SummaryResult 摘要提取结果
type SummaryResult struct {
	Summary string
	Topics  string
}

// SummaryExtractor 摘要提取接口，供 Agent 层依赖
type SummaryExtractor interface {
	ExtractSummary(ctx context.Context, content string) (*SummaryResult, error)
}

// SummarizerLLM Summarizer 所需的 LLM 能力接口（解耦 memory→llm 依赖）
type SummarizerLLM interface {
	Complete(ctx context.Context, messages []ChatMessageForSummary, model string) (string, error)
}

// ChatMessageForSummary 摘要提取用的简化消息结构
type ChatMessageForSummary struct {
	Role    string
	Content string
}

// llmAdapter 将 llm.Provider 适配为 SummarizerLLM 接口
// 定义在此文件中以避免 memory 包直接依赖 llm 包
type llmAdapter struct {
	completeFn func(ctx context.Context, messages []ChatMessageForSummary, model string) (string, error)
}

func (a *llmAdapter) Complete(ctx context.Context, messages []ChatMessageForSummary, model string) (string, error) {
	return a.completeFn(ctx, messages, model)
}

// Summarizer 使用 LLM 从内容中提取摘要和标签
type Summarizer struct {
	provider   SummarizerLLM
	model      string
	maxRetries int
	logger     *slog.Logger
}

// NewSummarizer 创建摘要提取器
// 接受 SummarizerLLM 接口，通过适配器模式解耦 llm 包依赖
func NewSummarizer(provider SummarizerLLM) *Summarizer {
	return &Summarizer{
		provider:   provider,
		maxRetries: defaultSummarizerRetries,
		logger:     slog.Default(),
	}
}

// WithModel 设置摘要使用的模型（如 flash/mini 版本以降低成本）
func (s *Summarizer) WithModel(model string) *Summarizer {
	s.model = model
	return s
}

// ExtractSummary 从内容中提取摘要和标签
func (s *Summarizer) ExtractSummary(ctx context.Context, content string) (*SummaryResult, error) {
	prompt := fmt.Sprintf(`请为以下内容生成简短摘要（1-2句话）和标签。

内容：
%s

请按以下格式输出：
第一行：摘要
第二行：topics: 标签1,标签2,标签3`, content)

	messages := []ChatMessageForSummary{
		{Role: "system", Content: "你是一个摘要提取助手。请简洁地提取摘要和标签。"},
		{Role: "user", Content: prompt},
	}

	resp, err := s.provider.Complete(ctx, messages, s.model)
	if err != nil {
		return nil, fmt.Errorf("summary extraction failed: %w", err)
	}

	summary, topics := parseSummaryResponse(resp)
	return &SummaryResult{
		Summary: summary,
		Topics:  topics,
	}, nil
}

// parseSummaryResponse 解析 LLM 返回的摘要和标签
// 格式：第一部分为摘要，最后一行 "topics: xxx" 为标签
func parseSummaryResponse(response string) (summary string, topics string) {
	lines := strings.Split(response, "\n")
	var summaryLines []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "topics:") || strings.HasPrefix(trimmed, "Topics:") {
			topics = strings.TrimSpace(strings.TrimPrefix(trimmed, "topics:"))
			topics = strings.TrimSpace(strings.TrimPrefix(topics, "Topics:"))
			continue
		}
		if trimmed != "" {
			summaryLines = append(summaryLines, trimmed)
		}
	}

	summary = strings.Join(summaryLines, "\n")
	return
}

// CleanupConfig 自动清理配置
type CleanupConfig struct {
	MaxAgeDays    int           // 过期天数（默认 30）
	Interval      time.Duration // 清理间隔（默认 24 小时）
	PreserveRoles []string      // 保留的角色（默认 ["tool"]）
}

// DefaultCleanupConfig 返回默认清理配置
func DefaultCleanupConfig() CleanupConfig {
	return CleanupConfig{
		MaxAgeDays:    30,
		Interval:      24 * time.Hour,
		PreserveRoles: []string{"tool"},
	}
}

// StartAutoCleanup 启动后台自动清理 goroutine
// 返回 stop 函数供调用方停止清理
func (s *SQLiteStore) StartAutoCleanup(cfg CleanupConfig) (stop func()) {
	if cfg.Interval <= 0 {
		cfg.Interval = 24 * time.Hour
	}
	if cfg.MaxAgeDays <= 0 {
		cfg.MaxAgeDays = 30
	}
	if cfg.PreserveRoles == nil {
		cfg.PreserveRoles = []string{"tool"}
	}

	done := make(chan struct{})

	go func() {
		ticker := time.NewTicker(cfg.Interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				s.autoCleanup(cfg)
			case <-done:
				return
			}
		}
	}()

	return func() { close(done) }
}

// autoCleanup 执行一次自动清理
func (s *SQLiteStore) autoCleanup(cfg CleanupConfig) {
	s.mu.RLock()
	db := s.db
	s.mu.RUnlock()

	if db == nil {
		return
	}

	ctx := context.Background()

	if len(cfg.PreserveRoles) > 0 {
		s.cleanupWithPreserve(ctx, cfg.MaxAgeDays, cfg.PreserveRoles)
	} else {
		_, _ = s.CleanupExpired(ctx, cfg.MaxAgeDays)
	}
}

// cleanupWithPreserve 清理过期记忆但保留指定角色的记录
func (s *SQLiteStore) cleanupWithPreserve(ctx context.Context, maxAgeDays int, preserveRoles []string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db == nil {
		return
	}

	cutoff := time.Now().AddDate(0, 0, -maxAgeDays)

	placeholders := make([]string, len(preserveRoles))
	args := make([]any, len(preserveRoles))
	for i, role := range preserveRoles {
		placeholders[i] = "?"
		args[i] = role
	}

	query := fmt.Sprintf(
		"DELETE FROM episodes WHERE created_at < ? AND role NOT IN (%s)",
		strings.Join(placeholders, ","),
	)
	args = append([]any{cutoff}, args...)

	_, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		s.logger.Error("自动清理失败", "error", err)
	}
}

// ExtractSummaryAsync 异步提取摘要，不阻塞调用方
// 成功后自动更新 Episode 的 summary 和 topics
// 返回错误通道供可选的错误监听
func (s *SQLiteStore) ExtractSummaryAsync(ctx context.Context, id string, summarizer *Summarizer) <-chan error {
	errCh := make(chan error, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				errCh <- fmt.Errorf("summary extraction panic: %v", r)
			}
		}()

		ep, err := s.Get(ctx, id)
		if err != nil {
			errCh <- fmt.Errorf("failed to get Episode: %w", err)
			return
		}

		result, err := summarizer.ExtractSummary(ctx, ep.Content)
		if err != nil {
			s.logger.Warn("异步摘要提取失败", "id", id, "error", err)
			errCh <- err
			return
		}

		if err := s.UpdateSummary(ctx, id, result.Summary, result.Topics); err != nil {
			s.logger.Warn("异步更新摘要失败", "id", id, "error", err)
			errCh <- err
			return
		}

		errCh <- nil
	}()

	return errCh
}

type SummaryStrategy interface {
	ShouldSummarize(ctx context.Context, store Memory, sessionID string) (bool, error)
	SelectEpisodes(ctx context.Context, store Memory, sessionID string) ([]*Episode, error)
	MaxGroupSize() int
}

type WindowSummaryStrategy struct {
	WindowSize    int
	MinEpisodes   int
	RoleFilter    string
	ImportanceMin float64
}

func NewWindowSummaryStrategy(windowSize int) *WindowSummaryStrategy {
	return &WindowSummaryStrategy{
		WindowSize:  windowSize,
		MinEpisodes: 2,
	}
}

func (s *WindowSummaryStrategy) ShouldSummarize(ctx context.Context, store Memory, sessionID string) (bool, error) {
	count, err := store.Count(ctx, sessionID)
	if err != nil {
		return false, err
	}
	return count >= int64(s.WindowSize), nil
}

func (s *WindowSummaryStrategy) SelectEpisodes(ctx context.Context, store Memory, sessionID string) ([]*Episode, error) {
	opts := &ListOptions{
		SessionID: sessionID,
		Limit:     s.WindowSize,
		OrderBy:   "created_at",
		Ascending: true,
	}

	episodes, err := store.List(ctx, opts)
	if err != nil {
		return nil, err
	}

	if s.RoleFilter != "" {
		var filtered []*Episode
		for _, ep := range episodes {
			if ep.Role == s.RoleFilter {
				filtered = append(filtered, ep)
			}
		}
		episodes = filtered
	}

	if s.ImportanceMin > 0 {
		var filtered []*Episode
		for _, ep := range episodes {
			if ep.Importance >= s.ImportanceMin {
				filtered = append(filtered, ep)
			}
		}
		episodes = filtered
	}

	if len(episodes) < s.MinEpisodes {
		return nil, nil
	}

	return episodes, nil
}

func (s *WindowSummaryStrategy) MaxGroupSize() int {
	return s.WindowSize
}

type ImportanceSummaryStrategy struct {
	Threshold   float64
	MinEpisodes int
	MaxEpisodes int
	SessionID   string
}

func NewImportanceSummaryStrategy(threshold float64) *ImportanceSummaryStrategy {
	return &ImportanceSummaryStrategy{
		Threshold:   threshold,
		MinEpisodes: 2,
		MaxEpisodes: 20,
	}
}

func (s *ImportanceSummaryStrategy) ShouldSummarize(ctx context.Context, store Memory, sessionID string) (bool, error) {
	episodes, err := store.GetImportant(ctx, s.Threshold, s.MaxEpisodes)
	if err != nil {
		return false, err
	}
	return len(episodes) >= s.MinEpisodes, nil
}

func (s *ImportanceSummaryStrategy) SelectEpisodes(ctx context.Context, store Memory, sessionID string) ([]*Episode, error) {
	episodes, err := store.GetImportant(ctx, s.Threshold, s.MaxEpisodes)
	if err != nil {
		return nil, err
	}

	if s.SessionID != "" {
		var filtered []*Episode
		for _, ep := range episodes {
			if ep.SessionID == s.SessionID {
				filtered = append(filtered, ep)
			}
		}
		episodes = filtered
	}

	if len(episodes) < s.MinEpisodes {
		return nil, nil
	}

	return episodes, nil
}

func (s *ImportanceSummaryStrategy) MaxGroupSize() int {
	return s.MaxEpisodes
}

type SessionSummaryStrategy struct {
	MinEpisodes int
	MaxEpisodes int
}

func NewSessionSummaryStrategy() *SessionSummaryStrategy {
	return &SessionSummaryStrategy{
		MinEpisodes: 3,
		MaxEpisodes: 50,
	}
}

func (s *SessionSummaryStrategy) ShouldSummarize(ctx context.Context, store Memory, sessionID string) (bool, error) {
	if sessionID == "" {
		return false, nil
	}
	count, err := store.Count(ctx, sessionID)
	if err != nil {
		return false, err
	}
	return count >= int64(s.MinEpisodes), nil
}

func (s *SessionSummaryStrategy) SelectEpisodes(ctx context.Context, store Memory, sessionID string) ([]*Episode, error) {
	if sessionID == "" {
		return nil, nil
	}

	opts := &ListOptions{
		SessionID: sessionID,
		Limit:     s.MaxEpisodes,
		OrderBy:   "created_at",
		Ascending: true,
	}

	episodes, err := store.List(ctx, opts)
	if err != nil {
		return nil, err
	}

	if len(episodes) < s.MinEpisodes {
		return nil, nil
	}

	return episodes, nil
}

func (s *SessionSummaryStrategy) MaxGroupSize() int {
	return s.MaxEpisodes
}

type SummaryEngine struct {
	strategy   SummaryStrategy
	summarizer *Summarizer
	store      Memory
	logger     *slog.Logger
}

func NewSummaryEngine(strategy SummaryStrategy, summarizer *Summarizer, store Memory) *SummaryEngine {
	return &SummaryEngine{
		strategy:   strategy,
		summarizer: summarizer,
		store:      store,
		logger:     slog.Default(),
	}
}

func (e *SummaryEngine) Run(ctx context.Context, sessionID string) (*SummaryResult, error) {
	should, err := e.strategy.ShouldSummarize(ctx, e.store, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to check if summarization needed: %w", err)
	}
	if !should {
		return nil, nil
	}

	episodes, err := e.strategy.SelectEpisodes(ctx, e.store, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to select memory episodes: %w", err)
	}
	if len(episodes) == 0 {
		return nil, nil
	}

	var contents []string
	for _, ep := range episodes {
		contents = append(contents, ep.Content)
	}
	combined := strings.Join(contents, "\n---\n")

	result, err := e.summarizer.ExtractSummary(ctx, combined)
	if err != nil {
		return nil, fmt.Errorf("failed to extract summary: %w", err)
	}

	return result, nil
}

func (e *SummaryEngine) RunAndStore(ctx context.Context, sessionID string) (*SummaryResult, error) {
	result, err := e.Run(ctx, sessionID)
	if err != nil || result == nil {
		return result, err
	}

	summaryEp := &Episode{
		ID:        fmt.Sprintf("summary-%s-%d", sessionID, time.Now().UnixMilli()),
		SessionID: sessionID,
		Role:      "system",
		Content:   result.Summary,
		Summary:   result.Summary,
		Topics:    result.Topics,
		Metadata:  map[string]string{"type": "auto_summary"},
	}

	if err := e.store.Add(ctx, summaryEp); err != nil {
		e.logger.Warn("存储摘要失败", "error", err)
	}

	return result, nil
}

package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

const defaultSearchLimit = 10

type BackendType string

const (
	BackendSQLite BackendType = "sqlite"
	BackendMemory BackendType = "memory"
)

type Config struct {
	Type BackendType
	Path string // 用于 SQLite，路径
}

func NewMemory(cfg Config) (Memory, error) {
	switch cfg.Type {
	case BackendSQLite:
		if cfg.Path == "" {
			return nil, fmt.Errorf("sqlite backend requires path")
		}
		return NewSQLiteStore(cfg.Path)
	case BackendMemory:
		return NewInMemoryStore(), nil
	default:
		return nil, fmt.Errorf("unsupported backend type: %s", cfg.Type)
	}
}

// InMemoryStore 内存版 memory store。
// perf-v6 round 8 Task 2：新增倒排索引 ftsIndex（term → episode ID 集合），
// Search 走索引而非全表扫描。
type InMemoryStore struct {
	mu       sync.RWMutex
	episodes map[string]*Episode
	// ftsIndex 倒排索引：lowercased token → episode ID 集合
	// 同步策略：与 episodes 在同一把 mu 下读写，保证一致性。
	ftsIndex map[string]map[string]struct{}
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		episodes: make(map[string]*Episode),
		ftsIndex: make(map[string]map[string]struct{}),
	}
}

// addToIndex 把 episode 的 content + summary + topics 的所有 token 加入倒排索引
// 调用方须持有 s.mu 写锁
func (s *InMemoryStore) addToIndex(ep *Episode) {
	for _, tok := range indexTokens(ep.Content, ep.Summary, ep.Topics) {
		postings, ok := s.ftsIndex[tok]
		if !ok {
			postings = make(map[string]struct{}, 4)
			s.ftsIndex[tok] = postings
		}
		postings[ep.ID] = struct{}{}
	}
}

// removeFromIndex 从倒排索引移除 episode 的所有 token
// 调用方须持有 s.mu 写锁
func (s *InMemoryStore) removeFromIndex(ep *Episode) {
	for _, tok := range indexTokens(ep.Content, ep.Summary, ep.Topics) {
		postings, ok := s.ftsIndex[tok]
		if !ok {
			continue
		}
		delete(postings, ep.ID)
		if len(postings) == 0 {
			delete(s.ftsIndex, tok)
		}
	}
}

// indexTokens 返回 text 字段集合的 token 集合（lowercased）
// 复用 sqlite.go 的 tokenizeRe，行为与 SQLite FTS5 一致
func indexTokens(fields ...string) []string {
	combined := strings.Join(fields, " ")
	if combined == "" {
		return nil
	}
	parts := tokenizeRe.Split(strings.ToLower(combined), -1)
	out := parts[:0]
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (s *InMemoryStore) Add(ctx context.Context, episode *Episode) error {
	if err := episode.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// 若 ID 已存在，先移除旧索引（保证更新语义正确）
	if old, ok := s.episodes[episode.ID]; ok {
		s.removeFromIndex(old)
	}
	s.episodes[episode.ID] = episode
	s.addToIndex(episode)
	return nil
}

// AddBatch 批量添加 episodes（perf-v6 round 5 Task 3）
// 单次加锁，避免 N 次 lock/unlock 开销
// 返回第一个失败的 episode 的索引和错误
func (s *InMemoryStore) AddBatch(ctx context.Context, episodes []*Episode) error {
	if len(episodes) == 0 {
		return nil
	}

	// 第一阶段：先做所有 validation（锁外）
	for i, ep := range episodes {
		if ep == nil {
			return fmt.Errorf("episode at index %d is nil", i)
		}
		if err := ep.Validate(); err != nil {
			return fmt.Errorf("episode at index %d: %w", i, err)
		}
	}

	// 第二阶段：单次锁写入
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ep := range episodes {
		if old, ok := s.episodes[ep.ID]; ok {
			s.removeFromIndex(old)
		}
		s.episodes[ep.ID] = ep
		s.addToIndex(ep)
	}
	return nil
}

// GetBatch 批量获取 episodes（perf-v6 round 5 Task 3）
// 单次 RLock + 一次 map 遍历，N 次 lock 优化为 1 次
// 返回 ID → Episode 映射（不存在的 ID 不在结果中）
func (s *InMemoryStore) GetBatch(ctx context.Context, ids []string) (map[string]*Episode, error) {
	if len(ids) == 0 {
		return map[string]*Episode{}, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]*Episode, len(ids))
	for _, id := range ids {
		if ep, ok := s.episodes[id]; ok {
			result[id] = ep
		}
	}
	return result, nil
}

// DeleteBatch 批量删除 episodes（perf-v6 round 5 Task 3）
func (s *InMemoryStore) DeleteBatch(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range ids {
		if ep, ok := s.episodes[id]; ok {
			s.removeFromIndex(ep)
			delete(s.episodes, id)
		}
	}
	return nil
}

// Search 走倒排索引（perf-v6 round 8 Task 2）
//
// 语义：
//   - empty query → 返回符合 session/role filter 的所有 episodes
//   - non-empty query → tokenize 后取倒排索引 postings 集合的交集
//     （AND 语义：所有 token 都必须出现）。这一步是 O(|tokens| × |postings|)，
//     对典型 1-3 个 token 的查询远快于 O(N) 扫描。
//
// 与原 substring 语义相比的差异：
//   - 多词查询 "go language" 现在要求两个词都出现（AND），原实现是子串匹配
//   - 这是 FTS 的标准行为，与 SQLite FTS5 默认模式一致
func (s *InMemoryStore) Search(ctx context.Context, query string, opts *SearchOptions) ([]*Episode, error) {
	if opts == nil {
		opts = &SearchOptions{Limit: defaultSearchLimit}
	}
	if opts.Limit <= 0 {
		opts.Limit = defaultSearchLimit
	}

	// Tokenize query（lowercased）— 与 sqlite.go tokenizeRe 行为一致
	queryTokens := uniqueTokens(query)
	hasQuery := len(queryTokens) > 0

	s.mu.RLock()
	defer s.mu.RUnlock()

	// 1) 收集候选 ID
	var candidateIDs []string
	if hasQuery {
		// 走倒排索引：取所有 token 的 postings 交集
		candidateIDs = intersectPostings(s.ftsIndex, queryTokens)
	} else {
		// 无 query：返回全部 ID（受 session/role filter 约束）
		candidateIDs = make([]string, 0, len(s.episodes))
		for id := range s.episodes {
			candidateIDs = append(candidateIDs, id)
		}
	}

	// 2) 应用 session/role filter + 收集结果
	results := make([]*Episode, 0, min(len(candidateIDs), opts.Limit))
	for _, id := range candidateIDs {
		ep, ok := s.episodes[id]
		if !ok {
			continue
		}
		if opts.SessionID != "" && ep.SessionID != opts.SessionID {
			continue
		}
		if opts.RoleFilter != "" && ep.Role != opts.RoleFilter {
			continue
		}
		// 注意：倒排索引已是 FTS-style 语义（token 级别 AND 匹配），
		// 不再做 substring 兜底。FTS 标准行为是：所有 token 出现即命中，
		// 词之间可以间隔其他内容。比原 substring 匹配更宽松（更符合
		// 用户对"搜索"的直觉）。
		results = append(results, ep)
		if len(results) >= opts.Limit {
			break
		}
	}
	return results, nil
}

// uniqueTokens 把 query 拆成唯一的 lowercased token 集合
func uniqueTokens(query string) []string {
	parts := tokenizeRe.Split(strings.ToLower(query), -1)
	seen := make(map[string]struct{}, len(parts))
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

// intersectPostings 倒排索引的 AND 交集
// 返回所有 token 都出现过的 episode ID 列表
func intersectPostings(idx map[string]map[string]struct{}, tokens []string) []string {
	if len(tokens) == 0 {
		return nil
	}
	// 取最小 postings 集作为起点（性能优化）
	var minPostings map[string]struct{}
	for _, tok := range tokens {
		postings, ok := idx[tok]
		if !ok {
			// 任一 token 不存在 → 交集为空
			return nil
		}
		if minPostings == nil || len(postings) < len(minPostings) {
			minPostings = postings
		}
	}
	if minPostings == nil {
		return nil
	}
	out := make([]string, 0, len(minPostings))
	for id := range minPostings {
		// 校验其他 token 也包含此 ID
		hit := true
		for _, tok := range tokens {
			postings := idx[tok]
			if _, ok := postings[id]; !ok {
				hit = false
				break
			}
		}
		if hit {
			out = append(out, id)
		}
	}
	return out
}

// substringMatch 兜底：检查 query 是否作为子串出现在 episode 的 content/summary
// 行为与原 InMemoryStore.Search phase 1 一致（大小写敏感快速检查）
func substringMatch(ep *Episode, query string) bool {
	if query == "" {
		return true
	}
	return strings.Contains(ep.Content, query) || strings.Contains(ep.Summary, query)
}

// substringMatchCI 兜底：case-insensitive substring 校验（用于倒排索引后）
// query 必须是已经 ToLower 过的
func substringMatchCI(ep *Episode, lowerQuery string) bool {
	if lowerQuery == "" {
		return true
	}
	return strings.Contains(strings.ToLower(ep.Content), lowerQuery) ||
		strings.Contains(strings.ToLower(ep.Summary), lowerQuery)
}

func (s *InMemoryStore) SearchAdvanced(ctx context.Context, opts SearchOptions) ([]*SearchResult, error) {
	episodes, err := s.Search(ctx, opts.Query, &opts)
	if err != nil {
		return nil, err
	}
	results := make([]*SearchResult, len(episodes))
	for i, e := range episodes {
		results[i] = &SearchResult{
			Episode:       e,
			KeywordScore:  1.0,
			SemanticScore: 0.0,
			CombinedScore: 1.0,
		}
	}
	return results, nil
}

func (s *InMemoryStore) Get(ctx context.Context, id string) (*Episode, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ep, ok := s.episodes[id]
	if !ok {
		return nil, ErrEpisodeNotFound
	}
	return ep, nil
}

func (s *InMemoryStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ep, ok := s.episodes[id]; ok {
		s.removeFromIndex(ep)
	}
	delete(s.episodes, id)
	return nil
}

func (s *InMemoryStore) Count(ctx context.Context, sessionID string) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if sessionID == "" {
		return int64(len(s.episodes)), nil
	}
	var count int64
	for _, e := range s.episodes {
		if e.SessionID == sessionID {
			count++
		}
	}
	return count, nil
}

func (s *InMemoryStore) List(ctx context.Context, opts *ListOptions) ([]*Episode, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if opts == nil {
		opts = &ListOptions{}
	}
	var results []*Episode
	for _, e := range s.episodes {
		if opts.SessionID != "" && e.SessionID != opts.SessionID {
			continue
		}
		results = append(results, e)
	}
	if opts.Limit > 0 && len(results) > opts.Limit {
		results = results[:opts.Limit]
	}
	return results, nil
}

// UpdateSummary 更新 summary/topics 时同步刷新倒排索引（perf-v6 round 8 Task 2）
func (s *InMemoryStore) UpdateSummary(ctx context.Context, id string, summary, topics string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ep, ok := s.episodes[id]
	if !ok {
		return ErrEpisodeNotFound
	}
	s.removeFromIndex(ep)
	ep.Summary = summary
	ep.Topics = topics
	s.addToIndex(ep)
	return nil
}

func (s *InMemoryStore) SetImportance(ctx context.Context, episodeID string, importance float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ep, ok := s.episodes[episodeID]
	if !ok {
		return ErrEpisodeNotFound
	}
	ep.Importance = importance
	return nil
}

func (s *InMemoryStore) SearchByTag(ctx context.Context, tag string, opts *SearchOptions) ([]*Episode, error) {
	// In-memory search by tag is limited, we can check content/topics
	return s.Search(ctx, tag, opts)
}

func (s *InMemoryStore) GetImportant(ctx context.Context, threshold float64, limit int) ([]*Episode, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var results []*Episode
	for _, e := range s.episodes {
		if e.Importance >= threshold {
			results = append(results, e)
		}
	}
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func (s *InMemoryStore) GetTimeline(ctx context.Context, days int) (map[string][]*Episode, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	timeline := make(map[string][]*Episode)
	for _, e := range s.episodes {
		date := e.CreatedAt
		if len(date) >= 10 {
			date = date[:10]
		}
		timeline[date] = append(timeline[date], e)
	}
	return timeline, nil
}

func (s *InMemoryStore) GetMemoriesByTag(ctx context.Context, tag string, limit int) ([]*Episode, error) {
	return s.Search(ctx, tag, &SearchOptions{Limit: limit})
}

func (s *InMemoryStore) GetMemoriesBySession(ctx context.Context, sessionID string) ([]*Episode, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var results []*Episode
	for _, e := range s.episodes {
		if e.SessionID == sessionID {
			results = append(results, e)
		}
	}
	return results, nil
}

func (s *InMemoryStore) GetImportantMemories(ctx context.Context, threshold float64, limit int) ([]*Episode, error) {
	return s.GetImportant(ctx, threshold, limit)
}

func (s *InMemoryStore) GetMemoryTimeline(ctx context.Context, days int) ([]*MemoryTimelineGroup, error) {
	timelineMap, err := s.GetTimeline(ctx, days)
	if err != nil {
		return nil, err
	}
	groups := make([]*MemoryTimelineGroup, 0, len(timelineMap))
	for date, eps := range timelineMap {
		groups = append(groups, &MemoryTimelineGroup{
			Date:     date,
			Episodes: eps,
			Count:    len(eps),
		})
	}
	return groups, nil
}

func (s *InMemoryStore) CleanupExpired(ctx context.Context, maxAgeDays int) (int64, error) {
	if maxAgeDays <= 0 {
		return 0, nil
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -maxAgeDays)
	s.mu.Lock()
	defer s.mu.Unlock()
	var deleted int64
	for id, ep := range s.episodes {
		if ep.CreatedAt == "" {
			continue
		}
		createdAt, err := time.Parse(time.RFC3339, ep.CreatedAt)
		if err != nil {
			continue
		}
		if createdAt.Before(cutoff) {
			s.removeFromIndex(ep)
			delete(s.episodes, id)
			deleted++
		}
	}
	return deleted, nil
}

func (s *InMemoryStore) Stats(ctx context.Context) (*MemoryStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return &MemoryStats{
		TotalEpisodes: int64(len(s.episodes)),
		TotalSessions: 1,
	}, nil
}

func (s *InMemoryStore) RecordToolUse(ctx context.Context, sessionID, agentName, toolName, args, result string) error {
	return nil
}

// ClearAll 清空记忆（同时清空倒排索引，perf-v6 round 8 Task 2）
func (s *InMemoryStore) ClearAll(ctx context.Context, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sessionID == "" {
		s.episodes = make(map[string]*Episode)
		s.ftsIndex = make(map[string]map[string]struct{})
	} else {
		for id, e := range s.episodes {
			if e.SessionID == sessionID {
				s.removeFromIndex(e)
				delete(s.episodes, id)
			}
		}
	}
	return nil
}

func (s *InMemoryStore) ExportMemories(ctx context.Context, sessionID, format string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	episodes := make([]*Episode, 0, len(s.episodes))
	for _, ep := range s.episodes {
		if sessionID != "" && ep.SessionID != sessionID {
			continue
		}
		episodes = append(episodes, ep)
	}
	switch format {
	case "markdown", "md":
		return exportMarkdown(episodes)
	default:
		return json.Marshal(episodes)
	}
}

func (s *InMemoryStore) ImportMemories(ctx context.Context, data []byte, format string) (int, error) {
	var episodes []*Episode
	if err := json.Unmarshal(data, &episodes); err != nil {
		return 0, fmt.Errorf("import: unmarshal json: %w", err)
	}
	if len(episodes) == 0 {
		return 0, nil
	}
	// 使用 AddBatch 保证单次锁写入
	if err := s.AddBatch(ctx, episodes); err != nil {
		return 0, err
	}
	return len(episodes), nil
}

func (s *InMemoryStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.episodes = nil
	s.ftsIndex = nil
	return nil
}

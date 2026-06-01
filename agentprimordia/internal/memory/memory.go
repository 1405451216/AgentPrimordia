package memory

import (
	"context"
	"fmt"
	"strings"
	"sync"
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

type InMemoryStore struct {
	mu       sync.RWMutex
	episodes map[string]*Episode
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		episodes: make(map[string]*Episode),
	}
}

func (s *InMemoryStore) Add(ctx context.Context, episode *Episode) error {
	if err := episode.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.episodes[episode.ID] = episode
	return nil
}

func (s *InMemoryStore) Search(ctx context.Context, query string, opts *SearchOptions) ([]*Episode, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if opts == nil {
		opts = &SearchOptions{Limit: defaultSearchLimit}
	}
	if opts.Limit <= 0 {
		opts.Limit = defaultSearchLimit
	}
	var results []*Episode
	for _, e := range s.episodes {
		if opts.SessionID != "" && e.SessionID != opts.SessionID {
			continue
		}
		if opts.RoleFilter != "" && e.Role != opts.RoleFilter {
			continue
		}
		if query != "" {
			lowerQuery := strings.ToLower(query)
			if !strings.Contains(strings.ToLower(e.Content), lowerQuery) &&
				!strings.Contains(strings.ToLower(e.Summary), lowerQuery) {
				continue
			}
		}
		results = append(results, e)
	}
	if len(results) > opts.Limit {
		results = results[:opts.Limit]
	}
	return results, nil
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

func (s *InMemoryStore) UpdateSummary(ctx context.Context, id string, summary, topics string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ep, ok := s.episodes[id]
	if !ok {
		return ErrEpisodeNotFound
	}
	ep.Summary = summary
	ep.Topics = topics
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
	// 简化版实现，不处理时间过期
	return 0, nil
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

func (s *InMemoryStore) ClearAll(ctx context.Context, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sessionID == "" {
		s.episodes = make(map[string]*Episode)
	} else {
		for id, e := range s.episodes {
			if e.SessionID == sessionID {
				delete(s.episodes, id)
			}
		}
	}
	return nil
}

func (s *InMemoryStore) ExportMemories(ctx context.Context, sessionID, format string) ([]byte, error) {
	return nil, fmt.Errorf("export not implemented for memory backend")
}

func (s *InMemoryStore) ImportMemories(ctx context.Context, data []byte, format string) (int, error) {
	return 0, fmt.Errorf("import not implemented for memory backend")
}

func (s *InMemoryStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.episodes = nil
	return nil
}

// sqlite_search.go — SQLite 存储的搜索与查询路径
//   - Search / SearchAdvanced / searchFTS5Candidates
//   - normalizeFTS5Rank / tokenize / computeSemanticScore(Precomputed) / sortSearchResults
//   - sanitizeFTSQuery / escapeLike
//   - SearchByTag / GetImportant / GetTimeline
//   - GetMemoriesByTag / GetMemoriesBySession / GetImportantMemories / GetMemoryTimeline
//   - CleanupExpired
package memory

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
)

// Search 在 FTS5 全文索引上执行关键字搜索
func (s *SQLiteStore) Search(ctx context.Context, query string, opts *SearchOptions) ([]*Episode, error) {
	if s.closed.Load() {
		return nil, ErrStoreClosed
	}
	if opts == nil {
		opts = &SearchOptions{Limit: defaultSearchLimit}
	}
	if opts.Limit <= 0 {
		opts.Limit = defaultSearchLimit
	}

	cleanedQuery := sanitizeFTSQuery(query)

	conditions := []string{}
	args := []any{cleanedQuery}

	if opts.SessionID != "" {
		conditions = append(conditions, "e.session_id = ?")
		args = append(args, opts.SessionID)
	}
	if opts.RoleFilter != "" {
		conditions = append(conditions, "e.role = ?")
		args = append(args, opts.RoleFilter)
	}

	whereExtra := ""
	if len(conditions) > 0 {
		whereExtra = " AND " + strings.Join(conditions, " AND ")
	}

	sqlQuery := fmt.Sprintf(`
		SELECT e.id, e.session_id, e.role, e.content, e.summary, e.topics, e.importance, e.metadata, e.created_at
		FROM episodes e
		JOIN episodes_fts fts ON e.rowid = fts.rowid
		WHERE episodes_fts MATCH ?%s
		ORDER BY rank
		LIMIT ? OFFSET ?
	`, whereExtra)

	args = append(args, opts.Limit, opts.Offset)

	rows, err := s.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("search episodes: %w", err)
	}
	defer rows.Close()

	return s.scanEpisodes(rows)
}

// SearchAdvanced 在 FTS5 候选上做关键词+语义融合打分，返回 SearchResult 列表
func (s *SQLiteStore) SearchAdvanced(ctx context.Context, opts SearchOptions) ([]*SearchResult, error) {
	if s.closed.Load() {
		return nil, ErrStoreClosed
	}
	if opts.MaxResults <= 0 {
		opts.MaxResults = defaultSearchLimit
	}
	if opts.SemanticWeight < 0 {
		opts.SemanticWeight = 0
	}
	if opts.SemanticWeight > 1 {
		opts.SemanticWeight = 1
	}

	candidates, err := s.searchFTS5Candidates(ctx, opts)
	if err != nil {
		return nil, err
	}

	results := make([]*SearchResult, 0, len(candidates))
	queryTokens := tokenize(opts.Query)

	maxRank := 0.0
	for _, cand := range candidates {
		if cand.rawRank < maxRank {
			maxRank = cand.rawRank
		}
	}

	for _, cand := range candidates {
		keywordScore := normalizeFTS5Rank(cand.rawRank, maxRank)
		semanticScore := 0.0
		if opts.UseSemantic {
			// 优化（perf-v3）：使用预计算的 contentTokens，避免重复 tokenize
			semanticScore = computeSemanticScorePrecomputed(queryTokens, cand.contentTokens, cand.episode.Importance)
		}
		combinedScore := (1-opts.SemanticWeight)*keywordScore + opts.SemanticWeight*semanticScore

		if combinedScore < opts.MinScore {
			continue
		}

		results = append(results, &SearchResult{
			Episode:       cand.episode,
			KeywordScore:  keywordScore,
			SemanticScore: semanticScore,
			CombinedScore: combinedScore,
		})
	}

	sortSearchResults(results)
	if len(results) > opts.MaxResults {
		results = results[:opts.MaxResults]
	}
	return results, nil
}

// ftsCandidate 是 FTS5 召回的候选 Episode（含原始 rank 与预计算 token）
type ftsCandidate struct {
	episode *Episode
	rawRank float64
	// 优化（perf-v3）：预计算的内容 token 集合，避免 scoring 时重复 tokenize
	contentTokens map[string]struct{}
}

// searchFTS5Candidates 在 FTS5 召回候选 Episode（带预计算 token）
func (s *SQLiteStore) searchFTS5Candidates(ctx context.Context, opts SearchOptions) ([]*ftsCandidate, error) {
	if s.closed.Load() {
		return nil, ErrStoreClosed
	}
	cleanedQuery := sanitizeFTSQuery(opts.Query)
	if cleanedQuery == "" {
		return nil, nil
	}

	limit := opts.MaxResults * ftsResultMultiplier
	if limit < ftsMinLimit {
		limit = ftsMinLimit
	}

	conditions := []string{}
	args := []any{cleanedQuery}

	if opts.SessionID != "" {
		conditions = append(conditions, "e.session_id = ?")
		args = append(args, opts.SessionID)
	}
	if opts.RoleFilter != "" {
		conditions = append(conditions, "e.role = ?")
		args = append(args, opts.RoleFilter)
	}
	if len(opts.Tags) > 0 {
		tagConditions := make([]string, 0, len(opts.Tags))
		for _, tag := range opts.Tags {
			tagConditions = append(tagConditions, "e.topics LIKE ? ESCAPE '\\'")
			args = append(args, "%"+escapeLike(tag)+"%")
		}
		conditions = append(conditions, "("+strings.Join(tagConditions, " OR ")+")")
	}

	whereExtra := ""
	if len(conditions) > 0 {
		whereExtra = " AND " + strings.Join(conditions, " AND ")
	}

	sqlQuery := fmt.Sprintf(`
		SELECT e.id, e.session_id, e.role, e.content, e.summary, e.topics, e.importance, e.metadata, e.created_at,
			rank
		FROM episodes e
		JOIN episodes_fts fts ON e.rowid = fts.rowid
		WHERE episodes_fts MATCH ?%s
		ORDER BY rank
		LIMIT ?
	`, whereExtra)

	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("search fts5 candidates: %w", err)
	}
	defer rows.Close()

	var candidates []*ftsCandidate
	// 优化（perf-v3）：语义打分时预计算 token，避免每个候选重复 tokenize
	preTokenize := opts.UseSemantic
	for rows.Next() {
		var ep Episode
		var metadataJSON sql.NullString
		var rawRank sql.NullFloat64
		err := rows.Scan(
			&ep.ID, &ep.SessionID, &ep.Role, &ep.Content, &ep.Summary,
			&ep.Topics, &ep.Importance, &metadataJSON, &ep.CreatedAt,
			&rawRank,
		)
		if err != nil {
			return nil, fmt.Errorf("scan candidate row: %w", err)
		}
		if metadataJSON.Valid && metadataJSON.String != "" {
			if err := json.Unmarshal([]byte(metadataJSON.String), &ep.Metadata); err != nil {
				s.logger.Warn("元数据反序列化失败", "error", err, "id", ep.ID)
			}
		}
		rank := 0.0
		if rawRank.Valid {
			rank = rawRank.Float64
		}
		cand := &ftsCandidate{episode: &ep, rawRank: rank}
		if preTokenize {
			cand.contentTokens = tokenize(ep.Content + " " + ep.Summary + " " + ep.Topics)
		}
		candidates = append(candidates, cand)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate candidate rows: %w", err)
	}
	return candidates, nil
}

// normalizeFTS5Rank 把 FTS5 rank 归一化到 [0, 1]
func normalizeFTS5Rank(rank, maxRank float64) float64 {
	// FTS5 rank 为负数（越相关越负），maxRank 是最负值
	// rank==0 表示无匹配
	if rank == 0 || maxRank == 0 {
		return 0.0
	}
	// |rank| / |maxRank| 归一化到 [0, 1]，最相关的结果得 1.0
	score := math.Abs(rank) / math.Abs(maxRank)
	return math.Min(1.0, score)
}

// tokenize 将文本切分为 token 集合（按 Unicode letter/number 切分）
func tokenize(text string) map[string]struct{} {
	tokens := make(map[string]struct{})
	parts := tokenizeRe.Split(strings.ToLower(text), -1)
	for _, p := range parts {
		if p != "" {
			tokens[p] = struct{}{}
		}
	}
	return tokens
}

// computeSemanticScorePrecomputed 使用预计算的 content token 集合计算语义分数
// 优化（perf-v3）：避免搜索路径中重复 tokenize episode 内容
func computeSemanticScorePrecomputed(queryTokens, contentTokens map[string]struct{}, importance float64) float64 {
	if len(contentTokens) == 0 || len(queryTokens) == 0 {
		return 0
	}
	intersection := 0
	for tok := range queryTokens {
		if _, ok := contentTokens[tok]; ok {
			intersection++
		}
	}
	union := len(queryTokens) + len(contentTokens) - intersection
	if union == 0 {
		return 0
	}
	jaccard := float64(intersection) / float64(union)
	importanceWeight := 1.0 + importance
	return jaccard * importanceWeight
}

// sortSearchResults 按 CombinedScore 降序排序
func sortSearchResults(results []*SearchResult) {
	// 优化（Task 19）：使用泛型 slices.SortFunc 替代 sort.Slice，避免反射开销
	slices.SortFunc(results, func(a, b *SearchResult) int { return cmp.Compare(b.CombinedScore, a.CombinedScore) })
}

// sanitizeFTSQuery 清洗 FTS5 全文搜索查询字符串
// 移除可能导致语法错误的特殊字符和关键字
func sanitizeFTSQuery(query string) string {
	query = ftsSanitizeRe.ReplaceAllString(query, "")
	query = ftsKeywordRe.ReplaceAllString(query, "")
	return strings.TrimSpace(query)
}

// escapeLike 转义 SQL LIKE 通配符，防止用户输入中的 % 和 _ 被误解释
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "%", "\\%")
	s = strings.ReplaceAll(s, "_", "\\_")
	return s
}

// SearchByTag 按标签模糊搜索 Episode
func (s *SQLiteStore) SearchByTag(ctx context.Context, tag string, opts *SearchOptions) ([]*Episode, error) {
	if s.closed.Load() {
		return nil, ErrStoreClosed
	}
	if opts == nil {
		opts = &SearchOptions{Limit: defaultSearchLimit}
	}
	if opts.Limit <= 0 {
		opts.Limit = defaultSearchLimit
	}

	conditions := []string{"e.topics LIKE ? ESCAPE '\\'"}
	args := []any{"%" + escapeLike(tag) + "%"}

	if opts.SessionID != "" {
		conditions = append(conditions, "e.session_id = ?")
		args = append(args, opts.SessionID)
	}
	if opts.RoleFilter != "" {
		conditions = append(conditions, "e.role = ?")
		args = append(args, opts.RoleFilter)
	}

	whereClause := strings.Join(conditions, " AND ")

	sqlQuery := fmt.Sprintf(`
		SELECT e.id, e.session_id, e.role, e.content, e.summary, e.topics, e.importance, e.metadata, e.created_at
		FROM episodes e
		WHERE %s
		ORDER BY e.created_at DESC
		LIMIT ? OFFSET ?
	`, whereClause)

	args = append(args, opts.Limit, opts.Offset)

	rows, err := s.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("search by tag: %w", err)
	}
	defer rows.Close()

	return s.scanEpisodes(rows)
}

// GetImportant 按重要性阈值查询 Episode
func (s *SQLiteStore) GetImportant(ctx context.Context, threshold float64, limit int) ([]*Episode, error) {
	if s.closed.Load() {
		return nil, ErrStoreClosed
	}
	if limit <= 0 {
		limit = defaultSearchLimit
	}

	query := `
		SELECT id, session_id, role, content, summary, topics, importance, metadata, created_at
		FROM episodes
		WHERE importance >= ?
		ORDER BY importance DESC, created_at DESC
		LIMIT ?
	`

	rows, err := s.db.QueryContext(ctx, query, threshold, limit)
	if err != nil {
		return nil, fmt.Errorf("get important episodes: %w", err)
	}
	defer rows.Close()

	return s.scanEpisodes(rows)
}

// GetTimeline 返回最近 N 天 Episode 的按日期分组时间线
func (s *SQLiteStore) GetTimeline(ctx context.Context, days int) (map[string][]*Episode, error) {
	if s.closed.Load() {
		return nil, ErrStoreClosed
	}
	if days <= 0 {
		days = 30
	}

	query := `
		SELECT id, session_id, role, content, summary, topics, importance, metadata, created_at
		FROM episodes
		WHERE created_at >= datetime('now', '-' || ? || ' days')
		ORDER BY created_at DESC
	`

	rows, err := s.db.QueryContext(ctx, query, days)
	if err != nil {
		return nil, fmt.Errorf("get timeline: %w", err)
	}
	defer rows.Close()

	timeline := make(map[string][]*Episode)
	for rows.Next() {
		ep, err := scanRows(rows)
		if err != nil {
			return nil, err
		}
		date := ep.CreatedAt
		if len(date) >= 10 {
			date = date[:10]
		}
		timeline[date] = append(timeline[date], ep)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate timeline rows: %w", err)
	}

	return timeline, nil
}

// GetMemoriesByTag 按标签模糊查询（GetImportant 风格）
func (s *SQLiteStore) GetMemoriesByTag(ctx context.Context, tag string, limit int) ([]*Episode, error) {
	if s.closed.Load() {
		return nil, ErrStoreClosed
	}
	if limit <= 0 {
		limit = defaultSearchLimit
	}

	query := `
		SELECT id, session_id, role, content, summary, topics, importance, metadata, created_at
		FROM episodes
		WHERE topics LIKE ? ESCAPE '\'
		ORDER BY created_at DESC
		LIMIT ?
	`

	rows, err := s.db.QueryContext(ctx, query, "%"+escapeLike(tag)+"%", limit)
	if err != nil {
		return nil, fmt.Errorf("get memories by tag: %w", err)
	}
	defer rows.Close()

	return s.scanEpisodes(rows)
}

// GetMemoriesBySession 按会话 ID 查询所有 Episode（按时间升序）
func (s *SQLiteStore) GetMemoriesBySession(ctx context.Context, sessionID string) ([]*Episode, error) {
	if s.closed.Load() {
		return nil, ErrStoreClosed
	}
	query := `
		SELECT id, session_id, role, content, summary, topics, importance, metadata, created_at
		FROM episodes
		WHERE session_id = ?
		ORDER BY created_at ASC
	`

	rows, err := s.db.QueryContext(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("get memories by session: %w", err)
	}
	defer rows.Close()

	return s.scanEpisodes(rows)
}

// GetImportantMemories 是 GetImportant 的别名（pkg API 命名风格）
func (s *SQLiteStore) GetImportantMemories(ctx context.Context, threshold float64, limit int) ([]*Episode, error) {
	if limit <= 0 {
		limit = defaultSearchLimit
	}

	query := `
		SELECT id, session_id, role, content, summary, topics, importance, metadata, created_at
		FROM episodes
		WHERE importance >= ?
		ORDER BY importance DESC, created_at DESC
		LIMIT ?
	`

	rows, err := s.db.QueryContext(ctx, query, threshold, limit)
	if err != nil {
		return nil, fmt.Errorf("get important memories: %w", err)
	}
	defer rows.Close()

	return s.scanEpisodes(rows)
}

// GetMemoryTimeline 返回带日期排序的 MemoryTimelineGroup 列表
func (s *SQLiteStore) GetMemoryTimeline(ctx context.Context, days int) ([]*MemoryTimelineGroup, error) {
	if s.closed.Load() {
		return nil, ErrStoreClosed
	}
	if days <= 0 {
		days = 30
	}

	cutoff := time.Now().AddDate(0, 0, -days).UTC().Format(time.RFC3339)

	query := `
		SELECT id, session_id, role, content, summary, topics, importance, metadata, created_at
		FROM episodes
		WHERE created_at >= ?
		ORDER BY created_at ASC
	`

	rows, err := s.db.QueryContext(ctx, query, cutoff)
	if err != nil {
		return nil, fmt.Errorf("get memory timeline: %w", err)
	}
	defer rows.Close()

	groupMap := make(map[string][]*Episode)
	var dateOrder []string

	for rows.Next() {
		ep, err := scanRows(rows)
		if err != nil {
			return nil, err
		}
		date := ep.CreatedAt
		if len(date) >= 10 {
			date = date[:10]
		}
		if _, exists := groupMap[date]; !exists {
			dateOrder = append(dateOrder, date)
		}
		groupMap[date] = append(groupMap[date], ep)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate memory timeline rows: %w", err)
	}

	timeline := make([]*MemoryTimelineGroup, 0, len(dateOrder))
	for _, date := range dateOrder {
		episodes := groupMap[date]
		timeline = append(timeline, &MemoryTimelineGroup{
			Date:     date,
			Episodes: episodes,
			Count:    len(episodes),
		})
	}
	return timeline, nil
}

// CleanupExpired 删除创建时间超过 maxAgeDays 天的 Episode
func (s *SQLiteStore) CleanupExpired(ctx context.Context, maxAgeDays int) (int64, error) {
	if s.closed.Load() {
		return 0, ErrStoreClosed
	}
	if maxAgeDays <= 0 {
		return 0, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	query := `DELETE FROM episodes WHERE created_at < datetime('now', '-' || ? || ' days')`
	result, err := s.db.ExecContext(ctx, query, maxAgeDays)
	if err != nil {
		return 0, fmt.Errorf("cleanup expired: %w", err)
	}

	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("cleanup expired: %w", err)
	}
	return deleted, nil
}

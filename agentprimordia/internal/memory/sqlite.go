package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const (
	sqliteMaxOpenConns       = 10
	sqliteMaxIdleConns       = 5
	ftsResultMultiplier      = 3
	ftsMinLimit              = 30
	defaultToolUseImportance = 0.3
	maxExportLimit           = 100000
)

var (
	ErrEpisodeNotFound = errors.New("episode not found")

	ftsSanitizeRe = regexp.MustCompile(`["*(){}^:]`)
	ftsKeywordRe  = regexp.MustCompile(`\b(AND|OR|NOT|NEAR)\b`)
	tokenizeRe    = regexp.MustCompile(`[^\p{L}\p{N}]+`)
)

type SQLiteStore struct {
	db     *sql.DB
	path   string
	mu     sync.RWMutex
	logger *slog.Logger
}

func NewSQLiteStore(path string) (*SQLiteStore, error) {
	// 对 :memory: 数据库使用 cache=shared 模式，确保多连接共享同一实例
	openPath := path
	isInMemory := path == ":memory:"
	if isInMemory {
		openPath = "file::memory:?cache=shared"
	}

	db, err := sql.Open("sqlite", openPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	store := &SQLiteStore{
		db:     db,
		path:   path,
		logger: slog.Default(),
	}

	// :memory: 数据库使用 cache=shared 允许多连接并发读
	// 文件数据库使用 WAL 模式允许多连接并发吞吐
	if isInMemory {
		db.SetMaxOpenConns(2)
		db.SetMaxIdleConns(2)
	} else {
		db.SetMaxOpenConns(sqliteMaxOpenConns)
		db.SetMaxIdleConns(sqliteMaxIdleConns)
	}

	if err := store.initSchema(); err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

func WithInMemory() (*SQLiteStore, error) {
	return NewSQLiteStore(":memory:")
}

func (s *SQLiteStore) initSchema() error {
	pragmas := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA busy_timeout=5000;",
		"PRAGMA synchronous=NORMAL;",
	}
	for _, pragma := range pragmas {
		if _, err := s.db.Exec(pragma); err != nil {
			return fmt.Errorf("set pragma %s: %w", pragma, err)
		}
	}

	schema := `
CREATE TABLE IF NOT EXISTS episodes (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    role TEXT NOT NULL,
    content TEXT NOT NULL,
    summary TEXT,
    topics TEXT DEFAULT '',
    importance REAL DEFAULT 0,
    metadata TEXT,
    created_at TEXT NOT NULL
);

CREATE VIRTUAL TABLE IF NOT EXISTS episodes_fts USING fts5(
    content,
    summary,
    topics,
    content=episodes,
    content_rowid=rowid
);

CREATE TRIGGER IF NOT EXISTS episodes_ai AFTER INSERT ON episodes BEGIN
    INSERT INTO episodes_fts(rowid, content, summary, topics) VALUES (new.rowid, new.content, new.summary, new.topics);
END;

CREATE TRIGGER IF NOT EXISTS episodes_ad AFTER DELETE ON episodes BEGIN
    INSERT INTO episodes_fts(episodes_fts, rowid, content, summary, topics) VALUES('delete', old.rowid, old.content, old.summary, old.topics);
END;

CREATE TRIGGER IF NOT EXISTS episodes_au AFTER UPDATE ON episodes BEGIN
    INSERT INTO episodes_fts(episodes_fts, rowid, content, summary, topics) VALUES('delete', old.rowid, old.content, old.summary, old.topics);
    INSERT INTO episodes_fts(rowid, content, summary, topics) VALUES (new.rowid, new.content, new.summary, new.topics);
END;

CREATE INDEX IF NOT EXISTS idx_episodes_session ON episodes(session_id);
CREATE INDEX IF NOT EXISTS idx_episodes_created ON episodes(created_at);
CREATE INDEX IF NOT EXISTS idx_episodes_role ON episodes(role);
CREATE INDEX IF NOT EXISTS idx_episodes_topics ON episodes(topics);
CREATE INDEX IF NOT EXISTS idx_episodes_importance ON episodes(importance);
`
	_, err := s.db.Exec(schema)
	return err
}

func (s *SQLiteStore) Add(ctx context.Context, episode *Episode) error {
	if err := episode.Validate(); err != nil {
		return err
	}

	var metadataJSON []byte
	if len(episode.Metadata) > 0 {
		var err error
		metadataJSON, err = json.Marshal(episode.Metadata)
		if err != nil {
			return fmt.Errorf("marshal metadata: %w", err)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	query := `INSERT INTO episodes (id, session_id, role, content, summary, topics, importance, metadata, created_at)
	          VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := s.db.ExecContext(ctx, query,
		episode.ID,
		episode.SessionID,
		episode.Role,
		episode.Content,
		episode.Summary,
		episode.Topics,
		episode.Importance,
		string(metadataJSON),
		episode.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert episode: %w", err)
	}
	return nil
}

func (s *SQLiteStore) Search(ctx context.Context, query string, opts *SearchOptions) ([]*Episode, error) {
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

	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("search episodes: %w", err)
	}
	defer rows.Close()

	return s.scanEpisodes(rows)
}

func (s *SQLiteStore) SearchAdvanced(ctx context.Context, opts SearchOptions) ([]*SearchResult, error) {
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
			semanticScore = computeSemanticScore(queryTokens, cand.episode)
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

type ftsCandidate struct {
	episode *Episode
	rawRank float64
}

func (s *SQLiteStore) searchFTS5Candidates(ctx context.Context, opts SearchOptions) ([]*ftsCandidate, error) {
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

	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("search fts5 candidates: %w", err)
	}
	defer rows.Close()

	var candidates []*ftsCandidate
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
		candidates = append(candidates, &ftsCandidate{episode: &ep, rawRank: rank})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate candidate rows: %w", err)
	}
	return candidates, nil
}

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

func computeSemanticScore(queryTokens map[string]struct{}, ep *Episode) float64 {
	contentTokens := tokenize(ep.Content + " " + ep.Summary + " " + ep.Topics)
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
	importanceWeight := 1.0 + ep.Importance
	return jaccard * importanceWeight
}

func sortSearchResults(results []*SearchResult) {
	sort.Slice(results, func(i, j int) bool {
		return results[i].CombinedScore > results[j].CombinedScore
	})
}

func (s *SQLiteStore) Get(ctx context.Context, id string) (*Episode, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `SELECT id, session_id, role, content, summary, topics, importance, metadata, created_at FROM episodes WHERE id = ?`
	row := s.db.QueryRowContext(ctx, query, id)

	ep, err := scanEpisode(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("%w: %s", ErrEpisodeNotFound, id)
		}
		return nil, err
	}
	return ep, nil
}

func (s *SQLiteStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.ExecContext(ctx, "DELETE FROM episodes WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete episode: %w", err)
	}
	if _, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("delete episode: %w", err)
	}
	return nil
}

func (s *SQLiteStore) Count(ctx context.Context, sessionID string) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var query string
	var args []any
	if sessionID != "" {
		query = "SELECT COUNT(*) FROM episodes WHERE session_id = ?"
		args = []any{sessionID}
	} else {
		query = "SELECT COUNT(*) FROM episodes"
	}

	var count int64
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count episodes: %w", err)
	}
	return count, nil
}

func (s *SQLiteStore) List(ctx context.Context, opts *ListOptions) ([]*Episode, error) {
	if opts == nil {
		opts = &ListOptions{Limit: defaultSearchLimit}
	}
	if opts.Limit <= 0 {
		opts.Limit = defaultSearchLimit
	}

	conditions := []string{}
	args := []any{}

	if opts.SessionID != "" {
		conditions = append(conditions, "session_id = ?")
		args = append(args, opts.SessionID)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	orderCol := "created_at"
	if opts.OrderBy == "id" {
		orderCol = "id"
	}
	orderDir := "DESC"
	if opts.Ascending {
		orderDir = "ASC"
	}

	sqlQuery := fmt.Sprintf(
		`SELECT id, session_id, role, content, summary, topics, importance, metadata, created_at
		 FROM episodes %s ORDER BY %s %s LIMIT ? OFFSET ?`,
		whereClause, orderCol, orderDir,
	)
	args = append(args, opts.Limit, opts.Offset)

	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("list episodes: %w", err)
	}
	defer rows.Close()

	return s.scanEpisodes(rows)
}

func (s *SQLiteStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		// 对文件数据库执行 WAL checkpoint，确保 -wal/-shm 文件释放
		// 避免在 Windows 上因文件锁导致临时目录清理失败
		if s.path != ":memory:" {
			_, _ = s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
		}
		err := s.db.Close()
		s.db = nil
		return err
	}
	return nil
}

func (s *SQLiteStore) UpdateSummary(ctx context.Context, id string, summary, topics string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `UPDATE episodes SET summary = ?, topics = ? WHERE id = ?`
	result, err := s.db.ExecContext(ctx, query, summary, topics, id)
	if err != nil {
		return fmt.Errorf("update summary: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update summary: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("%w: %s", ErrEpisodeNotFound, id)
	}
	return nil
}

func (s *SQLiteStore) SetImportance(ctx context.Context, episodeID string, importance float64) error {
	if importance < 0 || importance > 1 {
		return ErrInvalidImportance
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	query := `UPDATE episodes SET importance = ? WHERE id = ?`
	result, err := s.db.ExecContext(ctx, query, importance, episodeID)
	if err != nil {
		return fmt.Errorf("set importance: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("set importance: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("%w: %s", ErrEpisodeNotFound, episodeID)
	}
	return nil
}

// escapeLike 转义 SQL LIKE 通配符，防止用户输入中的 % 和 _ 被误解释
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "%", "\\%")
	s = strings.ReplaceAll(s, "_", "\\_")
	return s
}

func (s *SQLiteStore) SearchByTag(ctx context.Context, tag string, opts *SearchOptions) ([]*Episode, error) {
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

	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("search by tag: %w", err)
	}
	defer rows.Close()

	return s.scanEpisodes(rows)
}

func (s *SQLiteStore) GetImportant(ctx context.Context, threshold float64, limit int) ([]*Episode, error) {
	if limit <= 0 {
		limit = defaultSearchLimit
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

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

func (s *SQLiteStore) GetTimeline(ctx context.Context, days int) (map[string][]*Episode, error) {
	if days <= 0 {
		days = 30
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

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

func (s *SQLiteStore) GetMemoriesByTag(ctx context.Context, tag string, limit int) ([]*Episode, error) {
	if limit <= 0 {
		limit = defaultSearchLimit
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

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

func (s *SQLiteStore) GetMemoriesBySession(ctx context.Context, sessionID string) ([]*Episode, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

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

func (s *SQLiteStore) GetImportantMemories(ctx context.Context, threshold float64, limit int) ([]*Episode, error) {
	if limit <= 0 {
		limit = defaultSearchLimit
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

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

func (s *SQLiteStore) GetMemoryTimeline(ctx context.Context, days int) ([]*MemoryTimelineGroup, error) {
	if days <= 0 {
		days = 30
	}

	cutoff := time.Now().AddDate(0, 0, -days).UTC().Format(time.RFC3339)

	s.mu.RLock()
	defer s.mu.RUnlock()

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

func (s *SQLiteStore) CleanupExpired(ctx context.Context, maxAgeDays int) (int64, error) {
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

func (s *SQLiteStore) Stats(ctx context.Context) (*MemoryStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := &MemoryStats{}

	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM episodes").Scan(&stats.TotalEpisodes)
	if err != nil {
		return nil, fmt.Errorf("count episodes: %w", err)
	}

	err = s.db.QueryRowContext(ctx, "SELECT COUNT(DISTINCT session_id) FROM episodes").Scan(&stats.TotalSessions)
	if err != nil {
		return nil, fmt.Errorf("count sessions: %w", err)
	}

	var oldest, newest sql.NullString
	err = s.db.QueryRowContext(ctx, "SELECT MIN(created_at), MAX(created_at) FROM episodes").Scan(&oldest, &newest)
	if err != nil {
		return nil, fmt.Errorf("get date range: %w", err)
	}
	if oldest.Valid {
		stats.OldestEpisode = oldest.String
	}
	if newest.Valid {
		stats.NewestEpisode = newest.String
	}

	if stats.TotalSessions > 0 {
		stats.AvgEpisodesPerSession = float64(stats.TotalEpisodes) / float64(stats.TotalSessions)
	}

	return stats, nil
}

func (s *SQLiteStore) scanEpisodes(rows *sql.Rows) ([]*Episode, error) {
	var episodes []*Episode
	for rows.Next() {
		ep, err := scanRows(rows)
		if err != nil {
			return nil, err
		}
		episodes = append(episodes, ep)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}
	return episodes, nil
}

func scanRows(rows *sql.Rows) (*Episode, error) {
	var ep Episode
	var metadataJSON sql.NullString

	err := rows.Scan(&ep.ID, &ep.SessionID, &ep.Role, &ep.Content, &ep.Summary, &ep.Topics, &ep.Importance, &metadataJSON, &ep.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan episode row: %w", err)
	}

	if metadataJSON.Valid && metadataJSON.String != "" {
		if err := json.Unmarshal([]byte(metadataJSON.String), &ep.Metadata); err != nil {
			return nil, fmt.Errorf("unmarshal metadata: %w", err)
		}
	}

	return &ep, nil
}

// RecordToolUse 记录工具调用（特殊记忆类型）
func (s *SQLiteStore) RecordToolUse(ctx context.Context, sessionID, agentName, toolName, args, result string) error {
	ep, err := NewEpisode(sessionID, "tool_use", args)
	if err != nil {
		return fmt.Errorf("create tool use episode: %w", err)
	}
	ep.Metadata = map[string]string{
		"tool_name":  toolName,
		"result":     result,
		"agent_name": agentName,
	}
	ep.Topics = toolName
	ep.Importance = defaultToolUseImportance
	return s.Add(ctx, ep)
}

// ClearAll 清空记忆（可选按会话）
func (s *SQLiteStore) ClearAll(ctx context.Context, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sessionID != "" {
		_, err := s.db.ExecContext(ctx, "DELETE FROM episodes WHERE session_id = ?", sessionID)
		if err != nil {
			return fmt.Errorf("clear episodes by session: %w", err)
		}
	} else {
		_, err := s.db.ExecContext(ctx, "DELETE FROM episodes")
		if err != nil {
			return fmt.Errorf("clear all episodes: %w", err)
		}
	}
	return nil
}

// ExportMemories 导出记忆为指定格式
func (s *SQLiteStore) ExportMemories(ctx context.Context, sessionID, format string) ([]byte, error) {
	var opts *ListOptions
	if sessionID != "" {
		opts = &ListOptions{SessionID: sessionID, Limit: maxExportLimit, Offset: 0}
	} else {
		opts = &ListOptions{Limit: maxExportLimit, Offset: 0}
	}

	episodes, err := s.List(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("query episodes for export: %w", err)
	}

	switch format {
	case "markdown", "md":
		return exportMarkdown(episodes)
	default:
		return exportJSON(episodes)
	}
}

// ImportMemories 从数据导入记忆
func (s *SQLiteStore) ImportMemories(ctx context.Context, data []byte, format string) (int, error) {
	switch format {
	case "json":
		return s.importJSON(ctx, data)
	default:
		return s.importJSON(ctx, data)
	}
}

func exportJSON(episodes []*Episode) ([]byte, error) {
	data, err := json.Marshal(episodes)
	if err != nil {
		return nil, fmt.Errorf("marshal episodes to json: %w", err)
	}
	return data, nil
}

func exportMarkdown(episodes []*Episode) ([]byte, error) {
	var b strings.Builder
	b.WriteString("# 记忆导出\n\n")
	for _, ep := range episodes {
		b.WriteString(fmt.Sprintf("## %s\n", ep.Role))
		b.WriteString(fmt.Sprintf("- **ID**: %s\n", ep.ID))
		b.WriteString(fmt.Sprintf("- **会话**: %s\n", ep.SessionID))
		b.WriteString(fmt.Sprintf("- **时间**: %s\n", ep.CreatedAt))
		if ep.Topics != "" {
			b.WriteString(fmt.Sprintf("- **标签**: %s\n", ep.Topics))
		}
		if ep.Importance > 0 {
			b.WriteString(fmt.Sprintf("- **重要性**: %.2f\n", ep.Importance))
		}
		b.WriteString(fmt.Sprintf("\n%s\n\n", ep.Content))
		if ep.Summary != "" {
			b.WriteString(fmt.Sprintf("> 摘要: %s\n\n", ep.Summary))
		}
	}
	return []byte(b.String()), nil
}

func (s *SQLiteStore) importJSON(ctx context.Context, data []byte) (int, error) {
	var episodes []*Episode
	if err := json.Unmarshal(data, &episodes); err != nil {
		return 0, fmt.Errorf("unmarshal episodes from json: %w", err)
	}

	count := 0
	for _, ep := range episodes {
		if err := s.Add(ctx, ep); err != nil {
			return count, fmt.Errorf("import episode %s: %w", ep.ID, err)
		}
		count++
	}
	return count, nil
}

// sanitizeFTSQuery 清洗 FTS5 全文搜索查询字符串
// 移除可能导致语法错误的特殊字符和关键字
func sanitizeFTSQuery(query string) string {
	query = ftsSanitizeRe.ReplaceAllString(query, "")
	query = ftsKeywordRe.ReplaceAllString(query, "")
	return strings.TrimSpace(query)
}

func scanEpisode(row *sql.Row) (*Episode, error) {
	var ep Episode
	var metadataJSON sql.NullString

	err := row.Scan(&ep.ID, &ep.SessionID, &ep.Role, &ep.Content, &ep.Summary, &ep.Topics, &ep.Importance, &metadataJSON, &ep.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan episode: %w", err)
	}

	if metadataJSON.Valid && metadataJSON.String != "" {
		if err := json.Unmarshal([]byte(metadataJSON.String), &ep.Metadata); err != nil {
			return nil, fmt.Errorf("unmarshal metadata: %w", err)
		}
	}

	return &ep, nil
}

// sqlite.go — SQLite 存储基础：类型 + 构造 + schema + 关闭 + 扫描器
//
// 1004 LoC 拆分（Phase 7 优化）：
//   - sqlite.go         本文件 — 类型/常量/正则/构造/initSchema/Close/Stats + 扫描器
//   - sqlite_crud.go    — Add/BatchAdd/Get/Delete/Count/List/Update/RecordToolUse/ClearAll
//   - sqlite_search.go  — Search + FTS5 + 各种 Get*/SearchBy*/CleanupExpired
//   - sqlite_export.go  — ExportMemories/ImportMemories + JSON/Markdown 序列化
package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"sync"

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

// SQLiteStore 是基于 SQLite + FTS5 的记忆存储实现。
type SQLiteStore struct {
	db     *sql.DB
	path   string
	mu     sync.RWMutex
	logger *slog.Logger
}

// NewSQLiteStore 创建或打开一个 SQLite 存储实例
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
	// 连接数限制为 2 以控制 :memory: 数据库的并发线程数
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

// WithInMemory 创建一个内存 SQLite 存储（用于测试与临时场景）
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

// Close 关闭底层数据库连接
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

// Stats 返回记忆库统计信息
func (s *SQLiteStore) Stats(ctx context.Context) (*MemoryStats, error) {
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

// scanEpisodes 扫描多行 Episodes 结果
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

// scanRows 从 sql.Rows 中扫描 Episode 行
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

// scanEpisode 从 sql.Row 中扫描单行 Episode
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

// failure_sqlite.go — 基于 SQLite 的 FailureStore 持久化实现（v4.1 真实接线）
//
// 与 MemoryFailureStore 接口等价，但记录持久化到 SQLite 文件，
// 重启后失败记录与可重放检查点不丢失。使用白名单内 modernc sqlite 驱动。
package persist

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// ErrFailureStoreClosed 表示在 Close 之后访问已关闭的失败存储。
var ErrFailureStoreClosed = errors.New("failure store closed")

// SQLiteFailureStore 基于 modernc sqlite 的 FailureStore（持久化）。
type SQLiteFailureStore struct {
	mu     sync.RWMutex
	db     *sql.DB
	closed bool
}

// NewSQLiteFailureStore 创建 SQLite 失败存储（dsn 为 SQLite 文件路径）。
func NewSQLiteFailureStore(dsn string) (*SQLiteFailureStore, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open failure db: %w", err)
	}
	s := &SQLiteFailureStore{db: db}
	if err := s.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("init failure schema: %w", err)
	}
	return s, nil
}

func (s *SQLiteFailureStore) initSchema() error {
	// v4.9-1 性能优化：WAL + 降级同步 → 写入延迟大幅下降（fsync 次数减少）
	if _, err := s.db.Exec(`PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL;`); err != nil {
		return fmt.Errorf("enable wal: %w", err)
	}
	schema := `
	CREATE TABLE IF NOT EXISTS failures (
		id TEXT PRIMARY KEY,
		agent_id TEXT NOT NULL,
		session_id TEXT NOT NULL DEFAULT '',
		phase TEXT NOT NULL DEFAULT '',
		error TEXT NOT NULL,
		turn INTEGER NOT NULL DEFAULT 0,
		subtask_id TEXT NOT NULL DEFAULT '',
		input TEXT NOT NULL DEFAULT '',
		state TEXT,
		created_at TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_failures_agent ON failures(agent_id, created_at);
	`
	_, err := s.db.Exec(schema)
	return err
}

// Close 关闭底层数据库；之后所有方法返回 ErrFailureStoreClosed。
func (s *SQLiteFailureStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return s.db.Close()
}

func (s *SQLiteFailureStore) checkOpen() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return ErrFailureStoreClosed
	}
	return nil
}

// Record 保存失败记录（同 ID 覆盖；要求 ID 非空）。
func (s *SQLiteFailureStore) Record(ctx context.Context, rec *FailureRecord) error {
	if rec == nil || rec.ID == "" {
		return fmt.Errorf("failure record requires non-empty id")
	}
	// 写操作串行化，避免并发写 SQLite 文件锁冲突（SQLITE_BUSY）
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrFailureStoreClosed
	}

	var stateJSON []byte
	var err error
	if rec.State != nil {
		stateJSON, err = rec.State.Marshal()
		if err != nil {
			return fmt.Errorf("marshal failure state: %w", err)
		}
	}
	createdAt := rec.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO failures (id, agent_id, session_id, phase, error, turn, subtask_id, input, state, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			agent_id=excluded.agent_id, session_id=excluded.session_id, phase=excluded.phase,
			error=excluded.error, turn=excluded.turn, subtask_id=excluded.subtask_id,
			input=excluded.input, state=excluded.state, created_at=excluded.created_at`,
		rec.ID, rec.AgentID, rec.SessionID, rec.Phase, rec.Error, rec.Turn, rec.SubTaskID,
		rec.Input, string(stateJSON), createdAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("record failure: %w", err)
	}
	return nil
}

// Get 按 ID 读取失败记录（含可重放检查点）。
func (s *SQLiteFailureStore) Get(ctx context.Context, id string) (*FailureRecord, error) {
	if err := s.checkOpen(); err != nil {
		return nil, err
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT id, agent_id, session_id, phase, error, turn, subtask_id, input, state, created_at
		FROM failures WHERE id = ?`, id)
	rec, err := scanFailure(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("failure record %q not found", id)
	}
	if err != nil {
		return nil, err
	}
	return rec, nil
}

// List 列出失败记录（新→旧，created_at 倒序）；agentID 为空时返回全部。
func (s *SQLiteFailureStore) List(ctx context.Context, agentID string) ([]*FailureRecord, error) {
	if err := s.checkOpen(); err != nil {
		return nil, err
	}
	var rows *sql.Rows
	var err error
	if agentID == "" {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, agent_id, session_id, phase, error, turn, subtask_id, input, state, created_at
			FROM failures ORDER BY created_at DESC, rowid DESC`)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, agent_id, session_id, phase, error, turn, subtask_id, input, state, created_at
			FROM failures WHERE agent_id = ? ORDER BY created_at DESC, rowid DESC`, agentID)
	}
	if err != nil {
		return nil, fmt.Errorf("list failures: %w", err)
	}
	defer rows.Close()

	out := make([]*FailureRecord, 0)
	for rows.Next() {
		rec, err := scanFailure(rows)
		if err != nil {
			return nil, fmt.Errorf("scan failure: %w", err)
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// Delete 删除失败记录（不存在时报错，与 MemoryFailureStore 一致）。
func (s *SQLiteFailureStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrFailureStoreClosed
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM failures WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete failure: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("failure record %q not found", id)
	}
	return nil
}

// rowScanner 兼容 *sql.Row 与 *sql.Rows 的扫描接口。
type rowScanner interface {
	Scan(dest ...any) error
}

// scanFailure 从一行扫描出 FailureRecord（state 为 AgentState JSON）。
func scanFailure(sc rowScanner) (*FailureRecord, error) {
	var rec FailureRecord
	var sessionID, phase, subtaskID, input, stateJSON, createdAt string
	if err := sc.Scan(&rec.ID, &rec.AgentID, &sessionID, &phase, &rec.Error, &rec.Turn,
		&subtaskID, &input, &stateJSON, &createdAt); err != nil {
		return nil, err
	}
	rec.SessionID = sessionID
	rec.Phase = phase
	rec.SubTaskID = subtaskID
	rec.Input = input

	if stateJSON != "" {
		st, err := UnmarshalAgentState([]byte(stateJSON))
		if err != nil {
			return nil, fmt.Errorf("unmarshal failure state: %w", err)
		}
		rec.State = st
	}
	ts, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse failure created_at: %w", err)
	}
	rec.CreatedAt = ts
	return &rec, nil
}

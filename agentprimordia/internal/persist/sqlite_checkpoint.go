package persist

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

var (
	ErrCheckpointNotFound = errors.New("checkpoint not found")
)

type SQLiteCheckpointStore struct {
	mu sync.RWMutex
	db *sql.DB
}

func NewSQLiteCheckpointStore(dsn string) (*SQLiteCheckpointStore, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open checkpoint db: %w", err)
	}

	store := &SQLiteCheckpointStore{db: db}
	if err := store.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}

	return store, nil
}

func InMemoryCheckpointStore() (*SQLiteCheckpointStore, error) {
	return NewSQLiteCheckpointStore("file::memory:?cache=shared")
}

func (s *SQLiteCheckpointStore) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS checkpoints (
		agent_id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		status TEXT NOT NULL,
		messages TEXT NOT NULL,
		turn_count INTEGER NOT NULL,
		metrics TEXT NOT NULL,
		saved_at TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_checkpoints_session ON checkpoints(session_id);
	`
	_, err := s.db.Exec(schema)
	return err
}

func (s *SQLiteCheckpointStore) Save(ctx context.Context, state *AgentState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	messagesJSON, err := json.Marshal(state.Messages)
	if err != nil {
		return fmt.Errorf("marshal messages: %w", err)
	}

	metricsJSON, err := json.Marshal(state.Metrics)
	if err != nil {
		return fmt.Errorf("marshal metrics: %w", err)
	}

	query := `
	INSERT OR REPLACE INTO checkpoints (agent_id, session_id, status, messages, turn_count, metrics, saved_at)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	_, err = s.db.ExecContext(ctx, query,
		state.AgentID,
		state.SessionID,
		state.Status,
		string(messagesJSON),
		state.TurnCount,
		string(metricsJSON),
		state.SavedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("save checkpoint: %w", err)
	}

	return nil
}

func (s *SQLiteCheckpointStore) Load(ctx context.Context, agentID string) (*AgentState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `SELECT agent_id, session_id, status, messages, turn_count, metrics, saved_at FROM checkpoints WHERE agent_id = ?`
	row := s.db.QueryRowContext(ctx, query, agentID)

	var state AgentState
	var messagesJSON, metricsJSON, savedAt string

	err := row.Scan(&state.AgentID, &state.SessionID, &state.Status, &messagesJSON, &state.TurnCount, &metricsJSON, &savedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("%w: %s", ErrCheckpointNotFound, agentID)
		}
		return nil, fmt.Errorf("load checkpoint: %w", err)
	}

	if err := json.Unmarshal([]byte(messagesJSON), &state.Messages); err != nil {
		return nil, fmt.Errorf("unmarshal messages: %w", err)
	}
	if err := json.Unmarshal([]byte(metricsJSON), &state.Metrics); err != nil {
		return nil, fmt.Errorf("unmarshal metrics: %w", err)
	}

	if t, err := timeParse(savedAt); err != nil {
		slog.Warn("解析保存时间失败", "error", err, "saved_at", savedAt)
	} else {
		state.SavedAt = t
	}
	return &state, nil
}

func (s *SQLiteCheckpointStore) List(ctx context.Context, sessionID string) ([]*AgentState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `SELECT agent_id, session_id, status, messages, turn_count, metrics, saved_at FROM checkpoints WHERE session_id = ? ORDER BY saved_at DESC`
	rows, err := s.db.QueryContext(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list checkpoints: %w", err)
	}
	defer rows.Close()

	var results []*AgentState
	for rows.Next() {
		var state AgentState
		var messagesJSON, metricsJSON, savedAt string

		if err := rows.Scan(&state.AgentID, &state.SessionID, &state.Status, &messagesJSON, &state.TurnCount, &metricsJSON, &savedAt); err != nil {
			return nil, fmt.Errorf("scan checkpoint: %w", err)
		}

		if err := json.Unmarshal([]byte(messagesJSON), &state.Messages); err != nil {
			return nil, fmt.Errorf("unmarshal messages: %w", err)
		}
		if err := json.Unmarshal([]byte(metricsJSON), &state.Metrics); err != nil {
			return nil, fmt.Errorf("unmarshal metrics: %w", err)
		}
		if t, err := timeParse(savedAt); err != nil {
			slog.Warn("解析保存时间失败", "error", err, "saved_at", savedAt)
		} else {
			state.SavedAt = t
		}

		results = append(results, &state)
	}

	return results, nil
}

func (s *SQLiteCheckpointStore) Delete(ctx context.Context, agentID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.ExecContext(ctx, "DELETE FROM checkpoints WHERE agent_id = ?", agentID)
	if err != nil {
		return fmt.Errorf("delete checkpoint: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("%w: %s", ErrCheckpointNotFound, agentID)
	}

	return nil
}

func (s *SQLiteCheckpointStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

func timeParse(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, s)
}

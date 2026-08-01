// sqlite_crud.go — SQLite 存储的 CRUD（增删改查）操作
//   - Add / BatchAdd / AddBatch / GetBatch / DeleteBatch
//   - Get / Delete / Count / List
//   - UpdateSummary / SetImportance / RecordToolUse / ClearAll
package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// Add 插入单条 Episode
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

// BatchAdd 批量插入多个 episode，使用单一事务包裹所有 INSERT。
// 优化（Task 4）：在高吞吐场景（如 Pool 多 Agent 并发写入）下，批量事务比逐条
// INSERT 减少 fsync 次数并降低全局互斥锁的串行化开销。
func (s *SQLiteStore) BatchAdd(ctx context.Context, episodes []*Episode) error {
	if len(episodes) == 0 {
		return nil
	}
	// 预验证所有 episodes
	for i, ep := range episodes {
		if ep == nil {
			return fmt.Errorf("episode at index %d is nil", i)
		}
		if err := ep.Validate(); err != nil {
			return fmt.Errorf("validate episode %d: %w", i, err)
		}
	}

	// 预序列化 metadata
	type prepared struct {
		ep           *Episode
		metadataJSON []byte
	}
	prepareds := make([]prepared, len(episodes))
	for i, ep := range episodes {
		var metadataJSON []byte
		if len(ep.Metadata) > 0 {
			data, err := json.Marshal(ep.Metadata)
			if err != nil {
				return fmt.Errorf("marshal metadata for episode %d: %w", i, err)
			}
			metadataJSON = data
		}
		prepareds[i] = prepared{ep: ep, metadataJSON: metadataJSON}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin batch tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	stmt, err := tx.PrepareContext(ctx, `INSERT INTO episodes (id, session_id, role, content, summary, topics, importance, metadata, created_at)
	          VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare batch insert: %w", err)
	}
	defer stmt.Close()

	for _, p := range prepareds {
		_, err := stmt.ExecContext(ctx,
			p.ep.ID,
			p.ep.SessionID,
			p.ep.Role,
			p.ep.Content,
			p.ep.Summary,
			p.ep.Topics,
			p.ep.Importance,
			string(p.metadataJSON),
			p.ep.CreatedAt,
		)
		if err != nil {
			return fmt.Errorf("batch insert episode %s: %w", p.ep.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit batch tx: %w", err)
	}
	committed = true
	return nil
}

// AddBatch 是 BatchAdd 的别名（perf-v6 round 5 Task 3：统一接口名）
func (s *SQLiteStore) AddBatch(ctx context.Context, episodes []*Episode) error {
	return s.BatchAdd(ctx, episodes)
}

// GetBatch 批量获取（perf-v6 round 5 Task 3）
// 单次查询使用 IN(?,?,?) 优化
func (s *SQLiteStore) GetBatch(ctx context.Context, ids []string) (map[string]*Episode, error) {
	if len(ids) == 0 {
		return map[string]*Episode{}, nil
	}

	// 预构建 IN 子句的占位符
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := `SELECT id, session_id, role, content, summary, topics, importance, metadata, created_at
		FROM episodes WHERE id IN (` + strings.Join(placeholders, ",") + `)`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]*Episode, len(ids))
	for rows.Next() {
		ep, err := scanRows(rows)
		if err != nil {
			return nil, err
		}
		result[ep.ID] = ep
	}
	return result, rows.Err()
}

// DeleteBatch 批量删除（perf-v6 round 5 Task 3）
// 单次 DELETE 使用 IN 子句
func (s *SQLiteStore) DeleteBatch(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	query := `DELETE FROM episodes WHERE id IN (` + strings.Join(placeholders, ",") + `)`
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

// Get 按 ID 查询单条 Episode
func (s *SQLiteStore) Get(ctx context.Context, id string) (*Episode, error) {
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

// Delete 按 ID 删除单条 Episode
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

// Count 统计 Episode 数量（可选按 session 过滤）
func (s *SQLiteStore) Count(ctx context.Context, sessionID string) (int64, error) {
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

// List 列出 Episode，支持分页、排序、按 session 过滤
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

	rows, err := s.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("list episodes: %w", err)
	}
	defer rows.Close()

	return s.scanEpisodes(rows)
}

// UpdateSummary 更新指定 Episode 的摘要与标签
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

// SetImportance 设置指定 Episode 的重要性（0-1）
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

// RecordToolUse 记录tool调用（特殊记忆类型）
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

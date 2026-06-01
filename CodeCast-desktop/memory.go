package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type MemoryStore struct {
	db     *sql.DB
	dbPath string
}

func NewMemoryStore(dbPath string) (*MemoryStore, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("创建记忆数据库目录失败: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("打开记忆数据库失败: %w", err)
	}

	db.SetMaxOpenConns(1)

	ms := &MemoryStore{db: db, dbPath: dbPath}
	if err := ms.initTables(); err != nil {
		db.Close()
		return nil, fmt.Errorf("初始化记忆表失败: %w", err)
	}

	return ms, nil
}

func (m *MemoryStore) initTables() error {
	_, err := m.db.Exec(`
		CREATE TABLE IF NOT EXISTS episodes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL,
			timestamp INTEGER NOT NULL,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			summary TEXT,
			topics TEXT,
			importance REAL DEFAULT 0.5
		);

		CREATE VIRTUAL TABLE IF NOT EXISTS episodes_fts USING fts5(
			content, summary, topics,
			content=episodes, content_rowid=id
		);

		CREATE TRIGGER IF NOT EXISTS episodes_ai AFTER INSERT ON episodes BEGIN
			INSERT INTO episodes_fts(rowid, content, summary, topics)
			VALUES (new.id, new.content, new.summary, new.topics);
		END;

		CREATE TRIGGER IF NOT EXISTS episodes_ad AFTER DELETE ON episodes BEGIN
			INSERT INTO episodes_fts(episodes_fts, rowid, content, summary, topics)
			VALUES('delete', old.id, old.content, old.summary, old.topics);
		END;

		CREATE TRIGGER IF NOT EXISTS episodes_au AFTER UPDATE ON episodes BEGIN
			INSERT INTO episodes_fts(episodes_fts, rowid, content, summary, topics)
			VALUES('delete', old.id, old.content, old.summary, old.topics);
			INSERT INTO episodes_fts(rowid, content, summary, topics)
			VALUES (new.id, new.content, new.summary, new.topics);
		END;

		CREATE INDEX IF NOT EXISTS idx_episodes_session_id ON episodes(session_id);
		CREATE INDEX IF NOT EXISTS idx_episodes_timestamp ON episodes(timestamp);
	`)
	return err
}

func (m *MemoryStore) SaveEpisode(sessionID, role, content string) (int64, error) {
	result, err := m.db.Exec(
		"INSERT INTO episodes (session_id, timestamp, role, content) VALUES (?, ?, ?, ?)",
		sessionID, time.Now().Unix(), role, content,
	)
	if err != nil {
		return 0, fmt.Errorf("保存对话记录失败: %w", err)
	}
	return result.LastInsertId()
}

func (m *MemoryStore) UpdateEpisodeSummary(episodeID int64, summary, topics string) error {
	_, err := m.db.Exec(
		"UPDATE episodes SET summary = ?, topics = ? WHERE id = ?",
		summary, topics, episodeID,
	)
	if err != nil {
		return fmt.Errorf("更新摘要失败: %w", err)
	}
	return nil
}

func (m *MemoryStore) RecallEpisodes(query string, limit int) (string, error) {
	if strings.TrimSpace(query) == "" {
		return "", nil
	}

	// Sanitize FTS5 query: escape quotes and strip potentially dangerous operators/wildcards
	sanitized := strings.TrimSpace(query)
	sanitized = strings.ReplaceAll(sanitized, `"`, `""`)
	sanitized = strings.ReplaceAll(sanitized, `*`, ``)
	sanitized = strings.ReplaceAll(sanitized, `(`, ``)
	sanitized = strings.ReplaceAll(sanitized, `)`, ``)
	searchQuery := `"` + sanitized + `"`
	rows, err := m.db.Query(`
		SELECT e.timestamp, e.summary, e.content, e.topics, rank
		FROM episodes e
		JOIN episodes_fts fts ON e.id = fts.rowid
		WHERE episodes_fts MATCH ?
		ORDER BY rank
		LIMIT ?
	`, searchQuery, limit)
	if err != nil {
		return "", fmt.Errorf("检索历史记忆失败: %w", err)
	}
	defer rows.Close()

	var memories []string
	for rows.Next() {
		var timestamp int64
		var summary, content, topics sql.NullString
		var rank float64

		if err := rows.Scan(&timestamp, &summary, &content, &topics, &rank); err != nil {
			continue
		}

		t := time.Unix(timestamp, 0).Format("2006-01-02")
		displayContent := ""
		if summary.Valid && summary.String != "" {
			displayContent = summary.String
		} else if content.Valid && len(content.String) > 100 {
			displayContent = content.String[:100] + "..."
		} else if content.Valid {
			displayContent = content.String
		}

		if displayContent != "" {
			entry := fmt.Sprintf("- [%s] %s", t, displayContent)
			if topics.Valid && topics.String != "" {
				entry += fmt.Sprintf(" (标签: %s)", topics.String)
			}
			memories = append(memories, entry)
		}
	}

	if len(memories) == 0 {
		return "", nil
	}

	result := "以下是与你当前任务相关的历史信息：\n" + strings.Join(memories, "\n")
	return result, nil
}

func (m *MemoryStore) GetRecentEpisodes(limit int) ([]string, error) {
	rows, err := m.db.Query(`
		SELECT timestamp, COALESCE(summary, substr(content, 1, 100)), topics
		FROM episodes
		ORDER BY timestamp DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("查询最近记忆失败: %w", err)
	}
	defer rows.Close()

	var memories []string
	for rows.Next() {
		var timestamp int64
		var content, topics sql.NullString

		if err := rows.Scan(&timestamp, &content, &topics); err != nil {
			continue
		}

		t := time.Unix(timestamp, 0).Format("2006-01-02")
		entry := fmt.Sprintf("- [%s] %s", t, content.String)
		if topics.Valid && topics.String != "" {
			entry += fmt.Sprintf(" (标签: %s)", topics.String)
		}
		memories = append(memories, entry)
	}

	return memories, nil
}

func (m *MemoryStore) ClearAll() error {
	_, err := m.db.Exec("DELETE FROM episodes")
	if err != nil {
		return fmt.Errorf("清空记忆数据失败: %w", err)
	}
	return nil
}

// RecordToolUse 记录一次工具操作到情景记忆（ToolMemory 功能）
// toolName: 工具名称 (如 "ReadFile", "WriteFile", "SearchFiles")
// detail: 操作详情 (如 "读取了 src/main.go", "写入了 utils.ts")
func (m *MemoryStore) RecordToolUse(sessionID, toolName, detail string) error {
	content := fmt.Sprintf("[工具调用] %s: %s", toolName, detail)
	_, err := m.db.Exec(
		"INSERT INTO episodes (session_id, timestamp, role, content, topics, importance) VALUES (?, ?, ?, ?, ?, ?)",
		sessionID, time.Now().Unix(), "tool", content, toolName, 0.3,
	)
	if err != nil {
		return fmt.Errorf("记录工具操作失败: %w", err)
	}
	return nil
}

func (m *MemoryStore) Stats() (totalEpisodes int, dbSize int64, err error) {
	err = m.db.QueryRow("SELECT COUNT(*) FROM episodes").Scan(&totalEpisodes)
	if err != nil {
		return 0, 0, fmt.Errorf("查询记忆统计失败: %w", err)
	}

 fileInfo, statErr := os.Stat(m.dbPath)
	if statErr == nil {
		dbSize = fileInfo.Size()
	}

	return totalEpisodes, dbSize, nil
}

func (m *MemoryStore) Close() error {
	if m.db != nil {
		return m.db.Close()
	}
	return nil
}

// ==================== 自动清理 ====================

const (
	memoryMaxAgeDays    = 30 // 记忆保留天数
	memoryCleanupInterval = 24 * time.Hour // 清理间隔：每天一次
)

// CleanupExpired 删除超过保留天数的旧记忆，返回删除的条数
func (m *MemoryStore) CleanupExpired() (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -memoryMaxAgeDays).Unix()
	result, err := m.db.Exec("DELETE FROM episodes WHERE timestamp < ? AND role != 'tool'", cutoff)
	if err != nil {
		return 0, fmt.Errorf("清理过期记忆失败: %w", err)
	}
	deleted, _ := result.RowsAffected()
	if deleted > 0 {
		fmt.Printf("[Memory] 已清理 %d 条过期记忆（超过 %d 天）\n", deleted, memoryMaxAgeDays)
	}
	return deleted, nil
}

// StartAutoCleanup 启动后台定期清理 goroutine
// stopCh: 外部关闭信号；返回清理停止 channel
func StartAutoCleanup(store *MemoryStore, stopCh <-chan struct{}) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(memoryCleanupInterval)
		defer ticker.Stop()
		defer close(done)

		for {
			select {
			case <-ticker.C:
				if store != nil {
					store.CleanupExpired()
				}
			case <-stopCh:
				return
			}
		}
	}()
	return done
}

func (a *App) ExtractSummaryAsync(sessionID string, userContent, assistantContent string) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("[Memory] 摘要提取异常恢复: %v\n", r)
			}
		}()

		a.mu.Lock()
		creds, credErr := a.resolveCredentialsLocked("")
		a.mu.Unlock()

		if credErr != nil {
			return
		}
		apiKey := creds.APIKey
		apiURL := creds.APIURL
		modelName := creds.Model

		promptText := fmt.Sprintf(`请用一句话概括以下对话的核心内容，并提取关键词标签。

用户输入：%s

AI回复：%s

请严格按照以下格式输出（不要添加其他内容）：
摘要内容 | 标签1,标签2,标签3`, userContent, assistantContent)

		messages := []Message{
			{Role: "system", Content: "你是一个专业的对话摘要助手。你的任务是用简洁的语言总结对话内容并提取关键标签。只输出要求的格式，不要多余解释。"},
			{Role: "user", Content: promptText},
		}

		flashModel := modelName
		if strings.Contains(modelName, "deepseek") {
			flashModel = "deepseek-v4-flash"
		} else if strings.Contains(modelName, "moonshot") {
			flashModel = "moonshot-v1-auto"
		} else if strings.Contains(modelName, "glm") {
			flashModel = "glm-4-flash"
		} else if strings.Contains(modelName, "gpt") {
			flashModel = "gpt-4o-mini"
		}

		resp, err := a.callAPIEx(messages, apiKey, apiURL, flashModel, false, false, sessionID+"_summary", nil)
		if err != nil {
			fmt.Printf("[Memory] 摘要提取API调用失败: %v\n", err)
			return
		}

		content := resp.Content
		parts := strings.Split(content, "|")
		summary := ""
		topics := ""

		if len(parts) >= 1 {
			summary = strings.TrimSpace(parts[0])
		}
		if len(parts) >= 2 {
			topics = strings.TrimSpace(parts[1])
		}

		if summary == "" {
			return
		}

		if a.memory == nil {
			return
		}

		var lastEpisodeID int64
		err = a.memory.db.QueryRow(
			"SELECT id FROM episodes WHERE session_id = ? AND role = 'assistant' ORDER BY id DESC LIMIT 1",
			sessionID,
		).Scan(&lastEpisodeID)

		if err != nil {
			fmt.Printf("[Memory] 查找最新episode ID失败: %v\n", err)
			return
		}

		if err := a.memory.UpdateEpisodeSummary(lastEpisodeID, summary, topics); err != nil {
			fmt.Printf("[Memory] 更新摘要失败: %v\n", err)
		} else {
			fmt.Printf("[Memory] 摘要已更新: %s (标签: %s)\n", summary, topics)
		}
	}()
}

// ==================== 扩展查询方法 ====================

// MemoryEntry 表示单条记忆条目
type MemoryEntry struct {
	ID         int64   `json:"id"`
	SessionID  string  `json:"session_id"`
	Timestamp  int64   `json:"timestamp"`
	Role       string  `json:"role"`
	Content    string  `json:"content"`
	Summary    string  `json:"summary,omitempty"`
	Topics     string  `json:"topics,omitempty"`
	Importance float64 `json:"importance"`
}

// GetMemoriesByTag 根据主题标签过滤记忆
// tag 参数会在 topics 字段中进行模糊匹配
func (m *MemoryStore) GetMemoriesByTag(tag string, limit int) ([]MemoryEntry, error) {
	if strings.TrimSpace(tag) == "" {
		return nil, fmt.Errorf("标签不能为空")
	}

	rows, err := m.db.Query(`
		SELECT id, session_id, timestamp, role, content,
			COALESCE(summary, ''), COALESCE(topics, ''), importance
		FROM episodes
		WHERE topics LIKE ?
		ORDER BY timestamp DESC
		LIMIT ?
	`, "%"+tag+"%", limit)
	if err != nil {
		return nil, fmt.Errorf("按标签查询记忆失败: %w", err)
	}
	defer rows.Close()

	var entries []MemoryEntry
	for rows.Next() {
		var e MemoryEntry
		if err := rows.Scan(&e.ID, &e.SessionID, &e.Timestamp, &e.Role,
			&e.Content, &e.Summary, &e.Topics, &e.Importance); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// GetMemoriesBySession 获取指定会话的所有记忆
func (m *MemoryStore) GetMemoriesBySession(sessionID string) ([]MemoryEntry, error) {
	if strings.TrimSpace(sessionID) == "" {
		return nil, fmt.Errorf("会话ID不能为空")
	}

	rows, err := m.db.Query(`
		SELECT id, session_id, timestamp, role, content,
			COALESCE(summary, ''), COALESCE(topics, ''), importance
		FROM episodes
		WHERE session_id = ?
		ORDER BY timestamp ASC
	`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("按会话查询记忆失败: %w", err)
	}
	defer rows.Close()

	var entries []MemoryEntry
	for rows.Next() {
		var e MemoryEntry
		if err := rows.Scan(&e.ID, &e.SessionID, &e.Timestamp, &e.Role,
			&e.Content, &e.Summary, &e.Topics, &e.Importance); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// SetImportance 手动设置指定记忆的重要性评分
// importance 值范围为 0.0 到 1.0
func (m *MemoryStore) SetImportance(episodeID int64, importance float64) error {
	if importance < 0.0 || importance > 1.0 {
		return fmt.Errorf("重要性评分必须在 0.0 到 1.0 之间，当前值: %f", importance)
	}

	result, err := m.db.Exec(
		"UPDATE episodes SET importance = ? WHERE id = ?",
		importance, episodeID,
	)
	if err != nil {
		return fmt.Errorf("设置重要性评分失败: %w", err)
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("未找到 ID 为 %d 的记忆条目", episodeID)
	}
	return nil
}

// GetImportantMemories 获取重要性评分高于指定阈值的记忆
func (m *MemoryStore) GetImportantMemories(threshold float64, limit int) ([]MemoryEntry, error) {
	if threshold < 0.0 || threshold > 1.0 {
		return nil, fmt.Errorf("阈值必须在 0.0 到 1.0 之间，当前值: %f", threshold)
	}

	rows, err := m.db.Query(`
		SELECT id, session_id, timestamp, role, content,
			COALESCE(summary, ''), COALESCE(topics, ''), importance
		FROM episodes
		WHERE importance >= ?
		ORDER BY importance DESC, timestamp DESC
		LIMIT ?
	`, threshold, limit)
	if err != nil {
		return nil, fmt.Errorf("查询重要记忆失败: %w", err)
	}
	defer rows.Close()

	var entries []MemoryEntry
	for rows.Next() {
		var e MemoryEntry
		if err := rows.Scan(&e.ID, &e.SessionID, &e.Timestamp, &e.Role,
			&e.Content, &e.Summary, &e.Topics, &e.Importance); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// ExportMemories 导出所有记忆为 JSON 格式字符串
func (m *MemoryStore) ExportMemories() (string, error) {
	rows, err := m.db.Query(`
		SELECT id, session_id, timestamp, role, content,
			COALESCE(summary, ''), COALESCE(topics, ''), importance
		FROM episodes
		ORDER BY timestamp ASC
	`)
	if err != nil {
		return "", fmt.Errorf("导出记忆失败: %w", err)
	}
	defer rows.Close()

	var entries []MemoryEntry
	for rows.Next() {
		var e MemoryEntry
		if err := rows.Scan(&e.ID, &e.SessionID, &e.Timestamp, &e.Role,
			&e.Content, &e.Summary, &e.Topics, &e.Importance); err != nil {
			continue
		}
		entries = append(entries, e)
	}

	// 手动构建 JSON 以避免引入 encoding/json 依赖（如已引入可替换）
	var b strings.Builder
	b.WriteString("[\n")
	for i, e := range entries {
		b.WriteString(fmt.Sprintf(`  {"id":%d,"session_id":"%s","timestamp":%d,"role":"%s","content":"%s","summary":"%s","topics":"%s","importance":%.2f}`,
			e.ID,
			jsonEscape(e.SessionID),
			e.Timestamp,
			jsonEscape(e.Role),
			jsonEscape(e.Content),
			jsonEscape(e.Summary),
			jsonEscape(e.Topics),
			e.Importance,
		))
		if i < len(entries)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("]")
	return b.String(), nil
}

// jsonEscape 对字符串进行 JSON 转义
func jsonEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	return s
}

// MemoryTimelineGroup 表示按日期分组的记忆
type MemoryTimelineGroup struct {
	Date    string        `json:"date"`
	Entries []MemoryEntry `json:"entries"`
}

// GetMemoryTimeline 获取按日期分组的记忆时间线
func (m *MemoryStore) GetMemoryTimeline(days int) ([]MemoryTimelineGroup, error) {
	if days <= 0 {
		days = 7
	}

	since := time.Now().AddDate(0, 0, -days).Unix()

	rows, err := m.db.Query(`
		SELECT id, session_id, timestamp, role, content,
			COALESCE(summary, ''), COALESCE(topics, ''), importance
		FROM episodes
		WHERE timestamp >= ?
		ORDER BY timestamp ASC
	`, since)
	if err != nil {
		return nil, fmt.Errorf("查询记忆时间线失败: %w", err)
	}
	defer rows.Close()

	groupMap := make(map[string][]MemoryEntry)
	var dateOrder []string

	for rows.Next() {
		var e MemoryEntry
		if err := rows.Scan(&e.ID, &e.SessionID, &e.Timestamp, &e.Role,
			&e.Content, &e.Summary, &e.Topics, &e.Importance); err != nil {
			continue
		}

		dateKey := time.Unix(e.Timestamp, 0).Format("2006-01-02")
		if _, exists := groupMap[dateKey]; !exists {
			dateOrder = append(dateOrder, dateKey)
		}
		groupMap[dateKey] = append(groupMap[dateKey], e)
	}

	var timeline []MemoryTimelineGroup
	for _, date := range dateOrder {
		timeline = append(timeline, MemoryTimelineGroup{
			Date:    date,
			Entries: groupMap[date],
		})
	}
	return timeline, nil
}

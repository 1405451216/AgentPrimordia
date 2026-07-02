// sqlite_export.go — SQLite 存储的导入导出
//   - ExportMemories / ImportMemories
//   - exportJSON / exportMarkdown / importJSON
package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// ExportMemories 导出记忆为指定格式（json | markdown）
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

// ImportMemories 从指定格式数据导入记忆（当前仅支持 json）
func (s *SQLiteStore) ImportMemories(ctx context.Context, data []byte, format string) (int, error) {
	switch format {
	case "json":
		return s.importJSON(ctx, data)
	default:
		return s.importJSON(ctx, data)
	}
}

// exportJSON 将 Episode 切片序列化为 JSON
func exportJSON(episodes []*Episode) ([]byte, error) {
	data, err := json.Marshal(episodes)
	if err != nil {
		return nil, fmt.Errorf("marshal episodes to json: %w", err)
	}
	return data, nil
}

// exportMarkdown 将 Episode 切片渲染为 Markdown 文档
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

// importJSON 反序列化 JSON 为 Episode 切片，通过 BatchAdd 单事务导入
func (s *SQLiteStore) importJSON(ctx context.Context, data []byte) (int, error) {
	var episodes []*Episode
	if err := json.Unmarshal(data, &episodes); err != nil {
		return 0, fmt.Errorf("unmarshal episodes from json: %w", err)
	}

	// 优化（Task 4）：使用批量事务导入，避免每条记录都 fsync 一次
	if err := s.BatchAdd(ctx, episodes); err != nil {
		return 0, fmt.Errorf("batch import: %w", err)
	}
	return len(episodes), nil
}

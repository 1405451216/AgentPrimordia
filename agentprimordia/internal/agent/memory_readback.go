// memory_readback.go — 长期记忆回读注入（v3.4-3）
//
// 此前 MemoryStore 只写不读（loop 仅 Add/UpdateSummary）。本文件定义可选
// 接口 MemoryQuerier 与注入逻辑：agent 的 memory store 若实现回读能力，
// 循环首轮将用户目标作为查询召回相关记忆，作为 system 消息注入 LLM。
package agent

import (
	"context"
	"fmt"
	"strings"

	"agentprimordia/internal/memory"
)

// MemoryQuerier 可选接口：实现者支持长期记忆回读。
// loop 首轮用目标检索相关记忆并注入，实现「跨 session 记忆召回」。
type MemoryQuerier interface {
	Search(ctx context.Context, query string, opts *memory.SearchOptions) ([]*memory.Episode, error)
}

// searchMemoryContext 以历史中的用户目标为查询，检索长期记忆并格式化为注入上下文。
// store 未实现 MemoryQuerier 或检索无结果时返回空串。
func (a *ReActAgent) searchMemoryContext(ctx context.Context, history []Message) string {
	store := a.getMemoryStore()
	q, ok := store.(MemoryQuerier)
	if !ok {
		return ""
	}
	query := extractUserInput(history)
	if query == "" {
		return ""
	}
	episodes, err := q.Search(ctx, query, nil)
	if err != nil || len(episodes) == 0 {
		return ""
	}
	return formatMemoryContext(episodes)
}

// formatMemoryContext 将检索到的记忆片段格式化为注入文本。
func formatMemoryContext(episodes []*memory.Episode) string {
	if len(episodes) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("[长期记忆]\n")
	for _, ep := range episodes {
		content := ep.Content
		if len(content) > 300 {
			content = content[:300] + "..."
		}
		role := ep.Role
		if role == "" {
			role = "episode"
		}
		sb.WriteString(fmt.Sprintf("- [%s] %s\n", role, content))
	}
	return sb.String()
}

// injectMemoryContext 将记忆上下文作为 system 消息注入历史。
// 已存在 memory_context 消息时替换；否则在系统消息之后插入。
func injectMemoryContext(history []Message, memoryContext string) []Message {
	if memoryContext == "" {
		return history
	}

	memMsg := SystemMessage(memoryContext)
	if memMsg.Metadata.Extra == nil {
		memMsg.Metadata.Extra = make(map[string]string)
	}
	memMsg.Metadata.Extra["memory_context"] = "true"

	// 查找已有的记忆上下文消息并替换
	for i, m := range history {
		if m.Role == RoleSystem && m.Metadata.Extra["memory_context"] == "true" {
			history[i] = memMsg
			return history
		}
	}

	// 没有找到已有的记忆消息，在 system 消息之后插入
	systemEnd := 0
	for i, m := range history {
		if m.Role != RoleSystem {
			systemEnd = i
			break
		}
		if i == len(history)-1 {
			systemEnd = len(history)
		}
	}

	newHistory := make([]Message, 0, len(history)+1)
	newHistory = append(newHistory, history[:systemEnd]...)
	newHistory = append(newHistory, memMsg)
	newHistory = append(newHistory, history[systemEnd:]...)
	return newHistory
}

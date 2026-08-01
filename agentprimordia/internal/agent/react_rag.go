// react_rag.go — RAG 检索相关
// 负责 RAG（检索增强生成）的判断、检索和上下文注入
package agent

import (
	"context"
)

// shouldRAG 判断当前轮次是否需要执行 RAG 检索
func (a *ReActAgent) shouldRAG(turn int) bool {
	rag := a.getRAGConfig()
	if rag == nil || rag.Provider == nil {
		return false
	}
	switch rag.Mode {
	case RAGModeFirst:
		return turn == 0
	case RAGModeOnDemand:
		return false // 由 knowledge_search tool主动触发
	case RAGModeAuto:
		fallthrough
	default:
		return true
	}
}

// ragTopK 返回 RAG 检索的 TopK 值
func (a *ReActAgent) ragTopK() int {
	if rag := a.getRAGConfig(); rag != nil && rag.TopK > 0 {
		return rag.TopK
	}
	return 5
}

// ragMinScore 返回 RAG 检索的最低相关度阈值
func (a *ReActAgent) ragMinScore() float32 {
	if rag := a.getRAGConfig(); rag != nil && rag.MinScore > 0 {
		return rag.MinScore
	}
	return 0.3
}

// searchRAG 执行 RAG 检索并返回格式化上下文
func (a *ReActAgent) searchRAG(ctx context.Context, query string) (string, []*RAGDocument) {
	_ = a.fireHook(HookBeforeRAG, &HookContext{Metadata: map[string]any{"query": query}})

	rag := a.getRAGConfig()
	if rag == nil || rag.Provider == nil {
		a.logger.Debug("RAG 未配置，跳过检索", "query", query)
		return "", nil
	}
	docs, err := rag.Provider.Search(ctx, query, a.ragTopK())
	if err != nil {
		a.logger.Warn("RAG 检索失败", "error", err, "query", query)
		_ = a.fireHook(HookOnError, &HookContext{Error: err})
		return "", nil
	}

	// 过滤低分结果
	minScore := a.ragMinScore()
	filtered := make([]*RAGDocument, 0, len(docs))
	for _, doc := range docs {
		if doc.Score >= minScore {
			filtered = append(filtered, doc)
		}
	}

	_ = a.fireHook(HookAfterRAG, &HookContext{Metadata: map[string]any{"results": len(filtered), "query": query}})

	if len(filtered) == 0 {
		return "", nil
	}

	// 格式化 RAG 上下文
	context := FormatRAGDocuments(filtered)
	return context, filtered
}

// injectRAGContext 将 RAG 检索结果注入到历史消息中
// 如果已存在 RAG 上下文消息，则替换；否则在 system 消息之后插入
func (a *ReActAgent) injectRAGContext(history []Message, ragContext string) []Message {
	if ragContext == "" {
		return history
	}

	ragMsg := SystemMessage(ragContext)
	if ragMsg.Metadata.Extra == nil {
		ragMsg.Metadata.Extra = make(map[string]string)
	}
	ragMsg.Metadata.Extra["rag_context"] = "true"

	// 查找已有的 RAG 上下文消息并替换
	for i, m := range history {
		if m.Role == RoleSystem && m.Metadata.Extra["rag_context"] == "true" {
			// 替换现有的 RAG 上下文消息
			history[i] = ragMsg
			return history
		}
	}

	// 没有找到已有的 RAG 消息，在 system 消息之后插入
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
	newHistory = append(newHistory, ragMsg)
	newHistory = append(newHistory, history[systemEnd:]...)

	return newHistory
}

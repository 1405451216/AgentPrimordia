// Stability: Stable — 记忆存储核心接口与实现（除 VectorStore 之外）。
package ap

import (
	"agentprimordia/internal/memory"
)

// Memory 是记忆存储的核心接口，提供增删查改、搜索、统计、导入导出等完整能力
type Memory = memory.Memory

// Episode 是一条记忆片段，包含会话 ID、角色、内容、摘要、主题、重要性等
type Episode = memory.Episode

// SearchOptions 是记忆搜索的选项，支持会话过滤、分页和角色过滤
type SearchOptions = memory.SearchOptions

// ListOptions 是记忆列表的选项，支持会话过滤、分页、排序方向
type ListOptions = memory.ListOptions

// MemoryStats 是记忆存储的统计信息，包含总数、会话数、时间范围和存储大小
type MemoryStats = memory.MemoryStats

// SQLiteStore 是基于 SQLite + FTS5 的记忆存储实现，支持全文搜索和自动清理
type SQLiteStore = memory.SQLiteStore

// VectorStore 是内存向量存储，支持余弦相似度搜索
type VectorStore = memory.VectorStore

// VectorEntry 是向量存储中的一条记录，包含 ID、向量和元数据
type VectorEntry = memory.VectorEntry

// VectorSearchResult 是向量搜索的结果，包含 ID、相似度分数和元数据
type VectorSearchResult = memory.VectorSearchResult

// EmbeddingProvider 是向量化接口，由 LLM Provider 适配实现
type EmbeddingProvider = memory.EmbeddingProvider

// RAGStore 封装了 Memory + VectorStore + EmbeddingProvider，提供混合 RAG 检索能力
type RAGStore = memory.RAGStore

// RAGResult 是 RAG 查询的结果，包含匹配的记忆片段、相关度分数和来源（fts / vector）
type RAGResult = memory.RAGResult

// CleanupConfig 是记忆自动清理配置，包含过期天数、清理间隔和保留角色
type CleanupConfig = memory.CleanupConfig

var (
	// NewSQLiteStore 创建基于 SQLite 的记忆存储实例，参数为数据库文件路径
	NewSQLiteStore = memory.NewSQLiteStore
	// WithInMemory 创建内存模式的 SQLite 记忆存储，适用于测试
	WithInMemory = memory.WithInMemory
	// NewVectorStore 创建内存向量存储实例，参数为向量维度
	NewVectorStore = memory.NewVectorStore
	// FormatRAGContext 将 RAG 检索结果格式化为可注入 Prompt 的上下文字符串
	FormatRAGContext = memory.FormatRAGContext
)

// ===== RAG 端到端生成 =====

// LLMGenerator 是 LLM 文本生成接口，用于 RAG 端到端生成（与 llm.Provider 解耦）
type LLMGenerator = memory.LLMGenerator

// RetrievalAugmentedGenerator 是 RAG 端到端生成器（检索→上下文组装→LLM 生成）
type RetrievalAugmentedGenerator = memory.RetrievalAugmentedGenerator

// RAGConfig 是 RAG 生成器配置
type RAGGeneratorConfig = memory.RAGConfig

// QueryResult 是 RAG 查询结果（答案 + 来源 + 上下文）
type QueryResult = memory.QueryResult

var (
	NewRetrievalAugmentedGenerator = memory.NewRetrievalAugmentedGenerator
)

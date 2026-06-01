package memory

import "context"

// MemoryReader 只读操作接口
type MemoryReader interface {
	Get(ctx context.Context, id string) (*Episode, error)
	Search(ctx context.Context, query string, opts *SearchOptions) ([]*Episode, error)
	List(ctx context.Context, opts *ListOptions) ([]*Episode, error)
	Count(ctx context.Context, sessionID string) (int64, error)
	Stats(ctx context.Context) (*MemoryStats, error)
}

// MemoryWriter 写入操作接口
type MemoryWriter interface {
	Add(ctx context.Context, episode *Episode) error
	Delete(ctx context.Context, id string) error
	UpdateSummary(ctx context.Context, id string, summary, topics string) error
	SetImportance(ctx context.Context, episodeID string, importance float64) error
}

// MemorySearcher 高级搜索接口
type MemorySearcher interface {
	SearchAdvanced(ctx context.Context, opts SearchOptions) ([]*SearchResult, error)
	SearchByTag(ctx context.Context, tag string, opts *SearchOptions) ([]*Episode, error)
	GetImportant(ctx context.Context, threshold float64, limit int) ([]*Episode, error)
	GetTimeline(ctx context.Context, days int) (map[string][]*Episode, error)
}

// MemoryLifecycle 生命周期管理接口
type MemoryLifecycle interface {
	Close() error
	CleanupExpired(ctx context.Context, maxAgeDays int) (int64, error)
	ClearAll(ctx context.Context, sessionID string) error
}

// MemoryExporter 导入导出接口
type MemoryExporter interface {
	ExportMemories(ctx context.Context, sessionID, format string) ([]byte, error)
	ImportMemories(ctx context.Context, data []byte, format string) (int, error)
}

// MemoryQuery 辅助查询接口
type MemoryQuery interface {
	GetMemoriesByTag(ctx context.Context, tag string, limit int) ([]*Episode, error)
	GetMemoriesBySession(ctx context.Context, sessionID string) ([]*Episode, error)
	GetImportantMemories(ctx context.Context, threshold float64, limit int) ([]*Episode, error)
	GetMemoryTimeline(ctx context.Context, days int) ([]*MemoryTimelineGroup, error)
}

// MemoryToolUse 工具使用记录接口
type MemoryToolUse interface {
	RecordToolUse(ctx context.Context, sessionID, agentName, toolName, args, result string) error
}

// Memory 组合接口（保持向后兼容）
type Memory interface {
	MemoryReader
	MemoryWriter
	MemorySearcher
	MemoryLifecycle
	MemoryExporter
	MemoryQuery
	MemoryToolUse
}

type Episode struct {
	ID         string            `json:"id"`
	SessionID  string            `json:"session_id"`
	Role       string            `json:"role"`
	Content    string            `json:"content"`
	Summary    string            `json:"summary,omitempty"`
	Topics     string            `json:"topics,omitempty"`
	Importance float64           `json:"importance,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	CreatedAt  string            `json:"created_at"`
}

type SearchOptions struct {
	Query          string
	SessionID      string
	Limit          int     // 分页限制：与 Offset 配合实现分页查询
	Offset         int
	RoleFilter     string
	Tags           []string
	MinScore       float64
	MaxResults     int     // 最大结果数：限制搜索返回的总结果数（不受分页影响）
	UseSemantic    bool
	SemanticWeight float64
}

type SearchResult struct {
	Episode       *Episode
	KeywordScore  float64
	SemanticScore float64
	CombinedScore float64
}

type ListOptions struct {
	SessionID string
	Limit     int
	Offset    int
	OrderBy   string
	Ascending bool
}

type MemoryStats struct {
	TotalEpisodes         int64   `json:"total_episodes"`
	TotalSessions         int64   `json:"total_sessions"`
	OldestEpisode         string  `json:"oldest_episode,omitempty"`
	NewestEpisode         string  `json:"newest_episode,omitempty"`
	AvgEpisodesPerSession float64 `json:"avg_episodes_per_session"`
	SizeBytes             int64   `json:"size_bytes"`
}

// MemoryTimelineGroup 记忆时间线分组
//
// 按日期将记忆条目分组，用于展示时间线视图。
// 从 CodeCast-desktop/memory.go 的 Timeline 功能提取。
type MemoryTimelineGroup struct {
	Date     string     `json:"date"`              // 日期 (YYYY-MM-DD)
	Episodes []*Episode `json:"episodes"`          // 当天的记忆条目列表
	Count    int        `json:"count"`             // 条目数量
	Summary  string     `json:"summary,omitempty"` // 当天摘要（可选）
}

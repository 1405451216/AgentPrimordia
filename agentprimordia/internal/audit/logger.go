// Package audit 提供审计日志功能，用于记录和查询 Agent 操作的合规性事件。
//
// 审计日志记录所有关键操作（如文件访问、tool调用、权限变更等），
// 支持按 Actor/Action/Resource/时间范围进行查询，并可生成合规报告。
package audit

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// Event 表示一条审计事件。
type Event struct {
	// Timestamp 事件发生时间，若为零值则由 Logger 自动填充为当前时间
	Timestamp time.Time `json:"timestamp"`
	// Actor 执行操作的主体（如 Agent 名称、用户 ID）
	Actor string `json:"actor"`
	// Action 执行的操作类型（如 file.read、shell.exec）
	Action string `json:"action"`
	// Resource 操作的目标资源（如文件路径、API 端点）
	Resource string `json:"resource"`
	// Details 操作的附加详情
	Details map[string]any `json:"details,omitempty"`
	// Result 操作结果（如 success、denied、error）
	Result string `json:"result"`
	// TraceID 关联的分布式追踪 ID（v3.5-4 全链路回溯关联键）
	TraceID string `json:"trace_id,omitempty"`
}

// QueryFilter 用于筛选审计事件的查询条件。
type QueryFilter struct {
	// Actor 按操作主体过滤，空值表示不过滤
	Actor string `json:"actor,omitempty"`
	// Action 按操作类型过滤，空值表示不过滤
	Action string `json:"action,omitempty"`
	// Resource 按目标资源过滤，空值表示不过滤
	Resource string `json:"resource,omitempty"`
	// Start 时间范围起始，零值表示不限制起始时间
	Start time.Time `json:"start,omitempty"`
	// End 时间范围结束，零值表示不限制结束时间
	End time.Time `json:"end,omitempty"`
	// Limit 返回结果的最大数量，0 表示不限制
	Limit int `json:"limit,omitempty"`
}

// ActorStats 单个 Actor 的操作统计。
type ActorStats struct {
	// TotalActions 该 Actor 的总操作次数
	TotalActions int `json:"total_actions"`
	// Actions 按操作类型统计的次数
	Actions map[string]int `json:"actions"`
}

// ComplianceReport 合规报告，汇总指定时间段内的审计事件统计。
type ComplianceReport struct {
	// Period 报告的时间范围
	Period PeriodStats `json:"period"`
	// TotalEvents 报告期间的总事件数
	TotalEvents int `json:"total_events"`
	// ActorStats 按 Actor 分组的统计
	ActorStats map[string]ActorStats `json:"actor_stats"`
	// ActionStats 按 Action 类型分组的统计
	ActionStats map[string]int `json:"action_stats"`
}

// PeriodStats 报告的时间范围。
type PeriodStats struct {
	// Start 起始时间
	Start time.Time `json:"start"`
	// End 结束时间
	End time.Time `json:"end"`
}

// Output 定义审计日志的存储后端接口。
// 实现此接口可对接不同的存储系统（内存、文件、数据库等）。
type Output interface {
	// Write 写入一条审计事件
	Write(Event) error
	// Query 按条件查询审计事件
	Query(QueryFilter) ([]Event, error)
}

// LoggerConfig 审计日志器的配置。
type LoggerConfig struct {
	// Output 审计事件的存储后端，必填
	Output Output
}

// Logger 审计日志器，提供事件记录、查询和合规报告生成功能。
type Logger struct {
	config LoggerConfig
}

// ErrOutputRequired 当 LoggerConfig.Output 为 nil 时返回
var ErrOutputRequired = errors.New("audit: LoggerConfig.Output must not be nil")

// NewLogger 创建审计日志器。
// cfg.Output 不能为 nil，否则返回 ErrOutputRequired。
func NewLogger(cfg LoggerConfig) (*Logger, error) {
	if cfg.Output == nil {
		return nil, ErrOutputRequired
	}
	return &Logger{config: cfg}, nil
}

// Log 记录一条审计事件。
// 若 event.Timestamp 为零值，自动填充为当前时间。
func (l *Logger) Log(ctx context.Context, event Event) error {
	// 自动填充时间戳
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	return l.config.Output.Write(event)
}

// Query 按条件查询审计事件，委托给 Output 实现。
func (l *Logger) Query(ctx context.Context, filter QueryFilter) ([]Event, error) {
	return l.config.Output.Query(filter)
}

// GenerateReport 生成指定时间范围内的合规报告。
// 查询该时间段内的所有事件，并按 Actor 和 Action 维度汇总统计。
func (l *Logger) GenerateReport(ctx context.Context, start, end time.Time) (*ComplianceReport, error) {
	// 查询时间范围内的事件
	filter := QueryFilter{
		Start: start,
		End:   end,
	}
	events, err := l.config.Output.Query(filter)
	if err != nil {
		return nil, err
	}

	report := &ComplianceReport{
		Period:      PeriodStats{Start: start, End: end},
		TotalEvents: len(events),
		ActorStats:  make(map[string]ActorStats),
		ActionStats: make(map[string]int),
	}

	// 按 Actor 和 Action 维度汇总
	for _, e := range events {
		// 更新 Actor 统计
		as, ok := report.ActorStats[e.Actor]
		if !ok {
			as = ActorStats{
				Actions: make(map[string]int),
			}
		}
		as.TotalActions++
		as.Actions[e.Action]++
		report.ActorStats[e.Actor] = as

		// 更新 Action 统计
		report.ActionStats[e.Action]++
	}

	return report, nil
}

// ExportJSON 将合规报告导出为格式化的 JSON 字符串。
func (r *ComplianceReport) ExportJSON() (string, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

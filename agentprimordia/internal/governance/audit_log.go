// audit_log.go — 审计日志持久化（G3-3 治理强化）
//
// 将策略违规事件持久化到审计日志，支持：
// - 文件持久化（JSONL 格式，追加写入）
// - 按时间/Agent/违规类型检索
// - 日志轮转（按大小自动切分）
// - 告警回调（违规事件实时推送到 Webhook / Slack / Prometheus AlertManager）
//
// 设计为独立于 PolicyEnforcer，通过 AuditLogger 接口解耦，
// 使 Enforcer 调用 logger.Log() 记录违规，logger 内部决定持久化与告警策略。
package governance

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// AuditEventType 审计事件类型。
type AuditEventType string

const (
	AuditToolCallBlocked  AuditEventType = "tool_call_blocked"
	AuditCostExceeded     AuditEventType = "cost_exceeded"
	AuditOutputBlocked    AuditEventType = "output_blocked"
	AuditPIIDetected      AuditEventType = "pii_detected"
	AuditPolicyViolation  AuditEventType = "policy_violation"
	AuditPolicyLoaded     AuditEventType = "policy_loaded"
	AuditPolicyHotSwapped AuditEventType = "policy_hot_swapped"
)

// AuditEvent 审计日志事件。
type AuditEvent struct {
	ID        string          `json:"id"`
	Timestamp time.Time       `json:"timestamp"`
	Type      AuditEventType  `json:"type"`
	AgentID   string          `json:"agent_id"`
	ToolName  string          `json:"tool_name,omitempty"`
	Reason    string          `json:"reason"`
	Detail    json.RawMessage `json:"detail,omitempty"`
	Severity  string          `json:"severity"` // info | warning | critical
}

// AlertCallback 告警回调函数。
// 返回 error 仅记录日志，不阻断审计流程。
type AlertCallback func(event AuditEvent) error

// AuditLogger 审计日志接口。
type AuditLogger interface {
	Log(event AuditEvent)
	Query(filter AuditQuery) ([]AuditEvent, error)
	Close() error
}

// AuditQuery 审计日志查询条件。
type AuditQuery struct {
	StartTime *time.Time
	EndTime   *time.Time
	AgentID   string
	Type      AuditEventType
	Severity  string
	Limit     int
}

// FileAuditLogger 文件审计日志（JSONL 格式 + 轮转）。
type FileAuditLogger struct {
	mu         sync.Mutex
	filePath   string
	maxSizeMB  int
	file       *os.File
	writer     io.Writer
	alertFn    AlertCallback
	eventCount int64
	metrics    *GovernanceMetrics // 可选的可观测性指标
}

// NewFileAuditLogger 创建文件审计日志。
//   - filePath: 日志文件路径
//   - maxSizeMB: 单文件最大大小（MB），超过后轮转
//   - alertFn: 可选的告警回调（违规事件实时推送）
func NewFileAuditLogger(filePath string, maxSizeMB int, alertFn AlertCallback) (*FileAuditLogger, error) {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create audit log directory: %w", err)
	}
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("failed to open audit log file: %w", err)
	}
	return &FileAuditLogger{
		filePath:  filePath,
		maxSizeMB: maxSizeMB,
		file:      f,
		writer:    f,
		alertFn:   alertFn,
	}, nil
}

// NewFileAuditLoggerWithMetrics 创建带可观测性指标的文件审计日志。
func NewFileAuditLoggerWithMetrics(filePath string, maxSizeMB int, alertFn AlertCallback, metrics *GovernanceMetrics) (*FileAuditLogger, error) {
	logger, err := NewFileAuditLogger(filePath, maxSizeMB, alertFn)
	if err != nil {
		return nil, err
	}
	logger.metrics = metrics
	return logger, nil
}

// Log 记录一条审计事件。
func (l *FileAuditLogger) Log(event AuditEvent) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// 对 Reason 和 Detail 进行 Secret 脱敏
	event.Reason = maskSecret(event.Reason)

	// 确保 ID 和 Timestamp
	if event.ID == "" {
		event.ID = fmt.Sprintf("audit-%d-%d", time.Now().UnixNano(), l.eventCount)
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	l.eventCount++

	// 序列化写入
	data, err := json.Marshal(event)
	if err != nil {
		if l.metrics != nil {
			l.metrics.RecordAuditLogError()
		}
		return
	}
	data = append(data, '\n')
	if _, err := l.writer.Write(data); err != nil {
		if l.metrics != nil {
			l.metrics.RecordAuditLogError()
		}
		// 写入失败：尝试轮转后重试
		if rotateErr := l.rotate(); rotateErr == nil {
			l.writer.Write(data)
		}
		return
	}

	// 记录成功写入指标
	if l.metrics != nil {
		l.metrics.RecordAuditLogWrite()
	}

	// 检查是否需要轮转
	if l.shouldRotate() {
		l.rotate()
	}

	// 触发告警回调（不持锁，但在此处同步执行以保证顺序）
	if l.alertFn != nil {
		go l.alertFn(event)
	}
}

// Query 查询审计日志。
func (l *FileAuditLogger) Query(filter AuditQuery) ([]AuditEvent, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// 关闭当前文件以刷新缓冲
	if l.file != nil {
		l.file.Sync()
	}

	data, err := os.ReadFile(l.filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read audit log: %w", err)
	}

	var events []AuditEvent
	lines := splitLines(data)
	limit := filter.Limit
	if limit <= 0 {
		limit = 1000 // 默认限制
	}

	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		var event AuditEvent
		if err := json.Unmarshal(line, &event); err != nil {
			continue
		}
		if !matchFilter(event, filter) {
			continue
		}
		events = append(events, event)
		if len(events) >= limit {
			break
		}
	}

	return events, nil
}

// Close 关闭日志文件。
func (l *FileAuditLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// shouldRotate 检查是否需要轮转。
func (l *FileAuditLogger) shouldRotate() bool {
	if l.file == nil {
		return false
	}
	info, err := l.file.Stat()
	if err != nil {
		return false
	}
	return info.Size() > int64(l.maxSizeMB)*1024*1024
}

// rotate 轮转日志文件。
func (l *FileAuditLogger) rotate() error {
	if l.file != nil {
		l.file.Close()
	}
	// 重命名旧文件
	rotatedPath := fmt.Sprintf("%s.%s.rotated", l.filePath, time.Now().Format("20060102-150405"))
	os.Rename(l.filePath, rotatedPath)

	// 打开新文件
	f, err := os.OpenFile(l.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	l.file = f
	l.writer = f
	return nil
}

// matchFilter 检查事件是否匹配查询条件。
func matchFilter(event AuditEvent, filter AuditQuery) bool {
	if filter.StartTime != nil && event.Timestamp.Before(*filter.StartTime) {
		return false
	}
	if filter.EndTime != nil && event.Timestamp.After(*filter.EndTime) {
		return false
	}
	if filter.AgentID != "" && event.AgentID != filter.AgentID {
		return false
	}
	if filter.Type != "" && event.Type != filter.Type {
		return false
	}
	if filter.Severity != "" && event.Severity != filter.Severity {
		return false
	}
	return true
}

// splitLines 按行分割字节切片。
func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, data[start:i])
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}

// NopAuditLogger 空审计日志（不持久化，用于测试）。
type NopAuditLogger struct{}

func (NopAuditLogger) Log(event AuditEvent)                          {}
func (NopAuditLogger) Query(filter AuditQuery) ([]AuditEvent, error) { return nil, nil }
func (NopAuditLogger) Close() error                                  { return nil }

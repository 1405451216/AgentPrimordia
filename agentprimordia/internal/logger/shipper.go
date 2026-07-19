package logger

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// LogEntry 日志聚合基本单元，用于 Shipper 传输。
type LogEntry struct {
	Timestamp time.Time      `json:"timestamp"`
	Level     string         `json:"level"`
	Message   string         `json:"message"`
	Fields    map[string]any `json:"fields,omitempty"`
	TraceID   string         `json:"trace_id,omitempty"`
	SpanID    string         `json:"span_id,omitempty"`
	AgentID   string         `json:"agent_id,omitempty"`
}

// Shipper 日志投递接口。
type Shipper interface {
	Ship(entries []LogEntry) error
	Close() error
}

// ===== StdoutShipper =====

// StdoutShipper 将日志投递到 io.Writer，用于开发环境。
type StdoutShipper struct {
	mu  sync.Mutex
	out io.Writer
}

// NewStdoutShipper 创建输出到 os.Stdout 的 Shipper。
func NewStdoutShipper() *StdoutShipper {
	return &StdoutShipper{out: os.Stdout}
}

// NewStdoutShipperWithWriter 创建输出到指定 writer 的 Shipper。
func NewStdoutShipperWithWriter(w io.Writer) *StdoutShipper {
	return &StdoutShipper{out: w}
}

// Ship 将日志条目以 JSON 行格式写入输出。
func (s *StdoutShipper) Ship(entries []LogEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, e := range entries {
		data, err := json.Marshal(e)
		if err != nil {
			return fmt.Errorf("marshal log entry: %w", err)
		}
		data = append(data, '\n')
		if _, err := s.out.Write(data); err != nil {
			return fmt.Errorf("write log entry: %w", err)
		}
	}
	return nil
}

// Close 关闭 Shipper。
func (s *StdoutShipper) Close() error {
	if closer, ok := s.out.(io.Closer); ok && s.out != os.Stdout && s.out != os.Stderr {
		return closer.Close()
	}
	return nil
}

// ===== FileShipper =====

// FileShipper 将日志写入文件，支持按文件大小滚动。
type FileShipper struct {
	mu          sync.Mutex
	path        string
	maxSize     int64
	maxFiles    int
	file        *os.File
	currentSize int64
}

// NewFileShipper 创建文件日志 Shipper。
func NewFileShipper(path string, maxSizeMB int, maxFiles int) (*FileShipper, error) {
	if maxSizeMB <= 0 {
		maxSizeMB = 100
	}
	if maxFiles <= 0 {
		maxFiles = 5
	}

	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("create log directory: %w", err)
		}
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}

	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("stat log file: %w", err)
	}

	return &FileShipper{
		path:        path,
		maxSize:     int64(maxSizeMB) * 1024 * 1024,
		maxFiles:    maxFiles,
		file:        f,
		currentSize: info.Size(),
	}, nil
}

// Ship 写入日志条目，当文件超过 maxSize 时触发滚动。
func (s *FileShipper) Ship(entries []LogEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, e := range entries {
		data, err := json.Marshal(e)
		if err != nil {
			return fmt.Errorf("marshal log entry: %w", err)
		}
		data = append(data, '\n')

		if s.currentSize+int64(len(data)) > s.maxSize {
			if err := s.rotate(); err != nil {
				return err
			}
		}

		n, err := s.file.Write(data)
		if err != nil {
			return fmt.Errorf("write log entry: %w", err)
		}
		s.currentSize += int64(n)
	}
	return nil
}

// rotate 执行日志文件滚动。
func (s *FileShipper) rotate() error {
	if s.file != nil {
		_ = s.file.Close()
	}

	for i := s.maxFiles - 1; i >= 1; i-- {
		old := fmt.Sprintf("%s.%d", s.path, i)
		new := fmt.Sprintf("%s.%d", s.path, i+1)
		if _, err := os.Stat(old); err == nil {
			_ = os.Rename(old, new)
		}
	}

	if s.maxFiles > 0 {
		_ = os.Rename(s.path, s.path+".1")
	}

	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("create new log file: %w", err)
	}
	s.file = f
	s.currentSize = 0
	return nil
}

// Close 关闭文件。
func (s *FileShipper) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file != nil {
		return s.file.Close()
	}
	return nil
}

// ===== Hook =====

// Hook 实现了 slog.Handler 接口，可将 slog.Record 转发到 Shipper。
type Hook struct {
	mu      sync.Mutex
	shipper Shipper
	attrs   []slog.Attr
	groups  []string
}

// NewHook 创建 Hook，将日志记录转发到指定的 Shipper。
func NewHook(shipper Shipper) *Hook {
	return &Hook{shipper: shipper}
}

// Handle 处理一条 slog.Record，将其转换为 LogEntry 并投递。
func (h *Hook) Handle(ctx context.Context, r slog.Record) error {
	h.mu.Lock()
	shipper := h.shipper
	h.mu.Unlock()

	if shipper == nil {
		return nil
	}

	fields := make(map[string]any)
	for _, a := range h.attrs {
		fields[a.Key] = a.Value.Any()
	}
	r.Attrs(func(a slog.Attr) bool {
		fields[a.Key] = a.Value.Any()
		return true
	})

	entry := LogEntry{
		Timestamp: r.Time,
		Level:     r.Level.String(),
		Message:   r.Message,
		Fields:    fields,
		TraceID:   TraceIDFromContext(ctx),
		SpanID:    SpanIDFromContext(ctx),
	}

	if v, ok := fields[FieldAgentID]; ok {
		if s, ok := v.(string); ok {
			entry.AgentID = s
		}
	}

	return shipper.Ship([]LogEntry{entry})
}

// WithAttrs 返回一个携带额外静态字段的新 Hook。
func (h *Hook) WithAttrs(attrs []slog.Attr) slog.Handler {
	h.mu.Lock()
	defer h.mu.Unlock()
	newAttrs := make([]slog.Attr, 0, len(h.attrs)+len(attrs))
	newAttrs = append(newAttrs, h.attrs...)
	newAttrs = append(newAttrs, attrs...)
	return &Hook{shipper: h.shipper, attrs: newAttrs, groups: h.groups}
}

// WithGroup 返回一个关联到指定 group 的新 Hook。
func (h *Hook) WithGroup(name string) slog.Handler {
	h.mu.Lock()
	defer h.mu.Unlock()
	newGroups := make([]string, 0, len(h.groups)+1)
	newGroups = append(newGroups, h.groups...)
	newGroups = append(newGroups, name)
	return &Hook{shipper: h.shipper, attrs: h.attrs, groups: newGroups}
}

// Enabled 判断是否处理指定级别的日志（始终返回 true）。
func (h *Hook) Enabled(ctx context.Context, level slog.Level) bool {
	return true
}

// ===== 辅助函数 =====

// ParseLogLevel 解析字符串为 slog.Level。
func ParseLogLevel(s string) slog.Level {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "DEBUG":
		return slog.LevelDebug
	case "INFO", "":
		return slog.LevelInfo
	case "WARN", "WARNING":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		if n, err := strconv.Atoi(s); err == nil {
			return slog.Level(n)
		}
		return slog.LevelInfo
	}
}

// FormatLogEntry 将 LogEntry 格式化为可读字符串（用于调试）。
func FormatLogEntry(e LogEntry) string {
	var sb strings.Builder
	sb.WriteString(e.Timestamp.Format(time.RFC3339))
	sb.WriteByte(' ')
	sb.WriteString(e.Level)
	sb.WriteByte(' ')
	sb.WriteString(e.Message)
	if e.TraceID != "" {
		sb.WriteString(" trace_id=")
		sb.WriteString(e.TraceID)
	}
	if e.AgentID != "" {
		sb.WriteString(" agent_id=")
		sb.WriteString(e.AgentID)
	}
	return sb.String()
}

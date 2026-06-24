package logger

import (
	"io"
	"log/slog"
	"os"
)

// Level 日志级别
type Level = slog.Level

const (
	LevelDebug Level = slog.LevelDebug
	LevelInfo  Level = slog.LevelInfo
	LevelWarn  Level = slog.LevelWarn
	LevelError Level = slog.LevelError
)

// Format 日志格式
type Format string

const (
	FormatJSON Format = "json"
	FormatText Format = "text"
)

// Config 日志配置
type Config struct {
	Level  Level
	Format Format
	Output io.Writer
}

// Logger 结构化日志器
type Logger struct {
	*slog.Logger
}

// New 创建结构化日志器
func New(cfg *Config) *Logger {
	if cfg == nil {
		cfg = &Config{Level: LevelInfo, Format: FormatJSON, Output: os.Stdout}
	}
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}

	opts := &slog.HandlerOptions{Level: cfg.Level}

	var handler slog.Handler
	switch cfg.Format {
	case FormatText:
		handler = slog.NewTextHandler(cfg.Output, opts)
	default:
		handler = slog.NewJSONHandler(cfg.Output, opts)
	}

	return &Logger{Logger: slog.New(handler)}
}

// WithAgent 返回带 Agent 上下文的日志器
func (l *Logger) WithAgent(agentName, sessionID string) *Logger {
	return &Logger{
		Logger: l.Logger.With("agent_name", agentName, "session_id", sessionID),
	}
}

// WithComponent 返回带组件名的日志器
func (l *Logger) WithComponent(component string) *Logger {
	return &Logger{
		Logger: l.Logger.With("component", component),
	}
}

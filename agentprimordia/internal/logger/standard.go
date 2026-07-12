// Package logger 提供结构化日志能力（全局单例 + Config-based 实例）。
//
// 本文件实现全局单例 Logger，基于 log/slog。
// 与本包的 Config-based Logger（logger.go）互补：
//   - Logger（logger.go）：需要显式创建的实例，适合测试和子组件
//   - 全局 Logger（standard.go）：包级别默认 Logger，开箱即用，无需初始化
//
// 全局 Logger 默认以 JSON 格式输出到 os.Stderr，级别为 Info。
// 可通过 SetLevel 动态调整级别，通过 AP_LOG_FORMAT 环境变量切换为 text 格式。
package logger

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"sync"
)

// LogLevel 全局日志级别枚举。
type LogLevel int

const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarn
	LogLevelError
)

// 全局单例状态。
var (
	defaultLogger *slog.Logger
	mu            sync.RWMutex
	currentLevel  slog.Level = slog.LevelInfo
)

func init() {
	defaultLogger = createLogger(currentLevel)
}

// toSlogLevel 将 LogLevel 映射为 slog.Level。
func toSlogLevel(l LogLevel) slog.Level {
	switch l {
	case LogLevelDebug:
		return slog.LevelDebug
	case LogLevelInfo:
		return slog.LevelInfo
	case LogLevelWarn:
		return slog.LevelWarn
	case LogLevelError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// createLogger 根据环境变量创建 Logger 实例。
func createLogger(level slog.Level) *slog.Logger {
	opts := &slog.HandlerOptions{Level: level}
	if strings.EqualFold(os.Getenv("AP_LOG_FORMAT"), "text") {
		return slog.New(slog.NewTextHandler(os.Stderr, opts))
	}
	return slog.New(slog.NewJSONHandler(os.Stderr, opts))
}

// SetLevel 动态设置全局日志级别。
func SetLevel(level LogLevel) {
	mu.Lock()
	defer mu.Unlock()
	currentLevel = toSlogLevel(level)
	defaultLogger = createLogger(currentLevel)
}

// Get 返回全局 *slog.Logger 实例。
func Get() *slog.Logger {
	mu.RLock()
	defer mu.RUnlock()
	return defaultLogger
}

// Debug 以 Debug 级别记录全局日志。
func Debug(msg string, args ...any) {
	Get().Debug(msg, args...)
}

// Info 以 Info 级别记录全局日志。
func Info(msg string, args ...any) {
	Get().Info(msg, args...)
}

// Warn 以 Warn 级别记录全局日志。
func Warn(msg string, args ...any) {
	Get().Warn(msg, args...)
}

// Error 以 Error 级别记录全局日志。
func Error(msg string, args ...any) {
	Get().Error(msg, args...)
}

// DebugCtx 从 ctx 提取 trace_id/span_id 并以 Debug 级别记录日志。
func DebugCtx(ctx context.Context, msg string, args ...any) {
	l := FromContext(ctx, Get())
	l.Debug(msg, args...)
}

// InfoCtx 从 ctx 提取 trace_id/span_id 并以 Info 级别记录日志。
func InfoCtx(ctx context.Context, msg string, args ...any) {
	l := FromContext(ctx, Get())
	l.Info(msg, args...)
}

// With 返回一个携带固定字段的新 *slog.Logger。
func With(args ...any) *slog.Logger {
	return Get().With(args...)
}

// WithAgent 返回携带 agent_id 字段的全局 Logger。
func WithAgent(agentID string) *slog.Logger {
	return Get().With(FieldAgentID, agentID)
}

// WithSession 返回携带 session_id 字段的全局 Logger。
func WithSession(sessionID string) *slog.Logger {
	return Get().With(FieldSessionID, sessionID)
}

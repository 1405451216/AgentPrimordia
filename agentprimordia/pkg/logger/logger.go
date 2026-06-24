// Package logger 提供统一的日志标准。
//
// 所有模块通过 logger.Default() 获取默认日志器，通过 logger.WithComponent(name)
// 创建带模块标签的子日志器。用户可在应用入口一次性配置全局日志级别和输出格式。
package logger

import (
	"log/slog"
	"os"
	"sync"
)

var (
	mu     sync.RWMutex
	level  = slog.LevelInfo
	handler slog.Handler
)

// SetLevel 设置全局日志级别。应在应用启动时调用一次。
func SetLevel(l slog.Level) {
	mu.Lock()
	defer mu.Unlock()
	level = l
	// 重置 handler 以应用新级别
	handler = nil
}

// Default 返回全局默认日志器。
// 首次调用时根据当前 level 创建 handler，后续复用。
func Default() *slog.Logger {
	mu.RLock()
	h := handler
	mu.RUnlock()
	if h != nil {
		return slog.New(h)
	}
	mu.Lock()
	defer mu.Unlock()
	opts := &slog.HandlerOptions{Level: level}
	handler = slog.NewTextHandler(os.Stderr, opts)
	return slog.New(handler)
}

// WithComponent 返回带 "component" 属性的子日志器，便于区分模块来源。
func WithComponent(name string) *slog.Logger {
	return Default().With("component", name)
}

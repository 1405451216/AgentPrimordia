// Package audit 提供审计日志帮助函数，便于将 audit.Logger 桥接到其他包的接口。
package audit

import (
	"context"
	"time"
)

// ExternalAuditEvent 外部包使用的审计事件通用结构（避免循环依赖）。
// agent 等上层包可定义自己的事件类型，但应能转换为此结构。
// 字段命名与 audit.Event 保持一致。
type ExternalAuditEvent struct {
	Timestamp time.Time
	Actor     string
	Action    string
	Resource  string
	Result    string
	Details   map[string]any
}

// WriteExternal 写入外部事件到 Logger，自动补充时间戳。
// 用户可在自己的包中实现适配器：
//
//	func myAdapter(logger *audit.Logger) func(ctx context.Context, e myEvent) error {
//	    return func(ctx context.Context, e myEvent) error {
//	        return logger.WriteExternal(ctx, audit.ExternalAuditEvent{
//	            Actor: e.Actor, Action: e.Action, ...
//	        })
//	    }
//	}
func (l *Logger) WriteExternal(ctx context.Context, e ExternalAuditEvent) error {
	return l.Log(ctx, Event{
		Timestamp: e.Timestamp,
		Actor:     e.Actor,
		Action:    e.Action,
		Resource:  e.Resource,
		Result:    e.Result,
		Details:   e.Details,
	})
}

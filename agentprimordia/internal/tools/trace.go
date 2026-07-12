package tools

import (
	"context"
	"errors"
	"sync"
	"time"
)

// MaxTraceDepth 默认最大追踪深度
const MaxTraceDepth = 10

var (
	ErrTraceTooDeep  = errors.New("tool call trace exceeds maximum depth")
	ErrTraceNotFound = errors.New("trace not found")
)

// ToolCallTrace 表示一次工具调用的追踪记录
type ToolCallTrace struct {
	ID        string        `json:"id"`
	ParentID  string        `json:"parent_id,omitempty"`
	ToolName  string        `json:"tool_name"`
	Input     any           `json:"input,omitempty"`
	Output    any           `json:"output,omitempty"`
	Duration  time.Duration `json:"duration"`
	Err       error         `json:"-"`
	ErrorStr  string        `json:"error,omitempty"`
	Children  []string      `json:"children,omitempty"`
	Timestamp time.Time     `json:"timestamp"`
	Depth     int           `json:"depth"`
}

// ToMap 转换为 map（用于序列化）
func (t *ToolCallTrace) ToMap() map[string]any {
	m := map[string]any{
		"id":        t.ID,
		"tool_name": t.ToolName,
		"duration":  t.Duration.Milliseconds(),
		"timestamp": t.Timestamp.Format(time.RFC3339),
		"depth":     t.Depth,
	}
	if t.ParentID != "" {
		m["parent_id"] = t.ParentID
	}
	if t.Input != nil {
		m["input"] = t.Input
	}
	if t.Output != nil {
		m["output"] = t.Output
	}
	if t.Err != nil {
		m["error"] = t.Err.Error()
	}
	if len(t.Children) > 0 {
		m["children"] = t.Children
	}
	return m
}

// TraceCollector 工具调用链收集器接口
type TraceCollector interface {
	Record(call *ToolCallTrace) error
	GetTrace(traceID string) (*ToolCallTrace, error)
	GetChildren(parentID string) ([]*ToolCallTrace, error)
}

// InMemoryTraceCollector 内存实现
type InMemoryTraceCollector struct {
	mu       sync.RWMutex
	traces   map[string]*ToolCallTrace
	children map[string][]string
	maxDepth int
}

// NewTraceCollector 创建调用链收集器
func NewTraceCollector() *InMemoryTraceCollector {
	return &InMemoryTraceCollector{
		traces:   make(map[string]*ToolCallTrace),
		children: make(map[string][]string),
		maxDepth: MaxTraceDepth,
	}
}

// Record 记录一次工具调用
func (c *InMemoryTraceCollector) Record(call *ToolCallTrace) error {
	if call.ID == "" {
		return errors.New("trace ID cannot be empty")
	}
	if call.Depth > c.maxDepth {
		return ErrTraceTooDeep
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if call.Err != nil {
		call.ErrorStr = call.Err.Error()
	}
	c.traces[call.ID] = call

	if call.ParentID != "" {
		c.children[call.ParentID] = append(c.children[call.ParentID], call.ID)
		if parent, ok := c.traces[call.ParentID]; ok {
			parent.Children = append(parent.Children, call.ID)
		}
	}
	return nil
}

// GetTrace 获取单条追踪记录
func (c *InMemoryTraceCollector) GetTrace(traceID string) (*ToolCallTrace, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	call, ok := c.traces[traceID]
	if !ok {
		return nil, ErrTraceNotFound
	}
	return call, nil
}

// GetChildren 获取子调用列表
func (c *InMemoryTraceCollector) GetChildren(parentID string) ([]*ToolCallTrace, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	childIDs, ok := c.children[parentID]
	if !ok {
		return nil, nil
	}
	result := make([]*ToolCallTrace, 0, len(childIDs))
	for _, id := range childIDs {
		if call, ok := c.traces[id]; ok {
			result = append(result, call)
		}
	}
	return result, nil
}

// AllTraces 返回所有追踪记录
func (c *InMemoryTraceCollector) AllTraces() []*ToolCallTrace {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]*ToolCallTrace, 0, len(c.traces))
	for _, call := range c.traces {
		result = append(result, call)
	}
	return result
}

// TraceCount 返回追踪记录总数
func (c *InMemoryTraceCollector) TraceCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.traces)
}

// Truncate 清空所有追踪记录
func (c *InMemoryTraceCollector) Truncate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.traces = make(map[string]*ToolCallTrace)
	c.children = make(map[string][]string)
}

// contextKeyTrace context key 类型
type contextKeyTrace struct{}

// WithTrace 将 traceID 注入 context
func WithTrace(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, contextKeyTrace{}, traceID)
}

// FromTrace 从 context 提取 traceID
func FromTrace(ctx context.Context) string {
	if v, ok := ctx.Value(contextKeyTrace{}).(string); ok {
		return v
	}
	return ""
}

// TraceContext 封装追踪上下文
type TraceContext struct {
	collector *InMemoryTraceCollector
	currentID string
	parentID  string
	depth     int
}

// NewTraceContext 创建追踪上下文
func NewTraceContext(collector *InMemoryTraceCollector) *TraceContext {
	return &TraceContext{
		collector: collector,
		depth:     0,
	}
}

// StartCall 记录一个工具调用的开始
func (tc *TraceContext) StartCall(traceID, toolName string, input any) *ToolCallTrace {
	trace := &ToolCallTrace{
		ID:        traceID,
		ParentID:  tc.parentID,
		ToolName:  toolName,
		Input:     input,
		Depth:     tc.depth,
		Timestamp: time.Now(),
	}
	tc.collector.Record(trace)
	return trace
}

// EndCall 记录工具调用的结束
func (tc *TraceContext) EndCall(trace *ToolCallTrace, output any, err error) {
	trace.Duration = time.Since(trace.Timestamp)
	trace.Output = output
	trace.Err = err
	tc.collector.Record(trace)
}

// ChildContext 创建子调用的追踪上下文
func (tc *TraceContext) ChildContext(childTraceID string) *TraceContext {
	return &TraceContext{
		collector: tc.collector,
		currentID: childTraceID,
		parentID:  tc.currentID,
		depth:     tc.depth + 1,
	}
}
package agent

import (
	"agentprimordia/internal/llm"
	"context"
	"fmt"
	"time"
)

// ===== RAG 接口 =====

// RAGDocument 是 RAG 检索返回的文档片段
type RAGDocument struct {
	ID      string  `json:"id"`
	Content string  `json:"content"`
	Score   float32 `json:"score"`
	Source  string  `json:"source,omitempty"` // "fts" 和/或 "vector"
	Role    string  `json:"role,omitempty"`   // 原始角色 (user/assistant)
}

// RAGProvider 是 Agent 可使用的 RAG 检索接口
// 由 memory.RAGStore 通过 pkg/adapters 适配实现
type RAGProvider interface {
	// Search 执行 RAG 检索，query 为查询文本，topK 为返回结果数
	Search(ctx context.Context, query string, topK int) ([]*RAGDocument, error)
}

// RAGMode 控制 RAG 在 ReAct Loop 中的注入方式
type RAGMode string

const (
	// RAGModeAuto 在每轮推理前自动查询知识库并注入上下文
	RAGModeAuto RAGMode = "auto"
	// RAGModeFirst 仅在第一轮推理前查询知识库
	RAGModeFirst RAGMode = "first"
	// RAGModeOnDemand 仅当 Agent 主动调用 knowledge_search 工具时查询
	RAGModeOnDemand RAGMode = "on_demand"
)

// RAGConfig 配置 RAG 注入行为
type RAGConfig struct {
	// Provider RAG 检索提供者
	Provider RAGProvider

	// Mode 注入模式，默认 auto
	Mode RAGMode

	// TopK 每次检索返回的最大文档数，默认 5
	TopK int

	// MinScore 最低相关度阈值，低于此值的结果将被过滤，默认 0.3
	MinScore float32

	// ContextTemplate 上下文注入模板，默认使用 FormatRAGContext
	// 可用占位符: {{context}}
	ContextTemplate string
}

// FormatRAGDocuments 将 RAG 检索结果格式化为可注入 Prompt 的上下文文本
func FormatRAGDocuments(docs []*RAGDocument) string {
	if len(docs) == 0 {
		return ""
	}
	result := "=== 相关知识 ===\n"
	for i, doc := range docs {
		role := doc.Role
		if role == "" {
			role = "知识"
		}
		result += fmt.Sprintf("[%d | 相关度: %.2f | %s] %s\n", i+1, doc.Score, role, doc.Content)
	}
	result += "=== 知识结束 ===\n"
	return result
}

// RAGContextForPrompt 将单个 RAGDocument 格式化为 Prompt 上下文
func (d *RAGDocument) RAGContextForPrompt() string {
	role := d.Role
	if role == "" {
		role = "知识"
	}
	return fmt.Sprintf("[相关度: %.2f | %s] %s", d.Score, role, d.Content)
}

// Agent 是所有 Agent 实现的核心接口
// 编排模式（Pipeline/Handoff/Parallel）和 Pool 均面向此接口编程
type Agent interface {
	// Run 执行同步推理，接收 Message 输入并返回 Response
	Run(ctx context.Context, input Message) (*Response, error)
	// StreamRun 执行流式推理，返回 StreamEvent 通道
	StreamRun(ctx context.Context, input Message) (<-chan StreamEvent, error)
	// Stop 停止当前运行
	Stop()
	// Stats 返回运行统计
	Stats() AgentStats
	// Name 返回 Agent 名称
	Name() string
}

// Role represents the role of a message sender
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// ContentPart 消息内容片段（多模态）
type ContentPart struct {
	Type   string `json:"type"`
	Text   string `json:"text,omitempty"`
	URL    string `json:"url,omitempty"`
	Data   string `json:"data,omitempty"`
	MIME   string `json:"mime,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// Message represents a single message in the conversation
type Message struct {
	Role         Role          `json:"role"`
	Content      string        `json:"content"`
	ContentParts []ContentPart `json:"content_parts,omitempty"`
	ToolCalls    []ToolCall    `json:"tool_calls,omitempty"`
	Metadata     Metadata      `json:"metadata,omitempty"`
}

// HasMultimodal 判断消息是否包含多模态内容
func (m *Message) HasMultimodal() bool {
	for _, p := range m.ContentParts {
		if p.Type != "text" {
			return true
		}
	}
	return false
}

// TextContent 提取纯文本内容
func (m *Message) TextContent() string {
	if m.Content != "" && len(m.ContentParts) == 0 {
		return m.Content
	}
	result := ""
	for _, p := range m.ContentParts {
		if p.Type == "text" && p.Text != "" {
			if result != "" {
				result += " "
			}
			result += p.Text
		}
	}
	if result == "" {
		return m.Content
	}
	return result
}

// Metadata carries additional message information
type Metadata struct {
	SessionID string            `json:"session_id,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
	Extra     map[string]string `json:"extra,omitempty"`
}

// UserMessage creates a user message helper
func UserMessage(content string) Message {
	return Message{
		Role:     RoleUser,
		Content:  content,
		Metadata: Metadata{Timestamp: time.Now()},
	}
}

// SystemMessage creates a system message helper
func SystemMessage(content string) Message {
	return Message{
		Role:     RoleSystem,
		Content:  content,
		Metadata: Metadata{Timestamp: time.Now()},
	}
}

// ToolCall represents a function call request from LLM
type ToolCall struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Args string `json:"args"` // JSON-encoded arguments
}

// ToolResult represents the result of executing a tool
type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
	IsError    bool   `json:"is_error"`
}

// ToMessage converts ToolResult to a Message with RoleTool
func (tr *ToolResult) ToMessage() Message {
	extra := map[string]string{"tool_call_id": tr.ToolCallID}
	if tr.IsError {
		extra["is_error"] = "true"
	}
	return Message{
		Role:    RoleTool,
		Content: tr.Content,
		Metadata: Metadata{
			Extra: extra,
		},
	}
}

// Thought represents the LLM's reasoning output
type Thought struct {
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	Usage     llm.Usage  `json:"usage,omitempty"`
}

// Response represents the final response from an Agent
type Response struct {
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	Usage     Usage      `json:"usage"`
	Metrics   Metrics    `json:"metrics"`
	Error     error      `json:"-"`
}

// Usage tracks token usage
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Metrics tracks performance metrics
type Metrics struct {
	TotalTurns  int           `json:"total_turns"`
	TotalTools  int           `json:"total_tools_called"`
	Duration    time.Duration `json:"duration"`
	LLMLatency  time.Duration `json:"llm_latency_ms"`
	ToolLatency time.Duration `json:"tool_latency_ms"`
}

// AgentStatus represents the current state of an agent
type AgentStatus string

const (
	StatusIdle            AgentStatus = "idle"
	StatusRunning         AgentStatus = "running"
	StatusPaused          AgentStatus = "paused"
	StatusWaitingForInput AgentStatus = "waiting_for_input"
	StatusCompleted       AgentStatus = "completed"
	StatusFailed          AgentStatus = "failed"
	StatusCancelled       AgentStatus = "cancelled"
)

// AgentStats provides runtime statistics about an agent
type AgentStats struct {
	Status        AgentStatus    `json:"status"`
	CurrentTurn   int            `json:"current_turn"`
	TotalMessages int            `json:"total_messages"`
	ToolsCalled   map[string]int `json:"tools_called"`
	StartTime     time.Time      `json:"start_time"`
}

// StreamEventType 标识流式事件的类型
type StreamEventType string

const (
	StreamEventToken      StreamEventType = "token"       // 逐 token 输出
	StreamEventThought    StreamEventType = "thought"     // 思考/推理
	StreamEventToolCall   StreamEventType = "tool_call"   // 工具调用开始
	StreamEventToolResult StreamEventType = "tool_result" // 工具执行结果
	StreamEventComplete   StreamEventType = "complete"    // 运行完成
	StreamEventError      StreamEventType = "error"       // 错误
)

// StreamEvent 是流式输出的事件
type StreamEvent struct {
	Type    StreamEventType `json:"type"`
	Content string          `json:"content,omitempty"`
	Data    any             `json:"data,omitempty"`
}

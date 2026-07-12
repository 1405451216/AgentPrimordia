// Package protocol 提供 AgentPrimordia 统一协议层。
//
// 本包定义了跨语言（Go / TypeScript）共享的消息类型和序列化格式，
// 确保两端对同一份 JSON 数据的编解码结果严格兼容。
//
// 设计原则：
//   - 所有类型使用纯 struct + json tag，不依赖 protobuf 编译器；
//   - Go 端字段名必须与 TS 端字段名一一对应（camelCase）；
//   - omitempty 规则与 TS 端 optional 字段对齐。
package protocol

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ===== 通用常量 =====

// ProtocolVersion 为当前协议版本号。
const ProtocolVersion = "1.0.0"

// 角色常量，与 TS 端枚举对齐。
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// ===== Agent 消息 =====

// AgentMessage 统一 Agent 消息格式。
// 对应 TS 端 AgentMessage 接口。
type AgentMessage struct {
	ID        string            `json:"id"`
	Role      string            `json:"role"` // system / user / assistant / tool
	Content   string            `json:"content"`
	ToolCalls []ToolCall        `json:"tool_calls,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Timestamp int64             `json:"timestamp"`
}

// ToolCall 表示一次工具调用请求。
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON 字符串
}

// ToolResult 表示工具执行结果。
type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Result     string `json:"result"` // JSON 字符串
	IsError    bool   `json:"is_error"`
}

// ===== 记忆消息 =====

// MemoryEntry 表示一条记忆条目。
type MemoryEntry struct {
	ID        string            `json:"id"`
	Topic     string            `json:"topic"`
	Content   string            `json:"content"`
	Importance float64          `json:"importance"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	CreatedAt int64             `json:"created_at"`
}

// MemoryQuery 表示记忆查询请求。
type MemoryQuery struct {
	Topic    string            `json:"topic,omitempty"`
	Keyword  string            `json:"keyword,omitempty"`
	TopK     int               `json:"top_k"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// ===== 事件消息 =====

// EventMessage 表示系统内部事件。
type EventMessage struct {
	ID        string            `json:"id"`
	Type      string            `json:"type"` // tool_call / tool_result / error / lifecycle
	Source    string            `json:"source"`
	Payload   string            `json:"payload"` // JSON 字符串
	Timestamp int64             `json:"timestamp"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// ===== 序列化辅助 =====

// ParseError 表示反序列化失败的详细信息。
type ParseError struct {
	Field   string
	Message string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("protocol: parse error on field %q: %s", e.Field, e.Message)
}

// GenerateID 生成 16 字节随机 hex 字符串作为消息 ID。
func GenerateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Now 返回当前 Unix 毫秒时间戳。
func Now() int64 {
	return time.Now().UnixMilli()
}

// ToJSON 将 AgentMessage 序列化为 JSON 字节。
func (m *AgentMessage) ToJSON() ([]byte, error) {
	return json.Marshal(m)
}

// AgentMessageFromJSON 从 JSON 字节反序列化为 AgentMessage。
func AgentMessageFromJSON(data []byte) (*AgentMessage, error) {
	var m AgentMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, &ParseError{Field: "AgentMessage", Message: err.Error()}
	}
	return &m, nil
}

// Validate 校验消息必填字段。
func (m *AgentMessage) Validate() error {
	if m.ID == "" {
		return &ParseError{Field: "id", Message: "cannot be empty"}
	}
	switch m.Role {
	case RoleSystem, RoleUser, RoleAssistant, RoleTool:
		// ok
	default:
		return &ParseError{Field: "role", Message: "must be one of system/user/assistant/tool"}
	}
	if m.Content == "" && len(m.ToolCalls) == 0 {
		return &ParseError{Field: "content", Message: "cannot be empty when tool_calls is absent"}
	}
	for i, tc := range m.ToolCalls {
		if tc.ID == "" {
			return &ParseError{Field: fmt.Sprintf("tool_calls[%d].id", i), Message: "cannot be empty"}
		}
		if tc.Name == "" {
			return &ParseError{Field: fmt.Sprintf("tool_calls[%d].name", i), Message: "cannot be empty"}
		}
	}
	return nil
}

// Validate 校验工具调用。
func (tc *ToolCall) Validate() error {
	if tc.ID == "" {
		return &ParseError{Field: "tool_call.id", Message: "cannot be empty"}
	}
	if tc.Name == "" {
		return &ParseError{Field: "tool_call.name", Message: "cannot be empty"}
	}
	return nil
}

// Validate 校验工具结果。
func (tr *ToolResult) Validate() error {
	if tr.ToolCallID == "" {
		return &ParseError{Field: "tool_result.tool_call_id", Message: "cannot be empty"}
	}
	return nil
}

// Validate 校验事件消息。
func (e *EventMessage) Validate() error {
	if e.ID == "" {
		return &ParseError{Field: "event.id", Message: "cannot be empty"}
	}
	if e.Type == "" {
		return &ParseError{Field: "event.type", Message: "cannot be empty"}
	}
	return nil
}

// ===== JSON 兼容工具 =====

// CompactJSON 移除 JSON 字符串中不影响语义的空白字符。
// 用于跨语言对比测试中消除格式差异。
func CompactJSON(s string) string {
	var sb strings.Builder
	sb.Grow(len(s))
	inString := false
	escaped := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inString {
			sb.WriteByte(c)
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == '"' {
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
			sb.WriteByte(c)
		case ' ', '\t', '\n', '\r':
			// 跳过空白
		default:
			sb.WriteByte(c)
		}
	}
	return sb.String()
}

// ParseTimestamp 从支持 Unix 毫秒（int64）或 ISO 字符串的时间戳字段解析。
// 用于兼容 TS 端 number 与 Go 端 int64。
func ParseTimestamp(v string) (int64, error) {
	// 尝试整数
	if n, err := strconv.ParseInt(v, 10, 64); err == nil {
		return n, nil
	}
	// 尝试 RFC3339
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t.UnixMilli(), nil
	}
	return 0, fmt.Errorf("protocol: cannot parse timestamp %q", v)
}

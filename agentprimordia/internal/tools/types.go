package tools

import (
	"context"
	"encoding/json"
)

// Tool 是所有工具必须实现的接口
type Tool interface {
	Name() string
	Description() string
	Parameters() json.RawMessage
	Execute(ctx context.Context, args json.RawMessage) (*Result, error)
}

// Result 表示工具执行的结果
type Result struct {
	Content  string         `json:"content"`
	IsError  bool           `json:"is_error"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// NewResult 创建一个成功的结果
func NewResult(content string) *Result {
	return &Result{Content: content, IsError: false}
}

// NewErrorResult 创建一个错误结果
func NewErrorResult(content string) *Result {
	return &Result{Content: content, IsError: true}
}

// ConfirmationFunc 是工具执行前的确认回调
// 返回 true 表示允许执行，false 表示拒绝
type ConfirmationFunc func(toolName string, args json.RawMessage) bool

// Permission 定义工具的访问控制
type Permission struct {
	AllowedRoles        []string         `json:"allowed_roles,omitempty"`
	BlockedPaths        []string         `json:"blocked_paths,omitempty"`
	RequireConfirmation bool             `json:"require_confirmation,omitempty"`
	ConfirmFunc         ConfirmationFunc `json:"-"`
}

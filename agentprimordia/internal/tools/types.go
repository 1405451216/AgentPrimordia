package tools

import (
	"context"
	"encoding/json"
)

// Tool is the interface that all tools must implement
type Tool interface {
	Name() string
	Description() string
	Parameters() json.RawMessage
	Execute(ctx context.Context, args json.RawMessage) (*Result, error)
}

// Result represents the outcome of a tool execution
type Result struct {
	Content  string                 `json:"content"`
	IsError  bool                   `json:"is_error"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// NewResult creates a successful result
func NewResult(content string) *Result {
	return &Result{Content: content, IsError: false}
}

// NewErrorResult creates an error result
func NewErrorResult(content string) *Result {
	return &Result{Content: content, IsError: true}
}

// ConfirmationFunc 是工具执行前的确认回调
// 返回 true 表示允许执行，false 表示拒绝
type ConfirmationFunc func(toolName string, args json.RawMessage) bool

// Permission defines access control for a tool
type Permission struct {
	AllowedRoles        []string         `json:"allowed_roles,omitempty"`
	BlockedPaths        []string         `json:"blocked_paths,omitempty"`
	RequireConfirmation bool             `json:"require_confirmation,omitempty"`
	ConfirmFunc         ConfirmationFunc `json:"-"`
}

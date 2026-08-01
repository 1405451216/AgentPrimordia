// Package hitl 提供人机协作（Human-in-the-Loop）管理
package hitl

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// ErrHumanChannelClosed human input channel closed错误
var ErrHumanChannelClosed = errors.New("human input channel closed")

// InterruptReason 中断原因
type InterruptReason string

const (
	InterruptToolConfirm   InterruptReason = "tool_confirm"
	InterruptDecisionPoint InterruptReason = "decision_point"
	InterruptBudgetExceed  InterruptReason = "budget_exceed"
	InterruptCustom        InterruptReason = "custom"
)

// InterruptPoint 中断点配置
type InterruptPoint struct {
	Type     InterruptReason
	ToolName string
	Message  string
}

// InterruptRequest 中断请求（Agent 发出）
type InterruptRequest struct {
	Reason  InterruptReason `json:"reason"`
	Message string          `json:"message"`
	Data    map[string]any  `json:"data,omitempty"`
	Turn    int             `json:"turn"`
}

// HumanResponse 人类响应
type HumanResponse struct {
	Approved bool           `json:"approved"`
	Input    string         `json:"input,omitempty"`
	Modified map[string]any `json:"modified,omitempty"`
}

// HITLConfig 人机协作配置
type HITLConfig struct {
	InterruptPoints  []InterruptPoint
	HumanInputChan   <-chan *HumanResponse
	OnInterrupt      func(req *InterruptRequest)
	AutoApproveTools []string
}

// HITLManager 人机协作管理器
type HITLManager struct {
	config     HITLConfig
	pending    *InterruptRequest
	responseCh chan *HumanResponse
	mu         sync.RWMutex
}

// NewHITLManager 创建人机协作管理器
func NewHITLManager(config HITLConfig) *HITLManager {
	responseCh := make(chan *HumanResponse, 8)

	mgr := &HITLManager{
		config:     config,
		responseCh: responseCh,
	}

	return mgr
}

// ShouldInterrupt 判断当前操作是否需要中断
func (m *HITLManager) ShouldInterrupt(toolName string, reason InterruptReason) bool {
	if m.isAutoApproved(toolName) {
		return false
	}

	for _, ip := range m.config.InterruptPoints {
		if ip.Type != reason {
			continue
		}
		if ip.ToolName == "" || ip.ToolName == toolName {
			return true
		}
	}
	return false
}

// isAutoApproved 检查tool是否在自动批准列表中
func (m *HITLManager) isAutoApproved(toolName string) bool {
	for _, name := range m.config.AutoApproveTools {
		if name == toolName {
			return true
		}
	}
	return false
}

// RequestInterrupt 发起中断请求，阻塞等待人类响应
func (m *HITLManager) RequestInterrupt(ctx context.Context, req *InterruptRequest) (*HumanResponse, error) {
	m.mu.Lock()
	m.pending = req
	m.mu.Unlock()

	if m.config.OnInterrupt != nil {
		m.config.OnInterrupt(req)
	}

	var resp *HumanResponse

	select {
	case r, ok := <-m.config.HumanInputChan:
		if !ok {
			return nil, ErrHumanChannelClosed
		}
		resp = r
	case r, ok := <-m.responseCh:
		if !ok {
			return nil, fmt.Errorf("response channel closed")
		}
		resp = r
	case <-ctx.Done():
		m.mu.Lock()
		m.pending = nil
		m.mu.Unlock()
		return nil, fmt.Errorf("timed out waiting for human response: %w", ctx.Err())
	}

	m.mu.Lock()
	m.pending = nil
	m.mu.Unlock()

	return resp, nil
}

// Resume 恢复 Agent 执行（外部调用）
func (m *HITLManager) Resume(response *HumanResponse) {
	m.responseCh <- response
}

// Pending 返回当前挂起的中断请求
func (m *HITLManager) Pending() *InterruptRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.pending
}

// failure.go — 失败记录与失败存储（v3.4-6）
// FailureRecord 捕获一次 Agent 运行失败的完整上下文：错误信息、失败阶段、
// 失败时的可恢复检查点（State）。配合 FailureStore 与 Agent 的 ReplayFailure，
// 实现「任意失败可一键重放定位」。
package persist

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// 失败阶段常量
const (
	// PhaseRun 普通 ReAct 循环执行阶段失败
	PhaseRun = "run"
	// PhasePlan 计划（executePlan）执行阶段失败
	PhasePlan = "plan"
)

// FailureRecord 一次失败的完整记录。
// State 非空表示携带可恢复检查点，支持一键重放。
type FailureRecord struct {
	ID        string      `json:"id"`
	AgentID   string      `json:"agent_id"`
	SessionID string      `json:"session_id"`
	Phase     string      `json:"phase"`               // run | plan
	Error     string      `json:"error"`               // 失败错误信息
	Turn      int         `json:"turn"`                // 失败时已执行的轮次
	SubTaskID string      `json:"subtask_id,omitempty"` // plan 阶段失败的子任务 ID
	Input     string      `json:"input,omitempty"`      // 触发本次运行的用户输入
	State     *AgentState `json:"state,omitempty"`      // 失败时的检查点快照（可重放）
	CreatedAt time.Time   `json:"created_at"`
}

// Diagnose 生成人类可读的失败诊断摘要：失败在哪、为何失败、能否重放。
func (r *FailureRecord) Diagnose() string {
	var b strings.Builder
	b.WriteString("失败诊断:\n")
	fmt.Fprintf(&b, "  Agent: %s", r.AgentID)
	if r.SessionID != "" {
		fmt.Fprintf(&b, "（会话 %s）", r.SessionID)
	}
	b.WriteString("\n")

	if r.Phase == PhasePlan && r.SubTaskID != "" {
		fmt.Fprintf(&b, "  阶段: plan（子任务 %s，第 %d 轮）\n", r.SubTaskID, r.Turn)
	} else {
		phase := r.Phase
		if phase == "" {
			phase = PhaseRun
		}
		fmt.Fprintf(&b, "  阶段: %s（第 %d 轮）\n", phase, r.Turn)
	}

	fmt.Fprintf(&b, "  错误: %s\n", r.Error)
	if r.Input != "" {
		input := r.Input
		if len(input) > 120 {
			input = input[:120] + "..."
		}
		fmt.Fprintf(&b, "  输入: %s\n", input)
	}

	if r.State != nil {
		b.WriteString("  重放: 可重放（已嵌入失败时检查点）\n")
	} else {
		b.WriteString("  重放: 不可重放（无检查点快照）\n")
	}
	return b.String()
}

// FailureStore 失败记录存储接口
type FailureStore interface {
	// Record 保存一条失败记录
	Record(ctx context.Context, rec *FailureRecord) error
	// Get 按 ID 读取失败记录
	Get(ctx context.Context, id string) (*FailureRecord, error)
	// List 列出失败记录；agentID 为空时返回全部，否则按 Agent 过滤（新→旧）
	List(ctx context.Context, agentID string) ([]*FailureRecord, error)
	// Delete 删除失败记录
	Delete(ctx context.Context, id string) error
}

// MemoryFailureStore 内存实现的 FailureStore（并发安全）。
// 生产环境可自行实现基于 SQLite/etcd 的持久化版本。
type MemoryFailureStore struct {
	mu      sync.RWMutex
	records map[string]*FailureRecord
	order   []string // 按记录顺序保存 ID，List 时倒序返回（新→旧）
}

// NewMemoryFailureStore 创建内存失败存储
func NewMemoryFailureStore() *MemoryFailureStore {
	return &MemoryFailureStore{
		records: make(map[string]*FailureRecord),
	}
}

// Record 保存失败记录（要求 ID 非空）
func (s *MemoryFailureStore) Record(_ context.Context, rec *FailureRecord) error {
	if rec == nil || rec.ID == "" {
		return fmt.Errorf("failure record requires non-empty id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.records[rec.ID]; !exists {
		s.order = append(s.order, rec.ID)
	}
	s.records[rec.ID] = rec
	return nil
}

// Get 按 ID 读取失败记录
func (s *MemoryFailureStore) Get(_ context.Context, id string) (*FailureRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.records[id]
	if !ok {
		return nil, fmt.Errorf("failure record %q not found", id)
	}
	return rec, nil
}

// List 列出失败记录（新→旧）；agentID 为空时返回全部
func (s *MemoryFailureStore) List(_ context.Context, agentID string) ([]*FailureRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*FailureRecord, 0, len(s.order))
	for i := len(s.order) - 1; i >= 0; i-- {
		rec := s.records[s.order[i]]
		if rec == nil {
			continue
		}
		if agentID != "" && rec.AgentID != agentID {
			continue
		}
		out = append(out, rec)
	}
	return out, nil
}

// Delete 删除失败记录
func (s *MemoryFailureStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.records[id]; !ok {
		return fmt.Errorf("failure record %q not found", id)
	}
	delete(s.records, id)
	for i, oid := range s.order {
		if oid == id {
			s.order = append(s.order[:i], s.order[i+1:]...)
			break
		}
	}
	return nil
}

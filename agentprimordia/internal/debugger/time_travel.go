package debugger

import (
	"fmt"
	"sync"
)

// StateSnapshot 表示某一 turn 的 Agent 状态快照，用于时间旅行调试
type StateSnapshot struct {
	Turn  int
	State *AgentState
}

// TimeTravelDebugger 记录每个 turn 的状态快照，支持前后回放
type TimeTravelDebugger struct {
	mu        sync.RWMutex
	snapshots []StateSnapshot // 每 turn 快照记录
	maxSize   int             // 最大快照数量限制
	cursor    int             // 当前回放位置（snapshots 中的索引）
}

// NewTimeTravelDebugger 创建 TimeTravelDebugger 实例
// maxSize 表示最多保留的快照数量，超出时自动淘汰最早的快照
func NewTimeTravelDebugger(maxSize int) *TimeTravelDebugger {
	if maxSize <= 0 {
		maxSize = 1000
	}
	return &TimeTravelDebugger{
		snapshots: make([]StateSnapshot, 0, maxSize),
		maxSize:   maxSize,
		cursor:    -1,
	}
}

// Record 记录某个 turn 的状态快照（深拷贝，避免外部修改污染快照）
func (tt *TimeTravelDebugger) Record(turn int, state *AgentState) {
	if tt == nil {
		return
	}
	tt.mu.Lock()
	defer tt.mu.Unlock()

	snapshot := StateSnapshot{
		Turn:  turn,
		State: cloneAgentState(state),
	}

	tt.snapshots = append(tt.snapshots, snapshot)

	// 超出最大限制时淘汰最早的快照
	if len(tt.snapshots) > tt.maxSize {
		tt.snapshots = tt.snapshots[len(tt.snapshots)-tt.maxSize:]
	}

	// 更新 cursor 到最新位置
	tt.cursor = len(tt.snapshots) - 1
}

// Restore 恢复到指定 turn 的状态
func (tt *TimeTravelDebugger) Restore(turn int) (*AgentState, error) {
	if tt == nil {
		return nil, fmt.Errorf("TimeTravelDebugger is nil")
	}
	tt.mu.Lock()
	defer tt.mu.Unlock()

	for i, snap := range tt.snapshots {
		if snap.Turn == turn {
			tt.cursor = i
			return snap.State, nil
		}
	}
	return nil, fmt.Errorf("state for turn %d not found", turn)
}

// StepForward 前进一步，返回下一个 turn 的状态
func (tt *TimeTravelDebugger) StepForward() (*AgentState, error) {
	if tt == nil {
		return nil, fmt.Errorf("TimeTravelDebugger is nil")
	}
	tt.mu.Lock()
	defer tt.mu.Unlock()

	if tt.cursor < 0 {
		return nil, fmt.Errorf("no current state, call Restore first")
	}
	if tt.cursor >= len(tt.snapshots)-1 {
		return nil, fmt.Errorf("already at latest snapshot (turn %d)", tt.snapshots[tt.cursor].Turn)
	}
	tt.cursor++
	return tt.snapshots[tt.cursor].State, nil
}

// StepBackward 后退一步，返回上一个 turn 的状态
func (tt *TimeTravelDebugger) StepBackward() (*AgentState, error) {
	if tt == nil {
		return nil, fmt.Errorf("TimeTravelDebugger is nil")
	}
	tt.mu.Lock()
	defer tt.mu.Unlock()

	if tt.cursor < 0 {
		return nil, fmt.Errorf("no current state, call Restore first")
	}
	if tt.cursor == 0 {
		return nil, fmt.Errorf("already at earliest snapshot (turn %d)", tt.snapshots[tt.cursor].Turn)
	}
	tt.cursor--
	return tt.snapshots[tt.cursor].State, nil
}

// GetCurrent 获取当前 cursor 位置的快照状态
func (tt *TimeTravelDebugger) GetCurrent() *AgentState {
	if tt == nil {
		return nil
	}
	tt.mu.RLock()
	defer tt.mu.RUnlock()

	if tt.cursor < 0 || tt.cursor >= len(tt.snapshots) {
		return nil
	}
	return tt.snapshots[tt.cursor].State
}

// GetSnapshots 返回所有快照的拷贝
func (tt *TimeTravelDebugger) GetSnapshots() []StateSnapshot {
	if tt == nil {
		return nil
	}
	tt.mu.RLock()
	defer tt.mu.RUnlock()

	result := make([]StateSnapshot, len(tt.snapshots))
	copy(result, tt.snapshots)
	return result
}

// GetCursor 返回当前 cursor 所指的 turn
func (tt *TimeTravelDebugger) GetCursorTurn() int {
	if tt == nil {
		return -1
	}
	tt.mu.RLock()
	defer tt.mu.RUnlock()

	if tt.cursor < 0 || tt.cursor >= len(tt.snapshots) {
		return -1
	}
	return tt.snapshots[tt.cursor].Turn
}

// Len 返回已记录的快照数量
func (tt *TimeTravelDebugger) Len() int {
	if tt == nil {
		return 0
	}
	tt.mu.RLock()
	defer tt.mu.RUnlock()
	return len(tt.snapshots)
}

// Clear 清除所有快照记录
func (tt *TimeTravelDebugger) Clear() {
	if tt == nil {
		return
	}
	tt.mu.Lock()
	defer tt.mu.Unlock()
	tt.snapshots = tt.snapshots[:0]
	tt.cursor = -1
}

// cloneAgentState 深拷贝 AgentState，确保快照不受外部修改影响。
func cloneAgentState(s *AgentState) *AgentState {
	if s == nil {
		return nil
	}
	clone := &AgentState{
		Turn:   s.Turn,
		Status: s.Status,
	}
	if s.Memory != nil {
		m := *s.Memory
		clone.Memory = &m
	}
	if len(s.Attributes) > 0 {
		clone.Attributes = make(map[string]interface{}, len(s.Attributes))
		for k, v := range s.Attributes {
			clone.Attributes[k] = v
		}
	}
	return clone
}

package debugger

import (
	"fmt"
	"reflect"
	"sync"
)

// WatchVar 表示一个被监视的变量
type WatchVar struct {
	Name      string             // 变量名称
	Path      string             // 变量路径（例如 "state.memory.latest"）
	LastValue any                // 上一次的值
	OnChange  func(old, new any) // 变化时的回调
	mu        sync.Mutex
}

// Update 更新变量值，如果值发生变化则触发 OnChange 回调
// 使用 reflect.DeepEqual 进行值比较
func (wv *WatchVar) Update(newValue any) {
	if wv == nil {
		return
	}
	wv.mu.Lock()
	defer wv.mu.Unlock()

	if !reflect.DeepEqual(wv.LastValue, newValue) {
		old := wv.LastValue
		wv.LastValue = newValue
		if wv.OnChange != nil {
			wv.OnChange(old, newValue)
		}
	}
}

// WatchManager 管理一组监视变量，支持批量轮询
type WatchManager struct {
	mu     sync.RWMutex
	watches []*WatchVar
}

// NewWatchManager 创建一个新的监视管理器
func NewWatchManager() *WatchManager {
	return &WatchManager{
		watches: make([]*WatchVar, 0),
	}
}

// Add 注册一个监视变量
func (m *WatchManager) Add(wv *WatchVar) {
	if wv == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.watches = append(m.watches, wv)
}

// Remove 按名称移除一个监视变量
func (m *WatchManager) Remove(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, wv := range m.watches {
		if wv.Name == name {
			m.watches = append(m.watches[:i], m.watches[i+1:]...)
			return
		}
	}
}

// Poll 轮询所有监视变量，从 AgentState 中提取最新值
// 目前支持的路径格式：
//   - "state.Turn"         -> agentState.Turn
//   - "state.Status"       -> agentState.Status
//   - "state.Memory.Latest"-> agentState.Memory.Latest
//   - "state.Memory.Summary"-> agentState.Memory.Summary
//   - "state.Memory.Episodes"-> agentState.Memory.Episodes
func (m *WatchManager) Poll(state *AgentState) {
	m.mu.RLock()
	watches := make([]*WatchVar, len(m.watches))
	copy(watches, m.watches)
	m.mu.RUnlock()

	for _, wv := range watches {
		val := extractValue(state, wv.Path)
		wv.Update(val)
	}
}

// GetWatches 返回所有监视变量的快照
func (m *WatchManager) GetWatches() []*WatchVar {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*WatchVar, len(m.watches))
	copy(result, m.watches)
	return result
}

// Clear 清除所有监视变量
func (m *WatchManager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.watches = m.watches[:0]
}

// extractValue 从 AgentState 中提取指定路径的值
// 支持格式： "state.Turn", "state.Memory.Latest" 等
func extractValue(state *AgentState, path string) any {
	if state == nil {
		return nil
	}

	switch path {
	case "state.Turn":
		return state.Turn
	case "state.Status":
		return state.Status
	case "state.Memory.Latest":
		if state.Memory != nil {
			return state.Memory.Latest
		}
	case "state.Memory.Summary":
		if state.Memory != nil {
			return state.Memory.Summary
		}
	case "state.Memory.Episodes":
		if state.Memory != nil {
			return state.Memory.Episodes
		}
	default:
		// 尝试从 Attributes 中提取
		if state.Attributes != nil {
			if v, ok := state.Attributes[path]; ok {
				return v
			}
		}
	}
	return nil
}

// PollWithValues 允许外部直接传入 path->value 映射进行轮询
func (m *WatchManager) PollWithValues(values map[string]any) {
	m.mu.RLock()
	watches := make([]*WatchVar, len(m.watches))
	copy(watches, m.watches)
	m.mu.RUnlock()

	for _, wv := range watches {
		if val, ok := values[wv.Path]; ok {
			wv.Update(val)
		}
	}
}

// String 返回 WatchVar 的可读表示
func (wv *WatchVar) String() string {
	if wv == nil {
		return "<nil>"
	}
	return fmt.Sprintf("WatchVar{name=%s, path=%s, last=%v}", wv.Name, wv.Path, wv.LastValue)
}

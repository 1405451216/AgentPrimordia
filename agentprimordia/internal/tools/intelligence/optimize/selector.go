// selector.go — 基于历史成功率的工具选择器
package optimize

import (
	"context"
	"fmt"
	"sync"
)

// toolStats 工具统计（并发安全由外部 HistorySelector 保护）
type toolStats struct {
	Success int
	Total   int
}

// HistorySelector 基于历史成功率的工具选择器
type HistorySelector struct {
	mu    sync.RWMutex
	stats map[string]*toolStats
}

// NewHistorySelector 创建选择器
func NewHistorySelector() *HistorySelector {
	return &HistorySelector{stats: make(map[string]*toolStats)}
}

// Select 从候选列表中选择成功率最高的工具
// 无历史记录的工具视为成功率 0；多个工具成功率相同时返回第一个
func (s *HistorySelector) Select(_ context.Context, _ string, candidates []string) (string, error) {
	if len(candidates) == 0 {
		return "", fmt.Errorf("候选列表为空")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var best string
	bestRate := -1.0

	for _, name := range candidates {
		rate := 0.0
		if st, ok := s.stats[name]; ok && st.Total > 0 {
			rate = float64(st.Success) / float64(st.Total)
		}
		if rate > bestRate {
			bestRate = rate
			best = name
		}
	}

	return best, nil
}

// RecordOutcome 记录工具调用结果
func (s *HistorySelector) RecordOutcome(_ context.Context, toolName string, success bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	st, ok := s.stats[toolName]
	if !ok {
		st = &toolStats{}
		s.stats[toolName] = st
	}
	st.Total++
	if success {
		st.Success++
	}
	return nil
}

// GetStats 获取工具统计（测试用）
func (s *HistorySelector) GetStats(toolName string) (success, total int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st := s.stats[toolName]
	if st == nil {
		return 0, 0
	}
	return st.Success, st.Total
}

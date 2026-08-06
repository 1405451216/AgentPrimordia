package skills

import (
	"sync"
	"time"
)

// UsageRecord 技能调用日志
type UsageRecord struct {
	// SkillID 技能 ID
	SkillID string `json:"skill_id"`
	// TaskDescription 任务描述
	TaskDescription string `json:"task_description"`
	// Success 是否成功
	Success bool `json:"success"`
	// Duration 执行耗时
	Duration time.Duration `json:"duration"`
	// Timestamp 时间戳
	Timestamp time.Time `json:"timestamp"`
}

// UsageStats 技能使用统计
type UsageStats struct {
	// SkillID 技能 ID
	SkillID string
	// TotalCalls 总调用次数
	TotalCalls int
	// SuccessCount 成功次数
	SuccessCount int
	// HitRate 命中率
	HitRate float64
	// SuccessRate 成功率
	SuccessRate float64
	// AvgDuration 平均耗时
	AvgDuration time.Duration
}

// UsageTracker 技能调用日志追踪器
type UsageTracker struct {
	mu      sync.RWMutex
	records []UsageRecord
}

// NewUsageTracker 创建使用追踪器
func NewUsageTracker() *UsageTracker {
	return &UsageTracker{}
}

// Record 记录一次调用
func (u *UsageTracker) Record(rec UsageRecord) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.records = append(u.records, rec)
}

// Stats 获取指定技能的统计
func (u *UsageTracker) Stats(skillID string) UsageStats {
	u.mu.RLock()
	defer u.mu.RUnlock()

	stats := UsageStats{SkillID: skillID}
	var totalDuration time.Duration
	for _, r := range u.records {
		if r.SkillID != skillID {
			continue
		}
		stats.TotalCalls++
		totalDuration += r.Duration
		if r.Success {
			stats.SuccessCount++
		}
	}
	if stats.TotalCalls > 0 {
		stats.SuccessRate = float64(stats.SuccessCount) / float64(stats.TotalCalls)
		stats.HitRate = stats.SuccessRate // 简化：命中率 = 成功率
		stats.AvgDuration = totalDuration / time.Duration(stats.TotalCalls)
	}
	return stats
}

// LowPerformers 返回低效技能列表（成功率低于阈值）
func (u *UsageTracker) LowPerformers(threshold float64) []UsageStats {
	u.mu.RLock()
	skillIDs := make(map[string]bool)
	for _, r := range u.records {
		skillIDs[r.SkillID] = true
	}
	u.mu.RUnlock()

	var result []UsageStats
	for id := range skillIDs {
		stats := u.Stats(id)
		if stats.TotalCalls >= 3 && stats.SuccessRate < threshold {
			result = append(result, stats)
		}
	}
	return result
}

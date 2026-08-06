package skills

// TriggerStrategy 习得触发策略
type TriggerStrategy string

const (
	// TriggerRepeatPattern 重复模式检测（同类任务出现 N 次）
	TriggerRepeatPattern TriggerStrategy = "repeat_pattern"
	// TriggerLowSuccess 任务完成率低（低于阈值时触发习得）
	TriggerLowSuccess TriggerStrategy = "low_success"
	// TriggerExplicit 显式请求（用户/系统主动触发）
	TriggerExplicit TriggerStrategy = "explicit"
)

// TriggerConfig 触发器配置
type TriggerConfig struct {
	// Strategy 触发策略
	Strategy TriggerStrategy
	// RepeatThreshold 重复次数阈值（repeat_pattern 策略）
	RepeatThreshold int
	// SuccessRateThreshold 成功率阈值（low_success 策略）
	SuccessRateThreshold float64
}

// Trigger 习得触发器
type Trigger struct {
	cfg         TriggerConfig
	taskCounts  map[string]int
	successRate float64
	totalTasks  int
	successTasks int
}

// NewTrigger 创建触发器
func NewTrigger(cfg TriggerConfig) *Trigger {
	if cfg.RepeatThreshold <= 0 {
		cfg.RepeatThreshold = 3
	}
	if cfg.SuccessRateThreshold <= 0 {
		cfg.SuccessRateThreshold = 0.5
	}
	return &Trigger{
		cfg:        cfg,
		taskCounts: make(map[string]int),
	}
}

// RecordTask 记录任务执行（用于触发判断）
func (t *Trigger) RecordTask(taskType string, success bool) {
	t.taskCounts[taskType]++
	t.totalTasks++
	if success {
		t.successTasks++
	}
	if t.totalTasks > 0 {
		t.successRate = float64(t.successTasks) / float64(t.totalTasks)
	}
}

// ShouldAcquire 判断是否应触发习得
func (t *Trigger) ShouldAcquire(taskType string) bool {
	switch t.cfg.Strategy {
	case TriggerRepeatPattern:
		return t.taskCounts[taskType] >= t.cfg.RepeatThreshold
	case TriggerLowSuccess:
		return t.totalTasks >= 5 && t.successRate < t.cfg.SuccessRateThreshold
	case TriggerExplicit:
		return false // 仅由外部显式触发
	default:
		return false
	}
}

// Reset 重置指定任务的计数
func (t *Trigger) Reset(taskType string) {
	delete(t.taskCounts, taskType)
}

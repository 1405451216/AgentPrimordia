package observability

import "sync"

// AlertEngine 告警引擎——管理告警规则并定期评估。
type AlertEngine struct {
	mu    sync.RWMutex
	store *CorrelationStore
	rules []AlertRule
}

// NewAlertEngine 创建告警引擎。
func NewAlertEngine(store *CorrelationStore) *AlertEngine {
	return &AlertEngine{store: store}
}

// RegisterRule 注册一条告警规则。
func (e *AlertEngine) RegisterRule(rule AlertRule) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rules = append(e.rules, rule)
}

// Evaluate 执行所有已注册规则，返回触发的告警事件列表。
// 单条规则评估失败时跳过（不阻塞其他规则）。
func (e *AlertEngine) Evaluate() []AlertEvent {
	e.mu.RLock()
	rules := make([]AlertRule, len(e.rules))
	copy(rules, e.rules)
	e.mu.RUnlock()

	var alerts []AlertEvent
	for _, rule := range rules {
		events, err := rule.Evaluate(e.store)
		if err != nil {
			continue
		}
		alerts = append(alerts, events...)
	}
	return alerts
}

// ===== 阈值告警规则 =====

// ThresholdAlertConfig 阈值告警规则配置。
type ThresholdAlertConfig struct {
	Name      string
	Threshold float64
	Severity  AlertSeverity
	MetricFn  func(store *CorrelationStore) float64
}

// ThresholdAlertRule 阈值告警规则（NewThresholdAlertRule 的返回值）。
type ThresholdAlertRule = thresholdRule

// NewThresholdAlertRule 创建阈值告警规则（公开构造函数）。
func NewThresholdAlertRule(cfg ThresholdAlertConfig) AlertRule {
	return &thresholdRule{
		name:      cfg.Name,
		threshold: cfg.Threshold,
		severity:  cfg.Severity,
		metricFn:  cfg.MetricFn,
	}
}

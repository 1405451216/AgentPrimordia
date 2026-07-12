package guardrail

import (
	"strings"
)

// TopicConstraintRule 话题约束规则
// 限制对话在允许的话题范围内
type TopicConstraintRule struct {
	action   Action
	severity Severity
	priority int
	allowed  []string
	denied   []string
	mode     TopicMode
}

// TopicMode 话题约束模式
type TopicMode string

const (
	TopicModeAllowlist TopicMode = "allowlist"
	TopicModeDenylist  TopicMode = "denylist"
)

// TopicConstraintConfig 话题约束配置
type TopicConstraintConfig struct {
	Action   Action
	Severity Severity
	Priority int // 规则优先级，默认 PriorityNormal
	Mode     TopicMode
	Topics   []string
}

// NewTopicConstraintRule 创建话题约束规则
func NewTopicConstraintRule(config TopicConstraintConfig) *TopicConstraintRule {
	priority := config.Priority
	if priority == 0 {
		priority = PriorityNormal
	}
	r := &TopicConstraintRule{
		action:   config.Action,
		severity: config.Severity,
		priority: priority,
		mode:     config.Mode,
	}
	if config.Mode == TopicModeAllowlist {
		// 注意：allowlist 模式下空 topics 列表会拒绝所有输入
		r.allowed = config.Topics
	} else {
		r.denied = config.Topics
	}
	return r
}

// Name 返回规则名
func (r *TopicConstraintRule) Name() string { return "topic_constraint" }

// Priority 返回规则优先级
func (r *TopicConstraintRule) Priority() int { return r.priority }

// Check 检查输入是否在话题约束范围内
func (r *TopicConstraintRule) Check(input string, _ CheckPoint) (*Result, error) {
	lower := strings.ToLower(input)

	if r.mode == TopicModeDenylist {
		for _, topic := range r.denied {
			if strings.Contains(lower, strings.ToLower(topic)) {
				return &Result{
					RuleName: r.Name(),
					Action:   r.action,
					Severity: r.severity,
					Message:  "denied topic detected: " + topic,
					Metadata: map[string]any{"topic": topic},
				}, nil
			}
		}
		return &Result{RuleName: r.Name(), Action: ActionPass}, nil
	}

	for _, topic := range r.allowed {
		if strings.Contains(lower, strings.ToLower(topic)) {
			return &Result{RuleName: r.Name(), Action: ActionPass}, nil
		}
	}

	return &Result{
		RuleName: r.Name(),
		Action:   r.action,
		Severity: r.severity,
		Message:  "input outside allowed topics",
		Metadata: map[string]any{"allowed_topics": r.allowed},
	}, nil
}

package guardrail

import (
	"sort"
	"sync"
	"sync/atomic"
)

type CheckPoint string

const (
	CheckInput  CheckPoint = "input"
	CheckOutput CheckPoint = "output"
)

type Action string

const (
	ActionPass     Action = "pass"
	ActionReject   Action = "reject"
	ActionSanitize Action = "sanitize"
	ActionFlag     Action = "flag"
)

type Severity string

const (
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

type Result struct {
	RuleName  string
	Action    Action
	Severity  Severity
	Message   string
	Sanitized string
	Metadata  map[string]any
}

type Report struct {
	Passed  bool
	Results []Result
	Action  Action
}

// 默认优先级常量。数值越大，优先级越高，越先执行。
const (
	PriorityCritical = 1000 // 安全关键（如 Prompt 注入检测）
	PriorityHigh     = 500  // 高优先级（如 PII 脱敏、输出安全）
	PriorityNormal   = 100  // 常规（如敏感词、话题约束）
	PriorityLow      = 0    // 最低优先级（默认）
)

type Rule interface {
	Name() string
	// Priority 返回规则优先级，数值越大越先执行。
	// 未显式设置时返回 PriorityNormal(100)。
	Priority() int
	Check(input string, point CheckPoint) (*Result, error)
}

type Engine struct {
	mu    sync.Mutex
	rules []Rule
	// 优化（perf-v3）：copy-on-write 快照，Check hot-path 无锁读取
	rulesSnapshot atomic.Pointer[[]Rule]
}

func NewEngine() *Engine {
	e := &Engine{rules: make([]Rule, 0)}
	e.refreshSnapshot()
	return e
}

// refreshSnapshot 在 mu 保护下重建原子快照（写路径调用）
// 快照按 Priority 降序排列，保证高优先级规则先执行。
func (e *Engine) refreshSnapshot() {
	snap := make([]Rule, len(e.rules))
	copy(snap, e.rules)
	sort.SliceStable(snap, func(i, j int) bool {
		return snap[i].Priority() > snap[j].Priority()
	})
	e.rulesSnapshot.Store(&snap)
}

func (e *Engine) AddRule(r Rule) {
	if r == nil {
		return
	}
	e.mu.Lock()
	e.rules = append(e.rules, r)
	e.refreshSnapshot()
	e.mu.Unlock()
}

func (e *Engine) Rules() []string {
	snap := e.rulesSnapshot.Load()
	if snap == nil {
		return nil
	}
	names := make([]string, len(*snap))
	for i, r := range *snap {
		names[i] = r.Name()
	}
	return names
}

// RuleCount returns the number of registered rules.
func (e *Engine) RuleCount() int {
	snap := e.rulesSnapshot.Load()
	if snap == nil {
		return 0
	}
	return len(*snap)
}

func (e *Engine) Check(input string, point CheckPoint) (*Report, error) {
	// 优化（perf-v3）：无锁读取快照，Check hot-path 零分配 + 零锁竞争
	snap := e.rulesSnapshot.Load()
	if snap == nil || len(*snap) == 0 {
		return &Report{Passed: true, Action: ActionPass}, nil
	}

	report := &Report{Passed: true, Action: ActionPass}
	for _, rule := range *snap {
		result, err := rule.Check(input, point)
		if err != nil {
			return nil, err
		}
		if result == nil {
			continue
		}
		report.Results = append(report.Results, *result)
		if result.Action == ActionReject {
			report.Passed = false
			report.Action = ActionReject
			return report, nil
		}
		if result.Action == ActionSanitize {
			report.Passed = false
			report.Action = ActionSanitize
			input = result.Sanitized
		}
		if result.Action == ActionFlag {
			if report.Action == ActionPass {
				report.Action = ActionFlag
			}
		}
	}
	return report, nil
}

func (e *Engine) CheckInput(input string) (*Report, error) {
	return e.Check(input, CheckInput)
}

func (e *Engine) CheckOutput(output string) (*Report, error) {
	return e.Check(output, CheckOutput)
}

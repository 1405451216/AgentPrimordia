package guardrail

import (
	"sync"
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

type Rule interface {
	Name() string
	Check(input string, point CheckPoint) (*Result, error)
}

type Engine struct {
	mu    sync.RWMutex
	rules []Rule
}

func NewEngine() *Engine {
	return &Engine{rules: make([]Rule, 0)}
}

func (e *Engine) AddRule(r Rule) {
	if r == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rules = append(e.rules, r)
}

func (e *Engine) Rules() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	names := make([]string, len(e.rules))
	for i, r := range e.rules {
		names[i] = r.Name()
	}
	return names
}

func (e *Engine) Check(input string, point CheckPoint) (*Report, error) {
	e.mu.RLock()
	rulesCopy := make([]Rule, len(e.rules))
	copy(rulesCopy, e.rules)
	e.mu.RUnlock()

	report := &Report{Passed: true, Action: ActionPass}
	for _, rule := range rulesCopy {
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

package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

type Evaluator interface {
	Evaluate(ctx context.Context, input EvalInput) (*EvalResult, error)
}

type EvalInput struct {
	Task        string
	AgentOutput *Response
	Expected    string
	Metadata    map[string]any
}

type EvalResult struct {
	Score    float64
	Passed   bool
	Criteria []CriterionResult
}

type CriterionResult struct {
	Name   string
	Score  float64
	Passed bool
	Reason string
}

type EvalCase struct {
	Task     string
	Input    string
	Expected string
	Metadata map[string]any
}

type EvalSuiteResult struct {
	Total    int
	Passed   int
	Failed   int
	PassRate float64
	Results  []CaseResult
}

type CaseResult struct {
	Case   EvalCase
	Score  float64
	Passed bool
	Error  error
}

type ExactMatchEvaluator struct {
	CaseInsensitive     bool
	NormalizeWhitespace bool
}

func (e *ExactMatchEvaluator) Evaluate(ctx context.Context, input EvalInput) (*EvalResult, error) {
	actual := input.AgentOutput.Content
	expected := input.Expected

	if e.NormalizeWhitespace {
		actual = normalizeWhitespace(actual)
		expected = normalizeWhitespace(expected)
	}

	if e.CaseInsensitive {
		actual = strings.ToLower(actual)
		expected = strings.ToLower(expected)
	}

	passed := actual == expected
	score := 0.0
	if passed {
		score = 1.0
	}

	return &EvalResult{
		Score:  score,
		Passed: passed,
		Criteria: []CriterionResult{
			{
				Name:   "exact_match",
				Score:  score,
				Passed: passed,
				Reason: fmt.Sprintf("output %q vs expected %q", actual, expected),
			},
		},
	}, nil
}

type ContainsEvaluator struct {
	Keywords []string
}

func (e *ContainsEvaluator) Evaluate(ctx context.Context, input EvalInput) (*EvalResult, error) {
	content := input.AgentOutput.Content

	if len(e.Keywords) == 0 {
		return &EvalResult{
			Score:  1.0,
			Passed: true,
			Criteria: []CriterionResult{
				{Name: "contains", Score: 1.0, Passed: true, Reason: "no keywords required"},
			},
		}, nil
	}

	found := 0
	var criteria []CriterionResult
	for _, kw := range e.Keywords {
		isFound := strings.Contains(strings.ToLower(content), strings.ToLower(kw))
		if isFound {
			found++
		}
		criteria = append(criteria, CriterionResult{
			Name:   fmt.Sprintf("contains_%q", kw),
			Score:  boolToScore(isFound),
			Passed: isFound,
			Reason: fmt.Sprintf("keyword %q %s", kw, foundNotFound(isFound)),
		})
	}

	score := float64(found) / float64(len(e.Keywords))
	passed := found == len(e.Keywords)

	return &EvalResult{
		Score:    score,
		Passed:   passed,
		Criteria: criteria,
	}, nil
}

type ToolUsageEvaluator struct {
	ExpectedTools []string
}

func (e *ToolUsageEvaluator) Evaluate(ctx context.Context, input EvalInput) (*EvalResult, error) {
	usedTools := make(map[string]bool)
	for _, tc := range input.AgentOutput.ToolCalls {
		usedTools[tc.Name] = true
	}

	if len(e.ExpectedTools) == 0 {
		return &EvalResult{
			Score:  1.0,
			Passed: true,
			Criteria: []CriterionResult{
				{Name: "tool_usage", Score: 1.0, Passed: true, Reason: "no tools expected"},
			},
		}, nil
	}

	found := 0
	var criteria []CriterionResult
	for _, tool := range e.ExpectedTools {
		isUsed := usedTools[tool]
		if isUsed {
			found++
		}
		criteria = append(criteria, CriterionResult{
			Name:   fmt.Sprintf("tool_%q", tool),
			Score:  boolToScore(isUsed),
			Passed: isUsed,
			Reason: fmt.Sprintf("tool %q %s", tool, foundNotFound(isUsed)),
		})
	}

	score := float64(found) / float64(len(e.ExpectedTools))
	passed := found == len(e.ExpectedTools)

	if !passed && found > 0 {
		criteria = append(criteria, CriterionResult{
			Name:   "tool_usage_summary",
			Score:  score,
			Passed: passed,
			Reason: fmt.Sprintf("%d/%d expected tools used", found, len(e.ExpectedTools)),
		})
	}

	return &EvalResult{
		Score:    score,
		Passed:   passed,
		Criteria: criteria,
	}, nil
}

type CompositeMode int

const (
	CompositeAll CompositeMode = iota
	CompositeAny
	CompositeWeighted
)

type WeightedEvaluator struct {
	Evaluator Evaluator
	Weight    float64
}

type CompositeEvaluator struct {
	Evaluators         []Evaluator
	WeightedEvaluators []WeightedEvaluator
	Mode               CompositeMode
}

func (e *CompositeEvaluator) Evaluate(ctx context.Context, input EvalInput) (*EvalResult, error) {
	if e.Mode == CompositeWeighted {
		return e.evaluateWeighted(ctx, input)
	}

	var allResults []*EvalResult
	var allCriteria []CriterionResult

	for _, ev := range e.Evaluators {
		result, err := ev.Evaluate(ctx, input)
		if err != nil {
			return nil, err
		}
		allResults = append(allResults, result)
		allCriteria = append(allCriteria, result.Criteria...)
	}

	switch e.Mode {
	case CompositeAll:
		passed := true
		totalScore := 0.0
		for _, r := range allResults {
			if !r.Passed {
				passed = false
			}
			totalScore += r.Score
		}
		avgScore := totalScore / float64(len(allResults))
		return &EvalResult{
			Score:    avgScore,
			Passed:   passed,
			Criteria: allCriteria,
		}, nil

	case CompositeAny:
		passed := false
		maxScore := 0.0
		for _, r := range allResults {
			if r.Passed {
				passed = true
			}
			if r.Score > maxScore {
				maxScore = r.Score
			}
		}
		return &EvalResult{
			Score:    maxScore,
			Passed:   passed,
			Criteria: allCriteria,
		}, nil
	}

	return nil, fmt.Errorf("unknown composite mode")
}

func (e *CompositeEvaluator) evaluateWeighted(ctx context.Context, input EvalInput) (*EvalResult, error) {
	var allCriteria []CriterionResult
	weightedScore := 0.0
	totalWeight := 0.0

	for _, we := range e.WeightedEvaluators {
		result, err := we.Evaluator.Evaluate(ctx, input)
		if err != nil {
			return nil, err
		}
		weightedScore += result.Score * we.Weight
		totalWeight += we.Weight
		allCriteria = append(allCriteria, result.Criteria...)
	}

	if totalWeight > 0 {
		weightedScore /= totalWeight
	}

	passed := weightedScore >= 0.5

	return &EvalResult{
		Score:    weightedScore,
		Passed:   passed,
		Criteria: allCriteria,
	}, nil
}

type LLMEvaluator struct {
	Provider interface {
		Evaluate(ctx context.Context, prompt string) (string, error)
	}
	PromptTemplate string
}

func (e *LLMEvaluator) Evaluate(ctx context.Context, input EvalInput) (*EvalResult, error) {
	prompt := e.buildPrompt(input)
	resp, err := e.Provider.Evaluate(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("LLM evaluation failed: %w", err)
	}

	return parseLLMEvalResponse(resp)
}

func (e *LLMEvaluator) buildPrompt(input EvalInput) string {
	tmpl := e.PromptTemplate
	if tmpl == "" {
		tmpl = `Evaluate the following agent response.
Task: {{.Task}}
Expected: {{.Expected}}
Agent Output: {{.Output}}

Respond in JSON format: {"score": <0-1>, "passed": <bool>, "reason": "<explanation>"}`
	}

	r := strings.NewReplacer(
		"{{.Task}}", input.Task,
		"{{.Expected}}", input.Expected,
		"{{.Output}}", input.AgentOutput.Content,
	)
	return r.Replace(tmpl)
}

func parseLLMEvalResponse(resp string) (*EvalResult, error) {
	resp = strings.TrimSpace(resp)

	score := 0.0
	passed := false
	reason := ""

	if idx := strings.Index(resp, "{"); idx >= 0 {
		segment := resp[idx:]
		if endIdx := strings.Index(segment, "}"); endIdx > 0 {
			segment = segment[:endIdx+1]

			for _, field := range []string{`"score"`, `"passed"`, `"reason"`} {
				if !strings.Contains(segment, field) {
					return &EvalResult{
						Score:  0.5,
						Passed: false,
						Criteria: []CriterionResult{
							{Name: "llm_judge", Score: 0.5, Passed: false, Reason: "incomplete LLM response"},
						},
					}, nil
				}
			}

			passedStr := extractJSONValue(segment, `"passed"`)
			passed = passedStr == "true"

			scoreStr := extractJSONNumber(segment, `"score"`)
			if scoreStr != "" {
				fmt.Sscanf(scoreStr, "%f", &score)
			} else if passed {
				score = 1.0
			}

			reason = extractJSONString(segment, `"reason"`)
		}
	}

	return &EvalResult{
		Score:  score,
		Passed: passed,
		Criteria: []CriterionResult{
			{Name: "llm_judge", Score: score, Passed: passed, Reason: reason},
		},
	}, nil
}

func extractJSONNumber(json, key string) string {
	idx := strings.Index(json, key)
	if idx < 0 {
		return ""
	}
	after := json[idx+len(key):]
	after = strings.TrimLeft(after, ": ")
	numEnd := 0
	for numEnd < len(after) && (after[numEnd] >= '0' && after[numEnd] <= '9' || after[numEnd] == '.') {
		numEnd++
	}
	if numEnd == 0 {
		return ""
	}
	return after[:numEnd]
}

func extractJSONValue(json, key string) string {
	idx := strings.Index(json, key)
	if idx < 0 {
		return ""
	}
	after := json[idx+len(key):]
	after = strings.TrimLeft(after, ": ")
	if strings.HasPrefix(after, `"`) {
		return extractJSONString(json, key)
	}
	valEnd := 0
	for valEnd < len(after) && after[valEnd] != ',' && after[valEnd] != '}' && after[valEnd] != ' ' {
		valEnd++
	}
	if valEnd == 0 {
		return ""
	}
	return after[:valEnd]
}

func extractJSONString(json, key string) string {
	idx := strings.Index(json, key)
	if idx < 0 {
		return ""
	}
	after := json[idx+len(key):]
	after = strings.TrimLeft(after, ": ")
	if len(after) == 0 || after[0] != '"' {
		return ""
	}
	after = after[1:]
	end := strings.Index(after, `"`)
	if end < 0 {
		return after
	}
	return after[:end]
}

type EvalRunner struct {
	evaluators []Evaluator
	mu         sync.Mutex
}

func NewEvalRunner(evaluators ...Evaluator) *EvalRunner {
	return &EvalRunner{
		evaluators: evaluators,
	}
}

func (r *EvalRunner) RunSuite(ctx context.Context, agent Agent, cases []EvalCase) (*EvalSuiteResult, error) {
	result := &EvalSuiteResult{
		Total: len(cases),
	}

	for _, c := range cases {
		resp, err := agent.Run(ctx, UserMessage(c.Input))
		if err != nil {
			result.Failed++
			result.Results = append(result.Results, CaseResult{
				Case:  c,
				Error: err,
			})
			continue
		}

		evalInput := EvalInput{
			Task:        c.Task,
			AgentOutput: resp,
			Expected:    c.Expected,
			Metadata:    c.Metadata,
		}

		var caseScore float64
		casePassed := true

		for _, ev := range r.evaluators {
			evalResult, err := ev.Evaluate(ctx, evalInput)
			if err != nil {
				casePassed = false
				continue
			}
			if !evalResult.Passed {
				casePassed = false
			}
			caseScore += evalResult.Score
		}

		if len(r.evaluators) > 0 {
			caseScore /= float64(len(r.evaluators))
		}

		if casePassed {
			result.Passed++
		} else {
			result.Failed++
		}

		result.Results = append(result.Results, CaseResult{
			Case:   c,
			Score:  caseScore,
			Passed: casePassed,
		})
	}

	if result.Total > 0 {
		result.PassRate = float64(result.Passed) / float64(result.Total)
	}

	return result, nil
}

func normalizeWhitespace(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
		} else {
			b.WriteRune(r)
			prevSpace = false
		}
	}
	return b.String()
}

func boolToScore(b bool) float64 {
	if b {
		return 1.0
	}
	return 0.0
}

func foundNotFound(found bool) string {
	if found {
		return "found"
	}
	return "not found"
}

package a2a

// v3.5 兼容性报告工具：对照开放规范逐项生成符合性报告

// InteropCheck 单项符合性检查
type InteropCheck struct {
	// Name 检查项名称
	Name string `json:"name"`
	// Passed 是否通过
	Passed bool `json:"passed"`
	// Detail 说明
	Detail string `json:"detail"`
}

// InteropReport 协议符合性报告
type InteropReport struct {
	// Mode 当前互操作模式
	Mode string `json:"mode"`
	// Checks 检查项列表
	Checks []InteropCheck `json:"checks"`
	// Score 符合性得分 [0,1]
	Score float64 `json:"score"`
}

// GenerateInteropReport 生成协议符合性报告
func GenerateInteropReport(card OpenAgentCard, cfg InteropConfig) InteropReport {
	var checks []InteropCheck

	checks = append(checks, InteropCheck{
		Name:   "agent_card.name",
		Passed: card.Name != "",
		Detail: "Agent Card 必须包含 name",
	})
	checks = append(checks, InteropCheck{
		Name:   "agent_card.url",
		Passed: card.URL != "",
		Detail: "Agent Card 必须包含 url 端点",
	})
	checks = append(checks, InteropCheck{
		Name:   "agent_card.version",
		Passed: card.Version != "",
		Detail: "Agent Card 应包含 version",
	})
	checks = append(checks, InteropCheck{
		Name:   "capabilities.streaming",
		Passed: true,
		Detail: "流式能力声明字段存在",
	})
	checks = append(checks, InteropCheck{
		Name:   "default_input_modes",
		Passed: len(card.DefaultInputModes) > 0,
		Detail: "应声明默认输入模式",
	})
	checks = append(checks, InteropCheck{
		Name:   "default_output_modes",
		Passed: len(card.DefaultOutputModes) > 0,
		Detail: "应声明默认输出模式",
	})
	checks = append(checks, InteropCheck{
		Name:   "agent_card.exposed",
		Passed: cfg.ExposeAgentCard,
		Detail: "Agent Card 发现端点是否暴露",
	})
	checks = append(checks, InteropCheck{
		Name:   "io_modes.text",
		Passed: cfg.IOModes.SupportsInput(IOModeText) && cfg.IOModes.SupportsOutput(IOModeText),
		Detail: "文本输入输出模式支持",
	})

	passed := 0
	for _, c := range checks {
		if c.Passed {
			passed++
		}
	}
	score := 0.0
	if len(checks) > 0 {
		score = float64(passed) / float64(len(checks))
	}

	return InteropReport{
		Mode:   string(cfg.Mode),
		Checks: checks,
		Score:  score,
	}
}

// FailedChecks 返回未通过的检查项
func (r InteropReport) FailedChecks() []InteropCheck {
	var failed []InteropCheck
	for _, c := range r.Checks {
		if !c.Passed {
			failed = append(failed, c)
		}
	}
	return failed
}

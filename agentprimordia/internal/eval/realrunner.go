// realrunner.go — V7 弧线 S0-1：真实线（nightly）评测核心——与 Provider 无关、可离线测试
//
// 双轨质量门的「真实轨」落地：对 docs/evals/ 冻结题面（留出子集）跑真实 LLM：
//   - external-general：逐题问答 + 确定性机检判分（answer_check.exact）；
//   - judge-calibration：LLM-as-judge 对 good/bad 样本对输出裁决，
//     与注册客观标签算 Cohen κ（S0-1 验收要求 κ ≥0.6、≥200 双标样本）。
//
// 本文件只含纯逻辑（判分/提示词/解析/统计），网络调用由 bench/eval-real 注入，
// 因此全部可用脚本化 CompletionFunc 在 CI 离线穷尽测试。
package eval

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// 评测轨标签：回放轨（CI 回归）与真实轨（nightly 真实 LLM）。
const (
	TrackReplay = "replay"
	TrackReal   = "real"
)

// CompletionFunc 单轮补全函数（真实 Provider 或脚本化替身）。
type CompletionFunc func(ctx context.Context, prompt string) (string, error)

// JudgeFunc judge 裁决函数：对 (任务, 回答) 输出 good/bad。
type JudgeFunc func(ctx context.Context, task, response string) (string, error)

// NormalizeAnswer 机检判分的答案规范化：去首尾空白/引号/句末标点，拉丁字母小写，剥离 markdown/LaTeX/千分位逗号。
func NormalizeAnswer(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "\"'")
	s = strings.TrimRight(s, "。.！!？?\n\t ")
	// 剥离 markdown 格式标记
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "*", "")
	s = strings.ReplaceAll(s, "`", "")
	// 剥离 LaTeX 标记
	s = strings.ReplaceAll(s, `\(`, "")
	s = strings.ReplaceAll(s, `\)`, "")
	s = strings.ReplaceAll(s, `\[`, "")
	s = strings.ReplaceAll(s, `\]`, "")
	s = strings.ReplaceAll(s, `\boxed{`, "")
	s = strings.ReplaceAll(s, "{", "")
	s = strings.ReplaceAll(s, "}", "")
	// 剥离千分位逗号（数字中的 "," 如 "2,128" → "2128"）
	s = stripThousandCommas(s)
	s = strings.ToLower(strings.TrimSpace(s))
	return s
}

// stripThousandCommas 剥离数字中的千分位逗号（仅当逗号前后都是数字时移除）。
func stripThousandCommas(s string) string {
	var b strings.Builder
	runes := []rune(s)
	for i, r := range runes {
		if r == ',' && i > 0 && i < len(runes)-1 {
			prev := runes[i-1]
			next := runes[i+1]
			if prev >= '0' && prev <= '9' && next >= '0' && next <= '9' {
				continue
			}
		}
		b.WriteRune(r)
	}
	return b.String()
}

// GradeExactAnswer 外部泛化题判分：短答案（≤10 字符）做包含匹配，长答案做全等匹配。
func GradeExactAnswer(item EvalSetItem, response string) (bool, error) {
	raw, ok := item.AnswerCheck["exact"]
	if !ok {
		return false, fmt.Errorf("eval: 题面 %s 缺 answer_check.exact，不可机检", item.ID)
	}
	want, ok := raw.(string)
	if !ok {
		return false, fmt.Errorf("eval: 题面 %s answer_check.exact 应为字符串", item.ID)
	}
	normWant := NormalizeAnswer(want)
	normResp := NormalizeAnswer(response)
	// 短答案（≤10 字符）：模型常把答案嵌在长文本里，做包含匹配
	if len(normWant) <= 10 {
		return strings.Contains(normResp, normWant), nil
	}
	// 长答案：全等匹配
	return normResp == normWant, nil
}

// JudgePrompt judge 标定/评审的系统化提示词（约束只输出 good 或 bad）。
func JudgePrompt(task, response string) string {
	return "你是严格的评审员。判断下面的「回答」是否正确完成了「任务」。\n" +
		"任务：" + task + "\n" +
		"回答：" + response + "\n" +
		"只输出一个小写单词：正确输出 good，错误输出 bad，不要输出任何其他内容。"
}

// ParseJudgeVerdict 从 judge 输出解析裁决；两个词都出现或都没出现时报错（不猜）。
func ParseJudgeVerdict(out string) (string, error) {
	s := strings.ToLower(out)
	hasGood := strings.Contains(s, "good")
	hasBad := strings.Contains(s, "bad")
	switch {
	case hasGood && !hasBad:
		return "good", nil
	case hasBad && !hasGood:
		return "bad", nil
	default:
		return "", fmt.Errorf("eval: judge 输出无法唯一解析为 good/bad: %q", out)
	}
}

// RealEvalRecord 单题真实轨记录。
type RealEvalRecord struct {
	ID       string `json:"id"`
	Kind     string `json:"kind,omitempty"`
	Holdout  bool   `json:"holdout"`
	Expected string `json:"expected,omitempty"`
	Response string `json:"response,omitempty"`
	JudgeSay string `json:"judge_say,omitempty"`
	Gold     string `json:"gold,omitempty"`
	Passed   bool   `json:"passed"`
	Error    string `json:"error,omitempty"`
}

// RealEvalReport 真实轨报告（落 bench/results/，nightly 趋势对账用）。
type RealEvalReport struct {
	Set          string           `json:"set"`
	SetSHA256    string           `json:"set_sha256"`
	FreezeCommit string           `json:"freeze_commit"`
	Track        string           `json:"track"`
	Mode         string           `json:"mode"`
	Provider     string           `json:"provider"`
	Model        string           `json:"model"`
	HoldoutOnly  bool             `json:"holdout_only"`
	GeneratedAt  string           `json:"generated_at"`
	Records      []RealEvalRecord `json:"records"`
	Summary      RatePoint        `json:"summary"`
	Kappa        *float64         `json:"kappa,omitempty"`
	KappaDropped int              `json:"kappa_dropped,omitempty"`
}

// RunExternalGeneralReal 真实轨跑外部泛化集：逐题补全 + 机检判分。
func RunExternalGeneralReal(ctx context.Context, items []EvalSetItem, fn CompletionFunc) ([]RealEvalRecord, error) {
	if fn == nil {
		return nil, fmt.Errorf("eval: CompletionFunc 为空")
	}
	records := make([]RealEvalRecord, 0, len(items))
	for _, it := range items {
		rec := RealEvalRecord{ID: it.ID, Kind: it.Kind, Holdout: it.Holdout}
		if raw, ok := it.AnswerCheck["exact"]; ok {
			if s, isStr := raw.(string); isStr {
				rec.Expected = s
			}
		}
		resp, err := fn(ctx, it.Prompt)
		if err != nil {
			rec.Error = err.Error()
		} else {
			rec.Response = resp
			passed, gerr := GradeExactAnswer(it, resp)
			if gerr != nil {
				rec.Error = gerr.Error()
			} else {
				rec.Passed = passed
			}
		}
		records = append(records, rec)
	}
	return records, nil
}

// RunJudgeCalibration 真实轨跑 judge 标定集：judge 裁决 vs 注册客观标签。
// Gold=注册标签、JudgeSay=judge 裁决；无法解析的裁决记 Error 并在统计中披露。
func RunJudgeCalibration(ctx context.Context, items []EvalSetItem, judge JudgeFunc) ([]RealEvalRecord, error) {
	if judge == nil {
		return nil, fmt.Errorf("eval: JudgeFunc 为空")
	}
	records := make([]RealEvalRecord, 0, len(items))
	for _, it := range items {
		rec := RealEvalRecord{ID: it.ID, Holdout: it.Holdout, Gold: it.Label, Response: it.Response}
		say, err := judge(ctx, it.Prompt, it.Response)
		if err != nil {
			rec.Error = err.Error()
		} else if say != "good" && say != "bad" {
			// 判定器契约收口：裁决只允许 good/bad，越界值按解析失败处理并披露，
			// 防止脏标签悄悄混入 κ 的混淆矩阵。
			rec.Error = fmt.Sprintf("eval: judge 裁决非法 %q（只允许 good/bad）", say)
		} else {
			rec.JudgeSay = say
			rec.Passed = say == it.Label
		}
		records = append(records, rec)
	}
	return records, nil
}

// SummarizeRealEval 汇总记录为 R3 口径（点估计 + Wilson 95% 下界）。
// 带 Error 的记录视为未通过（计入分母）——真实轨不允许静默丢样本。
func SummarizeRealEval(records []RealEvalRecord) (RatePoint, error) {
	if len(records) == 0 {
		return RatePoint{}, fmt.Errorf("%w: 无记录", ErrInvalidStatInput)
	}
	pass := 0
	for _, r := range records {
		if r.Passed {
			pass++
		}
	}
	return ReportRate(pass, len(records))
}

// JudgeCalibrationKappa 对 judge 标定记录计算 Cohen κ。
// 只统计双方都有裁决的样本，剔除数必须随报告披露（R3：任何剔除都要说出来）。
func JudgeCalibrationKappa(records []RealEvalRecord) (kappa float64, used, dropped int, err error) {
	var a, b []string
	for _, r := range records {
		if r.Error != "" || r.JudgeSay == "" || r.Gold == "" {
			dropped++
			continue
		}
		a = append(a, r.Gold)
		b = append(b, r.JudgeSay)
	}
	used = len(a)
	if used == 0 {
		return 0, 0, dropped, fmt.Errorf("%w: 无有效双标样本", ErrInvalidStatInput)
	}
	k, err := CohenKappa(a, b)
	if err != nil {
		return 0, used, dropped, err
	}
	return k, used, dropped, nil
}

// NowRFC3339 报告时间戳（独立出来便于测试注入）。
func NowRFC3339() string { return time.Now().UTC().Format(time.RFC3339) }

// SortRecords 按题号排序（报告稳定字节序，便于跨夜 diff）。
func SortRecords(records []RealEvalRecord) {
	sort.Slice(records, func(i, j int) bool { return records[i].ID < records[j].ID })
}

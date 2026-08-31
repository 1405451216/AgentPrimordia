// verifiers.go — 强验证器族（v6.3 核心任务内部构件；路线图 §五「强验证器族归位」）
//
//   - CodeExecVerifier：沙箱彩排复用——工具工件在彩排执行器上按规格用例
//     全过才放行（执行器接口注入，wasm.ToolExecutor 由组装根适配）；
//   - EnsembleJudgeVerifier：多裁判加权表决（平票=不过——保守语义）；
//   - CalibrateFAR：假接受率标定 harness（对抗样本集上验证器接受率，
//     R3 口径点估计 + Wilson 区间，漏检全量披露）。
package lifecycle

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// Verdict 验证结论（确定性记录）。
type Verdict struct {
	Pass   bool      `json:"pass"`
	Detail string    `json:"detail"` // 判定摘要（含用例数/票数）
	Probe  string    `json:"probe"`  // rehearsal / ensemble / far_calibration
	At     time.Time `json:"at"`
}

// SpecCase 工件规格用例（彩排/对抗共用形态）。
type SpecCase struct {
	Name     string `json:"name"`
	Input    string `json:"input"`
	Expected string `json:"expected"`
}

// CodeExecutor 沙箱执行器窄接口（彩排复用面；组装根绑定 wasm.ToolExecutor
// 或测试替身——internal/tools 不直接依赖 wasm 模块，依赖方向不变）。
type CodeExecutor interface {
	// Run 在沙箱内执行工件的一个入口，返回输出摘要。
	Run(ctx context.Context, artifact []byte, function, input string) (string, error)
}

// CodeExecVerifier 沙箱彩排验证器。
type CodeExecVerifier struct {
	Executor CodeExecutor
	Function string // 工件导出入口名（如 "tool_main"）
}

// Verify 规格用例全过才 Pass（确定性：用例按 Name 升序执行）。
// 执行错误一律计失败（R3：全量披露，不静默跳过）。
func (v *CodeExecVerifier) Verify(ctx context.Context, artifact []byte, cases []SpecCase) Verdict {
	if v.Executor == nil {
		return Verdict{Pass: false, Probe: "rehearsal", Detail: "沙箱执行器未注入", At: time.Now().UTC()}
	}
	ordered := append([]SpecCase(nil), cases...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Name < ordered[j].Name })
	var failed []string
	for _, c := range ordered {
		out, err := v.Executor.Run(ctx, artifact, v.Function, c.Input)
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s: 执行错误 %v", c.Name, err))
			continue
		}
		if normalizeAnswer(out) != normalizeAnswer(c.Expected) {
			failed = append(failed, fmt.Sprintf("%s: 输出 %q ≠ 期望 %q", c.Name, out, c.Expected))
		}
	}
	if len(failed) > 0 {
		return Verdict{Pass: false, Probe: "rehearsal", Detail: fmt.Sprintf("%d/%d 用例未过：%v", len(failed), len(ordered), failed), At: time.Now().UTC()}
	}
	return Verdict{Pass: true, Probe: "rehearsal", Detail: fmt.Sprintf("%d/%d 用例通过", len(ordered), len(ordered)), At: time.Now().UTC()}
}

// JudgeFunc 单裁判：对工件+输入给出裁决（"pass"/"fail"/弃权 "abstain"）。
type JudgeFunc func(ctx context.Context, artifact []byte, input string) string

// EnsembleJudgeVerifier 多裁判表决验证器。
type EnsembleJudgeVerifier struct {
	Judges []JudgeFunc
	// RequiredPasses 通过票数下限（缺省 = 多数严格：floor(N/2)+1，平票不过）
	RequiredPasses int
}

// Verify 多裁判表决（确定性：裁判按切片序）。
func (v *EnsembleJudgeVerifier) Verify(ctx context.Context, artifact []byte, input string) Verdict {
	if len(v.Judges) == 0 {
		return Verdict{Pass: false, Probe: "ensemble", Detail: "无裁判", At: time.Now().UTC()}
	}
	required := v.RequiredPasses
	if required <= 0 {
		required = len(v.Judges)/2 + 1
	}
	passes, fails, abstains := 0, 0, 0
	for _, j := range v.Judges {
		switch j(ctx, artifact, input) {
		case "pass":
			passes++
		case "fail":
			fails++
		default:
			abstains++
		}
	}
	pass := passes >= required
	return Verdict{
		Pass:   pass,
		Probe:  "ensemble",
		Detail: fmt.Sprintf("票型 pass=%d fail=%d abstain=%d（门槛 %d）", passes, fails, abstains, required),
		At:     time.Now().UTC(),
	}
}

// AdversarialSample 对抗样本（假接受率标定输入；本验证器必须拒绝）。
type AdversarialSample struct {
	ID       string `json:"id"`
	Artifact []byte `json:"-"`
	Input    string `json:"input"`
}

// FARReport 假接受率标定报告（R3：点估计 + Wilson 区间；漏检全量披露）。
type FARReport struct {
	Probe       string    `json:"probe"`
	N           int       `json:"n"`        // 对抗样本数
	Accepted    int       `json:"accepted"` // 被错误接受数
	Rate        float64   `json:"rate"`     // FAR 点估计
	WilsonLo    float64   `json:"wilson_lo"`
	WilsonHi    float64   `json:"wilson_hi"`
	AcceptedIDs []string  `json:"accepted_ids"` // 漏检全量披露（R3 强制）
	Generated   time.Time `json:"generated"`
}

// CalibrateFAR 假接受率标定：对抗样本全部应被拒绝；任何接受即漏检。
// verify 为被标定验证器（返回 Verdict.Pass）。
func CalibrateFAR(probe string, verify func(sample AdversarialSample) bool, samples []AdversarialSample) (*FARReport, error) {
	if len(samples) == 0 {
		return nil, fmt.Errorf("lifecycle: 对抗样本集为空，不构成标定")
	}
	ordered := append([]AdversarialSample(nil), samples...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
	rep := &FARReport{Probe: probe, N: len(ordered), Generated: time.Now().UTC()}
	for _, s := range ordered {
		if verify(s) {
			rep.Accepted++
			rep.AcceptedIDs = append(rep.AcceptedIDs, s.ID)
		}
	}
	rep.Rate = float64(rep.Accepted) / float64(rep.N)
	rep.WilsonLo, rep.WilsonHi = wilsonInterval(rep.Accepted, rep.N)
	return rep, nil
}

// normalizeAnswer 答案规范化（与 pipeline/shadow 同口径：去空白比对）。
func normalizeAnswer(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			continue
		}
		out = append(out, r)
	}
	return string(out)
}

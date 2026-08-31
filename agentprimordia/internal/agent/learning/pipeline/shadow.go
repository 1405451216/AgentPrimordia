// shadow.go — 影子评测（命题 1 判据的实现：蒸馏模型 vs 旗舰，同题配对）
//
// 统计口径（R3）：点估计 + Wilson 95% 下界；配对差异用 McNemar 精确二项
// 检验；比值判据 Ratio = shadow_rate / champion_rate，RatioLower 为保守
// 下界（以 shadow Wilson 下界 / champion 点估计——只对 shadow 不利，不会
// 因 champion 抖动虚高）。这些统计助手在本包自含（不 import internal/eval：
// agent 层不反向消费横向支撑包，分层约束优先于 DRY）。
package pipeline

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"
)

// AnswerFn 被评模型的可调用形态（champion/shadow 同签名）。
// 返回实际输出；判分由 Scorer 决定。
type AnswerFn func(ctx context.Context, input string) (string, error)

// Scorer 确定性判分：输出与期望匹配返回 true（external 机检口径）。
type Scorer func(output, expected string) string // "pass"/"fail"/"skip"

// ExactScorer 精确匹配判分器（默认；空白规范化后比对）。
func ExactScorer(output, expected string) string {
	if normalizeAnswer(output) == normalizeAnswer(expected) {
		return "pass"
	}
	return "fail"
}

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

// ShadowEvaluator 影子评测器。
type ShadowEvaluator struct {
	Champion      AnswerFn
	Shadow        AnswerFn
	ChampionModel string
	ShadowModel   string
	Scorer        Scorer
	Audit         *AuditChain
}

// Evaluate 同题双臂配对评测（确定性：题按 ID 升序跑）。
// n=0 时返回错误（无题不构成评测——R2 功效纪律）。
func (e *ShadowEvaluator) Evaluate(ctx context.Context, manifestID string, cases []ShadowCase) (*ShadowReport, error) {
	if e.Champion == nil || e.Shadow == nil {
		return nil, fmt.Errorf("pipeline: 影子评测需要 champion 与 shadow 两个可调用模型")
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("pipeline: 影子评测题数为 0，不构成评测（R2 功效纪律）")
	}
	scorer := e.Scorer
	if scorer == nil {
		scorer = ExactScorer
	}
	ordered := make([]ShadowCase, len(cases))
	copy(ordered, cases)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })

	var champPass, shadowPass, bothPass, champOnly, shadowOnly int
	for _, c := range ordered {
		cOut, cErr := e.Champion(ctx, c.Input)
		sOut, sErr := e.Shadow(ctx, c.Input)
		cRes := judge(scorer, cOut, cErr, c.Expected)
		sRes := judge(scorer, sOut, sErr, c.Expected)
		switch {
		case cRes == "pass" && sRes == "pass":
			bothPass++
		case cRes == "pass":
			champOnly++
		case sRes == "pass":
			shadowOnly++
		}
		if cRes == "pass" {
			champPass++
		}
		if sRes == "pass" {
			shadowPass++
		}
	}
	n := len(ordered)
	report := &ShadowReport{
		ManifestID:      manifestID,
		ChampionModel:   e.ChampionModel,
		ShadowModel:     e.ShadowModel,
		N:               n,
		ChampionSuccess: champPass,
		ShadowSuccess:   shadowPass,
		ChampionRate:    float64(champPass) / float64(n),
		ShadowRate:      float64(shadowPass) / float64(n),
		McNemarP:        mcnemarExactP(champOnly, shadowOnly),
		CreatedAt:       time.Now().UTC(),
	}
	lo, _ := wilsonInterval(shadowPass, n)
	report.ShadowWilsonLo = lo
	if report.ChampionRate > 0 {
		report.Ratio = report.ShadowRate / report.ChampionRate
		report.RatioLower = lo / report.ChampionRate
	} else {
		// 旗舰 0 成功：比值无定义，判据直接按 shadow 绝对值计（披露口径）
		report.Ratio = 0
		report.RatioLower = 0
	}
	// 命题 1 判据：点 ≥0.85× 且 CI 下界 ≥0.80×（V7 §四命题 1）
	report.Passed = report.Ratio >= 0.85 && report.RatioLower >= 0.80
	if e.Audit != nil {
		e.Audit.Append("shadow", fmt.Sprintf("影子评测 manifest=%s n=%d champion=%d/%d shadow=%d/%d ratio=%.3f(下界 %.3f) 判据=%v",
			manifestID, n, champPass, n, shadowPass, n, report.Ratio, report.RatioLower, report.Passed))
		report.AuditID = fmt.Sprintf("audit-%d", e.Audit.Count())
	}
	return report, nil
}

// judge 单题判定（错误一律 fail，skip 视为 fail 计入分母——R3 全量披露）。
func judge(scorer Scorer, output string, err error, expected string) string {
	if err != nil {
		return "fail"
	}
	return scorer(output, expected)
}

// ===== 统计助手（与 internal/eval/stats.go 同算法口径，本包自含）=====

// wilsonInterval Wilson 95% 成功率区间（z=1.959963984540054，与 Go/TS S0-1 一致）。
func wilsonInterval(k, n int) (lo, hi float64) {
	if n <= 0 {
		return 0, 0
	}
	z := 1.959963984540054
	p := float64(k) / float64(n)
	denom := 1 + z*z/float64(n)
	center := (p + z*z/(2*float64(n))) / denom
	rad := z / denom * math.Sqrt(p*(1-p)/float64(n)+z*z/(4*float64(n)*float64(n)))
	return center - rad, center + rad
}

// mcnemarExactP McNemar 精确二项双侧 p 值（b/c 为不一致格数）。
// p = 2 * P(X ≤ min(b,c))，X~Binom(b+c, 0.5)；b+c=0 时 p=1。
func mcnemarExactP(b, c int) float64 {
	if b+c == 0 {
		return 1
	}
	m := b + c
	k := b
	if c < k {
		k = c
	}
	// 对数阶乘防溢出：logC(m,i) 累加
	logSum := math.Inf(-1)
	for i := 0; i <= k; i++ {
		lg := logChoose(m, i) - float64(m)*math.Log(2)
		logSum = logAddExp(logSum, lg)
	}
	p := 2 * math.Exp(logSum)
	if p > 1 {
		p = 1
	}
	return p
}

// logChoose log(C(m,i))（ lgamma 复用；确定性）。
func logChoose(m, i int) float64 {
	return lgamma(float64(m)+1) - lgamma(float64(i)+1) - lgamma(float64(m-i)+1)
}

// lgamma math.Lgamma 包装（统一错误忽略——Lgamma 零值即 0 的 log）。
func lgamma(x float64) float64 {
	v, _ := math.Lgamma(x)
	return v
}

// logAddExp 数值稳定的 log(exp(a)+exp(b))。
func logAddExp(a, b float64) float64 {
	if math.IsInf(a, -1) {
		return b
	}
	if math.IsInf(b, -1) {
		return a
	}
	if a > b {
		return a + math.Log1p(math.Exp(b-a))
	}
	return b + math.Log1p(math.Exp(a-b))
}

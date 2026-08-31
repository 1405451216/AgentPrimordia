// stats.go — V7 弧线 S0-1：显著性检验与功效分析框架（新规 R2/R3 的执行内核）
//
// 背景（docs/V7路线图.md §一 R2/R3、§十）：v6.0 及以前的 A/B 宣称有两类病灶——
//
//	① 样本量拍脑袋（「N≥30, p<0.05」对 +15pp 量级不具备 80% 功效）；
//	② 质量类指标用裸 100%/0 容忍表述，可被定义窄化 gaming。
//
// 本文件把口径换成可计算函数：
//   - 比率类指标一律「点估计 + Wilson 95% 下界」（R3）；
//   - 同题双臂配对用 McNemar 精确二项检验（成本最低的合法设计）；
//   - 样本量由精确枚举的功效函数算出（SampleSizeMcNemar），不查表拍数；
//   - 均值类差值（token/轮数成本）用配对 bootstrap 95% CI，不假设正态；
//   - judge 标定用 Cohen κ（S0-1 要求 κ ≥0.6）。
//
// 纯标准库实现，所有函数确定性（bootstrap 显式 seed）。
// 双线对等：TS 侧 sdk/typescript/src/eval/stats.ts 与本文件逐函数同口径，
// 由 internal/eval/testdata/stats_fixtures.json 做跨语言数值对账门。
package eval

import (
	"errors"
	"fmt"
	"math"
	"sort"
)

// ErrInvalidStatInput 统计函数入参非法（调用方错误，不可重试）。
var ErrInvalidStatInput = errors.New("eval: 统计入参非法")

// Z95 双侧 95% 置信的正态分位数。
const Z95 = 1.959963984540054

// ==========================================================================
// 正态分布原语
// ==========================================================================

// NormalCDF 标准正态累积分布 Φ(x)。
func NormalCDF(x float64) float64 {
	return 0.5 * math.Erfc(-x/math.Sqrt2)
}

// NormalQuantile 标准正态分位数 Φ⁻¹(p)，0<p<1。
// 二分搜索实现：无魔法常数、确定性、精度 ~1e-15（200 次区间折半）。
func NormalQuantile(p float64) (float64, error) {
	if p <= 0 || p >= 1 {
		return 0, fmt.Errorf("%w: p=%v 须落在 (0,1)", ErrInvalidStatInput, p)
	}
	lo, hi := -12.0, 12.0
	for i := 0; i < 200; i++ {
		mid := (lo + hi) / 2
		if NormalCDF(mid) < p {
			lo = mid
		} else {
			hi = mid
		}
	}
	return (lo + hi) / 2, nil
}

// ==========================================================================
// Wilson 区间（R3：比率类指标强制口径）
// ==========================================================================

// WilsonInterval 成功数比例的 Wilson score 区间；z 取分位数（95% 双侧用 Z95）。
func WilsonInterval(successes, trials int, z float64) (lower, upper float64, err error) {
	if trials <= 0 {
		return 0, 0, fmt.Errorf("%w: trials=%d 须 >0", ErrInvalidStatInput, trials)
	}
	if successes < 0 || successes > trials {
		return 0, 0, fmt.Errorf("%w: successes=%d trials=%d", ErrInvalidStatInput, successes, trials)
	}
	if z <= 0 {
		return 0, 0, fmt.Errorf("%w: z=%v", ErrInvalidStatInput, z)
	}
	n := float64(trials)
	p := float64(successes) / n
	z2 := z * z
	denom := 1 + z2/n
	center := (p + z2/(2*n)) / denom
	margin := z * math.Sqrt(p*(1-p)/n+z2/(4*n*n)) / denom
	return center - margin, center + margin, nil
}

// RatePoint 比率指标的 R3 报告单元：点估计与 Wilson 下界成对出现。
type RatePoint struct {
	Successes   int     `json:"successes"`
	Trials      int     `json:"trials"`
	Point       float64 `json:"point"`
	WilsonLower float64 `json:"wilson_lower95"`
	WilsonUpper float64 `json:"wilson_upper95"`
}

// ReportRate 按 R3 口径报告一个比率（成功率/复用率/拦截率/准确率）。
func ReportRate(successes, trials int) (RatePoint, error) {
	lo, hi, err := WilsonInterval(successes, trials, Z95)
	if err != nil {
		return RatePoint{}, err
	}
	return RatePoint{Successes: successes, Trials: trials,
		Point: float64(successes) / float64(trials), WilsonLower: lo, WilsonUpper: hi}, nil
}

// String 评审可读口径，如 "0.900 (Wilson95 下界 0.826, n=100)"。
func (r RatePoint) String() string {
	return fmt.Sprintf("%.3f (Wilson95 下界 %.3f, n=%d)", r.Point, r.WilsonLower, r.Trials)
}

// ==========================================================================
// McNemar 精确二项检验（同题双臂配对）
// ==========================================================================

// McNemarExact 双侧精确 McNemar 检验 p 值。
// b = 仅基线臂成功数，c = 仅处理臂成功数（一致对不参与检验）。
// p = 2·P(X ≤ min(b,c))，X~Bin(b+c, 0.5)，上限截到 1。
func McNemarExact(b, c int) (float64, error) {
	if b < 0 || c < 0 {
		return 0, fmt.Errorf("%w: b=%d c=%d 不可为负", ErrInvalidStatInput, b, c)
	}
	n := b + c
	if n == 0 {
		return 1, nil // 无不一致对：无法拒绝原假设
	}
	m := b
	if c < m {
		m = c
	}
	term := math.Ldexp(1, -n) // C(n,0)/2^n，精确 2 的幂避免下溢
	sum := term
	for k := 1; k <= m; k++ {
		term *= float64(n-k+1) / float64(k)
		sum += term
	}
	p := 2 * sum
	if p > 1 {
		p = 1
	}
	return p, nil
}

// PairedOutcome 同一题面在双臂上的一次配对判定。
type PairedOutcome struct {
	TaskID    string `json:"task_id"`
	Baseline  bool   `json:"baseline"`
	Treatment bool   `json:"treatment"`
}

// PairedAnalysis McNemar 配对分析结果（含双臂 Wilson 口径）。
type PairedAnalysis struct {
	N          int       `json:"n"`
	Concordant int       `json:"concordant"`
	DiscB      int       `json:"disc_b"`
	DiscC      int       `json:"disc_c"`
	Lift       float64   `json:"lift"`
	PValue     float64   `json:"p_value"`
	BaseRate   RatePoint `json:"baseline_rate"`
	TreatRate  RatePoint `json:"treatment_rate"`
}

// AnalyzePaired 同题双臂结果的 McNemar 配对分析。
func AnalyzePaired(pairs []PairedOutcome) (PairedAnalysis, error) {
	if len(pairs) == 0 {
		return PairedAnalysis{}, fmt.Errorf("%w: 配对结果为空", ErrInvalidStatInput)
	}
	var b, c, baseOK, treatOK int
	for _, p := range pairs {
		if p.Baseline {
			baseOK++
		}
		if p.Treatment {
			treatOK++
		}
		if p.Baseline && !p.Treatment {
			b++
		}
		if !p.Baseline && p.Treatment {
			c++
		}
	}
	pv, err := McNemarExact(b, c)
	if err != nil {
		return PairedAnalysis{}, err
	}
	br, err := ReportRate(baseOK, len(pairs))
	if err != nil {
		return PairedAnalysis{}, err
	}
	tr, err := ReportRate(treatOK, len(pairs))
	if err != nil {
		return PairedAnalysis{}, err
	}
	return PairedAnalysis{N: len(pairs), Concordant: len(pairs) - b - c, DiscB: b, DiscC: c,
		Lift: float64(treatOK-baseOK) / float64(len(pairs)), PValue: pv,
		BaseRate: br, TreatRate: tr}, nil
}

// ==========================================================================
// 两独立比例检验与样本量（无法配对时的退路）
// ==========================================================================

// TwoProportionZTest 两独立比例 z 检验，返回差值（p2-p1）、z、双侧 p。
func TwoProportionZTest(x1, n1, x2, n2 int) (diff, z, pValue float64, err error) {
	if n1 <= 0 || n2 <= 0 || x1 < 0 || x2 < 0 || x1 > n1 || x2 > n2 {
		return 0, 0, 0, fmt.Errorf("%w: (%d/%d) vs (%d/%d)", ErrInvalidStatInput, x1, n1, x2, n2)
	}
	p1, p2 := float64(x1)/float64(n1), float64(x2)/float64(n2)
	pool := float64(x1+x2) / float64(n1+n2)
	se := math.Sqrt(pool * (1 - pool) * (1/float64(n1) + 1/float64(n2)))
	if se == 0 {
		return p2 - p1, 0, 1, nil
	}
	z = (p2 - p1) / se
	return p2 - p1, z, 2 * (1 - NormalCDF(math.Abs(z))), nil
}

// SampleSizeTwoProportion 独立双臂每组样本量（正态近似，双侧 alpha）。
func SampleSizeTwoProportion(p1, p2, alpha, targetPower float64) (int, error) {
	if p1 <= 0 || p1 >= 1 || p2 <= 0 || p2 >= 1 || p1 == p2 {
		return 0, fmt.Errorf("%w: p1=%v p2=%v", ErrInvalidStatInput, p1, p2)
	}
	if alpha <= 0 || alpha >= 1 || targetPower <= 0 || targetPower >= 1 {
		return 0, fmt.Errorf("%w: alpha=%v power=%v", ErrInvalidStatInput, alpha, targetPower)
	}
	zA, err := NormalQuantile(1 - alpha/2)
	if err != nil {
		return 0, err
	}
	zB, err := NormalQuantile(targetPower)
	if err != nil {
		return 0, err
	}
	pool := (p1 + p2) / 2
	num := zA*math.Sqrt(2*pool*(1-pool)) + zB*math.Sqrt(p1*(1-p1)+p2*(1-p2))
	return int(math.Ceil(num * num / ((p2 - p1) * (p2 - p1)))), nil
}

// ==========================================================================
// McNemar 功效精确枚举与样本量（R2 的落地函数）
// ==========================================================================

// McNemarPower 精确枚举配对设计功效。
// n=题数；delta=真实成功率差（处理-基线，如 0.15）；omega=真实不一致率（须 >|delta|）。
// 模型：b~Bin(n,(ω+δ)/2)、c~Bin(n,(ω-δ)/2) 独立；McNemarExact(b,c) ≤ alpha 判为拒绝。
func McNemarPower(n int, delta, omega, alpha float64) (float64, error) {
	if n <= 0 {
		return 0, fmt.Errorf("%w: n=%d", ErrInvalidStatInput, n)
	}
	if alpha <= 0 || alpha >= 1 {
		return 0, fmt.Errorf("%w: alpha=%v", ErrInvalidStatInput, alpha)
	}
	if omega <= 0 || omega > 1 {
		return 0, fmt.Errorf("%w: omega=%v", ErrInvalidStatInput, omega)
	}
	if math.Abs(delta) >= omega {
		return 0, fmt.Errorf("%w: |delta|=%v 须 < omega=%v（不一致率必须容得下真实差值）",
			ErrInvalidStatInput, math.Abs(delta), omega)
	}
	p10 := (omega + delta) / 2 // 仅处理臂成功
	p01 := (omega - delta) / 2 // 仅基线臂成功
	rej := rejectionTable(n, alpha)
	pb := binomialPMF(n, p10)
	pc := binomialPMF(n, p01)
	power := 0.0
	for i := 0; i <= n; i++ {
		if pb[i] <= 0 {
			continue
		}
		for j := 0; j <= n; j++ {
			if pc[j] <= 0 {
				continue
			}
			if rej[i][j] {
				power += pb[i] * pc[j]
			}
		}
	}
	return power, nil
}

// SampleSizeMcNemar 达到 targetPower 所需的最小题数（正态近似起算 + 精确枚举校正）。
// 这是 R2「样本量由设计算出而非拍 30」的落地入口。
func SampleSizeMcNemar(delta, omega, alpha, targetPower float64) (int, error) {
	if math.Abs(delta) <= 0 || math.Abs(delta) >= omega {
		return 0, fmt.Errorf("%w: delta=%v omega=%v", ErrInvalidStatInput, delta, omega)
	}
	if targetPower <= 0 || targetPower >= 1 {
		return 0, fmt.Errorf("%w: targetPower=%v", ErrInvalidStatInput, targetPower)
	}
	if alpha <= 0 || alpha >= 1 {
		return 0, fmt.Errorf("%w: alpha=%v", ErrInvalidStatInput, alpha)
	}
	zA, err := NormalQuantile(1 - alpha/2)
	if err != nil {
		return 0, err
	}
	zB, err := NormalQuantile(targetPower)
	if err != nil {
		return 0, err
	}
	// 正态近似只作搜索起点（可能偏大也可能偏小）：先把它当区间上界的种子，
	// 再用精确枚举二分找**最小**达标 n——不能只向上走，否则近似偏大时会虚报样本量。
	const maxN = 4000
	meets := func(n int) (bool, error) {
		pw, err := McNemarPower(n, delta, omega, alpha)
		if err != nil {
			return false, err
		}
		return pw >= targetPower, nil
	}
	hi := int(math.Ceil((zA + zB) * (zA + zB) * omega / (delta * delta)))
	if hi < 1 {
		hi = 1
	}
	for {
		if hi >= maxN {
			return 0, fmt.Errorf("%w: maxN=%d 内达不到 power=%v（delta=%v omega=%v）",
				ErrInvalidStatInput, maxN, targetPower, delta, omega)
		}
		ok, err := meets(hi)
		if err != nil {
			return 0, err
		}
		if ok {
			break
		}
		hi *= 2
	}
	lo := 1
	for lo < hi {
		mid := (lo + hi) / 2
		ok, err := meets(mid)
		if err != nil {
			return 0, err
		}
		if ok {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	// 离散非严格单调：向下逐步校正到真正的局部最小达标点
	for lo > 1 {
		ok, err := meets(lo - 1)
		if err != nil {
			return 0, err
		}
		if !ok {
			break
		}
		lo--
	}
	return lo, nil
}

// rejectionTable 预计算 (b,c) 网格上的「是否拒绝 H0」，尾概率按 t=b+c 递推复用。
func rejectionTable(n int, alpha float64) [][]bool {
	tails := make([][]float64, 2*n+1)
	for t := 0; t <= 2*n; t++ {
		row := make([]float64, t+1)
		term := math.Ldexp(1, -t)
		sum := term
		row[0] = sum
		for k := 1; k <= t; k++ {
			term *= float64(t-k+1) / float64(k)
			sum += term
			row[k] = sum
		}
		tails[t] = row
	}
	rej := make([][]bool, n+1)
	for i := range rej {
		rej[i] = make([]bool, n+1)
	}
	for b := 0; b <= n; b++ {
		for c := 0; c <= n; c++ {
			t := b + c
			if t == 0 {
				continue
			}
			m := b
			if c < m {
				m = c
			}
			p := 2 * tails[t][m]
			if p > 1 {
				p = 1
			}
			rej[b][c] = p <= alpha
		}
	}
	return rej
}

// binomialPMF 返回 k=0..n 的二项概率质量（递推，容忍极端 p）。
func binomialPMF(n int, p float64) []float64 {
	out := make([]float64, n+1)
	if p <= 0 {
		out[0] = 1
		return out
	}
	if p >= 1 {
		out[n] = 1
		return out
	}
	pmf := math.Pow(1-p, float64(n))
	out[0] = pmf
	for k := 1; k <= n; k++ {
		pmf *= (float64(n-k+1) / float64(k)) * (p / (1 - p))
		out[k] = pmf
	}
	return out
}

// ==========================================================================
// Cohen κ（judge 标定，S0-1 要求 κ ≥0.6）
// ==========================================================================

// CohenKappa 两名评分者的未加权一致性系数。输入等长标签切片。
func CohenKappa(raterA, raterB []string) (float64, error) {
	if len(raterA) == 0 || len(raterA) != len(raterB) {
		return 0, fmt.Errorf("%w: 标签切片为空或长度不等", ErrInvalidStatInput)
	}
	cats := map[string]struct{}{}
	for _, s := range raterA {
		cats[s] = struct{}{}
	}
	for _, s := range raterB {
		cats[s] = struct{}{}
	}
	n := float64(len(raterA))
	agree := 0
	cntA := map[string]int{}
	cntB := map[string]int{}
	for i := range raterA {
		if raterA[i] == raterB[i] {
			agree++
		}
		cntA[raterA[i]]++
		cntB[raterB[i]]++
	}
	po := float64(agree) / n
	pe := 0.0
	for c := range cats {
		pe += (float64(cntA[c]) / n) * (float64(cntB[c]) / n)
	}
	if pe == 1 {
		return 0, fmt.Errorf("%w: 期望一致率为 1（单一类别），κ 无定义", ErrInvalidStatInput)
	}
	return (po - pe) / (1 - pe), nil
}

// ==========================================================================
// 配对 bootstrap CI（成本类差值，不假设正态）
// ==========================================================================

// BootstrapCI 自助法置信区间。
type BootstrapCI struct {
	Point      float64 `json:"point"`
	Lower      float64 `json:"lower95"`
	Upper      float64 `json:"upper95"`
	Iterations int     `json:"iterations"`
	Seed       int64   `json:"seed"`
}

// PairedBootstrapCI 对每题差值序列做均值 percentile bootstrap。
// deltas[i] = 处理臂观测 - 基线臂观测（同题）；iterations ≥100；seed 固定保证可复现。
func PairedBootstrapCI(deltas []float64, iterations int, seed int64) (BootstrapCI, error) {
	if len(deltas) == 0 {
		return BootstrapCI{}, fmt.Errorf("%w: 差值序列为空", ErrInvalidStatInput)
	}
	if iterations < 100 {
		return BootstrapCI{}, fmt.Errorf("%w: iterations=%d 须 ≥100", ErrInvalidStatInput, iterations)
	}
	point := 0.0
	for _, d := range deltas {
		if math.IsNaN(d) || math.IsInf(d, 0) {
			return BootstrapCI{}, fmt.Errorf("%w: 差值序列含非有限值", ErrInvalidStatInput)
		}
		point += d
	}
	point /= float64(len(deltas))
	rng := newLCG(seed)
	means := make([]float64, iterations)
	n := len(deltas)
	for it := 0; it < iterations; it++ {
		sum := 0.0
		for i := 0; i < n; i++ {
			sum += deltas[rng.intn(n)]
		}
		means[it] = sum / float64(n)
	}
	sort.Float64s(means)
	lo := means[int(0.025*float64(iterations))]
	hi := means[int(0.975*float64(iterations))-1]
	return BootstrapCI{Point: point, Lower: lo, Upper: hi, Iterations: iterations, Seed: seed}, nil
}

// lcg 确定性 xorshift64 伪随机源（TS 侧同常数实现，保证 bootstrap 跨语言可复现）。
type lcg struct{ state uint64 }

func newLCG(seed int64) *lcg { return &lcg{state: uint64(seed) | 1} }

func (l *lcg) next() uint64 {
	l.state ^= l.state << 13
	l.state ^= l.state >> 7
	l.state ^= l.state << 17
	return l.state
}

func (l *lcg) intn(n int) int { return int(l.next() % uint64(n)) }

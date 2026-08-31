// stats_test.go — S0-1 统计框架数值对账测试
//
// 参考值由独立实现（Python math + 精确二项枚举）离线算出后写死，
// 与本文件构成「双实现互验」：任何一侧改动破坏口径即测试失败。
// 跨语言对账见 testdata/stats_fixtures.json + TS src/eval/__tests__/stats.test.ts。
package eval

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func almost(got, want, tol float64) bool { return math.Abs(got-want) <= tol }

func TestWilsonInterval(t *testing.T) {
	cases := []struct {
		s, n   int
		lo, hi float64
	}{
		{90, 100, 0.8256343384950865, 0.9447708629393249},
		{24, 24, 0.8620237953250198, 1.0},
		{60, 65, 0.8322406672988999, 0.9666964985048365},
		{0, 10, 0.0, 0.2775327999},
		{5, 500, 0.0042787539, 0.0231930998},
		{377, 1101, 0.3149783346, 0.3709494542},
	}
	for _, c := range cases {
		lo, hi, err := WilsonInterval(c.s, c.n, Z95)
		if err != nil {
			t.Fatalf("WilsonInterval(%d,%d): %v", c.s, c.n, err)
		}
		if !almost(lo, c.lo, 1e-6) || !almost(hi, c.hi, 1e-6) {
			t.Errorf("WilsonInterval(%d/%d) = (%.7f,%.7f), 期望 (%.7f,%.7f)", c.s, c.n, lo, hi, c.lo, c.hi)
		}
	}
}

func TestWilsonRejectsBadInput(t *testing.T) {
	if _, _, err := WilsonInterval(3, 0, Z95); err == nil {
		t.Error("trials=0 应报错")
	}
	if _, _, err := WilsonInterval(11, 10, Z95); err == nil {
		t.Error("successes>trials 应报错")
	}
	if _, _, err := WilsonInterval(1, 10, 0); err == nil {
		t.Error("z=0 应报错")
	}
}

// TestReportRateR3 R3 口径：点估计与 Wilson 下界必须成对报告，
// 且 n=24 全对时宣称值只能是下界（0.862）而非裸 100%。
func TestReportRateR3(t *testing.T) {
	r, err := ReportRate(24, 24)
	if err != nil {
		t.Fatal(err)
	}
	if r.Point != 1.0 {
		t.Errorf("点估计应为 1.0，得 %v", r.Point)
	}
	if r.WilsonLower > 0.87 || r.WilsonLower < 0.86 {
		t.Errorf("n=24 全对的 Wilson 下界应在 0.86 附近（说明裸 100%% 宣称无意义），得 %v", r.WilsonLower)
	}
	if got := r.String(); got == "" {
		t.Error("String() 不应为空")
	}
	t.Logf("R3 口径示例: %s", r)
}

func TestMcNemarExact(t *testing.T) {
	cases := []struct {
		b, c int
		p    float64
	}{
		{25, 10, 0.0166738478},
		{10, 25, 0.0166738478}, // 对称性
		{0, 5, 0.0625},
		{3, 3, 1.0},
		{60, 20, 8.5806e-06},
		{12, 8, 0.5034446716},
		{0, 0, 1.0},
	}
	for _, c := range cases {
		p, err := McNemarExact(c.b, c.c)
		if err != nil {
			t.Fatalf("McNemarExact(%d,%d): %v", c.b, c.c, err)
		}
		if !almost(p, c.p, 1e-6) {
			t.Errorf("McNemarExact(%d,%d) = %v, 期望 %v", c.b, c.c, p, c.p)
		}
	}
	if _, err := McNemarExact(-1, 3); err == nil {
		t.Error("负数应报错")
	}
}

// TestMcNemarDecisionBoundary 验收语义：+15pp 若在 n=71 上做出 b=8/c=21 的不一致模式，
// p 值应显著；而 n=24 的小样本即使全对也只能给出「功效不足」的警告。
func TestMcNemarDecisionBoundary(t *testing.T) {
	p, err := McNemarExact(8, 21)
	if err != nil {
		t.Fatal(err)
	}
	if p >= 0.05 {
		t.Errorf("8 vs 21 应显著（p<0.05），得 %v", p)
	}
}

func TestAnalyzePaired(t *testing.T) {
	pairs := make([]PairedOutcome, 0, 20)
	// 10 题基线成功/处理成功(6) 基线成功处理失败(4) ...
	specs := []PairedOutcome{
		{TaskID: "t1", Baseline: true, Treatment: true},
		{TaskID: "t2", Baseline: false, Treatment: true},
		{TaskID: "t3", Baseline: false, Treatment: true},
		{TaskID: "t4", Baseline: true, Treatment: false},
		{TaskID: "t5", Baseline: true, Treatment: true},
		{TaskID: "t6", Baseline: false, Treatment: false},
		{TaskID: "t7", Baseline: false, Treatment: true},
		{TaskID: "t8", Baseline: true, Treatment: true},
	}
	A, err := AnalyzePaired(specs)
	if err != nil {
		t.Fatal(err)
	}
	if A.N != 8 || A.DiscB != 1 || A.DiscC != 3 || A.Concordant != 4 {
		t.Errorf("配对计数错误: %+v", A)
	}
	if A.BaseRate.Successes != 4 || A.TreatRate.Successes != 6 {
		t.Errorf("成功率计数错误: base=%v treat=%v", A.BaseRate.Successes, A.TreatRate.Successes)
	}
	if !almost(A.Lift, 2.0/8.0, 1e-12) {
		t.Errorf("Lift 应为 0.25，得 %v", A.Lift)
	}
	// 与直接 McNemarExact(1,3) 一致
	direct, _ := McNemarExact(1, 3)
	if !almost(A.PValue, direct, 1e-12) {
		t.Errorf("PValue %v != %v", A.PValue, direct)
	}
	if _, err := AnalyzePaired(nil); err == nil {
		t.Error("空输入应报错")
	}
	for i := range pairs {
		_ = i
	}
}

func TestTwoProportionZTest(t *testing.T) {
	diff, z, p, err := TwoProportionZTest(30, 100, 50, 100)
	if err != nil {
		t.Fatal(err)
	}
	if !almost(diff, 0.2, 1e-12) {
		t.Errorf("diff 应为 0.2，得 %v", diff)
	}
	// 参考值：pool=0.4, se=sqrt(0.4*0.6*0.02)=0.06928203, z=2.8867513, p=0.00389
	if !almost(z, 2.8867513, 1e-5) || !almost(p, 0.003889, 1e-4) {
		t.Errorf("z/p 不符: %v %v", z, p)
	}
	if _, _, _, err := TwoProportionZTest(1, 0, 1, 10); err == nil {
		t.Error("n=0 应报错")
	}
}

func TestSampleSizeTwoProportion(t *testing.T) {
	cases := []struct {
		p1, p2 float64
		want   int
	}{
		{0.5, 0.65, 170},
		{0.5, 0.70, 93},
		{0.5, 0.80, 39},
		{0.3, 0.50, 93},
	}
	for _, c := range cases {
		n, err := SampleSizeTwoProportion(c.p1, c.p2, 0.05, 0.80)
		if err != nil {
			t.Fatalf("%v->%v: %v", c.p1, c.p2, err)
		}
		if n != c.want {
			t.Errorf("SampleSizeTwoProportion(%v,%v) = %d, 期望 %d", c.p1, c.p2, n, c.want)
		}
	}
}

// TestMcNemarPowerCurve 功效曲线单调 + 关键点位与离线精确枚举一致。
func TestMcNemarPowerCurve(t *testing.T) {
	p71, err := McNemarPower(71, 0.15, 0.30, 0.05)
	if err != nil {
		t.Fatal(err)
	}
	if !almost(p71, 0.5806, 5e-4) {
		t.Errorf("n=71/+15pp/ω=0.30 功效应 ≈0.5806（远不足 80%%），得 %v", p71)
	}
	p80, err := McNemarPower(108, 0.15, 0.30, 0.05)
	if err != nil {
		t.Fatal(err)
	}
	if p80 < 0.80 {
		t.Errorf("n=108 应达 80%% 功效，得 %v", p80)
	}
	// 单调性
	for _, n := range []int{30, 60, 90, 120, 150} {
		pa, err := McNemarPower(n, 0.15, 0.30, 0.05)
		if err != nil {
			t.Fatal(err)
		}
		pb, err := McNemarPower(n+1, 0.15, 0.30, 0.05)
		if err != nil {
			t.Fatal(err)
		}
		if pb < pa-1e-9 {
			t.Fatalf("功效应随 n 单调不减: n=%d %v -> %v", n, pa, pb)
		}
	}
	if _, err := McNemarPower(50, 0.4, 0.3, 0.05); err == nil {
		t.Error("|delta|>=omega 应报错")
	}
}

// TestSampleSizeMcNemarR2 R2 落地：+15pp 在 ω=0.30 下需 108 题（不是拍出来的 30/71）。
func TestSampleSizeMcNemarR2(t *testing.T) {
	n, err := SampleSizeMcNemar(0.15, 0.30, 0.05, 0.80)
	if err != nil {
		t.Fatal(err)
	}
	if n != 108 {
		t.Errorf("期望 n=108（离线精确枚举），得 %d", n)
	}
	if pw, _ := McNemarPower(n-1, 0.15, 0.30, 0.05); pw >= 0.80 {
		t.Errorf("n=%d 已达标，最小题数应为 %d", n-1, n)
	}
	for _, c := range []struct {
		delta, omega float64
		want         int
	}{{0.20, 0.40, 80}, {0.30, 0.40, 34}, {0.20, 0.30, 59}} {
		got, err := SampleSizeMcNemar(c.delta, c.omega, 0.05, 0.80)
		if err != nil {
			t.Fatal(err)
		}
		if got != c.want {
			t.Errorf("SampleSizeMcNemar(%v,%v) = %d, 期望 %d", c.delta, c.omega, got, c.want)
		}
	}
}

func TestCohenKappa(t *testing.T) {
	a := []string{"1", "1", "1", "1", "1", "0", "0", "0", "0", "0", "1", "0", "1", "0", "1"}
	b := []string{"1", "1", "0", "0", "1", "0", "0", "1", "0", "0", "1", "1", "1", "0", "0"}
	k, err := CohenKappa(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if !almost(k, 0.3362831858, 1e-6) {
		t.Errorf("κ = %v, 期望 0.3362831858", k)
	}
	a2 := []string{"cat", "cat", "dog", "dog", "bird", "cat", "dog", "bird", "cat", "dog"}
	b2 := []string{"cat", "dog", "dog", "dog", "bird", "cat", "cat", "bird", "cat", "dog"}
	k2, err := CohenKappa(a2, b2)
	if err != nil {
		t.Fatal(err)
	}
	if !almost(k2, 0.6875, 1e-9) {
		t.Errorf("κ = %v, 期望 0.6875", k2)
	}
	if _, err := CohenKappa(nil, nil); err == nil {
		t.Error("空输入应报错")
	}
	if _, err := CohenKappa([]string{"a"}, []string{"b", "c"}); err == nil {
		t.Error("长度不等应报错")
	}
	if _, err := CohenKappa([]string{"a", "a"}, []string{"a", "a"}); err == nil {
		t.Error("单一类别 κ 无定义应报错")
	}
}

// TestPairedBootstrap 固定 seed 可复现 + CI 覆盖点估计 + 单调合理。
func TestPairedBootstrap(t *testing.T) {
	deltas := make([]float64, 40)
	for i := range deltas {
		deltas[i] = float64(i%7) - 2.5
	}
	c1, err := PairedBootstrapCI(deltas, 2000, 20260831)
	if err != nil {
		t.Fatal(err)
	}
	c2, err := PairedBootstrapCI(deltas, 2000, 20260831)
	if err != nil {
		t.Fatal(err)
	}
	if c1 != c2 {
		t.Errorf("同 seed 结果必须逐位一致: %+v vs %+v", c1, c2)
	}
	if !(c1.Lower <= c1.Point && c1.Point <= c1.Upper) {
		t.Errorf("CI 应覆盖点估计: %+v", c1)
	}
	if c1.Point < -1 || c1.Point > 1 {
		t.Errorf("均值超出合理范围: %v", c1.Point)
	}
	if _, err := PairedBootstrapCI(deltas, 50, 1); err == nil {
		t.Error("iterations<100 应报错")
	}
	if _, err := PairedBootstrapCI([]float64{math.NaN()}, 200, 1); err == nil {
		t.Error("NaN 应报错")
	}
}

// TestLCGStreamCrossCheck 与 TS 侧 xorshift64 常数逐位一致（跨语言 bootstrap 可复现的基础）。
func TestLCGStreamCrossCheck(t *testing.T) {
	want := []uint64{
		21903561661438544, 12621275415332881636, 7120044806649196269, 1120328294130832624,
	}
	l := newLCG(20260831)
	for i, w := range want {
		if got := l.next(); got != w {
			t.Fatalf("xorshift64 第 %d 个输出 = %d, 期望 %d（TS 侧同常数须同步修订）", i+1, got, w)
		}
	}
}

// 跨语言对账夹具（Go 侧为权威生成方，TS 测试读同一文件比对）。
// 字段一律显式 json tag：TS 侧按这些 snake_case 键取值，改名即破坏对账门。

// fxWilson 一条 Wilson 区间夹具。
type fxWilson struct {
	S  int     `json:"s"`
	N  int     `json:"n"`
	Lo float64 `json:"lower95"`
	Hi float64 `json:"upper95"`
}

// fxMcNemar 一条 McNemar 精确 p 值夹具。
type fxMcNemar struct {
	B int     `json:"b"`
	C int     `json:"c"`
	P float64 `json:"p"`
}

// fxKappa 一条 Cohen κ 夹具。
type fxKappa struct {
	A []string `json:"rater_a"`
	B []string `json:"rater_b"`
	K float64  `json:"kappa"`
}

// fxPower 一条配对设计功效夹具。
type fxPower struct {
	N     int     `json:"n"`
	Delta float64 `json:"delta"`
	Omega float64 `json:"omega"`
	Power float64 `json:"power"`
}

// fxBoot 一条配对 bootstrap CI 夹具（同 seed 跨语言可复现）。
type fxBoot struct {
	Deltas []float64 `json:"deltas"`
	Seed   int64     `json:"seed"`
	Iters  int       `json:"iterations"`
	Point  float64   `json:"point"`
	Lower  float64   `json:"lower95"`
	Upper  float64   `json:"upper95"`
}

// fxQuantile 一条标准正态分位数夹具。
type fxQuantile struct {
	P float64 `json:"p"`
	X float64 `json:"x"`
}

// statsFixture 夹具根结构。
type statsFixture struct {
	Wilson   []fxWilson   `json:"wilson"`
	McNemar  []fxMcNemar  `json:"mcnemar"`
	Kappa    []fxKappa    `json:"kappa"`
	PowerMcN []fxPower    `json:"mcnemar_power"`
	Boot     []fxBoot     `json:"bootstrap"`
	Quantile []fxQuantile `json:"normal_quantile"`
	FormatRt []fxFormat   `json:"format_rate"`
}

// fxFormat 文案格式化对账：锁 %.3f 的平局舍入口径（ties-to-even）。
type fxFormat struct {
	S    int    `json:"s"`
	N    int    `json:"n"`
	Want string `json:"want"`
}

// TestStatsFixturesAgainstTS 读 testdata/stats_fixtures.json，逐项校验 Go 实现与夹具一致。
// 夹具由 TestWriteStatsFixtures 生成（Go 为权威侧）；TS 侧同文件同容差。
func TestStatsFixturesAgainstTS(t *testing.T) {
	path := filepath.Join("testdata", "stats_fixtures.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("夹具不存在（用 go test -run TestWriteStatsFixtures 生成）: %v", err)
	}
	var f statsFixture
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("夹具解析失败: %v", err)
	}
	for _, c := range f.Wilson {
		lo, hi, err := WilsonInterval(c.S, c.N, Z95)
		if err != nil || !almost(lo, c.Lo, 1e-9) || !almost(hi, c.Hi, 1e-9) {
			t.Errorf("Wilson %d/%d 不一致: (%v,%v) vs (%v,%v)", c.S, c.N, lo, hi, c.Lo, c.Hi)
		}
	}
	for _, c := range f.McNemar {
		p, err := McNemarExact(c.B, c.C)
		if err != nil || !almost(p, c.P, 1e-12) {
			t.Errorf("McNemar %d/%d 不一致: %v vs %v", c.B, c.C, p, c.P)
		}
	}
	for _, c := range f.Kappa {
		k, err := CohenKappa(c.A, c.B)
		if err != nil || !almost(k, c.K, 1e-9) {
			t.Errorf("κ 不一致: %v vs %v", k, c.K)
		}
	}
	for _, c := range f.PowerMcN {
		pw, err := McNemarPower(c.N, c.Delta, c.Omega, 0.05)
		if err != nil || !almost(pw, c.Power, 1e-6) {
			t.Errorf("功效 n=%d 不一致: %v vs %v", c.N, pw, c.Power)
		}
	}
	for _, c := range f.Boot {
		ci, err := PairedBootstrapCI(c.Deltas, c.Iters, c.Seed)
		if err != nil {
			t.Fatal(err)
		}
		if !almost(ci.Point, c.Point, 1e-9) || !almost(ci.Lower, c.Lower, 1e-9) || !almost(ci.Upper, c.Upper, 1e-9) {
			t.Errorf("bootstrap 不一致: %+v vs %+v", ci, c)
		}
	}
	for _, c := range f.Quantile {
		x, err := NormalQuantile(c.P)
		if err != nil || !almost(x, c.X, 1e-9) {
			t.Errorf("Φ⁻¹(%v) 不一致: %v vs %v", c.P, x, c.X)
		}
	}
	for _, c := range f.FormatRt {
		rp, err := ReportRate(c.S, c.N)
		if err != nil {
			t.Errorf("format_rate %d/%d 构造失败: %v", c.S, c.N, err)
			continue
		}
		if got := rp.String(); got != c.Want {
			t.Errorf("format_rate %d/%d 文案不一致: %q vs %q", c.S, c.N, got, c.Want)
		}
	}
}

// TestWriteStatsFixtures 生成跨语言对账夹具（Go 为权威侧）。
// 默认跳过；AP_WRITE_STATS_FIXTURE=1 时写出 internal/eval/testdata/stats_fixtures.json。
// TS 侧 src/eval/__tests__/stats.test.ts 读同一文件做双线数值对账。
func TestWriteStatsFixtures(t *testing.T) {
	if os.Getenv("AP_WRITE_STATS_FIXTURE") == "" {
		t.Skip("设置 AP_WRITE_STATS_FIXTURE=1 以重新生成夹具")
	}
	f := statsFixture{}
	for _, c := range [][2]int{{90, 100}, {24, 24}, {60, 65}, {0, 10}, {5, 500}, {377, 1101}} {
		lo, hi, err := WilsonInterval(c[0], c[1], Z95)
		if err != nil {
			t.Fatal(err)
		}
		f.Wilson = append(f.Wilson, fxWilson{S: c[0], N: c[1], Lo: lo, Hi: hi})
	}
	for _, c := range [][2]int{{25, 10}, {0, 5}, {3, 3}, {60, 20}, {12, 8}, {8, 21}, {1, 3}} {
		p, err := McNemarExact(c[0], c[1])
		if err != nil {
			t.Fatal(err)
		}
		f.McNemar = append(f.McNemar, fxMcNemar{B: c[0], C: c[1], P: p})
	}
	kpairs := [][2][]string{
		{{"1", "1", "1", "1", "1", "0", "0", "0", "0", "0", "1", "0", "1", "0", "1"},
			{"1", "1", "0", "0", "1", "0", "0", "1", "0", "0", "1", "1", "1", "0", "0"}},
		{{"cat", "cat", "dog", "dog", "bird", "cat", "dog", "bird", "cat", "dog"},
			{"cat", "dog", "dog", "dog", "bird", "cat", "cat", "bird", "cat", "dog"}},
		{{"good", "bad", "good", "good", "bad", "bad", "good", "bad"},
			{"good", "bad", "good", "bad", "bad", "bad", "good", "good"}},
	}
	for _, kp := range kpairs {
		k, err := CohenKappa(kp[0], kp[1])
		if err != nil {
			t.Fatal(err)
		}
		f.Kappa = append(f.Kappa, fxKappa{A: kp[0], B: kp[1], K: k})
	}
	for _, c := range []struct {
		n    int
		d, o float64
	}{{71, 0.15, 0.30}, {108, 0.15, 0.30}, {80, 0.20, 0.40}, {41, 0.30, 0.40}, {200, 0.15, 0.30}, {30, 0.20, 0.40}} {
		pw, err := McNemarPower(c.n, c.d, c.o, 0.05)
		if err != nil {
			t.Fatal(err)
		}
		f.PowerMcN = append(f.PowerMcN, fxPower{N: c.n, Delta: c.d, Omega: c.o, Power: pw})
	}
	for _, seed := range []int64{20260831, 7, 42} {
		deltas := make([]float64, 40)
		for i := range deltas {
			deltas[i] = float64(i%7) - 2.5
		}
		ci, err := PairedBootstrapCI(deltas, 2000, seed)
		if err != nil {
			t.Fatal(err)
		}
		f.Boot = append(f.Boot, fxBoot{Deltas: deltas, Seed: seed, Iters: 2000, Point: ci.Point, Lower: ci.Lower, Upper: ci.Upper})
	}
	for _, p := range []float64{0.025, 0.5, 0.8, 0.95, 0.975, 0.995} {
		x, err := NormalQuantile(p)
		if err != nil {
			t.Fatal(err)
		}
		f.Quantile = append(f.Quantile, fxQuantile{P: p, X: x})
	}
	// 文案对账：刻意覆盖二进制可精确表示的平局值（1/16=0.0625、15/16=0.9375）与非平局值（1/8、1/128），锁定 %.3f 平局舍到偶的双线一致口径
	for _, c := range [][2]int{{1, 16}, {15, 16}, {1, 8}, {1, 128}, {3, 16}, {90, 100}} {
		rp, err := ReportRate(c[0], c[1])
		if err != nil {
			t.Fatal(err)
		}
		f.FormatRt = append(f.FormatRt, fxFormat{S: c[0], N: c[1], Want: rp.String()})
	}
	if err := os.MkdirAll("testdata", 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(f, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("testdata", "stats_fixtures.json"), append(data, byte(10)), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Log("夹具已写出")
}

// soak_test.go — v6.4 命题 1 压缩口径 soak harness 测试（2026-09-01 维护者裁定：
// ≥72h 真机 + ≥75 次加速崩溃注入，自愈 Wilson 下界 ≥95%——见 docs/V7路线图.md §五裁定记录）。
package live

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestWilsonLowerBound 自愈成功率 Wilson 下界：对拍手算常量（z=1.959964）。
// 全自愈 n=75 → 下界 0.9513 ≥0.95 达标（压缩口径的数学依据）；n=74 → 0.9282 不达标。
func TestWilsonLowerBound(t *testing.T) {
	cases := []struct {
		k, n int64
		want float64
	}{
		{20, 20, 0.838875},
		{75, 75, 0.951276},
		{74, 75, 0.928266},
		{0, 20, 0},
	}
	for _, c := range cases {
		got := WilsonLowerBound(c.k, c.n, WilsonZ95)
		if math.Abs(got-c.want) > 1e-4 {
			t.Errorf("WilsonLowerBound(%d,%d) = %.6f, want %.6f", c.k, c.n, got, c.want)
		}
	}
}

// TestChaosRunnerArmedPanicOnce 注入器语义：Arm 后首次 Run panic（由调用方
// Guardian 捕获），随后恢复委托；注入计数如实。
func TestChaosRunnerArmedPanicOnce(t *testing.T) {
	inner := RunnerFunc(func(task TaskSpec) (string, int, error) { return "ok", 7, nil })
	c := NewChaosRunner(inner)
	c.Arm()

	func() {
		defer func() {
			if recover() == nil {
				t.Error("Arm 后首次 Run 应触发 panic")
			}
		}()
		_, _, _ = c.Run(TaskSpec{})
	}()

	out, tokens, err := c.Run(TaskSpec{})
	if err != nil || out != "ok" || tokens != 7 {
		t.Errorf("第二次 Run 应恢复委托：out=%q tokens=%d err=%v", out, tokens, err)
	}
	if c.Injected() != 1 {
		t.Errorf("Injected() = %d, want 1", c.Injected())
	}
}

// newSoakFixture 构造确定性 soak 测试环境（fakeClock 步进）。
func newSoakFixture(t *testing.T, cfg SoakConfig) (*Soak, *Runtime, *ChaosRunner, *fakeClock) {
	t.Helper()
	fc := &fakeClock{t: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}
	chaos := NewChaosRunner(RunnerFunc(func(task TaskSpec) (string, int, error) {
		return "echo:" + task.Input, 1, nil
	}))
	rt := NewRuntime(chaos, NewWaker(fc, 0), fc, &Budget{})
	return NewSoak(rt, chaos, fc, cfg), rt, chaos, fc
}

// TestSoakStepScheduleAndHeal 步进调度：每小时步进 1 次（注入/遥测/代谢唤醒
// 各按节奏触发）；注入全部经 Guardian 自愈计入 runtime 统计；时长到即 finished。
func TestSoakStepScheduleAndHeal(t *testing.T) {
	h := time.Hour
	cfg := SoakConfig{TargetDuration: 10 * h, InjectEvery: h, WakeEvery: 2 * h, TelemetryEvery: h}
	soak, rt, chaos, fc := newSoakFixture(t, cfg)

	finished := false
	for i := 0; i < 20 && !finished; i++ {
		fc.advance(h)
		finished = soak.Step(fc.Now())
	}
	if !finished {
		t.Fatal("10h 目标时长内未报告 finished")
	}
	if got := chaos.Injected(); got != 10 {
		t.Errorf("注入次数 = %d, want 10（start 后每小时 1 次，共 10 步）", got)
	}
	st := rt.Stats()
	if st.CrashesHealed != 10 {
		t.Errorf("Runtime 自愈计数 = %d, want 10（注入即 panic → Guardian 恢复）", st.CrashesHealed)
	}
	// 代谢唤醒：start+2h 起每 2h 一次，与注入同 Step 时先注入后代谢 → 5 次
	if st.TasksDone != 15 {
		t.Errorf("任务总数 = %d, want 15（注入 10 + 代谢唤醒 5）", st.TasksDone)
	}
	if len(soak.Report().Samples) != 10 {
		t.Errorf("遥测样本 = %d, want 10", len(soak.Report().Samples))
	}
}

// TestSoakReportVerdict 判定：75 次全自愈即满足注入/点估计/Wilson 三项，但
// 时长未到 → Pass=false；72h（此处 2h 等比缩短）走满后 Pass=true。
func TestSoakReportVerdict(t *testing.T) {
	m := time.Minute
	cfg := SoakConfig{TargetDuration: 120 * m, InjectEvery: m, WakeEvery: 60 * m, TelemetryEvery: 30 * m}
	soak, _, _, fc := newSoakFixture(t, cfg)

	// +76min：elapsed 75min < 2h → duration 不达标，Pass=false（其余三项已达标）
	for i := 0; i < 76; i++ {
		fc.advance(m)
		soak.Step(fc.Now())
	}
	mid := soak.Report()
	if mid.Injections != 75 || mid.Healed != 75 {
		t.Fatalf("中途快照：注入 %d/自愈 %d, want 75/75", mid.Injections, mid.Healed)
	}
	if !mid.Verdict.InjectionsOK || !mid.Verdict.HealPointOK || !mid.Verdict.HealWilsonOK {
		t.Errorf("75 次全自愈应满足注入/点估计/Wilson 三项：%+v", mid.Verdict)
	}
	if mid.Verdict.DurationOK || mid.Verdict.Pass {
		t.Errorf("时长未到不应 Pass：%+v", mid.Verdict)
	}

	// 走满 2h → Pass=true
	for i := 0; i < 60 && !soak.Step(fc.Now()); i++ {
		fc.advance(m)
	}
	final := soak.Report()
	if !final.Verdict.Pass {
		t.Errorf("全条件满足应 Pass：%+v", final.Verdict)
	}
	if final.Verdict.AuditChainOK != true {
		t.Error("审计链应全链可复算")
	}
	if final.HealWilsonLB95 < 0.95 {
		t.Errorf("120 次全自愈 Wilson 下界 %.4f 应 ≥0.95", final.HealWilsonLB95)
	}
}

// TestSoakSelfHealFailureAccounting 自愈失败的如实口径：Runner 正常返回错误
// 不算崩溃；注入 panic 才计自愈——失败注入（若 Guardiain 外逃致 outcome 缺失）
// 计入 Failures 而非 Healed。
func TestSoakSelfHealFailureAccounting(t *testing.T) {
	m := time.Minute
	cfg := SoakConfig{TargetDuration: 3 * m, InjectEvery: m, WakeEvery: 0, TelemetryEvery: 0}
	soak, rt, chaos, fc := newSoakFixture(t, cfg)
	_ = rt
	for i := 0; i < 3; i++ {
		fc.advance(m)
		soak.Step(fc.Now())
	}
	if chaos.Injected() != 2 { // +1min 步设定基线不注入；+2min、+3min 各 1 次
		t.Errorf("注入 = %d, want 2", chaos.Injected())
	}
	rep := soak.Report()
	if rep.Healed+rep.Failures != rep.Injections {
		t.Errorf("自愈+失败应等于注入总数：%+v", rep)
	}
}

// TestSoakTelemetryStateDir 遥测：状态目录体积纳入样本（披露口径）。
func TestSoakTelemetryStateDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "audit.jsonl"), []byte("0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := time.Minute
	cfg := SoakConfig{TargetDuration: m, InjectEvery: 0, WakeEvery: 0, TelemetryEvery: m, StateDir: dir}
	soak, _, _, fc := newSoakFixture(t, cfg)
	fc.advance(m)
	soak.Step(fc.Now()) // 基线步：设定 started 与采样节奏
	fc.advance(m)
	soak.Step(fc.Now()) // 到采样点
	rep := soak.Report()
	if len(rep.Samples) != 1 {
		t.Fatalf("遥测样本 = %d, want 1", len(rep.Samples))
	}
	if rep.Samples[0].StateDirBytes < 10 {
		t.Errorf("StateDirBytes = %d, want ≥10", rep.Samples[0].StateDirBytes)
	}
	if rep.Samples[0].Goroutines <= 0 {
		t.Error("Goroutines 应 >0")
	}
}

// TestWriteSoakReport 报告 JSON 落盘可回读（中途快照 = 崩溃安全证据）。
func TestWriteSoakReport(t *testing.T) {
	dir := t.TempDir()
	rep := SoakReport{Injections: 75, Healed: 75, HealRatePoint: 1.0, HealWilsonLB95: 0.951284}
	rep.Verdict.Pass = true
	path := filepath.Join(dir, "soak-report.json")
	if err := WriteSoakReport(path, rep); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var back SoakReport
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back.Injections != 75 || !back.Verdict.Pass || back.HealWilsonLB95 < 0.95 {
		t.Errorf("回读不一致：%+v", back)
	}
}

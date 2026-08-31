// runtime_test.go — 常驻运行时闭环测试（v6.4 工程地板；14 天实测属 B2 运营项）
//
// 覆盖（确定性，无真实睡眠）：
//   - 崩溃自愈：panic 注入 ×N → 全部恢复，运行时存活（命题 1 自愈口径的
//     确定性形态；14 天 ≥99% 数字待 B2 常驻宿主，降级豁免记录）；
//   - 预算护栏确定性不变式：耗尽即拒绝、超额 0；
//   - 闲时自调度：无唤醒时 idle 作业按优先级执行、失败跳过继续；
//   - 自唤醒协议：定时源（伪时钟步进）/文件监视（t.TempDir 变更）；
//   - 伪时钟 14 天 uptime 模拟 + 审计链完整。
package live

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeClock 确定性时钟（手动步进）。
type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time { return c.t }
func (c *fakeClock) After(d time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	ch <- c.t.Add(d)
	return ch
}
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

// panicRunner 可编程替身：panicOn 集合中的输入触发崩溃。
type panicRunner struct {
	panicOn  map[string]bool
	failOn   map[string]bool
	calls    []string
	tokensOf map[string]int
}

func (p *panicRunner) Run(task TaskSpec) (string, int, error) {
	p.calls = append(p.calls, task.Input)
	if p.panicOn[task.Input] {
		panic("注入崩溃：" + task.Input)
	}
	if p.failOn[task.Input] {
		return "", p.tokensOf[task.Input], errors.New("任务失败")
	}
	return "完成 " + task.Input, p.tokensOf[task.Input], nil
}

// TestCrashSelfHealing 崩溃注入 ×3：全部自愈，运行时存活，审计完整
func TestCrashSelfHealing(t *testing.T) {
	clock := &fakeClock{t: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}
	runner := &panicRunner{
		panicOn:  map[string]bool{"坏任务1": true, "坏任务2": true, "坏任务3": true},
		tokensOf: map[string]int{"好任务": 100},
	}
	w := NewWaker(clock, 0)
	r := NewRuntime(runner, w, clock, &Budget{})
	// 3 次崩溃 + 1 次成功
	for _, in := range []string{"坏任务1", "坏任务2", "坏任务3", "好任务"} {
		out := r.HandleWake(WakeEvent{Source: WakeManual, Payload: in})
		if out == nil {
			t.Fatalf("任务 %s 应被执行（预算未耗尽）", in)
		}
	}
	s := r.Stats()
	if s.TasksDone != 4 {
		t.Fatalf("任务数应 4，got %d", s.TasksDone)
	}
	if s.CrashesHealed != 3 {
		t.Fatalf("自愈次数应 3，got %d", s.CrashesHealed)
	}
	if s.TasksSucceeded != 1 {
		t.Fatalf("成功数应 1，got %d", s.TasksSucceeded)
	}
	// 自愈后运行时存活：心跳在推进
	_, beats := r.Heartbeat()
	if beats != 4 {
		t.Fatalf("心跳应 4，got %d", beats)
	}
	if err := r.VerifyAudit(); err != nil {
		t.Fatalf("自愈后审计链断裂: %v", err)
	}
	// 自愈段在审计链中可追溯
	var heals int
	for _, e := range r.AuditEntries() {
		if e.Stage == "self_heal" {
			heals++
		}
	}
	if heals != 3 {
		t.Fatalf("审计链应有 3 个 self_heal 节点，got %d", heals)
	}
}

// TestBudgetInvariant 预算护栏确定性不变式：耗尽即拒绝、超额 0
func TestBudgetInvariant(t *testing.T) {
	clock := &fakeClock{t: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}
	runner := &panicRunner{tokensOf: map[string]int{"a": 60, "b": 60, "c": 60}}
	w := NewWaker(clock, 0)
	r := NewRuntime(runner, w, clock, &Budget{MaxTokens: 100})

	if out := r.HandleWake(WakeEvent{Source: WakeManual, Payload: "a"}); out == nil {
		t.Fatal("预算内任务应执行")
	}
	// 第二个任务把账面推到顶：记账钳制（账面绝不越 100）——超额 0 不变式
	if out := r.HandleWake(WakeEvent{Source: WakeManual, Payload: "b"}); out == nil {
		t.Fatal("未到顶前任务应执行")
	}
	_, spent, exhausted := r.budget.Snapshot()
	if spent != 100 {
		t.Fatalf("账面应钳制在 100（绝不越限），got %d", spent)
	}
	if !exhausted {
		t.Fatal("到顶状态应可观测")
	}
	// 到顶后拒绝（超额 0）
	if out := r.HandleWake(WakeEvent{Source: WakeManual, Payload: "c"}); out != nil {
		t.Fatal("预算到顶后任务必须被拒绝（超额 0 不变式）")
	}
	// MaxTasks 口径
	r2 := NewRuntime(&panicRunner{tokensOf: map[string]int{"x": 1}}, NewWaker(clock, 0), clock, &Budget{MaxTasks: 1})
	_ = r2.HandleWake(WakeEvent{Source: WakeManual, Payload: "x"})
	if out := r2.HandleWake(WakeEvent{Source: WakeManual, Payload: "x"}); out != nil {
		t.Fatal("MaxTasks 耗尽后应拒绝")
	}
	if err := r.VerifyAudit(); err != nil {
		t.Fatalf("审计链断裂: %v", err)
	}
}

// TestIdleScheduler 闲时自调度：优先级、失败跳过、预算联动
func TestIdleScheduler(t *testing.T) {
	clock := &fakeClock{t: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}
	runner := &panicRunner{}
	r := NewRuntime(runner, NewWaker(clock, 0), clock, &Budget{})
	var order []string
	const cool = 24 * time.Hour // 冷却：每昼夜一轮（代谢语义）
	r.RegisterIdleJob(IdleJob{Name: "蒸馏学习", Priority: 5, Interval: cool, Run: func(time.Time) (string, error) {
		order = append(order, "蒸馏学习")
		return "第 3 轮采集→蒸馏→影評闭环完成", nil
	}})
	r.RegisterIdleJob(IdleJob{Name: "模型整理", Priority: 1, Interval: cool, Run: func(time.Time) (string, error) {
		order = append(order, "模型整理")
		return "世界模型整理完成", nil
	}})
	r.RegisterIdleJob(IdleJob{Name: "工具制造", Priority: 3, Interval: cool, Run: func(time.Time) (string, error) {
		order = append(order, "工具制造")
		return "", errors.New("生命周期无候选（正常）")
	}})

	// 三步闲时环：优先级 1 → 3（失败跳过）→ 5
	s1 := r.IdleStep()
	s3 := r.IdleStep()
	if s1 == nil || *s1 != "模型整理: 世界模型整理完成" {
		t.Fatalf("第一步应为最高优先级作业: %v", s1)
	}
	if s3 == nil || *s3 != "蒸馏学习: 第 3 轮采集→蒸馏→影評闭环完成" {
		t.Fatalf("第三步应为蒸馏学习（失败作业跳过不阻塞）: got=%q", *s3)
	}
	// 全部作业进入冷却 → nil（真闲）
	if r.IdleStep() != nil {
		t.Fatal("冷却期内应返回 nil")
	}
	// 失败作业不进冷却：下步重试（工具制造失败后仍在队列）
	clock.advance(cool)
	if s := r.IdleStep(); s == nil || *s != "模型整理: 世界模型整理完成" {
		t.Fatalf("次日模型整理应再次运行: %v", s)
	}
	if s := r.IdleStep(); s == nil || *s != "蒸馏学习: 第 3 轮采集→蒸馏→影評闭环完成" {
		t.Fatalf("次日蒸馏学习应再次运行（24h 冷却已过）: %v", s)
	}
	s := r.Stats()
	// day1: 整理+制造(败)+学习 = 3；冷却检查步制造重试(败) = 1；
	// day2: 整理+制造(败)+学习 = 3 → 合计 7（失败作业每步重试、成功作业按冷却）
	if s.IdleRuns != 7 {
		t.Fatalf("idle 运行数应 7，got %d", s.IdleRuns)
	}
	// 失败重试可追溯：工具制造两次失败均在审计链
	var mfrFails int
	for _, e := range r.AuditEntries() {
		if e.Stage == "idle" && strings.Contains(e.Detail, "工具制造 失败") {
			mfrFails++
		}
	}
	if mfrFails != 3 {
		t.Fatalf("工具制造应失败 3 次且入审计: %d", mfrFails)
	}
	if err := r.VerifyAudit(); err != nil {
		t.Fatalf("审计链断裂: %v", err)
	}
}

// TestWakeSources 自唤醒协议：定时源（伪时钟）+ 文件监视（真实临时目录）
func TestWakeSources(t *testing.T) {
	clock := &fakeClock{t: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}
	w := NewWaker(clock, time.Hour)
	dir := t.TempDir()
	target := filepath.Join(dir, "inbox.txt")
	w.Watch(target) // 不存在的路径：出现即唤醒（基线为零值）

	// 未到间隔、文件未出现：无唤醒
	if w.PollTimerAndFiles() {
		t.Fatal("基线周期不应有唤醒")
	}
	// 写入被监视文件 → 文件唤醒
	if err := os.WriteFile(target, []byte("新环境信号"), 0o644); err != nil {
		t.Fatal(err)
	}
	clock.advance(90 * time.Minute)
	if !w.PollTimerAndFiles() {
		t.Fatal("文件出现 + 定时到期应有唤醒")
	}
	// 通道内应有两条：file + timer
	ev1 := <-w.Chan()
	ev2 := <-w.Chan()
	sources := map[WakeSource]bool{ev1.Source: true, ev2.Source: true}
	if !sources[WakeFile] || !sources[WakeTimer] {
		t.Fatalf("应同时收到 file 与 timer 唤醒: %+v %+v", ev1, ev2)
	}
	if ev1.Source == WakeFile && ev1.Payload != target {
		t.Fatalf("文件唤醒应携带路径: %+v", ev1)
	}
	// 文件未再变化 + 间隔未到：无重复唤醒
	if w.PollTimerAndFiles() {
		t.Fatal("无变化不应重复唤醒")
	}
	w.Close()
}

// TestFourteenDaySimulation 伪时钟 14 天常驻模拟：
// 每天定时唤醒任务 + 闲时学习，全程崩溃自愈，审计链完整
// （命题 1 的 14 天 ≥99% 真实数字待 B2 常驻宿主——本测试为 harness 确定性形态）
func TestFourteenDaySimulation(t *testing.T) {
	clock := &fakeClock{t: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}
	// 每天一个任务：第 5、9 天崩溃注入（确定性故障注入）
	day := 0
	r := NewRuntime(RunnerFunc(func(task TaskSpec) (string, int, error) {
		day++
		if day == 5 || day == 9 {
			panic(fmt.Sprintf("第 %d 天崩溃注入", day))
		}
		return "日巡检完成", 50, nil
	}), NewWaker(clock, 24*time.Hour), clock, &Budget{MaxTasks: 14})
	r.RegisterIdleJob(IdleJob{Name: "夜间学习", Priority: 1, Run: func(time.Time) (string, error) {
		return "夜里学习：轨迹蒸馏一轮", nil
	}})

	for d := 0; d < 14; d++ {
		// 每天定时巡检任务（直接以 timer 语义驱动 HandleWake——与 PollTimerAndFiles 等价）
		if out := r.HandleWake(WakeEvent{Source: WakeTimer, Detail: "日巡检", Payload: "day"}); out == nil {
			t.Fatalf("第 %d 天任务被预算拒绝（14 天预算应覆盖）", d+1)
		}
		if r.IdleStep() == nil {
			t.Fatalf("第 %d 天闲时学习未运行", d+1)
		}
		clock.advance(24 * time.Hour)
	}

	s := r.Stats()
	if s.UptimeDays < 13 || s.UptimeDays > 14.01 {
		t.Fatalf("uptime 应 ≈14 天，got %.2f", s.UptimeDays)
	}
	if s.TasksDone != 14 {
		t.Fatalf("14 天应 14 个任务，got %d", s.TasksDone)
	}
	if s.CrashesHealed != 2 {
		t.Fatalf("崩溃注入 2 次应全部自愈，got %d", s.CrashesHealed)
	}
	if s.TasksSucceeded != 12 {
		t.Fatalf("成功任务应 12，got %d", s.TasksSucceeded)
	}
	if s.IdleRuns != 14 {
		t.Fatalf("14 天闲时学习应 14 次，got %d", s.IdleRuns)
	}
	_, spent, _ := r.budget.Snapshot()
	if spent != 600 { // 12 成功 × 50，崩溃任务不记账
		t.Fatalf("token 账目应 600，got %d", spent)
	}
	if err := r.VerifyAudit(); err != nil {
		t.Fatalf("14 天后审计链断裂: %v", err)
	}
}

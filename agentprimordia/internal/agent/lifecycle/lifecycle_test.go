package lifecycle

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestNew 测试创建 Lifecycle
func TestNew(t *testing.T) {
	lc := New()
	if lc == nil {
		t.Fatal("New 返回 nil")
	}
	if lc.Status() != StatusIdle {
		t.Fatalf("初始状态应该为 StatusIdle，实际为 %q", lc.Status())
	}
}

// TestStatusConstants 测试状态常量值
func TestStatusConstants(t *testing.T) {
	tests := []struct {
		status   Status
		expected string
	}{
		{StatusIdle, "idle"},
		{StatusRunning, "running"},
		{StatusPaused, "paused"},
		{StatusWaitingForInput, "waiting_for_input"},
		{StatusCompleted, "completed"},
		{StatusFailed, "failed"},
		{StatusCancelled, "cancelled"},
	}

	for _, tt := range tests {
		if string(tt.status) != tt.expected {
			t.Errorf("状态常量 %q 期望为 %q，实际为 %q", tt.status, tt.expected, string(tt.status))
		}
	}
}

// TestSetStatus 测试设置状态
func TestSetStatus(t *testing.T) {
	lc := New()

	// Idle -> Running
	if err := lc.SetStatus(StatusRunning); err != nil {
		t.Fatalf("Idle -> Running 应该成功: %v", err)
	}
	if lc.Status() != StatusRunning {
		t.Fatalf("状态应该为 Running，实际为 %q", lc.Status())
	}

	// Running -> Paused
	if err := lc.SetStatus(StatusPaused); err != nil {
		t.Fatalf("Running -> Paused 应该成功: %v", err)
	}
	if lc.Status() != StatusPaused {
		t.Fatalf("状态应该为 Paused，实际为 %q", lc.Status())
	}
}

// TestSetStatusSame 测试设置相同状态（应该为空操作）
func TestSetStatusSame(t *testing.T) {
	lc := New()
	if err := lc.SetStatus(StatusIdle); err != nil {
		t.Fatalf("设置相同状态应该成功: %v", err)
	}
	if lc.TransitionCount() != 0 {
		t.Fatal("设置相同状态不应该产生状态转换记录")
	}
}

// TestSetStatusWithReason 测试带原因设置状态
func TestSetStatusWithReason(t *testing.T) {
	lc := New()
	_ = lc.SetStatusWithReason(StatusRunning, "start work")

	history := lc.History()
	if len(history) != 1 {
		t.Fatalf("期望 1 条转换记录，实际有 %d 条", len(history))
	}
	if history[0].Reason != "start work" {
		t.Fatalf("原因应该为 %q，实际为 %q", "start work", history[0].Reason)
	}
	if history[0].From != StatusIdle || history[0].To != StatusRunning {
		t.Fatalf("转换记录: From=%q, To=%q", history[0].From, history[0].To)
	}
}

// TestPauseResume 测试暂停和恢复
func TestPauseResume(t *testing.T) {
	lc := New()

	// 非运行状态暂停应该无效
	lc.Pause()
	if lc.Status() != StatusIdle {
		t.Fatal("Idle 状态下 Pause 不应该改变状态")
	}

	// 转到 Running 再暂停
	_ = lc.SetStatus(StatusRunning)
	lc.Pause()
	if lc.Status() != StatusPaused {
		t.Fatalf("状态应该为 Paused，实际为 %q", lc.Status())
	}

	// 非暂停状态恢复应该无效
	lc2 := New()
	lc2.Resume()
	if lc2.Status() != StatusIdle {
		t.Fatal("Idle 状态下 Resume 不应该改变状态")
	}

	// 暂停后恢复
	lc.Resume()
	if lc.Status() != StatusRunning {
		t.Fatalf("状态应该为 Running，实际为 %q", lc.Status())
	}
}

// TestWaitResume_AfterResume 回归测试：Resume 完成后调用 WaitResume 应立即返回。
// 修复前：WaitResume 无锁读 channel，可能读到 Resume 替换后的新 channel 而永久阻塞
// （错过唤醒）；修复后通过状态兜底检查立即返回。
func TestWaitResume_AfterResume(t *testing.T) {
	lc := New()
	_ = lc.SetStatus(StatusRunning)
	lc.Pause()
	lc.Resume()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if err := lc.WaitResume(ctx); err != nil {
		t.Fatalf("Resume 完成后 WaitResume 应立即返回，实际: %v", err)
	}
}

// TestWaitResume_MultipleWaiters 多个 waiter 并发等待时，一次 Resume 应唤醒全部。
// 回归保护：close(ch) 语义对多个 select 读者同时生效，任何 waiter 被遗漏即失败。
func TestWaitResume_MultipleWaiters(t *testing.T) {
	lc := New()
	_ = lc.SetStatus(StatusRunning)
	lc.Pause()

	const waiters = 8
	var wg sync.WaitGroup
	for i := 0; i < waiters; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := lc.WaitResume(ctx); err != nil {
				t.Errorf("waiter 超时（一次 Resume 应唤醒全部 waiter）: %v", err)
			}
		}()
	}

	// 等待所有 waiter 进入等待状态后触发 Resume
	time.Sleep(50 * time.Millisecond)
	lc.Resume()
	wg.Wait()
}

// TestWaitResume_Concurrent 并发 Pause/Resume 与 WaitResume 交错，不应阻塞或触发数据竞争。
// 修复前：WaitResume 无锁读 l.resumeCh，与 Resume 的 close+替换并发即数据竞争
// （-race 必现）；且可能读到替换后的新 channel 导致超时。修复后应零警告、零超时。
//
// 设计要点：单个写者协程以低频（带让步）执行 Pause/Resume，模拟真实控制面节奏；
// 多个 waiter 并发等待。写者风暴（多协程忙循环抢写锁）会让 RWMutex 读者饥饿，
// 那属于测试失真而非被测代码问题，故刻意避免。
func TestWaitResume_Concurrent(t *testing.T) {
	lc := New()
	_ = lc.SetStatus(StatusRunning)

	stop := make(chan struct{})
	var prWG sync.WaitGroup
	prWG.Add(1)
	// 低频 Pause/Resume 写者协程（模拟控制面），持续运行直到 waiter 全部完成
	go func() {
		defer prWG.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			lc.Pause()
			lc.Resume()
			// 让步：给等待者的 RLock 留出获取机会（真实场景 Pause/Resume 为低频操作）
			time.Sleep(50 * time.Microsecond)
		}
	}()

	// 并发等待恢复的协程
	var waitWG sync.WaitGroup
	for i := 0; i < 4; i++ {
		waitWG.Add(1)
		go func() {
			defer waitWG.Done()
			for j := 0; j < 25; j++ {
				ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
				err := lc.WaitResume(ctx)
				cancel()
				if err != nil {
					t.Errorf("WaitResume 超时（可能错过唤醒）: %v", err)
				}
			}
		}()
	}

	waitWG.Wait()
	close(stop)
	// 兜底：恢复为运行态后等待写者协程退出
	lc.Pause()
	lc.Resume()
	prWG.Wait()
}

// TestStop 测试停止
func TestStop(t *testing.T) {
	lc := New()

	if lc.IsStopped() {
		t.Fatal("新创建的 Lifecycle 不应该已停止")
	}

	lc.Stop()

	if !lc.IsStopped() {
		t.Fatal("调用 Stop 后应该已停止")
	}

	// 重复调用不应该 panic
	lc.Stop()
}

// TestStopChan 测试停止通道
func TestStopChan(t *testing.T) {
	lc := New()

	select {
	case <-lc.StopChan():
		t.Fatal("StopChan 不应该在未停止时有信号")
	default:
	}

	lc.Stop()

	select {
	case <-lc.StopChan():
		// 正确
	default:
		t.Fatal("StopChan 应该在停止后有信号")
	}
}

// TestGracefulShutdown 测试优雅关闭
func TestGracefulShutdown(t *testing.T) {
	lc := New()

	if lc.IsGracefulShutdown() {
		t.Fatal("新创建的 Lifecycle 不应该已请求优雅关闭")
	}

	// 在非运行状态下优雅关闭应该立即返回
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := lc.GracefulShutdown(ctx); err != nil {
		t.Fatalf("非运行状态优雅关闭应该成功: %v", err)
	}

	if !lc.IsGracefulShutdown() {
		t.Fatal("优雅关闭后应该标记已请求")
	}
}

// TestGracefulShutdownRunning 测试运行中优雅关闭
func TestGracefulShutdownRunning(t *testing.T) {
	lc := New()
	_ = lc.SetStatus(StatusRunning)

	// 在另一个 goroutine 中完成运行
	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = lc.SetStatus(StatusCompleted)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := lc.GracefulShutdown(ctx); err != nil {
		t.Fatalf("优雅关闭应该成功: %v", err)
	}
}

// TestGracefulShutdownContextCancel 测试优雅关闭时上下文取消
func TestGracefulShutdownContextCancel(t *testing.T) {
	lc := New()
	_ = lc.SetStatus(StatusRunning)

	ctx, cancel := context.WithCancel(context.Background())
	// 立即取消上下文
	cancel()

	err := lc.GracefulShutdown(ctx)
	if err == nil {
		t.Fatal("上下文取消后优雅关闭应该返回错误")
	}
	// 应该已被强制停止
	if !lc.IsStopped() {
		t.Fatal("上下文取消后应该强制停止")
	}
}

// TestHistory 测试状态转换历史
func TestHistory(t *testing.T) {
	lc := New()

	_ = lc.SetStatusWithReason(StatusRunning, "start")
	_ = lc.SetStatusWithReason(StatusPaused, "break")
	_ = lc.SetStatusWithReason(StatusRunning, "resume")
	_ = lc.SetStatusWithReason(StatusCompleted, "done")

	history := lc.History()
	if len(history) != 4 {
		t.Fatalf("期望 4 条转换记录，实际有 %d 条", len(history))
	}

	// 验证第一条
	if history[0].From != StatusIdle || history[0].To != StatusRunning {
		t.Errorf("第 1 条转换: From=%q, To=%q", history[0].From, history[0].To)
	}
	// 验证最后一条
	if history[3].From != StatusRunning || history[3].To != StatusCompleted {
		t.Errorf("第 4 条转换: From=%q, To=%q", history[3].From, history[3].To)
	}
}

// TestTransitionCount 测试转换次数
func TestTransitionCount(t *testing.T) {
	lc := New()
	if lc.TransitionCount() != 0 {
		t.Fatal("初始转换次数应该为 0")
	}

	_ = lc.SetStatus(StatusRunning)
	if lc.TransitionCount() != 1 {
		t.Fatalf("1 次转换后计数应该为 1，实际为 %d", lc.TransitionCount())
	}

	_ = lc.SetStatus(StatusCompleted)
	if lc.TransitionCount() != 2 {
		t.Fatalf("2 次转换后计数应该为 2，实际为 %d", lc.TransitionCount())
	}
}

// TestLastTransition 测试最后一次转换
func TestLastTransition(t *testing.T) {
	lc := New()

	// 无转换时
	_, ok := lc.LastTransition()
	if ok {
		t.Fatal("无转换时 LastTransition 应该返回 false")
	}

	_ = lc.SetStatusWithReason(StatusRunning, "go")
	trans, ok := lc.LastTransition()
	if !ok {
		t.Fatal("有转换时 LastTransition 应该返回 true")
	}
	if trans.To != StatusRunning || trans.Reason != "go" {
		t.Errorf("最后转换: To=%q, Reason=%q", trans.To, trans.Reason)
	}
}

// TestIsTerminal 测试终态判断
func TestIsTerminal(t *testing.T) {
	lc := New()

	if lc.IsTerminal() {
		t.Fatal("Idle 不是终态")
	}

	_ = lc.SetStatus(StatusRunning)
	if lc.IsTerminal() {
		t.Fatal("Running 不是终态")
	}

	_ = lc.SetStatus(StatusCompleted)
	if !lc.IsTerminal() {
		t.Fatal("Completed 是终态")
	}
}

// TestReset 测试重置状态
func TestReset(t *testing.T) {
	lc := New()
	_ = lc.SetStatus(StatusRunning)
	_ = lc.SetStatus(StatusFailed)

	if err := lc.Reset(); err != nil {
		t.Fatalf("从终态 Reset 应该成功: %v", err)
	}
	if lc.Status() != StatusIdle {
		t.Fatalf("Reset 后状态应该为 Idle，实际为 %q", lc.Status())
	}
}

// TestResetNonTerminal 测试从非终态重置
func TestResetNonTerminal(t *testing.T) {
	lc := New()
	_ = lc.SetStatus(StatusRunning)

	err := lc.Reset()
	if err == nil {
		t.Fatal("从非终态 Reset 应该返回错误")
	}
}

// TestFailCompleteCancel 测试 Fail/Complete/Cancel 快捷方法
func TestFailCompleteCancel(t *testing.T) {
	tests := []struct {
		name     string
		method   func(lc *Lifecycle) error
		expected Status
	}{
		{"Fail", func(lc *Lifecycle) error { return lc.Fail("error") }, StatusFailed},
		{"Complete", func(lc *Lifecycle) error { return lc.Complete("done") }, StatusCompleted},
		{"Cancel", func(lc *Lifecycle) error { return lc.Cancel("aborted") }, StatusCancelled},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lc := New()
			_ = lc.SetStatus(StatusRunning)
			if err := tt.method(lc); err != nil {
				t.Fatalf("%s 返回错误: %v", tt.name, err)
			}
			if lc.Status() != tt.expected {
				t.Fatalf("状态应该为 %q，实际为 %q", tt.expected, lc.Status())
			}
		})
	}
}

// TestWaitForInput 测试等待输入状态
func TestWaitForInput(t *testing.T) {
	lc := New()
	_ = lc.SetStatus(StatusRunning)

	if lc.IsWaitingForInput() {
		t.Fatal("Running 状态不应该等待输入")
	}

	_ = lc.WaitForInput("need user input")
	if !lc.IsWaitingForInput() {
		t.Fatal("WaitForInput 后应该处于等待输入状态")
	}
}

// TestCanTransitionTo 测试状态转换合法性检查
func TestCanTransitionTo(t *testing.T) {
	tests := []struct {
		from  Status
		to    Status
		valid bool
	}{
		{StatusIdle, StatusRunning, true},
		{StatusIdle, StatusPaused, false},
		{StatusRunning, StatusPaused, true},
		{StatusRunning, StatusCompleted, true},
		{StatusRunning, StatusIdle, false},
		{StatusPaused, StatusRunning, true},
		{StatusPaused, StatusCancelled, true},
		{StatusCompleted, StatusIdle, true},
		{StatusFailed, StatusIdle, true},
	}

	for _, tt := range tests {
		lc := New()
		// 先设到 from 状态
		if tt.from != StatusIdle {
			_ = lc.SetStatus(tt.from)
		}
		result := lc.CanTransitionTo(tt.to)
		if result != tt.valid {
			t.Errorf("从 %q 到 %q: 期望 %v，实际 %v", tt.from, tt.to, tt.valid, result)
		}
	}
}

// TestAvailableTransitions 测试可用转换列表
func TestAvailableTransitions(t *testing.T) {
	lc := New()
	transitions := lc.AvailableTransitions()

	// Idle 只能转到 Running
	if len(transitions) != 1 || transitions[0] != StatusRunning {
		t.Fatalf("Idle 的可用转换应该只有 Running，实际为 %v", transitions)
	}
}

// TestRetry 测试从失败状态重试
func TestRetry(t *testing.T) {
	lc := New()
	_ = lc.SetStatus(StatusRunning)
	_ = lc.Fail("error")

	if err := lc.Retry(); err != nil {
		t.Fatalf("Retry 应该成功: %v", err)
	}
	if lc.Status() != StatusRunning {
		t.Fatalf("Retry 后状态应该为 Running，实际为 %q", lc.Status())
	}
}

// TestRetryNonFailed 测试从非失败状态重试
func TestRetryNonFailed(t *testing.T) {
	lc := New()
	err := lc.Retry()
	if err == nil {
		t.Fatal("从非失败状态 Retry 应该返回错误")
	}
}

// TestOnTransition 测试状态转换监听器
func TestOnTransition(t *testing.T) {
	lc := New()

	var mu sync.Mutex
	var captured []Status
	lc.OnTransition(func(s Status) {
		mu.Lock()
		captured = append(captured, s)
		mu.Unlock()
	})

	_ = lc.SetStatus(StatusRunning)
	_ = lc.SetStatus(StatusCompleted)

	mu.Lock()
	defer mu.Unlock()
	if len(captured) != 2 {
		t.Fatalf("期望捕获 2 次状态变化，实际捕获 %d 次", len(captured))
	}
	if captured[0] != StatusRunning || captured[1] != StatusCompleted {
		t.Fatalf("捕获的状态变化: %v", captured)
	}
}

// TestRegisterHook 测试注册状态转换钩子
func TestRegisterHook(t *testing.T) {
	lc := New()

	var mu sync.Mutex
	var hookCalls []string
	lc.RegisterHook(StatusRunning, func(from, to Status) {
		mu.Lock()
		hookCalls = append(hookCalls, string(from)+"->"+string(to))
		mu.Unlock()
	})

	_ = lc.SetStatus(StatusRunning)

	mu.Lock()
	defer mu.Unlock()
	if len(hookCalls) != 1 || hookCalls[0] != "idle->running" {
		t.Fatalf("钩子调用: %v", hookCalls)
	}
}

// TestStateDuration 测试状态持续时间
func TestStateDuration(t *testing.T) {
	lc := New()

	dur := lc.StateDuration()
	if dur < 0 {
		t.Fatal("状态持续时间不应该为负数")
	}

	time.Sleep(10 * time.Millisecond)
	dur2 := lc.StateDuration()
	if dur2 <= dur {
		t.Fatal("状态持续时间应该随时间增长")
	}
}

// TestTotalRunningTime 测试累计运行时间
func TestTotalRunningTime(t *testing.T) {
	lc := New()
	_ = lc.SetStatus(StatusRunning)

	time.Sleep(20 * time.Millisecond)
	_ = lc.SetStatus(StatusPaused)

	total := lc.TotalRunningTime()
	if total < 20*time.Millisecond {
		t.Fatalf("累计运行时间应该 >= 20ms，实际为 %v", total)
	}
}

// TestSetTimeout 测试设置超时
func TestSetTimeout(t *testing.T) {
	lc := New()
	lc.SetTimeout(50 * time.Millisecond)
	_ = lc.SetStatus(StatusRunning)

	// 等待超时触发
	time.Sleep(100 * time.Millisecond)

	if lc.Status() != StatusFailed {
		t.Fatalf("超时后状态应该为 Failed，实际为 %q", lc.Status())
	}
}

// TestStateSince 测试状态开始时间
func TestStateSince(t *testing.T) {
	lc := New()
	before := time.Now()
	_ = lc.SetStatus(StatusRunning)

	since := lc.StateSince()
	if since.Before(before) {
		t.Fatal("StateSince 应该在 SetStatus 之后")
	}
}

// TestErrorConstants 测试错误变量
func TestErrorConstants(t *testing.T) {
	if ErrInvalidTransition == nil {
		t.Fatal("ErrInvalidTransition 不应该为 nil")
	}
	if ErrNotResettable == nil {
		t.Fatal("ErrNotResettable 不应该为 nil")
	}
	if ErrAgentStopped == nil {
		t.Fatal("ErrAgentStopped 不应该为 nil")
	}
}

// TestWaitPause 测试等待暂停信号
func TestWaitPause(t *testing.T) {
	lc := New()
	_ = lc.SetStatus(StatusRunning)

	go func() {
		time.Sleep(20 * time.Millisecond)
		lc.Pause()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := lc.WaitPause(ctx); err != nil {
		t.Fatalf("WaitPause 应该成功: %v", err)
	}
}

// TestWaitResume 测试等待恢复信号
func TestWaitResume(t *testing.T) {
	lc := New()
	_ = lc.SetStatus(StatusRunning)
	lc.Pause()

	go func() {
		time.Sleep(20 * time.Millisecond)
		lc.Resume()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := lc.WaitResume(ctx); err != nil {
		t.Fatalf("WaitResume 应该成功: %v", err)
	}
}

// TestConcurrentStatusReads 测试并发读取状态
func TestConcurrentStatusReads(t *testing.T) {
	lc := New()
	_ = lc.SetStatus(StatusRunning)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = lc.Status()
			_ = lc.IsTerminal()
			_ = lc.IsStopped()
			_ = lc.TransitionCount()
		}()
	}
	wg.Wait()
	// 并发读取不应该 panic
}

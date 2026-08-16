// Package lifecycle 提供 Agent 生命周期管理
package lifecycle

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Status 表示 Agent 的运行状态
type Status string

const (
	StatusIdle            Status = "idle"
	StatusRunning         Status = "running"
	StatusPaused          Status = "paused"
	StatusWaitingForInput Status = "waiting_for_input"
	StatusCompleted       Status = "completed"
	StatusFailed          Status = "failed"
	StatusCancelled       Status = "cancelled"
)

// StateTransition 记录状态转换
type StateTransition struct {
	From      Status
	To        Status
	Timestamp time.Time
	Reason    string
}

// StateHook 是状态转换钩子函数
type StateHook func(from, to Status)

// 错误定义
var (
	ErrInvalidTransition = errors.New("invalid state transition")
	ErrNotResettable     = errors.New("state is not resettable")
	ErrAgentStopped      = errors.New("agent is stopped")
)

// Lifecycle 管理 Agent 的生命周期状态
type Lifecycle struct {
	mu              sync.RWMutex
	status          Status
	stopCh          chan struct{}
	stopOnce        sync.Once
	pauseCh         chan struct{}
	resumeCh        chan struct{}
	listeners       []func(Status)
	hooks           map[Status][]StateHook
	history         []StateTransition
	stateSince      time.Time
	totalRunning    time.Duration
	timeoutTimer    *time.Timer
	timeoutDur      time.Duration
	gracefulCh      chan struct{}
	gracefulOnce    sync.Once
	runningDone     chan struct{}
	runningDoneOnce sync.Once
}

// New 创建新的生命周期管理器
func New() *Lifecycle {
	return &Lifecycle{
		status:      StatusIdle,
		stopCh:      make(chan struct{}),
		pauseCh:     make(chan struct{}, 1),
		resumeCh:    make(chan struct{}),
		hooks:       make(map[Status][]StateHook),
		history:     make([]StateTransition, 0, 10),
		stateSince:  time.Now(),
		gracefulCh:  make(chan struct{}),
		runningDone: make(chan struct{}),
	}
}

// Status 返回当前状态
func (l *Lifecycle) Status() Status {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.status
}

// SetStatus 设置状态
func (l *Lifecycle) SetStatus(status Status) error {
	return l.SetStatusWithReason(status, "")
}

// SetStatusWithReason 设置状态并附带原因
func (l *Lifecycle) SetStatusWithReason(status Status, reason string) error {
	l.mu.Lock()

	if l.status == status {
		l.mu.Unlock()
		return nil
	}

	now := time.Now()
	duration := now.Sub(l.stateSince)

	if l.status == StatusRunning {
		l.totalRunning += duration
	}

	from := l.status
	transition := StateTransition{
		From:      from,
		To:        status,
		Timestamp: now,
		Reason:    reason,
	}
	l.history = append(l.history, transition)
	l.status = status
	l.stateSince = now

	l.manageTimeoutTimer(status)
	l.manageRunningDone(status)

	// 锁内复制 listener/hook 列表
	listeners := make([]func(Status), len(l.listeners))
	copy(listeners, l.listeners)
	var statusHooks []StateHook
	if hooks, ok := l.hooks[status]; ok {
		statusHooks = make([]StateHook, len(hooks))
		copy(statusHooks, hooks)
	}
	l.mu.Unlock()

	// 锁外调用 listener/hook
	for _, listener := range listeners {
		listener(status)
	}
	for _, hook := range statusHooks {
		hook(transition.From, transition.To)
	}
	return nil
}

// manageTimeoutTimer 管理状态超时定时器
func (l *Lifecycle) manageTimeoutTimer(status Status) {
	if l.timeoutTimer != nil {
		l.timeoutTimer.Stop()
		l.timeoutTimer = nil
	}

	if status == StatusRunning && l.timeoutDur > 0 {
		l.timeoutTimer = time.AfterFunc(l.timeoutDur, func() {
			if l.Status() == StatusRunning {
				_ = l.SetStatusWithReason(StatusFailed, "running timeout exceeded")
			}
		})
	}
}

// manageRunningDone 管理运行完成信号
func (l *Lifecycle) manageRunningDone(status Status) {
	if status != StatusRunning {
		l.runningDoneOnce.Do(func() {
			close(l.runningDone)
		})
	}
}

// GracefulShutdown 请求优雅关闭
func (l *Lifecycle) GracefulShutdown(ctx context.Context) error {
	l.gracefulOnce.Do(func() {
		close(l.gracefulCh)
	})

	l.mu.RLock()
	isRunning := l.status == StatusRunning
	l.mu.RUnlock()

	if !isRunning {
		return nil
	}

	select {
	case <-l.runningDone:
		return nil
	case <-ctx.Done():
		l.Stop()
		return ctx.Err()
	}
}

// IsGracefulShutdown 检查是否已请求优雅关闭
func (l *Lifecycle) IsGracefulShutdown() bool {
	select {
	case <-l.gracefulCh:
		return true
	default:
		return false
	}
}

// Stop 停止 Agent
func (l *Lifecycle) Stop() {
	l.stopOnce.Do(func() {
		close(l.stopCh)
	})
}

// StopChan 返回停止通道
func (l *Lifecycle) StopChan() <-chan struct{} {
	return l.stopCh
}

// IsStopped 检查是否已停止
func (l *Lifecycle) IsStopped() bool {
	select {
	case <-l.stopCh:
		return true
	default:
		return false
	}
}

// Pause 暂停 Agent
func (l *Lifecycle) Pause() {
	l.mu.Lock()
	if l.status != StatusRunning {
		l.mu.Unlock()
		return
	}
	from := l.status
	now := time.Now()
	duration := now.Sub(l.stateSince)
	l.totalRunning += duration
	l.history = append(l.history, StateTransition{
		From:      from,
		To:        StatusPaused,
		Timestamp: now,
		Reason:    "pause",
	})
	l.status = StatusPaused
	l.stateSince = now
	l.manageTimeoutTimer(StatusPaused)

	// 在锁内复制 listeners 和 hooks
	listeners := make([]func(Status), len(l.listeners))
	copy(listeners, l.listeners)
	pausedHooks := make([]StateHook, 0)
	if hooks, ok := l.hooks[StatusPaused]; ok {
		pausedHooks = append(pausedHooks, hooks...)
	}
	l.mu.Unlock()

	// 锁外发送暂停信号，且为非阻塞发送。
	// 修复（评估实测发现）：原实现在持有写锁时向 buffered(1) 的 pauseCh 发送，
	// 若信号未被 WaitPause 消费（buffer 已满），第二次 Pause 会锁内永久阻塞，
	// 并连带饿死所有等待 RLock 的调用方（如 WaitResume）。
	// 非阻塞发送 + 缓冲 1 的语义：信号未被消费时无需重复发送。
	select {
	case l.pauseCh <- struct{}{}:
	default:
	}

	for _, listener := range listeners {
		listener(StatusPaused)
	}
	for _, hook := range pausedHooks {
		hook(from, StatusPaused)
	}
}

// Resume 恢复 Agent
func (l *Lifecycle) Resume() {
	l.mu.Lock()
	if l.status != StatusPaused {
		l.mu.Unlock()
		return
	}
	from := l.status
	now := time.Now()
	l.history = append(l.history, StateTransition{
		From:      from,
		To:        StatusRunning,
		Timestamp: now,
		Reason:    "resume",
	})
	l.status = StatusRunning
	l.stateSince = now
	l.manageTimeoutTimer(StatusRunning)

	// 在锁内复制 listeners 和 hooks
	listeners := make([]func(Status), len(l.listeners))
	copy(listeners, l.listeners)
	runningHooks := make([]StateHook, 0)
	if hooks, ok := l.hooks[StatusRunning]; ok {
		runningHooks = append(runningHooks, hooks...)
	}

	close(l.resumeCh)
	l.resumeCh = make(chan struct{})
	l.mu.Unlock()

	for _, listener := range listeners {
		listener(StatusRunning)
	}
	for _, hook := range runningHooks {
		hook(from, StatusRunning)
	}
}

// WaitPause 等待暂停信号
func (l *Lifecycle) WaitPause(ctx context.Context) error {
	select {
	case <-l.pauseCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-l.stopCh:
		return errors.New("agent is stopped")
	}
}

// WaitResume 等待恢复信号
//
// 修复（评估实测发现）：Resume 会在锁内 close(resumeCh) 后替换为新 channel，
// 旧实现无锁读取 l.resumeCh 存在两个问题：
//   - 数据竞争：与 Resume 的 close+替换并发读写同一字段（race 检测确认）；
//   - 错过唤醒：等待者若读到替换后的新 channel（尚未被 close），
//     会永久阻塞直到下一次 Resume。
//
// 新实现：在锁内快照 channel 引用并同时检查状态——若状态已切回 Running
// （Resume 已完成），直接返回；否则在锁外等待快照到的 channel。
// 状态检查 + 锁内快照可覆盖所有交错，既消除数据竞争，也避免错过唤醒。
func (l *Lifecycle) WaitResume(ctx context.Context) error {
	l.mu.RLock()
	ch := l.resumeCh
	resumed := l.status == StatusRunning
	l.mu.RUnlock()
	if resumed {
		// Resume 已完成（状态已切回 Running），无需等待
		return nil
	}
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-l.stopCh:
		return errors.New("agent is stopped")
	}
}

// RegisterHook 注册状态转换钩子
func (l *Lifecycle) RegisterHook(targetStatus Status, hook StateHook) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.hooks[targetStatus] = append(l.hooks[targetStatus], hook)
}

// OnTransition 注册状态转换监听器
func (l *Lifecycle) OnTransition(fn func(Status)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.listeners = append(l.listeners, fn)
}

// History 返回状态转换历史
func (l *Lifecycle) History() []StateTransition {
	l.mu.RLock()
	defer l.mu.RUnlock()
	result := make([]StateTransition, len(l.history))
	copy(result, l.history)
	return result
}

// StateDuration 返回当前状态持续时间
func (l *Lifecycle) StateDuration() time.Duration {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return time.Since(l.stateSince)
}

// TotalRunningTime 返回累计运行时间
func (l *Lifecycle) TotalRunningTime() time.Duration {
	l.mu.RLock()
	defer l.mu.RUnlock()
	total := l.totalRunning
	if l.status == StatusRunning {
		total += time.Since(l.stateSince)
	}
	return total
}

// SetTimeout 设置运行超时
func (l *Lifecycle) SetTimeout(d time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.timeoutDur = d
}

// Reset 重置状态到 Idle
func (l *Lifecycle) Reset() error {
	l.mu.Lock()
	isTerminal := l.status == StatusCompleted || l.status == StatusFailed || l.status == StatusCancelled
	if !isTerminal {
		status := l.status
		l.mu.Unlock()
		return errors.New("only terminal states can be reset, current: " + string(status))
	}

	// 重置所有可重置的字段
	l.stopCh = make(chan struct{})
	l.stopOnce = sync.Once{}
	l.pauseCh = make(chan struct{}, 1)
	l.resumeCh = make(chan struct{})
	l.gracefulCh = make(chan struct{})
	l.gracefulOnce = sync.Once{}
	l.runningDone = make(chan struct{})
	l.runningDoneOnce = sync.Once{}

	// 执行状态转换
	now := time.Now()
	l.history = append(l.history, StateTransition{
		From:      l.status,
		To:        StatusIdle,
		Timestamp: now,
		Reason:    "reset",
	})
	l.status = StatusIdle
	l.stateSince = now

	// 在锁内复制 listeners
	listeners := make([]func(Status), len(l.listeners))
	copy(listeners, l.listeners)

	l.mu.Unlock()

	// 在锁外通知
	for _, listener := range listeners {
		listener(StatusIdle)
	}

	return nil
}

// Fail 设置失败状态
func (l *Lifecycle) Fail(reason string) error {
	return l.SetStatusWithReason(StatusFailed, reason)
}

// Complete 设置完成状态
func (l *Lifecycle) Complete(reason string) error {
	return l.SetStatusWithReason(StatusCompleted, reason)
}

// Cancel 设置取消状态
func (l *Lifecycle) Cancel(reason string) error {
	return l.SetStatusWithReason(StatusCancelled, reason)
}

// WaitForInput 设置等待输入状态
func (l *Lifecycle) WaitForInput(reason string) error {
	return l.SetStatusWithReason(StatusWaitingForInput, reason)
}

// IsWaitingForInput 检查是否等待输入
func (l *Lifecycle) IsWaitingForInput() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.status == StatusWaitingForInput
}

// CanTransitionTo 检查是否可以转换到指定状态
func (l *Lifecycle) CanTransitionTo(target Status) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return isValidTransition(l.status, target)
}

// AvailableTransitions 返回可用的状态转换
func (l *Lifecycle) AvailableTransitions() []Status {
	l.mu.RLock()
	defer l.mu.RUnlock()
	allowed := validTransitions[l.status]
	result := make([]Status, len(allowed))
	copy(result, allowed)
	return result
}

// Retry 重试，从失败状态恢复
func (l *Lifecycle) Retry() error {
	l.mu.Lock()
	if l.status != StatusFailed {
		l.mu.Unlock()
		return errors.New("only failed state can be retried, current: " + string(l.status))
	}
	l.mu.Unlock()
	return l.SetStatusWithReason(StatusRunning, "retry")
}

// TransitionCount 返回状态转换次数
func (l *Lifecycle) TransitionCount() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.history)
}

// LastTransition 返回最后一次状态转换
func (l *Lifecycle) LastTransition() (StateTransition, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if len(l.history) == 0 {
		return StateTransition{}, false
	}
	return l.history[len(l.history)-1], true
}

// StateSince 返回当前状态开始时间
func (l *Lifecycle) StateSince() time.Time {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.stateSince
}

// IsTerminal 检查是否为终态
func (l *Lifecycle) IsTerminal() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.status == StatusCompleted || l.status == StatusFailed || l.status == StatusCancelled
}

// isValidTransition 检查状态转换是否合法
func isValidTransition(from, to Status) bool {
	allowed, ok := validTransitions[from]
	if !ok {
		return false
	}
	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}

// validTransitions 定义合法的状态转换
var validTransitions = map[Status][]Status{
	StatusIdle:            {StatusRunning},
	StatusRunning:         {StatusPaused, StatusWaitingForInput, StatusCompleted, StatusFailed, StatusCancelled},
	StatusPaused:          {StatusRunning, StatusCancelled},
	StatusWaitingForInput: {StatusRunning, StatusCancelled, StatusFailed},
	StatusCompleted:       {StatusIdle},
	StatusFailed:          {StatusIdle},
	StatusCancelled:       {StatusIdle},
}

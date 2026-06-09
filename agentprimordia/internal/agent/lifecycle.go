package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// validTransitions 定义 Agent 状态机的合法转换规则
var validTransitions = map[AgentStatus][]AgentStatus{
	StatusIdle:            {StatusRunning},
	StatusRunning:         {StatusPaused, StatusWaitingForInput, StatusCompleted, StatusFailed, StatusCancelled},
	StatusPaused:          {StatusRunning, StatusCancelled},
	StatusWaitingForInput: {StatusRunning, StatusCancelled, StatusFailed},
	StatusCompleted:       {StatusIdle},
	StatusFailed:          {StatusIdle},
	StatusCancelled:       {StatusIdle},
}

// ErrInvalidTransition 表示非法的状态转换
var ErrInvalidTransition = errors.New("invalid status transition")

// ErrNotResettable 表示当前状态不支持重置
var ErrNotResettable = errors.New("current status is not resettable, only terminal states can be reset")

// StateTransition 状态转换记录
type StateTransition struct {
	From      AgentStatus   `json:"from"`
	To        AgentStatus   `json:"to"`
	Timestamp time.Time     `json:"timestamp"`
	Duration  time.Duration `json:"duration,omitempty"`
	Reason    string        `json:"reason,omitempty"`
}

// StateHook 状态转换钩子函数类型
type StateHook func(from, to AgentStatus)

// TransitionGuard 状态转换守卫，返回 true 允许转换，false 拒绝
type TransitionGuard func(from, to AgentStatus) bool

// Lifecycle manages the agent's lifecycle states
type Lifecycle struct {
	mu              sync.RWMutex
	status          AgentStatus
	stopCh          chan struct{}
	stopOnce        sync.Once
	pauseCh         chan struct{}
	resumeCh        chan struct{}
	listeners       []func(AgentStatus)
	hooks           map[AgentStatus][]StateHook
	guards          []TransitionGuard
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

// NewLifecycle creates a new lifecycle manager
func NewLifecycle() *Lifecycle {
	return &Lifecycle{
		status:      StatusIdle,
		stopCh:      make(chan struct{}),
		pauseCh:     make(chan struct{}, 1),
		resumeCh:    make(chan struct{}),
		hooks:       make(map[AgentStatus][]StateHook),
		guards:      make([]TransitionGuard, 0),
		history:     make([]StateTransition, 0, 10),
		stateSince:  time.Now(),
		gracefulCh:  make(chan struct{}),
		runningDone: make(chan struct{}),
	}
}

// Status returns current agent status
func (l *Lifecycle) Status() AgentStatus {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.status
}

// SetStatus updates the agent status with validation
func (l *Lifecycle) SetStatus(status AgentStatus) error {
	return l.SetStatusWithReason(status, "")
}

// SetStatusWithReason 更新状态并附带原因
func (l *Lifecycle) SetStatusWithReason(status AgentStatus, reason string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.status == status {
		return nil
	}

	if !isValidTransition(l.status, status) {
		return fmt.Errorf("%w: from %q to %q", ErrInvalidTransition, l.status, status)
	}

	for _, guard := range l.guards {
		if !guard(l.status, status) {
			return fmt.Errorf("%w: transition from %q to %q blocked by guard", ErrInvalidTransition, l.status, status)
		}
	}

	now := time.Now()
	duration := now.Sub(l.stateSince)

	if l.status == StatusRunning {
		l.totalRunning += duration
	}

	transition := StateTransition{
		From:      l.status,
		To:        status,
		Timestamp: now,
		Duration:  duration,
		Reason:    reason,
	}
	l.history = append(l.history, transition)
	l.status = status
	l.stateSince = now

	l.manageTimeoutTimer(status)
	l.manageRunningDone(status)

	for _, listener := range l.listeners {
		listener(status)
	}

	if hooks, ok := l.hooks[status]; ok {
		for _, hook := range hooks {
			hook(transition.From, transition.To)
		}
	}
	return nil
}

// manageTimeoutTimer 管理状态超时定时器
func (l *Lifecycle) manageTimeoutTimer(status AgentStatus) {
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
// 当状态离开 Running 时，关闭 runningDone 通道通知等待者
func (l *Lifecycle) manageRunningDone(status AgentStatus) {
	if status != StatusRunning {
		l.runningDoneOnce.Do(func() {
			close(l.runningDone)
		})
	}
}

// GracefulShutdown 请求优雅关闭
// 标记优雅关闭信号，等待当前运行中的 Agent 完成当前 turn
// 如果 ctx 超时，则回退到强制停止
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

// isValidTransition 检查从 from 到 to 的状态转换是否合法
func isValidTransition(from, to AgentStatus) bool {
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

// CanTransitionTo 检查当前状态是否可以转换到目标状态
func (l *Lifecycle) CanTransitionTo(status AgentStatus) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return isValidTransition(l.status, status)
}

// IsTerminal 检查当前状态是否为终态
func (l *Lifecycle) IsTerminal() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.status == StatusCompleted || l.status == StatusFailed || l.status == StatusCancelled
}

// IsWaitingForInput 检查是否正在等待人类输入
func (l *Lifecycle) IsWaitingForInput() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.status == StatusWaitingForInput
}

// WaitForInput 将状态设为 WaitingForInput
func (l *Lifecycle) WaitForInput(reason string) error {
	return l.SetStatusWithReason(StatusWaitingForInput, reason)
}

// AvailableTransitions 返回当前状态可用的所有转换目标
func (l *Lifecycle) AvailableTransitions() []AgentStatus {
	l.mu.RLock()
	defer l.mu.RUnlock()

	allowed := validTransitions[l.status]
	result := make([]AgentStatus, len(allowed))
	copy(result, allowed)
	return result
}

// Reset 从终态重置回 Idle，允许 Agent 重新运行
// 只有 Completed/Failed/Cancelled 状态可以重置
func (l *Lifecycle) Reset() error {
	l.mu.Lock()
	isTerminal := l.status == StatusCompleted || l.status == StatusFailed || l.status == StatusCancelled
	if !isTerminal {
		status := l.status
		l.mu.Unlock()
		return fmt.Errorf("%w: only terminal states (completed/failed/cancelled) can be reset, current: %q", ErrInvalidTransition, status)
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

	// 在锁内复制 listeners 和 hooks，避免锁外竞态
	listeners := make([]func(AgentStatus), len(l.listeners))
	copy(listeners, l.listeners)
	idleHooks := make([]StateHook, 0)
	if hooks, ok := l.hooks[StatusIdle]; ok {
		idleHooks = append(idleHooks, hooks...)
	}

	l.mu.Unlock()

	// 使用复制的数据在锁外通知
	for _, listener := range listeners {
		listener(StatusIdle)
	}
	for _, hook := range idleHooks {
		hook(StatusIdle, StatusIdle)
	}

	return nil
}

// Retry 从 Failed 状态重试运行，等价于 Reset + SetStatus(Running)
func (l *Lifecycle) Retry() error {
	l.mu.RLock()
	isFailed := l.status == StatusFailed
	l.mu.RUnlock()

	if !isFailed {
		return fmt.Errorf("%w: retry is only allowed from failed state, current: %q", ErrInvalidTransition, l.Status())
	}
	if err := l.SetStatusWithReason(StatusIdle, "retry-reset"); err != nil {
		return err
	}
	return l.SetStatusWithReason(StatusRunning, "retry")
}

// SetTimeout 设置 Running 状态的超时时间
// 超时后自动转为 Failed 状态
func (l *Lifecycle) SetTimeout(d time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.timeoutDur = d
}

// Stop signals the agent to stop
func (l *Lifecycle) Stop() {
	l.stopOnce.Do(func() {
		close(l.stopCh)
	})
}

// StopChan returns the stop channel
func (l *Lifecycle) StopChan() <-chan struct{} {
	return l.stopCh
}

// IsStopped checks if stop was signaled
func (l *Lifecycle) IsStopped() bool {
	select {
	case <-l.stopCh:
		return true
	default:
		return false
	}
}

// Pause pauses the agent execution
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
		Duration:  duration,
		Reason:    "pause",
	})
	l.status = StatusPaused
	l.stateSince = now
	l.manageTimeoutTimer(StatusPaused)

	// 在锁内复制 listeners 和 hooks
	listeners := make([]func(AgentStatus), len(l.listeners))
	copy(listeners, l.listeners)
	pausedHooks := make([]StateHook, 0)
	if hooks, ok := l.hooks[StatusPaused]; ok {
		pausedHooks = append(pausedHooks, hooks...)
	}

	l.pauseCh <- struct{}{}
	l.mu.Unlock()

	for _, listener := range listeners {
		listener(StatusPaused)
	}
	for _, hook := range pausedHooks {
		hook(from, StatusPaused)
	}
}

// Resume resumes a paused agent
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
	listeners := make([]func(AgentStatus), len(l.listeners))
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

// WaitPause blocks until pause signal or context done
func (l *Lifecycle) WaitPause(ctx context.Context) error {
	select {
	case <-l.pauseCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-l.stopCh:
		return ErrAgentStopped
	}
}

// ErrAgentStopped is returned when the agent is stopped
var ErrAgentStopped = errors.New("agent is stopped")

// WaitResume blocks until resume signal or context done
func (l *Lifecycle) WaitResume(ctx context.Context) error {
	select {
	case <-l.resumeCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-l.stopCh:
		return ErrAgentStopped
	}
}

// RegisterHook 注册状态进入时的钩子函数
func (l *Lifecycle) RegisterHook(targetStatus AgentStatus, hook StateHook) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.hooks[targetStatus] = append(l.hooks[targetStatus], hook)
}

// OnTransition 注册全局状态转换监听器
func (l *Lifecycle) OnTransition(fn func(AgentStatus)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.listeners = append(l.listeners, fn)
}

// AddGuard 添加状态转换守卫
func (l *Lifecycle) AddGuard(guard TransitionGuard) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.guards = append(l.guards, guard)
}

// History 返回状态转换历史记录
func (l *Lifecycle) History() []StateTransition {
	l.mu.RLock()
	defer l.mu.RUnlock()
	result := make([]StateTransition, len(l.history))
	copy(result, l.history)
	return result
}

// StateDuration 返回当前状态的持续时间
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

// TransitionCount 返回状态转换次数
func (l *Lifecycle) TransitionCount() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.history)
}

// LastTransition 返回最近一次状态转换记录
func (l *Lifecycle) LastTransition() (StateTransition, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if len(l.history) == 0 {
		return StateTransition{}, false
	}
	return l.history[len(l.history)-1], true
}

// StateSince 返回进入当前状态的时间
func (l *Lifecycle) StateSince() time.Time {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.stateSince
}

// Fail 将状态设为 Failed 并附带原因
func (l *Lifecycle) Fail(reason string) error {
	return l.SetStatusWithReason(StatusFailed, reason)
}

// Complete 将状态设为 Completed 并附带原因
func (l *Lifecycle) Complete(reason string) error {
	return l.SetStatusWithReason(StatusCompleted, reason)
}

// Cancel 将状态设为 Cancelled 并附带原因
func (l *Lifecycle) Cancel(reason string) error {
	return l.SetStatusWithReason(StatusCancelled, reason)
}

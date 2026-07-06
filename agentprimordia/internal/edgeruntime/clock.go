// Package edgeruntime 的时钟抽象（Phase 5 Task 7）。
//
// Edge Runtime 通常 time.Now() 也可用，但对一些 timer-based 行为需要可注入
// 时钟以便测试。
package edgeruntime

import (
	"sync"
	"time"
)

// Clock 是抽象的时钟接口。
type Clock interface {
	Now() time.Time
	NewTimer(d time.Duration) Timer
	Sleep(d time.Duration)
}

// Timer 是 time.Timer 的抽象。
type Timer interface {
	C() <-chan time.Time
	Stop() bool
	Reset(d time.Duration) bool
}

// ===========================================================================
// SystemClock：默认（真实）实现
// ===========================================================================

// SystemClock 基于 time.Now() / time.NewTimer()。
type SystemClock struct{}

// NewSystemClock 构造默认时钟。
func NewSystemClock() *SystemClock { return &SystemClock{} }

// Now 返回当前时间。
func (c *SystemClock) Now() time.Time { return time.Now() }

// NewTimer 返回基于 time.Timer 的实现。
func (c *SystemClock) NewTimer(d time.Duration) Timer {
	return &systemTimer{t: time.NewTimer(d)}
}

// Sleep 阻塞 d 时间。
func (c *SystemClock) Sleep(d time.Duration) { time.Sleep(d) }

type systemTimer struct {
	t *time.Timer
}

func (s *systemTimer) C() <-chan time.Time   { return s.t.C }
func (s *systemTimer) Stop() bool            { return s.t.Stop() }
func (s *systemTimer) Reset(d time.Duration) bool { return s.t.Reset(d) }

// ===========================================================================
// FakeClock：测试用可控时钟
// ===========================================================================

// FakeClock 允许测试时手动推进时间。
//
// 使用方式：
//   fc := edgeruntime.NewFakeClock()
//   fc.Advance(time.Second)
//   fc.Now() // 比构造时晚 1 秒
type FakeClock struct {
	mu      sync.Mutex
	now     time.Time
	pending []*fakeTimer
}

// NewFakeClock 以给定时间构造；time.Now() 等价。
func NewFakeClock(start time.Time) *FakeClock {
	return &FakeClock{now: start}
}

// NewFakeClockNow 以当前时间构造。
func NewFakeClockNow() *FakeClock { return NewFakeClock(time.Now()) }

// Now 返回当前假时间。
func (f *FakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

// SetNow 直接设置时间（测试用）。
func (f *FakeClock) SetNow(t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = t
}

// Advance 推进时间并触发到期的 timer。
func (f *FakeClock) Advance(d time.Duration) {
	f.mu.Lock()
	f.now = f.now.Add(d)
	fired := make([]*fakeTimer, 0)
	remaining := make([]*fakeTimer, 0)
	for _, t := range f.pending {
		if t.dueAt.Before(f.now) || t.dueAt.Equal(f.now) {
			t.fired = true
			t.c <- t.dueAt
			fired = append(fired, t)
		} else {
			remaining = append(remaining, t)
		}
	}
	f.pending = remaining
	f.mu.Unlock()
}

// NewTimer 创建一个假 timer（由 Advance 推进触发）。
func (f *FakeClock) NewTimer(d time.Duration) Timer {
	t := &fakeTimer{
		c:      make(chan time.Time, 1),
		dueAt:  f.Now().Add(d),
		clock:  f,
		active: true,
	}
	f.mu.Lock()
	f.pending = append(f.pending, t)
	f.mu.Unlock()
	return t
}

// Sleep 通过 Advance 模拟睡眠（测试时阻塞）。
func (f *FakeClock) Sleep(d time.Duration) {
	end := f.Now().Add(d)
	for f.Now().Before(end) {
		time.Sleep(time.Millisecond) // 让出控制权
	}
}

type fakeTimer struct {
	c      chan time.Time
	dueAt  time.Time
	clock  *FakeClock
	mu     sync.Mutex
	active bool
	fired  bool
	stopped bool
}

func (t *fakeTimer) C() <-chan time.Time { return t.c }

func (t *fakeTimer) Stop() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.active {
		return false
	}
	t.active = false
	return true
}

func (t *fakeTimer) Reset(d time.Duration) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.dueAt = t.clock.Now().Add(d)
	t.fired = false
	t.active = true
	// 重新加入 pending 列表由调用方负责（简化为不重入）
	return true
}
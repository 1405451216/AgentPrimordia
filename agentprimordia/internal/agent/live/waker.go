// waker.go — 自唤醒协议（定时 / 文件监视 / webhook / 手动注入）
//
// 设计：Waker 聚合多唤醒源，产出到统一通道；真实时钟源注入（测试用
// 确定性时钟）；文件监视用轮询 mtime（标准库 io/fs 语义，跨平台、无
// 系统调用依赖——kqueue/inotify 抽象差异不进框架）。
package live

import (
	"os"
	"sync"
	"time"
)

// WakeChannel 唤醒事件出口（运行时主循环消费）。
type WakeChannel chan WakeEvent

// Clock 时钟抽象（确定性测试注入点）。
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

// RealClock 真实时钟。
type RealClock struct{}

// Now 实现 Clock。
func (RealClock) Now() time.Time { return time.Now() }

// After 实现 Clock。
func (RealClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// Waker 多源唤醒聚合器。
type Waker struct {
	ch     WakeChannel
	clock  Clock
	mu     sync.Mutex
	closed bool

	// 文件监视注册表：路径 → 上次 mtime
	watching map[string]time.Time
	// 定时唤醒间隔（0 = 不启用定时源）
	interval time.Duration
	lastTick time.Time
}

// NewWaker 构造（缓冲 16 的唤醒通道）。
func NewWaker(clock Clock, interval time.Duration) *Waker {
	if interval <= 0 {
		interval = 0
	}
	return &Waker{
		ch:       make(WakeChannel, 16),
		clock:    clock,
		watching: make(map[string]time.Time),
		interval: interval,
	}
}

// Chan 暴露唤醒通道（只读语义由使用方约定）。
func (w *Waker) Chan() WakeChannel { return w.ch }

// Emit 注入一次唤醒（webhook/手动共用入口；通道关闭后静默丢弃）。
func (w *Waker) Emit(ev WakeEvent) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	if ev.At.IsZero() {
		ev.At = w.clock.Now()
	}
	select {
	case w.ch <- ev:
	default:
		// 通道满：丢弃并保留最新（唤醒风暴防御——事件源自带细节，任务
		// 侧以幂等输入消化；不阻塞 webhook 面）
	}
}

// Watch 注册文件监视路径（首个观测周期只记基线不唤醒——防启动风暴）。
func (w *Waker) Watch(path string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, err := os.Stat(path); err == nil {
		if info, err2 := os.Stat(path); err2 == nil {
			w.watching[path] = info.ModTime()
		}
	} else {
		w.watching[path] = time.Time{} // 尚不存在的路径：出现即唤醒
	}
}

// PollTimerAndFiles 定时器与文件源的巡检步（由运行时 idle 环周期调用；
// 返回是否发出过唤醒——纯轮询无 goroutine 泄漏面）。
func (w *Waker) PollTimerAndFiles() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return false
	}
	emitted := false
	now := w.clock.Now()
	// 定时源
	if w.interval > 0 {
		if w.lastTick.IsZero() {
			w.lastTick = now // 基线
		}
		if now.Sub(w.lastTick) >= w.interval {
			w.lastTick = now
			w.emitLocked(WakeEvent{Source: WakeTimer, Detail: w.interval.String()})
			emitted = true
		}
	}
	// 文件源
	for path, last := range w.watching {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		mt := info.ModTime()
		if last.IsZero() || mt.After(last) {
			w.watching[path] = mt
			w.emitLocked(WakeEvent{Source: WakeFile, Detail: path, Payload: path})
			emitted = true
		}
	}
	return emitted
}

// emitLocked 须持锁投递。
func (w *Waker) emitLocked(ev WakeEvent) {
	if ev.At.IsZero() {
		ev.At = w.clock.Now()
	}
	select {
	case w.ch <- ev:
	default:
	}
}

// Close 关闭唤醒通道（优雅停机）。
func (w *Waker) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.closed {
		w.closed = true
		close(w.ch)
	}
}

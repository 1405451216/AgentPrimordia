package autonomy

import (
	"context"
	"sync"
	"time"
)

// SchedulerConfig 调度器配置
type SchedulerConfig struct {
	// Interval 定时唤醒间隔（零值表示不启用定时调度）
	Interval time.Duration
	// CronInterval cron 式定时间隔（与 Interval 等价，语义区分）
	CronInterval time.Duration
	// EventBufferSize 事件驱动通道缓冲大小（默认 64）
	EventBufferSize int
}

// Scheduler 自治调度器：支持定时唤醒与事件驱动
type Scheduler struct {
	cfg      SchedulerConfig
	mu       sync.Mutex
	tickFns  []func()
	eventFns []func(string)
	eventCh  chan string
	wg       sync.WaitGroup
	started  bool
}

// NewScheduler 创建调度器
func NewScheduler(cfg SchedulerConfig) *Scheduler {
	if cfg.EventBufferSize <= 0 {
		cfg.EventBufferSize = 64
	}
	// CronInterval 优先于 Interval
	if cfg.CronInterval > 0 && cfg.Interval == 0 {
		cfg.Interval = cfg.CronInterval
	}
	return &Scheduler{
		cfg:     cfg,
		eventCh: make(chan string, cfg.EventBufferSize),
	}
}

// OnTick 注册定时唤醒回调
func (s *Scheduler) OnTick(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tickFns = append(s.tickFns, fn)
}

// OnEvent 注册事件驱动回调
func (s *Scheduler) OnEvent(fn func(event string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.eventFns = append(s.eventFns, fn)
}

// EmitEvent 发送事件触发调度
func (s *Scheduler) EmitEvent(event string) {
	select {
	case s.eventCh <- event:
	default:
		// 缓冲满时丢弃（非阻塞）
	}
}

// Start 启动调度器（非阻塞）
func (s *Scheduler) Start(ctx context.Context) {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.mu.Unlock()

	// 定时调度协程
	if s.cfg.Interval > 0 {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			ticker := time.NewTicker(s.cfg.Interval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					s.fireTicks()
				}
			}
		}()
	}

	// 事件驱动协程
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case event := <-s.eventCh:
				s.fireEvents(event)
			}
		}
	}()
}

// Wait 等待调度器所有协程退出
func (s *Scheduler) Wait() {
	s.wg.Wait()
}

// fireTicks 触发所有定时回调
func (s *Scheduler) fireTicks() {
	s.mu.Lock()
	fns := make([]func(), len(s.tickFns))
	copy(fns, s.tickFns)
	s.mu.Unlock()

	for _, fn := range fns {
		fn()
	}
}

// fireEvents 触发所有事件回调
func (s *Scheduler) fireEvents(event string) {
	s.mu.Lock()
	fns := make([]func(string), len(s.eventFns))
	copy(fns, s.eventFns)
	s.mu.Unlock()

	for _, fn := range fns {
		fn(event)
	}
}

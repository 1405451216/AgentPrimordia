package autonomy

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestSchedulerInterval 验证定时调度
func TestSchedulerInterval(t *testing.T) {
	var mu sync.Mutex
	var fired []string

	s := NewScheduler(SchedulerConfig{
		Interval: 50 * time.Millisecond,
	})
	s.OnTick(func() {
		mu.Lock()
		fired = append(fired, "tick")
		mu.Unlock()
	})

	ctx, cancel := context.WithCancel(context.Background())
	s.Start(ctx)

	time.Sleep(180 * time.Millisecond)
	cancel()
	s.Wait()

	mu.Lock()
	count := len(fired)
	mu.Unlock()

	if count < 2 {
		t.Errorf("expected at least 2 ticks, got %d", count)
	}
}

// TestSchedulerEventDriven 验证事件驱动调度
func TestSchedulerEventDriven(t *testing.T) {
	var mu sync.Mutex
	var triggered []string

	s := NewScheduler(SchedulerConfig{})
	s.OnEvent(func(event string) {
		mu.Lock()
		triggered = append(triggered, event)
		mu.Unlock()
	})

	ctx, cancel := context.WithCancel(context.Background())
	s.Start(ctx)

	s.EmitEvent("dependency_ready")
	s.EmitEvent("external_trigger")

	time.Sleep(50 * time.Millisecond)
	cancel()
	s.Wait()

	mu.Lock()
	count := len(triggered)
	mu.Unlock()

	if count != 2 {
		t.Errorf("expected 2 events, got %d", count)
	}
}

// TestSchedulerStop 验证调度器停止
func TestSchedulerStop(t *testing.T) {
	s := NewScheduler(SchedulerConfig{
		Interval: 10 * time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())
	s.Start(ctx)
	time.Sleep(30 * time.Millisecond)
	cancel()
	s.Wait()

	// 停止后不应再有 tick
	var mu sync.Mutex
	count := 0
	s.OnTick(func() {
		mu.Lock()
		count++
		mu.Unlock()
	})
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	c := count
	mu.Unlock()
	if c > 0 {
		t.Errorf("expected 0 ticks after stop, got %d", c)
	}
}

// TestSchedulerCronStyle 验证 cron 式定时唤醒（整点触发）
func TestSchedulerCronStyle(t *testing.T) {
	var mu sync.Mutex
	fired := false

	s := NewScheduler(SchedulerConfig{
		CronInterval: 50 * time.Millisecond,
	})
	s.OnTick(func() {
		mu.Lock()
		fired = true
		mu.Unlock()
	})

	ctx, cancel := context.WithCancel(context.Background())
	s.Start(ctx)
	time.Sleep(80 * time.Millisecond)
	cancel()
	s.Wait()

	mu.Lock()
	f := fired
	mu.Unlock()
	if !f {
		t.Error("cron-style scheduler should have fired")
	}
}

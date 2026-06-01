package agent

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestLifecycle_InitialState(t *testing.T) {
	lc := NewLifecycle()

	if lc.Status() != StatusIdle {
		t.Errorf("expected StatusIdle, got %s", lc.Status())
	}
}

func TestLifecycle_SetStatus(t *testing.T) {
	lc := NewLifecycle()

	if err := lc.SetStatus(StatusRunning); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lc.Status() != StatusRunning {
		t.Errorf("expected StatusRunning, got %s", lc.Status())
	}

	if err := lc.SetStatus(StatusPaused); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lc.Status() != StatusPaused {
		t.Errorf("expected StatusPaused, got %s", lc.Status())
	}
}

func TestLifecycle_Stop(t *testing.T) {
	lc := NewLifecycle()

	if lc.IsStopped() {
		t.Error("should not be stopped initially")
	}

	lc.Stop()

	if !lc.IsStopped() {
		t.Error("should be stopped after Stop()")
	}
}

func TestLifecycle_StopChan(t *testing.T) {
	lc := NewLifecycle()

	select {
	case <-lc.StopChan():
		t.Error("stop chan should not be closed initially")
	default:
	}

	lc.Stop()

	select {
	case <-lc.StopChan():
	default:
		t.Error("stop chan should be closed after Stop()")
	}
}

func TestLifecycle_PauseAndResume(t *testing.T) {
	lc := NewLifecycle()

	_ = lc.SetStatus(StatusRunning)
	lc.Pause()

	if lc.Status() != StatusPaused {
		t.Errorf("expected StatusPaused, got %s", lc.Status())
	}

	lc.Resume()

	if lc.Status() != StatusRunning {
		t.Errorf("expected StatusRunning after resume, got %s", lc.Status())
	}
}

func TestLifecycle_PauseOnlyWhenRunning(t *testing.T) {
	lc := NewLifecycle()

	lc.Pause()

	if lc.Status() != StatusIdle {
		t.Errorf("pause on idle agent should not change status, got %s", lc.Status())
	}
}

func TestLifecycle_ResumeOnlyWhenPaused(t *testing.T) {
	lc := NewLifecycle()

	lc.Resume()

	if lc.Status() != StatusIdle {
		t.Errorf("resume on idle agent should not change status, got %s", lc.Status())
	}
}

func TestLifecycle_WaitPause(t *testing.T) {
	lc := NewLifecycle()
	_ = lc.SetStatus(StatusRunning)

	go func() {
		time.Sleep(20 * time.Millisecond)
		lc.Pause()
	}()

	err := lc.WaitPause(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLifecycle_WaitResume(t *testing.T) {
	lc := NewLifecycle()
	_ = lc.SetStatus(StatusRunning)

	go func() {
		time.Sleep(20 * time.Millisecond)
		lc.Pause()
		time.Sleep(20 * time.Millisecond)
		lc.Resume()
	}()

	lc.WaitPause(context.Background())
	err := lc.WaitResume(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLifecycle_WaitPauseContextCancel(t *testing.T) {
	lc := NewLifecycle()
	_ = lc.SetStatus(StatusRunning)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err := lc.WaitPause(ctx)
	if err != context.DeadlineExceeded {
		t.Errorf("expected context deadline, got %v", err)
	}
}

func TestLifecycle_WaitResumeContextCancel(t *testing.T) {
	lc := NewLifecycle()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err := lc.WaitResume(ctx)
	if err == nil {
		t.Error("expected error from canceled context")
	}
}

func TestLifecycle_ConcurrentStatusAccess(t *testing.T) {
	lc := NewLifecycle()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			lc.Status()
		}()
		go func() {
			defer wg.Done()
			_ = lc.SetStatus(StatusRunning)
		}()
	}
	wg.Wait()
}

func TestLifecycle_StatusTransitions(t *testing.T) {
	lc := NewLifecycle()

	transitions := []AgentStatus{
		StatusRunning,
		StatusPaused,
		StatusRunning,
		StatusCompleted,
	}

	for _, status := range transitions {
		if err := lc.SetStatus(status); err != nil {
			t.Fatalf("transition to %s failed: %v", status, err)
		}
		if lc.Status() != status {
			t.Errorf("expected %s, got %s", status, lc.Status())
		}
	}
}

func TestLifecycle_GracefulShutdown_IdleAgent(t *testing.T) {
	lc := NewLifecycle()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := lc.GracefulShutdown(ctx)
	if err != nil {
		t.Errorf("graceful shutdown on idle agent should return nil, got %v", err)
	}
}

func TestLifecycle_GracefulShutdown_WaitsForCompletion(t *testing.T) {
	lc := NewLifecycle()
	_ = lc.SetStatus(StatusRunning)

	done := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		done <- lc.GracefulShutdown(ctx)
	}()

	time.Sleep(50 * time.Millisecond)

	_ = lc.SetStatus(StatusCompleted)

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	case <-time.After(time.Second):
		t.Error("graceful shutdown should complete after agent finishes")
	}
}

func TestLifecycle_GracefulShutdown_TimeoutFallback(t *testing.T) {
	lc := NewLifecycle()
	_ = lc.SetStatus(StatusRunning)

	done := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		done <- lc.GracefulShutdown(ctx)
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Error("expected context deadline error, got nil")
		}
		if !lc.IsStopped() {
			t.Error("lifecycle should be stopped after timeout fallback")
		}
	case <-time.After(time.Second):
		t.Error("graceful shutdown should timeout")
	}
}

func TestLifecycle_IsGracefulShutdown(t *testing.T) {
	lc := NewLifecycle()

	if lc.IsGracefulShutdown() {
		t.Error("should not be in graceful shutdown initially")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = lc.GracefulShutdown(ctx)

	if !lc.IsGracefulShutdown() {
		t.Error("should be in graceful shutdown after calling GracefulShutdown")
	}
}

func TestLifecycle_GracefulShutdown_ResetClearsState(t *testing.T) {
	lc := NewLifecycle()
	_ = lc.SetStatus(StatusRunning)
	_ = lc.SetStatus(StatusFailed)

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	_ = lc.GracefulShutdown(ctx)

	_ = lc.Reset()

	if lc.IsGracefulShutdown() {
		t.Error("reset should clear graceful shutdown state")
	}
}

package tools

import (
	"errors"
	"testing"
	"time"
)

func TestResourceLimiter_AcquireRelease(t *testing.T) {
	lim := NewResourceLimiter(ResourceLimits{
		ConcurrentCalls: 2,
	})

	if err := lim.Acquire(ResourceConcurrent); err != nil {
		t.Fatalf("first acquire should succeed: %v", err)
	}
	if err := lim.Acquire(ResourceConcurrent); err != nil {
		t.Fatalf("second acquire should succeed: %v", err)
	}
	// 第三次应阻塞等待（修复评估报告 §5.2：此前立即返回错误不等待）
	done := make(chan error, 1)
	go func() { done <- lim.Acquire(ResourceConcurrent) }()
	select {
	case err := <-done:
		t.Fatalf("third acquire 不应立即返回, got %v", err)
	case <-time.After(200 * time.Millisecond):
		// 正确：仍在等待
	}

	lim.Release(ResourceConcurrent)
	// 释放后等待者应成功获取
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("acquire after release should succeed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("等待者未被唤醒")
	}
}

func TestResourceLimiter_WaitTimeout(t *testing.T) {
	// 验证等待超时返回 ErrResourceTimeout（修复评估报告 §5.2：ErrResourceTimeout 死代码）
	lim := NewResourceLimiter(ResourceLimits{ConcurrentCalls: 1})
	_ = lim.Acquire(ResourceConcurrent)

	start := time.Now()
	err := lim.Acquire(ResourceConcurrent)
	if !errors.Is(err, ErrResourceTimeout) {
		t.Fatalf("期望 ErrResourceTimeout, got %v", err)
	}
	if elapsed := time.Since(start); elapsed < acquireTimeout-300*time.Millisecond {
		t.Fatalf("应等待约 %v 后超时, 实际 %v", acquireTimeout, elapsed)
	}
}

func TestResourceLimiter_ContextTimeout(t *testing.T) {
	lim := NewResourceLimiter(ResourceLimits{
		ConcurrentCalls: 1,
	})

	_ = lim.Acquire(ResourceConcurrent)

	done := make(chan error, 1)
	go func() {
		done <- lim.Acquire(ResourceConcurrent)
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected blocking or error")
		}
	case <-time.After(200 * time.Millisecond):
		// Good - it blocked
	}

	lim.Release(ResourceConcurrent)
}

func TestResourceLimiter_CheckLimit(t *testing.T) {
	lim := NewResourceLimiter(ResourceLimits{
		MaxMemoryMB:     100,
		MaxFileOps:      5,
		ConcurrentCalls: 3,
	})

	// 检查合法资源使用
	err := lim.CheckLimit(ResourceLimits{
		MaxMemoryMB:     90,
		MaxFileOps:      4,
		ConcurrentCalls: 2,
	})
	if err != nil {
		t.Errorf("should be within limits, got: %v", err)
	}

	// 检查超出内存
	err = lim.CheckLimit(ResourceLimits{
		MaxMemoryMB: 200,
	})
	if err == nil {
		t.Fatal("expected memory exceeded error")
	}

	// 检查超出文件操作
	err = lim.CheckLimit(ResourceLimits{
		MaxFileOps: 10,
	})
	if err == nil {
		t.Fatal("expected file ops exceeded error")
	}
}

func TestResourceLimiter_Usage(t *testing.T) {
	lim := NewResourceLimiter(ResourceLimits{
		ConcurrentCalls: 5,
		MaxMemoryMB:     1024,
	})

	_ = lim.Acquire(ResourceConcurrent)
	_ = lim.Acquire(ResourceConcurrent)

	usage := lim.Usage()
	if usage.ConcurrentCalls != 2 {
		t.Errorf("expected 2 concurrent, got %d", usage.ConcurrentCalls)
	}
	if usage.MaxConcurrent != 5 {
		t.Errorf("expected max 5, got %d", usage.MaxConcurrent)
	}
}

func TestSessionLimiter(t *testing.T) {
	global := NewResourceLimiter(ResourceLimits{
		ConcurrentCalls: 10,
		MaxMemoryMB:     1024,
	})
	session := NewSessionLimiter(global, ResourceLimits{
		ConcurrentCalls: 2,
		MaxMemoryMB:     100,
	})

	// 在 session 限制内
	_ = session.Acquire(ResourceConcurrent)
	_ = session.Acquire(ResourceConcurrent)

	// 第三次超出 session 限制：阻塞等待（修复评估报告 §5.2）
	done := make(chan error, 1)
	go func() { done <- session.Acquire(ResourceConcurrent) }()
	select {
	case err := <-done:
		t.Fatalf("第三次 session acquire 不应立即返回, got %v", err)
	case <-time.After(200 * time.Millisecond):
		// 正确：仍在等待
	}

	// global 仍有余量
	if err := global.Acquire(ResourceConcurrent); err != nil {
		t.Fatalf("global should still have capacity: %v", err)
	}
	global.Release(ResourceConcurrent)

	// 释放一个 session 槽位后等待者应成功
	session.Release(ResourceConcurrent)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("session acquire after release should succeed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("session 等待者未被唤醒")
	}
}

func TestResourceLimiter_AllResourceTypes(t *testing.T) {
	lim := NewResourceLimiter(ResourceLimits{
		MaxNetworkReqs: 3,
		MaxFileOps:     4,
	})

	for i := 0; i < 3; i++ {
		if err := lim.Acquire(ResourceNetwork); err != nil {
			t.Fatalf("network acquire %d: %v", i, err)
		}
	}
	// 第 4 个 network 应阻塞等待
	netDone := make(chan error, 1)
	go func() { netDone <- lim.Acquire(ResourceNetwork) }()
	select {
	case err := <-netDone:
		t.Fatalf("network 第 4 次不应立即返回, got %v", err)
	case <-time.After(200 * time.Millisecond):
	}
	lim.Release(ResourceNetwork)
	select {
	case err := <-netDone:
		if err != nil {
			t.Fatalf("network acquire after release: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("network 等待者未被唤醒")
	}

	for i := 0; i < 4; i++ {
		if err := lim.Acquire(ResourceFileIO); err != nil {
			t.Fatalf("fileio acquire %d: %v", i, err)
		}
	}
	// 第 5 个 fileio 应阻塞等待
	ioDone := make(chan error, 1)
	go func() { ioDone <- lim.Acquire(ResourceFileIO) }()
	select {
	case err := <-ioDone:
		t.Fatalf("fileio 第 5 次不应立即返回, got %v", err)
	case <-time.After(200 * time.Millisecond):
	}
	lim.Release(ResourceFileIO)
	select {
	case err := <-ioDone:
		if err != nil {
			t.Fatalf("fileio acquire after release: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("fileio 等待者未被唤醒")
	}
}

func TestResourceLimiter_MemoryTracking(t *testing.T) {
	// 修复评估报告 §5.2：memoryMB 此前从不跟踪
	lim := NewResourceLimiter(ResourceLimits{MaxMemoryMB: 2})

	_ = lim.Acquire(ResourceMemory)
	_ = lim.Acquire(ResourceMemory)
	if u := lim.Usage(); u.MemoryMB != 2 {
		t.Fatalf("MemoryMB = %d, want 2", u.MemoryMB)
	}
	// 超出限额阻塞
	memDone := make(chan error, 1)
	go func() { memDone <- lim.Acquire(ResourceMemory) }()
	select {
	case err := <-memDone:
		t.Fatalf("memory 第 3 次不应立即返回, got %v", err)
	case <-time.After(200 * time.Millisecond):
	}
	lim.Release(ResourceMemory)
	select {
	case err := <-memDone:
		if err != nil {
			t.Fatalf("memory acquire after release: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("memory 等待者未被唤醒")
	}
}

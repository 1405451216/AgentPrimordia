package tools

import (
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
	// 第三次应该失败 (超过并发限制)
	if err := lim.Acquire(ResourceConcurrent); err == nil {
		t.Fatal("expected resource exceeded error on third acquire")
	}
	if err := lim.Acquire(ResourceConcurrent); err != ErrResourceExceeded {
		t.Errorf("expected ErrResourceExceeded, got %v", err)
	}

	lim.Release(ResourceConcurrent)
	// 释放后应能再获取
	if err := lim.Acquire(ResourceConcurrent); err != nil {
		t.Fatalf("acquire after release should succeed: %v", err)
	}
}

func TestResourceLimiter_ContextTimeout(t *testing.T) {
	lim := NewResourceLimiter(ResourceLimits{
		ConcurrentCalls: 1,
	})

	lim.Acquire(ResourceConcurrent)

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
		MaxMemoryMB:    100,
		MaxFileOps:     5,
		ConcurrentCalls: 3,
	})

	// 检查合法资源使用
	err := lim.CheckLimit(ResourceLimits{
		MaxMemoryMB:    90,
		MaxFileOps:     4,
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
		MaxMemoryMB:  1024,
	})

	lim.Acquire(ResourceConcurrent)
	lim.Acquire(ResourceConcurrent)

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
		MaxMemoryMB: 1024,
	})
	session := NewSessionLimiter(global, ResourceLimits{
		ConcurrentCalls: 2,
		MaxMemoryMB: 100,
	})

	// 在 session 限制内
	session.Acquire(ResourceConcurrent)
	session.Acquire(ResourceConcurrent)

	// 第三次超出 session 限制
	if err := session.Acquire(ResourceConcurrent); err == nil {
		t.Fatal("expected session limit exceeded")
	}

	// 但 global 还有余量
	if err := global.Acquire(ResourceConcurrent); err != nil {
		t.Fatalf("global should still have capacity: %v", err)
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
	if err := lim.Acquire(ResourceNetwork); err == nil {
		t.Fatal("expected network limit exceeded")
	}

	for i := 0; i < 4; i++ {
		if err := lim.Acquire(ResourceFileIO); err != nil {
			t.Fatalf("fileio acquire %d: %v", i, err)
		}
	}
	if err := lim.Acquire(ResourceFileIO); err == nil {
		t.Fatal("expected fileio limit exceeded")
	}
}

package tools

import (
	"errors"
	"sync"
	"time"
)

var (
	ErrResourceExceeded = errors.New("tools: resource limit exceeded")
	ErrResourceTimeout  = errors.New("tools: resource acquire timeout")
)

type ResourceType int

const (
	ResourceConcurrent ResourceType = iota
	ResourceNetwork
	ResourceFileIO
	// ResourceMemory 内存配额（MaxMemoryMB，按 1MB 单位计数）
	ResourceMemory
)

// acquireTimeout 资源等待超时（修复评估报告 §5.2：此前 Acquire 超限直接
// 报错不等待，cond 字段与 ErrResourceTimeout 均为死代码）。
const acquireTimeout = 2 * time.Second

type ResourceLimits struct {
	ConcurrentCalls int
	MaxMemoryMB     int
	MaxFileOps      int
	MaxNetworkReqs  int
}

type ResourceUsage struct {
	ConcurrentCalls int
	MaxConcurrent   int
	MemoryMB        int
	MaxMemoryMB     int
	FileOps         int
	MaxFileOps      int
	NetworkReqs     int
	MaxNetworkReqs  int
}

type ResourceLimiter struct {
	limits ResourceLimits
	usage  struct {
		concurrent  int
		memoryMB    int
		fileOps     int
		networkReqs int
	}
	mu   sync.Mutex
	cond *sync.Cond
}

func NewResourceLimiter(limits ResourceLimits) *ResourceLimiter {
	rl := &ResourceLimiter{
		limits: limits,
	}
	rl.cond = sync.NewCond(&rl.mu)
	return rl
}

func (rl *ResourceLimiter) Acquire(rt ResourceType) error {
	deadline := time.Now().Add(acquireTimeout)
	rl.mu.Lock()
	defer rl.mu.Unlock()

	for {
		// 当前资源是否可获取（limit <= 0 表示无上限）
		switch rt {
		case ResourceConcurrent:
			if rl.limits.ConcurrentCalls <= 0 || rl.usage.concurrent < rl.limits.ConcurrentCalls {
				rl.usage.concurrent++
				return nil
			}
		case ResourceNetwork:
			if rl.limits.MaxNetworkReqs <= 0 || rl.usage.networkReqs < rl.limits.MaxNetworkReqs {
				rl.usage.networkReqs++
				return nil
			}
		case ResourceFileIO:
			if rl.limits.MaxFileOps <= 0 || rl.usage.fileOps < rl.limits.MaxFileOps {
				rl.usage.fileOps++
				return nil
			}
		case ResourceMemory:
			if rl.limits.MaxMemoryMB <= 0 || rl.usage.memoryMB < rl.limits.MaxMemoryMB {
				rl.usage.memoryMB++
				return nil
			}
		default:
			return ErrResourceExceeded
		}

		// 等待 Release 广播；超时到期同样唤醒以返回 ErrResourceTimeout
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return ErrResourceTimeout
		}
		timer := time.AfterFunc(remaining, func() { rl.cond.Broadcast() })
		rl.cond.Wait()
		timer.Stop()
	}
}

func (rl *ResourceLimiter) Release(rt ResourceType) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	switch rt {
	case ResourceConcurrent:
		if rl.usage.concurrent > 0 {
			rl.usage.concurrent--
		}
	case ResourceNetwork:
		if rl.usage.networkReqs > 0 {
			rl.usage.networkReqs--
		}
	case ResourceFileIO:
		if rl.usage.fileOps > 0 {
			rl.usage.fileOps--
		}
	case ResourceMemory:
		if rl.usage.memoryMB > 0 {
			rl.usage.memoryMB--
		}
	}
	rl.cond.Broadcast()
}

func (rl *ResourceLimiter) CheckLimit(limits ResourceLimits) error {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if limits.MaxMemoryMB > 0 && rl.usage.memoryMB+limits.MaxMemoryMB > rl.limits.MaxMemoryMB {
		return ErrResourceExceeded
	}
	if limits.MaxFileOps > 0 && rl.usage.fileOps+limits.MaxFileOps > rl.limits.MaxFileOps {
		return ErrResourceExceeded
	}
	if limits.ConcurrentCalls > 0 && rl.usage.concurrent+limits.ConcurrentCalls > rl.limits.ConcurrentCalls {
		return ErrResourceExceeded
	}
	return nil
}

func (rl *ResourceLimiter) Usage() ResourceUsage {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	return ResourceUsage{
		ConcurrentCalls: rl.usage.concurrent,
		MaxConcurrent:   rl.limits.ConcurrentCalls,
		MemoryMB:        rl.usage.memoryMB,
		MaxMemoryMB:     rl.limits.MaxMemoryMB,
		FileOps:         rl.usage.fileOps,
		MaxFileOps:      rl.limits.MaxFileOps,
		NetworkReqs:     rl.usage.networkReqs,
		MaxNetworkReqs:  rl.limits.MaxNetworkReqs,
	}
}

type SessionLimiter struct {
	global *ResourceLimiter
	local  *ResourceLimiter
}

func NewSessionLimiter(global *ResourceLimiter, limits ResourceLimits) *SessionLimiter {
	return &SessionLimiter{
		global: global,
		local:  NewResourceLimiter(limits),
	}
}

func (s *SessionLimiter) Acquire(rt ResourceType) error {
	return s.local.Acquire(rt)
}

func (s *SessionLimiter) Release(rt ResourceType) {
	s.local.Release(rt)
}

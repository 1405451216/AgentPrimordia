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
)

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
	rl.mu.Lock()
	defer rl.mu.Unlock()

	switch rt {
	case ResourceConcurrent:
		if rl.limits.ConcurrentCalls > 0 && rl.usage.concurrent >= rl.limits.ConcurrentCalls {
			return ErrResourceExceeded
		}
		rl.usage.concurrent++
		return nil
	case ResourceNetwork:
		if rl.limits.MaxNetworkReqs > 0 && rl.usage.networkReqs >= rl.limits.MaxNetworkReqs {
			return ErrResourceExceeded
		}
		rl.usage.networkReqs++
		return nil
	case ResourceFileIO:
		if rl.limits.MaxFileOps > 0 && rl.usage.fileOps >= rl.limits.MaxFileOps {
			return ErrResourceExceeded
		}
		rl.usage.fileOps++
		return nil
	default:
		return ErrResourceExceeded
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

var _ = time.Second

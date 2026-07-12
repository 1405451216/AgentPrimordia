package mesh

import (
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// HealthChecker monitors agent health with active TTL expiry and passive
// success-rate tracking.
type HealthChecker struct {
	mu       sync.RWMutex
	registry Registry
	ttl      time.Duration
	stats    map[string]*agentHealthStats
	stopCh   chan struct{}
	stopped  atomic.Bool
	logger   *slog.Logger
}

type agentHealthStats struct {
	totalReqs   atomic.Int64
	successReqs atomic.Int64
}

func NewHealthChecker(registry Registry, ttl time.Duration, logger *slog.Logger) *HealthChecker {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &HealthChecker{
		registry: registry,
		ttl:      ttl,
		stats:    make(map[string]*agentHealthStats),
		stopCh:   make(chan struct{}),
		logger:   logger,
	}
}

func (h *HealthChecker) Start() {
	if h.stopped.Load() {
		return
	}
	go h.loop()
}

func (h *HealthChecker) Stop() {
	if h.stopped.CompareAndSwap(false, true) {
		close(h.stopCh)
	}
}

func (h *HealthChecker) loop() {
	ticker := time.NewTicker(h.ttl / 3)
	defer ticker.Stop()
	for !h.stopped.Load() {
		select {
		case <-h.stopCh:
			return
		case <-ticker.C:
			h.evictExpired()
		}
	}
}

func (h *HealthChecker) evictExpired() {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	healthy, _ := h.registry.Discover("")
	for _, a := range healthy {
		if now.Sub(a.LastHeartbeat) > h.ttl && a.Status != AgentStatusUnhealthy {
			h.registry.Deregister(a.ID)
			h.logger.Info("mesh: evicted unhealthy agent", "id", a.ID)
		}
	}
}

func (h *HealthChecker) GetStats(agentID string) (total, success int64) {
	h.mu.RLock()
	s, ok := h.stats[agentID]
	h.mu.RUnlock()
	if !ok {
		return 0, 0
	}
	return s.totalReqs.Load(), s.successReqs.Load()
}

func (h *HealthChecker) RecordRequest(agentID string, success bool) {
	h.mu.RLock()
	s, ok := h.stats[agentID]
	h.mu.RUnlock()
	if !ok {
		h.mu.Lock()
		s, ok = h.stats[agentID]
		if !ok {
			s = &agentHealthStats{}
			h.stats[agentID] = s
		}
		h.mu.Unlock()
	}
	s.totalReqs.Add(1)
	if success {
		s.successReqs.Add(1)
	}
}

func (h *HealthChecker) SuccessRate(agentID string) float64 {
	total, success := h.GetStats(agentID)
	if total == 0 {
		return 0
	}
	return float64(success) / float64(total)
}
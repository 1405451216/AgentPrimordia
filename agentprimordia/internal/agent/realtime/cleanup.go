package realtime

import (
	"context"
	"sync"
	"time"
)

// CleanupConfig 清理配置
type CleanupConfig struct {
	// IdleTimeout 空闲超时（默认 5 分钟）
	IdleTimeout time.Duration
	// CheckInterval 检查间隔（默认 30 秒）
	CheckInterval time.Duration
	// OnCleanup 清理回调
	OnCleanup func(sessionID string)
}

// CleanupManager 会话超时与清理管理器
type CleanupManager struct {
	cfg      CleanupConfig
	hub      *RealtimeHub
	mu       sync.Mutex
	lastActive map[string]time.Time
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// NewCleanupManager 创建清理管理器
func NewCleanupManager(hub *RealtimeHub, cfg CleanupConfig) *CleanupManager {
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = 5 * time.Minute
	}
	if cfg.CheckInterval <= 0 {
		cfg.CheckInterval = 30 * time.Second
	}
	return &CleanupManager{
		cfg:        cfg,
		hub:        hub,
		lastActive: make(map[string]time.Time),
	}
}

// Start 启动清理协程
func (cm *CleanupManager) Start(ctx context.Context) {
	ctx, cm.cancel = context.WithCancel(ctx)
	cm.wg.Add(1)
	go func() {
		defer cm.wg.Done()
		ticker := time.NewTicker(cm.cfg.CheckInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cm.sweep()
			}
		}
	}()
}

// Stop 停止清理协程
func (cm *CleanupManager) Stop() {
	if cm.cancel != nil {
		cm.cancel()
	}
	cm.wg.Wait()
}

// Touch 更新会话活跃时间
func (cm *CleanupManager) Touch(sessionID string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.lastActive[sessionID] = time.Now()
}

// sweep 扫描并清理超时会话
func (cm *CleanupManager) sweep() {
	cm.mu.Lock()
	now := time.Now()
	var expired []string
	for id, last := range cm.lastActive {
		if now.Sub(last) > cm.cfg.IdleTimeout {
			expired = append(expired, id)
		}
	}
	for _, id := range expired {
		delete(cm.lastActive, id)
	}
	cm.mu.Unlock()

	for _, id := range expired {
		cm.hub.CloseSession(id)
		if cm.cfg.OnCleanup != nil {
			cm.cfg.OnCleanup(id)
		}
	}
}

// TrackedCount 返回追踪的会话数
func (cm *CleanupManager) TrackedCount() int {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	return len(cm.lastActive)
}

package memory

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	ErrPolicyNotFound = errors.New("lifecycle policy not found")
)

// LifecycleAction 生命周期执行的动作类型
type LifecycleAction string

const (
	ActionArchive  LifecycleAction = "archive"
	ActionDelete   LifecycleAction = "delete"
	ActionCompress LifecycleAction = "compress"
)

// RetentionPolicy 数据保留策略
type RetentionPolicy struct {
	WorkingMemoryTTL  time.Duration `json:"working_memory_ttl"`
	SemanticMemoryTTL time.Duration `json:"semantic_memory_ttl"`
	EpisodeMemoryTTL  time.Duration `json:"episode_memory_ttl"`
	SessionTTL        time.Duration `json:"session_ttl"`
}

// DefaultRetentionPolicy 返回默认保留策略
func DefaultRetentionPolicy() RetentionPolicy {
	return RetentionPolicy{
		WorkingMemoryTTL:  7 * 24 * time.Hour,   // 7 天
		SemanticMemoryTTL: 90 * 24 * time.Hour,  // 90 天
		EpisodeMemoryTTL:  365 * 24 * time.Hour, // 365 天
		SessionTTL:        30 * 24 * time.Hour,  // 30 天
	}
}

// LifecycleReport 生命周期执行报告
type LifecycleReport struct {
	ArchivedEpisodes   int64            `json:"archived_episodes"`
	DeletedEpisodes    int64            `json:"deleted_episodes"`
	CompressedEpisodes int64            `json:"compressed_episodes"`
	Details            map[string]int64 `json:"details"`
	ExecutedAt         time.Time        `json:"executed_at"`
}

// LifecycleHook 生命周期回调接口
type LifecycleHook interface {
	OnArchive(ctx context.Context, episodeIDs []string) error
	OnDelete(ctx context.Context, episodeIDs []string) error
	OnCompress(ctx context.Context, episodeIDs []string) (int64, error)
}

// NoOpLifecycleHook 空实现
type NoOpLifecycleHook struct{}

func (h *NoOpLifecycleHook) OnArchive(ctx context.Context, episodeIDs []string) error {
	_ = ctx
	_ = episodeIDs
	return nil
}

func (h *NoOpLifecycleHook) OnDelete(ctx context.Context, episodeIDs []string) error {
	_ = ctx
	_ = episodeIDs
	return nil
}

func (h *NoOpLifecycleHook) OnCompress(ctx context.Context, episodeIDs []string) (int64, error) {
	_ = ctx
	_ = episodeIDs
	return 0, nil
}

// LifecycleManager 数据生命周期管理器
type LifecycleManager struct {
	mu       sync.RWMutex
	policies map[string]RetentionPolicy
	hook     LifecycleHook
}

// NewLifecycleManager 创建生命周期管理器
func NewLifecycleManager() *LifecycleManager {
	policies := make(map[string]RetentionPolicy)
	policies["default"] = DefaultRetentionPolicy()
	return &LifecycleManager{
		policies: policies,
		hook:     &NoOpLifecycleHook{},
	}
}

// SetPolicy 设置租户的保留策略
func (m *LifecycleManager) SetPolicy(tenantID string, policy RetentionPolicy) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.policies[tenantID] = policy
}

// GetPolicy 获取租户的保留策略
func (m *LifecycleManager) GetPolicy(tenantID string) (RetentionPolicy, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	policy, ok := m.policies[tenantID]
	if !ok {
		return RetentionPolicy{}, ErrPolicyNotFound
	}
	return policy, nil
}

// SetHook 设置生命周期钩子
func (m *LifecycleManager) SetHook(hook LifecycleHook) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hook = hook
}

// Enforce 执行生命周期策略
// 遍历所有策略，对过期数据执行配置的动作
func (m *LifecycleManager) Enforce(ctx context.Context) (*LifecycleReport, error) {
	m.mu.RLock()
	policies := make(map[string]RetentionPolicy, len(m.policies))
	for k, v := range m.policies {
		policies[k] = v
	}
	hook := m.hook
	m.mu.RUnlock()

	report := &LifecycleReport{
		Details:    make(map[string]int64),
		ExecutedAt: time.Now(),
	}

	for name, policy := range policies {
		// 根据各 TTL 执行对应动作
		// WorkingMemory -> 删除
		if policy.WorkingMemoryTTL > 0 {
			deleted, err := m.enforceTTL(ctx, hook, "working", policy.WorkingMemoryTTL)
			if err != nil {
				return nil, err
			}
			report.DeletedEpisodes += deleted
			report.Details[name+"_working"] = deleted
		}
		// SemanticMemory -> 归档
		if policy.SemanticMemoryTTL > 0 {
			archived, err := m.enforceTTL(ctx, hook, "semantic", policy.SemanticMemoryTTL)
			if err != nil {
				return nil, err
			}
			report.ArchivedEpisodes += archived
			report.Details[name+"_semantic"] = archived
		}
		// EpisodeMemory -> 压缩
		if policy.EpisodeMemoryTTL > 0 {
			compressed, err := m.enforceTTL(ctx, hook, "episode", policy.EpisodeMemoryTTL)
			if err != nil {
				return nil, err
			}
			report.CompressedEpisodes += compressed
			report.Details[name+"_episode"] = compressed
		}
	}

	return report, nil
}

// enforceTTL 执行单个 TTL 策略
func (m *LifecycleManager) enforceTTL(ctx context.Context, hook LifecycleHook, kind string, ttl time.Duration) (int64, error) {
	if hook == nil {
		hook = &NoOpLifecycleHook{}
	}
	switch kind {
	case "working":
		// Working memory 直接删除
		if err := hook.OnDelete(ctx, nil); err != nil {
			return 0, err
		}
		return 0, nil
	case "semantic":
		if err := hook.OnArchive(ctx, nil); err != nil {
			return 0, err
		}
		return 0, nil
	case "episode":
		compressed, err := hook.OnCompress(ctx, nil)
		if err != nil {
			return 0, err
		}
		return compressed, nil
	}
	return 0, nil
}

// ScheduleArchive 调度归档操作（根据时间阈值）
func (m *LifecycleManager) ScheduleArchive(ctx context.Context, olderThan time.Duration) error {
	if olderThan <= 0 {
		return errors.New("olderThan must be positive")
	}
	m.mu.RLock()
	hook := m.hook
	m.mu.RUnlock()
	return hook.OnArchive(ctx, nil)
}

// DeleteByUser GDPR 合规：按用户删除所有数据
func (m *LifecycleManager) DeleteByUser(ctx context.Context, userID string) error {
	if userID == "" {
		return errors.New("userID cannot be empty")
	}
	m.mu.RLock()
	hook := m.hook
	m.mu.RUnlock()
	return hook.OnDelete(ctx, []string{userID})
}

// Policies 返回所有策略快照
func (m *LifecycleManager) Policies() map[string]RetentionPolicy {
	m.mu.RLock()
	defer m.mu.RUnlock()
	snapshot := make(map[string]RetentionPolicy, len(m.policies))
	for k, v := range m.policies {
		snapshot[k] = v
	}
	return snapshot
}

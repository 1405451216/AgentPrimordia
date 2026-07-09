// leader_election.go — Leader 选举 + 心跳 + 网络分区恢复（G2-3 生产强化）
//
// 在 Coordinator 的分布式锁基础上增加：
//   - Leader 选举：通过 Acquire 实现 "先到先得" 式 Leader 选举
//   - 心跳续约：后台 goroutine 定期 Renew 租约，防止 TTL 过期
//   - 网络分区恢复：检测到分区（Renew 失败）后进入 "降级模式"，
//     停止写入并尝试重新获取锁
//   - 优雅降级：失去 Leader 身份后通知上层（通过 callback）
//   - 健康检查：Leader 定期检查自身健康状态，异常时主动让出
//
// 设计原则：
//   - 仅依赖 Coordinator 接口，不绑定具体后端（memory/fs/etcd/redis 均可）
//   - 网络分区下宁可停止写入也不冒脑裂风险（CP 优先于 AP）
package persist

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// LeaderState Leader 的运行时状态。
type LeaderState string

const (
	LeaderLeading   LeaderState = "leading"   // 当前节点是 Leader，正常工作
	LeaderFollowing LeaderState = "following" // 当前节点是 Follower，只读
	LeaderDegraded  LeaderState = "degraded"  // 网络分区或锁丢失，停止写入
	LeaderCandidate LeaderState = "candidate" // 正在竞选 Leader
)

// LeaderConfig Leader 选举配置。
type LeaderConfig struct {
	// ElectionKey 选举的锁键名（如 "leader:agent-cluster"）
	ElectionKey string
	// HeartbeatInterval 心跳间隔（默认 TTL/3）
	HeartbeatInterval time.Duration
	// TTL 租约有效期
	TTL time.Duration
	// RetryInterval 竞选失败后的重试间隔
	RetryInterval time.Duration
	// MaxRetries 最大重试次数（0 = 无限重试）
	MaxRetries int
	// HealthCheck 可选的健康检查函数（返回 false 时主动让出 Leader）
	HealthCheck func() bool
}

// DefaultLeaderConfig 默认配置。
func DefaultLeaderConfig(electionKey string) LeaderConfig {
	return LeaderConfig{
		ElectionKey:       electionKey,
		TTL:               30 * time.Second,
		HeartbeatInterval: 10 * time.Second,
		RetryInterval:     5 * time.Second,
		MaxRetries:        0, // 无限重试
	}
}

// LeaderEvent Leader 状态变化事件。
type LeaderEvent struct {
	OldState LeaderState
	NewState LeaderState
	Reason   string
	At       time.Time
}

// LeaderEventCallback Leader 状态变化回调。
type LeaderEventCallback func(event LeaderEvent)

// LeaderElector Leader 选举器。
type LeaderElector struct {
	config LeaderConfig
	coord  Coordinator
	nodeID string

	state atomic.Pointer[LeaderState]
	lease atomic.Pointer[Lease]

	eventCbMu sync.RWMutex
	eventCb   LeaderEventCallback

	stopCh     chan struct{}
	stopped    atomic.Bool
	resigned   atomic.Bool // 防止重复 resign
	renewFails atomic.Int64

	// 统计
	heartbeatsSent   atomic.Int64
	heartbeatsFailed atomic.Int64
	leaderSince      atomic.Int64 // unix nano, 0 = not leader

	// 可观测性指标（可选）
	metrics *LeaderMetrics
}

// NewLeaderElector 创建 Leader 选举器。
func NewLeaderElector(coord Coordinator, nodeID string, config LeaderConfig) *LeaderElector {
	le := &LeaderElector{
		config: config,
		coord:  coord,
		nodeID: nodeID,
		stopCh: make(chan struct{}),
	}
	initial := LeaderFollowing
	le.state.Store(&initial)
	return le
}

// OnEvent 设置状态变化回调（线程安全）。
// 通常在 Start 之前调用，但也可在运行时动态替换。
func (le *LeaderElector) OnEvent(cb LeaderEventCallback) {
	le.eventCbMu.Lock()
	defer le.eventCbMu.Unlock()
	le.eventCb = cb
}

// CurrentState 返回当前状态（无锁）。
func (le *LeaderElector) CurrentState() LeaderState {
	s := le.state.Load()
	if s == nil {
		return LeaderFollowing
	}
	return *s
}

// IsLeader 当前节点是否是 Leader。
func (le *LeaderElector) IsLeader() bool {
	return le.CurrentState() == LeaderLeading
}

// GetStats 返回运行时统计。
func (le *LeaderElector) GetStats() (heartbeatsSent, heartbeatsFailed, renewFails int64) {
	return le.heartbeatsSent.Load(), le.heartbeatsFailed.Load(), le.renewFails.Load()
}

// LeaderSince 返回成为 Leader 的时间（0 = 非 Leader）。
func (le *LeaderElector) LeaderSince() time.Time {
	nano := le.leaderSince.Load()
	if nano == 0 {
		return time.Time{}
	}
	return time.Unix(0, nano)
}

// Start 启动 Leader 选举循环。
// 阻塞直到竞选成功或 ctx 取消。
// 竞选成功后启动心跳 goroutine 保持 Leader 身份。
func (le *LeaderElector) Start(ctx context.Context) error {
	// 重置 resigned 标志（可能从上一轮恢复）
	le.resigned.Store(false)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-le.stopCh:
			return nil
		default:
		}

		// 竞选 Leader
		le.setState(LeaderCandidate, "开始竞选 Leader")

		lease, err := le.coord.Acquire(ctx, le.config.ElectionKey)
		if err != nil {
			var held *ErrLockHeld
			if errors.As(err, &held) {
				// 被他人持有，进入 Following 状态
				le.setState(LeaderFollowing, fmt.Sprintf("Leader 是 %s，进入 Follower 模式", held.Holder))
				le.waitForRetry(ctx)
				continue
			}
			// 其他错误（网络等），进入 Degraded
			le.setState(LeaderDegraded, fmt.Sprintf("竞选失败: %v", err))
			le.waitForRetry(ctx)
			continue
		}

		// 竞选成功
		le.lease.Store(&lease)
		le.leaderSince.Store(time.Now().UnixNano())
		le.setState(LeaderLeading, "成功当选 Leader")

		// 启动心跳维持
		le.startHeartbeat(ctx)

		return nil
	}
}

// StartInBackground 在后台启动 Leader 选举（非阻塞）。
// 通过 OnEvent 回调通知状态变化。
func (le *LeaderElector) StartInBackground(ctx context.Context) {
	go func() {
		for {
			err := le.Start(ctx)
			if err != nil {
				return
			}
			// 成为 Leader 后，心跳 goroutine 会持续运行
			// 如果失去 Leader，心跳 goroutine 会退出，此处循环重新竞选
			select {
			case <-ctx.Done():
				return
			case <-le.stopCh:
				return
			case <-time.After(le.config.RetryInterval):
				// 重新竞选
			}
		}
	}()
}

// startHeartbeat 启动心跳续约 goroutine。
// 当续约失败时，切换到 Degraded 状态并退出，触发重新竞选。
func (le *LeaderElector) startHeartbeat(ctx context.Context) {
	interval := le.config.HeartbeatInterval
	if interval <= 0 {
		interval = le.config.TTL / 3
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				le.resign("上下文取消")
				return
			case <-le.stopCh:
				le.resign("主动停止")
				return
			case <-ticker.C:
				le.heartbeat(ctx)
			}
		}
	}()
}

// heartbeat 执行一次心跳续约。
func (le *LeaderElector) heartbeat(ctx context.Context) {
	// 如果已停止或已 resign，跳过
	if le.stopped.Load() || le.resigned.Load() {
		return
	}

	le.heartbeatsSent.Add(1)
	if le.metrics != nil {
		le.metrics.RecordHeartbeatSent()
	}

	// 健康检查
	if le.config.HealthCheck != nil && !le.config.HealthCheck() {
		le.resign("健康检查失败，主动让出 Leader")
		return
	}

	leasePtr := le.lease.Load()
	if leasePtr == nil {
		le.setState(LeaderDegraded, "租约丢失")
		return
	}
	lease := *leasePtr

	if err := lease.Renew(ctx); err != nil {
		le.heartbeatsFailed.Add(1)
		le.renewFails.Add(1)
		if le.metrics != nil {
			le.metrics.RecordHeartbeatFailed()
			le.metrics.RecordRenewFailure()
		}

		// 连续 3 次续约失败 → 进入 Degraded
		if le.renewFails.Load() >= 3 {
			le.resign(fmt.Sprintf("续约连续失败 %d 次，进入降级模式", le.renewFails.Load()))
			return
		}
		return
	}

	// 续约成功，重置失败计数
	le.renewFails.Store(0)
}

// resign 主动让出 Leader 身份（确保只执行一次）。
func (le *LeaderElector) resign(reason string) {
	if !le.resigned.CompareAndSwap(false, true) {
		return // 已 resign，跳过
	}
	leasePtr := le.lease.Load()
	if leasePtr != nil {
		lease := *leasePtr
		le.lease.Store(nil) // 先清空，防止 heartbeat goroutine 再用
		le.coord.Release(context.Background(), lease)
	}
	le.leaderSince.Store(0)
	le.setState(LeaderDegraded, reason)
}

// Stop 停止 Leader 选举（主动让出 Leader 并停止心跳）。
func (le *LeaderElector) Stop() {
	if le.stopped.CompareAndSwap(false, true) {
		close(le.stopCh)
		le.resign("主动停止")
	}
}

// ForceTakeover 强制接管 Leader（用于 Leader 节点已知故障的场景）。
// 仅当当前 Leader 的租约已过期时成功。
func (le *LeaderElector) ForceTakeover(ctx context.Context) error {
	le.resigned.Store(false) // 重置
	lease, err := le.coord.Acquire(ctx, le.config.ElectionKey)
	if err != nil {
		return fmt.Errorf("强制接管失败: %w", err)
	}
	le.lease.Store(&lease)
	le.leaderSince.Store(time.Now().UnixNano())
	le.setState(LeaderLeading, "强制接管成功")
	le.startHeartbeat(ctx)
	return nil
}

// setState 原子更新状态并触发回调。
func (le *LeaderElector) setState(newState LeaderState, reason string) {
	oldState := le.CurrentState()
	le.state.Store(&newState)

	if le.metrics != nil {
		le.metrics.SetState(newState)
	}

	le.eventCbMu.RLock()
	cb := le.eventCb
	le.eventCbMu.RUnlock()
	if cb != nil && oldState != newState {
		cb(LeaderEvent{
			OldState: oldState,
			NewState: newState,
			Reason:   reason,
			At:       time.Now(),
		})
	}
}

// waitForRetry 等待重试间隔。
func (le *LeaderElector) waitForRetry(ctx context.Context) {
	select {
	case <-ctx.Done():
	case <-le.stopCh:
	case <-time.After(le.config.RetryInterval):
	}
}

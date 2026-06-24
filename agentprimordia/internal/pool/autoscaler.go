package pool

import (
	"sync"
	"time"
)

// AutoScalerConfig 自动扩缩容配置
type AutoScalerConfig struct {
	MinConcurrency     int           // 最小并发度
	MaxConcurrency     int           // 最大并发度
	ScaleUpThreshold   float64       // 扩容阈值（利用率超过此值时扩容）
	ScaleDownThreshold float64       // 缩容阈值（利用率低于此值时缩容）
	CoolDownPeriod     time.Duration // 冷却期（防止频繁扩缩容）
	CheckInterval      time.Duration // 检查间隔
}

// AutoScaler 自动扩缩容器
type AutoScaler struct {
	cfg           AutoScalerConfig
	mu            sync.RWMutex
	lastScaleTime time.Time // 上次扩缩容时间
	lastScaleUp   bool      // 上次是扩容还是缩容
}

// NewAutoScaler 创建自动扩缩容器
func NewAutoScaler(cfg AutoScalerConfig) *AutoScaler {
	if cfg.MinConcurrency <= 0 {
		cfg.MinConcurrency = 1
	}
	if cfg.MaxConcurrency <= 0 {
		cfg.MaxConcurrency = 100
	}
	if cfg.ScaleUpThreshold <= 0 {
		cfg.ScaleUpThreshold = 0.8
	}
	if cfg.ScaleDownThreshold <= 0 {
		cfg.ScaleDownThreshold = 0.2
	}
	if cfg.CoolDownPeriod <= 0 {
		cfg.CoolDownPeriod = 5 * time.Second
	}
	if cfg.CheckInterval <= 0 {
		cfg.CheckInterval = 10 * time.Second
	}

	return &AutoScaler{
		cfg: cfg,
	}
}

// Calculate 根据当前负载计算新的并发度
// running: 当前运行中的任务数
// queued: 当前排队中的任务数
// current: 当前并发度
// 返回: 新的并发度
func (a *AutoScaler) Calculate(running, queued, current int) int {
	a.mu.Lock()
	defer a.mu.Unlock()

	// 防止除零
	if current <= 0 {
		current = a.cfg.MinConcurrency
	}

	// 计算利用率：(运行中 + 排队) / 当前并发度
	totalDemand := running + queued
	utilization := float64(totalDemand) / float64(current)

	// 检查是否在冷却期内
	now := time.Now()
	if !a.lastScaleTime.IsZero() && now.Sub(a.lastScaleTime) < a.cfg.CoolDownPeriod {
		// 冷却期内不进行调整
		return current
	}

	newConcurrency := current

	// 扩容逻辑
	if utilization >= a.cfg.ScaleUpThreshold {
		// 计算目标并发度：增加 50% 或至少增加 1
		increase := int(float64(current) * 0.5)
		if increase < 1 {
			increase = 1
		}
		newConcurrency = current + increase

		// 限制不超过最大值
		if newConcurrency > a.cfg.MaxConcurrency {
			newConcurrency = a.cfg.MaxConcurrency
		}

		// 记录扩缩容时间和方向
		a.lastScaleTime = now
		a.lastScaleUp = true
	}

	// 缩容逻辑
	if utilization <= a.cfg.ScaleDownThreshold && current > a.cfg.MinConcurrency {
		// 计算目标并发度：减少 30% 或至少减少 1
		decrease := int(float64(current) * 0.3)
		if decrease < 1 {
			decrease = 1
		}
		newConcurrency = current - decrease

		// 限制不低于最小值
		if newConcurrency < a.cfg.MinConcurrency {
			newConcurrency = a.cfg.MinConcurrency
		}

		// 记录扩缩容时间和方向
		a.lastScaleTime = now
		a.lastScaleUp = false
	}

	return newConcurrency
}

// GetConfig 获取配置
func (a *AutoScaler) GetConfig() AutoScalerConfig {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.cfg
}

// Pool 集成 AutoScaler 的方法

// StartAutoScaler 启动自动扩缩容
func (p *Pool) StartAutoScaler() {
	if p.autoScaler == nil {
		return
	}

	if p.autoScalerRunning.Load() {
		return
	}

	p.autoScalerRunning.Store(true)

	go func() {
		ticker := time.NewTicker(p.config.AutoScaler.CheckInterval)
		defer ticker.Stop()

		for p.autoScalerRunning.Load() {
			select {
			case <-ticker.C:
				p.autoScale()
			case <-p.ctx.Done():
				return
			}
		}
	}()
}

// StopAutoScaler 停止自动扩缩容
func (p *Pool) StopAutoScaler() {
	p.autoScalerRunning.Store(false)
}

// autoScale 执行自动扩缩容
// 使用动态并发度计数器替代重建 semaphore，避免正在持有旧令牌的 goroutine 释放到新 channel
func (p *Pool) autoScale() {
	if p.autoScaler == nil {
		return
	}

	// 获取当前状态
	running := int(p.runningCount.Load())
	queued := int(p.queuedCount.Load())

	p.mu.RLock()
	currentConcurrency := p.config.MaxConcurrency
	p.mu.RUnlock()

	// 计算新的并发度
	newConcurrency := p.autoScaler.Calculate(running, queued, currentConcurrency)

	// 如果并发度变化，更新配置
	if newConcurrency != currentConcurrency {
		p.mu.Lock()
		p.config.MaxConcurrency = newConcurrency
		// 更新动态并发度限制（不替换 semaphore，避免令牌错乱）
		p.dynamicConcurrency.Store(int64(newConcurrency))
		// 更新统计信息（在锁内写入，避免数据竞争）
		p.stats.MaxConcurrency = newConcurrency
		p.mu.Unlock()

		// 发出事件
		p.emitEvent(PoolEvent{
			Type: "autoscale",
			Data: map[string]interface{}{
				"old_concurrency": currentConcurrency,
				"new_concurrency": newConcurrency,
				"running":         running,
				"queued":          queued,
			},
		})
	}
}

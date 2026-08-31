// router.go — 蒸馏域三段路由（影子 → 灰度 → 全量；回滚门常驻）
//
// 治理裁决（路线图 §四）：新路由档「蒸馏域」为一等公民，灰度+回滚内建；
// **默认不参与任何既有路由决策**——本路由器只服务窄域内显式流量，llm
// ModelRouter 既有决策面零变更（铁律 7）。
//
// 状态机：
//
//	shadow →（影子报告 Passed）→ canary →（灰度观察成功 ≥ 最小样本且无回滚触发）→ full
//	任一阶段：连续失败 ≥ 阈值 → 回滚到上一阶段（full→canary→shadow→禁用），
//	回滚事件入审计链。
package pipeline

import (
	"fmt"
	"sync"
	"time"
)

// RouterConfig 三段路由配置。
type RouterConfig struct {
	// CanaryPct 灰度承接百分比 [1,100]（进 canary 阶段生效）
	CanaryPct int
	// RollbackThreshold 连续失败回滚阈值（默认 3；确定性不变式——
	// 超额即回滚，代码强制，允许 0 容忍的 R3 确定性门）
	RollbackThreshold int
	// CanariMinCalls 灰度转全量的最小观察样本（默认 20）
	CanaryMinCalls int
}

// DistillationRouter 蒸馏域路由器（并发安全）。
type DistillationRouter struct {
	mu       sync.Mutex
	domain   string
	manifest string // 当前生效蒸馏数据集
	cfg      RouterConfig
	state    RouterState
	audit    *AuditChain
	canaryOK int // 灰度成功计数（晋升输入）
	disabled bool
}

// NewDistillationRouter 构造路由器（初始 shadow 阶段，禁用态）。
func NewDistillationRouter(domain, manifestID string, cfg RouterConfig, audit *AuditChain) *DistillationRouter {
	if cfg.CanaryPct <= 0 {
		cfg.CanaryPct = 20
	}
	if cfg.CanaryPct > 100 {
		cfg.CanaryPct = 100
	}
	if cfg.RollbackThreshold <= 0 {
		cfg.RollbackThreshold = 3
	}
	if cfg.CanaryMinCalls <= 0 {
		cfg.CanaryMinCalls = 20
	}
	return &DistillationRouter{
		domain:   domain,
		manifest: manifestID,
		cfg:      cfg,
		state: RouterState{
			Stage:      StageShadow,
			ManifestID: manifestID,
			CanaryPct:  cfg.CanaryPct,
		},
		audit: audit,
	}
}

// State 状态快照。
func (r *DistillationRouter) State() RouterState {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state
}

// Disabled 是否已被回滚禁用（回滚到 shadow 之下 = 蒸馏域下线）。
func (r *DistillationRouter) Disabled() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.disabled
}

// PromoteOnShadowReport 影子阶段：依据报告判据晋升（Passed 才升 canary）。
func (r *DistillationRouter) PromoteOnShadowReport(rep *ShadowReport) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.disabled {
		return fmt.Errorf("pipeline: 蒸馏域 %s 已禁用（回滚超限），先人工复审", r.domain)
	}
	if !rep.Passed {
		if r.state.Stage == StageCanary {
			// 灰度期复评不达标 = 蒸馏模型退化，回滚门介入（回滚到影子）
			r.rollbackLocked()
			return nil
		}
		if r.audit != nil {
			r.audit.Append("shadow", fmt.Sprintf("蒸馏域 %s 影子判据未过（ratio=%.3f 下界=%.3f），保持影子阶段", r.domain, rep.Ratio, rep.RatioLower))
		}
		return nil // 未达标不晋升、不禁用——继续影子积累
	}
	if r.state.Stage == StageShadow {
		r.state.Stage = StageCanary
		if r.audit != nil {
			r.audit.Append("promote", fmt.Sprintf("蒸馏域 %s 影子判据通过（ratio=%.3f 下界=%.3f），晋升灰度 %d%%", r.domain, rep.Ratio, rep.RatioLower, r.state.CanaryPct))
		}
	}
	// canary 阶段复评通过 = 保持（全量晋升由灰度成功计数驱动）
	return nil
}

// ShouldRoute 请求路由判定：窄域内流量按阶段决定是否由蒸馏模型承接。
// 返回 false = 交还旗舰（默认路径，铁律 7 的运行时体现）。
func (r *DistillationRouter) ShouldRoute(domain string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.disabled || domain != r.domain {
		return false
	}
	switch r.state.Stage {
	case StageShadow:
		return false // 影子阶段不接真实流量
	case StageCanary:
		// 确定性灰度：请求数取模百分比桶（无随机数，可复现）
		return r.canaryBucketOK()
	case StageFull:
		return true
	}
	return false
}

// canaryBucketOK 灰度桶判定（确定性递增计数取模——同配置同序列）。
func (r *DistillationRouter) canaryBucketOK() bool {
	// 以 canaryOK+ConsecFails 合成请求序号，避免引入随机源
	seq := r.canaryOK + r.state.ConsecFails + r.state.Rollbacks*7919
	return seq%100 < r.cfg.CanaryPct
}

// RecordOutcome 记录一次蒸馏模型承接结果（回滚门输入）。
func (r *DistillationRouter) RecordOutcome(success bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.disabled {
		return
	}
	if success {
		r.state.ConsecFails = 0
		if r.state.Stage == StageCanary {
			r.canaryOK++
			if r.canaryOK >= r.cfg.CanaryMinCalls {
				r.state.Stage = StageFull
				if r.audit != nil {
					r.audit.Append("promote", fmt.Sprintf("蒸馏域 %s 灰度 %d 次全成功，晋升全量", r.domain, r.canaryOK))
				}
			}
		}
		return
	}
	r.state.ConsecFails++
	if r.state.ConsecFails >= r.cfg.RollbackThreshold {
		r.rollbackLocked()
	}
}

// rollbackLocked 回滚（须持锁）：full→canary→shadow→禁用。
func (r *DistillationRouter) rollbackLocked() {
	r.state.Rollbacks++
	r.state.ConsecFails = 0
	r.canaryOK = 0
	switch r.state.Stage {
	case StageFull:
		r.state.Stage = StageCanary
	case StageCanary:
		r.state.Stage = StageShadow
	case StageShadow:
		r.disabled = true
	}
	if r.audit != nil {
		r.audit.Append("rollback", fmt.Sprintf("蒸馏域 %s 连续失败达阈值，回滚至 %s（第 %d 次）", r.domain, r.state.Stage, r.state.Rollbacks))
	}
}

// ForceRollback 显式回滚（运维动作，同样入审计）。
func (r *DistillationRouter) ForceRollback() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rollbackLocked()
}

// Now 便捷时间（测试确定性注入点保留）。
var Now = time.Now

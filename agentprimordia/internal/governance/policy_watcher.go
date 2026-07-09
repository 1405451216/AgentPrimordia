// policy_watcher.go — 策略热加载（G3-3 治理强化）
//
// 监控策略文件变化，实现策略热加载：
// - 轮询检测文件修改时间（每 5s），避免引入 fsnotify 依赖
// - 文件变更后解析新策略，验证有效性
// - 原子切换：通过 atomic.Pointer 保证策略切换的无锁读取
// - 版本管理：每次热加载记录版本号与变更历史
// - 回滚机制：新策略验证失败时回滚到上一个有效策略
package governance

import (
	"encoding/json"
	"fmt"
	"os"
	"sync/atomic"
	"time"
)

// PolicyVersion 策略版本信息。
type PolicyVersion struct {
	Version   int       `json:"version"`
	LoadedAt  time.Time `json:"loadedAt"`
	Source    string    `json:"source"`
	Checksum  string    `json:"checksum"`
	IsValid   bool      `json:"isValid"`
	LastError string    `json:"lastError,omitempty"`
}

// PolicyHistory 策略变更历史。
type PolicyHistory struct {
	Versions []PolicyVersion `json:"versions"`
}

// WatchablePolicy 可热加载的策略容器（无锁读取）。
type WatchablePolicy struct {
	current  atomic.Pointer[Policy]
	version  atomic.Int64
	history  atomic.Pointer[PolicyHistory]
	auditLog AuditLogger
}

// NewWatchablePolicy 创建可热加载的策略容器。
func NewWatchablePolicy(initial *Policy, auditLog AuditLogger) *WatchablePolicy {
	w := &WatchablePolicy{auditLog: auditLog}
	w.current.Store(initial)
	w.version.Store(1)
	w.history.Store(&PolicyHistory{
		Versions: []PolicyVersion{{
			Version:  1,
			LoadedAt: time.Now(),
			Source:   "initial",
			IsValid:  true,
		}},
	})
	return w
}

// Current 返回当前策略（无锁）。
func (w *WatchablePolicy) Current() *Policy {
	return w.current.Load()
}

// CurrentVersion 返回当前版本号。
func (w *WatchablePolicy) CurrentVersion() int64 {
	return w.version.Load()
}

// GetHistory 返回策略变更历史。
func (w *WatchablePolicy) GetHistory() *PolicyHistory {
	return w.history.Load()
}

// Swap 原子切换策略。验证失败则保持旧策略。
func (w *WatchablePolicy) Swap(newPolicy *Policy, source string) error {
	if err := ValidatePolicy(newPolicy); err != nil {
		if w.auditLog != nil {
			w.auditLog.Log(AuditEvent{
				Type:     AuditPolicyViolation,
				AgentID:  "system",
				Reason:   fmt.Sprintf("策略验证失败: %v", err),
				Severity: "critical",
				Detail:   json.RawMessage(fmt.Sprintf(`{"source":%q,"error":%q}`, source, err.Error())),
			})
		}
		w.appendHistory(PolicyVersion{
			Version:   int(w.version.Load() + 1),
			LoadedAt:  time.Now(),
			Source:    source,
			IsValid:   false,
			LastError: err.Error(),
		})
		return fmt.Errorf("策略验证失败，保持旧策略: %w", err)
	}

	oldVersion := w.version.Load()
	w.current.Store(newPolicy)
	newVersion := w.version.Add(1)
	checksum := policyChecksum(newPolicy)

	w.appendHistory(PolicyVersion{
		Version:  int(newVersion),
		LoadedAt: time.Now(),
		Source:   source,
		Checksum: checksum,
		IsValid:  true,
	})

	if w.auditLog != nil {
		w.auditLog.Log(AuditEvent{
			Type:     AuditPolicyHotSwapped,
			AgentID:  "system",
			Reason:   fmt.Sprintf("策略热加载: v%d → v%d, source=%s", oldVersion, newVersion, source),
			Severity: "info",
			Detail:   json.RawMessage(fmt.Sprintf(`{"oldVersion":%d,"newVersion":%d,"source":%q,"checksum":%q}`, oldVersion, newVersion, source, checksum)),
		})
	}
	return nil
}

// Rollback 回滚到上一个有效策略。
func (w *WatchablePolicy) Rollback(previous *Policy, source string) error {
	if previous == nil {
		return fmt.Errorf("无可回滚的策略")
	}
	oldVersion := w.version.Load()
	w.current.Store(previous)
	newVersion := w.version.Add(1)

	w.appendHistory(PolicyVersion{
		Version:  int(newVersion),
		LoadedAt: time.Now(),
		Source:   fmt.Sprintf("rollback from v%d, %s", oldVersion, source),
		IsValid:  true,
	})

	if w.auditLog != nil {
		w.auditLog.Log(AuditEvent{
			Type:     AuditPolicyHotSwapped,
			AgentID:  "system",
			Reason:   fmt.Sprintf("策略回滚: v%d → v%d", oldVersion, newVersion),
			Severity: "warning",
		})
	}
	return nil
}

func (w *WatchablePolicy) appendHistory(v PolicyVersion) {
	for {
		old := w.history.Load()
		newVersions := append([]PolicyVersion{}, old.Versions...)
		newVersions = append(newVersions, v)
		if len(newVersions) > 100 {
			newVersions = newVersions[len(newVersions)-100:]
		}
		newHistory := &PolicyHistory{Versions: newVersions}
		if w.history.CompareAndSwap(old, newHistory) {
			break
		}
	}
}

// PolicyWatcher 策略文件热加载器。
type PolicyWatcher struct {
	policy   *WatchablePolicy
	filePath string
	auditLog AuditLogger
	stopCh   chan struct{}
	stopped  atomic.Bool
}

// NewPolicyWatcher 创建策略热加载器。
func NewPolicyWatcher(filePath string, initial *Policy, auditLog AuditLogger) *PolicyWatcher {
	return &PolicyWatcher{
		policy:   NewWatchablePolicy(initial, auditLog),
		filePath: filePath,
		auditLog: auditLog,
		stopCh:   make(chan struct{}),
	}
}

// Start 开始轮询监听策略文件变化。
func (w *PolicyWatcher) Start(pollInterval time.Duration) {
	if pollInterval <= 0 {
		pollInterval = 5 * time.Second
	}

	var lastMod time.Time
	if info, err := os.Stat(w.filePath); err == nil {
		lastMod = info.ModTime()
	}

	go func() {
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-w.stopCh:
				return
			case <-ticker.C:
				info, err := os.Stat(w.filePath)
				if err != nil {
					continue
				}
				if info.ModTime().After(lastMod) {
					lastMod = info.ModTime()
					w.reload()
				}
			}
		}
	}()
}

func (w *PolicyWatcher) reload() {
	newPolicy, err := LoadPolicyFile(w.filePath)
	if err != nil {
		if w.auditLog != nil {
			w.auditLog.Log(AuditEvent{
				Type:     AuditPolicyViolation,
				AgentID:  "system",
				Reason:   fmt.Sprintf("策略热加载失败: %v", err),
				Severity: "critical",
			})
		}
		return
	}

	if err := w.policy.Swap(newPolicy, w.filePath); err != nil {
		return
	}

	if w.auditLog != nil {
		w.auditLog.Log(AuditEvent{
			Type:     AuditPolicyLoaded,
			AgentID:  "system",
			Reason:   fmt.Sprintf("策略热加载成功: v%d", w.policy.CurrentVersion()),
			Severity: "info",
		})
	}
}

// Stop 停止监听。
func (w *PolicyWatcher) Stop() {
	if w.stopped.CompareAndSwap(false, true) {
		close(w.stopCh)
	}
}

// Current 返回当前策略。
func (w *PolicyWatcher) Current() *Policy {
	return w.policy.Current()
}

// CurrentVersion 返回当前版本号。
func (w *PolicyWatcher) CurrentVersion() int64 {
	return w.policy.CurrentVersion()
}

// GetHistory 返回变更历史。
func (w *PolicyWatcher) GetHistory() *PolicyHistory {
	return w.policy.GetHistory()
}

// GetPolicy 返回可热加载策略容器。
func (w *PolicyWatcher) GetPolicy() *WatchablePolicy {
	return w.policy
}

// ValidatePolicy 验证策略有效性。
func ValidatePolicy(p *Policy) error {
	if p == nil {
		return fmt.Errorf("策略为空")
	}
	if p.Spec.CostLimits.PerRequest < 0 {
		return fmt.Errorf("perRequest 成本限制不能为负")
	}
	if p.Spec.CostLimits.PerDay < 0 {
		return fmt.Errorf("perDay 成本限制不能为负")
	}
	if p.Spec.OutputGuardrail.MaxLength < 0 {
		return fmt.Errorf("maxLength 不能为负")
	}
	if p.Spec.BehaviorConstraints.MaxTurns < 0 {
		return fmt.Errorf("maxTurns 不能为负")
	}
	for _, tr := range p.Spec.ToolRestrictions {
		if tr.MaxCallsPerRun < 0 {
			return fmt.Errorf("工具 %s 的 maxCallsPerRun 不能为负", tr.Tool)
		}
	}
	return nil
}

// policyChecksum FNV-1a 哈希校验和。
func policyChecksum(p *Policy) string {
	if p == nil {
		return ""
	}
	data, err := json.Marshal(p)
	if err != nil {
		return ""
	}
	var h uint64 = 14695981039346656037
	for _, b := range data {
		h ^= uint64(b)
		h *= 1099511628211
	}
	return fmt.Sprintf("%016x", h)
}

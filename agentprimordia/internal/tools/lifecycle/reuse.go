// reuse.go — 复用追踪与劣化退役（第六段；命题 2 测量口径）
//
// 命题 2（7 天复用率点估计 ≥60%、Wilson 下界 ≥50%，分母=注册工具数）：
// FleetReuseReport 提供该口径的确定性测量；单工具退役策略为确定性规则
// （长期零调用 / 失败率超限），退役动作入生命周期审计。
package lifecycle

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"time"
)

// ToolUsage 单次工具使用观测（注册后由执行器投喂）。
type ToolUsage struct {
	ToolID  string
	Success bool
	At      time.Time
}

// ReuseTracker 复用追踪（并发安全）。
type ReuseTracker struct {
	mu     sync.Mutex
	usages []ToolUsage
}

// NewReuseTracker 构造。
func NewReuseTracker() *ReuseTracker {
	return &ReuseTracker{}
}

// Record 投喂一次使用观测。
func (t *ReuseTracker) Record(u ToolUsage) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if u.At.IsZero() {
		u.At = time.Now().UTC()
	}
	t.usages = append(t.usages, u)
}

// FleetReuseReport 舰队级复用报表（命题 2 口径：分母=注册工具数）。
type FleetReuseReport struct {
	WindowDays    int       `json:"window_days"`
	Registered    int       `json:"registered"`      // 分母：注册工具数
	Reused        int       `json:"reused"`          // 窗口内被调用过的注册工具数
	ReuseRate     float64   `json:"reuse_rate"`      // 点估计
	ReuseWilsonLo float64   `json:"reuse_wilson_lo"` // Wilson 95% 下界（R3 口径）
	TotalCalls    int       `json:"total_calls"`
	FailedCalls   int       `json:"failed_calls"`
	Generated     time.Time `json:"generated"`
}

// FleetReport 舰队报表。registeredIDs 为当前注册工具全集（分母）。
func (t *ReuseTracker) FleetReport(registeredIDs []string, windowDays int, now time.Time) FleetReuseReport {
	t.mu.Lock()
	defer t.mu.Unlock()
	if windowDays <= 0 {
		windowDays = 7
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	cutoff := now.AddDate(0, 0, -windowDays)
	rep := FleetReuseReport{
		WindowDays: windowDays,
		Registered: len(registeredIDs),
		Generated:  now.UTC(),
	}
	registered := make(map[string]bool, len(registeredIDs))
	for _, id := range registeredIDs {
		registered[id] = true
	}
	reused := make(map[string]bool)
	for _, u := range t.usages {
		if u.At.Before(cutoff) || !registered[u.ToolID] {
			continue
		}
		rep.TotalCalls++
		if !u.Success {
			rep.FailedCalls++
		}
		reused[u.ToolID] = true
	}
	rep.Reused = len(reused)
	if rep.Registered > 0 {
		rep.ReuseRate = float64(rep.Reused) / float64(rep.Registered)
	}
	lo, _ := wilsonInterval(rep.Reused, rep.Registered)
	rep.ReuseWilsonLo = lo
	return rep
}

// RetireReason 单工具退役判定结论。
type RetireReason struct {
	Should bool   `json:"should"`
	Policy string `json:"policy,omitempty"` // retirement 理由（入审计）
	Detail string `json:"detail"`
}

// EvaluateRetirement 单工具退役策略（确定性）：
//   - 窗口内零调用 →「冷工具」（注册后长期无人用 = 未成资产）；
//   - 窗口内调用 ≥ minCalls 且失败率 > 50% →「劣化工具」；
//
// 其余保留。
func (t *ReuseTracker) EvaluateRetirement(toolID string, windowDays, minCalls int, now time.Time) RetireReason {
	t.mu.Lock()
	defer t.mu.Unlock()
	if windowDays <= 0 {
		windowDays = 7
	}
	if minCalls <= 0 {
		minCalls = 5
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	cutoff := now.AddDate(0, 0, -windowDays)
	var total, fails int
	for _, u := range t.usages {
		if u.ToolID != toolID || u.At.Before(cutoff) {
			continue
		}
		total++
		if !u.Success {
			fails++
		}
	}
	if total == 0 {
		return RetireReason{Should: true, Policy: "cold_tool", Detail: fmt.Sprintf("窗口 %d 天内零调用", windowDays)}
	}
	if total >= minCalls && float64(fails)/float64(total) > 0.5 {
		return RetireReason{Should: true, Policy: "degraded", Detail: fmt.Sprintf("窗口内 %d 调用 %d 失败（失败率 %.0f%%）", total, fails, 100*float64(fails)/float64(total))}
	}
	return RetireReason{Should: false, Detail: fmt.Sprintf("窗口内 %d 调用 %d 失败，保留", total, fails)}
}

// SweepRetirements 舰队扫描：对每个注册工具跑退役策略，应退役者调用
// manager.Retire（审计入链）。返回实际退役的 toolID（升序）。
func (t *ReuseTracker) SweepRetirements(m *Manager, registeredIDs []string, windowDays, minCalls int, now time.Time) []string {
	ids := append([]string(nil), registeredIDs...)
	sort.Strings(ids)
	var retired []string
	for _, id := range ids {
		r := t.EvaluateRetirement(id, windowDays, minCalls, now)
		if r.Should {
			if err := m.Retire(id, r.Policy+"（"+r.Detail+"）"); err == nil {
				retired = append(retired, id)
			}
		}
	}
	return retired
}

// ===== 统计助手（与 S0-1/pipeline 同算法口径；分层自含）=====

// wilsonInterval Wilson 95% 成功率区间（z=1.959963984540054）。
func wilsonInterval(k, n int) (lo, hi float64) {
	if n <= 0 {
		return 0, 0
	}
	z := 1.959963984540054
	p := float64(k) / float64(n)
	denom := 1 + z*z/float64(n)
	center := (p + z*z/(2*float64(n))) / denom
	rad := z / denom * math.Sqrt(p*(1-p)/float64(n)+z*z/(4*float64(n)*float64(n)))
	return center - rad, center + rad
}

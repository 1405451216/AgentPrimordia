// gap.go — 缺口审计报表（第一段入口的证据来源）
//
// 两类缺口信号（确定性聚合，同输入必同报表）：
//  1. missing_tool：对不存在工具的调用（DynamicRegistry 查找失败的原始
//     记录——能力真空的直接证据）；
//  2. repeated_failure：既有工具在窗口内反复失败（能力不足信号）。
//
// 报表按（计数降序, 能力键升序）稳定排序；样本错误去重后最多保留 3 条。
package lifecycle

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// 缺口信号类别。
const (
	GapKindMissingTool     = "missing_tool"
	GapKindRepeatedFailure = "repeated_failure"
)

// GapCall 一次工具调用观测（由执行器/回溯差异通道投喂）。
type GapCall struct {
	ToolName string    // 工具名（missing 时不要求已注册）
	OK       bool      // 是否成功
	ErrText  string    // 失败摘要（成功时忽略）
	At       time.Time // 观测时间
}

// Gap 单条缺口。
type Gap struct {
	Kind         string    `json:"kind"`                    // missing_tool / repeated_failure
	Key          string    `json:"key"`                     // 能力键（工具名）
	Count        int       `json:"count"`                   // 窗口内观测数
	SampleErrors []string  `json:"sample_errors,omitempty"` // 去重样本（≤3）
	FirstSeen    time.Time `json:"first_seen"`
	LastSeen     time.Time `json:"last_seen"`
}

// GapReport 缺口审计报表（确定性）。
type GapReport struct {
	Window    time.Duration `json:"window"`
	Total     int           `json:"total"` // 观测调用总数
	Gaps      []Gap         `json:"gaps"`
	Generated time.Time     `json:"generated"`
}

// GapAuditor 缺口审计器（并发安全）。
type GapAuditor struct {
	mu            sync.Mutex
	calls         []GapCall
	window        time.Duration // 聚合窗口（默认 24h）
	failThreshold int           // repeated_failure 阈值（默认 3）
}

// NewGapAuditor 构造（window≤0 取 24h；failThreshold≤0 取 3）。
func NewGapAuditor(window time.Duration, failThreshold int) *GapAuditor {
	if window <= 0 {
		window = 24 * time.Hour
	}
	if failThreshold <= 0 {
		failThreshold = 3
	}
	return &GapAuditor{window: window, failThreshold: failThreshold}
}

// Record 投喂一次调用观测（missing 判定由调用方给出：OK=false 且
// ErrText 含 "tool not found" 前缀按 missing_tool 聚类；执行器约定见
// internal/tools executor 的 ErrToolNotFound 文案口径）。
func (a *GapAuditor) Record(c GapCall) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if c.At.IsZero() {
		c.At = time.Now().UTC()
	}
	a.calls = append(a.calls, c)
}

// Report 生成窗口内缺口报表（确定性）。
func (a *GapAuditor) Report(now time.Time) GapReport {
	a.mu.Lock()
	defer a.mu.Unlock()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	cutoff := now.Add(-a.window)
	rep := GapReport{Window: a.window, Generated: now.UTC()}

	type agg struct {
		count   int
		samples []string
		seenErr map[string]bool
		first   time.Time
		last    time.Time
	}
	missing := make(map[string]*agg)
	failing := make(map[string]*agg)

	for _, c := range a.calls {
		if c.At.Before(cutoff) {
			continue
		}
		rep.Total++
		if isMissingToolCall(c) {
			g := missing[c.ToolName]
			if g == nil {
				g = &agg{seenErr: make(map[string]bool)}
				missing[c.ToolName] = g
			}
			g.count++
			g.last = maxTime(g.last, c.At)
			if g.first.IsZero() {
				g.first = c.At
			}
			if c.ErrText != "" && !g.seenErr[c.ErrText] && len(g.samples) < 3 {
				g.seenErr[c.ErrText] = true
				g.samples = append(g.samples, c.ErrText)
			}
			continue
		}
		if c.OK {
			continue
		}
		g := failing[c.ToolName]
		if g == nil {
			g = &agg{seenErr: make(map[string]bool)}
			failing[c.ToolName] = g
		}
		g.count++
		g.last = maxTime(g.last, c.At)
		if g.first.IsZero() {
			g.first = c.At
		}
		if c.ErrText != "" && !g.seenErr[c.ErrText] && len(g.samples) < 3 {
			g.seenErr[c.ErrText] = true
			g.samples = append(g.samples, c.ErrText)
		}
	}

	for name, g := range missing {
		rep.Gaps = append(rep.Gaps, Gap{
			Kind: GapKindMissingTool, Key: name, Count: g.count,
			SampleErrors: g.samples, FirstSeen: g.first, LastSeen: g.last,
		})
	}
	for name, g := range failing {
		if g.count < a.failThreshold {
			continue // 未达阈值不构成缺口信号
		}
		rep.Gaps = append(rep.Gaps, Gap{
			Kind: GapKindRepeatedFailure, Key: name, Count: g.count,
			SampleErrors: g.samples, FirstSeen: g.first, LastSeen: g.last,
		})
	}
	sort.Slice(rep.Gaps, func(i, j int) bool {
		if rep.Gaps[i].Count != rep.Gaps[j].Count {
			return rep.Gaps[i].Count > rep.Gaps[j].Count
		}
		return rep.Gaps[i].Key < rep.Gaps[j].Key
	})
	return rep
}

// isMissingToolCall 缺工具判定（执行器错误文案口径；防误伤普通失败）。
func isMissingToolCall(c GapCall) bool {
	if c.OK {
		return false
	}
	t := strings.ToLower(c.ErrText)
	return strings.Contains(t, "tool not found") ||
		strings.Contains(t, "未注册") ||
		strings.Contains(t, "unknown tool")
}

// EnrollFromReport 把报表中计数 ≥ minCount 的缺口登记为生命周期候选
// （第一段入口；命名规则 deterministic：gap-<kind>-<key>）。
func (m *Manager) EnrollFromReport(rep GapReport, minCount int) []string {
	if minCount <= 0 {
		minCount = 1
	}
	var enrolled []string
	for _, g := range rep.Gaps {
		if g.Count < minCount {
			continue
		}
		id := fmt.Sprintf("gap-%s-%s", g.Kind, g.Key)
		err := m.Enroll(Candidate{
			ID:          id,
			Name:        g.Key,
			Domain:      g.Kind,
			Description: fmt.Sprintf("窗口内 %s 观测 %d 次", g.Kind, g.Count),
		})
		if err == nil {
			enrolled = append(enrolled, id)
		}
	}
	return enrolled
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

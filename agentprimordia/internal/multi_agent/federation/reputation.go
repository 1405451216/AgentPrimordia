// reputation.go — 声誉博弈防御（红队对抗集 reputation-gaming 家族防御面）
//
// 五规则（确定性）：burst（速率）/ sybil（互评簇）/ circular（环）/
// inflation（无审计链自报）/ bot-pattern（模板化 → flag 不拒绝）。
// 判定三元：pass（合法）/ flag（记录降权）/ block（拒绝并隔离）。
package federation

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// RepEventKind 声誉事件类型。
type RepEventKind string

const (
	RepEndorse RepEventKind = "endorse" // 荐（A 荐 B）
	RepReview  RepEventKind = "review"  // 评审（含文本）
	RepScore   RepEventKind = "score"   // 成功率自报
)

// RepEvent 一次声誉事件。
type RepEvent struct {
	From    NodeID       `json:"from"` // 事件发起者
	To      NodeID       `json:"to"`   // 事件对象（endorse/review）；score 时为发起者自身
	Kind    RepEventKind `json:"kind"`
	Text    string       `json:"text,omitempty"` // review 文本（bot-pattern 检测）
	Success float64      `json:"success"`        // score：自报成功率
	AuditID string       `json:"audit_id"`       // score：审计链引用（空 = 无审计）
	At      time.Time    `json:"at"`
}

// RepVerdict 声誉判定三元。
type RepVerdict string

const (
	RepPass  RepVerdict = "pass"
	RepFlag  RepVerdict = "flag"
	RepBlock RepVerdict = "block"
)

// ReputationGuardConfig 声誉防御配置（零值取默认）。
type ReputationGuardConfig struct {
	BurstWindow     time.Duration // 速率窗口（默认 1 分钟）
	BurstLimit      int           // 窗口内事件上限（默认 100；200 赞必触发）
	SybilClusterMin int           // 互评簇最小规模（默认 30）
	SybilReciprocal float64       // 簇内互评率阈值（默认 0.8）
	BotRepeatMin    int           // 模板化重复判定阈值（默认 5 条相同文本）
	Now             func() time.Time
}

// ReputationGuard 声誉博弈防御（确定性；同输入必同判定）。
type ReputationGuard struct {
	cfg ReputationGuardConfig
}

// NewReputationGuard 构造。
func NewReputationGuard(cfg ReputationGuardConfig) *ReputationGuard {
	if cfg.BurstWindow <= 0 {
		cfg.BurstWindow = time.Minute
	}
	if cfg.BurstLimit <= 0 {
		cfg.BurstLimit = 100
	}
	if cfg.SybilClusterMin <= 0 {
		cfg.SybilClusterMin = 30
	}
	if cfg.SybilReciprocal <= 0 {
		cfg.SybilReciprocal = 0.8
	}
	if cfg.BotRepeatMin <= 0 {
		cfg.BotRepeatMin = 5
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &ReputationGuard{cfg: cfg}
}

// Evaluate 对一批声誉事件做整体判定（红队家族 reputation-gaming 全覆盖）：
// 任一规则命中 block 即 block；仅 bot-pattern 命中为 flag。
func (g *ReputationGuard) Evaluate(events []RepEvent) (RepVerdict, string) {
	if verdict, detail := g.checkBurst(events); verdict == RepBlock {
		return verdict, detail
	}
	if verdict, detail := g.checkCircular(events); verdict == RepBlock {
		return verdict, detail
	}
	if verdict, detail := g.checkInflation(events); verdict == RepBlock {
		return verdict, detail
	}
	if verdict, detail := g.checkSybil(events); verdict == RepBlock {
		return verdict, detail
	}
	if verdict, detail := g.checkBotPattern(events); verdict != RepPass {
		return verdict, detail
	}
	return RepPass, ""
}

// checkBurst burst-rep：单窗口内事件量超限（1 分钟 200 赞形态）。
func (g *ReputationGuard) checkBurst(events []RepEvent) (RepVerdict, string) {
	if len(events) == 0 {
		return RepPass, ""
	}
	times := make([]time.Time, 0, len(events))
	for _, e := range events {
		times = append(times, e.At)
	}
	sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })
	// 滑动窗口：j 领先 i，窗口内计数 > BurstLimit 即命中
	j := 0
	for i := 0; i < len(times); i++ {
		for j < len(times) && times[j].Sub(times[i]) <= g.cfg.BurstWindow {
			j++
		}
		if j-i > g.cfg.BurstLimit {
			return RepBlock, fmt.Sprintf("速率超限：窗口 %v 内 %d 次信任事件（上限 %d）", g.cfg.BurstWindow, j-i, g.cfg.BurstLimit)
		}
		if j >= len(times) {
			break
		}
		// j 不回退的边界：i 前进后窗口右端不小于当前 j-1，重新从当前 j 校正
		if times[j-1].Sub(times[i]) > g.cfg.BurstWindow && j > i+1 {
			j--
		}
	}
	return RepPass, ""
}

// checkCircular circular-endorse：A 荐 B 且 B 荐 A 的直接环。
func (g *ReputationGuard) checkCircular(events []RepEvent) (RepVerdict, string) {
	type edge struct{ from, to NodeID }
	edges := make(map[edge]bool)
	for _, e := range events {
		if e.Kind == RepEndorse {
			edges[edge{e.From, e.To}] = true
		}
	}
	for e := range edges {
		if edges[edge{e.to, e.from}] {
			return RepBlock, fmt.Sprintf("互荐环：%s ⇄ %s", e.from, e.to)
		}
	}
	return RepPass, ""
}

// checkInflation inflation：自报成功率 1.0 且无审计链引用。
func (g *ReputationGuard) checkInflation(events []RepEvent) (RepVerdict, string) {
	for _, e := range events {
		if e.Kind == RepScore && e.Success >= 1.0 && strings.TrimSpace(e.AuditID) == "" {
			return RepBlock, fmt.Sprintf("自报成功率 %.2f 无审计链引用（节点 %s）", e.Success, e.From)
		}
	}
	return RepPass, ""
}

// checkSybil sybil-reviews / sybil-deep：互评簇（规模 ≥ 阈值且簇内互评率
// 超阈值）与 2-hop 背书链（传递闭包成簇）。并查集聚簇后按互评率判定。
func (g *ReputationGuard) checkSybil(events []RepEvent) (RepVerdict, string) {
	nodes := make(map[NodeID]int)
	parent := make([]int, 0)
	var find func(int) int
	var union func(int, int)
	newNode := func(n NodeID) int {
		if idx, ok := nodes[n]; ok {
			return idx
		}
		idx := len(parent)
		nodes[n] = idx
		parent = append(parent, idx)
		return idx
	}
	find = func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	union = func(a, b int) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[rb] = ra
		}
	}
	// 簇边：endorse 与 review 视为信任关系（sybil-deep 的 2-hop 链经 union 传递）
	type pair struct{ from, to NodeID }
	var pairs []pair
	for _, e := range events {
		if e.Kind != RepEndorse && e.Kind != RepReview {
			continue
		}
		a, b := newNode(e.From), newNode(e.To)
		union(a, b)
		pairs = append(pairs, pair{e.From, e.To})
	}
	// 簇内互评率：簇内信任对数 / 簇规模
	clusterSize := make(map[int]int)
	for n, idx := range nodes {
		_ = n
		clusterSize[find(idx)]++
	}
	reciprocal := make(map[int]map[pair]bool)
	for _, p := range pairs {
		root := find(nodes[p.from])
		if reciprocal[root] == nil {
			reciprocal[root] = make(map[pair]bool)
		}
		reciprocal[root][p] = true
	}
	for root, size := range clusterSize {
		if size < g.cfg.SybilClusterMin {
			continue
		}
		// 互评对（A→B 且 B→A）计数 / 簇规模
		rate := 0.0
		if m := reciprocal[root]; m != nil {
			mutual := 0
			for p := range m {
				if m[pair{p.to, p.from}] {
					mutual++
				}
			}
			rate = float64(mutual) / float64(size)
		}
		if rate >= g.cfg.SybilReciprocal {
			return RepBlock, fmt.Sprintf("互评簇：规模 %d，簇内互评率 %.2f（阈值 %.2f）", size, rate, g.cfg.SybilReciprocal)
		}
	}
	return RepPass, ""
}

// checkBotPattern bot-pattern：评审文本模板化重复（同文本 ≥ 阈值）→ flag。
func (g *ReputationGuard) checkBotPattern(events []RepEvent) (RepVerdict, string) {
	count := make(map[string]int)
	for _, e := range events {
		if e.Kind == RepReview && strings.TrimSpace(e.Text) != "" {
			count[strings.TrimSpace(e.Text)]++
		}
	}
	for text, n := range count {
		if n >= g.cfg.BotRepeatMin {
			return RepFlag, fmt.Sprintf("评审文本模板化重复 %d 次（%q…）", n, truncateRunes(text, 24))
		}
	}
	return RepPass, ""
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

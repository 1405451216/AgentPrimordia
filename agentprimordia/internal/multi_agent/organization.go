// organization.go — v5.5 组织智能：共享记忆黑板 + 涌现分工 + 组织级调度。
//
// 从「多 Agent 协作」升级为「Agent 组织」：
//   - Blackboard 共享记忆黑板：任务轨迹（指令/认领/结果/观察）全组织可见
//   - OrgRouter 涌现分工：按历史成功率路由（数据驱动，非硬编码角色）
//   - Organization 组织级调度：任务入板→认领租约→执行→结果回板→分工学习
package multi_agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ===== 共享记忆黑板 =====

// EntryKind 黑板条目类型
type EntryKind string

const (
	KindDirective   EntryKind = "directive"   // 任务指令
	KindClaim       EntryKind = "claim"       // 认领
	KindResult      EntryKind = "result"      // 结果
	KindObservation EntryKind = "observation" // 观察/中间产物
)

// Entry 黑板条目
type Entry struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"task_id"`
	Author    string    `json:"author"`
	Kind      EntryKind `json:"kind"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// Blackboard 共享记忆黑板（并发安全）
type Blackboard struct {
	mu     sync.RWMutex
	seq    int64
	entries []Entry
	claims  map[string]string // taskID → 持有人
}

// NewBlackboard 创建黑板
func NewBlackboard() *Blackboard {
	return &Blackboard{claims: make(map[string]string)}
}

// Post 发布条目（时间序追加）
func (b *Blackboard) Post(e Entry) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.seq++
	if e.ID == "" {
		e.ID = fmt.Sprintf("e-%d", b.seq)
	}
	e.CreatedAt = time.Now()
	b.entries = append(b.entries, e)
}

// Read 按 TaskID 过滤读取（空串返回全部），时间序
func (b *Blackboard) Read(taskID string) []Entry {
	b.mu.RLock()
	defer b.mu.RUnlock()
	var out []Entry
	for _, e := range b.entries {
		if taskID == "" || e.TaskID == taskID {
			out = append(out, e)
		}
	}
	return out
}

// Claim 认领任务（租约语义：已认领则失败）
func (b *Blackboard) Claim(taskID, agentName string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, held := b.claims[taskID]; held {
		return false
	}
	b.claims[taskID] = agentName
	return true
}

// ClaimHolder 返回任务持有人（空串=无）
func (b *Blackboard) ClaimHolder(taskID string) string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.claims[taskID]
}

// Release 释放认领（成员故障时调度器强制回收）
func (b *Blackboard) Release(taskID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.claims, taskID)
}

// ===== 涌现分工路由 =====

// OrgRouter 数据驱动的分工路由：按 (agent, domain) 历史成功率选择执行者
type OrgRouter struct {
	mu    sync.RWMutex
	stats map[string]map[string]*orgStat // domain → agent → stat
}

type orgStat struct{ succ, fail int }

// NewOrgRouter 创建路由器
func NewOrgRouter() *OrgRouter {
	return &OrgRouter{stats: make(map[string]map[string]*orgStat)}
}

// Record 记录一次执行结果（分工学习的信号源）
func (r *OrgRouter) Record(agentName, domain string, success bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	dm := r.stats[domain]
	if dm == nil {
		dm = make(map[string]*orgStat)
		r.stats[domain] = dm
	}
	st := dm[agentName]
	if st == nil {
		st = &orgStat{}
		dm[agentName] = st
	}
	if success {
		st.succ++
	} else {
		st.fail++
	}
}

// Route 为 domain 选择最优候选（多臂老虎机式探索-利用）：
//   - 有成功历史者按成功率利用；
//   - 无历史候选得探索分 0.5（高于 0% 成员，保证冷启动探索）；
//   - 全员无历史时按候选顺序确定性回退。
func (r *OrgRouter) Route(domain string, candidates []string) string {
	if len(candidates) == 0 {
		return ""
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	best, bestScore := "", -1.0
	for i, a := range candidates {
		var score float64
		if st := r.stats[domain][a]; st != nil && st.succ+st.fail > 0 {
			score = float64(st.succ) / float64(st.succ+st.fail)
		} else {
			score = 0.5 + 1e-9*float64(len(candidates)-i) // 探索优先，序位打破平局
		}
		if score > bestScore {
			best, bestScore = a, score
		}
	}
	return best
}

// ===== 组织级调度 =====

// Member 组织成员最小接口（core.Agent 经 MemberAdapter 接入）
type Member interface {
	Name() string
	Execute(ctx context.Context, task string) (string, error)
}

// MemberAdapter 将 core.Agent（Run(Message)）适配为 Member
type MemberAdapter struct {
	AgentName string
	RunFn     func(ctx context.Context, task string) (string, error)
}

// Name 实现 Member
func (m *MemberAdapter) Name() string { return m.AgentName }

// Execute 实现 Member
func (m *MemberAdapter) Execute(ctx context.Context, task string) (string, error) {
	return m.RunFn(ctx, task)
}

// TaskReport 单任务执行报告
type TaskReport struct {
	TaskID string `json:"task_id"`
	Domain string `json:"domain"`
	Member string `json:"member"`
	Output string `json:"output"`
	Err    string `json:"err,omitempty"`
}

// OrgReport 组织执行报告
type OrgReport struct {
	Completed int          `json:"completed"`
	Failed    int          `json:"failed"`
	Results   []TaskReport `json:"results"`
}

// Organization Agent 组织：黑板 + 涌现分工 + 调度
type Organization struct {
	Board  *Blackboard
	Router *OrgRouter
	mu     sync.RWMutex
	roster []Member
	// DomainOf 任务→能力域提取器（默认取首个 '|' 前缀或整句）
	DomainOf func(task string) string
}

// NewOrganization 创建组织
func NewOrganization() *Organization {
	return &Organization{
		Board:  NewBlackboard(),
		Router: NewOrgRouter(),
		DomainOf: func(task string) string {
			if i := strings.Index(task, "|"); i > 0 {
				return strings.TrimSpace(task[:i])
			}
			return "general"
		},
	}
}

// Register 注册成员
func (o *Organization) Register(m Member) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.roster = append(o.roster, m)
}

// memberNames 候选名单快照
func (o *Organization) memberNames() []string {
	o.mu.RLock()
	defer o.mu.RUnlock()
	names := make([]string, len(o.roster))
	for i, m := range o.roster {
		names[i] = m.Name()
	}
	return names
}

// memberByName 取成员
func (o *Organization) memberByName(name string) Member {
	o.mu.RLock()
	defer o.mu.RUnlock()
	for _, m := range o.roster {
		if m.Name() == name {
			return m
		}
	}
	return nil
}

// Execute 组织级调度：逐任务 入板→涌现分工→认领→执行→结果回板→学习
func (o *Organization) Execute(ctx context.Context, tasks []string) *OrgReport {
	report := &OrgReport{Results: make([]TaskReport, 0, len(tasks))}
	for i, task := range tasks {
		taskID := fmt.Sprintf("task-%d", i)
		domain := o.DomainOf(task)
		o.Board.Post(Entry{TaskID: taskID, Author: "scheduler", Kind: KindDirective, Content: task})

		// 涌现分工：按历史成功率选人
		chosen := o.Router.Route(domain, o.memberNames())
		if !o.Board.Claim(taskID, chosen) { // 租约竞争（多组织共享黑板场景）
			o.Board.Release(taskID)
			chosen = o.Router.Route(domain, o.memberNames())
			_ = o.Board.Claim(taskID, chosen)
		}
		o.Board.Post(Entry{TaskID: taskID, Author: chosen, Kind: KindClaim, Content: task})

		member := o.memberByName(chosen)
		rep := TaskReport{TaskID: taskID, Domain: domain, Member: chosen}
		if member == nil {
			rep.Err = "组织内无成员"
		} else {
			out, err := member.Execute(ctx, task)
			rep.Output, rep.Err = out, errString(err)
		}
		kind := KindResult
		if rep.Err != "" {
			o.Router.Record(chosen, domain, false)
			o.Board.Release(taskID) // 失败释放租约供重试
		} else {
			o.Router.Record(chosen, domain, true)
			report.Completed++
		}
		o.Board.Post(Entry{TaskID: taskID, Author: chosen, Kind: kind,
			Content: rep.Output + errSuffix(rep.Err)})
		report.Results = append(report.Results, rep)
		if rep.Err != "" {
			report.Failed++
		}
	}
	return report
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func errSuffix(e string) string {
	if e == "" {
		return ""
	}
	return " [错误: " + e + "]"
}

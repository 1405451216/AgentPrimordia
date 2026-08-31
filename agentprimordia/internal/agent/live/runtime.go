// runtime.go — 常驻运行时主循环 + 闲时调度器 + 监护自愈守护
//
// 主循环（单 goroutine 事件驱动，无内部并发竞态面）：
//
//	for {
//	    select 唤醒事件：
//	      → 预算检查（确定性不变式：耗尽即拒绝，超额 0）
//	      → Guardian 守护执行（panic 恢复 = 崩溃自愈，计数入审计）
//	      → outcome 记账 + 任务完成入审计链
//	    idle 环（无唤醒时由 IdleStep 驱动）：闲时自调度学习/工具制造/模型整理
//	}
//
// 闲时自调度（命题 3 的执行面）：IdleJob 注册表按优先级串行执行，
// 产出交由调用方（学习管道/生命周期管理器实例），运行时只管调度与审计。
package live

import (
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"

	"crypto/sha256"
	"encoding/hex"
)

// IdleJob 闲时自调度作业（v6.2 学习管道 / v6.3 生命周期管理 / 模型整理）。
// 代谢语义：循环复发，Interval 为冷却间隔（0 = 每步可跑）。
type IdleJob struct {
	Name     string        `json:"name"`     // 作业名（唯一）
	Priority int           `json:"priority"` // 数值越小越先
	Interval time.Duration `json:"interval"` // 冷却间隔（上次成功运行后多久内不再跑）
	Run      func(now time.Time) (summary string, err error)
}

// auditEntry 审计链节点（链式哈希；与 pipeline/lifecycle 同模式）。
type auditEntry struct {
	Seq      int       `json:"seq"`
	Stage    string    `json:"stage"`
	Detail   string    `json:"detail"`
	PrevHash string    `json:"prev_hash"`
	Hash     string    `json:"hash"`
	At       time.Time `json:"at"`
}

const auditGenesis = "genesis"

// RuntimeStats 运行时可观测快照。
type RuntimeStats struct {
	TasksDone       int64    `json:"tasks_done"`
	TasksSucceeded  int64    `json:"tasks_succeeded"`
	CrashesHealed   int64    `json:"crashes_healed"` // 自愈成功次数（命题 1 统计口径）
	CrashFailures   int64    `json:"crash_failures"` // 自愈后仍失败（如实披露）
	IdleRuns        int64    `json:"idle_runs"`
	BudgetTasks     int64    `json:"budget_tasks"`
	BudgetTokens    int64    `json:"budget_tokens"`
	BudgetExhausted bool     `json:"budget_exhausted"`
	UptimeDays      float64  `json:"uptime_days"`
	IdleJobs        []string `json:"idle_jobs"`
	AuditCount      int      `json:"audit_count"`
}

// Runtime 常驻运行时（显式启动，ap live 形态；不触碰任何既有会话语义）。
type Runtime struct {
	mu       sync.Mutex
	runner   Runner
	waker    *Waker
	clock    Clock
	budget   *Budget
	started  time.Time
	seq      int64
	idleJobs []IdleJob
	audit    []auditEntry

	// 心跳（监护面：外部监护器按此判定存活）
	lastHeartbeat time.Time
	heartbeats    int64

	idleLastRun map[string]time.Time

	// 计数器（Stats 直读，不解析审计文本）
	tasksDone      int64
	tasksSucceeded int64
	crashesHealed  int64
	crashFailures  int64
	idleRuns       int64
}

// NewRuntime 构造常驻运行时。
func NewRuntime(runner Runner, waker *Waker, clock Clock, budget *Budget) *Runtime {
	if budget == nil {
		budget = &Budget{}
	}
	return &Runtime{
		runner:      runner,
		waker:       waker,
		clock:       clock,
		budget:      budget,
		started:     clock.Now(),
		idleLastRun: make(map[string]time.Time),
	}
}

// RegisterIdleJob 注册闲时作业（同名额覆盖）。
func (r *Runtime) RegisterIdleJob(j IdleJob) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.idleJobs {
		if r.idleJobs[i].Name == j.Name {
			r.idleJobs[i] = j
			return
		}
	}
	r.idleJobs = append(r.idleJobs, j)
	sort.Slice(r.idleJobs, func(a, b int) bool { return r.idleJobs[a].Priority < r.idleJobs[b].Priority })
}

// auditLocked 追加审计（须持锁）。
func (r *Runtime) auditLocked(stage, detail string) {
	prev := auditGenesis
	if len(r.audit) > 0 {
		prev = r.audit[len(r.audit)-1].Hash
	}
	at := r.clock.Now().UTC()
	e := auditEntry{Seq: len(r.audit) + 1, Stage: stage, Detail: detail, PrevHash: prev, At: at}
	h := sha256.Sum256([]byte(strconv.Itoa(e.Seq) + "|" + at.Format(time.RFC3339Nano) + "|" + stage + "|" + detail + "|" + prev))
	e.Hash = hex.EncodeToString(h[:])
	r.audit = append(r.audit, e)
}

// VerifyAudit 全链校验。
func (r *Runtime) VerifyAudit() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	prev := auditGenesis
	for i, e := range r.audit {
		if e.PrevHash != prev || e.Seq != i+1 {
			return fmt.Errorf("live: 审计链第 %d 节点断裂", i+1)
		}
		h := sha256.Sum256([]byte(strconv.Itoa(e.Seq) + "|" + e.At.Format(time.RFC3339Nano) + "|" + e.Stage + "|" + e.Detail + "|" + e.PrevHash))
		if hex.EncodeToString(h[:]) != e.Hash {
			return fmt.Errorf("live: 审计链第 %d 节点哈希不复算", i+1)
		}
		prev = e.Hash
	}
	return nil
}

// HandleWake 处理一次唤醒（主循环步进；确定性可逐步驱动——长活 harness
// 无需真实时间即可模拟任意时长的常驻生命）。
// 返回任务 outcome（nil = 未执行：预算耗尽/空唤醒）。
func (r *Runtime) HandleWake(ev WakeEvent) *TaskOutcome {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.clock.Now()
	r.heartbeats++
	r.lastHeartbeat = now

	// 预算护栏（确定性不变式：耗尽即拒绝，超额 0——不执行、不记账）
	if r.budget.Exhausted() {
		r.auditLocked("budget_block", fmt.Sprintf("唤醒 %s 被预算护栏拒绝（超额 0 不变式）", ev.Source))
		return nil
	}

	// 任务规格（确定性 ID：序号派生）
	r.seq++
	task := TaskSpec{ID: fmt.Sprintf("run-%06d", r.seq), Input: ev.Payload, Wake: ev}

	// Guardian 守护执行：panic → 自愈（转为 Crashed outcome，运行时存活）
	outcome := r.guardedRun(task, now)
	r.tasksDone++

	if outcome.Success {
		r.tasksSucceeded++
		r.budget.Record(int64(outcome.Tokens))
		r.auditLocked("task", fmt.Sprintf("%s 成功（唤醒 %s，tokens %d）", task.ID, ev.Source, outcome.Tokens))
	} else if outcome.Crashed {
		// 崩溃自愈：不消耗预算记账（失败任务 tokens 记 0——保守成本口径）
		r.budget.Record(0)
		r.crashesHealed++
		r.auditLocked("self_heal", fmt.Sprintf("%s 崩溃已恢复：%s", task.ID, outcome.ErrText))
	} else {
		r.crashFailures++ // 普通失败（非崩溃）计入如实披露口径
		r.budget.Record(int64(outcome.Tokens))
		r.auditLocked("task", fmt.Sprintf("%s 失败：%s", task.ID, outcome.ErrText))
	}
	return outcome
}

// guardedRun 守护执行（panic 恢复 = 崩溃自愈核心；必须持 r.mu 调用）。
func (r *Runtime) guardedRun(task TaskSpec, now time.Time) (outcome *TaskOutcome) {
	outcome = &TaskOutcome{TaskID: task.ID, At: now, Wake: task.Wake}
	defer func() {
		if rec := recover(); rec != nil {
			// 崩溃自愈：捕异常、留痕、运行时继续存活
			outcome.Crashed = true
			outcome.Success = false
			outcome.ErrText = fmt.Sprintf("panic recovered: %v", rec)
			outcome.Tokens = 0
		}
	}()
	output, tokens, err := r.runner.Run(task)
	outcome.Output = output
	outcome.Tokens = tokens
	if err != nil {
		outcome.Success = false
		outcome.ErrText = err.Error()
	} else {
		outcome.Success = true
	}
	return outcome
}

// IdleStep 闲时自调度步（主循环在无唤醒时调用；每次执行至多一个作业——
// 保持事件响应性）。返回执行摘要（无作业/预算耗尽返回 nil）。
func (r *Runtime) IdleStep() *string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.budget.TokensExhausted() {
		r.auditLocked("idle", "token 预算耗尽，闲时代谢暂停")
		return nil
	}
	now := r.clock.Now()
	for _, j := range r.idleJobs {
		// 冷却判定：Interval 内已成功运行过 → 跳过（失败不进入冷却——下步重试）
		if j.Interval > 0 {
			if last, ok := r.idleLastRun[j.Name]; ok && now.Sub(last) < j.Interval {
				continue
			}
		}
		summary, err := j.Run(now)
		r.idleRuns++
		if err != nil {
			r.auditLocked("idle", fmt.Sprintf("作业 %s 失败：%v", j.Name, err))
			continue
		}
		r.idleLastRun[j.Name] = now
		r.auditLocked("idle", fmt.Sprintf("作业 %s：%s", j.Name, summary))
		s := j.Name + ": " + summary
		return &s
	}
	return nil
}

// Heartbeat 监护心跳（外部守护进程据此判定存活；纯读）。
func (r *Runtime) Heartbeat() (time.Time, int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastHeartbeat, r.heartbeats
}

// Stats 可观测快照。
func (r *Runtime) Stats() RuntimeStats {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := RuntimeStats{
		TasksDone:       r.tasksDone,
		TasksSucceeded:  r.tasksSucceeded,
		CrashesHealed:   r.crashesHealed,
		CrashFailures:   r.crashFailures,
		IdleRuns:        r.idleRuns,
		BudgetTasks:     r.budget.tasksDone,
		BudgetTokens:    r.budget.tokensSpent,
		BudgetExhausted: r.budget.Exhausted(),
		UptimeDays:      r.clock.Now().Sub(r.started).Hours() / 24,
		AuditCount:      len(r.audit),
	}
	for _, j := range r.idleJobs {
		s.IdleJobs = append(s.IdleJobs, j.Name)
	}
	return s
}

// AuditEntries 审计链拷贝。
func (r *Runtime) AuditEntries() []auditEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]auditEntry, len(r.audit))
	copy(out, r.audit)
	return out
}

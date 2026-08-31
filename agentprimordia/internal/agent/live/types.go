// types.go — v6.4「长活」常驻运行时核心类型（V7 路线图 §六）
//
// 存在形态跃迁：Run(ctx, task) 返回即死亡 → 事件驱动常驻生命（自唤醒/
// 闲时自调度/崩溃自愈/对已负责世界的监护）。
// 治理裁决：新部署形态（ap live 显式启动），不改任何既有会话语义——
// 铁律 7 天然无冲突（路线图 §六 治理节）。
package live

import "time"

// WakeSource 自唤醒来源（路线图：定时/环境/webhook/文件监视）。
type WakeSource string

const (
	WakeTimer    WakeSource = "timer"     // 定时唤醒
	WakeFile     WakeSource = "file"      // 文件监视唤醒（被监视路径变化）
	WakeWebhook  WakeSource = "webhook"   // webhook 唤醒（外部注入通道）
	WakeManual   WakeSource = "manual"    // 人工/程序显式注入（测试与运维）
	WakeIdleTick WakeSource = "idle_tick" // 闲时钟（idle 环驱动信号，非任务唤醒）
)

// WakeEvent 一次唤醒。
type WakeEvent struct {
	Source  WakeSource `json:"source"`
	Detail  string     `json:"detail"`  // 来源细节（路径/规则名/负载摘要）
	Payload string     `json:"payload"` // 任务输入（唤醒即任务时携带）
	At      time.Time  `json:"at"`
}

// TaskSpec 常驻实例承接的任务规格。
type TaskSpec struct {
	ID    string    `json:"id"`    // 任务 ID（run-<n> 确定性派生）
	Input string    `json:"input"` // 任务输入
	Wake  WakeEvent `json:"wake"`  // 触发唤醒
}

// TaskOutcome 任务结果（监护与审计的输入）。
type TaskOutcome struct {
	TaskID  string    `json:"task_id"`
	Wake    WakeEvent `json:"wake"` // 触发唤醒（追溯链）
	Success bool      `json:"success"`
	Output  string    `json:"output"`
	ErrText string    `json:"err_text,omitempty"`
	Crashed bool      `json:"crashed"` // panic 恢复标记（自愈统计口径）
	Tokens  int       `json:"tokens"`
	At      time.Time `json:"at"`
}

// Budget 资源预算（护栏合规为确定性不变式：超额 0——路线图 §六命题 1，
// 代码强制，允许 R3 的 0 容忍）。
type Budget struct {
	MaxTasks    int   `json:"max_tasks"`  // 生命周期内最大任务数（0 = 不限）
	MaxTokens   int64 `json:"max_tokens"` // 生命周期内最大 token 消耗（0 = 不限）
	tasksDone   int64
	tokensSpent int64
}

// Exhausted 预算是否耗尽（确定性判定：任一已设上限被触及即真）。
func (b *Budget) Exhausted() bool {
	if b.MaxTasks > 0 && b.tasksDone >= int64(b.MaxTasks) {
		return true
	}
	if b.MaxTokens > 0 && b.tokensSpent >= b.MaxTokens {
		return true
	}
	return false
}

// TokensExhausted token 预算是否耗尽（闲时代谢作业以此为闸——任务数
// 上限管用户任务，不拦运行时自身的闲时学习/整理代谢）。
func (b *Budget) TokensExhausted() bool {
	return b.MaxTokens > 0 && b.tokensSpent >= b.MaxTokens
}

// Record 记账（超额防御：调用方必须先判 Exhausted；此处兜底钳制不越限）。
func (b *Budget) Record(tokens int64) {
	b.tasksDone++
	b.tokensSpent += tokens
	if b.MaxTokens > 0 && b.tokensSpent > b.MaxTokens {
		b.tokensSpent = b.MaxTokens // 钳制：账面不越上限（超额 0 不变式）
	}
}

// Snapshot 预算快照（可观测）。
func (b *Budget) Snapshot() (tasksDone, tokensSpent int64, exhausted bool) {
	return b.tasksDone, b.tokensSpent, b.Exhausted()
}

// Runner 常驻实例承接任务的执行面（Agent 接口的窄投影——live 不 import
// agent 包，依赖方向与 worldmodel 同纪律）。
type Runner interface {
	// Run 执行一次任务；panic 由 Guardian 捕获转为 Crashed outcome。
	Run(task TaskSpec) (output string, tokens int, err error)
}

// RunnerFunc 函数适配器。
type RunnerFunc func(task TaskSpec) (output string, tokens int, err error)

// Run 实现 Runner。
func (f RunnerFunc) Run(task TaskSpec) (string, int, error) { return f(task) }

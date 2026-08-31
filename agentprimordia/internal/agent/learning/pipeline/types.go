// types.go — v6.2「内化」蒸馏管道核心类型（V7 路线图 §四）
//
// 定位：轨迹→资产→权重→部署全自动管道的数据契约。
// 全部结构带 json tag 且确定性序列化——导出格式（ap-dataset-v1）与影子
// 评测报告即双线契约（矩阵 #2：TS 以工件消费者身份对等，见
// sdk/typescript/src/learning/pipeline.ts）。
package pipeline

import "time"

// TrajectoryTurn 轨迹单轮（ReAct 一轮的确定性重放所需最小字段）。
type TrajectoryTurn struct {
	Role        string   `json:"role"`                  // user / assistant / tool / system
	Content     string   `json:"content"`               // 消息内容
	ToolName    string   `json:"tool_name,omitempty"`   // assistant 轮：工具名
	ToolArgs    string   `json:"tool_args,omitempty"`   // assistant 轮：JSON 参数
	ToolCalls   []string `json:"tool_calls,omitempty"`  // assistant 轮：多工具调用摘要
	Observation string   `json:"observation,omitempty"` // tool 轮：观察结果
}

// Trajectory 一条完整任务轨迹（蒸馏数据的最小资产单位）。
type Trajectory struct {
	ID        string           `json:"id"`                   // 轨迹 ID（全局唯一）
	AgentID   string           `json:"agent_id"`             // 产出 agent
	Domain    string           `json:"domain"`               // 窄域标签（工具选择/编码修复/路由决策…，域定义注册于 S0-2）
	Success   bool             `json:"success"`              // 任务结果
	Turns     []TrajectoryTurn `json:"turns"`                // 有序轮次
	Tokens    int              `json:"tokens"`               // 全轨迹 token 消耗（成本账）
	FailureID string           `json:"failure_id,omitempty"` // 关联失败库记录（复用 failure 富矿）
	AuditID   string           `json:"audit_id,omitempty"`   // 关联审计事件 ID
	CreatedAt time.Time        `json:"created_at"`
}

// DistillationExample 蒸馏数据集单条样例（ap-dataset-v1 聊天格式）。
type DistillationExample struct {
	ID       string           `json:"id"`       // 样例 ID = 轨迹 ID（可追溯）
	Domain   string           `json:"domain"`   // 窄域标签
	Messages []DatasetMessage `json:"messages"` // 聊天格式（system/user/assistant/tool）
	Weight   float64          `json:"weight"`   // 课程权重 [0,1]（curator 打分）
}

// DatasetMessage 数据集消息（OpenAI chat 格式对齐，训练端点零转换直读）。
type DatasetMessage struct {
	Role       string `json:"role"`
	Content    string `json:"content"`
	ToolCalls  string `json:"tool_calls,omitempty"`   // assistant 工具调用（JSON 数组文本）
	ToolCallID string `json:"tool_call_id,omitempty"` // tool 消息关联 ID
	Name       string `json:"name,omitempty"`         // tool 消息工具名
}

// DatasetManifest 数据集清单（格式契约：manifest.sha256 可复算验证）。
type DatasetManifest struct {
	FormatVersion string    `json:"format_version"` // 固定 "ap-dataset-v1"
	ManifestID    string    `json:"manifest_id"`    // 内容哈希（sha256 前 16 位十六进制）
	Domain        string    `json:"domain"`         // 窄域（单域数据集——v6.2 命题 1 单域达标制）
	Count         int       `json:"count"`          // 样例数
	SHA256        string    `json:"sha256"`         // JSONL 全文字节哈希（十六进制）
	Bytes         int       `json:"bytes"`          // JSONL 字节数
	CreatedAt     time.Time `json:"created_at"`
	Source        string    `json:"source"` // 采集来源标识（collector 实例 ID）
}

// Dataset 导出产物：JSONL 字节 + 清单（命题 2「权重标准格式落盘」载体）。
type Dataset struct {
	Manifest DatasetManifest `json:"manifest"`
	JSONL    []byte          `json:"-"`
}

// ShadowCase 影子评测单题（确定性判分）。
type ShadowCase struct {
	ID       string `json:"id"`       // 题面 ID（S0-2 注册口径引用）
	Input    string `json:"input"`    // 任务输入
	Expected string `json:"expected"` // 期望输出（external 机检口径）
	Domain   string `json:"domain"`   // 窄域
}

// ShadowReport 影子评测报告（R3 口径：点估计 + Wilson 95% 下界，不裸报）。
type ShadowReport struct {
	ManifestID      string    `json:"manifest_id"` // 被评蒸馏数据集
	ChampionModel   string    `json:"champion_model"`
	ShadowModel     string    `json:"shadow_model"`
	N               int       `json:"n"` // 配对题数
	ChampionSuccess int       `json:"champion_success"`
	ShadowSuccess   int       `json:"shadow_success"`
	ChampionRate    float64   `json:"champion_rate"`
	ShadowRate      float64   `json:"shadow_rate"`
	ShadowWilsonLo  float64   `json:"shadow_wilson_lo"` // 影子成功率 Wilson 95% 下界
	Ratio           float64   `json:"ratio"`            // 影子/旗舰（命题 1 判据 ×0.85）
	RatioLower      float64   `json:"ratio_lower"`      // 比值下界（判据 CI 下界 0.80×）
	McNemarP        float64   `json:"mcnemar_p"`        // 配对显著性（b=仅 champ 对, c=仅 shadow 对）
	Passed          bool      `json:"passed"`           // 命题 1 判据：Ratio ≥0.85 且 RatioLower ≥0.80
	AuditID         string    `json:"audit_id"`
	CreatedAt       time.Time `json:"created_at"`
}

// AuditEntry 管道审计链节点（命题 3「audit 链完整」：每段一个节点，链式哈希）。
type AuditEntry struct {
	Seq      int       `json:"seq"`       // 链序号（从 1 递增）
	Stage    string    `json:"stage"`     // collect/curate/export/train/shadow/promote/rollback
	Detail   string    `json:"detail"`    // 阶段摘要（含产物 ID）
	PrevHash string    `json:"prev_hash"` // 前节点哈希（首节点 "genesis"）
	Hash     string    `json:"hash"`      // sha256(Stage|Detail|PrevHash|Seq|At)
	At       time.Time `json:"at"`
}

// RouteStage 三段路由阶段（影子 → 灰度 → 全量；治理：默认不参与任何既有路由决策）。
type RouteStage string

const (
	// StageShadow 影子阶段：shadow 模型只并行旁听，不接任何真实流量。
	StageShadow RouteStage = "shadow"
	// StageCanary 灰度阶段：按灰度百分比承接窄域内流量。
	StageCanary RouteStage = "canary"
	// StageFull 全量阶段：窄域内全量承接。
	StageFull RouteStage = "full"
)

// RouterState 路由器状态快照（回滚门常驻的可观测载体）。
type RouterState struct {
	Stage       RouteStage `json:"stage"`
	ManifestID  string     `json:"manifest_id"`
	CanaryPct   int        `json:"canary_pct"`   // 灰度百分比 [0,100]
	ConsecFails int        `json:"consec_fails"` // 连续失败计数（回滚门输入）
	Rollbacks   int        `json:"rollbacks"`    // 历史回滚次数
}

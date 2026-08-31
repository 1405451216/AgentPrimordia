// collector.go — 轨迹采集（管道第一段：复用 blackboard/audit/failure 富矿）
//
// 设计：Collector 只做「轨迹入口 + 去重 + 审计」，不感知 ReAct 内部——
// 接线由 agent 层把交互轨迹包成 Trajectory 投喂（与 v6.1 worldmodel_hook
// 同模式：内核不改默认路径）。去重键 = 全轨迹规范化 JSON 的 sha256，
// 同一轨迹重复采集（重放/多源）幂等。
package pipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// Collector 轨迹采集器（并发安全）。
type Collector struct {
	mu           sync.Mutex
	agentID      string
	source       string
	seen         map[string]bool // 内容哈希 → 已采集
	trajectories []Trajectory    // 采集序保留（确定性管道输入）
	audit        *AuditChain
}

// NewCollector 构造采集器。source 为来源标识（进 manifest.Source，可追溯）。
func NewCollector(agentID, source string, audit *AuditChain) *Collector {
	return &Collector{
		agentID: agentID,
		source:  source,
		seen:    make(map[string]bool),
		audit:   audit,
	}
}

// Ingest 投喂一条轨迹；重复内容幂等跳过。返回是否新采（false = 重复）。
func (c *Collector) Ingest(t Trajectory) (bool, error) {
	if t.ID == "" {
		return false, fmt.Errorf("pipeline: 轨迹 ID 不能为空")
	}
	if len(t.Turns) == 0 {
		return false, fmt.Errorf("pipeline: 轨迹 %s 无轮次", t.ID)
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now().UTC()
	}
	key, err := trajectoryHash(t)
	if err != nil {
		return false, fmt.Errorf("pipeline: 轨迹 %s 序列化失败: %w", t.ID, err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.seen[key] {
		return false, nil
	}
	c.seen[key] = true
	c.trajectories = append(c.trajectories, t)
	if c.audit != nil {
		c.audit.Append("collect", fmt.Sprintf("轨迹 %s（域 %s，%d 轮，成功=%v）", t.ID, t.Domain, len(t.Turns), t.Success))
	}
	return true, nil
}

// Trajectories 返回已采轨迹拷贝（按采集序；确定性管道输入）。
func (c *Collector) Trajectories() []Trajectory {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Trajectory, len(c.trajectories))
	copy(out, c.trajectories)
	return out
}

// Count 已采条数（去重后）。
func (c *Collector) Count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.trajectories)
}

// trajectoryHash 全轨迹规范化哈希：turns 之外字段不参与（同轨迹多次上报
// 时间戳/计数漂移不影响去重），turns 按序规范化后哈希。
func trajectoryHash(t Trajectory) (string, error) {
	type normTurn struct {
		Role, Content, ToolName, ToolArgs, Observation string
	}
	turns := make([]normTurn, 0, len(t.Turns))
	for _, tt := range t.Turns {
		turns = append(turns, normTurn{tt.Role, tt.Content, tt.ToolName, tt.ToolArgs, tt.Observation})
	}
	// 结构体字段序固定 → json.Marshal 确定性；再排一层防御
	b, err := json.Marshal(turns)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// lifecycle.go — 工具六段生命周期状态机（v6.3「开源」工程地板，路线图 §五）
//
// 六段（路线图：缺口检测→生成→预演→对抗→签名注册→复用/退役）：
//
//	gap_detected → generated → rehearsed → adversarial_tested
//	            → signed_registered → retired
//
// 治理边界（重要）：
//   - 本状态机是**框架与门禁**，不实现自主生成——generated 段的工件由
//     调用方供给（外部开发/受控释放通道后接自主生成，后者以
//     docs/提案-code层沙箱受控释放.md 的维护者批准为前置）；
//   - 每次推进必须携带通过性证据（Evidence.Pass=true），无跳段；
//   - 全部转换入审计链（链式哈希 append-only）。
package lifecycle

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"sync"
	"time"
)

// Stage 生命周期阶段（封闭有序集合，禁止跳段）。
type Stage string

const (
	StageGap         Stage = "gap_detected"       // 缺口检出（缺口审计报表产出）
	StageGenerated   Stage = "generated"          // 工件生成（WASM 模块/描述就绪）
	StageRehearsed   Stage = "rehearsed"          // 彩排通过（沙箱用例 + 世界模型预演证据）
	StageAdversarial Stage = "adversarial_tested" // 对抗测试通过
	StageRegistered  Stage = "signed_registered"  // 签名验证通过并注册
	StageRetired     Stage = "retired"            // 劣化退役（复用/失败率驱动）
)

// stageOrder 阶段全序（推进合法性判定基准）。
var stageOrder = []Stage{StageGap, StageGenerated, StageRehearsed, StageAdversarial, StageRegistered, StageRetired}

// stageRank 阶段序号（未定义阶段 = -1）。
func stageRank(s Stage) int {
	for i, v := range stageOrder {
		if v == s {
			return i
		}
	}
	return -1
}

// Evidence 推进证据（确定性记录，可审计）。
type Evidence struct {
	Probe  string    `json:"probe"`  // 证据来源（rehearsal/adversarial/cosign/registry/retire_policy）
	Detail string    `json:"detail"` // 摘要（含产物哈希/用例数等）
	Pass   bool      `json:"pass"`   // 通过性
	At     time.Time `json:"at"`     // 证据产生时间（零值取当前 UTC）
}

// Candidate 生命周期管理中的工具候选（签名注册段之前是工件，之后是资产）。
type Candidate struct {
	ID           string    `json:"id"`             // 候选 ID（全局唯一）
	Name         string    `json:"name"`           // 工具名（注册名）
	Domain       string    `json:"domain"`         // 窄域（缺口报表聚类口径）
	Description  string    `json:"description"`    // 能力描述
	ArtifactSHA  string    `json:"artifact_sha"`   // 工件 sha256（十六进制；空 = 未生成）
	Artifact     []byte    `json:"-"`              // 工件字节（WASM 模块；不序列化——信任链验签对象）
	SignerKeyPEM string    `json:"signer_key_pem"` // 签名方公钥（注册段锚定）
	Stage        Stage     `json:"stage"`
	RegisteredAt time.Time `json:"registered_at,omitempty"`
	RetiredAt    time.Time `json:"retired_at,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// auditEntry 审计链节点（与 learning/pipeline 同模式：链式哈希 append-only）。
type auditEntry struct {
	Seq      int       `json:"seq"`
	Stage    string    `json:"stage"`
	Detail   string    `json:"detail"`
	PrevHash string    `json:"prev_hash"`
	Hash     string    `json:"hash"`
	At       time.Time `json:"at"`
}

const auditGenesis = "genesis"

// Manager 生命周期管理器（并发安全；单实例管全部候选）。
type Manager struct {
	mu         sync.Mutex
	candidates map[string]*Candidate
	audit      []auditEntry
	now        func() time.Time // 测试确定性注入点
}

// NewManager 构造管理器。
func NewManager() *Manager {
	return &Manager{candidates: make(map[string]*Candidate), now: time.Now}
}

// AuditCount 审计链长。
func (m *Manager) AuditCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.audit)
}

// AuditEntries 审计链拷贝（追加序）。
func (m *Manager) AuditEntries() []auditEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]auditEntry, len(m.audit))
	copy(out, m.audit)
	return out
}

// VerifyAudit 全链校验（逐节点哈希复算 + 前驱链接）。
func (m *Manager) VerifyAudit() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	prev := auditGenesis
	for i, e := range m.audit {
		if e.PrevHash != prev || e.Hash != auditHash(e) || e.Seq != i+1 {
			return fmt.Errorf("lifecycle: 审计链第 %d 节点校验失败", i+1)
		}
		prev = e.Hash
	}
	return nil
}

// Enroll 登记缺口候选（第一段入口；同 ID 重复登记幂等拒绝）。
func (m *Manager) Enroll(c Candidate) error {
	if c.ID == "" || c.Name == "" {
		return fmt.Errorf("lifecycle: 候选 ID 与工具名必填")
	}
	if c.Stage != StageGap && c.Stage != "" {
		return fmt.Errorf("lifecycle: 候选 %s 登记阶段必须为 gap_detected", c.ID)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.candidates[c.ID]; ok {
		return fmt.Errorf("lifecycle: 候选 %s 已登记", c.ID)
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = m.now().UTC()
	}
	c.Stage = StageGap
	m.candidates[c.ID] = &c
	m.appendAuditLocked("enroll", fmt.Sprintf("候选 %s（工具 %s，域 %s）缺口登记", c.ID, c.Name, c.Domain))
	return nil
}

// AttachArtifact 第二段：工件生成完成，附上字节（sha256 由管理器复算锚定）。
// 治理提示：本方法是受控释放通道的**唯一工件入口**；通道未开放时工件只能
// 来自外部开发（提案 §一 红线不变式不受本方法影响——宿主进程零写入零加载
// 由 code 层提案的确定性断言守卫，不归本状态机管）。
func (m *Manager) AttachArtifact(id string, artifact []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.candidates[id]
	if !ok {
		return fmt.Errorf("lifecycle: 候选 %s 未登记", id)
	}
	if c.Stage != StageGap {
		return fmt.Errorf("lifecycle: 候选 %s 处于 %s，不得重复附加工件", id, c.Stage)
	}
	if len(artifact) == 0 {
		return fmt.Errorf("lifecycle: 候选 %s 工件为空", id)
	}
	sum := sha256.Sum256(artifact)
	c.Artifact = append([]byte(nil), artifact...)
	c.ArtifactSHA = hex.EncodeToString(sum[:])
	m.appendAuditLocked("generate", fmt.Sprintf("候选 %s 工件就绪（sha256 %s，%d 字节）", id, c.ArtifactSHA[:12], len(artifact)))
	return nil
}

// Advance 向下一阶段推进（禁止跳段与回退；retire 走 Retire）。
// 每次推进要求 evidence.Pass=true，证据入审计。
func (m *Manager) Advance(id string, evidence Evidence) error {
	if !evidence.Pass {
		return fmt.Errorf("lifecycle: 候选 %s 推进证据未通过（probe=%s）", id, evidence.Probe)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.candidates[id]
	if !ok {
		return fmt.Errorf("lifecycle: 候选 %s 未登记", id)
	}
	cur := stageRank(c.Stage)
	next := cur + 1
	if next < 0 || next >= len(stageOrder)-1 {
		return fmt.Errorf("lifecycle: 候选 %s 处于终态/未知阶段 %s", id, c.Stage)
	}
	target := stageOrder[next]
	if target == StageGenerated && len(c.Artifact) == 0 {
		return fmt.Errorf("lifecycle: 候选 %s 无工件，不得推进 generated（先 AttachArtifact）", id)
	}
	if !stageProbeMatch(target, evidence.Probe) {
		return fmt.Errorf("lifecycle: 候选 %s 推进到 %s 需要 %s 类证据，got %s", id, target, probeFor(target), evidence.Probe)
	}
	if evidence.At.IsZero() {
		evidence.At = m.now().UTC()
	}
	c.Stage = target
	if target == StageRegistered {
		c.RegisteredAt = evidence.At
	}
	m.appendAuditLocked("advance", fmt.Sprintf("候选 %s → %s（%s：%s）", id, target, evidence.Probe, evidence.Detail))
	return nil
}

// Retire 退役（从任意存活阶段可达；policy 为退役理由，可审计）。
func (m *Manager) Retire(id, policy string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.candidates[id]
	if !ok {
		return fmt.Errorf("lifecycle: 候选 %s 未登记", id)
	}
	if c.Stage == StageRetired {
		return fmt.Errorf("lifecycle: 候选 %s 已退役", id)
	}
	c.Stage = StageRetired
	c.RetiredAt = m.now().UTC()
	m.appendAuditLocked("retire", fmt.Sprintf("候选 %s 退役（%s）", id, policy))
	return nil
}

// Get 候选快照。
func (m *Manager) Get(id string) (Candidate, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.candidates[id]
	if !ok {
		return Candidate{}, false
	}
	cp := *c
	cp.Artifact = append([]byte(nil), c.Artifact...)
	return cp, true
}

// ListRegistered 全部已注册（signed_registered）候选 ID（升序）。
func (m *Manager) ListRegistered() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for id, c := range m.candidates {
		if c.Stage == StageRegistered {
			out = append(out, id)
		}
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// probeFor 目标阶段的必备证据类别。
func probeFor(s Stage) string {
	switch s {
	case StageGenerated:
		return "generation"
	case StageRehearsed:
		return "rehearsal"
	case StageAdversarial:
		return "adversarial"
	case StageRegistered:
		return "cosign"
	}
	return "unknown"
}

// stageProbeMatch 推进证据类别匹配（防用彩排证据混过对抗门）。
func stageProbeMatch(target Stage, probe string) bool {
	return probeFor(target) == probe
}

// appendAuditLocked 追加审计节点（须持锁）。
func (m *Manager) appendAuditLocked(stage, detail string) {
	prev := auditGenesis
	if len(m.audit) > 0 {
		prev = m.audit[len(m.audit)-1].Hash
	}
	e := auditEntry{
		Seq:      len(m.audit) + 1,
		Stage:    stage,
		Detail:   detail,
		PrevHash: prev,
		At:       m.now().UTC(),
	}
	e.Hash = auditHash(e)
	m.audit = append(m.audit, e)
}

// auditHash 节点哈希（确定性复算）。
func auditHash(e auditEntry) string {
	h := sha256.Sum256([]byte(strconv.Itoa(e.Seq) + "|" + e.At.Format(time.RFC3339Nano) + "|" + e.Stage + "|" + e.Detail + "|" + e.PrevHash))
	return hex.EncodeToString(h[:])
}

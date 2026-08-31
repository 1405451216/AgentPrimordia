// audit.go — 管道审计链（命题 3：≥3 轮闭环全程零人工、audit 链完整）
//
// 链式哈希：Hash = sha256(Seq|At|Stage|Detail|PrevHash)。首节点 PrevHash =
// "genesis"。任何环节被篡改/删除都会让后续校验断裂——Append-only。
package pipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"sync"
	"time"
)

// auditGenesis 首节点前驱哈希。
const auditGenesis = "genesis"

// AuditChain append-only 审计链（并发安全）。
type AuditChain struct {
	mu      sync.Mutex
	entries []AuditEntry
}

// NewAuditChain 构造空审计链。
func NewAuditChain() *AuditChain {
	return &AuditChain{}
}

// Append 追加一个审计节点（Stage/Detail 必填；At 缺省取当前 UTC 时间）。
// 返回该节点的审计 ID（"audit-<seq>"）。
func (a *AuditChain) Append(stage, detail string) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	at := time.Now().UTC()
	prev := auditGenesis
	if len(a.entries) > 0 {
		prev = a.entries[len(a.entries)-1].Hash
	}
	seq := len(a.entries) + 1
	e := AuditEntry{
		Seq:      seq,
		Stage:    stage,
		Detail:   detail,
		PrevHash: prev,
		At:       at,
	}
	e.Hash = auditHash(e)
	a.entries = append(a.entries, e)
	return fmt.Sprintf("audit-%d", seq)
}

// auditHash 节点哈希：sha256(Seq|At|Stage|Detail|PrevHash)，确定性可复算。
func auditHash(e AuditEntry) string {
	h := sha256.Sum256([]byte(strconv.Itoa(e.Seq) + "|" + e.At.Format(time.RFC3339Nano) + "|" + e.Stage + "|" + e.Detail + "|" + e.PrevHash))
	return hex.EncodeToString(h[:])
}

// Entries 返回审计链拷贝（追加序）。
func (a *AuditChain) Entries() []AuditEntry {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]AuditEntry, len(a.entries))
	copy(out, a.entries)
	return out
}

// Verify 校验整链完整性：逐节点复算哈希与前驱链接。
// 返回 nil = 链完整；否则返回首个断裂位置的错误。
func (a *AuditChain) Verify() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	prev := auditGenesis
	for i, e := range a.entries {
		if e.PrevHash != prev {
			return fmt.Errorf("pipeline: 审计链第 %d 节点前驱断裂", i+1)
		}
		if e.Hash != auditHash(e) {
			return fmt.Errorf("pipeline: 审计链第 %d 节点哈希不复算", i+1)
		}
		if e.Seq != i+1 {
			return fmt.Errorf("pipeline: 审计链第 %d 节点序号错乱", i+1)
		}
		prev = e.Hash
	}
	return nil
}

// Count 链长。
func (a *AuditChain) Count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.entries)
}

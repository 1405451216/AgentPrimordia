package memory

import (
	"sync"
)

// WorkingMemory 短期工作记忆（分层记忆的第一层）。
// 保存当前对话上下文（以 Episode 表示每条消息），
// 当估算 token 数超过预算时，做滑动窗口裁剪，避免无限增长。
//
// 设计取舍：
//   - 纯内存结构，不依赖持久化层；
//   - 压缩策略为"保留最近一半"，更重的语义压缩由 MemoryDistiller 在持久层完成；
//   - 并发安全：所有读写走 mu。
type WorkingMemory struct {
	mu        sync.Mutex
	messages  []*Episode
	maxTokens int
}

// NewWorkingMemory 创建工作记忆。
// maxTokens <= 0 时使用默认预算 4000。
func NewWorkingMemory(maxTokens int) *WorkingMemory {
	if maxTokens <= 0 {
		maxTokens = 4000
	}
	return &WorkingMemory{
		messages:  make([]*Episode, 0),
		maxTokens: maxTokens,
	}
}

// Append 追加一条消息（对话的一轮）。
func (w *WorkingMemory) Append(ep *Episode) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.messages = append(w.messages, ep)
}

// Messages 返回当前上下文的快照（副本，调用方安全持有）。
func (w *WorkingMemory) Messages() []*Episode {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]*Episode, len(w.messages))
	copy(out, w.messages)
	return out
}

// EstimateTokens 估算当前上下文的 token 数（持锁）。
func (w *WorkingMemory) EstimateTokens() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	total := 0
	for _, m := range w.messages {
		total += estimateTokensFor(m.Content)
	}
	return total
}

// MaxTokens 返回当前预算。
func (w *WorkingMemory) MaxTokens() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.maxTokens
}

// Compress 超过预算时裁剪早期消息（滑动窗口：保留最近一半）。
// 返回 true 表示发生了裁剪。
func (w *WorkingMemory) Compress() bool {
	w.mu.Lock()
	defer w.mu.Unlock()

	total := 0
	for _, m := range w.messages {
		total += estimateTokensFor(m.Content)
	}
	if total <= w.maxTokens || len(w.messages) <= 1 {
		return false
	}

	keep := len(w.messages) / 2
	if keep < 1 {
		keep = 1
	}
	// 复制保留部分，释放早期切片引用
	w.messages = append([]*Episode{}, w.messages[keep:]...)
	return true
}

// estimateTokensFor 粗略估算字符串 token 数。
// 英文等：4 字符 ≈ 1 token；CJK：1.5 字符 ≈ 1 token。
func estimateTokensFor(s string) int {
	cjk := 0
	other := 0
	for _, r := range s {
		if (r >= 0x4E00 && r <= 0x9FFF) || // CJK 统一表意文字
			(r >= 0x3040 && r <= 0x309F) || // 平假名
			(r >= 0x30A0 && r <= 0x30FF) { // 片假名
			cjk++
		} else {
			other++
		}
	}
	return int(float64(cjk)/1.5 + float64(other)/4.0)
}

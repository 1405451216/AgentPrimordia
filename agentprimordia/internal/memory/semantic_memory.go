package memory

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Pattern 结构化tool使用模式（从 Episodic 蒸馏得到）。
type Pattern struct {
	Pattern     string
	Description string
	SuccessRate float64
	Examples    []string
	LastUpdated time.Time
}

// Fact 结构化事实（来源：蒸馏 或 用户直供）。
type Fact struct {
	Key        string
	Value      string
	Confidence float64
	Source     string // "distilled" | "user_provided"
	CreatedAt  time.Time
}

// SemanticMemory 长期语义记忆（分层记忆的第三层）。
// 保存从 Episodic 蒸馏出的结构化模式与事实，
// 供 ReAct 每轮开始时注入 system prompt，提升一致性。
type SemanticMemory struct {
	mu       sync.RWMutex
	patterns map[string]Pattern
	facts    map[string]Fact
}

// NewSemanticMemory 创建语义记忆。
func NewSemanticMemory() *SemanticMemory {
	return &SemanticMemory{
		patterns: make(map[string]Pattern),
		facts:    make(map[string]Fact),
	}
}

// AddPattern 记录/更新一个tool使用模式。
func (s *SemanticMemory) AddPattern(ctx context.Context, p Pattern) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p.LastUpdated = time.Now()
	s.patterns[p.Pattern] = p
}

// AddFact 记录/更新一个结构化事实。
func (s *SemanticMemory) AddFact(ctx context.Context, f Fact) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f.CreatedAt = time.Now()
	s.facts[f.Key] = f
}

// Patterns 返回所有模式快照。
func (s *SemanticMemory) Patterns() []Pattern {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Pattern, 0, len(s.patterns))
	for _, p := range s.patterns {
		out = append(out, p)
	}
	return out
}

// Facts 返回所有事实快照。
func (s *SemanticMemory) Facts() []Fact {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Fact, 0, len(s.facts))
	for _, f := range s.facts {
		out = append(out, f)
	}
	return out
}

// InjectPrompt 把语义记忆序列化为 system prompt 片段。
// 无内容时返回空字符串（调用方据此决定是否注入）。
func (s *SemanticMemory) InjectPrompt() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var b strings.Builder
	if len(s.facts) > 0 {
		b.WriteString("## 已知事实\n")
		for _, f := range s.facts {
			fmt.Fprintf(&b, "- %s: %s（置信度 %.2f，来源 %s）\n", f.Key, f.Value, f.Confidence, f.Source)
		}
	}
	if len(s.patterns) > 0 {
		b.WriteString("## tool使用模式\n")
		for _, p := range s.patterns {
			fmt.Fprintf(&b, "- %s: %s（成功率 %.2f）\n", p.Pattern, p.Description, p.SuccessRate)
		}
	}
	return b.String()
}

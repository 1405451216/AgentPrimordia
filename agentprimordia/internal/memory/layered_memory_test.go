package memory

import (
	"context"
	"strings"
	"testing"
)

// stubLLM 是 LLMClient 的测试桩。
type stubLLM struct {
	resp string
	err  error
}

func (s *stubLLM) Complete(ctx context.Context, prompt string) (string, error) {
	return s.resp, s.err
}

func TestWorkingMemory_AppendAndEstimate(t *testing.T) {
	wm := NewWorkingMemory(4000)
	if wm.MaxTokens() != 4000 {
		t.Errorf("MaxTokens = %d", wm.MaxTokens())
	}
	ep := MustEpisode("s1", "user", strings.Repeat("hello ", 40))
	wm.Append(ep)
	if got := wm.EstimateTokens(); got <= 0 {
		t.Errorf("EstimateTokens = %d, want > 0", got)
	}
	if len(wm.Messages()) != 1 {
		t.Errorf("Messages len = %d, want 1", len(wm.Messages()))
	}
}

func TestWorkingMemory_Compress(t *testing.T) {
	// 预算很小，4 条大消息必超预算
	wm := NewWorkingMemory(50)
	for i := 0; i < 4; i++ {
		wm.Append(MustEpisode("s1", "user", strings.Repeat("hello ", 40)))
	}
	if wm.EstimateTokens() <= wm.MaxTokens() {
		t.Fatalf("预期超预算，tokens=%d budget=%d", wm.EstimateTokens(), wm.MaxTokens())
	}
	if !wm.Compress() {
		t.Fatal("首次 Compress 应返回 true（发生了裁剪）")
	}
	if len(wm.Messages()) >= 4 {
		t.Errorf("Compress 后应减少消息数，got %d", len(wm.Messages()))
	}

	// 反复压缩应单调收敛到 1 条，且最终（len<=1）返回 false
	last := len(wm.Messages())
	for i := 0; i < 5; i++ {
		if !wm.Compress() {
			break
		}
		cur := len(wm.Messages())
		if cur >= last {
			t.Errorf("Compress 未继续减少：%d -> %d", last, cur)
		}
		last = cur
	}
	if len(wm.Messages()) != 1 {
		t.Errorf("收敛后应只剩 1 条，got %d", len(wm.Messages()))
	}
	if wm.Compress() {
		t.Error("len<=1 时 Compress 应返回 false")
	}
}

func TestSemanticMemory_AddAndInject(t *testing.T) {
	sem := NewSemanticMemory()
	ctx := context.Background()

	sem.AddPattern(ctx, Pattern{Pattern: "shell", Description: "文件操作", SuccessRate: 0.9})
	sem.AddFact(ctx, Fact{Key: "project", Value: "AgentPrimordia", Confidence: 0.8, Source: "user_provided"})

	if len(sem.Patterns()) != 1 {
		t.Errorf("Patterns len = %d, want 1", len(sem.Patterns()))
	}
	if len(sem.Facts()) != 1 {
		t.Errorf("Facts len = %d, want 1", len(sem.Facts()))
	}

	prompt := sem.InjectPrompt()
	if !strings.Contains(prompt, "AgentPrimordia") {
		t.Errorf("InjectPrompt 应含事实值，got %q", prompt)
	}
	if !strings.Contains(prompt, "shell") {
		t.Errorf("InjectPrompt 应含模式名，got %q", prompt)
	}
}

func TestMemoryDistiller_ValidJSON(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()
	_ = store.Add(ctx, MustEpisode("s1", "user", "用 shell 列出文件"))
	_ = store.Add(ctx, MustEpisode("s1", "assistant", "已执行 ls"))

	sem := NewSemanticMemory()
	resp := `{"patterns":[{"pattern":"shell","description":"文件操作","success_rate":0.9}],"facts":[{"key":"project","value":"AgentPrimordia","confidence":0.8}]}`
	dist := NewMemoryDistiller(store, sem, &stubLLM{resp: resp})

	if err := dist.Distill(ctx, "s1"); err != nil {
		t.Fatalf("Distill 失败: %v", err)
	}
	if len(sem.Patterns()) != 1 {
		t.Fatalf("Patterns = %d, want 1", len(sem.Patterns()))
	}
	if got := sem.Patterns()[0].SuccessRate; got != 0.9 {
		t.Errorf("SuccessRate = %v, want 0.9", got)
	}
	if len(sem.Facts()) != 1 {
		t.Fatalf("Facts = %d, want 1", len(sem.Facts()))
	}
	if got := sem.Facts()[0]; got.Confidence != 0.8 || got.Source != "distilled" {
		t.Errorf("Fact = %+v, want confidence 0.8 source distilled", got)
	}
}

func TestMemoryDistiller_EmptyStore(t *testing.T) {
	store := NewInMemoryStore()
	sem := NewSemanticMemory()
	dist := NewMemoryDistiller(store, sem, &stubLLM{resp: "{}"})
	if err := dist.Distill(context.Background(), "empty"); err != nil {
		t.Fatalf("空会话 Distill 应返回 nil, got %v", err)
	}
	if len(sem.Patterns()) != 0 || len(sem.Facts()) != 0 {
		t.Error("空会话不应写入任何知识")
	}
}

func TestMemoryDistiller_BadJSON(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()
	_ = store.Add(ctx, MustEpisode("s1", "user", "hello"))

	sem := NewSemanticMemory()
	// 输出无法解析为 JSON：应优雅降级（不 panic、不写入）
	dist := NewMemoryDistiller(store, sem, &stubLLM{resp: "抱歉，我无法蒸馏。"})
	if err := dist.Distill(ctx, "s1"); err != nil {
		t.Fatalf("坏 JSON 不应返回 error, got %v", err)
	}
	if len(sem.Patterns()) != 0 || len(sem.Facts()) != 0 {
		t.Error("坏 JSON 不应写入任何知识")
	}
}

func TestMemoryDistiller_LLMError(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()
	_ = store.Add(ctx, MustEpisode("s1", "user", "hello"))
	sem := NewSemanticMemory()
	dist := NewMemoryDistiller(store, sem, &stubLLM{err: context.Canceled})
	if err := dist.Distill(ctx, "s1"); err == nil {
		t.Error("LLM 错误应向上返回")
	}
}

// TestLayeredMemoryIntegration 端到端验证三层记忆协作：
// WorkingMemory（上下文）→ Episodic（InMemoryStore）→ Semantic（蒸馏）→ InjectPrompt。
func TestLayeredMemoryIntegration(t *testing.T) {
	ctx := context.Background()

	store := NewInMemoryStore()
	_ = store.Add(ctx, MustEpisode("s1", "user", "用 shell 创建目录"))
	_ = store.Add(ctx, MustEpisode("s1", "assistant", "已执行 mkdir"))

	// 第一层：工作记忆保存当前上下文
	wm := NewWorkingMemory(4000)
	eps, _ := store.List(ctx, &ListOptions{SessionID: "s1"})
	for _, ep := range eps {
		wm.Append(ep)
	}
	if len(wm.Messages()) != 2 {
		t.Fatalf("工作记忆消息数 = %d, want 2", len(wm.Messages()))
	}

	// 第三层：语义记忆（经蒸馏填充）
	sem := NewSemanticMemory()
	resp := `{"patterns":[{"pattern":"shell","description":"创建目录","success_rate":0.95}],"facts":[]}`
	dist := NewMemoryDistiller(store, sem, &stubLLM{resp: resp})
	if err := dist.Distill(ctx, "s1"); err != nil {
		t.Fatalf("Distill 失败: %v", err)
	}

	// 每轮注入 system prompt 片段
	inject := sem.InjectPrompt()
	if !strings.Contains(inject, "shell") {
		t.Errorf("注入片段应含蒸馏出的模式，got %q", inject)
	}
}

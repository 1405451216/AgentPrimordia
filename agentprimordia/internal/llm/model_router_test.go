// model_router_test.go — G2-1 模型路由器单元测试
package llm

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// routerMockProvider 用于测试的最小化 Provider 实现
type routerMockProvider struct {
	name string
}

func (m *routerMockProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	return &CompletionResponse{Model: m.name, Content: "ok"}, nil
}
func (m *routerMockProvider) Stream(ctx context.Context, req *CompletionRequest) (<-chan Chunk, error) {
	ch := make(chan Chunk, 1)
	ch <- Chunk{Content: "ok", Done: true}
	close(ch)
	return ch, nil
}
func (m *routerMockProvider) CallTools(ctx context.Context, req *ToolCallRequest) (*ToolCallResponse, error) {
	return &ToolCallResponse{}, nil
}
func (m *routerMockProvider) Info() ModelInfo { return ModelInfo{Name: m.name, Provider: "mock"} }

// ===== evaluateComplexity 测试 =====

func TestEvaluateComplexity_ShortMessage(t *testing.T) {
	r := NewModelRouter(StrategyBalanced)
	messages := []ChatMessage{{Role: "user", Content: "hi"}}
	c := r.evaluateComplexity(messages)
	if c != 0 {
		t.Errorf("expected 0 complexity for short message, got %f", c)
	}
}

func TestEvaluateComplexity_LongMessage(t *testing.T) {
	r := NewModelRouter(StrategyBalanced)
	long := strings.Repeat("a", 5000)
	messages := []ChatMessage{{Role: "user", Content: long}}
	c := r.evaluateComplexity(messages)
	if c < 0.3 {
		t.Errorf("expected >= 0.3 complexity for long message, got %f", c)
	}
}

func TestEvaluateComplexity_WithKeywords(t *testing.T) {
	r := NewModelRouter(StrategyBalanced)
	messages := []ChatMessage{{Role: "user", Content: "请帮我实现一个 function"}}
	c := r.evaluateComplexity(messages)
	if c < 0.2 {
		t.Errorf("expected >= 0.2 for code keywords, got %f", c)
	}
}

func TestEvaluateComplexity_CappedAt1(t *testing.T) {
	r := NewModelRouter(StrategyBalanced)
	long := strings.Repeat("代码 code step 步骤 分析 analyze ", 1000)
	messages := []ChatMessage{{Role: "user", Content: long}}
	c := r.evaluateComplexity(messages)
	if c > 1.0 {
		t.Errorf("complexity must be capped at 1.0, got %f", c)
	}
}

// ===== estimateTokens 测试 =====

func TestEstimateTokens_Empty(t *testing.T) {
	tokens := estimateTokens(nil)
	if tokens != 0 {
		t.Errorf("expected 0 tokens for empty messages, got %d", tokens)
	}
}

func TestEstimateTokens_EnglishShort(t *testing.T) {
	messages := []ChatMessage{{Role: "user", Content: "Hello world"}}
	tokens := estimateTokens(messages)
	// "Hello world" = 11 chars ≈ 3 tokens + 4 (role) = 7
	if tokens < 5 || tokens > 15 {
		t.Errorf("expected ~7 tokens, got %d", tokens)
	}
}

func TestEstimateTokens_Chinese(t *testing.T) {
	messages := []ChatMessage{{Role: "user", Content: "你好世界"}}
	tokens := estimateTokens(messages)
	// 4 CJK chars ≈ 3 tokens + 4 (role) = 7
	if tokens < 4 || tokens > 12 {
		t.Errorf("expected ~7 tokens for Chinese, got %d", tokens)
	}
}

// ===== Route 测试 =====

func TestRoute_CostFirst(t *testing.T) {
	r := NewModelRouter(StrategyCostFirst)
	r.Register(ModelRouteConfig{
		Name: "expensive", Provider: &routerMockProvider{name: "exp"},
		CostPer1K: 0.1, ComplexityLimit: 1.0, MaxContext: 100000, Priority: 2,
	})
	r.Register(ModelRouteConfig{
		Name: "cheap", Provider: &routerMockProvider{name: "cheap"},
		CostPer1K: 0.001, ComplexityLimit: 1.0, MaxContext: 100000, Priority: 2,
	})
	messages := []ChatMessage{{Role: "user", Content: "hi"}}
	_, decision, err := r.Route(context.Background(), messages, false)
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}
	if decision.ModelName != "cheap" {
		t.Errorf("expected cheap model, got %s", decision.ModelName)
	}
}

func TestRoute_QualityFirst(t *testing.T) {
	r := NewModelRouter(StrategyQualityFirst)
	r.Register(ModelRouteConfig{
		Name: "small", Provider: &routerMockProvider{name: "small"},
		CostPer1K: 0.001, ComplexityLimit: 0.5, MaxContext: 100000, Priority: 2,
	})
	r.Register(ModelRouteConfig{
		Name: "big", Provider: &routerMockProvider{name: "big"},
		CostPer1K: 0.1, ComplexityLimit: 1.0, MaxContext: 100000, Priority: 2,
	})
	messages := []ChatMessage{{Role: "user", Content: "hi"}}
	_, decision, err := r.Route(context.Background(), messages, false)
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}
	if decision.ModelName != "big" {
		t.Errorf("expected big model for quality-first, got %s", decision.ModelName)
	}
}

func TestRoute_ToolFilter(t *testing.T) {
	r := NewModelRouter(StrategyCostFirst)
	r.Register(ModelRouteConfig{
		Name: "no-tools", Provider: &routerMockProvider{name: "no-tools"},
		CostPer1K: 0.001, ComplexityLimit: 1.0, MaxContext: 100000, SupportsTools: false,
	})
	r.Register(ModelRouteConfig{
		Name: "with-tools", Provider: &routerMockProvider{name: "with-tools"},
		CostPer1K: 0.01, ComplexityLimit: 1.0, MaxContext: 100000, SupportsTools: true,
	})
	messages := []ChatMessage{{Role: "user", Content: "hi"}}
	_, decision, err := r.Route(context.Background(), messages, true)
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}
	if decision.ModelName != "with-tools" {
		t.Errorf("expected with-tools when tools needed, got %s", decision.ModelName)
	}
}

func TestRoute_ContextLimit(t *testing.T) {
	r := NewModelRouter(StrategyCostFirst)
	r.Register(ModelRouteConfig{
		Name: "small-ctx", Provider: &routerMockProvider{name: "small-ctx"},
		CostPer1K: 0.001, ComplexityLimit: 1.0, MaxContext: 10,
	})
	r.Register(ModelRouteConfig{
		Name: "big-ctx", Provider: &routerMockProvider{name: "big-ctx"},
		CostPer1K: 0.001, ComplexityLimit: 1.0, MaxContext: 1000000,
	})
	// 构造超长消息触发 small-ctx 过滤
	long := strings.Repeat("a", 1000)
	messages := []ChatMessage{{Role: "user", Content: long}}
	_, decision, err := r.Route(context.Background(), messages, false)
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}
	if decision.ModelName != "big-ctx" {
		t.Errorf("expected big-ctx for long context, got %s", decision.ModelName)
	}
}

func TestRoute_NoSuitable(t *testing.T) {
	r := NewModelRouter(StrategyCostFirst)
	r.Register(ModelRouteConfig{
		Name: "low-cap", Provider: &routerMockProvider{name: "low-cap"},
		CostPer1K: 0.001, ComplexityLimit: 0.1, MaxContext: 100000,
	})
	// 构造高复杂度消息触发过滤
	long := strings.Repeat("代码 code step 步骤 ", 1000)
	messages := []ChatMessage{{Role: "user", Content: long}}
	_, _, err := r.Route(context.Background(), messages, false)
	if !errors.Is(err, ErrNoSuitableModel) {
		t.Errorf("expected ErrNoSuitableModel, got %v", err)
	}
}

func TestRoute_Fallback(t *testing.T) {
	r := NewModelRouter(StrategyCostFirst)
	r.Register(ModelRouteConfig{
		Name: "fallback-model", Provider: &routerMockProvider{name: "fallback"},
		CostPer1K: 0.001, ComplexityLimit: 0.1, MaxContext: 100000,
	})
	r.SetFallback("fallback-model")
	long := strings.Repeat("代码 code step 步骤 ", 1000)
	messages := []ChatMessage{{Role: "user", Content: long}}
	_, decision, err := r.Route(context.Background(), messages, false)
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}
	if decision.ModelName != "fallback-model" {
		t.Errorf("expected fallback-model, got %s", decision.ModelName)
	}
}

func TestRoute_ComplexityEstimate(t *testing.T) {
	r := NewModelRouter(StrategyCostFirst)
	r.Register(ModelRouteConfig{
		Name: "any", Provider: &routerMockProvider{name: "any"},
		CostPer1K: 0.001, ComplexityLimit: 1.0, MaxContext: 100000,
	})
	long := strings.Repeat("a", 5000)
	messages := []ChatMessage{{Role: "user", Content: long}}
	_, decision, err := r.Route(context.Background(), messages, false)
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}
	if decision.Complexity < 0.3 {
		t.Errorf("expected complexity >= 0.3 for 5000-char msg, got %f", decision.Complexity)
	}
	if decision.EstimatedCost <= 0 {
		t.Errorf("expected positive estimated cost, got %f", decision.EstimatedCost)
	}
}

// ===== Stats 测试 =====

func TestStatsSnapshot(t *testing.T) {
	r := NewModelRouter(StrategyCostFirst)
	r.Register(ModelRouteConfig{
		Name: "m1", Provider: &routerMockProvider{name: "m1"},
		CostPer1K: 0.001, ComplexityLimit: 1.0, MaxContext: 100000,
	})
	r.Record("m1", 100, 0.001, nil)
	r.Record("m1", 200, 0.002, errors.New("fail"))
	snap := r.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("expected 1 model stat, got %d", len(snap))
	}
	if snap["m1"].Calls != 2 {
		t.Errorf("expected 2 calls, got %d", snap["m1"].Calls)
	}
	if snap["m1"].Failures != 1 {
		t.Errorf("expected 1 failure, got %d", snap["m1"].Failures)
	}
	if snap["m1"].TotalMs != 300 {
		t.Errorf("expected 300 ms total, got %d", snap["m1"].TotalMs)
	}
	if snap["m1"].TotalCost < 0.0029 || snap["m1"].TotalCost > 0.0031 {
		t.Errorf("expected ~0.003 cost, got %f", snap["m1"].TotalCost)
	}
}

// ===== 并发安全测试 =====

func TestRouter_Concurrent(t *testing.T) {
	r := NewModelRouter(StrategyCostFirst)
	r.Register(ModelRouteConfig{
		Name: "m1", Provider: &routerMockProvider{name: "m1"},
		CostPer1K: 0.001, ComplexityLimit: 1.0, MaxContext: 100000,
	})
	r.Register(ModelRouteConfig{
		Name: "m2", Provider: &routerMockProvider{name: "m2"},
		CostPer1K: 0.005, ComplexityLimit: 1.0, MaxContext: 100000,
	})

	const goroutines = 20
	const iterations = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_, _, _ = r.Route(context.Background(), []ChatMessage{{Role: "user", Content: "test"}}, false)
				r.Record("m1", 10, 0.0001, nil)
			}
		}()
	}
	wg.Wait()

	snap := r.Snapshot()
	totalCalls := snap["m1"].Calls + snap["m2"].Calls
	expected := int64(goroutines * iterations)
	if totalCalls != expected {
		t.Errorf("expected %d total record calls, got %d", expected, totalCalls)
	}
}

func TestRouter_AtomicStatsNoLoss(t *testing.T) {
	// 验证并发 Record 不丢失计数
	r := NewModelRouter(StrategyCostFirst)
	r.Register(ModelRouteConfig{
		Name: "m", Provider: &routerMockProvider{name: "m"},
		CostPer1K: 0.001, ComplexityLimit: 1.0, MaxContext: 100000,
	})

	const n = 1000
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			r.Record("m", int64(time.Now().UnixNano()%100), 0.0001, nil)
		}()
	}
	wg.Wait()

	snap := r.Snapshot()
	if snap["m"].Calls != int64(n) {
		t.Errorf("expected %d calls, got %d (atomic stats lost updates)", n, snap["m"].Calls)
	}
}

func TestRouteStrategy_String(t *testing.T) {
	cases := []struct {
		s        RouteStrategy
		expected string
	}{
		{StrategyCostFirst, "cost-first"},
		{StrategyQualityFirst, "quality-first"},
		{StrategyBalanced, "balanced"},
		{RouteStrategy(999), "unknown"},
	}
	for _, tc := range cases {
		if got := tc.s.String(); got != tc.expected {
			t.Errorf("RouteStrategy(%d).String() = %s, want %s", tc.s, got, tc.expected)
		}
	}
}

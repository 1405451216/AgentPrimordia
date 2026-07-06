package metrics

import (
	"io"
	"log/slog"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ===== RecordCost 系列测试 =====

// TestRecordCostUSD_Basic 测试 RecordCostUSD 累加性
func TestRecordCostUSD_Basic(t *testing.T) {
	m := NewMetrics()

	m.RecordCostUSD("openai", "gpt-4", "agent-1", 0.01)
	m.RecordCostUSD("openai", "gpt-4", "agent-1", 0.02)
	m.RecordCostUSD("openai", "gpt-4", "agent-1", 0.005)

	m.mu.RLock()
	lc, ok := m.CostByLabel["openai|gpt-4|agent-1"]
	m.mu.RUnlock()

	if !ok {
		t.Fatal("expected CostByLabel[\"openai|gpt-4|agent-1\"] to exist")
	}

	cost := math.Float64frombits(lc.costBits.Load())
	expected := 0.035
	if math.Abs(cost-expected) > 1e-9 {
		t.Errorf("expected cost=%v, got %v", expected, cost)
	}
}

// TestRecordCostUSD_MultipleLabels 测试不同 (provider, model, agentName) 维度独立计数
func TestRecordCostUSD_MultipleLabels(t *testing.T) {
	m := NewMetrics()

	m.RecordCostUSD("openai", "gpt-4", "agent-1", 0.10)
	m.RecordCostUSD("openai", "gpt-4", "agent-2", 0.20)
	m.RecordCostUSD("anthropic", "claude-3", "agent-1", 0.30)
	m.RecordCostUSD("anthropic", "claude-3", "agent-2", 0.40)

	tests := []struct {
		key      string
		expected float64
	}{
		{"openai|gpt-4|agent-1", 0.10},
		{"openai|gpt-4|agent-2", 0.20},
		{"anthropic|claude-3|agent-1", 0.30},
		{"anthropic|claude-3|agent-2", 0.40},
	}

	for _, tc := range tests {
		m.mu.RLock()
		lc, ok := m.CostByLabel[tc.key]
		m.mu.RUnlock()

		if !ok {
			t.Errorf("expected %s to exist", tc.key)
			continue
		}

		got := math.Float64frombits(lc.costBits.Load())
		if math.Abs(got-tc.expected) > 1e-9 {
			t.Errorf("%s: expected cost=%v, got %v", tc.key, tc.expected, got)
		}
	}
}

// TestRecordCostCalls_Basic 测试 RecordCostCalls 累加
func TestRecordCostCalls_Basic(t *testing.T) {
	m := NewMetrics()

	m.RecordCostCalls("openai", "gpt-4", "agent-1", 1)
	m.RecordCostCalls("openai", "gpt-4", "agent-1", 2)
	m.RecordCostCalls("openai", "gpt-4", "agent-1", 3)

	m.mu.RLock()
	lc, ok := m.CostByLabel["openai|gpt-4|agent-1"]
	m.mu.RUnlock()

	if !ok {
		t.Fatal("expected CostByLabel[\"openai|gpt-4|agent-1\"] to exist")
	}

	got := lc.calls.Load()
	if got != 6 {
		t.Errorf("expected calls=6, got %d", got)
	}
}

// TestRecordCostCalls_NegativeIgnored 测试负数调用次数被忽略
func TestRecordCostCalls_NegativeIgnored(t *testing.T) {
	m := NewMetrics()

	m.RecordCostCalls("openai", "gpt-4", "agent-1", 5)
	m.RecordCostCalls("openai", "gpt-4", "agent-1", -1)
	m.RecordCostCalls("openai", "gpt-4", "agent-1", 0)

	m.mu.RLock()
	lc := m.CostByLabel["openai|gpt-4|agent-1"]
	m.mu.RUnlock()

	if lc == nil {
		t.Fatal("expected labeledCost to exist")
	}

	got := lc.calls.Load()
	if got != 5 {
		t.Errorf("expected calls=5 (negative ignored), got %d", got)
	}
}

// TestRecordCostTokens_Basic 测试 RecordCostTokens 按 kind 累加
func TestRecordCostTokens_Basic(t *testing.T) {
	m := NewMetrics()

	m.RecordCostTokens("openai", "gpt-4", "agent-1", "prompt", 100)
	m.RecordCostTokens("openai", "gpt-4", "agent-1", "prompt", 200)
	m.RecordCostTokens("openai", "gpt-4", "agent-1", "completion", 50)

	m.mu.RLock()
	promptLC, ok1 := m.CostTokensByLabel["openai|gpt-4|agent-1|prompt"]
	completionLC, ok2 := m.CostTokensByLabel["openai|gpt-4|agent-1|completion"]
	m.mu.RUnlock()

	if !ok1 || !ok2 {
		t.Fatal("expected both prompt and completion to exist in CostTokensByLabel")
	}

	if got := promptLC.tokens.Load(); got != 300 {
		t.Errorf("expected prompt tokens=300, got %d", got)
	}
	if got := completionLC.tokens.Load(); got != 50 {
		t.Errorf("expected completion tokens=50, got %d", got)
	}
}

// TestRecordCostTokens_NegativeIgnored 测试负数 token 数被忽略
func TestRecordCostTokens_NegativeIgnored(t *testing.T) {
	m := NewMetrics()

	m.RecordCostTokens("openai", "gpt-4", "agent-1", "prompt", 100)
	m.RecordCostTokens("openai", "gpt-4", "agent-1", "prompt", -50)

	m.mu.RLock()
	lc := m.CostTokensByLabel["openai|gpt-4|agent-1|prompt"]
	m.mu.RUnlock()

	if lc == nil {
		t.Fatal("expected labeledCost to exist")
	}
	if got := lc.tokens.Load(); got != 100 {
		t.Errorf("expected tokens=100 (negative ignored), got %d", got)
	}
}

// TestSetLastCostUSD_Basic 测试 SetLastCostUSD gauge 语义（覆盖式赋值）
func TestSetLastCostUSD_Basic(t *testing.T) {
	m := NewMetrics()

	m.SetLastCostUSD("openai", "gpt-4", "agent-1", 0.05)
	m.SetLastCostUSD("openai", "gpt-4", "agent-1", 0.10) // 覆盖

	m.mu.RLock()
	cost, ok := m.CostLastUSDByLabel["openai|gpt-4|agent-1"]
	m.mu.RUnlock()

	if !ok {
		t.Fatal("expected CostLastUSDByLabel to contain key")
	}
	if math.Abs(cost-0.10) > 1e-9 {
		t.Errorf("expected last cost=0.10 (overwritten), got %v", cost)
	}
}

// TestString_CostMetricsFormat 验证 Prometheus 输出格式正确
func TestString_CostMetricsFormat(t *testing.T) {
	m := NewMetrics()

	m.RecordCostUSD("openai", "gpt-4", "agent-1", 0.123456)
	m.RecordCostCalls("openai", "gpt-4", "agent-1", 5)
	m.RecordCostTokens("openai", "gpt-4", "agent-1", "prompt", 100)
	m.RecordCostTokens("openai", "gpt-4", "agent-1", "completion", 50)
	m.SetLastCostUSD("openai", "gpt-4", "agent-1", 0.025)

	output := m.String()

	requireds := []string{
		"# HELP ap_cost_usd_total",
		"# TYPE ap_cost_usd_total counter",
		`ap_cost_usd_total{provider="openai",model="gpt-4",agent_name="agent-1"} 0.123456`,
		"# HELP ap_cost_calls_total",
		"# TYPE ap_cost_calls_total counter",
		`ap_cost_calls_total{provider="openai",model="gpt-4",agent_name="agent-1"} 5`,
		"# HELP ap_cost_tokens_total",
		"# TYPE ap_cost_tokens_total counter",
		`ap_cost_tokens_total{provider="openai",model="gpt-4",agent_name="agent-1",kind="prompt"} 100`,
		`ap_cost_tokens_total{provider="openai",model="gpt-4",agent_name="agent-1",kind="completion"} 50`,
		"# HELP ap_cost_last_call_usd",
		"# TYPE ap_cost_last_call_usd gauge",
		`ap_cost_last_call_usd{provider="openai",model="gpt-4",agent_name="agent-1"} 0.025`,
	}

	for _, want := range requireds {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, output)
		}
	}
}

// TestString_EmptyCostMetrics 验证无 cost 数据时不写入 cost 指标段
func TestString_EmptyCostMetrics(t *testing.T) {
	m := NewMetrics()
	m.RecordLLMCall(100*time.Millisecond, nil)

	output := m.String()

	if strings.Contains(output, "ap_cost_usd_total") {
		t.Error("expected no ap_cost_usd_total when no cost data")
	}
}

// TestRecordCostUSD_Concurrent 测试并发安全性
func TestRecordCostUSD_Concurrent(t *testing.T) {
	m := NewMetrics()
	const goroutines = 50
	const perGoroutine = 100

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				m.RecordCostUSD("openai", "gpt-4", "agent-1", 0.001)
			}
		}()
	}
	wg.Wait()

	m.mu.RLock()
	lc, ok := m.CostByLabel["openai|gpt-4|agent-1"]
	m.mu.RUnlock()

	if !ok {
		t.Fatal("expected labeledCost to exist")
	}

	got := math.Float64frombits(lc.costBits.Load())
	expected := float64(goroutines*perGoroutine) * 0.001
	if math.Abs(got-expected) > 1e-6 {
		t.Errorf("expected concurrent accumulated cost=%v, got %v", expected, got)
	}
}

// TestSplitCostKey 验证 splitCostKey 对各种输入的处理
func TestSplitCostKey(t *testing.T) {
	tests := []struct {
		input string
		wantA string
		wantB string
		wantC string
	}{
		{"openai|gpt-4|agent-1", "openai", "gpt-4", "agent-1"},
		{"openai|gpt-4", "openai", "gpt-4", ""},
		{"single", "single", "", ""},
		{"", "", "", ""},
		{"|empty|a", "", "empty", "a"},
	}

	for _, tc := range tests {
		a, b, c := splitCostKey(tc.input, 3)
		if a != tc.wantA || b != tc.wantB || c != tc.wantC {
			t.Errorf("splitCostKey(%q) = (%q,%q,%q), want (%q,%q,%q)",
				tc.input, a, b, c, tc.wantA, tc.wantB, tc.wantC)
		}
	}
}

// TestCostAtomicAdd 验证 costAtomicAdd 的 CAS 自旋正确性
func TestCostAtomicAdd(t *testing.T) {
	var target atomic.Uint64
	costAtomicAdd(&target, 0.5)
	costAtomicAdd(&target, 0.3)
	costAtomicAdd(&target, 0.2)

	got := math.Float64frombits(target.Load())
	if math.Abs(got-1.0) > 1e-9 {
		t.Errorf("expected accumulated=1.0, got %v", got)
	}
}

// TestDefaultMetrics 验证包级 helper 路由到 defaultMetrics
func TestDefaultMetrics(t *testing.T) {
	// 重置 defaultMetrics（避免与其他测试干扰）
	defaultMetrics.Reset()
	defaultMetrics.RecordCostUSD("test", "test-model", "test-agent", 0.5)

	// 调用包级 helper
	RecordCostUSD("test", "test-model", "test-agent", 0.5)
	RecordCostCalls("test", "test-model", "test-agent", 1)
	RecordCostTokens("test", "test-model", "test-agent", "prompt", 10)
	SetLastCostUSD("test", "test-model", "test-agent", 0.5)

	// 通过 DefaultMetrics() 验证
	m := DefaultMetrics()

	m.mu.RLock()
	lc, ok := m.CostByLabel["test|test-model|test-agent"]
	last, ok2 := m.CostLastUSDByLabel["test|test-model|test-agent"]
	tokenLC, ok3 := m.CostTokensByLabel["test|test-model|test-agent|prompt"]
	m.mu.RUnlock()

	if !ok {
		t.Fatal("expected cost entry")
	}
	if !ok2 {
		t.Fatal("expected last cost entry")
	}
	if !ok3 {
		t.Fatal("expected token entry")
	}

	cost := math.Float64frombits(lc.costBits.Load())
	if math.Abs(cost-1.0) > 1e-9 {
		t.Errorf("expected cost=1.0, got %v", cost)
	}
	if lc.calls.Load() != 1 {
		t.Errorf("expected calls=1, got %d", lc.calls.Load())
	}
	if tokenLC.tokens.Load() != 10 {
		t.Errorf("expected tokens=10, got %d", tokenLC.tokens.Load())
	}
	if math.Abs(last-0.5) > 1e-9 {
		t.Errorf("expected last=0.5, got %v", last)
	}
}

// ===== CostExporter 测试 =====

// stubLogger 静默 logger 用于测试
func stubLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// stubCostSource 是 CostSource 的简单测试桩
type stubCostSource struct {
	summary CostSourceSummary
	records []CostSourceRecord
}

func (s *stubCostSource) Summary() CostSourceSummary  { return s.summary }
func (s *stubCostSource) Records() []CostSourceRecord { return s.records }

// TestNewCostExporter_Defaults 测试默认配置
func TestNewCostExporter_Defaults(t *testing.T) {
	src := &stubCostSource{}
	cfg := CostExporterConfig{
		Source: src,
		Logger: stubLogger(),
	}
	exp := NewCostExporter(cfg)

	if exp.interval != 15*time.Second {
		t.Errorf("expected default interval=15s, got %v", exp.interval)
	}
	if exp.source != src {
		t.Error("expected source to be set")
	}
}

// TestNewCostExporter_CustomInterval 测试自定义 interval
func TestNewCostExporter_CustomInterval(t *testing.T) {
	src := &stubCostSource{}
	cfg := CostExporterConfig{
		Source:   src,
		Interval: 5 * time.Second,
		Logger:   stubLogger(),
	}
	exp := NewCostExporter(cfg)

	if exp.interval != 5*time.Second {
		t.Errorf("expected interval=5s, got %v", exp.interval)
	}
}

// TestCostExporter_ExportOnce_NilSource 测试 source 为 nil 时的行为
func TestCostExporter_ExportOnce_NilSource(t *testing.T) {
	exp := NewCostExporter(CostExporterConfig{
		Source: nil,
		Logger: stubLogger(),
	})

	if err := exp.ExportOnce(); err != nil {
		t.Errorf("expected no error for nil source, got %v", err)
	}
}

// TestCostExporter_ExportOnce 测试从 stub source 导入成本数据
func TestCostExporter_ExportOnce(t *testing.T) {
	src := &stubCostSource{
		summary: CostSourceSummary{
			ByModel: map[string]CostSourceModelCost{
				"gpt-4": {CostUSD: 0.05, Calls: 3, Tokens: 150},
			},
		},
		records: []CostSourceRecord{
			{Model: "gpt-4", AgentName: "agent-1", CostUSD: 0.025, TotalTokens: 75},
		},
	}

	exp := NewCostExporter(CostExporterConfig{
		Source: src,
		Logger: stubLogger(),
	})

	// 重置默认 metrics 以避免测试间干扰
	defaultMetrics.Reset()

	if err := exp.ExportOnce(); err != nil {
		t.Fatalf("ExportOnce failed: %v", err)
	}

	// 验证默认 metrics 中已存在 gpt-4 累计成本
	defaultMetrics.mu.RLock()
	lc, ok := defaultMetrics.CostByLabel["openai|gpt-4|"]
	defaultMetrics.mu.RUnlock()

	if !ok {
		t.Fatal("expected CostByLabel to have openai|gpt-4| entry")
	}

	cost := math.Float64frombits(lc.costBits.Load())
	if math.Abs(cost-0.05) > 1e-9 {
		t.Errorf("expected cost=0.05, got %v", cost)
	}
	if lc.calls.Load() != 3 {
		t.Errorf("expected calls=3, got %d", lc.calls.Load())
	}

	// 验证最后一次调用成本
	defaultMetrics.mu.RLock()
	last, ok := defaultMetrics.CostLastUSDByLabel["openai|gpt-4|agent-1"]
	defaultMetrics.mu.RUnlock()

	if !ok {
		t.Error("expected CostLastUSDByLabel to have openai|gpt-4|agent-1 entry")
	}
	if math.Abs(last-0.025) > 1e-9 {
		t.Errorf("expected last=0.025, got %v", last)
	}
}

// TestCostExporter_LastExportTime 测试 LastExportTime
func TestCostExporter_LastExportTime(t *testing.T) {
	src := &stubCostSource{}
	exp := NewCostExporter(CostExporterConfig{
		Source: src,
		Logger: stubLogger(),
	})

	if !exp.LastExportTime().IsZero() {
		t.Error("expected LastExportTime to be zero initially")
	}

	if err := exp.ExportOnce(); err != nil {
		t.Fatalf("ExportOnce failed: %v", err)
	}

	after := exp.LastExportTime()
	if after.IsZero() {
		t.Error("expected LastExportTime to be set after ExportOnce")
	}
}

// TestCostExporter_StartStop 测试生命周期
func TestCostExporter_StartStop(t *testing.T) {
	src := &stubCostSource{}
	exp := NewCostExporter(CostExporterConfig{
		Source:   src,
		Interval: 50 * time.Millisecond,
		Logger:   stubLogger(),
	})

	exp.Start()
	time.Sleep(120 * time.Millisecond) // 让 loop 至少跑一次
	exp.Stop()

	// 重复 Stop 应幂等
	exp.Stop()
}

// TestCostExporter_Start_AfterStop 测试 Stop 后 Start 是 no-op
func TestCostExporter_Start_AfterStop(t *testing.T) {
	src := &stubCostSource{}
	exp := NewCostExporter(CostExporterConfig{
		Source: src,
		Logger: stubLogger(),
	})

	exp.Stop()
	exp.Start() // 无效（stopped 已为 true）

	// 等待几毫秒确保即使有 loop 也不会发生 panic
	time.Sleep(50 * time.Millisecond)
}

// TestInferProviderFromModel 验证 provider 推断
func TestInferProviderFromModel(t *testing.T) {
	tests := []struct {
		model    string
		expected string
	}{
		{"gpt-4", "openai"},
		{"gpt-3.5-turbo", "openai"},
		{"claude-3-5-sonnet-20241022", "anthropic"},
		{"claude-3-opus-20240229", "anthropic"},
		{"gemini-1.5-pro", "google"},
		{"qwen-turbo", "alibaba"},
		{"deepseek-chat", "deepseek"},
		{"unknown-model", "unknown"},
		{"", "unknown"},
	}

	for _, tc := range tests {
		got := inferProviderFromModel(tc.model)
		if got != tc.expected {
			t.Errorf("inferProviderFromModel(%q) = %q, want %q", tc.model, got, tc.expected)
		}
	}
}

// TestCostExporter_Snapshot 测试快照字段
func TestCostExporter_Snapshot(t *testing.T) {
	src := &stubCostSource{}
	exp := NewCostExporter(CostExporterConfig{
		Source:   src,
		Interval: 30 * time.Second,
		Logger:   stubLogger(),
	})

	snap := exp.Snapshot()
	if !snap.Enabled {
		t.Error("expected Enabled=true initially")
	}
	if snap.Interval != "30s" {
		t.Errorf("expected Interval=30s, got %v", snap.Interval)
	}
	if snap.SourceAddr != "configured" {
		t.Errorf("expected SourceAddr=configured, got %v", snap.SourceAddr)
	}

	exp.Stop()
	snapStopped := exp.Snapshot()
	if snapStopped.Enabled {
		t.Error("expected Enabled=false after Stop")
	}
}

// TestCostExporter_Snapshot_NilSource 测试 nil source 的快照
func TestCostExporter_Snapshot_NilSource(t *testing.T) {
	exp := NewCostExporter(CostExporterConfig{
		Source: nil,
		Logger: stubLogger(),
	})

	snap := exp.Snapshot()
	if snap.SourceAddr != "<nil>" {
		t.Errorf("expected SourceAddr=<nil>, got %v", snap.SourceAddr)
	}
}

// TestCostExporter_ExportOnce_NoRecords 测试 source 有 summary 但无 records 的情况
func TestCostExporter_ExportOnce_NoRecords(t *testing.T) {
	src := &stubCostSource{
		summary: CostSourceSummary{
			ByModel: map[string]CostSourceModelCost{
				"claude-3-opus": {CostUSD: 0.10, Calls: 1, Tokens: 50},
			},
		},
		records: []CostSourceRecord{},
	}

	exp := NewCostExporter(CostExporterConfig{
		Source: src,
		Logger: stubLogger(),
	})

	// 在 ExportOnce 前 reset 避免测试间干扰
	defaultMetrics.Reset()

	if err := exp.ExportOnce(); err != nil {
		t.Fatalf("ExportOnce failed: %v", err)
	}

	defaultMetrics.mu.RLock()
	lc, ok := defaultMetrics.CostByLabel["anthropic|claude-3-opus|"]
	defaultMetrics.mu.RUnlock()

	if !ok {
		t.Fatal("expected cost entry for claude")
	}
	cost := math.Float64frombits(lc.costBits.Load())
	if math.Abs(cost-0.10) > 1e-9 {
		t.Errorf("expected cost=0.10, got %v", cost)
	}
}

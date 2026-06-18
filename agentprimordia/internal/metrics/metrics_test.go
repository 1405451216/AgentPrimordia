package metrics

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMetrics_RecordLLMCall_Success(t *testing.T) {
	m := NewMetrics()

	m.RecordLLMCall(100*time.Millisecond, nil)

	snap := m.Snapshot()
	if snap.LLMTotalCalls != 1 {
		t.Errorf("expected LLMTotalCalls=1, got %d", snap.LLMTotalCalls)
	}
	if snap.LLMTotalErrors != 0 {
		t.Errorf("expected LLMTotalErrors=0, got %d", snap.LLMTotalErrors)
	}
}

func TestMetrics_RecordLLMCall_Error(t *testing.T) {
	m := NewMetrics()

	m.RecordLLMCall(100*time.Millisecond, errTest)

	snap := m.Snapshot()
	if snap.LLMTotalCalls != 1 {
		t.Errorf("expected LLMTotalCalls=1, got %d", snap.LLMTotalCalls)
	}
	if snap.LLMTotalErrors != 1 {
		t.Errorf("expected LLMTotalErrors=1, got %d", snap.LLMTotalErrors)
	}
}

func TestMetrics_RecordToolCall(t *testing.T) {
	m := NewMetrics()

	m.RecordToolCall(50*time.Millisecond, nil)
	m.RecordToolCall(150*time.Millisecond, errTest)

	snap := m.Snapshot()
	if snap.ToolTotalCalls != 2 {
		t.Errorf("expected ToolTotalCalls=2, got %d", snap.ToolTotalCalls)
	}
	if snap.ToolTotalErrors != 1 {
		t.Errorf("expected ToolTotalErrors=1, got %d", snap.ToolTotalErrors)
	}
}

func TestMetrics_ActiveAgentsGauge(t *testing.T) {
	m := NewMetrics()

	m.IncActiveAgents()
	m.IncActiveAgents()
	m.DecActiveAgents()

	snap := m.Snapshot()
	if snap.ActiveAgents != 1 {
		t.Errorf("expected ActiveAgents=1, got %d", snap.ActiveAgents)
	}
}

func TestMetrics_Histogram_BasicDistribution(t *testing.T) {
	h := NewHistogram(defaultLatencyBuckets)

	h.Record(5)
	h.Record(50)
	h.Record(500)
	h.Record(5000)

	snap := h.Snapshot()
	if snap.Count != 4 {
		t.Errorf("expected Count=4, got %d", snap.Count)
	}
}

func TestMetrics_Histogram_MinMaxSum(t *testing.T) {
	h := NewHistogram(defaultLatencyBuckets)

	h.Record(10)
	h.Record(50)
	h.Record(100)

	snap := h.Snapshot()
	if snap.Min != 10 {
		t.Errorf("expected Min=10, got %d", snap.Min)
	}
	if snap.Max != 100 {
		t.Errorf("expected Max=100, got %d", snap.Max)
	}
	if snap.Sum != 160 {
		t.Errorf("expected Sum=160, got %d", snap.Sum)
	}
}

func TestMetrics_Snapshot(t *testing.T) {
	m := NewMetrics()

	m.RecordLLMCall(100*time.Millisecond, nil)
	m.IncActiveAgents()

	snap1 := m.Snapshot()

	m.RecordLLMCall(200*time.Millisecond, nil)
	m.IncActiveAgents()

	snap2 := m.Snapshot()

	if snap1.LLMTotalCalls != 1 {
		t.Errorf("snap1: expected LLMTotalCalls=1, got %d", snap1.LLMTotalCalls)
	}
	if snap2.LLMTotalCalls != 2 {
		t.Errorf("snap2: expected LLMTotalCalls=2, got %d", snap2.LLMTotalCalls)
	}
	if snap1.ActiveAgents != 1 {
		t.Errorf("snap1: expected ActiveAgents=1, got %d", snap1.ActiveAgents)
	}
	if snap2.ActiveAgents != 2 {
		t.Errorf("snap2: expected ActiveAgents=2, got %d", snap2.ActiveAgents)
	}
}

func TestMetrics_Reset(t *testing.T) {
	m := NewMetrics()

	m.RecordLLMCall(100*time.Millisecond, nil)
	m.RecordToolCall(50*time.Millisecond, nil)
	m.IncActiveAgents()
	m.RecordTurn(200 * time.Millisecond)

	m.Reset()

	snap := m.Snapshot()
	if snap.LLMTotalCalls != 0 {
		t.Errorf("expected LLMTotalCalls=0, got %d", snap.LLMTotalCalls)
	}
	if snap.ToolTotalCalls != 0 {
		t.Errorf("expected ToolTotalCalls=0, got %d", snap.ToolTotalCalls)
	}
	if snap.ActiveAgents != 0 {
		t.Errorf("expected ActiveAgents=0, got %d", snap.ActiveAgents)
	}
	if snap.TotalTurns != 0 {
		t.Errorf("expected TotalTurns=0, got %d", snap.TotalTurns)
	}
}

func TestMetrics_StringFormat(t *testing.T) {
	m := NewMetrics()

	m.RecordLLMCall(100*time.Millisecond, nil)
	m.IncActiveAgents()

	output := m.String()

	if !strings.Contains(output, "ap_llm_total_calls 1") {
		t.Error("expected ap_llm_total_calls 1 in output")
	}
	if !strings.Contains(output, "ap_active_agents 1") {
		t.Error("expected ap_active_agents 1 in output")
	}
	if !strings.Contains(output, "# TYPE ap_llm_latency_ms histogram") {
		t.Error("expected histogram type in output")
	}
}

func TestMetrics_ConcurrentRecording(t *testing.T) {
	m := NewMetrics()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			m.RecordLLMCall(10*time.Millisecond, nil)
		}()
		go func() {
			defer wg.Done()
			m.RecordToolCall(5*time.Millisecond, nil)
		}()
	}

	wg.Wait()

	snap := m.Snapshot()
	if snap.LLMTotalCalls != 100 {
		t.Errorf("expected LLMTotalCalls=100, got %d", snap.LLMTotalCalls)
	}
	if snap.ToolTotalCalls != 100 {
		t.Errorf("expected ToolTotalCalls=100, got %d", snap.ToolTotalCalls)
	}
}

func TestMetrics_PoolAndMemory(t *testing.T) {
	m := NewMetrics()

	m.SetPoolQueue(5)
	m.SetMemorySize(1024)

	snap := m.Snapshot()
	if snap.PoolQueueLength != 5 {
		t.Errorf("expected PoolQueueLength=5, got %d", snap.PoolQueueLength)
	}
	if snap.MemorySizeBytes != 1024 {
		t.Errorf("expected MemorySizeBytes=1024, got %d", snap.MemorySizeBytes)
	}
}

var errTest = &testError{"test error"}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

// ===== 新增测试：提升覆盖率到 70%+ =====

// mockTelemetryExporter 模拟 TelemetryExporter 接口
type mockTelemetryExporter struct {
	mu             sync.Mutex
	metricsExports int
	eventExports   int
	closed         bool
}

func (m *mockTelemetryExporter) ExportMetrics(snapshot MetricsSnapshot) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.metricsExports++
}

func (m *mockTelemetryExporter) ExportEvent(eventType string, source string, payload any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.eventExports++
}

func (m *mockTelemetryExporter) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

// TestRecordLLMCallWithLabels 测试带标签的 LLM 调用记录
func TestRecordLLMCallWithLabels(t *testing.T) {
	m := NewMetrics()

	// 测试基本调用（成功）
	m.RecordLLMCallWithLabels(100*time.Millisecond, nil, "openai", "gpt-4")

	m.mu.RLock()
	counter, ok := m.LLMCallsByLabel["openai|gpt-4"]
	m.mu.RUnlock()

	if !ok {
		t.Fatal("expected LLMCallsByLabel to contain 'openai|gpt-4'")
	}
	if counter.calls.Load() != 1 {
		t.Errorf("expected calls=1, got %d", counter.calls.Load())
	}
	if counter.errors.Load() != 0 {
		t.Errorf("expected errors=0, got %d", counter.errors.Load())
	}

	// 测试错误调用
	m.RecordLLMCallWithLabels(200*time.Millisecond, errTest, "openai", "gpt-4")

	if counter.calls.Load() != 2 {
		t.Errorf("expected calls=2, got %d", counter.calls.Load())
	}
	if counter.errors.Load() != 1 {
		t.Errorf("expected errors=1, got %d", counter.errors.Load())
	}

	// 测试多个不同标签
	m.RecordLLMCallWithLabels(150*time.Millisecond, nil, "anthropic", "claude-3")

	m.mu.RLock()
	counter2, ok := m.LLMCallsByLabel["anthropic|claude-3"]
	m.mu.RUnlock()

	if !ok {
		t.Fatal("expected LLMCallsByLabel to contain 'anthropic|claude-3'")
	}
	if counter2.calls.Load() != 1 {
		t.Errorf("expected calls=1, got %d", counter2.calls.Load())
	}

	// 验证全局计数器也被更新
	snap := m.Snapshot()
	if snap.LLMTotalCalls != 3 {
		t.Errorf("expected LLMTotalCalls=3, got %d", snap.LLMTotalCalls)
	}
	if snap.LLMTotalErrors != 1 {
		t.Errorf("expected LLMTotalErrors=1, got %d", snap.LLMTotalErrors)
	}
}

// TestRecordLLMCallWithLabels_MapLazyInit 测试 map 延迟初始化
func TestRecordLLMCallWithLabels_MapLazyInit(t *testing.T) {
	m := NewMetrics()

	// 初始时 map 应该为 nil
	m.mu.RLock()
	if m.LLMCallsByLabel != nil {
		t.Error("expected LLMCallsByLabel to be nil initially")
	}
	m.mu.RUnlock()

	// 第一次调用后应该被初始化
	m.RecordLLMCallWithLabels(100*time.Millisecond, nil, "test", "model")

	m.mu.RLock()
	if m.LLMCallsByLabel == nil {
		t.Error("expected LLMCallsByLabel to be initialized after first call")
	}
	m.mu.RUnlock()
}

// TestRecordToolCallWithLabels 测试带标签的工具调用记录
func TestRecordToolCallWithLabels(t *testing.T) {
	m := NewMetrics()

	// 测试基本调用（成功）
	m.RecordToolCallWithLabels(50*time.Millisecond, nil, "search")

	m.mu.RLock()
	counter, ok := m.ToolCallsByLabel["search"]
	m.mu.RUnlock()

	if !ok {
		t.Fatal("expected ToolCallsByLabel to contain 'search'")
	}
	if counter.calls.Load() != 1 {
		t.Errorf("expected calls=1, got %d", counter.calls.Load())
	}
	if counter.errors.Load() != 0 {
		t.Errorf("expected errors=0, got %d", counter.errors.Load())
	}

	// 测试错误调用
	m.RecordToolCallWithLabels(100*time.Millisecond, errTest, "search")

	if counter.calls.Load() != 2 {
		t.Errorf("expected calls=2, got %d", counter.calls.Load())
	}
	if counter.errors.Load() != 1 {
		t.Errorf("expected errors=1, got %d", counter.errors.Load())
	}

	// 测试多个不同工具
	m.RecordToolCallWithLabels(75*time.Millisecond, nil, "execute")

	m.mu.RLock()
	counter2, ok := m.ToolCallsByLabel["execute"]
	m.mu.RUnlock()

	if !ok {
		t.Fatal("expected ToolCallsByLabel to contain 'execute'")
	}
	if counter2.calls.Load() != 1 {
		t.Errorf("expected calls=1, got %d", counter2.calls.Load())
	}

	// 验证全局计数器也被更新
	snap := m.Snapshot()
	if snap.ToolTotalCalls != 3 {
		t.Errorf("expected ToolTotalCalls=3, got %d", snap.ToolTotalCalls)
	}
	if snap.ToolTotalErrors != 1 {
		t.Errorf("expected ToolTotalErrors=1, got %d", snap.ToolTotalErrors)
	}
}

// TestRecordTurnWithAgent 测试带 agent 名称的 Turn 记录
func TestRecordTurnWithAgent(t *testing.T) {
	m := NewMetrics()

	// 测试基本调用
	m.RecordTurnWithAgent(500*time.Millisecond, "agent-1")

	m.mu.RLock()
	counter, ok := m.TurnsByAgent["agent-1"]
	m.mu.RUnlock()

	if !ok {
		t.Fatal("expected TurnsByAgent to contain 'agent-1'")
	}
	if counter.calls.Load() != 1 {
		t.Errorf("expected calls=1, got %d", counter.calls.Load())
	}

	// 测试多个不同 agent
	m.RecordTurnWithAgent(600*time.Millisecond, "agent-2")
	m.RecordTurnWithAgent(700*time.Millisecond, "agent-1")

	m.mu.RLock()
	counter2, ok := m.TurnsByAgent["agent-2"]
	m.mu.RUnlock()

	if !ok {
		t.Fatal("expected TurnsByAgent to contain 'agent-2'")
	}
	if counter2.calls.Load() != 1 {
		t.Errorf("expected calls=1, got %d", counter2.calls.Load())
	}

	if counter.calls.Load() != 2 {
		t.Errorf("expected agent-1 calls=2, got %d", counter.calls.Load())
	}

	// 验证全局 TotalTurns 也被更新
	snap := m.Snapshot()
	if snap.TotalTurns != 3 {
		t.Errorf("expected TotalTurns=3, got %d", snap.TotalTurns)
	}
}

// TestRecordTokenUsage 测试 Token 使用量记录
func TestRecordTokenUsage(t *testing.T) {
	m := NewMetrics()

	// 测试基本调用
	m.RecordTokenUsage("gpt-4", 100, 50)

	m.mu.RLock()
	stats, ok := m.TokenUsageByModel["gpt-4"]
	m.mu.RUnlock()

	if !ok {
		t.Fatal("expected TokenUsageByModel to contain 'gpt-4'")
	}
	if stats.PromptTokens != 100 {
		t.Errorf("expected PromptTokens=100, got %d", stats.PromptTokens)
	}
	if stats.CompletionTokens != 50 {
		t.Errorf("expected CompletionTokens=50, got %d", stats.CompletionTokens)
	}
	if stats.TotalTokens != 150 {
		t.Errorf("expected TotalTokens=150, got %d", stats.TotalTokens)
	}
	if stats.Calls != 1 {
		t.Errorf("expected Calls=1, got %d", stats.Calls)
	}

	// 测试累加
	m.RecordTokenUsage("gpt-4", 200, 100)

	if stats.PromptTokens != 300 {
		t.Errorf("expected PromptTokens=300, got %d", stats.PromptTokens)
	}
	if stats.CompletionTokens != 150 {
		t.Errorf("expected CompletionTokens=150, got %d", stats.CompletionTokens)
	}
	if stats.TotalTokens != 450 {
		t.Errorf("expected TotalTokens=450, got %d", stats.TotalTokens)
	}
	if stats.Calls != 2 {
		t.Errorf("expected Calls=2, got %d", stats.Calls)
	}

	// 测试多个模型
	m.RecordTokenUsage("claude-3", 150, 75)

	m.mu.RLock()
	stats2, ok := m.TokenUsageByModel["claude-3"]
	m.mu.RUnlock()

	if !ok {
		t.Fatal("expected TokenUsageByModel to contain 'claude-3'")
	}
	if stats2.PromptTokens != 150 {
		t.Errorf("expected PromptTokens=150, got %d", stats2.PromptTokens)
	}
	if stats2.Calls != 1 {
		t.Errorf("expected Calls=1, got %d", stats2.Calls)
	}
}

// TestString_WithLabels 测试 String() 输出包含标签维度
func TestString_WithLabels(t *testing.T) {
	m := NewMetrics()

	m.RecordLLMCallWithLabels(100*time.Millisecond, nil, "openai", "gpt-4")
	m.RecordLLMCallWithLabels(200*time.Millisecond, errTest, "openai", "gpt-4")
	m.RecordToolCallWithLabels(50*time.Millisecond, nil, "search")
	m.RecordTurnWithAgent(500*time.Millisecond, "agent-1")

	output := m.String()

	// 验证 LLM 标签输出
	if !strings.Contains(output, `ap_llm_calls_by_provider{provider="openai",model="gpt-4"}`) {
		t.Error("expected ap_llm_calls_by_provider with openai|gpt-4 labels")
	}
	if !strings.Contains(output, `ap_llm_errors_by_provider{provider="openai",model="gpt-4"}`) {
		t.Error("expected ap_llm_errors_by_provider with openai|gpt-4 labels")
	}

	// 验证工具标签输出
	if !strings.Contains(output, `ap_tool_calls{tool_name="search"}`) {
		t.Error("expected ap_tool_calls with tool_name=search label")
	}

	// 验证 agent turn 标签输出
	if !strings.Contains(output, `ap_turns{agent_name="agent-1"}`) {
		t.Error("expected ap_turns with agent_name=agent-1 label")
	}
}

// TestPrometheusHandler_StartStop 测试 PrometheusHandler 的启动和停止
func TestPrometheusHandler_StartStop(t *testing.T) {
	m := NewMetrics()
	m.RecordLLMCall(100*time.Millisecond, nil)

	// 使用 httptest 测试 handler 功能
	handler := NewPrometheusHandler(m, "")

	// 测试 /metrics 端点
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	handler.handleMetrics(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "ap_llm_total_calls") {
		t.Error("expected /metrics to contain ap_llm_total_calls")
	}

	// 测试 /health 端点
	req2 := httptest.NewRequest(http.MethodGet, "/health", nil)
	w2 := httptest.NewRecorder()
	handler.handleHealth(w2, req2)

	resp2 := w2.Result()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp2.StatusCode)
	}

	body2, _ := io.ReadAll(resp2.Body)
	if !strings.Contains(string(body2), `{"status":"ok"}`) {
		t.Error("expected /health to return {\"status\":\"ok\"}")
	}

	// 测试方法不允许
	req3 := httptest.NewRequest(http.MethodPost, "/metrics", nil)
	w3 := httptest.NewRecorder()
	handler.handleMetrics(w3, req3)

	if w3.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405 for POST, got %d", w3.Code)
	}
}

// TestMultiExporter_Add 测试动态添加导出器
func TestMultiExporter_Add(t *testing.T) {
	mock1 := &mockTelemetryExporter{}
	mock2 := &mockTelemetryExporter{}

	multi := NewMultiExporter(mock1)

	// 初始时只有一个导出器
	multi.ExportMetrics(MetricsSnapshot{})

	mock1.mu.Lock()
	if mock1.metricsExports != 1 {
		t.Errorf("expected mock1 metricsExports=1, got %d", mock1.metricsExports)
	}
	mock1.mu.Unlock()

	// 动态添加第二个导出器
	multi.Add(mock2)
	multi.ExportMetrics(MetricsSnapshot{})

	mock1.mu.Lock()
	if mock1.metricsExports != 2 {
		t.Errorf("expected mock1 metricsExports=2, got %d", mock1.metricsExports)
	}
	mock1.mu.Unlock()

	mock2.mu.Lock()
	if mock2.metricsExports != 1 {
		t.Errorf("expected mock2 metricsExports=1, got %d", mock2.metricsExports)
	}
	mock2.mu.Unlock()

	// 测试事件导出
	multi.ExportEvent("test", "source", nil)

	mock1.mu.Lock()
	if mock1.eventExports != 1 {
		t.Errorf("expected mock1 eventExports=1, got %d", mock1.eventExports)
	}
	mock1.mu.Unlock()

	mock2.mu.Lock()
	if mock2.eventExports != 1 {
		t.Errorf("expected mock2 eventExports=1, got %d", mock2.eventExports)
	}
	mock2.mu.Unlock()

	// 测试关闭
	err := multi.Close()
	if err != nil {
		t.Errorf("unexpected error on close: %v", err)
	}

	mock1.mu.Lock()
	if !mock1.closed {
		t.Error("expected mock1 to be closed")
	}
	mock1.mu.Unlock()

	mock2.mu.Lock()
	if !mock2.closed {
		t.Error("expected mock2 to be closed")
	}
	mock2.mu.Unlock()
}

// TestMetrics_Reset_ClearsLabels 测试 Reset 清除标签 map
func TestMetrics_Reset_ClearsLabels(t *testing.T) {
	m := NewMetrics()

	// 添加一些标签数据
	m.RecordLLMCallWithLabels(100*time.Millisecond, nil, "openai", "gpt-4")
	m.RecordToolCallWithLabels(50*time.Millisecond, nil, "search")
	m.RecordTurnWithAgent(500*time.Millisecond, "agent-1")
	m.RecordTokenUsage("gpt-4", 100, 50)

	// 验证数据存在
	m.mu.RLock()
	if len(m.LLMCallsByLabel) == 0 {
		t.Error("expected LLMCallsByLabel to have data before reset")
	}
	if len(m.ToolCallsByLabel) == 0 {
		t.Error("expected ToolCallsByLabel to have data before reset")
	}
	if len(m.TurnsByAgent) == 0 {
		t.Error("expected TurnsByAgent to have data before reset")
	}
	if len(m.TokenUsageByModel) == 0 {
		t.Error("expected TokenUsageByModel to have data before reset")
	}
	m.mu.RUnlock()

	// 执行 Reset
	m.Reset()

	// 验证所有标签 map 被清除
	m.mu.RLock()
	if m.LLMCallsByLabel != nil {
		t.Error("expected LLMCallsByLabel to be nil after reset")
	}
	if m.ToolCallsByLabel != nil {
		t.Error("expected ToolCallsByLabel to be nil after reset")
	}
	if m.TurnsByAgent != nil {
		t.Error("expected TurnsByAgent to be nil after reset")
	}
	if m.TokenUsageByModel != nil {
		t.Error("expected TokenUsageByModel to be nil after reset")
	}
	m.mu.RUnlock()

	// 验证全局计数器也被清除
	snap := m.Snapshot()
	if snap.LLMTotalCalls != 0 {
		t.Errorf("expected LLMTotalCalls=0 after reset, got %d", snap.LLMTotalCalls)
	}
	if snap.ToolTotalCalls != 0 {
		t.Errorf("expected ToolTotalCalls=0 after reset, got %d", snap.ToolTotalCalls)
	}
	if snap.TotalTurns != 0 {
		t.Errorf("expected TotalTurns=0 after reset, got %d", snap.TotalTurns)
	}
}

// TestRecordLLMCallWithLabels_Concurrent 测试并发安全性
func TestRecordLLMCallWithLabels_Concurrent(t *testing.T) {
	m := NewMetrics()

	var wg sync.WaitGroup
	// 并发调用不同的标签
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			provider := fmt.Sprintf("provider-%d", id%5)
			model := fmt.Sprintf("model-%d", id%3)
			m.RecordLLMCallWithLabels(10*time.Millisecond, nil, provider, model)
		}(i)
	}

	wg.Wait()

	// 验证总调用次数
	snap := m.Snapshot()
	if snap.LLMTotalCalls != 50 {
		t.Errorf("expected LLMTotalCalls=50, got %d", snap.LLMTotalCalls)
	}

	// 验证标签 map 的总调用次数
	m.mu.RLock()
	totalFromLabels := int64(0)
	for _, counter := range m.LLMCallsByLabel {
		totalFromLabels += counter.calls.Load()
	}
	m.mu.RUnlock()

	if totalFromLabels != 50 {
		t.Errorf("expected total from labels=50, got %d", totalFromLabels)
	}
}

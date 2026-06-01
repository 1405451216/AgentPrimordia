package metrics

import (
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

package metrics

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const maxBucketValue = float64(^uint64(0) >> 1)

type AgentMetrics struct {
	mu sync.RWMutex

	LLMTotalCalls   int64
	LLMTotalErrors  int64
	ToolTotalCalls  int64
	ToolTotalErrors int64
	TotalTurns      int64
	TotalEpisodes   int64

	LLMLatencyMs   *Histogram
	ToolLatencyMs  *Histogram
	TurnDurationMs *Histogram

	ActiveAgents    int64
	PoolQueueLength int64
	MemorySizeBytes int64

	TokenUsageByModel map[string]*TokenUsageStats

	// 按标签维度追踪的计数器（用于 Grafana 多维查询）
	LLMCallsByLabel  map[string]*labeledCounter // key: "provider|model"
	ToolCallsByLabel map[string]*labeledCounter // key: "tool_name"
	TurnsByAgent     map[string]*labeledCounter // key: "agent_name"
}

type labeledCounter struct {
	mu     sync.Mutex
	calls  int64
	errors int64
}

type TokenUsageStats struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
	Calls            int64 `json:"calls"`
}

type Histogram struct {
	mu      sync.Mutex
	buckets []float64
	counts  []int64
	sum     int64
	count   int64
	min     int64
	max     int64
}

var defaultLatencyBuckets = []float64{
	1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000, maxBucketValue,
}

var defaultTurnBuckets = []float64{
	100, 500, 1000, 2000, 5000, 10000, 30000, 60000, 120000, 300000, maxBucketValue,
}

func NewHistogram(buckets []float64) *Histogram {
	return &Histogram{
		buckets: buckets,
		counts:  make([]int64, len(buckets)),
		min:     int64(^uint64(0) >> 1),
	}
}

func (h *Histogram) Record(valueMs int64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.sum += valueMs
	h.count++

	if valueMs < h.min {
		h.min = valueMs
	}
	if valueMs > h.max {
		h.max = valueMs
	}

	for i, bucket := range h.buckets {
		if float64(valueMs) <= bucket {
			h.counts[i]++
			break
		}
	}
}

func (h *Histogram) Snapshot() HistogramSnapshot {
	h.mu.Lock()
	defer h.mu.Unlock()

	counts := make([]int64, len(h.counts))
	copy(counts, h.counts)

	return HistogramSnapshot{
		Buckets: h.buckets,
		Counts:  counts,
		Sum:     h.sum,
		Count:   h.count,
		Min:     h.min,
		Max:     h.max,
	}
}

func (h *Histogram) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()

	for i := range h.counts {
		h.counts[i] = 0
	}
	h.sum = 0
	h.count = 0
	h.min = int64(^uint64(0) >> 1)
	h.max = 0
}

type HistogramSnapshot struct {
	Buckets []float64
	Counts  []int64
	Sum     int64
	Count   int64
	Min     int64
	Max     int64
}

type MetricsSnapshot struct {
	LLMTotalCalls   int64
	LLMTotalErrors  int64
	ToolTotalCalls  int64
	ToolTotalErrors int64
	TotalTurns      int64
	TotalEpisodes   int64

	LLMLatencyMs   HistogramSnapshot
	ToolLatencyMs  HistogramSnapshot
	TurnDurationMs HistogramSnapshot

	ActiveAgents    int64
	PoolQueueLength int64
	MemorySizeBytes int64
}

func NewMetrics() *AgentMetrics {
	return &AgentMetrics{
		LLMLatencyMs:   NewHistogram(defaultLatencyBuckets),
		ToolLatencyMs:  NewHistogram(defaultLatencyBuckets),
		TurnDurationMs: NewHistogram(defaultTurnBuckets),
	}
}

func (m *AgentMetrics) RecordLLMCall(duration time.Duration, err error) {
	atomic.AddInt64(&m.LLMTotalCalls, 1)
	if err != nil {
		atomic.AddInt64(&m.LLMTotalErrors, 1)
	}
	m.LLMLatencyMs.Record(duration.Milliseconds())
}

// RecordLLMCallWithLabels 记录带标签维度的 LLM 调用（供 Grafana 多维查询）
func (m *AgentMetrics) RecordLLMCallWithLabels(duration time.Duration, err error, provider, model string) {
	m.RecordLLMCall(duration, err)

	key := provider + "|" + model
	m.mu.RLock()
	counter, ok := m.LLMCallsByLabel[key]
	m.mu.RUnlock()

	if !ok {
		m.mu.Lock()
		if m.LLMCallsByLabel == nil {
			m.LLMCallsByLabel = make(map[string]*labeledCounter)
		}
		counter, ok = m.LLMCallsByLabel[key]
		if !ok {
			counter = &labeledCounter{}
			m.LLMCallsByLabel[key] = counter
		}
		m.mu.Unlock()
	}

	counter.mu.Lock()
	counter.calls++
	if err != nil {
		counter.errors++
	}
	counter.mu.Unlock()
}

func (m *AgentMetrics) RecordToolCall(duration time.Duration, err error) {
	atomic.AddInt64(&m.ToolTotalCalls, 1)
	if err != nil {
		atomic.AddInt64(&m.ToolTotalErrors, 1)
	}
	m.ToolLatencyMs.Record(duration.Milliseconds())
}

// RecordToolCallWithLabels 记录带标签维度的工具调用
func (m *AgentMetrics) RecordToolCallWithLabels(duration time.Duration, err error, toolName string) {
	m.RecordToolCall(duration, err)

	m.mu.RLock()
	counter, ok := m.ToolCallsByLabel[toolName]
	m.mu.RUnlock()

	if !ok {
		m.mu.Lock()
		if m.ToolCallsByLabel == nil {
			m.ToolCallsByLabel = make(map[string]*labeledCounter)
		}
		counter, ok = m.ToolCallsByLabel[toolName]
		if !ok {
			counter = &labeledCounter{}
			m.ToolCallsByLabel[toolName] = counter
		}
		m.mu.Unlock()
	}

	counter.mu.Lock()
	counter.calls++
	if err != nil {
		counter.errors++
	}
	counter.mu.Unlock()
}

// RecordTurnWithAgent 记录带 agent_name 标签的 Turn
func (m *AgentMetrics) RecordTurnWithAgent(duration time.Duration, agentName string) {
	m.RecordTurn(duration)

	m.mu.RLock()
	counter, ok := m.TurnsByAgent[agentName]
	m.mu.RUnlock()

	if !ok {
		m.mu.Lock()
		if m.TurnsByAgent == nil {
			m.TurnsByAgent = make(map[string]*labeledCounter)
		}
		counter, ok = m.TurnsByAgent[agentName]
		if !ok {
			counter = &labeledCounter{}
			m.TurnsByAgent[agentName] = counter
		}
		m.mu.Unlock()
	}

	counter.mu.Lock()
	counter.calls++
	counter.mu.Unlock()
}

func (m *AgentMetrics) RecordTurn(duration time.Duration) {
	atomic.AddInt64(&m.TotalTurns, 1)
	m.TurnDurationMs.Record(duration.Milliseconds())
}

func (m *AgentMetrics) IncActiveAgents() {
	atomic.AddInt64(&m.ActiveAgents, 1)
}

func (m *AgentMetrics) DecActiveAgents() {
	atomic.AddInt64(&m.ActiveAgents, -1)
}

// RecordTokenUsage 记录 Token 使用量
// 使用 mutex 而非 atomic：因为 TokenUsageByModel 是 map 类型，需要整体加锁保护，
// 而 TokenUsageStats 内部字段在锁保护下直接操作，无需额外的 atomic 开销
func (m *AgentMetrics) RecordTokenUsage(model string, promptTokens, completionTokens int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.TokenUsageByModel == nil {
		m.TokenUsageByModel = make(map[string]*TokenUsageStats)
	}
	stats, ok := m.TokenUsageByModel[model]
	if !ok {
		stats = &TokenUsageStats{}
		m.TokenUsageByModel[model] = stats
	}
	stats.PromptTokens += int64(promptTokens)
	stats.CompletionTokens += int64(completionTokens)
	stats.TotalTokens += int64(promptTokens + completionTokens)
	stats.Calls++
}

func (m *AgentMetrics) SetPoolQueue(n int64) {
	atomic.StoreInt64(&m.PoolQueueLength, n)
}

func (m *AgentMetrics) SetMemorySize(bytes int64) {
	atomic.StoreInt64(&m.MemorySizeBytes, bytes)
}

func (m *AgentMetrics) Snapshot() MetricsSnapshot {
	return MetricsSnapshot{
		LLMTotalCalls:   atomic.LoadInt64(&m.LLMTotalCalls),
		LLMTotalErrors:  atomic.LoadInt64(&m.LLMTotalErrors),
		ToolTotalCalls:  atomic.LoadInt64(&m.ToolTotalCalls),
		ToolTotalErrors: atomic.LoadInt64(&m.ToolTotalErrors),
		TotalTurns:      atomic.LoadInt64(&m.TotalTurns),
		TotalEpisodes:   atomic.LoadInt64(&m.TotalEpisodes),
		LLMLatencyMs:    m.LLMLatencyMs.Snapshot(),
		ToolLatencyMs:   m.ToolLatencyMs.Snapshot(),
		TurnDurationMs:  m.TurnDurationMs.Snapshot(),
		ActiveAgents:    atomic.LoadInt64(&m.ActiveAgents),
		PoolQueueLength: atomic.LoadInt64(&m.PoolQueueLength),
		MemorySizeBytes: atomic.LoadInt64(&m.MemorySizeBytes),
	}
}

func (m *AgentMetrics) Reset() {
	atomic.StoreInt64(&m.LLMTotalCalls, 0)
	atomic.StoreInt64(&m.LLMTotalErrors, 0)
	atomic.StoreInt64(&m.ToolTotalCalls, 0)
	atomic.StoreInt64(&m.ToolTotalErrors, 0)
	atomic.StoreInt64(&m.TotalTurns, 0)
	atomic.StoreInt64(&m.TotalEpisodes, 0)
	atomic.StoreInt64(&m.ActiveAgents, 0)
	atomic.StoreInt64(&m.PoolQueueLength, 0)
	atomic.StoreInt64(&m.MemorySizeBytes, 0)

	m.LLMLatencyMs.Reset()
	m.ToolLatencyMs.Reset()
	m.TurnDurationMs.Reset()

	m.mu.Lock()
	m.TokenUsageByModel = nil
	m.LLMCallsByLabel = nil
	m.ToolCallsByLabel = nil
	m.TurnsByAgent = nil
	m.mu.Unlock()
}

func (m *AgentMetrics) String() string {
	snap := m.Snapshot()

	var sb strings.Builder

	sb.WriteString("# HELP ap_llm_total_calls Total LLM API calls\n")
	sb.WriteString("# TYPE ap_llm_total_calls counter\n")
	sb.WriteString(fmt.Sprintf("ap_llm_total_calls %d\n", snap.LLMTotalCalls))

	// 按 provider/model 维度输出
	m.mu.RLock()
	llmLabels := make(map[string]*labeledCounter, len(m.LLMCallsByLabel))
	for k, v := range m.LLMCallsByLabel {
		llmLabels[k] = v
	}
	toolLabels := make(map[string]*labeledCounter, len(m.ToolCallsByLabel))
	for k, v := range m.ToolCallsByLabel {
		toolLabels[k] = v
	}
	agentTurns := make(map[string]*labeledCounter, len(m.TurnsByAgent))
	for k, v := range m.TurnsByAgent {
		agentTurns[k] = v
	}
	m.mu.RUnlock()

	if len(llmLabels) > 0 {
		for key, counter := range llmLabels {
			parts := strings.SplitN(key, "|", 2)
			provider, model := parts[0], ""
			if len(parts) > 1 {
				model = parts[1]
			}
			counter.mu.Lock()
			calls, errors := counter.calls, counter.errors
			counter.mu.Unlock()
			sb.WriteString(fmt.Sprintf("ap_llm_calls_by_provider{provider=\"%s\",model=\"%s\"} %d\n", provider, model, calls))
			if errors > 0 {
				sb.WriteString(fmt.Sprintf("ap_llm_errors_by_provider{provider=\"%s\",model=\"%s\"} %d\n", provider, model, errors))
			}
		}
	}

	sb.WriteString("# HELP ap_llm_total_errors Total LLM API errors\n")
	sb.WriteString("# TYPE ap_llm_total_errors counter\n")
	sb.WriteString(fmt.Sprintf("ap_llm_total_errors %d\n", snap.LLMTotalErrors))

	sb.WriteString("# HELP ap_tool_total_calls Total tool calls\n")
	sb.WriteString("# TYPE ap_tool_total_calls counter\n")
	sb.WriteString(fmt.Sprintf("ap_tool_total_calls %d\n", snap.ToolTotalCalls))

	if len(toolLabels) > 0 {
		for toolName, counter := range toolLabels {
			counter.mu.Lock()
			calls, errors := counter.calls, counter.errors
			counter.mu.Unlock()
			sb.WriteString(fmt.Sprintf("ap_tool_calls{tool_name=\"%s\"} %d\n", toolName, calls))
			if errors > 0 {
				sb.WriteString(fmt.Sprintf("ap_tool_errors{tool_name=\"%s\"} %d\n", toolName, errors))
			}
		}
	}

	sb.WriteString("# HELP ap_tool_total_errors Total tool errors\n")
	sb.WriteString("# TYPE ap_tool_total_errors counter\n")
	sb.WriteString(fmt.Sprintf("ap_tool_total_errors %d\n", snap.ToolTotalErrors))

	sb.WriteString("# HELP ap_total_turns Total agent turns\n")
	sb.WriteString("# TYPE ap_total_turns counter\n")
	sb.WriteString(fmt.Sprintf("ap_total_turns %d\n", snap.TotalTurns))

	if len(agentTurns) > 0 {
		for agentName, counter := range agentTurns {
			counter.mu.Lock()
			calls := counter.calls
			counter.mu.Unlock()
			sb.WriteString(fmt.Sprintf("ap_turns{agent_name=\"%s\"} %d\n", agentName, calls))
		}
	}

	sb.WriteString("# HELP ap_active_agents Currently active agents\n")
	sb.WriteString("# TYPE ap_active_agents gauge\n")
	sb.WriteString(fmt.Sprintf("ap_active_agents %d\n", snap.ActiveAgents))

	sb.WriteString("# HELP ap_pool_queue_length Current pool queue length\n")
	sb.WriteString("# TYPE ap_pool_queue_length gauge\n")
	sb.WriteString(fmt.Sprintf("ap_pool_queue_length %d\n", snap.PoolQueueLength))

	sb.WriteString("# HELP ap_memory_size_bytes Memory store size in bytes\n")
	sb.WriteString("# TYPE ap_memory_size_bytes gauge\n")
	sb.WriteString(fmt.Sprintf("ap_memory_size_bytes %d\n", snap.MemorySizeBytes))

	writeHistogram(&sb, "ap_llm_latency_ms", snap.LLMLatencyMs)
	writeHistogram(&sb, "ap_tool_latency_ms", snap.ToolLatencyMs)
	writeHistogram(&sb, "ap_turn_duration_ms", snap.TurnDurationMs)

	return sb.String()
}

func writeHistogram(sb *strings.Builder, name string, h HistogramSnapshot) {
	sb.WriteString(fmt.Sprintf("# HELP %s %s histogram\n", name, name))
	sb.WriteString(fmt.Sprintf("# TYPE %s histogram\n", name))

	for i, bucket := range h.Buckets {
		if i == len(h.Buckets)-1 {
			sb.WriteString(fmt.Sprintf("%s_bucket{le=\"+Inf\"} %d\n", name, h.Counts[i]))
		} else {
			sb.WriteString(fmt.Sprintf("%s_bucket{le=\"%g\"} %d\n", name, bucket, h.Counts[i]))
		}
	}

	sb.WriteString(fmt.Sprintf("%s_sum %d\n", name, h.Sum))
	sb.WriteString(fmt.Sprintf("%s_count %d\n", name, h.Count))
}

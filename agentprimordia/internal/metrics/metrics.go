package metrics

import (
	"math"
	"sort"
	"strconv"
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

	// 成本追踪（按 provider|model|agent 维度聚合）
	// CostByLabel 累计成本（USD）
	CostByLabel map[string]*labeledCost // key: "provider|model|agent"
	// CostTokensByLabel 累计 token 数（按 kind 拆分：prompt/completion/total）
	CostTokensByLabel map[string]*labeledCost // key: "provider|model|agent|kind"
	// CostLastUSD 最近一次调用成本（Gauge 语义）
	CostLastUSDByLabel map[string]float64 // key: "provider|model|agent"
}

type labeledCounter struct {
	calls  atomic.Int64
	errors atomic.Int64
}

// labeledCost 累计 cost 与 token 计数器
//
// 与 labeledCounter 不同的是：cost 是浮点累加，token 也可能很大。
// 使用 atomic.Uint64 存储 math.Float64bits / uint64 位模式，实现无锁累加。
type labeledCost struct {
	costBits atomic.Uint64 // math.Float64bits
	tokens   atomic.Int64
	calls    atomic.Int64 // LLM 调用次数（用于 ap_cost_calls_total）
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

	h.sum += valueMs
	h.count++

	if valueMs < h.min {
		h.min = valueMs
	}
	if valueMs > h.max {
		h.max = valueMs
	}

	// 优化（perf-v3）：二分查找桶位置，O(log n) 替代 O(n) 线性扫描
	idx := sort.SearchFloat64s(h.buckets, float64(valueMs))
	if idx < len(h.counts) {
		h.counts[idx]++
	}

	h.mu.Unlock()
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

	counter.calls.Add(1)
	if err != nil {
		counter.errors.Add(1)
	}
}

func (m *AgentMetrics) RecordToolCall(duration time.Duration, err error) {
	atomic.AddInt64(&m.ToolTotalCalls, 1)
	if err != nil {
		atomic.AddInt64(&m.ToolTotalErrors, 1)
	}
	m.ToolLatencyMs.Record(duration.Milliseconds())
}

// RecordToolCallWithLabels 记录带标签维度的tool调用
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

	counter.calls.Add(1)
	if err != nil {
		counter.errors.Add(1)
	}
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

	counter.calls.Add(1)
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

// RecordCostUSD 增加指定 (provider, model, agentName) 维度的累计成本（USD）
//
// 使用 atomic.Uint64 存储 math.Float64bits 位模式，CAS 自旋累加，
// 既保证并发安全，又避免 mutex 阻塞。
func (m *AgentMetrics) RecordCostUSD(provider, model, agentName string, cost float64) {
	lc := m.costOrInit(provider, model, agentName, false)
	costAtomicAdd(&lc.costBits, cost)
}

// RecordCostCalls 增加指定 (provider, model, agentName) 维度下的 LLM 调用次数
//
// 调用次数本质是整数计数；签名仍为 float64 是为了与 cost 维度保持调用风格一致，
// 内部仍以整数原子累加；调用方传入非负 float64，截断为整数。
func (m *AgentMetrics) RecordCostCalls(provider, model, agentName string, calls float64) {
	if calls <= 0 {
		return
	}
	lc := m.costOrInit(provider, model, agentName, false)
	lc.calls.Add(int64(calls))
}

// RecordCostTokens 增加 (provider, model, agentName, kind) 维度下的 token 计数
//
// kind 用于区分 prompt / completion / total 等子维度；底层使用独立的
// CostTokensByLabel map，与 cost 累计互不影响。
func (m *AgentMetrics) RecordCostTokens(provider, model, agentName, kind string, tokens float64) {
	if tokens <= 0 {
		return
	}
	key := provider + "|" + model + "|" + agentName + "|" + kind
	m.mu.Lock()
	if m.CostTokensByLabel == nil {
		m.CostTokensByLabel = make(map[string]*labeledCost)
	}
	lc, ok := m.CostTokensByLabel[key]
	if !ok {
		lc = &labeledCost{}
		m.CostTokensByLabel[key] = lc
	}
	m.mu.Unlock()
	lc.tokens.Add(int64(tokens))
}

// SetLastCostUSD 设置最近一次调用的成本（USD，gauge 语义）
//
// 该方法是覆盖式赋值（gauge 而非 counter），用于 dashboard 展示最近一笔成本。
func (m *AgentMetrics) SetLastCostUSD(provider, model, agentName string, cost float64) {
	key := provider + "|" + model + "|" + agentName
	m.mu.Lock()
	if m.CostLastUSDByLabel == nil {
		m.CostLastUSDByLabel = make(map[string]float64)
	}
	m.CostLastUSDByLabel[key] = cost
	m.mu.Unlock()
}

// costOrInit 取或惰性初始化 (provider, model, agentName) 对应的 labeledCost
//
// kindOnly 暂未使用，预留以便将来引入额外的 cost 子分类（例如 input/output）。
// 返回的对象即使不在锁内创建，调用方仍依赖 atomic 操作访问其字段，因此安全。
func (m *AgentMetrics) costOrInit(provider, model, agentName string, _ bool) *labeledCost {
	key := provider + "|" + model + "|" + agentName
	m.mu.Lock()
	if m.CostByLabel == nil {
		m.CostByLabel = make(map[string]*labeledCost)
	}
	lc, ok := m.CostByLabel[key]
	if !ok {
		lc = &labeledCost{}
		m.CostByLabel[key] = lc
	}
	m.mu.Unlock()
	return lc
}

// costAtomicAdd 用 CAS 自旋把 delta 累加到 atomic.Uint64 存储的 float64 浮点值中
//
// 选择这种实现是为了与 cost_tracker.go 中 totalCostBits 的累加方式保持一致，
// 同时避免在高频写路径上引入 mutex 开销。
func costAtomicAdd(target *atomic.Uint64, delta float64) {
	for {
		oldBits := target.Load()
		oldVal := math.Float64frombits(oldBits)
		newVal := oldVal + delta
		if target.CompareAndSwap(oldBits, math.Float64bits(newVal)) {
			return
		}
	}
}

// ===== 包级 helper：默认 metrics 实例（用于零样板快速接入） =====

var defaultMetrics = NewMetrics()

// DefaultMetrics 返回包级共享的默认指标实例（适合不需要多副本隔离的简单场景）
//
// 大型部署应自行 NewMetrics() 并显式注入；这里提供的全局入口只是简化
// 在脚本式调用或 cmd/ 内部直接打点的成本。
func DefaultMetrics() *AgentMetrics {
	return defaultMetrics
}

// RecordCostUSD 向默认 metrics 实例记录累计成本（USD）
func RecordCostUSD(provider, model, agentName string, cost float64) {
	defaultMetrics.RecordCostUSD(provider, model, agentName, cost)
}

// RecordCostCalls 向默认 metrics 实例记录 LLM 调用次数
func RecordCostCalls(provider, model, agentName string, calls float64) {
	defaultMetrics.RecordCostCalls(provider, model, agentName, calls)
}

// RecordCostTokens 向默认 metrics 实例记录 token 数
func RecordCostTokens(provider, model, agentName, kind string, tokens float64) {
	defaultMetrics.RecordCostTokens(provider, model, agentName, kind, tokens)
}

// SetLastCostUSD 向默认 metrics 实例设置最近一次调用成本
func SetLastCostUSD(provider, model, agentName string, cost float64) {
	defaultMetrics.SetLastCostUSD(provider, model, agentName, cost)
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
	// 优化（perf-v3）：预分配 builder 容量，减少底层 []byte 扩容
	sb.Grow(2048)

	sb.WriteString("# HELP ap_llm_total_calls Total LLM API calls\n")
	sb.WriteString("# TYPE ap_llm_total_calls counter\n")
	sb.WriteString("ap_llm_total_calls ")
	sb.WriteString(strconv.FormatInt(snap.LLMTotalCalls, 10))
	sb.WriteByte('\n')

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

	for key, counter := range llmLabels {
		parts := strings.SplitN(key, "|", 2)
		provider, model := parts[0], ""
		if len(parts) > 1 {
			model = parts[1]
		}
		// 优化（perf-v3）：atomic 加载替代 mutex
		calls := counter.calls.Load()
		errors := counter.errors.Load()
		sb.WriteString(`ap_llm_calls_by_provider{provider="`)
		sb.WriteString(provider)
		sb.WriteString(`",model="`)
		sb.WriteString(model)
		sb.WriteString(`"} `)
		sb.WriteString(strconv.FormatInt(calls, 10))
		sb.WriteByte('\n')
		if errors > 0 {
			sb.WriteString(`ap_llm_errors_by_provider{provider="`)
			sb.WriteString(provider)
			sb.WriteString(`",model="`)
			sb.WriteString(model)
			sb.WriteString(`"} `)
			sb.WriteString(strconv.FormatInt(errors, 10))
			sb.WriteByte('\n')
		}
	}

	sb.WriteString("# HELP ap_llm_total_errors Total LLM API errors\n")
	sb.WriteString("# TYPE ap_llm_total_errors counter\n")
	sb.WriteString("ap_llm_total_errors ")
	sb.WriteString(strconv.FormatInt(snap.LLMTotalErrors, 10))
	sb.WriteByte('\n')

	sb.WriteString("# HELP ap_tool_total_calls Total tool calls\n")
	sb.WriteString("# TYPE ap_tool_total_calls counter\n")
	sb.WriteString("ap_tool_total_calls ")
	sb.WriteString(strconv.FormatInt(snap.ToolTotalCalls, 10))
	sb.WriteByte('\n')

	for toolName, counter := range toolLabels {
		calls := counter.calls.Load()
		errors := counter.errors.Load()
		sb.WriteString(`ap_tool_calls{tool_name="`)
		sb.WriteString(toolName)
		sb.WriteString(`"} `)
		sb.WriteString(strconv.FormatInt(calls, 10))
		sb.WriteByte('\n')
		if errors > 0 {
			sb.WriteString(`ap_tool_errors{tool_name="`)
			sb.WriteString(toolName)
			sb.WriteString(`"} `)
			sb.WriteString(strconv.FormatInt(errors, 10))
			sb.WriteByte('\n')
		}
	}

	sb.WriteString("# HELP ap_tool_total_errors Total tool errors\n")
	sb.WriteString("# TYPE ap_tool_total_errors counter\n")
	sb.WriteString("ap_tool_total_errors ")
	sb.WriteString(strconv.FormatInt(snap.ToolTotalErrors, 10))
	sb.WriteByte('\n')

	sb.WriteString("# HELP ap_total_turns Total agent turns\n")
	sb.WriteString("# TYPE ap_total_turns counter\n")
	sb.WriteString("ap_total_turns ")
	sb.WriteString(strconv.FormatInt(snap.TotalTurns, 10))
	sb.WriteByte('\n')

	for agentName, counter := range agentTurns {
		calls := counter.calls.Load()
		sb.WriteString(`ap_turns{agent_name="`)
		sb.WriteString(agentName)
		sb.WriteString(`"} `)
		sb.WriteString(strconv.FormatInt(calls, 10))
		sb.WriteByte('\n')
	}

	sb.WriteString("# HELP ap_active_agents Currently active agents\n")
	sb.WriteString("# TYPE ap_active_agents gauge\n")
	sb.WriteString("ap_active_agents ")
	sb.WriteString(strconv.FormatInt(snap.ActiveAgents, 10))
	sb.WriteByte('\n')

	sb.WriteString("# HELP ap_pool_queue_length Current pool queue length\n")
	sb.WriteString("# TYPE ap_pool_queue_length gauge\n")
	sb.WriteString("ap_pool_queue_length ")
	sb.WriteString(strconv.FormatInt(snap.PoolQueueLength, 10))
	sb.WriteByte('\n')

	sb.WriteString("# HELP ap_memory_size_bytes Memory store size in bytes\n")
	sb.WriteString("# TYPE ap_memory_size_bytes gauge\n")
	sb.WriteString("ap_memory_size_bytes ")
	sb.WriteString(strconv.FormatInt(snap.MemorySizeBytes, 10))
	sb.WriteByte('\n')

	writeHistogram(&sb, "ap_llm_latency_ms", snap.LLMLatencyMs)
	writeHistogram(&sb, "ap_tool_latency_ms", snap.ToolLatencyMs)
	writeHistogram(&sb, "ap_turn_duration_ms", snap.TurnDurationMs)

	// 成本相关指标
	writeCostMetrics(&sb, m)

	return sb.String()
}

// writeCostMetrics 输出 Prometheus 成本指标
func writeCostMetrics(sb *strings.Builder, m *AgentMetrics) {
	if len(m.CostByLabel) == 0 && len(m.CostTokensByLabel) == 0 && len(m.CostLastUSDByLabel) == 0 {
		return
	}

	sb.WriteString("# HELP ap_cost_usd_total Accumulated cost in USD\n")
	sb.WriteString("# TYPE ap_cost_usd_total counter\n")
	for key, lc := range m.CostByLabel {
		provider, model, agentName := splitCostKey(key, 3)
		cost := math.Float64frombits(lc.costBits.Load())
		sb.WriteString(`ap_cost_usd_total{provider="`)
		sb.WriteString(provider)
		sb.WriteString(`",model="`)
		sb.WriteString(model)
		sb.WriteString(`",agent_name="`)
		sb.WriteString(agentName)
		sb.WriteString(`"} `)
		sb.WriteString(strconv.FormatFloat(cost, 'f', -1, 64))
		sb.WriteByte('\n')
	}

	sb.WriteString("# HELP ap_cost_calls_total Accumulated LLM call count\n")
	sb.WriteString("# TYPE ap_cost_calls_total counter\n")
	for key, lc := range m.CostByLabel {
		provider, model, agentName := splitCostKey(key, 3)
		calls := lc.calls.Load()
		if calls == 0 {
			continue
		}
		sb.WriteString(`ap_cost_calls_total{provider="`)
		sb.WriteString(provider)
		sb.WriteString(`",model="`)
		sb.WriteString(model)
		sb.WriteString(`",agent_name="`)
		sb.WriteString(agentName)
		sb.WriteString(`"} `)
		sb.WriteString(strconv.FormatInt(calls, 10))
		sb.WriteByte('\n')
	}

	sb.WriteString("# HELP ap_cost_tokens_total Accumulated token count\n")
	sb.WriteString("# TYPE ap_cost_tokens_total counter\n")
	for key, lc := range m.CostTokensByLabel {
		parts := strings.SplitN(key, "|", 4)
		var provider, model, agentName, kind string
		if len(parts) >= 4 {
			provider, model, agentName, kind = parts[0], parts[1], parts[2], parts[3]
		} else {
			continue
		}
		tokens := lc.tokens.Load()
		if tokens == 0 {
			continue
		}
		sb.WriteString(`ap_cost_tokens_total{provider="`)
		sb.WriteString(provider)
		sb.WriteString(`",model="`)
		sb.WriteString(model)
		sb.WriteString(`",agent_name="`)
		sb.WriteString(agentName)
		sb.WriteString(`",kind="`)
		sb.WriteString(kind)
		sb.WriteString(`"} `)
		sb.WriteString(strconv.FormatInt(tokens, 10))
		sb.WriteByte('\n')
	}

	if len(m.CostLastUSDByLabel) > 0 {
		sb.WriteString("# HELP ap_cost_last_call_usd Last call cost in USD\n")
		sb.WriteString("# TYPE ap_cost_last_call_usd gauge\n")
		for key, cost := range m.CostLastUSDByLabel {
			provider, model, agentName := splitCostKey(key, 3)
			sb.WriteString(`ap_cost_last_call_usd{provider="`)
			sb.WriteString(provider)
			sb.WriteString(`",model="`)
			sb.WriteString(model)
			sb.WriteString(`",agent_name="`)
			sb.WriteString(agentName)
			sb.WriteString(`"} `)
			sb.WriteString(strconv.FormatFloat(cost, 'f', -1, 64))
			sb.WriteByte('\n')
		}
	}
}

// splitCostKey 解析 "a|b|c" 形式的 key
//
// parts 指定期望的段数；不足或空段用 "" 填充。
func splitCostKey(key string, parts int) (string, string, string) {
	segs := strings.SplitN(key, "|", parts)
	var a, b, c string
	if len(segs) > 0 {
		a = segs[0]
	}
	if len(segs) > 1 {
		b = segs[1]
	}
	if len(segs) > 2 {
		c = segs[2]
	}
	return a, b, c
}

// writeHistogram 输出 Prometheus histogram 格式
// 优化（perf-v3）：使用 strconv + strings.Builder 替代 fmt.Sprintf
func writeHistogram(sb *strings.Builder, name string, h HistogramSnapshot) {
	sb.WriteString("# HELP ")
	sb.WriteString(name)
	sb.WriteByte(' ')
	sb.WriteString(name)
	sb.WriteString(" histogram\n")
	sb.WriteString("# TYPE ")
	sb.WriteString(name)
	sb.WriteString(" histogram\n")

	for i, bucket := range h.Buckets {
		sb.WriteString(name)
		if i == len(h.Buckets)-1 {
			sb.WriteString(`_bucket{le="+Inf"} `)
		} else {
			sb.WriteString(`_bucket{le="`)
			sb.WriteString(strconv.FormatFloat(bucket, 'g', -1, 64))
			sb.WriteString(`"} `)
		}
		sb.WriteString(strconv.FormatInt(h.Counts[i], 10))
		sb.WriteByte('\n')
	}

	sb.WriteString(name)
	sb.WriteString("_sum ")
	sb.WriteString(strconv.FormatInt(h.Sum, 10))
	sb.WriteByte('\n')
	sb.WriteString(name)
	sb.WriteString("_count ")
	sb.WriteString(strconv.FormatInt(h.Count, 10))
	sb.WriteByte('\n')
}

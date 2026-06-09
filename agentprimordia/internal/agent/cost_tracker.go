package agent

import (
	"agentprimordia/internal/llm"
	"sync"
	"time"
)

// CostRecord 单次成本记录
type CostRecord struct {
	Model            string    `json:"model"`
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	TotalTokens      int       `json:"total_tokens"`
	CostUSD          float64   `json:"cost_usd"`
	Timestamp        time.Time `json:"timestamp"`
	SessionID        string    `json:"session_id"`
	AgentName        string    `json:"agent_name"`
}

// BudgetConfig 预算配置
type BudgetConfig struct {
	MaxTotalCostUSD     float64
	MaxTokensPerCall    int
	MaxTokensPerSession int
	OnBudgetExceed      func(summary *CostSummary)
}

// CostSummary 成本汇总
type CostSummary struct {
	TotalCostUSD      float64               `json:"total_cost_usd"`
	TotalPromptTokens int64                 `json:"total_prompt_tokens"`
	TotalCompTokens   int64                 `json:"total_completion_tokens"`
	TotalTokens       int64                 `json:"total_tokens"`
	CallCount         int                   `json:"call_count"`
	ByModel           map[string]*ModelCost `json:"by_model"`
}

// ModelCost 单模型成本
type ModelCost struct {
	CostUSD float64 `json:"cost_usd"`
	Calls   int     `json:"calls"`
	Tokens  int64   `json:"tokens"`
}

// CostTracker 成本追踪器
type CostTracker struct {
	pricing map[string]llm.ModelPricing
	budget  *BudgetConfig
	records []CostRecord
	mu      sync.RWMutex
}

// NewCostTracker 创建成本追踪器
func NewCostTracker(pricing map[string]llm.ModelPricing, budget *BudgetConfig) *CostTracker {
	return &CostTracker{
		pricing: pricing,
		budget:  budget,
		records: make([]CostRecord, 0),
	}
}

// Record 记录一次 LLM 调用的 Usage
func (t *CostTracker) Record(model, sessionID, agentName string, usage llm.Usage) error {
	cost := llm.EstimateCost(model, usage, t.pricing)

	record := CostRecord{
		Model:            model,
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		TotalTokens:      usage.TotalTokens,
		CostUSD:          cost,
		Timestamp:        time.Now(),
		SessionID:        sessionID,
		AgentName:        agentName,
	}

	// 在同一个锁保护下完成记录 + 预算检查，消除 TOCTOU 竞态
	t.mu.Lock()
	t.records = append(t.records, record)
	exceeded := t.checkBudgetLocked()
	t.mu.Unlock()

	if exceeded && t.budget != nil && t.budget.OnBudgetExceed != nil {
		t.budget.OnBudgetExceed(t.Summary())
	}

	return nil
}

// Summary 返回成本汇总
func (t *CostTracker) Summary() *CostSummary {
	t.mu.RLock()
	defer t.mu.RUnlock()

	summary := &CostSummary{
		ByModel: make(map[string]*ModelCost),
	}

	for _, r := range t.records {
		summary.CallCount++
		summary.TotalCostUSD += r.CostUSD
		summary.TotalPromptTokens += int64(r.PromptTokens)
		summary.TotalCompTokens += int64(r.CompletionTokens)
		summary.TotalTokens += int64(r.TotalTokens)

		mc, ok := summary.ByModel[r.Model]
		if !ok {
			mc = &ModelCost{}
			summary.ByModel[r.Model] = mc
		}
		mc.CostUSD += r.CostUSD
		mc.Calls++
		mc.Tokens += int64(r.TotalTokens)
	}

	return summary
}

// CheckBudget 检查是否超出预算
func (t *CostTracker) CheckBudget() bool {
	if t.budget == nil {
		return false
	}

	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.checkBudgetLocked()
}

// checkBudgetLocked 在已持有锁的情况下检查预算（调用方必须持有 t.mu）
func (t *CostTracker) checkBudgetLocked() bool {
	if t.budget == nil {
		return false
	}

	if t.budget.MaxTotalCostUSD > 0 {
		var totalCost float64
		for _, r := range t.records {
			totalCost += r.CostUSD
		}
		if totalCost > t.budget.MaxTotalCostUSD {
			return true
		}
	}

	if t.budget.MaxTokensPerSession > 0 {
		var totalTokens int64
		for _, r := range t.records {
			totalTokens += int64(r.TotalTokens)
		}
		if totalTokens > int64(t.budget.MaxTokensPerSession) {
			return true
		}
	}

	// 检查单次调用 Token 上限
	if t.budget.MaxTokensPerCall > 0 && len(t.records) > 0 {
		last := t.records[len(t.records)-1]
		if last.TotalTokens > t.budget.MaxTokensPerCall {
			return true
		}
	}

	return false
}

// Reset 重置追踪器
func (t *CostTracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.records = make([]CostRecord, 0)
}

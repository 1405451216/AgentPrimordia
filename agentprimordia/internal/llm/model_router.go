// model_router.go — Phase 2 G2-1 成本感知模型路由器
// 根据消息复杂度、上下文长度、是否需要工具等指标，从多个注册的 LLM Provider
// 中选择最优的一个。使用 atomic 无锁统计高频调用指标。
//
// 设计取舍：
//   - 路由策略：CostFirst / QualityFirst / Balanced（枚举）
//   - 无锁统计：每模型一组 atomic 计数器（避免锁开销）
//   - 评分模型：复杂度 [0,1] × 成本 × 能力等级 → 加权得分
//   - 降级：fallback 配置 + ErrNoSuitableModel
package llm

import (
	"context"
	"errors"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

// ErrNoSuitableModel 无可用模型时返回
var ErrNoSuitableModel = errors.New("no suitable model found for the request")

// RouteStrategy 路由策略
type RouteStrategy int

const (
	// StrategyCostFirst 优先选择成本最低的可用模型
	StrategyCostFirst RouteStrategy = iota
	// StrategyQualityFirst 优先选择能力最强的模型（按 ComplexityLimit 倒序）
	StrategyQualityFirst
	// StrategyBalanced 综合评分（成本 + 能力）
	StrategyBalanced
)

// String 返回策略名（便于日志/调试）
func (s RouteStrategy) String() string {
	switch s {
	case StrategyCostFirst:
		return "cost-first"
	case StrategyQualityFirst:
		return "quality-first"
	case StrategyBalanced:
		return "balanced"
	default:
		return "unknown"
	}
}

// ModelRouteConfig 单个模型的路由配置
type ModelRouteConfig struct {
	Name            string // 模型名称（用于日志/统计）
	Provider        Provider
	CostPer1K       float64 // 每 1K token 的成本（美元）
	ComplexityLimit float64 // 该模型能处理的复杂度上限 [0, 1]
	MaxContext      int     // 最大上下文长度（token 数）
	SupportsTools   bool    // 是否支持 tool calling
	Priority        int     // 优先级（数字越小优先级越高，相同评分时使用）
}

// modelStats 单个模型的运行时统计（无锁）
type modelStats struct {
	calls     atomic.Int64
	failures  atomic.Int64
	totalMs   atomic.Int64
	totalCost atomic.Uint64 // 用 Uint64 存 float64 的 bit pattern，避免 Float64 兼容性问题
}

// Snapshot 返回统计快照（用于监控/调试）
type StatsSnapshot struct {
	Calls     int64
	Failures  int64
	TotalMs   int64
	TotalCost float64
}

func (s *modelStats) Snapshot() StatsSnapshot {
	costBits := s.totalCost.Load()
	return StatsSnapshot{
		Calls:     s.calls.Load(),
		Failures:  s.failures.Load(),
		TotalMs:   s.totalMs.Load(),
		TotalCost: math.Float64frombits(costBits),
	}
}

func (s *modelStats) record(durationMs int64, cost float64, err error) {
	s.calls.Add(1)
	s.totalMs.Add(durationMs)
	for {
		oldBits := s.totalCost.Load()
		oldCost := math.Float64frombits(oldBits)
		newCost := oldCost + cost
		newBits := math.Float64bits(newCost)
		if s.totalCost.CompareAndSwap(oldBits, newBits) {
			break
		}
	}
	if err != nil {
		s.failures.Add(1)
	}
}

// ModelRouter 多模型路由器
//
// 并发安全：Register 在初始化阶段调用即可（运行时也可加锁注册，文档已注明）。
// Route/Record 是无锁读路径，适合高频调用。
type ModelRouter struct {
	mu       sync.RWMutex  // 保护 models 的添加/删除
	models   []*modelEntry // 指针切片：避免复制 modelStats 中的 atomic 锁（noCopy）
	strategy RouteStrategy
	fallback string // 兜底模型名
}

type modelEntry struct {
	config ModelRouteConfig
	stats  modelStats
}

// NewModelRouter 创建路由器
func NewModelRouter(strategy RouteStrategy) *ModelRouter {
	return &ModelRouter{strategy: strategy}
}

// Register 注册一个模型。线程安全，可运行时调用。
func (r *ModelRouter) Register(cfg ModelRouteConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.models = append(r.models, &modelEntry{config: cfg})
}

// SetFallback 设置兜底模型名（当 Route 找不到合适模型时返回）
func (r *ModelRouter) SetFallback(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fallback = name
}

// SetStrategy 动态切换策略
func (r *ModelRouter) SetStrategy(s RouteStrategy) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.strategy = s
}

// Snapshot 返回所有模型的统计快照
func (r *ModelRouter) Snapshot() map[string]StatsSnapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]StatsSnapshot, len(r.models))
	for _, e := range r.models {
		out[e.config.Name] = e.stats.Snapshot()
	}
	return out
}

// RouteDecision 路由决策结果
type RouteDecision struct {
	ModelName     string  // 选择的模型名称
	Complexity    float64 // 请求复杂度评分 [0,1]
	ContextLen    int     // 估算的上下文长度
	EstimatedCost float64 // 估算本次调用的成本
	Strategy      RouteStrategy
}

// Route 根据消息和配置选择最优模型。
// 返回选中的 Provider 和决策详情。
func (r *ModelRouter) Route(ctx context.Context, messages []ChatMessage, hasTools bool) (Provider, *RouteDecision, error) {
	complexity := r.evaluateComplexity(messages)
	contextLen := estimateTokens(messages)

	r.mu.RLock()
	strategy := r.strategy
	fallback := r.fallback
	// 拷贝候选列表以避免持锁做重活（复制指针，不复制 atomic 锁）
	candidates := make([]*modelEntry, len(r.models))
	copy(candidates, r.models)
	r.mu.RUnlock()

	// 过滤：上下文 + 工具能力
	filtered := make([]*modelEntry, 0, len(candidates))
	for _, e := range candidates {
		if e.config.MaxContext > 0 && contextLen > e.config.MaxContext {
			continue
		}
		if hasTools && !e.config.SupportsTools {
			continue
		}
		if complexity > e.config.ComplexityLimit {
			continue
		}
		filtered = append(filtered, e)
	}

	if len(filtered) == 0 {
		// 尝试 fallback
		if fallback != "" {
			for _, e := range candidates {
				if e.config.Name == fallback {
					return e.config.Provider, &RouteDecision{
						ModelName:     e.config.Name,
						Complexity:    complexity,
						ContextLen:    contextLen,
						EstimatedCost: 0,
						Strategy:      strategy,
					}, nil
				}
			}
		}
		return nil, nil, ErrNoSuitableModel
	}

	// 按策略排序
	r.sortByStrategy(filtered, strategy)

	selected := filtered[0]
	estimatedCost := selected.config.CostPer1K * float64(contextLen) / 1000.0

	return selected.config.Provider, &RouteDecision{
		ModelName:     selected.config.Name,
		Complexity:    complexity,
		ContextLen:    contextLen,
		EstimatedCost: estimatedCost,
		Strategy:      strategy,
	}, nil
}

// Record 记录一次调用的实际成本和耗时。
// 应在调用 Provider 之后调用（无论成功或失败）。
func (r *ModelRouter) Record(modelName string, durationMs int64, cost float64, err error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for i := range r.models {
		if r.models[i].config.Name == modelName {
			r.models[i].stats.record(durationMs, cost, err)
			return
		}
	}
}

// ===== 内部辅助 =====

// evaluateComplexity 评估消息复杂度 [0, 1]
//
// 启发式评分：
//   - 长度 > 4000 chars → +0.3
//   - 长度 > 8000 chars → +0.2 (cumulative)
//   - 含代码/函数关键词 → +0.2
//   - 多步骤/序列关键词 → +0.15
//   - 推理/分析关键词 → +0.15
func (r *ModelRouter) evaluateComplexity(messages []ChatMessage) float64 {
	var totalLen int
	var hasCode, hasMultistep, hasReasoning bool
	for _, m := range messages {
		totalLen += len(m.Content)
		if containsAny(m.Content, "代码", "code", "function", "func ", "实现", "implement") {
			hasCode = true
		}
		if containsAny(m.Content, "步骤", "step", "首先", "然后", "first", "then", "next") {
			hasMultistep = true
		}
		if containsAny(m.Content, "为什么", "分析", "推理", "explain", "why", "analyze", "compare") {
			hasReasoning = true
		}
	}
	complexity := 0.0
	if totalLen > 4000 {
		complexity += 0.3
	}
	if totalLen > 8000 {
		complexity += 0.2
	}
	if hasCode {
		complexity += 0.2
	}
	if hasMultistep {
		complexity += 0.15
	}
	if hasReasoning {
		complexity += 0.15
	}
	if complexity > 1.0 {
		complexity = 1.0
	}
	return complexity
}

// estimateTokens 粗略估算消息总 token 数（4 字符 ≈ 1 token 的英文启发式）
func estimateTokens(messages []ChatMessage) int {
	var total int
	for _, m := range messages {
		// 中文/日文等 CJK 字符按 1.5 char/token 估算
		cjkCount := countCJK(m.Content)
		otherCount := len(m.Content) - cjkCount
		total += int(float64(cjkCount)/1.5 + float64(otherCount)/4.0)
		// 加上 role/tool_calls 的固定开销
		total += 4
	}
	return total
}

// countCJK 统计 CJK 字符数量
func countCJK(s string) int {
	count := 0
	for _, r := range s {
		if (r >= 0x4E00 && r <= 0x9FFF) || // CJK Unified Ideographs
			(r >= 0x3040 && r <= 0x309F) || // Hiragana
			(r >= 0x30A0 && r <= 0x30FF) || // Katakana
			(r >= 0xAC00 && r <= 0xD7AF) { // Hangul
			count++
		}
	}
	return count
}

// containsAny 大小写不敏感地检查 s 是否包含 keywords 中的任意一个
func containsAny(s string, keywords ...string) bool {
	lower := strings.ToLower(s)
	for _, k := range keywords {
		if strings.Contains(lower, strings.ToLower(k)) {
			return true
		}
	}
	return false
}

// sortByStrategy 按指定策略对候选排序
func (r *ModelRouter) sortByStrategy(candidates []*modelEntry, strategy RouteStrategy) {
	switch strategy {
	case StrategyCostFirst:
		sort.SliceStable(candidates, func(i, j int) bool {
			if candidates[i].config.CostPer1K != candidates[j].config.CostPer1K {
				return candidates[i].config.CostPer1K < candidates[j].config.CostPer1K
			}
			return candidates[i].config.Priority < candidates[j].config.Priority
		})
	case StrategyQualityFirst:
		sort.SliceStable(candidates, func(i, j int) bool {
			if candidates[i].config.ComplexityLimit != candidates[j].config.ComplexityLimit {
				return candidates[i].config.ComplexityLimit > candidates[j].config.ComplexityLimit
			}
			return candidates[i].config.Priority < candidates[j].config.Priority
		})
	case StrategyBalanced:
		// 综合评分 = 0.5 * (1 - normalizedCost) + 0.5 * ComplexityLimit
		// 简化版：直接用 CostPer1K 倒数 + ComplexityLimit
		sort.SliceStable(candidates, func(i, j int) bool {
			scoreI := (1.0 / (1.0 + candidates[i].config.CostPer1K)) + candidates[i].config.ComplexityLimit
			scoreJ := (1.0 / (1.0 + candidates[j].config.CostPer1K)) + candidates[j].config.ComplexityLimit
			if scoreI != scoreJ {
				return scoreI > scoreJ
			}
			return candidates[i].config.Priority < candidates[j].config.Priority
		})
	}
}

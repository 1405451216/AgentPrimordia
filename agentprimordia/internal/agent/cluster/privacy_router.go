// Phase 2.3: 隐私×集群 — 隐私感知路由
//
// 将 guardrail.PIIDetector 与 cluster 集成：
//   - PII 请求路由到有本地推理能力（WebGPU）的节点
//   - 节点通过能力广播声明 WebGPU 可用性
//   - 脱敏映射表通过 DistributedState 跨节点同步
//
// 使用方式：
//
//	router := cluster.NewPrivacyRouter(
//	    cluster.WithPIIDetector(detector),
//	    cluster.WithPrivacyState(state),
//	)
//	target, err := router.Route(ctx, userInput)

package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// ===== PII 检测接口（解耦 guardrail 包） =====

// PIIFinding PII 检测结果
type PIIFinding struct {
	Type  string `json:"type"`  // "email"/"phone"/"id_card" 等
	Value string `json:"value"` // 原始值
	Start int    `json:"start"`
	End   int    `json:"end"`
}

// PIIDetector PII 检测器接口
// 由 guardrail.PIIDetector 适配实现
type PIIDetector interface {
	// Detect 检测文本中的 PII
	Detect(text string) []PIIFinding
}

// ===== 节点能力 =====

// NodeCapability 节点能力声明
type NodeCapability struct {
	// HasWebGPU 是否有本地 WebGPU 推理能力
	HasWebGPU bool `json:"has_webgpu"`
	// HasLocalLLM 是否有本地 LLM
	HasLocalLLM bool `json:"has_local_llm"`
	// PrivacyLevel 隐私处理等级（0=无, 1=脱敏, 2=本地推理）
	PrivacyLevel int `json:"privacy_level"`
	// MaxConcurrent 最大并发隐私请求数
	MaxConcurrent int `json:"max_concurrent"`
	// CurrentLoad 当前隐私请求负载
	CurrentLoad int `json:"current_load"`
}

// ===== 脱敏映射 =====

// RedactionMapping 脱敏映射条目
type RedactionMapping struct {
	// Original 原始值（仅存储在本地，不跨节点传输）
	Original string `json:"-"`
	// Token 替换令牌
	Token string `json:"token"`
	// PIIType PII 类型
	PIIType string `json:"pii_type"`
	// NodeID 创建此映射的节点
	NodeID string `json:"node_id"`
	// CreatedAt 创建时间
	CreatedAt time.Time `json:"created_at"`
	// ExpiresAt 过期时间
	ExpiresAt time.Time `json:"expires_at"`
}

// ===== 隐私路由器 =====

// PrivacyRouterConfig 隐私路由器配置
type PrivacyRouterConfig struct {
	// LocalNodeID 本地节点 ID
	LocalNodeID string
	// RedactionTTL 脱敏映射 TTL（默认 1h）
	RedactionTTL time.Duration
	// RequireLocalInference 是否强制要求本地推理（无 WebGPU 节点则拒绝）
	RequireLocalInference bool
	// FallbackToRedaction 无本地推理节点时是否回退到脱敏模式
	FallbackToRedaction bool
}

// DefaultPrivacyRouterConfig 默认配置
func DefaultPrivacyRouterConfig() PrivacyRouterConfig {
	return PrivacyRouterConfig{
		RedactionTTL:          time.Hour,
		RequireLocalInference: false,
		FallbackToRedaction:   true,
	}
}

// RouteDecision 路由决策
type RouteDecision struct {
	// TargetNode 目标节点 ID
	TargetNode string `json:"target_node"`
	// Strategy 路由策略
	Strategy RouteStrategy `json:"strategy"`
	// HasPII 是否检测到 PII
	HasPII bool `json:"has_pii"`
	// PIITypes 检测到的 PII 类型
	PIITypes []string `json:"pii_types,omitempty"`
	// Redacted 是否已脱敏
	Redacted bool `json:"redacted"`
	// RedactionTokens 脱敏令牌映射（token -> PII 类型）
	RedactionTokens map[string]string `json:"redaction_tokens,omitempty"`
}

// RouteStrategy 路由策略
type RouteStrategy string

const (
	// StrategyDirect 直接路由（无 PII）
	StrategyDirect RouteStrategy = "direct"
	// StrategyLocalInference 路由到本地推理节点
	StrategyLocalInference RouteStrategy = "local_inference"
	// StrategyRedact 脱敏后路由
	StrategyRedact RouteStrategy = "redact"
	// StrategyReject 拒绝（无可用隐私节点）
	StrategyReject RouteStrategy = "reject"
)

// PrivacyRouter 隐私感知路由器
//
// 根据请求中的 PII 内容和集群节点的隐私处理能力，
// 决定请求的路由目标和处理策略。
type PrivacyRouter struct {
	config   PrivacyRouterConfig
	detector PIIDetector
	state    StateProvider
	logger   *slog.Logger

	mu           sync.RWMutex
	capabilities map[string]*NodeCapability   // nodeID -> 能力
	redactions   map[string]*RedactionMapping // token -> 映射
	tokenCounter int
}

// StateProvider 分布式状态接口（复用 marketplace 中定义）
type StateProvider interface {
	Set(key, value string, ttl time.Duration)
	Get(key string) (string, bool)
	Delete(key string) bool
	Keys() []string
}

// PrivacyRouterOption 路由器选项
type PrivacyRouterOption func(*PrivacyRouter)

// WithPIIDetector 设置 PII 检测器
func WithPIIDetector(detector PIIDetector) PrivacyRouterOption {
	return func(r *PrivacyRouter) {
		r.detector = detector
	}
}

// WithPrivacyState 设置分布式状态存储
func WithPrivacyState(state StateProvider) PrivacyRouterOption {
	return func(r *PrivacyRouter) {
		r.state = state
	}
}

// WithPrivacyConfig 设置路由器配置
func WithPrivacyConfig(cfg PrivacyRouterConfig) PrivacyRouterOption {
	return func(r *PrivacyRouter) {
		r.config = cfg
	}
}

// NewPrivacyRouter 创建隐私感知路由器
func NewPrivacyRouter(opts ...PrivacyRouterOption) *PrivacyRouter {
	r := &PrivacyRouter{
		config:       DefaultPrivacyRouterConfig(),
		logger:       slog.Default(),
		capabilities: make(map[string]*NodeCapability),
		redactions:   make(map[string]*RedactionMapping),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// WithLogger 设置日志器
func (r *PrivacyRouter) WithLogger(logger *slog.Logger) *PrivacyRouter {
	r.logger = logger
	return r
}

// RegisterCapability 注册节点隐私处理能力
func (r *PrivacyRouter) RegisterCapability(nodeID string, cap *NodeCapability) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.capabilities[nodeID] = cap

	// 广播到分布式状态
	r.broadcastCapability(nodeID, cap)
}

// UnregisterCapability 注销节点能力
func (r *PrivacyRouter) UnregisterCapability(nodeID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.capabilities, nodeID)

	// 从分布式状态删除
	if r.state != nil {
		r.state.Delete("privacy:cap:" + nodeID)
	}
}

// Route 路由请求（核心方法）
//
// 决策逻辑：
//  1. 检测 PII → 无 PII 则直接路由
//  2. 有 PII → 查找有本地推理能力的节点
//  3. 找到 → 路由到该节点（StrategyLocalInference）
//  4. 未找到 → 脱敏后路由（StrategyRedact）或拒绝（StrategyReject）
func (r *PrivacyRouter) Route(ctx context.Context, text string, candidateNodes []string) (*RouteDecision, error) {
	decision := &RouteDecision{
		Strategy: StrategyDirect,
	}

	// 1. PII 检测
	var findings []PIIFinding
	if r.detector != nil {
		findings = r.detector.Detect(text)
	}

	if len(findings) == 0 {
		// 无 PII，直接路由到第一个候选节点
		if len(candidateNodes) > 0 {
			decision.TargetNode = candidateNodes[0]
		}
		return decision, nil
	}

	// 2. 有 PII
	decision.HasPII = true
	decision.PIITypes = extractPIITypes(findings)

	// 3. 查找有本地推理能力的节点
	targetNode := r.findPrivacyNode(candidateNodes)
	if targetNode != "" {
		decision.TargetNode = targetNode
		decision.Strategy = StrategyLocalInference
		r.logger.Info("PII 请求路由到本地推理节点",
			"target", targetNode,
			"pii_types", decision.PIITypes,
		)
		return decision, nil
	}

	// 4. 无本地推理节点
	if r.config.RequireLocalInference {
		decision.Strategy = StrategyReject
		r.logger.Warn("PII 请求被拒绝：无可用本地推理节点",
			"pii_types", decision.PIITypes,
		)
		return decision, nil
	}

	if r.config.FallbackToRedaction {
		// 脱敏后路由
		decision.Strategy = StrategyRedact
		decision.Redacted = true
		decision.RedactionTokens = r.createRedactionMappings(findings)
		if len(candidateNodes) > 0 {
			decision.TargetNode = candidateNodes[0]
		}
		r.logger.Info("PII 请求脱敏后路由",
			"target", decision.TargetNode,
			"tokens", len(decision.RedactionTokens),
		)
		return decision, nil
	}

	decision.Strategy = StrategyReject
	return decision, nil
}

// RestoreRedacted 恢复脱敏文本
// 将令牌替换回原始值（仅在创建映射的节点上可用）
func (r *PrivacyRouter) RestoreRedacted(text string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := text
	for token, mapping := range r.redactions {
		if mapping.Original != "" {
			result = replaceAll(result, token, mapping.Original)
		}
	}
	return result
}

// SyncCapabilities 从分布式状态同步节点能力
func (r *PrivacyRouter) SyncCapabilities() int {
	if r.state == nil {
		return 0
	}

	synced := 0
	keys := r.state.Keys()
	for _, key := range keys {
		if len(key) < 13 || key[:13] != "privacy:cap:{" {
			// 兼容两种格式
			if len(key) < 12 || key[:12] != "privacy:cap:" {
				continue
			}
		}
		val, ok := r.state.Get(key)
		if !ok {
			continue
		}

		var cap NodeCapability
		if err := json.Unmarshal([]byte(val), &cap); err != nil {
			continue
		}

		// 提取 nodeID
		nodeID := key[12:]
		r.mu.Lock()
		r.capabilities[nodeID] = &cap
		r.mu.Unlock()
		synced++
	}

	if synced > 0 {
		r.logger.Info("同步节点隐私能力", "count", synced)
	}
	return synced
}

// GetCapability 获取节点能力
func (r *PrivacyRouter) GetCapability(nodeID string) (*NodeCapability, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cap, ok := r.capabilities[nodeID]
	if !ok {
		return nil, false
	}
	cp := *cap
	return &cp, true
}

// CleanupExpiredRedactions 清理过期的脱敏映射
func (r *PrivacyRouter) CleanupExpiredRedactions() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	count := 0
	for token, mapping := range r.redactions {
		if !mapping.ExpiresAt.IsZero() && now.After(mapping.ExpiresAt) {
			delete(r.redactions, token)
			count++
		}
	}
	return count
}

// ===== 内部方法 =====

// findPrivacyNode 查找有本地推理能力的节点
func (r *PrivacyRouter) findPrivacyNode(candidates []string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var bestNode string
	bestLoad := int(^uint(0) >> 1) // max int

	for _, nodeID := range candidates {
		cap, ok := r.capabilities[nodeID]
		if !ok {
			continue
		}
		// 需要有本地推理能力
		if !cap.HasWebGPU && !cap.HasLocalLLM {
			continue
		}
		if cap.PrivacyLevel < 2 {
			continue
		}
		// 选择负载最低的
		if cap.CurrentLoad < bestLoad {
			bestLoad = cap.CurrentLoad
			bestNode = nodeID
		}
	}

	return bestNode
}

// createRedactionMappings 创建脱敏映射
func (r *PrivacyRouter) createRedactionMappings(findings []PIIFinding) map[string]string {
	r.mu.Lock()
	defer r.mu.Unlock()

	tokens := make(map[string]string)
	now := time.Now()

	for _, f := range findings {
		r.tokenCounter++
		token := fmt.Sprintf("{{PII_%s_%d}}", f.Type, r.tokenCounter)

		mapping := &RedactionMapping{
			Original:  f.Value,
			Token:     token,
			PIIType:   f.Type,
			NodeID:    r.config.LocalNodeID,
			CreatedAt: now,
			ExpiresAt: now.Add(r.config.RedactionTTL),
		}
		r.redactions[token] = mapping
		tokens[token] = f.Type
	}

	// 同步令牌元数据到分布式状态（不含原始值）
	if r.state != nil {
		for token, piiType := range tokens {
			meta := fmt.Sprintf(`{"type":"%s","node":"%s"}`, piiType, r.config.LocalNodeID)
			r.state.Set("privacy:redact:"+token, meta, r.config.RedactionTTL)
		}
	}

	return tokens
}

// broadcastCapability 广播节点能力到分布式状态
func (r *PrivacyRouter) broadcastCapability(nodeID string, cap *NodeCapability) {
	if r.state == nil {
		return
	}
	data, err := json.Marshal(cap)
	if err != nil {
		return
	}
	// 能力广播使用较长 TTL（5 分钟），由心跳续期
	r.state.Set("privacy:cap:"+nodeID, string(data), 5*time.Minute)
}

// extractPIITypes 提取去重的 PII 类型列表
func extractPIITypes(findings []PIIFinding) []string {
	seen := make(map[string]bool)
	var types []string
	for _, f := range findings {
		if !seen[f.Type] {
			seen[f.Type] = true
			types = append(types, f.Type)
		}
	}
	return types
}

// replaceAll 简单字符串替换
func replaceAll(s, old, new string) string {
	for {
		idx := indexOf(s, old)
		if idx < 0 {
			return s
		}
		s = s[:idx] + new + s[idx+len(old):]
	}
}

// indexOf 查找子串位置
func indexOf(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

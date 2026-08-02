package a2a

import (
	"context"
	"fmt"
)

// v3.5 Phase 2: 互操作 × 跨组件集成接口
//
// 所有集成通过接口解耦，运行时可选注入。

// --- 互操作 × 认证 ---

// InteropAuthenticator 开放协议请求认证接口（对接 a2a/auth.go + grpc_auth.go）
type InteropAuthenticator interface {
	// Authenticate 校验请求凭据，返回主体标识或错误
	Authenticate(ctx context.Context, credential string) (subject string, err error)
}

// AuthIntegration 互操作认证集成
type AuthIntegration struct{ auth InteropAuthenticator }

// NewAuthIntegration 创建认证集成
func NewAuthIntegration(auth InteropAuthenticator) *AuthIntegration {
	return &AuthIntegration{auth: auth}
}

// Check 校验开放协议请求
func (a *AuthIntegration) Check(ctx context.Context, credential string) (string, error) {
	return a.auth.Authenticate(ctx, credential)
}

// --- 互操作 × 发现 ---

// InteropRegistry 开放 Agent Card 注册接口（对接 cluster/ 发现服务）
type InteropRegistry interface {
	// Register 注册开放 Agent Card，使生态可发现
	Register(card OpenAgentCard) error
	// Deregister 注销
	Deregister(name string) error
}

// DiscoveryIntegration 互操作发现集成
type DiscoveryIntegration struct{ reg InteropRegistry }

// NewDiscoveryIntegration 创建发现集成
func NewDiscoveryIntegration(reg InteropRegistry) *DiscoveryIntegration {
	return &DiscoveryIntegration{reg: reg}
}

// RegisterCard 注册 Agent Card 到生态发现服务
func (d *DiscoveryIntegration) RegisterCard(card OpenAgentCard) error {
	if card.Name == "" || card.URL == "" {
		return fmt.Errorf("a2a interop: Agent Card 缺少 name 或 url")
	}
	return d.reg.Register(card)
}

// --- 互操作 × 追踪 ---

// InteropTracer 跨生态 trace 传播接口（对接 a2a/trace_propagation.go）
type InteropTracer interface {
	// Inject 将 trace 上下文注入出站请求头
	Inject(ctx context.Context, headers map[string]string)
	// Extract 从入站请求头提取 trace 上下文
	Extract(ctx context.Context, headers map[string]string) context.Context
}

// TraceIntegration 互操作追踪集成
type TraceIntegration struct{ tracer InteropTracer }

// NewTraceIntegration 创建追踪集成
func NewTraceIntegration(tracer InteropTracer) *TraceIntegration {
	return &TraceIntegration{tracer: tracer}
}

// Propagate 提取入站 trace 并返回带 trace 的上下文
func (t *TraceIntegration) Propagate(ctx context.Context, headers map[string]string) context.Context {
	return t.tracer.Extract(ctx, headers)
}

// --- 互操作 × 限流 ---

// InteropRateLimiter 第三方调用限流接口（对接 grpc_circuit_breaker.go + tool_lease.go）
type InteropRateLimiter interface {
	// Allow 判断指定主体是否允许调用
	Allow(subject string) bool
}

// RateLimitIntegration 互操作限流集成
type RateLimitIntegration struct{ limiter InteropRateLimiter }

// NewRateLimitIntegration 创建限流集成
func NewRateLimitIntegration(limiter InteropRateLimiter) *RateLimitIntegration {
	return &RateLimitIntegration{limiter: limiter}
}

// CheckQuota 检查第三方调用配额
func (r *RateLimitIntegration) CheckQuota(subject string) error {
	if !r.limiter.Allow(subject) {
		return NewOpenError(OpenErrUnsupportedOperation, "rate limit exceeded for "+subject)
	}
	return nil
}

// --- 互操作 × 技能 ---

// InteropSkillSource 技能清单来源接口（对接 v3.4 Skill 库，生态可见能力）
type InteropSkillSource interface {
	// Declarations 返回可声明到 Agent Card 的技能清单
	Declarations() []OpenSkillDecl
}

// SkillIntegration 互操作技能集成
type SkillIntegration struct{ src InteropSkillSource }

// NewSkillIntegration 创建技能集成
func NewSkillIntegration(src InteropSkillSource) *SkillIntegration {
	return &SkillIntegration{src: src}
}

// EnrichCard 用技能清单填充 Agent Card 的 skills 字段
func (s *SkillIntegration) EnrichCard(card *OpenAgentCard) {
	card.Skills = s.src.Declarations()
}

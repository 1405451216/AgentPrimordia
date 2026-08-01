// Package studio 提供 Studio Web UI 的后端 HTTP API。
//
// Studio 四面板（Chaos Lab / Cluster / Learning / Marketplace）依赖的
// /api/v1/* 端点在此实现。各面板通过 Service 接口与底层逻辑包解耦：
//
//   - ChaosService       → 可注入 internal/chaos 引擎
//   - ClusterService     → 可注入 internal/agent/cluster 管理器
//   - LearningService    → 可注入 internal/agent/learning 蒸馏器
//   - MarketplaceService → 可注入 internal/agent/marketplace 注册表
//
// 默认使用 demo 实现（demo.go），保证开箱即可演示；真实引擎通过
// WithChaos / WithCluster / WithLearning / WithMarketplace 注入。
//
// 响应类型与 studio/web/src/pages/*.tsx 中前端定义的接口一一对应。
package studio

import (
	"context"
	"net/http"
)

// ===== 前端契约类型（与 studio/web/src/pages/ 中 TS 接口对齐） =====

// Experiment 混沌实验定义。
type Experiment struct {
	Name                string  `json:"name"`
	Description         string  `json:"description"`
	Hypothesis          string  `json:"hypothesis"`
	Status              string  `json:"status"` // pending | running | completed | aborted | failed
	Duration            string  `json:"duration"`
	Faults              []Fault `json:"faults"`
	HypothesisValidated bool    `json:"hypothesisValidated"`
}

// Fault 单个故障注入描述。
type Fault struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

// SteadyState 稳态校验结果。
type SteadyState struct {
	Met     bool   `json:"met"`
	Message string `json:"message"`
}

// ExperimentResult 实验执行结果。
type ExperimentResult struct {
	Experiment      Experiment  `json:"experiment"`
	StartTime       string      `json:"startTime"`
	EndTime         string      `json:"endTime"`
	PreSteadyState  SteadyState `json:"preSteadyState"`
	PostSteadyState SteadyState `json:"postSteadyState"`
}

// NodeInfo 集群节点信息。
type NodeInfo struct {
	ID           string   `json:"id"`
	Address      string   `json:"address"`
	Role         string   `json:"role"`   // leader | follower | candidate
	Status       string   `json:"status"` // online | offline | leaving
	Capabilities []string `json:"capabilities"`
	LastSeen     string   `json:"lastSeen"`
}

// ClusterStatus 集群状态快照。
type ClusterStatus struct {
	Nodes        []NodeInfo `json:"nodes"`
	LeaderID     string     `json:"leaderId"`
	HashRingSize int        `json:"hashRingSize"`
	TotalShards  int        `json:"totalShards"`
}

// LearningStats 知识蒸馏统计。
type LearningStats struct {
	TotalInteractions   int `json:"totalInteractions"`
	TotalDistilled      int `json:"totalDistilled"`
	TotalKnowledgeItems int `json:"totalKnowledgeItems"`
}

// Capability 能力进化条目。
type Capability struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Score       float64 `json:"score"`
	TimesTested int     `json:"timesTested"`
	TimesPassed int     `json:"timesPassed"`
}

// PipelineStats 蒸馏管道统计。
type PipelineStats struct {
	TotalProcessed      int    `json:"totalProcessed"`
	TotalFactsWritten   int    `json:"totalFactsWritten"`
	TotalPatternsWritten int   `json:"totalPatternsWritten"`
	TotalRAGQueries     int    `json:"totalRAGQueries"`
	LastProcessTime     string `json:"lastProcessTime"`
}

// AgentTemplate 市场模板。
type AgentTemplate struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
	Author      string   `json:"author"`
	Category    string   `json:"category"`
	Tags        []string `json:"tags"`
	Rating      float64  `json:"rating"`
	Downloads   int      `json:"downloads"`
}

// CreateExperimentRequest POST /api/v1/chaos/experiments 请求体。
type CreateExperimentRequest struct {
	Name       string `json:"name"`
	Hypothesis string `json:"hypothesis"`
	FaultType  string `json:"faultType"`
}

// DeployRequest POST /api/v1/marketplace/deploy 请求体。
type DeployRequest struct {
	TemplateID string `json:"template_id"`
}

// ===== 服务接口（可注入真实引擎） =====

// ChaosService 混沌实验服务。
type ChaosService interface {
	ListExperiments(ctx context.Context) ([]ExperimentResult, error)
	CreateExperiment(ctx context.Context, req CreateExperimentRequest) error
}

// ClusterService 集群状态服务。
type ClusterService interface {
	Status(ctx context.Context) (*ClusterStatus, error)
}

// LearningService 学习监控服务。
type LearningService interface {
	Stats(ctx context.Context) (*LearningStats, error)
	Capabilities(ctx context.Context) ([]Capability, error)
	PipelineStats(ctx context.Context) (*PipelineStats, error)
}

// MarketplaceService 模板市场服务。
type MarketplaceService interface {
	SearchTemplates(ctx context.Context, query, category string) ([]AgentTemplate, error)
	Deploy(ctx context.Context, templateID string) error
}

// ===== StudioHandler =====

// StudioHandler 实现 Studio 四面板的 /api/v1/* HTTP 端点。
type StudioHandler struct {
	chaos      ChaosService
	cluster    ClusterService
	learning   LearningService
	marketplace MarketplaceService
	mux        *http.ServeMux
}

// Option 配置 StudioHandler。
type Option func(*StudioHandler)

// WithChaos 注入混沌实验服务（默认为 demo 实现）。
func WithChaos(s ChaosService) Option {
	return func(h *StudioHandler) { h.chaos = s }
}

// WithCluster 注入集群状态服务（默认为 demo 实现）。
func WithCluster(s ClusterService) Option {
	return func(h *StudioHandler) { h.cluster = s }
}

// WithLearning 注入学习监控服务（默认为 demo 实现）。
func WithLearning(s LearningService) Option {
	return func(h *StudioHandler) { h.learning = s }
}

// WithMarketplace 注入模板市场服务（默认为 demo 实现）。
func WithMarketplace(s MarketplaceService) Option {
	return func(h *StudioHandler) { h.marketplace = s }
}

// NewStudioHandler 创建 StudioHandler，默认挂载 demo 实现。
func NewStudioHandler(opts ...Option) *StudioHandler {
	h := &StudioHandler{
		chaos:       newDemoChaos(),
		cluster:     newDemoCluster(),
		learning:    newDemoLearning(),
		marketplace: newDemoMarketplace(),
		mux:         http.NewServeMux(),
	}
	for _, opt := range opts {
		opt(h)
	}
	h.registerRoutes()
	return h
}

// ServeHTTP 实现 http.Handler。
func (h *StudioHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

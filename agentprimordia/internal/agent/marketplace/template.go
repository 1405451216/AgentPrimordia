// Package marketplace 提供 Agent 模板市场。
//
// Stability: Stable
//
// 在现有 plugin_market.go 的基础上，扩展为 Agent 模板生态：
//   - AgentTemplate：Agent 模板定义（配置+tool集+系统提示+记忆策略）
//   - TemplateRegistry：模板注册表 + 搜索 + 评分
//   - Deployer：一键从模板部署运行 Agent
//   - Validator：模板配置校验 + 安全扫描
package marketplace

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// AgentTemplate Agent 模板定义
type AgentTemplate struct {
	// ID 模板唯一标识
	ID string `json:"id"`
	// Name 模板名称
	Name string `json:"name"`
	// Description 模板描述
	Description string `json:"description"`
	// Version 模板版本
	Version string `json:"version"`
	// Author 作者
	Author string `json:"author"`
	// Category 分类（"research"/"coding"/"analysis"/"chat"/"automation"）
	Category string `json:"category"`
	// Tags 标签
	Tags []string `json:"tags,omitempty"`
	// SystemPrompt 系统提示词
	SystemPrompt string `json:"system_prompt"`
	// DefaultProvider 默认 LLM Provider
	DefaultProvider string `json:"default_provider,omitempty"`
	// DefaultModel 默认模型
	DefaultModel string `json:"default_model,omitempty"`
	// MaxTurns 最大轮次
	MaxTurns int `json:"max_turns,omitempty"`
	// Tools 绑定的tool集
	Tools []string `json:"tools,omitempty"`
	// MemoryStrategy 记忆策略（"none"/"conversation"/"semantic"/"hybrid"）
	MemoryStrategy string `json:"memory_strategy,omitempty"`
	// Temperature 温度参数
	Temperature float64 `json:"temperature,omitempty"`
	// Config 额外配置（JSON）
	Config json.RawMessage `json:"config,omitempty"`
	// Rating 评分（0-5）
	Rating float64 `json:"rating"`
	// Downloads 下载次数
	Downloads int `json:"downloads"`
	// CreatedAt 创建时间
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt 更新时间
	UpdatedAt time.Time `json:"updated_at"`
}

// ValidationResult 模板验证结果
type ValidationResult struct {
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors,omitempty"`
	// SecurityWarnings 安全告警
	SecurityWarnings []string `json:"security_warnings,omitempty"`
}

// Validate 校验模板配置
func (t *AgentTemplate) Validate() *ValidationResult {
	result := &ValidationResult{Valid: true}

	if t.ID == "" {
		result.Valid = false
		result.Errors = append(result.Errors, "id is required")
	}
	if t.Name == "" {
		result.Valid = false
		result.Errors = append(result.Errors, "name is required")
	}
	if t.Version == "" {
		result.Valid = false
		result.Errors = append(result.Errors, "version is required")
	}
	if t.Author == "" {
		result.Valid = false
		result.Errors = append(result.Errors, "author is required")
	}
	if t.SystemPrompt == "" {
		result.Valid = false
		result.Errors = append(result.Errors, "system_prompt is required")
	}

	// 分类校验
	validCategories := map[string]bool{
		"research": true, "coding": true, "analysis": true,
		"chat": true, "automation": true,
	}
	if t.Category != "" && !validCategories[t.Category] {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("invalid category: %s", t.Category))
	}

	// 记忆策略校验
	validMemoryStrategies := map[string]bool{
		"": true, "none": true, "conversation": true,
		"semantic": true, "hybrid": true,
	}
	if !validMemoryStrategies[t.MemoryStrategy] {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("invalid memory_strategy: %s", t.MemoryStrategy))
	}

	// 温度参数校验
	if t.Temperature < 0 || t.Temperature > 2 {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("temperature must be 0-2, got %f", t.Temperature))
	}

	// 评分校验
	if t.Rating < 0 || t.Rating > 5 {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("rating must be 0-5, got %f", t.Rating))
	}

	// MaxTurns 校验
	if t.MaxTurns < 0 {
		result.Valid = false
		result.Errors = append(result.Errors, "max_turns must be non-negative")
	}

	// 安全扫描
	if strings.Contains(t.SystemPrompt, "rm -rf") {
		result.SecurityWarnings = append(result.SecurityWarnings, "system_prompt contains potentially dangerous command")
	}
	if strings.Contains(strings.ToLower(t.SystemPrompt), "ignore previous") {
		result.SecurityWarnings = append(result.SecurityWarnings, "system_prompt contains prompt injection pattern")
	}

	return result
}

// TemplateRegistry Agent 模板注册表
type TemplateRegistry struct {
	mu        sync.RWMutex
	templates map[string]*AgentTemplate
	ratings   map[string][]float64 // templateID -> ratings list
}

// NewTemplateRegistry 创建模板注册表
func NewTemplateRegistry() *TemplateRegistry {
	return &TemplateRegistry{
		templates: make(map[string]*AgentTemplate),
		ratings:   make(map[string][]float64),
	}
}

// Register 注册模板
func (r *TemplateRegistry) Register(tmpl *AgentTemplate) error {
	// 验证
	if vr := tmpl.Validate(); !vr.Valid {
		return fmt.Errorf("marketplace: template validation failed: %v", vr.Errors)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.templates[tmpl.ID]; exists {
		return fmt.Errorf("marketplace: template %q already exists", tmpl.ID)
	}

	now := time.Now()
	tmpl.CreatedAt = now
	tmpl.UpdatedAt = now

	r.templates[tmpl.ID] = tmpl
	return nil
}

// Update 更新模板
func (r *TemplateRegistry) Update(tmpl *AgentTemplate) error {
	if vr := tmpl.Validate(); !vr.Valid {
		return fmt.Errorf("marketplace: template validation failed: %v", vr.Errors)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.templates[tmpl.ID]; !exists {
		return fmt.Errorf("marketplace: template %q not found", tmpl.ID)
	}

	tmpl.UpdatedAt = time.Now()
	r.templates[tmpl.ID] = tmpl
	return nil
}

// Unregister 注销模板
func (r *TemplateRegistry) Unregister(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.templates[id]; !exists {
		return fmt.Errorf("marketplace: template %q not found", id)
	}

	delete(r.templates, id)
	delete(r.ratings, id)
	return nil
}

// Get 获取模板
func (r *TemplateRegistry) Get(id string) (*AgentTemplate, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tmpl, exists := r.templates[id]
	if !exists {
		return nil, false
	}
	cp := *tmpl
	return &cp, true
}

// Search 搜索模板
func (r *TemplateRegistry) Search(query, category string, tags []string) []*AgentTemplate {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*AgentTemplate
	for _, tmpl := range r.templates {
		// 分类过滤
		if category != "" && tmpl.Category != category {
			continue
		}

		// 标签过滤
		if len(tags) > 0 {
			matched := false
			for _, tag := range tags {
				for _, tTag := range tmpl.Tags {
					if strings.EqualFold(tag, tTag) {
						matched = true
						break
					}
				}
				if matched {
					break
				}
			}
			if !matched {
				continue
			}
		}

		// 关键词搜索
		if query != "" {
			queryLower := strings.ToLower(query)
			if !strings.Contains(strings.ToLower(tmpl.Name), queryLower) &&
				!strings.Contains(strings.ToLower(tmpl.Description), queryLower) &&
				!strings.Contains(strings.ToLower(tmpl.SystemPrompt), queryLower) {
				continue
			}
		}

		cp := *tmpl
		results = append(results, &cp)
	}

	return results
}

// List 列出所有模板
func (r *TemplateRegistry) List() []*AgentTemplate {
	r.mu.RLock()
	defer r.mu.RUnlock()

	results := make([]*AgentTemplate, 0, len(r.templates))
	for _, tmpl := range r.templates {
		cp := *tmpl
		results = append(results, &cp)
	}
	return results
}

// RateTemplate 对模板评分
func (r *TemplateRegistry) RateTemplate(id string, rating float64) error {
	if rating < 0 || rating > 5 {
		return fmt.Errorf("marketplace: rating must be 0-5, got %f", rating)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	tmpl, exists := r.templates[id]
	if !exists {
		return fmt.Errorf("marketplace: template %q not found", id)
	}

	r.ratings[id] = append(r.ratings[id], rating)

	// 重新计算平均评分
	var sum float64
	for _, r := range r.ratings[id] {
		sum += r
	}
	tmpl.Rating = sum / float64(len(r.ratings[id]))
	tmpl.UpdatedAt = time.Now()

	return nil
}

// IncrementDownloads 增加下载计数
func (r *TemplateRegistry) IncrementDownloads(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if tmpl, exists := r.templates[id]; exists {
		tmpl.Downloads++
	}
}

// TopByDownloads 按下载量排序获取前 N 个模板
func (r *TemplateRegistry) TopByDownloads(n int) []*AgentTemplate {
	r.mu.RLock()
	defer r.mu.RUnlock()

	all := r.List()

	// 简单选择排序（N 通常很小）
	for i := 0; i < n && i < len(all); i++ {
		maxIdx := i
		for j := i + 1; j < len(all); j++ {
			if all[j].Downloads > all[maxIdx].Downloads {
				maxIdx = j
			}
		}
		all[i], all[maxIdx] = all[maxIdx], all[i]
	}

	if n > len(all) {
		n = len(all)
	}
	return all[:n]
}

// TopByRating 按评分排序获取前 N 个模板
func (r *TemplateRegistry) TopByRating(n int) []*AgentTemplate {
	r.mu.RLock()
	defer r.mu.RUnlock()

	all := r.List()

	for i := 0; i < n && i < len(all); i++ {
		maxIdx := i
		for j := i + 1; j < len(all); j++ {
			if all[j].Rating > all[maxIdx].Rating {
				maxIdx = j
			}
		}
		all[i], all[maxIdx] = all[maxIdx], all[i]
	}

	if n > len(all) {
		n = len(all)
	}
	return all[:n]
}

// Deployer 模板部署器
type Deployer struct {
	registry *TemplateRegistry
}

// NewDeployer 创建部署器
func NewDeployer(registry *TemplateRegistry) *Deployer {
	return &Deployer{registry: registry}
}

// DeployConfig 部署配置
type DeployConfig struct {
	TemplateID       string
	ProviderOverride string
	ModelOverride    string
	MaxTurnsOverride int
	ConfigOverride   json.RawMessage
}

// DeployResult 部署结果
type DeployResult struct {
	Success    bool   `json:"success"`
	TemplateID string `json:"template_id"`
	Message    string `json:"message"`
	// AgentConfig 生成的 Agent 配置（JSON）
	AgentConfig json.RawMessage `json:"agent_config,omitempty"`
}

// Deploy 从模板部署 Agent
func (d *Deployer) Deploy(cfg DeployConfig) (*DeployResult, error) {
	tmpl, exists := d.registry.Get(cfg.TemplateID)
	if !exists {
		return &DeployResult{
			Success:    false,
			TemplateID: cfg.TemplateID,
			Message:    "template not found",
		}, nil
	}

	// 增加下载计数
	d.registry.IncrementDownloads(cfg.TemplateID)

	// 构建 Agent 配置
	agentConfig := map[string]any{
		"template_id":     tmpl.ID,
		"template_name":   tmpl.Name,
		"system_prompt":   tmpl.SystemPrompt,
		"provider":        tmpl.DefaultProvider,
		"model":           tmpl.DefaultModel,
		"max_turns":       tmpl.MaxTurns,
		"tools":           tmpl.Tools,
		"memory_strategy": tmpl.MemoryStrategy,
		"temperature":     tmpl.Temperature,
	}

	// 应用覆盖
	if cfg.ProviderOverride != "" {
		agentConfig["provider"] = cfg.ProviderOverride
	}
	if cfg.ModelOverride != "" {
		agentConfig["model"] = cfg.ModelOverride
	}
	if cfg.MaxTurnsOverride > 0 {
		agentConfig["max_turns"] = cfg.MaxTurnsOverride
	}
	if cfg.ConfigOverride != nil {
		agentConfig["extra_config"] = cfg.ConfigOverride
	}

	configJSON, err := json.Marshal(agentConfig)
	if err != nil {
		return nil, fmt.Errorf("marketplace: marshal agent config: %w", err)
	}

	return &DeployResult{
		Success:     true,
		TemplateID:  cfg.TemplateID,
		Message:     fmt.Sprintf("Agent deployed from template %q", tmpl.Name),
		AgentConfig: configJSON,
	}, nil
}

// Stability: Experimental — v3.0.0 新增 Agent 模板市场能力，API 可能随生态演进而调整。
package ap

import (
	"agentprimordia/internal/agent/marketplace"
)

// AgentTemplate Agent 模板定义（配置+tool集+系统提示+记忆策略）
type AgentTemplate = marketplace.AgentTemplate

// TemplateValidationResult 模板验证结果
type TemplateValidationResult = marketplace.ValidationResult

// TemplateRegistry Agent 模板注册表，支持注册、搜索、评分
type TemplateRegistry = marketplace.TemplateRegistry

// TemplateDeployer 模板部署器，一键从模板部署运行 Agent
type TemplateDeployer = marketplace.Deployer

// TemplateDeployConfig 部署配置
type TemplateDeployConfig = marketplace.DeployConfig

// TemplateDeployResult 部署结果
type TemplateDeployResult = marketplace.DeployResult

var (
	// NewTemplateRegistry 创建模板注册表
	NewTemplateRegistry = marketplace.NewTemplateRegistry
	// NewTemplateDeployer 创建模板部署器
	NewTemplateDeployer = marketplace.NewDeployer
)

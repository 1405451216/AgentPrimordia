// Stability: Stable — 健康检查端点。
package ap

import "agentprimordia/internal/health"

// HealthChecker 聚合健康检查器，处理 /healthz 和 /readyz 请求
type HealthChecker = health.HealthChecker

// HealthCheckable 健康检查接口，各组件实现此接口以注册到聚合检查器
type HealthCheckable = health.Checker

// NewHealthChecker 创建健康检查器
var NewHealthChecker = health.NewChecker

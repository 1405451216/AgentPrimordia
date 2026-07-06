// Package workflow 是 Workflow 编排模式的独立子包。
//
// Phase 3 Task 2 Step 2 的目标是把 internal/agent/workflow.go、
// internal/agent/workflow_engine.go、internal/agent/workflow_evaluator.go、
// internal/agent/workflow_executor.go、internal/agent/workflow_lifecycle.go
// 物理迁入本子包。
//
// 当前阶段（2026-07）仍按文件粒度聚合于 internal/agent/ 根目录。
// 下一阶段将在保留向后兼容类型别名（type WorkflowEngine = workflow.Engine 等）
// 的前提下完成物理迁移。
//
// 主要类型：
//   - Workflow：工作流定义
//   - WorkflowEngine：工作流执行引擎
//   - WorkflowEvaluator：状态求值器
//   - WorkflowExecutor：步骤执行器
//   - WorkflowLifecycle：生命周期管理
package workflow
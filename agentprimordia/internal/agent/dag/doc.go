// Package dag 是 DAG（有向无环图）编排模式的独立子包。
//
// Phase 3 Task 2 Step 1 的目标是把 internal/agent/dag.go、
// internal/agent/dag_builder.go、internal/agent/dag_delegate.go
// 物理迁入本子包。
//
// 当前阶段（2026-07）仍按文件粒度聚合于 internal/agent/ 根目录。
// 下一阶段将在保留向后兼容类型别名（type DAGNode = dag.Node 等）
// 的前提下完成物理迁移。
//
// 主要类型：
//   - DAGWorkflow：DAG 编排容器
//   - DAGNode / DAGEdge：图节点与边
//   - DAGExecutor：执行器
//   - DAGBuilder：流式构造器
package dag
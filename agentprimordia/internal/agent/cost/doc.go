// Package cost 是 LLM 成本追踪的独立子包。
//
// Phase 3 Task 2 Step 4 的目标是把 internal/agent/cost_tracker.go
// 物理迁入本子包。
//
// 当前阶段（2026-07）仍位于 internal/agent/cost_tracker.go。
// 下一阶段将迁入本子包并在 agent 包保留 type CostTracker = cost.Tracker 别名。
//
// 主要类型：
//   - CostTracker：成本追踪器
//   - CostEntry：单次 LLM 调用的成本条目
//   - CostSummary：聚合统计
package cost
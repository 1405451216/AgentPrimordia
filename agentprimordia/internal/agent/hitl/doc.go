// Package hitl 提供 Human-in-the-Loop（人在环）交互能力。
//
// Phase 3 Task 3 Step 4 的目标是把 internal/agent/hitl.go 物理迁入本子包。
//
// 当前阶段（2026-07）仍位于 internal/agent/hitl.go。
// 用于在 Agent 决策关键节点阻塞等待人类审批/反馈，常用于工具副作用确认场景。
//
// 主要导出：
//   - NewHumanApprovalChannel()：构造审批通道
//   - RequestApproval(ctx, prompt)：阻塞等待
//   - Respond(decision)：决策回调
package hitl
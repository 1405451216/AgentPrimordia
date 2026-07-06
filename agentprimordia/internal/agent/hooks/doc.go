// Package hooks 是 Agent 生命周期钩子的独立子包。
//
// Phase 3 Task 2 Step 3 的目标是把 internal/agent/hooks.go
// 物理迁入本子包。
//
// 当前阶段（2026-07）仍位于 internal/agent/hooks.go。
// 下一阶段将迁入本子包并在 agent 包保留 type HookManager = hooks.Manager 别名。
//
// 主要类型：
//   - HookManager：钩子注册中心
//   - HookFunc：钩子函数签名
//   - HookEvent：钩子事件枚举
package hooks
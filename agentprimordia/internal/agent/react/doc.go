// Package react 是 ReAct（Reasoning + Acting）循环的独立子包。
//
// Phase 3 Task 1 的目标是把 internal/agent/ 根目录下散落的 react_*.go 文件
// 物理迁移到本子包，并通过类型别名在 internal/agent/ 中保留向后兼容。
//
// ## 迁移路径
//
// 当前阶段（2026-07）已按文件粒度拆分到 react_loop_*.go / react_lifecycle.go /
// react_llm.go / react_rag.go / react_reasoning.go / react_persist.go /
// react_capabilities.go / react_convert.go（共 11 个文件，全部位于
// internal/agent/ 根目录），实现了"按文件聚合"的目标。
//
// 下一阶段（v1.4+）将进一步把这些文件物理迁入本子包：
//
//  1. 把 internal/agent/react_*.go 移动到 internal/agent/react/
//  2. 修改 package agent → package react
//  3. 在 internal/agent/ 中新增 react_alias.go 保留类型别名（如 type ReActAgent = react.ReActAgent）
//  4. 同步更新 internal/pool、internal/orchestration 等下游包的 import
//
// ## 拆分前须知
//
// 由于 react_*.go 内部大量引用同一个 package agent 的辅助类型（如
// ReActAgent / RecordStep / HookManager 等），完整迁出会触发跨包引用调整。
// 推荐路径：先用 build tag 隔离 react 包，逐步迁移，并在每一个 PR 完成后
// 运行 go test ./internal/agent/... 验证覆盖度不下降。
package react
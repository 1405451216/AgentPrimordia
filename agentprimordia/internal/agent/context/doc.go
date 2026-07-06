// Package context 提供 LLM 上下文窗口管理与压缩能力。
//
// Phase 3 Task 3 Step 5 的目标是把 internal/agent/context_compress.go、
// internal/agent/context_window.go 物理迁入本子包。
//
// 当前阶段（2026-07）仍位于 internal/agent/ 根目录。
// 用于在 LLM 调用前自动裁剪 / 压缩过长的上下文，避免超过 token 限额。
//
// 主要导出：
//   - Compress(messages, maxTokens)：滑动窗口压缩
//   - WindowSize() / Remaining()：上下文窗口状态
package context
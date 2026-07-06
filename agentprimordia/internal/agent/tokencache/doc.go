// Package tokencache 是 LLM Token 缓存工具包。
//
// Phase 3 Task 3 Step 2 的目标是把 internal/agent/tokencache.go 物理迁入本子包。
//
// 当前阶段（2026-07）仍位于 internal/agent/tokencache.go。
// 用于对相同 prompt 的 LLM 调用做结果缓存（命中率提升可降低 30%+ 成本）。
//
// 主要导出：
//   - New(WithCapacity, WithTTL)：构造
//   - Get(prompt string) / Set(prompt string, response string)：读写
//   - Stats() / Reset()：指标
package tokencache
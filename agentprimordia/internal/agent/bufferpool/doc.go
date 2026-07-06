// Package bufferpool 是 perf-v5 引入的字节/字符串 BufferPool 工具包。
//
// Phase 3 Task 3 Step 1 的目标是把 internal/agent/bufferpool.go 物理迁入本子包。
//
// 当前阶段（2026-07）仍位于 internal/agent/bufferpool.go。
// 在 ReAct 循环中用于减少 JSON 序列化过程中的字节切片分配，热点优化 -40% 以上。
//
// 主要导出：
//   - Get() / Put(*bytes.Buffer)：借/还 buffer
//   - GetString() / PutString(string)：借/还字符串 builder
package bufferpool
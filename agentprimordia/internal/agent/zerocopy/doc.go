// Package zerocopy 提供零拷贝字符串/字节转换工具。
//
// Phase 3 Task 3 Step 3 的目标是把 internal/agent/zerocopy.go 物理迁入本子包。
//
// 当前阶段（2026-07）仍位于 internal/agent/zerocopy.go。
// 用于在 JSON 序列化、HTTP 响应等场景下避免 string/[]byte 互转带来的内存分配。
//
// 主要导出：
//   - BytesToString([]byte) string：不拷贝
//   - StringToBytes(string) []byte：不拷贝（注意只读）
package zerocopy
// Stability: Stable — 文件锁与作用域校验。
package ap

import "agentprimordia/internal/concurrency"

// FileLockManager 是文件锁管理器，提供跨 Agent 的文件级互斥访问控制
type FileLockManager = concurrency.FileLockManager

var (
	// NewFileLockManager 创建文件锁管理器实例
	NewFileLockManager = concurrency.NewFileLockManager
	// ValidateScopes 验证 Agent 的文件作用域是否存在重叠冲突
	ValidateScopes = concurrency.ValidateScopes
)

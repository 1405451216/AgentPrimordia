package ap

import (
	"agentprimordia/internal/security"
)

// ACL 是访问控制列表，管理 Agent 对资源的访问权限
type ACL = security.ACL

// ACLRule 是 ACL 规则，定义 Agent 对特定资源的访问级别
type ACLRule = security.ACLRule

// AccessLevel 是访问级别（none / read / write / execute / all）
type AccessLevel = security.AccessLevel

// Sandbox 是沙箱环境，限制 Agent 可执行的命令和可访问的路径
type Sandbox = security.Sandbox

const (
	// AccessNone 表示无访问权限
	AccessNone = security.AccessNone
	// AccessRead 表示只读权限
	AccessRead = security.AccessRead
	// AccessWrite 表示读写权限
	AccessWrite = security.AccessWrite
	// AccessExecute 表示执行权限
	AccessExecute = security.AccessExecute
	// AccessAll 表示全部权限
	AccessAll = security.AccessAll
)

// NewACL 创建访问控制列表实例
var NewACL = security.NewACL

// NewSandbox 创建沙箱环境实例
var NewSandbox = security.NewSandbox

// Stability: 混合 —
//   内置工具（Tool / ToolRegistry / FileSystem / Shell / Web / KnowledgeSearch）
//   与 ScopePolicy: Stable。
//   MCP 客户端（MCPClient / MCPRegistry）: Experimental（协议仍在演进）。
//   插件（ToolPlugin / PluginLoader / PluginInfo）: Experimental。
package ap

import (
	"agentprimordia/internal/tools"
	"agentprimordia/internal/tools/builtin"
)

// Tool 是所有工具必须实现的接口，定义名称、描述、参数和执行方法
type Tool = tools.Tool

// ToolResult 是工具执行的结果，包含内容、错误标记和元数据
type ToolResult = tools.Result

// ToolRegistry 是工具注册中心，管理工具的注册、查找和权限配置
type ToolRegistry = tools.Registry

// ToolPermission 定义工具的访问控制规则，包括允许角色、禁止路径和确认回调
type ToolPermission = tools.Permission

// ToolExecutor 是工具执行器，处理工具调用、权限检查、文件锁和超时
type ToolExecutor = tools.Executor

// ToolFunctionCall 表示工具调用请求，包含调用 ID、工具名称和 JSON 参数
type ToolFunctionCall = tools.FunctionCall

// ScopePolicy 是作用域权限策略接口，定义 Agent 对资源的访问控制
type ScopePolicy = tools.ScopePolicy

// FileScopePolicy 是基于文件路径的作用域权限策略实现，按 Agent 分配可访问目录
type FileScopePolicy = tools.FileScopePolicy

// FileSystem 是文件系统操作工具，支持读写、搜索目录等操作，可注入权限策略和文件锁
type FileSystem = builtin.FileSystem

// Shell 是命令行执行工具，支持超时、白名单和作用域限制
type Shell = builtin.Shell

// Web 是 HTTP 请求工具，支持 GET/POST 等方法
type Web = builtin.Web

// KnowledgeSearcher 是知识库搜索接口，由外部注入 RAG Provider 实现
type KnowledgeSearcher = builtin.KnowledgeSearcher

// KnowledgeDoc 是知识库搜索返回的文档，包含 ID、内容、分数和来源
type KnowledgeDoc = builtin.KnowledgeDoc

// KnowledgeSearch 是内置知识库搜索工具，允许 Agent 主动查询知识库
type KnowledgeSearch = builtin.KnowledgeSearch

// ToolkitConfig 是工具包配置，包含根目录、启用的工具类型、权限策略和文件锁
type ToolkitConfig = builtin.ToolkitConfig

var (
	// NewToolRegistry 创建工具注册中心实例
	NewToolRegistry = tools.NewRegistry
	// NewToolExecutor 创建工具执行器实例
	NewToolExecutor = tools.NewExecutor
	// NewToolResult 创建工具执行成功结果
	NewToolResult = tools.NewResult
	// NewToolErrorResult 创建工具执行错误结果
	NewToolErrorResult = tools.NewErrorResult
	// NewFileScopePolicy 创建基于文件路径的作用域权限策略
	NewFileScopePolicy = tools.NewFileScopePolicy
	// NewScopeDeniedError 创建作用域拒绝错误
	NewScopeDeniedError = tools.NewScopeDeniedError
	// NewFileSystem 创建文件系统操作工具
	NewFileSystem = builtin.NewFileSystem
	// NewShell 创建命令行执行工具
	NewShell = builtin.NewShell
	// NewWeb 创建 HTTP 请求工具
	NewWeb = builtin.NewWeb
	// NewKnowledgeSearch 创建知识库搜索工具
	NewKnowledgeSearch = builtin.NewKnowledgeSearch
	// DefaultToolkit 创建默认工具包（文件系统 + Shell + Web），需指定根目录
	DefaultToolkit = builtin.DefaultToolkit
	// MinimalToolkit 创建最小工具包（文件系统 + Shell），需指定根目录
	MinimalToolkit = builtin.MinimalToolkit

	// NewMCPClient 创建 MCP 客户端
	NewMCPClient = tools.NewMCPClient
	// NewMCPRegistry 创建 MCP Server 注册中心
	NewMCPRegistry = tools.NewMCPRegistry
	// NewPluginLoader 创建插件加载器
	NewPluginLoader = tools.NewPluginLoader
)

// MCPClient 是 MCP 协议客户端，用于连接外部 MCP Server
type MCPClient = tools.MCPClient

// MCPRegistry 管理 MCP Server 的注册和生命周期
type MCPRegistry = tools.MCPRegistry

// MCPClientConfig 描述一个外部 MCP Server 的连接配置
type MCPClientConfig = tools.MCPClientConfig

// MCPToolDefinition 描述 MCP 服务端提供的工具
type MCPToolDefinition = tools.MCPToolDefinition

// ToolPlugin 是工具插件接口，支持批量注册工具
type ToolPlugin = tools.ToolPlugin

// PluginLoader 管理插件的加载、卸载和查询
type PluginLoader = tools.PluginLoader

// PluginInfo 描述已加载插件的信息
type PluginInfo = tools.PluginInfo

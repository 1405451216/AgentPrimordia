// Package ap — A2A（Agent-to-Agent）协议公共 API 导出。
//
// Stability: Experimental
//
// A2A 协议允许不同 Agent 之间通过 JSON-RPC 2.0 进行任务委派、状态查询
// 和事件订阅。本文件将 internal/agent/a2a 包中的核心类型与构造函数
// 通过类型别名导出，用户无需直接 import internal 包。

package ap

import (
	"log/slog"

	"agentprimordia/internal/agent/a2a"

	"google.golang.org/grpc"
)

// ============================================================================
// Agent Card 与元数据
// ============================================================================

// A2AAgentCard 描述 Agent 的能力、技能和端点信息
type A2AAgentCard = a2a.AgentCard

// A2AAgentSkill 描述 Agent 提供的一项技能
type A2AAgentSkill = a2a.AgentSkill

// A2AAgentCapabilities 描述 Agent 支持的能力（流式、推送等）
type A2AAgentCapabilities = a2a.AgentCapabilities

// A2AAgentEndpoints 描述 Agent 的服务端点
type A2AAgentEndpoints = a2a.AgentEndpoints

// NewA2AAgentCard 创建一张新的 Agent Card
func NewA2AAgentCard(agentID, name string) *A2AAgentCard {
	return a2a.NewAgentCard(agentID, name)
}

// ============================================================================
// Task 管理
// ============================================================================

// A2ATask 表示一个 A2A 任务
type A2ATask = a2a.Task

// A2ATaskState 表示任务状态
type A2ATaskState = a2a.TaskState

// A2ATaskStatus 表示任务状态结构
type A2ATaskStatus = a2a.TaskStatus

// A2ATaskFilter 任务过滤条件
type A2ATaskFilter = a2a.TaskFilter

// A2ATaskManager 任务管理器接口
type A2ATaskManager = a2a.TaskManager

// A2ATaskHandler 自定义任务处理器接口
type A2ATaskHandler = a2a.TaskHandler

// NewA2ATaskManager 创建默认的任务管理器实现
func NewA2ATaskManager() *a2a.TaskManagerImpl {
	return a2a.NewTaskManager()
}

// ============================================================================
// 消息与 Artifact
// ============================================================================

// A2AMessage A2A 协议消息
type A2AMessage = a2a.A2AMessage

// A2AArtifact 任务产物
type A2AArtifact = a2a.Artifact

// A2APart 消息内容片段接口
type A2APart = a2a.Part

// A2ATextPart 文本内容片段
type A2ATextPart = a2a.TextPart

// A2AFilePart 文件内容片段
type A2AFilePart = a2a.FilePart

// A2ADataPart 结构化数据片段
type A2ADataPart = a2a.DataPart

// NewA2ATextPart 创建文本片段
func NewA2ATextPart(text string) A2ATextPart {
	return a2a.NewTextPart(text)
}

// NewA2AFilePartFromURI 从 URI 创建文件片段
func NewA2AFilePartFromURI(uri, mime string) A2AFilePart {
	return a2a.NewFilePartFromURI(uri, mime)
}

// NewA2ADataPart 创建数据片段
func NewA2ADataPart(data []byte) A2ADataPart {
	return a2a.NewDataPart(data)
}

// ============================================================================
// Server
// ============================================================================

// A2AServer A2A 协议服务端
type A2AServer = a2a.A2AServer

// A2AServerOption 服务端配置选项
type A2AServerOption = a2a.ServerOption

// NewA2AServer 创建 A2A HTTP 服务端
func NewA2AServer(tm A2ATaskManager, opts ...A2AServerOption) *A2AServer {
	return a2a.NewA2AServer(tm, opts...)
}

// NewA2AServerWithService 使用已有的 A2AService 创建 A2A HTTP 服务端
func NewA2AServerWithService(service *A2AService, opts ...A2AServerOption) *A2AServer {
	return a2a.NewA2AServerWithService(service, opts...)
}

// ============================================================================
// Client
// ============================================================================

// A2AClient A2A 协议客户端
type A2AClient = a2a.A2AClient

// A2AClientOption 客户端配置选项
type A2AClientOption = a2a.ClientOption

// NewA2AClient 创建 A2A 客户端
func NewA2AClient(baseURL string, opts ...A2AClientOption) *A2AClient {
	return a2a.NewA2AClient(baseURL, opts...)
}

// ============================================================================
// 认证
// ============================================================================

// A2APrincipal 认证主体
type A2APrincipal = a2a.Principal

// A2AAuthenticator 认证器接口
type A2AAuthenticator = a2a.Authenticator

// A2ANoopAuthenticator 空认证器（不校验）
type A2ANoopAuthenticator = a2a.NoopAuthenticator

// A2AAPIKeyAuthenticator API Key 认证器
type A2AAPIKeyAuthenticator = a2a.APIKeyAuthenticator

// A2ABearerTokenAuthenticator Bearer Token 认证器
type A2ABearerTokenAuthenticator = a2a.BearerTokenAuthenticator

// A2ABearerTokenValidator Bearer Token 校验函数类型
type A2ABearerTokenValidator = a2a.BearerTokenValidator

// NewA2ANoopAuthenticator 创建空认证器
func NewA2ANoopAuthenticator() *A2ANoopAuthenticator {
	return a2a.NewNoopAuthenticator()
}

// NewA2AAPIKeyAuthenticator 创建 API Key 认证器
//
// keys: API Key -> 客户端 ID 的映射
// headerName: API Key 所在的 HTTP 头名称（如 "X-API-Key"）
func NewA2AAPIKeyAuthenticator(keys map[string]string, headerName string) *A2AAPIKeyAuthenticator {
	return a2a.NewAPIKeyAuthenticator(keys, headerName)
}

// NewA2ABearerTokenAuthenticator 创建 Bearer Token 认证器
func NewA2ABearerTokenAuthenticator(validate A2ABearerTokenValidator) *A2ABearerTokenAuthenticator {
	return a2a.NewBearerTokenAuthenticator(validate)
}

// ============================================================================
// 服务发现
// ============================================================================

// A2ADiscovery 服务发现接口
type A2ADiscovery = a2a.Discovery

// A2AAgentRegistry Agent 注册表
type A2AAgentRegistry = a2a.AgentRegistry

// A2ADiscoveryEvent 发现事件
type A2ADiscoveryEvent = a2a.DiscoveryEvent

// A2ADiscoveryEventType 发现事件类型
type A2ADiscoveryEventType = a2a.DiscoveryEventType

// A2ALocalDiscovery 本地服务发现实现
type A2ALocalDiscovery = a2a.LocalDiscovery

// NewA2ALocalDiscovery 创建本地服务发现实例
func NewA2ALocalDiscovery() *A2ALocalDiscovery {
	return a2a.NewLocalDiscovery()
}

// ============================================================================
// JSON-RPC
// ============================================================================

// A2AJSONRPCRequest JSON-RPC 2.0 请求
type A2AJSONRPCRequest = a2a.JSONRPCRequest

// A2AJSONRPCResponse JSON-RPC 2.0 响应
type A2AJSONRPCResponse = a2a.JSONRPCResponse

// A2AJSONRPCError JSON-RPC 2.0 错误
type A2AJSONRPCError = a2a.JSONRPCError

// ============================================================================
// 桥接
// ============================================================================

// A2AMessageBridge A2A 消息与内部 Message 的桥接器
type A2AMessageBridge = a2a.MessageBridge

// NewA2AMessageBridge 创建消息桥接器
func NewA2AMessageBridge() *A2AMessageBridge {
	return a2a.NewMessageBridge()
}

// ============================================================================
// gRPC Server / Client
// ============================================================================

// A2AService 传输无关的 A2A 业务核心
type A2AService = a2a.A2AService

// A2AServiceOption A2AService 配置选项
type A2AServiceOption = a2a.A2AServiceOption

// NewA2AService 创建 A2A 业务核心
func NewA2AService(card *A2AAgentCard, tm A2ATaskManager, opts ...A2AServiceOption) *A2AService {
	return a2a.NewA2AService(card, tm, opts...)
}

// A2ACreateTaskRequest 创建任务请求
type A2ACreateTaskRequest = a2a.CreateTaskRequest

// A2AGRPCServer A2A gRPC 服务端实现
type A2AGRPCServer = a2a.A2AGRPCServer

// A2AGRPCServerOption gRPC 服务端配置选项
type A2AGRPCServerOption = a2a.GRPCServerOption

// NewA2AGRPCServer 创建 A2A gRPC 服务端实现
func NewA2AGRPCServer(service *A2AService, opts ...A2AGRPCServerOption) *A2AGRPCServer {
	return a2a.NewA2AGRPCServer(service, opts...)
}

// NewA2AGRPCServerWithService 构造并返回已注册 A2A 服务的 *grpc.Server
func NewA2AGRPCServerWithService(service *A2AService, opts ...A2AGRPCServerOption) *grpc.Server {
	return a2a.NewGRPCServer(service, opts...)
}

// A2AGRPCClient A2A gRPC 客户端
type A2AGRPCClient = a2a.A2AGRPCClient

// A2AGRPCClientOption gRPC 客户端配置选项
type A2AGRPCClientOption = a2a.GRPCClientOption

// NewA2AGRPCClient 创建 A2A gRPC 客户端
func NewA2AGRPCClient(target string, opts ...A2AGRPCClientOption) (*A2AGRPCClient, error) {
	return a2a.NewA2AGRPCClient(target, opts...)
}

// NewA2AGRPCClientWithConn 使用已有 gRPC 连接创建客户端
func NewA2AGRPCClientWithConn(conn *grpc.ClientConn, opts ...A2AGRPCClientOption) *A2AGRPCClient {
	return a2a.NewA2AGRPCClientWithConn(conn, opts...)
}

// A2AGRPCAuthFunc gRPC 认证函数
type A2AGRPCAuthFunc = a2a.GRPCAuthFunc

// WithGRPCAuth 设置 gRPC server 认证函数
func WithGRPCAuth(auth A2AGRPCAuthFunc) A2AGRPCServerOption {
	return a2a.WithGRPCAuth(auth)
}

// WithGRPCLogger 设置 gRPC server 日志器
func WithGRPCLogger(logger *slog.Logger) A2AGRPCServerOption {
	return a2a.WithGRPCLogger(logger)
}

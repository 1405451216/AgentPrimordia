// Stability: Stable — 错误码与 sentinel 错误在 v1.0 前冻结，向后兼容。
package ap

import (
	"errors"

	"agentprimordia/internal/agent"
	"agentprimordia/internal/concurrency"
	"agentprimordia/internal/events"
	"agentprimordia/internal/llm"
	"agentprimordia/internal/memory"
	"agentprimordia/internal/persist"
	"agentprimordia/internal/pool"
	"agentprimordia/internal/security"
	"agentprimordia/internal/tools"
)

// CodeError 是带错误码的错误类型，用于结构化错误返回
type CodeError struct {
	Code    string
	Message string
}

// AggregatedError 聚合多个任务的错误信息，支持 errors.Is/errors.As 解包
type AggregatedError = pool.AggregatedError

// TaskError 单个任务的错误信息
type TaskError = pool.TaskError

// Error 返回 CodeError 的消息内容
func (e *CodeError) Error() string {
	return e.Message
}

// WithCode 创建带错误码的错误实例
func WithCode(code, message string) *CodeError {
	return &CodeError{Code: code, Message: message}
}

var (
	// ErrAgentStopped 表示 Agent 已被停止
	ErrAgentStopped = agent.ErrAgentStopped
	// ErrAgentRunning 表示 Agent 已在运行中
	ErrAgentRunning = errors.New("agent is already running")
	// ErrMaxTurnsExceeded 表示 Agent 超出最大推理轮次
	ErrMaxTurnsExceeded = agent.ErrMaxTurnsExceeded
	// ErrNoToolkit 表示 Agent 未配置工具包
	ErrNoToolkit = agent.ErrNoToolkit

	// ErrToolNotFound 表示请求的工具未注册
	ErrToolNotFound = tools.ErrToolNotFound
	// ErrToolExecution 表示工具执行失败
	ErrToolExecution = tools.ErrToolExecution
	// ErrConfirmDenied 表示工具确认被拒绝
	ErrConfirmDenied = tools.ErrConfirmDenied
	// ErrInvalidConfig 表示工具配置无效
	ErrInvalidConfig = tools.ErrInvalidConfig

	// ErrLLMCallFailed 表示 LLM 调用失败
	ErrLLMCallFailed = llm.ErrLLMCallFailed
	// ErrNotSupported 表示操作不被当前提供者支持
	ErrNotSupported = llm.ErrNotSupported
	// ErrCircuitOpen 表示熔断器已打开，请求被拒绝
	ErrCircuitOpen = llm.ErrCircuitOpen
	// ErrAPIKeyRequired 表示 API Key 未提供
	ErrAPIKeyRequired = llm.ErrAPIKeyRequired
	// ErrEmptyResponse 表示 LLM 返回空响应
	ErrEmptyResponse = llm.ErrEmptyResponse
	// ErrResponseParseFailed 表示 LLM 响应解析失败
	ErrResponseParseFailed = llm.ErrResponseParseFailed
	// ErrRetriesExhausted 表示所有重试已耗尽
	ErrRetriesExhausted = llm.ErrRetriesExhausted
	// ErrFallbackFailed 表示所有降级提供者均失败
	ErrFallbackFailed = llm.ErrFallbackFailed

	// ErrPoolFull 表示 Pool 已达最大容量
	ErrPoolFull = errors.New("pool is at max capacity")
	// ErrTaskNotFound 表示任务未找到
	ErrTaskNotFound = pool.ErrTaskNotFound
	// ErrTimeout 表示操作超时
	ErrTimeout = pool.ErrTimeout

	// ErrContextCanceled 表示上下文已取消
	ErrContextCanceled = errors.New("context canceled")

	// ErrEpisodeNotFound 表示记忆片段未找到
	ErrEpisodeNotFound = memory.ErrEpisodeNotFound
	// ErrInvalidImportance 表示重要性值无效
	ErrInvalidImportance = memory.ErrInvalidImportance
	// ErrEmptyEpisodeID 表示记忆片段 ID 为空
	ErrEmptyEpisodeID = memory.ErrEmptyEpisodeID
	// ErrEmptySessionID 表示会话 ID 为空
	ErrEmptySessionID = memory.ErrEmptySessionID
	// ErrEmptyRole 表示角色为空
	ErrEmptyRole = memory.ErrEmptyRole
	// ErrEmptyContent 表示内容为空
	ErrEmptyContent = memory.ErrEmptyContent
	// ErrDimensionMismatch 表示向量维度不匹配
	ErrDimensionMismatch = memory.ErrDimensionMismatch
	// ErrVectorNotFound 表示向量记录未找到
	ErrVectorNotFound = memory.ErrVectorNotFound

	// ErrCommandBlocked 表示命令被安全策略阻止
	ErrCommandBlocked = security.ErrCommandBlocked
	// ErrCommandNotAllowed 表示命令不在允许列表中
	ErrCommandNotAllowed = security.ErrCommandNotAllowed
	// ErrAccessDenied 表示访问被拒绝
	ErrAccessDenied = security.ErrAccessDenied
	// ErrPathTraversal 表示检测到路径遍历攻击
	ErrPathTraversal = security.ErrPathTraversal

	// ErrBusClosed 表示事件总线已关闭
	ErrBusClosed = events.ErrBusClosed

	// ErrCheckpointNotFound 表示检查点未找到
	ErrCheckpointNotFound = persist.ErrCheckpointNotFound

	// ErrGlobalWriteConflict 表示全局写入冲突
	ErrGlobalWriteConflict = concurrency.ErrGlobalWriteConflict
	// ErrScopeOverlap 表示作用域存在重叠
	ErrScopeOverlap = concurrency.ErrScopeOverlap
)

// GetErrorCode 从错误中提取结构化错误码，支持 sentinel 错误和 Code() 接口
// errorCodeMapping 将 sentinel 错误映射到结构化错误码，按模块分组
var errorCodeMapping = map[error]string{
	// --- Agent 错误 ---
	ErrAgentStopped:     "AGENT_001",
	ErrAgentRunning:     "AGENT_002",
	ErrMaxTurnsExceeded: "AGENT_003",
	ErrNoToolkit:        "AGENT_004",

	// --- Tool 错误 ---
	ErrToolNotFound:  "TOOL_001",
	ErrToolExecution: "TOOL_002",
	ErrInvalidConfig: "TOOL_003",
	ErrConfirmDenied: "TOOL_004",

	// --- LLM 错误 ---
	ErrLLMCallFailed:       "LLM_001",
	ErrNotSupported:        "LLM_002",
	ErrCircuitOpen:         "LLM_003",
	ErrAPIKeyRequired:      "LLM_004",
	ErrEmptyResponse:       "LLM_005",
	ErrResponseParseFailed: "LLM_006",
	ErrRetriesExhausted:    "LLM_007",
	ErrFallbackFailed:      "LLM_008",

	// --- Pool 错误 ---
	ErrPoolFull:     "POOL_001",
	ErrTaskNotFound: "POOL_002",
	ErrTimeout:      "POOL_003",

	// --- Context 错误 ---
	ErrContextCanceled: "CTX_001",

	// --- Memory 错误 ---
	ErrEpisodeNotFound:   "MEM_001",
	ErrInvalidImportance: "MEM_002",
	ErrEmptyEpisodeID:    "MEM_003",
	ErrEmptySessionID:    "MEM_004",
	ErrEmptyRole:         "MEM_005",
	ErrEmptyContent:      "MEM_006",
	ErrDimensionMismatch: "MEM_007",
	ErrVectorNotFound:    "MEM_008",

	// --- Security 错误 ---
	ErrCommandBlocked:    "SEC_001",
	ErrCommandNotAllowed: "SEC_002",
	ErrAccessDenied:      "SEC_003",
	ErrPathTraversal:     "SEC_004",

	// --- Event 错误 ---
	ErrBusClosed: "EVT_001",

	// --- Persistence 错误 ---
	ErrCheckpointNotFound: "PST_001",

	// --- Concurrency 错误 ---
	ErrGlobalWriteConflict: "CON_001",
	ErrScopeOverlap:        "CON_002",
}

func GetErrorCode(err error) string {
	type coded interface{ Code() string }
	var c coded
	if errors.As(err, &c) {
		return c.Code()
	}

	for sentinel, code := range errorCodeMapping {
		if errors.Is(err, sentinel) {
			return code
		}
	}

	return "UNKNOWN"
}

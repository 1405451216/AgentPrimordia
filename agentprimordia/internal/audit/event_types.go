package audit

// AuditAction 标准审计操作类型。
// 使用类型化常量确保日志事件的可比性和查询性能，
// 同时避免拼写错误（如 "llm.call" vs "llm.calls"）。
type AuditAction string

// 完整的审计事件类型枚举。
// 涵盖 Agent 生命周期、LLM 调用、tool调用、文件操作、网络请求、
// 权限变更、配置变更、Guardrail 拦截等所有合规相关事件。
const (
	// ===== Agent 生命周期 =====

	// ActionAgentStart Agent 启动
	ActionAgentStart AuditAction = "agent.start"
	// ActionAgentStop Agent 停止
	ActionAgentStop AuditAction = "agent.stop"
	// ActionAgentPanic Agent panic（异常退出）
	ActionAgentPanic AuditAction = "agent.panic"
	// ActionAgentResume Agent 从检查点恢复
	ActionAgentResume AuditAction = "agent.resume"
	// ActionAgentCheckpoint Agent 创建检查点
	ActionAgentCheckpoint AuditAction = "agent.checkpoint"

	// ===== LLM 调用 =====

	// ActionLLMCall LLM 同步调用
	ActionLLMCall AuditAction = "llm.call"
	// ActionLLMStream LLM 流式调用
	ActionLLMStream AuditAction = "llm.stream"
	// ActionLLMError LLM 调用错误
	ActionLLMError AuditAction = "llm.error"

	// ===== tool调用 =====

	// ActionToolCall tool调用
	ActionToolCall AuditAction = "tool.call"
	// ActionToolResult tool调用结果
	ActionToolResult AuditAction = "tool.result"
	// ActionToolError tool调用错误
	ActionToolError AuditAction = "tool.error"
	// ActionToolDenied tool调用被拒绝（权限不足）
	ActionToolDenied AuditAction = "tool.denied"

	// ===== 文件操作 =====

	// ActionFileRead 文件读取
	ActionFileRead AuditAction = "file.read"
	// ActionFileWrite 文件写入
	ActionFileWrite AuditAction = "file.write"
	// ActionFileDelete 文件删除
	ActionFileDelete AuditAction = "file.delete"

	// ===== 网络请求 =====

	// ActionHTTPRequest HTTP 请求
	ActionHTTPRequest AuditAction = "http.request"

	// ===== 权限变更 =====

	// ActionPermissionGrant 权限授予
	ActionPermissionGrant AuditAction = "permission.grant"
	// ActionPermissionRevoke 权限撤销
	ActionPermissionRevoke AuditAction = "permission.revoke"

	// ===== 配置变更 =====

	// ActionConfigChange 配置变更
	ActionConfigChange AuditAction = "config.change"

	// ===== Guardrail 拦截 =====

	// ActionGuardrailBlock Guardrail 拒绝（reject）
	ActionGuardrailBlock AuditAction = "guardrail.block"
	// ActionGuardrailSanitize Guardrail 脱敏（sanitize）
	ActionGuardrailSanitize AuditAction = "guardrail.sanitize"
	// ActionGuardrailFlag Guardrail 标记（flag）
	ActionGuardrailFlag AuditAction = "guardrail.flag"

	// ===== PII 处理 =====

	// ActionPIIDetected PII 被检测到
	ActionPIIDetected AuditAction = "pii.detected"
	// ActionPIIRedacted PII 被脱敏
	ActionPIIRedacted AuditAction = "pii.redacted"

	// ===== 审计自身 =====

	// ActionAuditExport 审计日志导出
	ActionAuditExport AuditAction = "audit.export"
	// ActionAuditQuery 审计日志查询
	ActionAuditQuery AuditAction = "audit.query"
)

// AuditResult 标准审计结果枚举
type AuditResult string

const (
	// ResultSuccess 操作成功
	ResultSuccess AuditResult = "success"
	// ResultDenied 操作被拒绝
	ResultDenied AuditResult = "denied"
	// ResultError 操作失败
	ResultError AuditResult = "error"
	// ResultBlocked 操作被拦截
	ResultBlocked AuditResult = "blocked"
)

// AllAuditActions 返回所有预定义的审计动作（用于枚举校验、文档生成等）
func AllAuditActions() []AuditAction {
	return []AuditAction{
		ActionAgentStart, ActionAgentStop, ActionAgentPanic, ActionAgentResume, ActionAgentCheckpoint,
		ActionLLMCall, ActionLLMStream, ActionLLMError,
		ActionToolCall, ActionToolResult, ActionToolError, ActionToolDenied,
		ActionFileRead, ActionFileWrite, ActionFileDelete,
		ActionHTTPRequest,
		ActionPermissionGrant, ActionPermissionRevoke,
		ActionConfigChange,
		ActionGuardrailBlock, ActionGuardrailSanitize, ActionGuardrailFlag,
		ActionPIIDetected, ActionPIIRedacted,
		ActionAuditExport, ActionAuditQuery,
	}
}

// IsValidAuditAction 检查给定的动作是否为标准预定义动作
func IsValidAuditAction(action AuditAction) bool {
	for _, a := range AllAuditActions() {
		if a == action {
			return true
		}
	}
	return false
}

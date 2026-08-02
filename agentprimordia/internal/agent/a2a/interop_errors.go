package a2a

import "fmt"

// v3.5 开放协议标准错误码对齐

// OpenErrorCode 开放规范 JSON-RPC 错误码
type OpenErrorCode int

const (
	// OpenErrParseError JSON 解析错误
	OpenErrParseError OpenErrorCode = -32700
	// OpenErrInvalidRequest 无效请求
	OpenErrInvalidRequest OpenErrorCode = -32600
	// OpenErrMethodNotFound 方法不存在
	OpenErrMethodNotFound OpenErrorCode = -32601
	// OpenErrInvalidParams 无效参数
	OpenErrInvalidParams OpenErrorCode = -32602
	// OpenErrInternal 内部错误
	OpenErrInternal OpenErrorCode = -32603
	// OpenErrTaskNotFound 任务不存在（扩展码）
	OpenErrTaskNotFound OpenErrorCode = -32001
	// OpenErrTaskAlreadyCanceled 任务已取消（扩展码）
	OpenErrTaskAlreadyCanceled OpenErrorCode = -32002
	// OpenErrPushNotSupported 不支持推送（扩展码）
	OpenErrPushNotSupported OpenErrorCode = -32003
	// OpenErrUnsupportedOperation 不支持的操作（扩展码）
	OpenErrUnsupportedOperation OpenErrorCode = -32004
)

// OpenError 开放规范错误结构
type OpenError struct {
	// Code 错误码
	Code OpenErrorCode `json:"code"`
	// Message 错误消息
	Message string `json:"message"`
	// Data 附加数据
	Data any `json:"data,omitempty"`
}

// Error 实现 error 接口
func (e *OpenError) Error() string {
	return fmt.Sprintf("a2a interop error %d: %s", e.Code, e.Message)
}

// NewOpenError 创建开放协议错误
func NewOpenError(code OpenErrorCode, message string) *OpenError {
	return &OpenError{Code: code, Message: message}
}

// StandardErrorMessage 返回标准错误码的默认消息
func StandardErrorMessage(code OpenErrorCode) string {
	switch code {
	case OpenErrParseError:
		return "Parse error"
	case OpenErrInvalidRequest:
		return "Invalid Request"
	case OpenErrMethodNotFound:
		return "Method not found"
	case OpenErrInvalidParams:
		return "Invalid params"
	case OpenErrInternal:
		return "Internal error"
	case OpenErrTaskNotFound:
		return "Task not found"
	case OpenErrTaskAlreadyCanceled:
		return "Task already canceled"
	case OpenErrPushNotSupported:
		return "Push notification not supported"
	case OpenErrUnsupportedOperation:
		return "Unsupported operation"
	default:
		return "Unknown error"
	}
}

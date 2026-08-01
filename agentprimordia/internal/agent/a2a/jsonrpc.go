package a2a

import (
	"encoding/json"
	"fmt"
)

// ===== JSON-RPC 错误码 =====

const (
	ErrCodeParseError     int = -32700
	ErrCodeInvalidRequest int = -32600
	ErrCodeMethodNotFound int = -32601
	ErrCodeInvalidParams  int = -32602
	ErrCodeInternalError  int = -32603
	ErrCodeTaskNotFound   int = -32000
	ErrCodeTaskConflict   int = -32001
	ErrCodeAuthFailed     int = -32002
	ErrCodeForbidden      int = -32003
)

// ===== JSON-RPC 数据结构 =====

type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

func (r *JSONRPCRequest) MarshalJSON() ([]byte, error) {
	type Alias JSONRPCRequest
	alias := (*Alias)(r)
	return json.Marshal(alias)
}

func (r *JSONRPCRequest) UnmarshalJSON(data []byte) error {
	type Alias JSONRPCRequest
	alias := &Alias{}
	if err := json.Unmarshal(data, alias); err != nil {
		return err
	}
	*r = JSONRPCRequest(*alias)
	if r.JSONRPC != "2.0" {
		return fmt.Errorf("unsupported JSON-RPC version: %s", r.JSONRPC)
	}
	return nil
}

type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data,omitempty"`
}

type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

func (r *JSONRPCResponse) MarshalJSON() ([]byte, error) {
	type Alias JSONRPCResponse
	alias := (*Alias)(r)
	return json.Marshal(alias)
}

func (r *JSONRPCResponse) UnmarshalJSON(data []byte) error {
	type Alias JSONRPCResponse
	alias := &Alias{}
	if err := json.Unmarshal(data, alias); err != nil {
		return err
	}
	*r = JSONRPCResponse(*alias)
	return nil
}

// ===== 构造函数 =====

func NewJSONRPCResult(id interface{}, result json.RawMessage) *JSONRPCResponse {
	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
}

func NewJSONRPCError(id interface{}, code int, msg, data string) *JSONRPCResponse {
	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &JSONRPCError{
			Code:    code,
			Message: msg,
			Data:    data,
		},
	}
}

func NewInvalidRequestError() *JSONRPCResponse {
	return NewJSONRPCError(nil, ErrCodeInvalidRequest, "无效请求", "")
}

func NewMethodNotFoundError(method string) *JSONRPCResponse {
	return NewJSONRPCError(nil, ErrCodeMethodNotFound, fmt.Sprintf("方法不存在: %s", method), "")
}

func NewParamsError(msg string) *JSONRPCResponse {
	return NewJSONRPCError(nil, ErrCodeInvalidParams, msg, "")
}

func NewAuthFailedError(msg string) *JSONRPCResponse {
	return NewJSONRPCError(nil, ErrCodeAuthFailed, msg, "")
}

func NewTaskNotFoundError(taskID string) *JSONRPCResponse {
	return NewJSONRPCError(nil, ErrCodeTaskNotFound, fmt.Sprintf("task not found: %s", taskID), "")
}

func NewTaskConflictError(from, to TaskState) *JSONRPCResponse {
	return NewJSONRPCError(nil, ErrCodeTaskConflict,
		fmt.Sprintf("illegal state transition: %s -> %s", from, to), "")
}

func NewInternalError(msg string) *JSONRPCResponse {
	return NewJSONRPCError(nil, ErrCodeInternalError, msg, "")
}

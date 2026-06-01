package a2a

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestJSONRPCRequest_Marshal(t *testing.T) {
	req := &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tasks/send",
		Params:  json.RawMessage(`{"task":{"id":"t1"}}`),
	}

	data, err := req.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON 失败: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if m["jsonrpc"] != "2.0" {
		t.Errorf("jsonrpc 字段错误: got %v", m["jsonrpc"])
	}
	if m["method"] != "tasks/send" {
		t.Errorf("method 字段错误: got %v", m["method"])
	}
	if m["id"].(float64) != 1 {
		t.Errorf("id 字段错误: got %v", m["id"])
	}
}

func TestJSONRPCRequest_MarshalNoID(t *testing.T) {
	req := &JSONRPCRequest{JSONRPC: "2.0", Method: "tasks/send"}
	data, _ := req.MarshalJSON()
	var m map[string]any
	json.Unmarshal(data, &m)
	if _, hasID := m["id"]; hasID {
		t.Error("通知请求不应包含 id 字段")
	}
}

func TestJSONRPCRequest_Unmarshal(t *testing.T) {
	raw := `{"jsonrpc":"2.0","id":1,"method":"tasks/get","params":{"task_id":"t1"}}`
	var req JSONRPCRequest
	if err := req.UnmarshalJSON([]byte(raw)); err != nil {
		t.Fatalf("UnmarshalJSON 失败: %v", err)
	}
	if req.Method != "tasks/get" {
		t.Errorf("Method 错误: got %s", req.Method)
	}
	if req.ID.(float64) != 1 {
		t.Errorf("ID 错误: got %v", req.ID)
	}
}

func TestJSONRPCRequest_InvalidVersion(t *testing.T) {
	raw := `{"jsonrpc":"1.0","id":1,"method":"test"}`
	var req JSONRPCRequest
	err := req.UnmarshalJSON([]byte(raw))
	if err == nil {
		t.Fatal("非 2.0 版本应返回错误")
	}
	if !strings.Contains(err.Error(), "不支持的 JSON-RPC 版本") {
		t.Errorf("错误信息不符合预期: %v", err)
	}
}

func TestJSONRPCRequest_InvalidJSON(t *testing.T) {
	raw := `{invalid json`
	var req JSONRPCRequest
	err := req.UnmarshalJSON([]byte(raw))
	if err == nil {
		t.Fatal("无效 JSON 应返回错误")
	}
}

func TestJSONRPCResponse_Error(t *testing.T) {
	resp := NewJSONRPCError(1, ErrCodeParseError, "解析错误", "额外信息")
	data, _ := json.Marshal(resp)

	var raw map[string]any
	json.Unmarshal(data, &raw)

	if raw["error"] == nil {
		t.Fatal("error 字段不应为空")
	}
	errMap := raw["error"].(map[string]any)
	if errMap["code"].(float64) != float64(ErrCodeParseError) {
		t.Errorf("error code 错误: got %v", errMap["code"])
	}
	if errMap["message"] != "解析错误" {
		t.Errorf("error message 错误: got %v", errMap["message"])
	}
	if errMap["data"] != "额外信息" {
		t.Errorf("error data 错误: got %v", errMap["data"])
	}
}

func TestJSONRPCResponse_Success(t *testing.T) {
	result := json.RawMessage(`{"task":{"id":"t1","state":"completed"}}`)
	resp := NewJSONRPCResult(1, result)
	data, _ := json.Marshal(resp)

	var raw map[string]any
	json.Unmarshal(data, &raw)

	if strings.Contains(string(data), `"error"`) {
		t.Error("成功响应不应包含 error 字段")
	}
	if !strings.Contains(string(data), `"result"`) {
		t.Error("成功响应应包含 result 字段")
	}
	if raw["id"].(float64) != 1 {
		t.Errorf("id 应为 1, got %v", raw["id"])
	}
}

func TestNewInvalidRequestError(t *testing.T) {
	resp := NewInvalidRequestError()
	if resp.Error == nil {
		t.Fatal("应有 error 字段")
	}
	if resp.Error.Code != ErrCodeInvalidRequest {
		t.Errorf("错误码应为 %d, got %d", ErrCodeInvalidRequest, resp.Error.Code)
	}
	if resp.ID != nil {
		t.Error("无效请求的 ID 应为 nil")
	}
}

func TestNewMethodNotFoundError(t *testing.T) {
	resp := NewMethodNotFoundError("test/method")
	if resp.Error.Code != ErrCodeMethodNotFound {
		t.Errorf("错误码应为 %d, got %d", ErrCodeMethodNotFound, resp.Error.Code)
	}
	if !strings.Contains(resp.Error.Message, "test/method") {
		t.Errorf("消息应包含方法名: %s", resp.Error.Message)
	}
}

func TestNewParamsError(t *testing.T) {
	resp := NewParamsError("缺少 task 参数")
	if resp.Error.Code != ErrCodeInvalidParams {
		t.Errorf("错误码应为 %d, got %d", ErrCodeInvalidParams, resp.Error.Code)
	}
}

func TestNewAuthFailedError(t *testing.T) {
	resp := NewAuthFailedError("API Key 无效")
	if resp.Error.Code != ErrCodeAuthFailed {
		t.Errorf("错误码应为 %d, got %d", ErrCodeAuthFailed, resp.Error.Code)
	}
}

func TestNewTaskNotFoundError(t *testing.T) {
	resp := NewTaskNotFoundError("task-xyz")
	if resp.Error.Code != ErrCodeTaskNotFound {
		t.Errorf("错误码应为 %d, got %d", ErrCodeTaskNotFound, resp.Error.Code)
	}
	if !strings.Contains(resp.Error.Message, "task-xyz") {
		t.Errorf("消息应包含任务ID: %s", resp.Error.Message)
	}
}

func TestNewTaskConflictError(t *testing.T) {
	resp := NewTaskConflictError(TaskSubmitted, TaskCompleted)
	if resp.Error.Code != ErrCodeTaskConflict {
		t.Errorf("错误码应为 %d, got %d", ErrCodeTaskConflict, resp.Error.Code)
	}
	msg := resp.Error.Message
	if !strings.Contains(msg, "submitted") || !strings.Contains(msg, "completed") {
		t.Errorf("消息应包含状态名: %s", msg)
	}
}

func TestNewInternalError(t *testing.T) {
	resp := NewInternalError("内部服务异常")
	if resp.Error.Code != ErrCodeInternalError {
		t.Errorf("错误码应为 %d, got %d", ErrCodeInternalError, resp.Error.Code)
	}
}

func TestJSONRPCResponse_Unmarshal(t *testing.T) {
	raw := `{"jsonrpc":"2.0","id":42,"result":{"ok":true}}`
	var resp JSONRPCResponse
	if err := resp.UnmarshalJSON([]byte(raw)); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if resp.ID.(float64) != 42 {
		t.Errorf("ID 错误: got %v", resp.ID)
	}
	if resp.Result == nil {
		t.Fatal("result 不应为空")
	}
}

func TestJSONRPCResponse_RoundTrip(t *testing.T) {
	original := NewJSONRPCResult(99, json.RawMessage(`{"status":"ok"}`))
	data, _ := original.MarshalJSON()

	var decoded JSONRPCResponse
	if err := decoded.UnmarshalJSON(data); err != nil {
		t.Fatalf("往返失败: %v", err)
	}
	if decoded.ID.(float64) != 99 {
		t.Errorf("ID 往返不一致: got %v", decoded.ID)
	}
	if string(decoded.Result) != `{"status":"ok"}` {
		t.Errorf("Result 往返不一致: got %s", string(decoded.Result))
	}
}

func TestErrorCodeConstants(t *testing.T) {
	tests := []struct {
		code     int
		expected int
	}{
		{ErrCodeParseError, -32700},
		{ErrCodeInvalidRequest, -32600},
		{ErrCodeMethodNotFound, -32601},
		{ErrCodeInvalidParams, -32602},
		{ErrCodeInternalError, -32603},
		{ErrCodeTaskNotFound, -32000},
		{ErrCodeTaskConflict, -32001},
		{ErrCodeAuthFailed, -32002},
		{ErrCodeForbidden, -32003},
	}
	for _, tt := range tests {
		if tt.code != tt.expected {
			t.Errorf("错误码常量值错误: got %d, want %d", tt.code, tt.expected)
		}
	}
}

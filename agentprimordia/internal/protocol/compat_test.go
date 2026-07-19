package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestAgentMessageSerialization 验证 AgentMessage 序列化为标准 JSON。
func TestAgentMessageSerialization(t *testing.T) {
	msg := &AgentMessage{
		ID:      "msg-001",
		Role:    RoleUser,
		Content: "Hello, world!",
		Metadata: map[string]string{
			"lang": "zh",
		},
		Timestamp: 1700000000000,
	}

	data, err := msg.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	// 验证 JSON 包含预期字段
	jsonStr := string(data)
	checks := []string{
		`"id":"msg-001"`,
		`"role":"user"`,
		`"content":"Hello, world!"`,
		`"metadata":{"lang":"zh"}`,
		`"timestamp":1700000000000`,
	}
	for _, c := range checks {
		if !strings.Contains(jsonStr, c) {
			t.Errorf("JSON should contain %s, got: %s", c, jsonStr)
		}
	}

	// 验证 tool_calls 字段为空时不存在于 JSON
	if strings.Contains(jsonStr, "tool_calls") {
		t.Errorf("tool_calls should be omitted when empty, got: %s", jsonStr)
	}
}

// TestAgentMessageDeserialization 验证 JSON 反序列化。
func TestAgentMessageDeserialization(t *testing.T) {
	jsonStr := `{
		"id": "msg-002",
		"role": "assistant",
		"content": "I can help you.",
		"tool_calls": [
			{"id": "tc-1", "name": "search", "arguments": "{\"query\":\"go\"}"}
		],
		"metadata": {"model": "gpt-4"},
		"timestamp": 1700000001000
	}`

	msg, err := AgentMessageFromJSON([]byte(jsonStr))
	if err != nil {
		t.Fatalf("AgentMessageFromJSON failed: %v", err)
	}

	if msg.ID != "msg-002" {
		t.Errorf("ID = %q, want %q", msg.ID, "msg-002")
	}
	if msg.Role != RoleAssistant {
		t.Errorf("Role = %q, want %q", msg.Role, RoleAssistant)
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("ToolCalls len = %d, want 1", len(msg.ToolCalls))
	}
	if msg.ToolCalls[0].Name != "search" {
		t.Errorf("ToolCall.Name = %q, want %q", msg.ToolCalls[0].Name, "search")
	}
}

// TestAgentMessageRoundTrip 验证序列化↔反序列化往返一致。
func TestAgentMessageRoundTrip(t *testing.T) {
	original := &AgentMessage{
		ID:      GenerateID(),
		Role:    RoleAssistant,
		Content: "The answer is 42.",
		ToolCalls: []ToolCall{
			{ID: "tc-abc", Name: "calculator", Arguments: `{"expr":"6*7"}`},
			{ID: "tc-def", Name: "web_search", Arguments: `{"query":"meaning of life"}`},
		},
		Metadata:  map[string]string{"source": "test", "version": "1"},
		Timestamp: Now(),
	}

	data, err := original.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	restored, err := AgentMessageFromJSON(data)
	if err != nil {
		t.Fatalf("AgentMessageFromJSON failed: %v", err)
	}

	if restored.ID != original.ID {
		t.Errorf("ID mismatch: %q vs %q", restored.ID, original.ID)
	}
	if restored.Role != original.Role {
		t.Errorf("Role mismatch")
	}
	if restored.Content != original.Content {
		t.Errorf("Content mismatch: %q vs %q", restored.Content, original.Content)
	}
	if len(restored.ToolCalls) != len(original.ToolCalls) {
		t.Fatalf("ToolCalls length mismatch: %d vs %d", len(restored.ToolCalls), len(original.ToolCalls))
	}
	for i := range original.ToolCalls {
		if restored.ToolCalls[i].ID != original.ToolCalls[i].ID {
			t.Errorf("ToolCalls[%d].ID mismatch", i)
		}
		if restored.ToolCalls[i].Name != original.ToolCalls[i].Name {
			t.Errorf("ToolCalls[%d].Name mismatch", i)
		}
		if restored.ToolCalls[i].Arguments != original.ToolCalls[i].Arguments {
			t.Errorf("ToolCalls[%d].Arguments mismatch", i)
		}
	}
	if restored.Timestamp != original.Timestamp {
		t.Errorf("Timestamp mismatch: %d vs %d", restored.Timestamp, original.Timestamp)
	}
}

// TestAgentMessageValidate 验证消息校验逻辑。
func TestAgentMessageValidate(t *testing.T) {
	tests := []struct {
		name    string
		msg     *AgentMessage
		wantErr bool
	}{
		{
			name: "valid user message",
			msg:  &AgentMessage{ID: "1", Role: RoleUser, Content: "hi", Timestamp: 1},
		},
		{
			name: "valid assistant with tool_calls",
			msg:  &AgentMessage{ID: "2", Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "tc1", Name: "tool"}}, Timestamp: 1},
		},
		{
			name:    "missing id",
			msg:     &AgentMessage{Role: RoleUser, Content: "hi", Timestamp: 1},
			wantErr: true,
		},
		{
			name:    "invalid role",
			msg:     &AgentMessage{ID: "3", Role: "ghost", Content: "hi", Timestamp: 1},
			wantErr: true,
		},
		{
			name:    "empty content and no tool_calls",
			msg:     &AgentMessage{ID: "4", Role: RoleAssistant, Timestamp: 1},
			wantErr: true,
		},
		{
			name: "tool_call missing id",
			msg: &AgentMessage{
				ID:        "5",
				Role:      RoleAssistant,
				ToolCalls: []ToolCall{{Name: "tool"}},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.msg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestToolResultSerialization 验证 ToolResult JSON 兼容性。
func TestToolResultSerialization(t *testing.T) {
	tr := &ToolResult{
		ToolCallID: "tc-1",
		Result:     `{"status":"ok","data":"hello"}`,
		IsError:    false,
	}

	data, err := json.Marshal(tr)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// 验证字段名
	expected := `"tool_call_id":"tc-1"`
	if !strings.Contains(string(data), expected) {
		t.Errorf("JSON should contain %s, got: %s", expected, string(data))
	}

	// 反序列化
	var restored ToolResult
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if restored.ToolCallID != tr.ToolCallID {
		t.Errorf("ToolCallID mismatch")
	}
	if restored.IsError != tr.IsError {
		t.Errorf("IsError mismatch")
	}
}

// TestEventMessageSerialization 验证 EventMessage JSON 格式。
func TestEventMessageSerialization(t *testing.T) {
	evt := &EventMessage{
		ID:        "evt-1",
		Type:      "tool_call",
		Source:    "agent-executor",
		Payload:   `{"tool":"search","args":{"query":"test"}}`,
		Timestamp: Now(),
		Metadata:  map[string]string{"session": "sess-1"},
	}

	data, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var restored EventMessage
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if restored.ID != evt.ID {
		t.Errorf("ID mismatch")
	}
	if restored.Type != evt.Type {
		t.Errorf("Type mismatch")
	}
	if restored.Payload != evt.Payload {
		t.Errorf("Payload mismatch")
	}
}

// TestCompactJSON 验证 JSON 压缩工具。
func TestCompactJSON(t *testing.T) {
	input := `{
		"id": "msg-1",
		"content": "HelloWorld"
	}`

	result := CompactJSON(input)
	// 紧凑后不应含空白
	if strings.ContainsAny(result, " \t\n\r") {
		t.Errorf("CompactJSON should remove whitespace, got: %s", result)
	}
	// 字段内容应保留
	if !strings.Contains(result, `"content":"HelloWorld"`) {
		t.Errorf("String content should be preserved, got: %s", result)
	}
}

// TestCrossLanguageCompat 验证 Go 序列化的 JSON 能被标准 JSON 解析器兼容消费。
// TS 端遵循相同的字段命名约定（camelCase json tag）。
func TestCrossLanguageCompat(t *testing.T) {
	msg := &AgentMessage{
		ID:      "compat-1",
		Role:    RoleAssistant,
		Content: "Cross-language test",
		ToolCalls: []ToolCall{
			{ID: "tc-compat", Name: "echo", Arguments: `{"msg":"hi"}`},
		},
		Metadata:  map[string]string{"phase": "B"},
		Timestamp: 1700000000000,
	}

	data, err := msg.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	// 用通用的 map 解码验证结构
	var generic map[string]interface{}
	if err := json.Unmarshal(data, &generic); err != nil {
		t.Fatalf("Unmarshal to map failed: %v", err)
	}

	// 验证顶层字段存在且类型正确
	if _, ok := generic["id"].(string); !ok {
		t.Error("id should be string")
	}
	if _, ok := generic["role"].(string); !ok {
		t.Error("role should be string")
	}
	if _, ok := generic["content"].(string); !ok {
		t.Error("content should be string")
	}
	if ts, ok := generic["timestamp"].(float64); !ok || ts != 1700000000000 {
		t.Error("timestamp should be number 1700000000000")
	}

	// 验证 tool_calls 是数组
	tcs, ok := generic["tool_calls"].([]interface{})
	if !ok {
		t.Fatal("tool_calls should be array")
	}
	if len(tcs) != 1 {
		t.Fatalf("tool_calls length = %d, want 1", len(tcs))
	}
	tcMap := tcs[0].(map[string]interface{})
	if tcMap["name"] != "echo" {
		t.Errorf("tool_call name = %v, want echo", tcMap["name"])
	}
}

package a2a

import (
	"encoding/json"
	"testing"
)

func TestAgentCard_MarshalUnmarshal(t *testing.T) {
	card := &AgentCard{
		Protocol:    "a2a",
		AgentID:     "agent-001",
		Name:        "DataAnalyst",
		Description: "数据分析专家",
		Capabilities: AgentCapabilities{
			InputModes:  []string{"text", "application/csv"},
			OutputModes: []string{"text", "application/pdf"},
			Streaming:   true,
		},
		Endpoints: AgentEndpoints{
			BaseURL:       "https://example.com/a2a",
			TaskSend:      "/tasks/send",
			TaskGet:       "/tasks/{id}",
			TaskCancel:    "/tasks/{id}/cancel",
			TaskSubscribe: "/tasks/{id}/events",
		},
		SecuritySchemes: []SecurityScheme{
			{Scheme: AuthAPIKey, In: "header", Name: "X-API-Key"},
		},
	}

	data, err := card.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON 失败: %v", err)
	}

	var decoded AgentCard
	if err := decoded.UnmarshalJSON(data); err != nil {
		t.Fatalf("UnmarshalJSON 失败: %v", err)
	}

	if decoded.AgentID != card.AgentID {
		t.Errorf("AgentID 不匹配: got %s, want %s", decoded.AgentID, card.AgentID)
	}
	if !decoded.Capabilities.Streaming {
		t.Error("Streaming 应为 true")
	}
	if decoded.Endpoints.TaskSend != "/tasks/send" {
		t.Errorf("TaskSend 不匹配: got %s", decoded.Endpoints.TaskSend)
	}
	if len(decoded.SecuritySchemes) != 1 || decoded.SecuritySchemes[0].Scheme != AuthAPIKey {
		t.Error("SecuritySchemes 解析错误")
	}
}

func TestAgentCard_NewDefault(t *testing.T) {
	card := NewAgentCard("test-agent", "TestAgent")
	if card.Protocol != "a2a" {
		t.Errorf("默认协议应为 a2a, got %s", card.Protocol)
	}
	if card.AgentID != "test-agent" {
		t.Errorf("AgentID 不匹配: got %s", card.AgentID)
	}

	data, _ := json.Marshal(card)
	var m map[string]string
	_ = json.Unmarshal(data, &m)
	if m["protocol"] != "a2a" {
		t.Error("序列化后 protocol 应为 a2a")
	}
}

func TestTaskState_ValidTransitions(t *testing.T) {
	tests := []struct {
		from TaskState
		to   TaskState
		want bool
	}{
		{TaskSubmitted, TaskWorking, true},
		{TaskSubmitted, TaskRejected, true},
		{TaskSubmitted, TaskCanceled, true},
		{TaskSubmitted, TaskCompleted, false},
		{TaskWorking, TaskCompleted, true},
		{TaskWorking, TaskFailed, true},
		{TaskWorking, TaskCanceled, true},
		{TaskWorking, TaskInputRequired, true},
		{TaskWorking, TaskSubmitted, false},
		{TaskInputRequired, TaskWorking, true},
		{TaskInputRequired, TaskCanceled, true},
		{TaskInputRequired, TaskCompleted, false},
		{TaskCompleted, TaskWorking, false},
		{TaskFailed, TaskWorking, false},
		{TaskCanceled, TaskWorking, false},
		{TaskRejected, TaskWorking, false},
	}

	for _, tt := range tests {
		got := IsValidTransition(tt.from, tt.to)
		if got != tt.want {
			t.Errorf("IsValidTransition(%s, %s) = %v, want %v", tt.from, tt.to, got, tt.want)
		}
	}
}

func TestIsTerminal(t *testing.T) {
	terminalStates := []TaskState{TaskCompleted, TaskFailed, TaskCanceled, TaskRejected}
	nonTerminal := []TaskState{TaskSubmitted, TaskWorking, TaskInputRequired}

	for _, s := range terminalStates {
		if !IsTerminal(s) {
			t.Errorf("%s 应为终态", s)
		}
	}
	for _, s := range nonTerminal {
		if IsTerminal(s) {
			t.Errorf("%s 不应为终态", s)
		}
	}
}

func TestTextPart_Marshal(t *testing.T) {
	p := NewTextPart("hello world")
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	var decoded map[string]string
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if decoded["type"] != "text" || decoded["text"] != "hello world" {
		t.Errorf("TextPart 序列化结果错误: %s", string(data))
	}
}

func TestFilePart_Marshal(t *testing.T) {
	p := NewFilePartFromURI("https://example.com/file.pdf", "application/pdf")
	data, _ := json.Marshal(p)
	var decoded map[string]any
	_ = json.Unmarshal(data, &decoded)
	if decoded["type"] != "file" {
		t.Errorf("FilePart type 错误: %v", decoded["type"])
	}
}

func TestDataPart_Marshal(t *testing.T) {
	p := NewDataPart(json.RawMessage(`{"key":"value"}`))
	data, _ := json.Marshal(p)
	var decoded map[string]any
	_ = json.Unmarshal(data, &decoded)
	if decoded["type"] != "data" {
		t.Error("DataPart type 错误")
	}
}

func TestA2AMessage_MarshalUnmarshal(t *testing.T) {
	msg := &A2AMessage{
		Role:      "user",
		MessageID: "msg-001",
		Parts: []Part{
			NewTextPart("你好"),
			NewFilePartFromURI("https://x.com/f.pdf", "application/pdf"),
		},
	}

	data, err := msg.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON 失败: %v", err)
	}

	var decoded A2AMessage
	if err := decoded.UnmarshalJSON(data); err != nil {
		t.Fatalf("UnmarshalJSON 失败: %v", err)
	}

	if decoded.Role != "user" {
		t.Errorf("Role 不匹配: got %s", decoded.Role)
	}
	if decoded.MessageID != "msg-001" {
		t.Errorf("MessageID 不匹配: got %s", decoded.MessageID)
	}
	if len(decoded.Parts) != 2 {
		t.Fatalf("Parts 数量不匹配: got %d, want 2", len(decoded.Parts))
	}
	if tp, ok := decoded.Parts[0].(TextPart); ok && tp.Text != "你好" {
		t.Errorf("TextPart 内容不匹配: got %s", tp.Text)
	}
}

func TestA2AMessage_EmptyParts(t *testing.T) {
	msg := &A2AMessage{Role: "assistant", Parts: nil}
	data, err := msg.MarshalJSON()
	if err != nil {
		t.Fatalf("空 Parts 序列化失败: %v", err)
	}
	var decoded A2AMessage
	if err := decoded.UnmarshalJSON(data); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if len(decoded.Parts) != 0 {
		t.Errorf("空 Parts 反序列化后应有 0 个元素, got %d", len(decoded.Parts))
	}
}

func TestArtifact_Fields(t *testing.T) {
	a := Artifact{
		ArtifactID: "art-001",
		MimeType:   "application/json",
		Bytes:      []byte(`{"result":true}`),
		URI:        "https://example.com/output.json",
	}
	data, _ := json.Marshal(a)
	var decoded Artifact
	_ = json.Unmarshal(data, &decoded)
	if decoded.ArtifactID != "art-001" {
		t.Errorf("ArtifactID 不匹配: got %s", decoded.ArtifactID)
	}
	if string(decoded.Bytes) != `{"result":true}` {
		t.Errorf("Bytes 不匹配: got %s", string(decoded.Bytes))
	}
}

func TestExtractTextFromParts(t *testing.T) {
	parts := []Part{
		NewTextPart("Hello "),
		FilePart{},
		NewTextPart("World"),
		DataPart{},
	}
	text := ExtractTextFromParts(parts)
	if text != "Hello World" {
		t.Errorf("文本提取错误: got %q, want 'Hello World'", text)
	}
}

func TestExtractTextFromParts_Empty(t *testing.T) {
	text := ExtractTextFromParts(nil)
	if text != "" {
		t.Errorf("空 Parts 应返回空字符串, got %q", text)
	}
}

func TestSecurityScheme_Marshal(t *testing.T) {
	ss := SecurityScheme{Scheme: AuthBearer, In: "header", Name: "Authorization"}
	data, _ := json.Marshal(ss)
	var decoded SecurityScheme
	_ = json.Unmarshal(data, &decoded)
	if decoded.Scheme != AuthBearer {
		t.Errorf("AuthType 不匹配: got %s", decoded.Scheme)
	}
}

func TestAgentSkills_Marshal(t *testing.T) {
	skill := AgentSkill{
		ID:          "skill-csv-parse",
		Name:        "CSV解析",
		Description: "解析CSV文件并提取数据",
		InputModes:  []string{"application/csv"},
		OutputModes: []string{"application/json"},
	}
	data, _ := json.Marshal(skill)
	var decoded AgentSkill
	_ = json.Unmarshal(data, &decoded)
	if decoded.ID != "skill-csv-parse" {
		t.Errorf("Skill ID 不匹配: got %s", decoded.ID)
	}
	if len(decoded.InputModes) != 1 {
		t.Error("InputModes 数量不正确")
	}
}

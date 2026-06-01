package a2a

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestTaskEvent_StateChange_Marshal(t *testing.T) {
	state := TaskWorking
	event := &TaskEvent{
		Type:      EventStateChange,
		TaskID:    "task-001",
		Timestamp: time.Now().UTC().Truncate(time.Second),
		State:     &state,
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	var decoded TaskEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if decoded.Type != EventStateChange {
		t.Errorf("Type 不匹配: got %s", decoded.Type)
	}
	if decoded.TaskID != "task-001" {
		t.Errorf("TaskID 不匹配: got %s", decoded.TaskID)
	}
	if decoded.State == nil || *decoded.State != TaskWorking {
		t.Error("State 不匹配")
	}
}

func TestTaskEvent_MessageEvent(t *testing.T) {
	event := &TaskEvent{
		Type:      EventMessage,
		TaskID:    "task-002",
		Timestamp: time.Now().UTC(),
		Message: &A2AMessage{
			Role:  "agent",
			Parts: []Part{NewTextPart("处理中...")},
		},
	}

	data, _ := json.Marshal(event)
	var decoded TaskEvent
	json.Unmarshal(data, &decoded)

	if decoded.Message == nil || len(decoded.Message.Parts) != 1 {
		t.Fatal("Message Parts 解析失败")
	}
	tp, ok := decoded.Message.Parts[0].(TextPart)
	if !ok || tp.Text != "处理中..." {
		t.Errorf("TextPart 内容不匹配: %v", decoded.Message.Parts[0])
	}
}

func TestTaskEvent_ArtifactEvent(t *testing.T) {
	event := &TaskEvent{
		Type:   EventArtifact,
		TaskID: "task-003",
		Artifact: &Artifact{
			ArtifactID: "art-001",
			MimeType:   "image/png",
			URI:        "https://example.com/img.png",
		},
	}

	data, _ := json.Marshal(event)
	var decoded TaskEvent
	json.Unmarshal(data, &decoded)

	if decoded.Artifact == nil || decoded.Artifact.ArtifactID != "art-001" {
		t.Error("Artifact 解析失败")
	}
}

func TestTaskEvent_ErrorEvent(t *testing.T) {
	event := &TaskEvent{
		Type:   EventError,
		TaskID: "task-004",
		Error:  "连接超时",
	}

	data, _ := json.Marshal(event)
	var decoded TaskEvent
	json.Unmarshal(data, &decoded)

	if decoded.Type != EventError || decoded.Error != "连接超时" {
		t.Errorf("Error 事件解析错误: type=%s, error=%s", decoded.Type, decoded.Error)
	}
}

func TestSSEFormat(t *testing.T) {
	state := TaskWorking
	event := &TaskEvent{
		Type:      EventStateChange,
		TaskID:    "task-sse",
		Timestamp: time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC),
		State:     &state,
	}

	formatted := FormatSSEEvent(event)
	if !strings.HasPrefix(formatted, "data: ") {
		t.Errorf("SSE 格式应以 'data: ' 开头, got: %s", formatted[:min(len(formatted), 20)])
	}
	if !strings.HasSuffix(formatted, "\n\n") {
		t.Error("SSE 格式应以双换行结尾")
	}
	var parsed TaskEvent
	dataStr := strings.TrimPrefix(formatted, "data: ")
	dataStr = strings.TrimSuffix(dataStr, "\n")
	if err := json.Unmarshal([]byte(dataStr), &parsed); err != nil {
		t.Errorf("SSE data 部分应为有效 JSON: %v", err)
	}
	if parsed.TaskID != "task-sse" {
		t.Errorf("SSE data 中 TaskID 错误: got %s", parsed.TaskID)
	}
}

func TestSSEFormat_MarshalError(t *testing.T) {
	event := &TaskEvent{Type: EventError, Error: "test"}
	formatted := FormatSSEEvent(event)
	if !strings.HasPrefix(formatted, "data: ") {
		t.Error("SSE 格式应以 'data: ' 开头")
	}
	if !strings.HasSuffix(formatted, "\n\n") {
		t.Error("SSE 格式应以双换行结尾")
	}
}

func TestTaskEventType_Constants(t *testing.T) {
	tests := []struct {
		val      TaskEventType
		expected string
	}{
		{EventStateChange, "state_change"},
		{EventMessage, "message"},
		{EventArtifact, "artifact"},
		{EventError, "error"},
		{EventCanceled, "canceled"},
	}
	for _, tt := range tests {
		if string(tt.val) != tt.expected {
			t.Errorf("事件类型常量值错误: got %s, want %s", tt.val, tt.expected)
		}
	}
}

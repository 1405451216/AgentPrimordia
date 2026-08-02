package a2a

import (
	"encoding/json"
	"testing"
)

// v3.5 Golden Tests：开放规范标准请求/响应样例逐字比对
//
// 固定一份符合开放 A2A 协议形状的 JSON 样例，验证我们的 schema 类型
// 能正确反序列化（字段对齐）并 round-trip 保真。

const goldenAgentCard = `{
  "name": "golden-agent",
  "description": "A golden reference agent",
  "url": "https://example.com/a2a",
  "version": "2.0.0",
  "capabilities": {"streaming": true, "pushNotifications": false, "stateTransitionHistory": true},
  "skills": [{"id": "sk1", "name": "translate", "description": "Translate text", "tags": ["nlp"]}],
  "defaultInputModes": ["text"],
  "defaultOutputModes": ["text", "audio"],
  "authentication": {"schemes": ["bearer"]}
}`

const goldenMessage = `{
  "role": "user",
  "parts": [{"type": "text", "text": "hello world"}],
  "metadata": {"lang": "en"}
}`

const goldenTask = `{
  "id": "task-42",
  "contextId": "ctx-1",
  "status": {"state": "working", "timestamp": "2026-08-02T00:00:00Z"},
  "artifacts": [{"name": "out", "parts": [{"type": "text", "text": "done"}], "index": 0}]
}`

func TestGoldenAgentCard(t *testing.T) {
	var card OpenAgentCard
	if err := json.Unmarshal([]byte(goldenAgentCard), &card); err != nil {
		t.Fatalf("unmarshal golden card: %v", err)
	}
	if card.Name != "golden-agent" {
		t.Errorf("name = %q", card.Name)
	}
	if card.URL != "https://example.com/a2a" {
		t.Errorf("url = %q", card.URL)
	}
	if !card.Capabilities.Streaming {
		t.Error("streaming should be true")
	}
	if !card.Capabilities.StateTransitionHistory {
		t.Error("stateTransitionHistory should be true")
	}
	if len(card.Skills) != 1 || card.Skills[0].ID != "sk1" {
		t.Errorf("skills = %+v", card.Skills)
	}
	if len(card.DefaultOutputModes) != 2 {
		t.Errorf("output modes = %v", card.DefaultOutputModes)
	}
	if card.Authentication == nil || card.Authentication.Schemes[0] != "bearer" {
		t.Errorf("auth = %+v", card.Authentication)
	}

	// round-trip 保真
	assertRoundTrip(t, []byte(goldenAgentCard), &OpenAgentCard{})
}

func TestGoldenMessage(t *testing.T) {
	var msg OpenMessage
	if err := json.Unmarshal([]byte(goldenMessage), &msg); err != nil {
		t.Fatalf("unmarshal golden message: %v", err)
	}
	if msg.Role != "user" {
		t.Errorf("role = %q", msg.Role)
	}
	if msg.TextContent() != "hello world" {
		t.Errorf("text = %q", msg.TextContent())
	}
	if msg.Metadata["lang"] != "en" {
		t.Errorf("metadata = %v", msg.Metadata)
	}
	assertRoundTrip(t, []byte(goldenMessage), &OpenMessage{})
}

func TestGoldenTask(t *testing.T) {
	var task OpenTask
	if err := json.Unmarshal([]byte(goldenTask), &task); err != nil {
		t.Fatalf("unmarshal golden task: %v", err)
	}
	if task.ID != "task-42" {
		t.Errorf("id = %q", task.ID)
	}
	if task.Status.State != OpenTaskWorking {
		t.Errorf("state = %q", task.Status.State)
	}
	if len(task.Artifacts) != 1 || task.Artifacts[0].Name != "out" {
		t.Errorf("artifacts = %+v", task.Artifacts)
	}
	assertRoundTrip(t, []byte(goldenTask), &OpenTask{})
}

func TestGoldenErrorCodes(t *testing.T) {
	// 标准错误码值对齐开放规范
	cases := map[OpenErrorCode]int{
		OpenErrParseError:      -32700,
		OpenErrInvalidRequest:  -32600,
		OpenErrMethodNotFound:  -32601,
		OpenErrInvalidParams:   -32602,
		OpenErrInternal:        -32603,
		OpenErrTaskNotFound:    -32001,
	}
	for code, want := range cases {
		if int(code) != want {
			t.Errorf("code %v = %d, want %d", code, int(code), want)
		}
	}
}

// assertRoundTrip 验证 JSON 反序列化→序列化→反序列化字段一致
func assertRoundTrip(t *testing.T, raw []byte, target any) {
	t.Helper()
	if err := json.Unmarshal(raw, target); err != nil {
		t.Fatalf("round-trip unmarshal 1: %v", err)
	}
	out, err := json.Marshal(target)
	if err != nil {
		t.Fatalf("round-trip marshal: %v", err)
	}
	var asMap, origMap map[string]any
	_ = json.Unmarshal(raw, &origMap)
	_ = json.Unmarshal(out, &asMap)
	if !mapsEqual(origMap, asMap) {
		t.Errorf("round-trip mismatch:\n orig=%s\n  out=%s", string(raw), string(out))
	}
}

func mapsEqual(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, va := range a {
		vb, ok := b[k]
		if !ok {
			return false
		}
		if !jsonEqual(va, vb) {
			return false
		}
	}
	return true
}

func jsonEqual(a, b any) bool {
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return string(ja) == string(jb)
}

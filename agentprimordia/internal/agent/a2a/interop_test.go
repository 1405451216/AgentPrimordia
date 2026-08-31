package a2a

import (
	"encoding/json"
	"testing"
)

func TestOpenAgentCardJSON(t *testing.T) {
	card := OpenAgentCard{
		Name:        "test-agent",
		Description: "A test agent",
		URL:         "http://localhost:8080",
		Version:     "1.0.0",
		Capabilities: OpenCapabilities{
			Streaming:         true,
			PushNotifications: false,
		},
		Skills: []OpenSkillDecl{
			{ID: "s1", Name: "data-fix", Description: "Fix data"},
		},
		DefaultInputModes:  []string{"text"},
		DefaultOutputModes: []string{"text"},
	}

	data, err := json.Marshal(card)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded OpenAgentCard
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Name != "test-agent" {
		t.Errorf("name = %q", decoded.Name)
	}
	if !decoded.Capabilities.Streaming {
		t.Error("streaming should be true")
	}
	if len(decoded.Skills) != 1 {
		t.Errorf("skills = %d", len(decoded.Skills))
	}
}

func TestOpenMessageText(t *testing.T) {
	msg := NewTextMessage("user", "hello")
	if msg.Role != "user" {
		t.Errorf("role = %q", msg.Role)
	}
	if msg.TextContent() != "hello" {
		t.Errorf("text = %q", msg.TextContent())
	}
}

func TestOpenTaskStateTerminal(t *testing.T) {
	if !OpenTaskCompleted.IsTerminal() {
		t.Error("completed should be terminal")
	}
	if !OpenTaskFailed.IsTerminal() {
		t.Error("failed should be terminal")
	}
	if OpenTaskWorking.IsTerminal() {
		t.Error("working should not be terminal")
	}
}

func TestOpenErrorCodes(t *testing.T) {
	err := NewOpenError(OpenErrTaskNotFound, "task xyz not found")
	if err.Code != -32001 {
		t.Errorf("code = %d, want -32001", err.Code)
	}
	if err.Error() != "a2a interop error -32001: task xyz not found" {
		t.Errorf("error = %q", err.Error())
	}
}

func TestStandardErrorMessages(t *testing.T) {
	if msg := StandardErrorMessage(OpenErrParseError); msg != "Parse error" {
		t.Errorf("msg = %q", msg)
	}
	if msg := StandardErrorMessage(OpenErrMethodNotFound); msg != "Method not found" {
		t.Errorf("msg = %q", msg)
	}
}

func TestIOModeConfig(t *testing.T) {
	cfg := DefaultIOModeConfig()
	if !cfg.SupportsInput(IOModeText) {
		t.Error("should support text input")
	}
	if cfg.SupportsInput(IOModeAudio) {
		t.Error("should not support audio input by default")
	}
}

func TestInteropConfig(t *testing.T) {
	cfg := DefaultInteropConfig()
	if cfg.IsStrict() {
		t.Error("default should be compatible mode")
	}
	if cfg.AgentCardPath != "/.well-known/agent.json" {
		t.Errorf("path = %q", cfg.AgentCardPath)
	}
}

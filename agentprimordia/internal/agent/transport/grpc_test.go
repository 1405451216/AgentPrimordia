package transport

import (
	"agentprimordia/internal/agent/bus"
	"context"
	"testing"
	"time"
)

func TestGrpcTransport_SendAndReceive(t *testing.T) {
	tr := NewGrpcTransport()
	defer tr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	msg := &bus.BusMessage{
		ID:        "test-1",
		From:      "agent-a",
		To:        "agent-b",
		Type:      bus.BusMessageType("request"),
		Content:   "hello",
		Timestamp: time.Now(),
	}

	err := tr.Send(ctx, "nonexistent", msg)
	if err == nil {
		t.Error("expected error for nonexistent peer")
	}
}

func TestGrpcTransport_Receive(t *testing.T) {
	tr := NewGrpcTransport()
	defer tr.Close()

	ch := tr.Receive()
	if ch == nil {
		t.Error("expected non-nil inbox channel")
	}
}

func TestGrpcMsgSerialization(t *testing.T) {
	msg := &bus.BusMessage{
		ID:        "test-1",
		From:      "agent-a",
		To:        "agent-b",
		Type:      bus.BusMessageType("request"),
		Content:   "hello world",
		Metadata:  map[string]string{"key": "value"},
		Timestamp: time.Now(),
	}

	gm := toGrpcMsg(msg)
	restored := fromGrpcMsg(gm)

	if restored.ID != msg.ID {
		t.Errorf("ID mismatch: %s vs %s", restored.ID, msg.ID)
	}
	if restored.From != msg.From {
		t.Errorf("From mismatch")
	}
	if restored.Content != msg.Content {
		t.Errorf("Content mismatch")
	}
	if restored.Metadata["key"] != "value" {
		t.Errorf("Metadata mismatch")
	}
}

func TestGrpcTransport_StartAndStop(t *testing.T) {
	tr := NewGrpcTransport()

	err := tr.Start("127.0.0.1:0")
	if err != nil {
		t.Fatalf("Start error: %v", err)
	}

	err = tr.Close()
	if err != nil {
		t.Fatalf("Close error: %v", err)
	}
}


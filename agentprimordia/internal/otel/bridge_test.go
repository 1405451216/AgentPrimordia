package otel

import "testing"

func TestBridge_DefaultNotEnabled(t *testing.T) {
	if BridgeEnabled {
		t.Error("BridgeEnabled should be false without -tags otel")
	}
}

func TestBridge_NoopSpanBridge(t *testing.T) {
	b := NewOTelBridge()
	span := b.StartSpan("test")
	span.SetAttribute("key", "value")
	span.End()
	if err := b.Shutdown(); err != nil {
		t.Errorf("Shutdown error: %v", err)
	}
}

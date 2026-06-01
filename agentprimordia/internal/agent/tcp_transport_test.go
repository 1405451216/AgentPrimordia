package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTCPTransport_StartAndClose(t *testing.T) {
	tr := NewTCPTransport()

	if err := tr.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if tr.Addr() == "" {
		t.Error("Addr should not be empty after Start")
	}

	if err := tr.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestTCPTransport_SendAndReceive(t *testing.T) {
	sender := NewTCPTransport()
	receiver := NewTCPTransport()

	if err := sender.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("sender Start failed: %v", err)
	}
	defer sender.Close()

	if err := receiver.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("receiver Start failed: %v", err)
	}
	defer receiver.Close()

	msg := &BusMessage{
		ID:        "msg-001",
		From:      "agent-sender",
		To:        "agent-receiver",
		Type:      BusMsgTaskRequest,
		Content:   "hello from sender",
		Timestamp: time.Now(),
	}

	if err := sender.Send(context.Background(), receiver.Addr(), msg); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	select {
	case received := <-receiver.Receive():
		if received.ID != msg.ID {
			t.Errorf("ID = %q, want %q", received.ID, msg.ID)
		}
		if received.From != msg.From {
			t.Errorf("From = %q, want %q", received.From, msg.From)
		}
		if received.To != msg.To {
			t.Errorf("To = %q, want %q", received.To, msg.To)
		}
		if received.Type != msg.Type {
			t.Errorf("Type = %q, want %q", received.Type, msg.Type)
		}
		if received.Content != msg.Content {
			t.Errorf("Content = %q, want %q", received.Content, msg.Content)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestTCPTransport_ConcurrentMessages(t *testing.T) {
	sender := NewTCPTransport()
	receiver := NewTCPTransport()

	if err := sender.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("sender Start failed: %v", err)
	}
	defer sender.Close()

	if err := receiver.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("receiver Start failed: %v", err)
	}
	defer receiver.Close()

	const count = 20
	var sendErr atomic.Int32

	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			msg := &BusMessage{
				ID:        fmt.Sprintf("msg-%d", idx),
				From:      "sender",
				To:        "receiver",
				Type:      BusMsgTaskRequest,
				Content:   fmt.Sprintf("concurrent message %d", idx),
				Timestamp: time.Now(),
			}
			if err := sender.Send(context.Background(), receiver.Addr(), msg); err != nil {
				sendErr.Add(1)
			}
		}(i)
	}
	wg.Wait()

	if sendErr.Load() > 0 {
		t.Errorf("%d concurrent sends failed", sendErr.Load())
	}

	received := 0
	timeout := time.After(5 * time.Second)
	for received < count {
		select {
		case <-receiver.Receive():
			received++
		case <-timeout:
			t.Fatalf("timeout: received %d/%d messages", received, count)
		}
	}

	if received != count {
		t.Errorf("received %d messages, want %d", received, count)
	}
}

func TestTCPTransport_LargeMessage(t *testing.T) {
	sender := NewTCPTransport()
	receiver := NewTCPTransport()

	if err := sender.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("sender Start failed: %v", err)
	}
	defer sender.Close()

	if err := receiver.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("receiver Start failed: %v", err)
	}
	defer receiver.Close()

	largeContent := strings.Repeat("A", 1024*100)
	msg := &BusMessage{
		ID:        "msg-large",
		From:      "agent-sender",
		To:        "agent-receiver",
		Type:      BusMsgTaskRequest,
		Content:   largeContent,
		Timestamp: time.Now(),
	}

	if err := sender.Send(context.Background(), receiver.Addr(), msg); err != nil {
		t.Fatalf("Send large message failed: %v", err)
	}

	select {
	case received := <-receiver.Receive():
		if len(received.Content) != len(largeContent) {
			t.Errorf("Content length = %d, want %d", len(received.Content), len(largeContent))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for large message")
	}
}

func TestTCPTransport_SendBeforeStart(t *testing.T) {
	tr := NewTCPTransport()

	msg := &BusMessage{
		ID:      "msg-no-start",
		From:    "agent-1",
		To:      "agent-2",
		Type:    BusMsgTaskRequest,
		Content: "should fail",
	}

	err := tr.Send(context.Background(), "127.0.0.1:9999", msg)
	if err == nil {
		t.Error("expected error when sending before Start")
	}
}

func TestTCPTransport_DoubleStart(t *testing.T) {
	tr := NewTCPTransport()

	if err := tr.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("first Start failed: %v", err)
	}
	defer tr.Close()

	if err := tr.Start("127.0.0.1:0"); err == nil {
		t.Error("expected error on double Start")
	}
}

func TestTCPTransport_CloseBeforeStart(t *testing.T) {
	tr := NewTCPTransport()

	if err := tr.Close(); err != nil {
		t.Errorf("Close before Start should not error, got: %v", err)
	}
}

func TestTCPTransport_ConnectionPool(t *testing.T) {
	cfg := DefaultTCPTransportConfig()
	cfg.PoolSize = 4
	sender := NewTCPTransportWithConfig(cfg)
	receiver := NewTCPTransport()

	if err := sender.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("sender Start failed: %v", err)
	}
	defer sender.Close()

	if err := receiver.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("receiver Start failed: %v", err)
	}
	defer receiver.Close()

	msg := &BusMessage{
		ID:        "msg-pool-1",
		From:      "sender",
		To:        "receiver",
		Type:      BusMsgTaskRequest,
		Content:   "pool test",
		Timestamp: time.Now(),
	}

	if err := sender.Send(context.Background(), receiver.Addr(), msg); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	select {
	case <-receiver.Receive():
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestTCPTransport_MessageAck_Timeout(t *testing.T) {
	cfg := TCPTransportConfig{
		AckTimeout:    100 * time.Millisecond,
		MaxRetries:    1,
		RetryInterval: 50 * time.Millisecond,
		PoolSize:      4,
	}
	sender := NewTCPTransportWithConfig(cfg)

	if err := sender.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("sender Start failed: %v", err)
	}
	defer sender.Close()

	msg := &BusMessage{
		ID:        "msg-ack-timeout",
		From:      "sender",
		To:        "receiver",
		Type:      BusMsgTaskRequest,
		Content:   "ack timeout test",
		Timestamp: time.Now(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := sender.SendWithAck(ctx, "127.0.0.1:1", msg)
	if err == nil {
		t.Error("expected send failure error for unreachable target")
	}
}

func TestTCPTransport_RetryOnFailure(t *testing.T) {
	cfg := TCPTransportConfig{
		AckTimeout:    1 * time.Second,
		MaxRetries:    2,
		RetryInterval: 100 * time.Millisecond,
		PoolSize:      4,
	}
	sender := NewTCPTransportWithConfig(cfg)

	if err := sender.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("sender Start failed: %v", err)
	}
	defer sender.Close()

	msg := &BusMessage{
		ID:        "msg-retry",
		From:      "sender",
		To:        "receiver",
		Type:      BusMsgTaskRequest,
		Content:   "retry test",
		Timestamp: time.Now(),
	}

	err := sender.Send(context.Background(), "127.0.0.1:1", msg)
	if err == nil {
		t.Error("expected error when target is unreachable")
	}
}

func TestTCPTransport_PoolStats(t *testing.T) {
	tr := NewTCPTransport()

	active, idle := tr.PoolStats()
	if active != 0 || idle != 0 {
		t.Errorf("PoolStats before start = (%d, %d), want (0, 0)", active, idle)
	}
}

func TestTCPTransportWithConfig(t *testing.T) {
	cfg := TCPTransportConfig{
		AckTimeout:    5 * time.Second,
		MaxRetries:    5,
		RetryInterval: 200 * time.Millisecond,
		PoolSize:      16,
	}
	tr := NewTCPTransportWithConfig(cfg)

	if err := tr.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer tr.Close()

	if tr.Addr() == "" {
		t.Error("Addr should not be empty after Start")
	}
}

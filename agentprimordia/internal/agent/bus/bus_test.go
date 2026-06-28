package bus

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// echoHandler 返回收到的消息内容
func echoHandler(ctx context.Context, msg *BusMessage) (*BusMessage, error) {
	return &BusMessage{
		ID:        msg.ID + "_resp",
		From:      msg.To,
		To:        msg.From,
		Type:      BusMsgResponse,
		Content:   msg.Content,
		Timestamp: time.Now(),
	}, nil
}

// TestNewLocalMessageBus 测试创建本地消息总线
func TestNewLocalMessageBus(t *testing.T) {
	bus := NewLocalMessageBus()
	if bus == nil {
		t.Fatal("NewLocalMessageBus 返回 nil")
	}
	if len(bus.ListAgents()) != 0 {
		t.Fatalf("新创建的总线应该没有注册的 Agent，实际有 %d 个", len(bus.ListAgents()))
	}
}

// TestRegisterUnregister 测试注册和注销 Agent
func TestRegisterUnregister(t *testing.T) {
	bus := NewLocalMessageBus()

	// 注册
	bus.Register("agent-1", echoHandler)
	agents := bus.ListAgents()
	if len(agents) != 1 {
		t.Fatalf("注册 1 个 Agent 后应该有 1 个，实际有 %d 个", len(agents))
	}

	// 注册第二个
	bus.Register("agent-2", echoHandler)
	agents = bus.ListAgents()
	if len(agents) != 2 {
		t.Fatalf("注册 2 个 Agent 后应该有 2 个，实际有 %d 个", len(agents))
	}

	// 注销
	bus.Unregister("agent-1")
	agents = bus.ListAgents()
	if len(agents) != 1 {
		t.Fatalf("注销 1 个 Agent 后应该有 1 个，实际有 %d 个", len(agents))
	}
}

// TestSend 测试发送消息
func TestSend(t *testing.T) {
	bus := NewLocalMessageBus()
	bus.Register("agent-1", echoHandler)

	msg := &BusMessage{
		ID:      "msg-1",
		From:    "sender",
		To:      "agent-1",
		Type:    BusMsgTaskRequest,
		Content: "hello",
	}

	resp, err := bus.Send(context.Background(), msg)
	if err != nil {
		t.Fatalf("Send 返回错误: %v", err)
	}
	if resp.Content != "hello" {
		t.Fatalf("期望响应内容为 %q，实际为 %q", "hello", resp.Content)
	}
	if resp.From != "agent-1" {
		t.Fatalf("期望响应来源为 %q，实际为 %q", "agent-1", resp.From)
	}
}

// TestSendToUnknownAgent 测试向未注册的 Agent 发送消息
func TestSendToUnknownAgent(t *testing.T) {
	bus := NewLocalMessageBus()

	msg := &BusMessage{
		ID:   "msg-1",
		From: "sender",
		To:   "unknown",
		Type: BusMsgTaskRequest,
	}

	_, err := bus.Send(context.Background(), msg)
	if err == nil {
		t.Fatal("向未注册 Agent 发送消息应该返回错误")
	}
}

// TestSendAutoTimestamp 测试发送消息时自动填充时间戳
func TestSendAutoTimestamp(t *testing.T) {
	bus := NewLocalMessageBus()
	bus.Register("agent-1", echoHandler)

	before := time.Now()
	msg := &BusMessage{
		ID:        "msg-1",
		From:      "sender",
		To:        "agent-1",
		Type:      BusMsgTaskRequest,
		Content:   "hello",
		Timestamp: time.Time{}, // 零值
	}

	_, err := bus.Send(context.Background(), msg)
	if err != nil {
		t.Fatalf("Send 返回错误: %v", err)
	}

	// msg.Timestamp 应该已被自动填充
	if msg.Timestamp.Before(before) {
		t.Fatal("时间戳应该被自动填充为当前时间")
	}
}

// TestBroadcast 测试广播消息
func TestBroadcast(t *testing.T) {
	bus := NewLocalMessageBus()

	var mu sync.Mutex
	receivedBy := map[string]bool{}

	handler := func(agentID string) BusMessageHandler {
		return func(ctx context.Context, msg *BusMessage) (*BusMessage, error) {
			mu.Lock()
			receivedBy[agentID] = true
			mu.Unlock()
			return &BusMessage{
				ID:      msg.ID + "_resp",
				From:    agentID,
				To:      msg.From,
				Type:    BusMsgResponse,
				Content: "ack",
			}, nil
		}
	}

	bus.Register("agent-1", handler("agent-1"))
	bus.Register("agent-2", handler("agent-2"))
	bus.Register("agent-3", handler("agent-3"))

	msg := &BusMessage{
		ID:      "broadcast-1",
		From:    "agent-1",
		Type:    BusMsgBroadcast,
		Content: "announcement",
	}

	results := bus.Broadcast(context.Background(), msg)

	// agent-1 是发送方，不应收到广播
	if _, ok := results["agent-1"]; ok {
		t.Fatal("发送方不应该收到自己的广播")
	}

	// agent-2 和 agent-3 应该收到
	if len(results) != 2 {
		t.Fatalf("期望 2 个响应，实际有 %d 个", len(results))
	}

	mu.Lock()
	if !receivedBy["agent-2"] || !receivedBy["agent-3"] {
		t.Fatal("agent-2 和 agent-3 应该都收到广播")
	}
	mu.Unlock()
}

// TestBroadcastAutoTimestamp 测试广播时自动填充时间戳
func TestBroadcastAutoTimestamp(t *testing.T) {
	bus := NewLocalMessageBus()
	bus.Register("agent-2", echoHandler)

	before := time.Now()
	msg := &BusMessage{
		ID:        "broadcast-1",
		From:      "agent-1",
		Type:      BusMsgBroadcast,
		Content:   "announcement",
		Timestamp: time.Time{},
	}

	bus.Broadcast(context.Background(), msg)

	if msg.Timestamp.Before(before) {
		t.Fatal("广播消息时间戳应该被自动填充")
	}
}

// TestSubscribe 测试订阅消息通道
func TestSubscribe(t *testing.T) {
	bus := NewLocalMessageBus()
	bus.Register("agent-1", echoHandler)

	ch := bus.Subscribe("agent-1")

	msg := &BusMessage{
		ID:        "msg-1",
		From:      "sender",
		To:        "agent-1",
		Type:      BusMsgTaskRequest,
		Content:   "hello",
		Timestamp: time.Now(),
	}

	_, err := bus.Send(context.Background(), msg)
	if err != nil {
		t.Fatalf("Send 返回错误: %v", err)
	}

	// 从订阅通道读取消息
	select {
	case received := <-ch:
		if received.ID != "msg-1" {
			t.Fatalf("期望收到消息 ID %q，实际为 %q", "msg-1", received.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("订阅通道应该收到消息")
	}
}

// TestUnregisterClosesChannels 测试注销时关闭订阅通道
func TestUnregisterClosesChannels(t *testing.T) {
	bus := NewLocalMessageBus()
	bus.Register("agent-1", echoHandler)

	ch := bus.Subscribe("agent-1")
	bus.Unregister("agent-1")

	// 通道应该被关闭
	_, ok := <-ch
	if ok {
		t.Fatal("注销后订阅通道应该被关闭")
	}
}

// TestListAgents 测试列出已注册 Agent
func TestListAgents(t *testing.T) {
	bus := NewLocalMessageBus()

	expected := map[string]bool{"a": true, "b": true, "c": true}
	for id := range expected {
		bus.Register(id, echoHandler)
	}

	agents := bus.ListAgents()
	if len(agents) != len(expected) {
		t.Fatalf("期望 %d 个 Agent，实际有 %d 个", len(expected), len(agents))
	}

	for _, id := range agents {
		if !expected[id] {
			t.Fatalf("未预期的 Agent: %q", id)
		}
	}
}

// TestConcurrentRegister 测试并发注册
func TestConcurrentRegister(t *testing.T) {
	bus := NewLocalMessageBus()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			bus.Register(fmt.Sprintf("agent_%d", i), echoHandler)
		}(i)
	}

	wg.Wait()
	agents := bus.ListAgents()
	if len(agents) != 100 {
		t.Fatalf("并发注册后应该有 100 个 Agent，实际有 %d 个", len(agents))
	}
}

// TestMessageBusInterface 测试 MessageBus 接口可被 LocalMessageBus 满足
func TestMessageBusInterface(t *testing.T) {
	// 编译期验证 LocalMessageBus 实现了 MessageBus 接口
	var _ MessageBus = (*LocalMessageBus)(nil)
}

// TestBusMessageTypeConstants 测试消息类型常量
func TestBusMessageTypeConstants(t *testing.T) {
	types := map[BusMessageType]string{
		BusMsgTaskRequest:  "task_request",
		BusMsgTaskResult:   "task_result",
		BusMsgQuery:        "query",
		BusMsgResponse:     "response",
		BusMsgHandoff:      "handoff",
		BusMsgBroadcast:    "broadcast",
		BusMsgStatusUpdate: "status_update",
		BusMsgNotify:       "notify",
	}

	for typ, expected := range types {
		if string(typ) != expected {
			t.Errorf("消息类型常量 %q 期望值为 %q，实际为 %q", typ, expected, string(typ))
		}
	}
}

// TestBusMessageFields 测试 BusMessage 字段
func TestBusMessageFields(t *testing.T) {
	now := time.Now()
	msg := &BusMessage{
		ID:        "test-id",
		From:      "sender",
		To:        "receiver",
		Type:      BusMsgQuery,
		Content:   "content",
		Metadata:  map[string]string{"key": "value"},
		Timestamp: now,
	}

	if msg.ID != "test-id" {
		t.Errorf("ID 字段不匹配")
	}
	if msg.From != "sender" {
		t.Errorf("From 字段不匹配")
	}
	if msg.To != "receiver" {
		t.Errorf("To 字段不匹配")
	}
	if msg.Type != BusMsgQuery {
		t.Errorf("Type 字段不匹配")
	}
	if msg.Content != "content" {
		t.Errorf("Content 字段不匹配")
	}
	if msg.Metadata["key"] != "value" {
		t.Errorf("Metadata 字段不匹配")
	}
	if !msg.Timestamp.Equal(now) {
		t.Errorf("Timestamp 字段不匹配")
	}
}

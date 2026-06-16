package agent

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestTCPTransport_StartClose(t *testing.T) {
	transport := NewTCPTransport()

	// 启动传输层
	if err := transport.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("启动失败: %v", err)
	}

	addr := transport.Addr()
	if addr == "" {
		t.Fatal("地址为空")
	}

	// 重复启动应该失败
	if err := transport.Start("127.0.0.1:0"); err == nil {
		t.Fatal("重复启动应该失败")
	}

	// 关闭传输层
	if err := transport.Close(); err != nil {
		t.Fatalf("关闭失败: %v", err)
	}

	// 重复关闭应该成功
	if err := transport.Close(); err != nil {
		t.Fatalf("重复关闭失败: %v", err)
	}
}

func TestTCPTransport_SendReceive(t *testing.T) {
	// 启动两个传输层
	transport1 := NewTCPTransport()
	transport2 := NewTCPTransport()

	if err := transport1.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("transport1 启动失败: %v", err)
	}
	defer transport1.Close()

	if err := transport2.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("transport2 启动失败: %v", err)
	}
	defer transport2.Close()

	// 发送消息
	ctx := context.Background()
	msg := &BusMessage{
		ID:      "test-msg-1",
		From:    "agent1",
		To:      "agent2",
		Content: `{"test": "data"}`,
	}

	if err := transport1.Send(ctx, transport2.Addr(), msg); err != nil {
		t.Fatalf("发送失败: %v", err)
	}

	// 接收消息
	select {
	case received := <-transport2.Receive():
		if received.ID != msg.ID {
			t.Errorf("消息 ID 不匹配: 期望 %s, 得到 %s", msg.ID, received.ID)
		}
		if received.From != msg.From {
			t.Errorf("消息 From 不匹配: 期望 %s, 得到 %s", msg.From, received.From)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("接收消息超时")
	}
}

func TestTCPTransport_SendWithAck(t *testing.T) {
	transport1 := NewTCPTransport()
	transport2 := NewTCPTransport()

	if err := transport1.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("transport1 启动失败: %v", err)
	}
	defer transport1.Close()

	if err := transport2.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("transport2 启动失败: %v", err)
	}
	defer transport2.Close()

	ctx := context.Background()
	msg := &BusMessage{
		ID:      "test-msg-ack",
		From:    "agent1",
		To:      "agent2",
		Content: `{"test": "ack"}`,
	}

	// 启动一个 goroutine 来接收消息并发送 ACK
	go func() {
		select {
		case received := <-transport2.Receive():
			// 收到消息后，handleConn 会自动发送 ACK
			t.Logf("收到消息: %s", received.ID)
		case <-time.After(2 * time.Second):
			t.Error("接收消息超时")
		}
	}()

	// 发送需要 ACK 的消息
	if err := transport1.SendWithAck(ctx, transport2.Addr(), msg); err != nil {
		t.Fatalf("发送带 ACK 的消息失败: %v", err)
	}
}

func TestTCPTransport_ConnectionPool(t *testing.T) {
	cfg := DefaultTCPTransportConfig()
	cfg.PoolSize = 4

	transport1 := NewTCPTransportWithConfig(cfg)
	transport2 := NewTCPTransportWithConfig(cfg)

	if err := transport1.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("transport1 启动失败: %v", err)
	}
	defer transport1.Close()

	if err := transport2.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("transport2 启动失败: %v", err)
	}
	defer transport2.Close()

	ctx := context.Background()

	// 发送多条消息以测试连接池
	for i := 0; i < 5; i++ {
		msg := &BusMessage{
			ID:      "test-msg-pool",
			From:    "agent1",
			To:      "agent2",
			Content: `{"test": "pool"}`,
		}

		if err := transport1.Send(ctx, transport2.Addr(), msg); err != nil {
			t.Fatalf("发送失败: %v", err)
		}

		// 接收消息
		select {
		case <-transport2.Receive():
		case <-time.After(2 * time.Second):
			t.Fatal("接收消息超时")
		}
	}

	// 检查连接池状态
	active, idle := transport1.PoolStats()
	t.Logf("连接池状态: active=%d, idle=%d", active, idle)

	// 应该有空闲连接
	if idle == 0 {
		t.Log("警告: 没有空闲连接")
	}
}

func TestTCPTransport_Retry(t *testing.T) {
	transport := NewTCPTransport()

	if err := transport.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	defer transport.Close()

	ctx := context.Background()
	msg := &BusMessage{
		ID:      "test-msg-retry",
		From:    "agent1",
		To:      "agent2",
		Content: `{"test": "retry"}`,
	}

	// 尝试发送到一个不存在的地址，应该重试后失败
	err := transport.Send(ctx, "127.0.0.1:59999", msg)
	if err == nil {
		t.Fatal("发送到不存在的地址应该失败")
	}

	t.Logf("重试后失败（符合预期）: %v", err)
}

func TestTCPTransport_SendBeforeStart(t *testing.T) {
	transport := NewTCPTransport()

	ctx := context.Background()
	msg := &BusMessage{
		ID:      "test-msg",
		From:    "agent1",
		To:      "agent2",
		Content: `{"test": "data"}`,
	}

	// 在启动前发送应该失败
	err := transport.Send(ctx, "127.0.0.1:8080", msg)
	if err == nil {
		t.Fatal("在启动前发送应该失败")
	}

	if err.Error() != "transport not started" {
		t.Errorf("错误消息不匹配: %v", err)
	}
}

func TestTCPTransport_AckTimeout(t *testing.T) {
	cfg := DefaultTCPTransportConfig()
	cfg.AckTimeout = 500 * time.Millisecond
	cfg.MaxRetries = 1                   // 最少重试次数
	cfg.RetryInterval = time.Millisecond // 最小间隔

	transport1 := NewTCPTransportWithConfig(cfg)

	if err := transport1.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("transport1 启动失败: %v", err)
	}
	defer transport1.Close()

	// 启动一个不响应 ACK 的服务器
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("监听失败: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		// 接收消息但不发送 ACK
		buf := make([]byte, 1024)
		conn.Read(buf)
		// 保持连接打开但不响应
		time.Sleep(2 * time.Second)
		conn.Close()
	}()

	ctx := context.Background()
	msg := &BusMessage{
		ID:      "test-msg-timeout",
		From:    "agent1",
		To:      "agent2",
		Content: `{"test": "timeout"}`,
	}

	// 发送需要 ACK 的消息，应该超时
	start := time.Now()
	err = transport1.SendWithAck(ctx, ln.Addr().String(), msg)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("应该超时失败")
	}

	// 检查超时时间是否合理（允许一定误差，考虑重试）
	if elapsed < 400*time.Millisecond || elapsed > 5*time.Second {
		t.Errorf("超时时间不合理: %v", elapsed)
	}

	t.Logf("超时失败（符合预期）: %v, 耗时: %v", err, elapsed)
}

func TestTCPTransport_MultipleMessages(t *testing.T) {
	transport1 := NewTCPTransport()
	transport2 := NewTCPTransport()

	if err := transport1.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("transport1 启动失败: %v", err)
	}
	defer transport1.Close()

	if err := transport2.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("transport2 启动失败: %v", err)
	}
	defer transport2.Close()

	ctx := context.Background()

	// 发送多条消息
	numMessages := 10
	for i := 0; i < numMessages; i++ {
		msg := &BusMessage{
			ID:      "test-msg",
			From:    "agent1",
			To:      "agent2",
			Content: `{"test": "data"}`,
		}

		if err := transport1.Send(ctx, transport2.Addr(), msg); err != nil {
			t.Fatalf("发送消息 %d 失败: %v", i, err)
		}
	}

	// 接收所有消息
	received := 0
	timeout := time.After(5 * time.Second)
	for received < numMessages {
		select {
		case <-transport2.Receive():
			received++
		case <-timeout:
			t.Fatalf("接收超时，只收到 %d/%d 条消息", received, numMessages)
		}
	}

	if received != numMessages {
		t.Errorf("消息数量不匹配: 期望 %d, 得到 %d", numMessages, received)
	}
}

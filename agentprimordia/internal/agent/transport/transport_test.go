package transport

import (
	"agentprimordia/internal/agent/bus"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// ===== Transport 接口满足性测试 =====

// TestHTTPTransportImplementsInterface 编译期验证 HTTPTransport 实现了 Transport 接口
func TestHTTPTransportImplementsInterface(t *testing.T) {
	var _ Transport = (*HTTPTransport)(nil)
}

// TestTCPTransportImplementsInterface 编译期验证 TCPTransport 实现了 Transport 接口
func TestTCPTransportImplementsInterface(t *testing.T) {
	var _ Transport = (*TCPTransport)(nil)
}

// ===== HTTPTransport 测试 =====

// TestHTTPTransportStartClose 测试 HTTP 传输层启动和关闭
func TestHTTPTransportStartClose(t *testing.T) {
	tr := NewHTTPTransport()

	// 使用端口 0 自动分配
	if err := tr.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("启动 HTTP 传输层失败: %v", err)
	}

	addr := tr.Addr()
	if addr == "" {
		t.Fatal("启动后地址不应该为空")
	}

	if err := tr.Close(); err != nil {
		t.Fatalf("关闭 HTTP 传输层失败: %v", err)
	}
}

// TestHTTPTransportDoubleStart 测试重复启动
func TestHTTPTransportDoubleStart(t *testing.T) {
	tr := NewHTTPTransport()

	if err := tr.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("首次启动失败: %v", err)
	}
	defer tr.Close()

	if err := tr.Start("127.0.0.1:0"); err == nil {
		t.Fatal("重复启动应该返回错误")
	}
}

// TestHTTPTransportSendNotStarted 测试未启动时发送
func TestHTTPTransportSendNotStarted(t *testing.T) {
	tr := NewHTTPTransport()

	msg := &bus.BusMessage{ID: "1", From: "a", To: "b", Type: bus.BusMsgTaskRequest}
	err := tr.Send(context.Background(), "127.0.0.1:12345", msg)
	if err == nil {
		t.Fatal("未启动时发送应该返回错误")
	}
}

// TestHTTPTransportCloseNotStarted 测试未启动时关闭
func TestHTTPTransportCloseNotStarted(t *testing.T) {
	tr := NewHTTPTransport()
	// 未启动时关闭不应该报错
	if err := tr.Close(); err != nil {
		t.Fatalf("未启动时关闭不应该报错: %v", err)
	}
}

// TestHTTPTransportSendReceive 测试完整的发送-接收流程
func TestHTTPTransportSendReceive(t *testing.T) {
	// 启动服务端
	server := NewHTTPTransport()
	if err := server.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("启动服务端失败: %v", err)
	}
	defer server.Close()

	// 启动客户端
	client := NewHTTPTransport()
	if err := client.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("启动客户端失败: %v", err)
	}
	defer client.Close()

	// 发送消息
	msg := &bus.BusMessage{
		ID:        "msg-1",
		From:      "client",
		To:        "server",
		Type:      bus.BusMsgTaskRequest,
		Content:   "hello",
		Timestamp: time.Now(),
	}

	if err := client.Send(context.Background(), server.Addr(), msg); err != nil {
		t.Fatalf("发送消息失败: %v", err)
	}

	// 接收消息
	select {
	case received := <-server.Receive():
		if received.ID != "msg-1" {
			t.Fatalf("期望消息 ID %q，实际为 %q", "msg-1", received.ID)
		}
		if received.Content != "hello" {
			t.Fatalf("期望内容 %q，实际为 %q", "hello", received.Content)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("接收消息超时")
	}
}

// TestHTTPTransportHandleInvalidMethod 测试 HTTP 非 POST 请求
func TestHTTPTransportHandleInvalidMethod(t *testing.T) {
	tr := NewHTTPTransport()
	if err := tr.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	defer tr.Close()

	url := fmt.Sprintf("http://%s%s", tr.Addr(), messageEndpoint)
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET 请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("期望状态码 %d，实际为 %d", http.StatusMethodNotAllowed, resp.StatusCode)
	}
}

// TestHTTPTransportHandleInvalidBody 测试 HTTP 无效请求体
func TestHTTPTransportHandleInvalidBody(t *testing.T) {
	tr := NewHTTPTransport()
	if err := tr.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	defer tr.Close()

	url := fmt.Sprintf("http://%s%s", tr.Addr(), messageEndpoint)
	resp, err := http.Post(url, "application/json", strings.NewReader("invalid json"))
	if err != nil {
		t.Fatalf("POST 请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("期望状态码 %d，实际为 %d", http.StatusBadRequest, resp.StatusCode)
	}
}

// ===== TCPTransport 测试 =====

// TestTCPTransportStartClose 测试 TCP 传输层启动和关闭
func TestTCPTransportStartClose(t *testing.T) {
	tr := NewTCPTransport()

	if err := tr.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("启动 TCP 传输层失败: %v", err)
	}

	addr := tr.Addr()
	if addr == "" {
		t.Fatal("启动后地址不应该为空")
	}

	if err := tr.Close(); err != nil {
		t.Fatalf("关闭 TCP 传输层失败: %v", err)
	}
}

// TestTCPTransportDoubleStart 测试重复启动
func TestTCPTransportDoubleStart(t *testing.T) {
	tr := NewTCPTransport()

	if err := tr.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("首次启动失败: %v", err)
	}
	defer tr.Close()

	if err := tr.Start("127.0.0.1:0"); err == nil {
		t.Fatal("重复启动应该返回错误")
	}
}

// TestTCPTransportSendNotStarted 测试未启动时发送
func TestTCPTransportSendNotStarted(t *testing.T) {
	tr := NewTCPTransport()

	msg := &bus.BusMessage{ID: "1", From: "a", To: "b", Type: bus.BusMsgTaskRequest}
	err := tr.Send(context.Background(), "127.0.0.1:12345", msg)
	if err == nil {
		t.Fatal("未启动时发送应该返回错误")
	}
}

// TestTCPTransportCloseNotStarted 测试未启动时关闭
func TestTCPTransportCloseNotStarted(t *testing.T) {
	tr := NewTCPTransport()
	if err := tr.Close(); err != nil {
		t.Fatalf("未启动时关闭不应该报错: %v", err)
	}
}

// TestTCPTransportSendReceive 测试 TCP 发送和接收
func TestTCPTransportSendReceive(t *testing.T) {
	// 启动服务端
	server := NewTCPTransport()
	if err := server.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("启动服务端失败: %v", err)
	}
	defer server.Close()

	// 启动客户端
	client := NewTCPTransport()
	if err := client.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("启动客户端失败: %v", err)
	}
	defer client.Close()

	msg := &bus.BusMessage{
		ID:        "msg-1",
		From:      "client",
		To:        "server",
		Type:      bus.BusMsgTaskRequest,
		Content:   "hello tcp",
		Timestamp: time.Now(),
	}

	if err := client.Send(context.Background(), server.Addr(), msg); err != nil {
		t.Fatalf("发送消息失败: %v", err)
	}

	select {
	case received := <-server.Receive():
		if received.ID != "msg-1" {
			t.Fatalf("期望消息 ID %q，实际为 %q", "msg-1", received.ID)
		}
		if received.Content != "hello tcp" {
			t.Fatalf("期望内容 %q，实际为 %q", "hello tcp", received.Content)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("接收消息超时")
	}
}

// TestTCPTransportSendWithAck 测试带 ACK 的发送
func TestTCPTransportSendWithAck(t *testing.T) {
	server := NewTCPTransport()
	if err := server.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("启动服务端失败: %v", err)
	}
	defer server.Close()

	client := NewTCPTransport()
	if err := client.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("启动客户端失败: %v", err)
	}
	defer client.Close()

	msg := &bus.BusMessage{
		ID:        "msg-ack-1",
		From:      "client",
		To:        "server",
		Type:      bus.BusMsgTaskRequest,
		Content:   "ack test",
		Timestamp: time.Now(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := client.SendWithAck(ctx, server.Addr(), msg); err != nil {
		t.Fatalf("SendWithAck 失败: %v", err)
	}
}

// ===== TCPTransportConfig 测试 =====

// TestDefaultTCPTransportConfig 测试默认配置
func TestDefaultTCPTransportConfig(t *testing.T) {
	cfg := DefaultTCPTransportConfig()

	if cfg.AckTimeout != defaultAckTimeout {
		t.Fatalf("AckTimeout 期望 %v，实际 %v", defaultAckTimeout, cfg.AckTimeout)
	}
	if cfg.MaxRetries != defaultMaxRetries {
		t.Fatalf("MaxRetries 期望 %d，实际 %d", defaultMaxRetries, cfg.MaxRetries)
	}
	if cfg.PoolSize != defaultPoolSize {
		t.Fatalf("PoolSize 期望 %d，实际 %d", defaultPoolSize, cfg.PoolSize)
	}
}

// TestNewTCPTransportWithConfig 测试自定义配置
func TestNewTCPTransportWithConfig(t *testing.T) {
	cfg := TCPTransportConfig{
		AckTimeout:    5 * time.Second,
		MaxRetries:    5,
		RetryInterval: 1 * time.Second,
		PoolSize:      16,
	}

	tr := NewTCPTransportWithConfig(cfg)
	if tr == nil {
		t.Fatal("NewTCPTransportWithConfig 返回 nil")
	}
}

// TestNewTCPTransportWithConfigDefaults 测试零值配置使用默认值
func TestNewTCPTransportWithConfigDefaults(t *testing.T) {
	cfg := TCPTransportConfig{} // 全部零值
	tr := NewTCPTransportWithConfig(cfg)

	if tr.config.AckTimeout != defaultAckTimeout {
		t.Fatalf("零值 AckTimeout 应该使用默认值 %v，实际 %v", defaultAckTimeout, tr.config.AckTimeout)
	}
	if tr.config.MaxRetries != defaultMaxRetries {
		t.Fatalf("零值 MaxRetries 应该使用默认值 %d，实际 %d", defaultMaxRetries, tr.config.MaxRetries)
	}
	if tr.config.PoolSize != defaultPoolSize {
		t.Fatalf("零值 PoolSize 应该使用默认值 %d，实际 %d", defaultPoolSize, tr.config.PoolSize)
	}
}

// TestTCPTransportPoolStats 测试连接池统计
func TestTCPTransportPoolStats(t *testing.T) {
	tr := NewTCPTransport()
	active, idle := tr.PoolStats()
	if active != 0 || idle != 0 {
		t.Fatalf("未启动时连接池统计应该为 0,0，实际为 %d,%d", active, idle)
	}
}

// ===== connPool 测试 =====

// TestConnPoolBasic 测试连接池基本操作
func TestConnPoolBasic(t *testing.T) {
	// 先启动一个 TCP 服务
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("监听失败: %v", err)
	}
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			// 简单回显
			go func(c net.Conn) {
				defer c.Close()
				io.Copy(c, c)
			}(conn)
		}
	}()

	dialer := &net.Dialer{Timeout: 5 * time.Second}
	pool := newConnPool(4, dialer)
	defer pool.CloseAll()

	ctx := context.Background()
	target := ln.Addr().String()

	// 获取连接
	conn, err := pool.Get(ctx, target)
	if err != nil {
		t.Fatalf("获取连接失败: %v", err)
	}

	// 归还连接
	pool.Put(target, conn)

	// 检查统计
	_, idle := pool.Stats()
	if idle != 1 {
		t.Fatalf("归还后空闲连接应该为 1，实际为 %d", idle)
	}
}

// TestConnPoolCloseAll 测试关闭连接池
func TestConnPoolCloseAll(t *testing.T) {
	pool := newConnPool(4, &net.Dialer{Timeout: 5 * time.Second})
	pool.CloseAll()

	ctx := context.Background()
	_, err := pool.Get(ctx, "127.0.0.1:1")
	if err == nil {
		t.Fatal("连接池关闭后获取连接应该返回错误")
	}
}

// ===== 辅助测试 =====

// TestHTTPTransportAddrNotStarted 测试未启动时获取地址
func TestHTTPTransportAddrNotStarted(t *testing.T) {
	tr := NewHTTPTransport()
	addr := tr.Addr()
	if addr != "" {
		t.Fatalf("未启动时地址应该为空，实际为 %q", addr)
	}
}

// TestTCPTransportAddrNotStarted 测试未启动时获取地址
func TestTCPTransportAddrNotStarted(t *testing.T) {
	tr := NewTCPTransport()
	addr := tr.Addr()
	if addr != "" {
		t.Fatalf("未启动时地址应该为空，实际为 %q", addr)
	}
}

// TestHTTPTransportWithTLS 测试 TLS 配置方法
func TestHTTPTransportWithTLS(t *testing.T) {
	tr := NewHTTPTransport()
	// 配置 TLS 但不实际使用（因为需要证书）
	result := tr.WithTLS(nil)
	if result != tr {
		t.Fatal("WithTLS 应该返回自身以支持链式调用")
	}
}

// TestHTTPTransportSendToInvalidTarget 测试向不可达目标发送
func TestHTTPTransportSendToInvalidTarget(t *testing.T) {
	tr := NewHTTPTransport()
	if err := tr.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	defer tr.Close()

	msg := &bus.BusMessage{ID: "1", From: "a", To: "b", Type: bus.BusMsgTaskRequest}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := tr.Send(ctx, "127.0.0.1:1", msg)
	if err == nil {
		t.Fatal("向不可达目标发送应该返回错误")
	}
}

// TestTCPTransportHandleConnInvalidData 测试 TCP 处理无效数据
func TestTCPTransportHandleConnInvalidData(t *testing.T) {
	server := NewTCPTransport()
	if err := server.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("启动服务端失败: %v", err)
	}
	defer server.Close()

	// 手动连接并发送无效 JSON
	conn, err := net.DialTimeout("tcp", server.Addr(), 2*time.Second)
	if err != nil {
		t.Fatalf("连接服务端失败: %v", err)
	}
	defer conn.Close()

	// 发送无效 JSON（应该被忽略，不会崩溃）
	_, _ = conn.Write([]byte("invalid json\n"))

	// 发送有效 JSON
	validMsg := bus.BusMessage{
		ID:        "valid-1",
		From:      "raw-client",
		To:        "server",
		Type:      bus.BusMsgQuery,
		Content:   "valid content",
		Timestamp: time.Now(),
	}
	data, _ := json.Marshal(validMsg)
	_, _ = conn.Write(append(data, '\n'))

	// 应该只收到有效消息
	select {
	case received := <-server.Receive():
		if received.ID != "valid-1" {
			t.Fatalf("期望收到有效消息 ID %q，实际为 %q", "valid-1", received.ID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("接收有效消息超时")
	}
}

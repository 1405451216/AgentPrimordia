package transport

import (
	"agentprimordia/internal/agent/bus"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"
)

// ===== #9 gRPC 双向流式通信 =====
//
// 状态：实验性占位实现（EXPERIMENTAL PLACEHOLDER）
// ---------------------------------------------------------------
// 重要：GrpcTransport.Send 当前不会通过网络发送消息。Send 序列化
// payload 后丢弃，调用方收到 nil 但对端永远收不到。
//
// 历史：本文件早期作为"未来 gRPC 传输层"的脚手架存在（代码
// 顶部注释 "Unary call placeholder — in production, use generated
// gRPC client"），但未跟进真实实现。本 PR（v6.x）将其标记为
// EXPERIMENTAL，使错误可在运行时被检测出来，避免线上"假绿"。
//
// 迁移路径：
//   - 立即：调用方需在 Send 返回 nil 时自行验证对端接收。
//   - v6.x：补全真实双向流实现（基于内部已生成的 a2a proto 桩）。
//
// 替代实现：internal/agent/cluster/grpc_bus.go 已提供基于共享
// codec 的远程消息总线（RemoteMessageBus.Send），用于跨节点通信。
// ---------------------------------------------------------------

// ErrGrpcTransportNotImplemented 标记 Send/Receive 为未实现。
// 调用方应通过 errors.Is(err, transport.ErrGrpcTransportNotImplemented)
// 判断是否需要回退到其他 Transport（如 cluster.RemoteMessageBus）。
var ErrGrpcTransportNotImplemented = errors.New("transport: GrpcTransport is an experimental placeholder; Send does not deliver messages")

// GrpcTransport 基于 gRPC 的传输层占位实现。
//
// EXPERIMENTAL: Send 方法不会真实投递消息，仅返回错误让调用方
// 显式感知。请使用 internal/agent/cluster.RemoteMessageBus 替代。
type GrpcTransport struct {
	mu        sync.RWMutex
	listener  net.Listener
	server    *grpc.Server
	inbox     chan *bus.BusMessage
	peers     map[string]*grpcPeerConn
	timeout   time.Duration
	localAddr string
}

type grpcPeerConn struct {
	conn *grpc.ClientConn
}

// grpcMessage JSON 编码的消息封装。
type grpcMessage struct {
	ID        string            `json:"id"`
	From      string            `json:"from"`
	To        string            `json:"to"`
	Type      string            `json:"type"`
	Content   string            `json:"content"`
	Timestamp int64             `json:"timestamp"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

func toGrpcMsg(msg *bus.BusMessage) *grpcMessage {
	return &grpcMessage{
		ID:        msg.ID,
		From:      msg.From,
		To:        msg.To,
		Type:      string(msg.Type),
		Content:   msg.Content,
		Timestamp: msg.Timestamp.UnixNano(),
		Metadata:  msg.Metadata,
	}
}

func fromGrpcMsg(gm *grpcMessage) *bus.BusMessage {
	return &bus.BusMessage{
		ID:        gm.ID,
		From:      gm.From,
		To:        gm.To,
		Type:      bus.BusMessageType(gm.Type),
		Content:   gm.Content,
		Metadata:  gm.Metadata,
		Timestamp: time.Unix(0, gm.Timestamp),
	}
}

// NewGrpcTransport 创建。
func NewGrpcTransport() *GrpcTransport {
	return &GrpcTransport{
		inbox:   make(chan *bus.BusMessage, 256),
		peers:   make(map[string]*grpcPeerConn),
		timeout: 30 * time.Second,
	}
}

// Send 向目标发送消息。
//
// EXPERIMENTAL: 此实现为占位符，不会真实投递消息。返回
// ErrGrpcTransportNotImplemented 让调用方在分支条件上明确感知，
// 避免"返回 nil 但对端收不到"的隐式失败。请改用
// internal/agent/cluster.RemoteMessageBus。
func (t *GrpcTransport) Send(ctx context.Context, target string, msg *bus.BusMessage) error {
	t.mu.RLock()
	peer, ok := t.peers[target]
	t.mu.RUnlock()
	if !ok {
		return fmt.Errorf("no connection to %s: %w", target, ErrGrpcTransportNotImplemented)
	}
	// 序列化校验仍然执行，以便早期捕获不兼容 payload。
	gm := toGrpcMsg(msg)
	if _, err := json.Marshal(gm); err != nil {
		return fmt.Errorf("marshal failed: %w", err)
	}
	_ = peer.conn // 实验性实现不发起 RPC
	return fmt.Errorf("GrpcTransport.Send target=%s: %w", target, ErrGrpcTransportNotImplemented)
}

// Receive 入站消息通道。
func (t *GrpcTransport) Receive() <-chan *bus.BusMessage {
	return t.inbox
}

// Start gRPC 服务。
func (t *GrpcTransport) Start(addr string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	t.listener = lis
	t.localAddr = addr
	t.server = grpc.NewServer()
	go func() { _ = t.server.Serve(lis) }()
	return nil
}

// Close 关闭。
func (t *GrpcTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.server != nil {
		t.server.Stop()
	}
	if t.listener != nil {
		t.listener.Close()
	}
	for _, peer := range t.peers {
		if peer.conn != nil {
			peer.conn.Close()
		}
	}
	close(t.inbox)
	return nil
}

var _ Transport = (*GrpcTransport)(nil)

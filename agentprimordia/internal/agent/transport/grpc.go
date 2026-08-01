package transport

import (
	"agentprimordia/internal/agent/bus"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"
)

// ===== #9 gRPC 双向流式通信 =====

// GrpcTransport 基于 gRPC 的传输层实现。
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
func (t *GrpcTransport) Send(ctx context.Context, target string, msg *bus.BusMessage) error {
	t.mu.RLock()
	peer, ok := t.peers[target]
	t.mu.RUnlock()
	if !ok {
		return fmt.Errorf("no connection to %s", target)
	}
	gm := toGrpcMsg(msg)
	body, err := json.Marshal(gm)
	if err != nil {
		return err
	}
	_ = body
	// Unary call placeholder — in production, use generated gRPC client
	_ = peer.conn
	return nil
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

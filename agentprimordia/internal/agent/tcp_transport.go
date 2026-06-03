package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultAckTimeout    = 10 * time.Second
	defaultMaxRetries    = 3
	defaultRetryInterval = 500 * time.Millisecond
	defaultPoolSize      = 8
)

// 安全警告：TCPTransport 以明文传输消息，不适合在生产环境中传输敏感数据
// 生产环境请使用 HTTPTransport + TLS 或自行实现加密传输层

type TCPTransportConfig struct {
	AckTimeout    time.Duration
	MaxRetries    int
	RetryInterval time.Duration
	PoolSize      int
}

func DefaultTCPTransportConfig() TCPTransportConfig {
	return TCPTransportConfig{
		AckTimeout:    defaultAckTimeout,
		MaxRetries:    defaultMaxRetries,
		RetryInterval: defaultRetryInterval,
		PoolSize:      defaultPoolSize,
	}
}

type TCPTransport struct {
	config  TCPTransportConfig
	client  *net.Dialer
	inbound chan *BusMessage
	addr    string
	mu      sync.RWMutex
	started bool
	ln      net.Listener
	logger  *slog.Logger

	pool   *connPool
	ackSeq atomic.Uint64
	acks   map[uint64]chan struct{}
	ackMu  sync.RWMutex
}

func NewTCPTransport() *TCPTransport {
	return NewTCPTransportWithConfig(DefaultTCPTransportConfig())
}

func NewTCPTransportWithConfig(cfg TCPTransportConfig) *TCPTransport {
	if cfg.AckTimeout == 0 {
		cfg.AckTimeout = defaultAckTimeout
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = defaultMaxRetries
	}
	if cfg.RetryInterval == 0 {
		cfg.RetryInterval = defaultRetryInterval
	}
	if cfg.PoolSize == 0 {
		cfg.PoolSize = defaultPoolSize
	}

	return &TCPTransport{
		config:  cfg,
		client:  &net.Dialer{Timeout: 30 * time.Second},
		inbound: make(chan *BusMessage, inboundBufSize),
		logger:  slog.Default(),
		acks:    make(map[uint64]chan struct{}),
	}
}

func (t *TCPTransport) Start(addr string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.started {
		return fmt.Errorf("transport already started on %s", t.addr)
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s failed: %w", addr, err)
	}

	t.ln = ln
	t.addr = ln.Addr().String()
	t.started = true
	t.pool = newConnPool(t.config.PoolSize, t.client)

	go t.acceptLoop()

	t.logger.Info("TCP transport 已启动", "addr", t.addr)
	return nil
}

func (t *TCPTransport) Send(ctx context.Context, target string, msg *BusMessage) error {
	t.mu.RLock()
	started := t.started
	t.mu.RUnlock()

	if !started {
		return fmt.Errorf("transport not started")
	}

	return t.sendWithRetry(ctx, target, msg, t.config.MaxRetries)
}

func (t *TCPTransport) SendWithAck(ctx context.Context, target string, msg *BusMessage) error {
	t.mu.RLock()
	started := t.started
	t.mu.RUnlock()

	if !started {
		return fmt.Errorf("transport not started")
	}

	seq := t.ackSeq.Add(1)
	ackCh := make(chan struct{}, 1)

	t.ackMu.Lock()
	t.acks[seq] = ackCh
	t.ackMu.Unlock()

	defer func() {
		t.ackMu.Lock()
		delete(t.acks, seq)
		t.ackMu.Unlock()
	}()

	ackMsg := *msg
	if ackMsg.Metadata == nil {
		ackMsg.Metadata = make(map[string]string)
	}
	ackMsg.Metadata["_ack_seq"] = fmt.Sprintf("%d", seq)
	ackMsg.Metadata["_ack_required"] = "true"

	if err := t.sendWithRetry(ctx, target, &ackMsg, t.config.MaxRetries); err != nil {
		return err
	}

	select {
	case <-ackCh:
		return nil
	case <-time.After(t.config.AckTimeout):
		return fmt.Errorf("ack timeout for message %s (seq=%d)", msg.ID, seq)
	case <-ctx.Done():
		return ctx.Err()
	}
}

// sendWithRetry 发送消息，对于需要ACK的消息保持连接以读取响应
func (t *TCPTransport) sendWithRetry(ctx context.Context, target string, msg *BusMessage, maxRetries int) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message failed: %w", err)
	}

	needAck := msg.Metadata != nil && msg.Metadata["_ack_required"] == "true"

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(t.config.RetryInterval):
			case <-ctx.Done():
				return ctx.Err()
			}
			t.logger.Debug("TCP 重试发送", "target", target, "attempt", attempt, "msg_id", msg.ID)
		}

		conn, err := t.pool.Get(ctx, target)
		if err != nil {
			lastErr = fmt.Errorf("dial %s failed: %w", target, err)
			t.pool.Invalidate(target)
			continue
		}

		_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if _, err := conn.Write(data); err != nil {
			conn.Close()
			t.pool.Invalidate(target)
			lastErr = fmt.Errorf("write to %s failed: %w", target, err)
			continue
		}

		if needAck {
			if err := t.readAckResponse(conn, msg.ID); err != nil {
				conn.Close()
				t.pool.Invalidate(target)
				lastErr = fmt.Errorf("read ack from %s failed: %w", target, err)
				continue
			}
			conn.Close()
			return nil
		}

		conn.Close()
		return nil
	}

	return fmt.Errorf("send failed after %d retries: %w", maxRetries, lastErr)
}

// readAckResponse 从连接中读取ACK响应
func (t *TCPTransport) readAckResponse(conn net.Conn, msgID string) error {
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	defer func() { _ = conn.SetReadDeadline(time.Time{}) }()

	var buf [4096]byte
	n, err := conn.Read(buf[:])
	if err != nil {
		return fmt.Errorf("read ack failed: %w", err)
	}

	var ack struct {
		Type string `json:"type"`
		Seq  uint64 `json:"seq"`
	}
	if err := json.Unmarshal(buf[:n], &ack); err != nil {
		return nil
	}

	if ack.Type == "ack" {
		t.handleAck(ack.Seq)
	}

	return nil
}

func (t *TCPTransport) Receive() <-chan *BusMessage {
	return t.inbound
}

func (t *TCPTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.started {
		return nil
	}

	if err := t.ln.Close(); err != nil {
		return fmt.Errorf("close listener failed: %w", err)
	}

	if t.pool != nil {
		t.pool.CloseAll()
	}

	close(t.inbound)
	t.started = false
	t.logger.Info("TCP transport 已关闭", "addr", t.addr)
	return nil
}

func (t *TCPTransport) Addr() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.addr
}

func (t *TCPTransport) PoolStats() (active, idle int) {
	if t.pool == nil {
		return 0, 0
	}
	return t.pool.Stats()
}

func (t *TCPTransport) acceptLoop() {
	for {
		conn, err := t.ln.Accept()
		if err != nil {
			return
		}
		go t.handleConn(conn)
	}
}

func (t *TCPTransport) handleConn(conn net.Conn) {
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	var msg BusMessage
	if err := json.NewDecoder(conn).Decode(&msg); err != nil {
		t.logger.Warn("TCP 解码消息失败", "error", err)
		return
	}

	if msg.Metadata != nil {
		if ackSeq, ok := msg.Metadata["_ack_seq"]; ok && msg.Metadata["_ack_required"] == "true" {
			t.sendAck(conn, ackSeq)
		}
	}

	select {
	case t.inbound <- &msg:
	default:
		t.logger.Warn("入站通道已满，丢弃消息", "from", msg.From, "id", msg.ID)
	}
}

func (t *TCPTransport) sendAck(conn net.Conn, seqStr string) {
	ackData := fmt.Sprintf(`{"type":"ack","seq":%s}`, seqStr)
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, _ = conn.Write([]byte(ackData + "\n"))
}

func (t *TCPTransport) handleAck(seq uint64) {
	t.ackMu.RLock()
	ch, ok := t.acks[seq]
	t.ackMu.RUnlock()

	if ok {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

type connPool struct {
	dialer  *net.Dialer
	maxSize int
	mu      sync.Mutex
	conns   map[string][]*poolConn
	closed  bool
}

type poolConn struct {
	net.Conn
	lastUsed time.Time
}

func newConnPool(maxSize int, dialer *net.Dialer) *connPool {
	return &connPool{
		dialer:  dialer,
		maxSize: maxSize,
		conns:   make(map[string][]*poolConn),
	}
}

func (p *connPool) Get(ctx context.Context, target string) (net.Conn, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil, fmt.Errorf("pool closed")
	}

	if conns := p.conns[target]; len(conns) > 0 {
		pc := conns[len(conns)-1]
		p.conns[target] = conns[:len(conns)-1]
		pc.lastUsed = time.Now()
		return pc.Conn, nil
	}

	conn, err := p.dialer.DialContext(ctx, "tcp", target)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

func (p *connPool) Put(target string, conn net.Conn) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		conn.Close()
		return
	}

	pc := &poolConn{Conn: conn, lastUsed: time.Now()}
	p.conns[target] = append(p.conns[target], pc)

	if len(p.conns[target]) > p.maxSize {
		oldest := p.conns[target][0]
		p.conns[target] = p.conns[target][1:]
		oldest.Close()
	}
}

func (p *connPool) Invalidate(target string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, pc := range p.conns[target] {
		pc.Close()
	}
	delete(p.conns, target)
}

func (p *connPool) Stats() (active, idle int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, conns := range p.conns {
		idle += len(conns)
	}
	return 0, idle
}

func (p *connPool) CloseAll() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.closed = true
	for target, conns := range p.conns {
		for _, pc := range conns {
			pc.Close()
		}
		delete(p.conns, target)
	}
}

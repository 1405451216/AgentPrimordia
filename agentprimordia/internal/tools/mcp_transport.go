package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

// mcpTransport 抽象 MCP 传输层，支持 HTTP 和 stdio 两种模式
type mcpTransport interface {
	// SendRequest 发送 JSON-RPC 请求并等待响应
	SendRequest(ctx context.Context, req *MCPRequest) (*MCPResponse, error)
	// SendNotification 发送 JSON-RPC 通知（无需响应）
	SendNotification(ctx context.Context, req *MCPRequest) error
	// Close 关闭传输层
	Close() error
}

// ===== HTTP Transport =====

type mcpHTTPTransport struct {
	baseURL string
	client  *http.Client
}

func newHTTPTransport(baseURL string) *mcpHTTPTransport {
	return &mcpHTTPTransport{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{},
	}
}

func (t *mcpHTTPTransport) SendRequest(ctx context.Context, req *MCPRequest) (*MCPResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", t.baseURL, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, mcpMaxResponseBody))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("MCP server returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var mcpResp MCPResponse
	if err := json.Unmarshal(respBody, &mcpResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &mcpResp, nil
}

func (t *mcpHTTPTransport) SendNotification(ctx context.Context, req *MCPRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", t.baseURL, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(httpReq)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (t *mcpHTTPTransport) Close() error { return nil }

// ===== stdio Transport =====

type mcpStdioTransport struct {
	stdin  io.WriteCloser
	stdout io.ReadCloser
	mu     sync.Mutex // 保护 stdin 写入顺序
	buf    *bufio.Scanner
	respCh map[int]chan *MCPResponse // 等待响应的回调通道
	respMu sync.Mutex
}

func newStdioTransport(stdin io.WriteCloser, stdout io.ReadCloser) *mcpStdioTransport {
	t := &mcpStdioTransport{
		stdin:  stdin,
		stdout: stdout,
		buf:    bufio.NewScanner(stdout),
		respCh: make(map[int]chan *MCPResponse),
	}
	// 启动后台 goroutine 持续读取响应
	go t.readLoop()
	return t
}

func (t *mcpStdioTransport) readLoop() {
	for t.buf.Scan() {
		line := strings.TrimSpace(t.buf.Text())
		if line == "" {
			continue
		}
		var resp MCPResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			continue
		}
		t.respMu.Lock()
		if ch, ok := t.respCh[resp.ID]; ok {
			delete(t.respCh, resp.ID)
			ch <- &resp
		}
		t.respMu.Unlock()
	}
}

func (t *mcpStdioTransport) SendRequest(ctx context.Context, req *MCPRequest) (*MCPResponse, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	respCh := make(chan *MCPResponse, 1)
	t.respMu.Lock()
	t.respCh[req.ID] = respCh
	t.respMu.Unlock()

	// 写入 stdin（JSON-RPC over stdio：每行一个 JSON）
	t.mu.Lock()
	_, err = fmt.Fprintf(t.stdin, "%s\n", string(reqBody))
	t.mu.Unlock()
	if err != nil {
		t.respMu.Lock()
		delete(t.respCh, req.ID)
		t.respMu.Unlock()
		return nil, fmt.Errorf("stdio write: %w", err)
	}

	select {
	case resp := <-respCh:
		return resp, nil
	case <-ctx.Done():
		t.respMu.Lock()
		delete(t.respCh, req.ID)
		t.respMu.Unlock()
		return nil, ctx.Err()
	}
}

func (t *mcpStdioTransport) SendNotification(ctx context.Context, req *MCPRequest) error {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return err
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	_, err = fmt.Fprintf(t.stdin, "%s\n", string(reqBody))
	return err
}

func (t *mcpStdioTransport) Close() error {
	var errs []error
	if t.stdin != nil {
		if err := t.stdin.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if t.stdout != nil {
		if err := t.stdout.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("close errors: %v", errs)
	}
	return nil
}

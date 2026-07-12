package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// notifications_test

func TestToolListChangedNotifier_SubscribeAndNotify(t *testing.T) {
	n := newToolListChangedNotifier()
	ch := n.Subscribe()
	if n.subscriberCount() != 1 {
		t.Fatalf("subscriberCount = %d, want 1", n.subscriberCount())
	}
	n.Notify()
	select {
	case <-ch:
		// good
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for notification")
	}
}

func TestToolListChangedNotifier_Unsubscribe(t *testing.T) {
	n := newToolListChangedNotifier()
	ch := n.Subscribe()
	n.Unsubscribe(ch)
	if n.subscriberCount() != 0 {
		t.Fatalf("subscriberCount = %d, want 0", n.subscriberCount())
	}
	n.Notify() // should not panic
}

func TestToolListChangedNotifier_MultipleSubscribers(t *testing.T) {
	n := newToolListChangedNotifier()
	for i := 0; i < 5; i++ {
		_ = n.Subscribe()
	}
	if n.subscriberCount() != 5 {
		t.Fatalf("subscriberCount = %d, want 5", n.subscriberCount())
	}
	n.Notify()
}

// resourceRegistry_test

func TestResourceRegistry_RegisterAndList(t *testing.T) {
	r := newResourceRegistry()
	h := func(ctx context.Context, uri string) ([]byte, error) { return []byte("data"), nil }
	r.Register("agent://mem", "Memory", "application/json", h)
	list := r.List()
	if len(list) != 1 {
		t.Fatalf("List len = %d, want 1", len(list))
	}
	if list[0].URI != "agent://mem" {
		t.Errorf("URI = %q, want agent://mem", list[0].URI)
	}
}

func TestResourceRegistry_Read(t *testing.T) {
	r := newResourceRegistry()
	expected := "hello-world"
	r.Register("agent://mem", "Memory", "application/json",
		func(ctx context.Context, uri string) ([]byte, error) { return []byte(expected), nil })
	rc, err := r.Read(context.Background(), "agent://mem")
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if rc.Text != expected {
		t.Errorf("Text = %q, want %q", rc.Text, expected)
	}
	if rc.MimeType != "application/json" {
		t.Errorf("MimeType = %q, want application/json", rc.MimeType)
	}
}

func TestResourceRegistry_ReadNotFound(t *testing.T) {
	r := newResourceRegistry()
	_, err := r.Read(context.Background(), "agent://missing")
	if err == nil {
		t.Fatal("expected error for missing resource")
	}
}

func TestResourceRegistry_Unregister(t *testing.T) {
	r := newResourceRegistry()
	h := func(ctx context.Context, uri string) ([]byte, error) { return []byte("x"), nil }
	r.Register("agent://mem", "Memory", "application/json", h)
	r.Unregister("agent://mem")
	if r.Count() != 0 {
		t.Fatalf("Count = %d, want 0", r.Count())
	}
}

// sessionResourceHandler_test

func TestSessionResourceHandler(t *testing.T) {
	h := sessionResourceHandler(func(ctx context.Context, sid string) ([]byte, error) {
		return []byte("session-" + sid), nil
	})
	data, err := h(context.Background(), "agent://session/abc123")
	if err != nil {
		t.Fatalf("handler failed: %v", err)
	}
	if string(data) != "session-abc123" {
		t.Errorf("got %q, want session-abc123", string(data))
	}
}

func TestSessionResourceHandler_BadPrefix(t *testing.T) {
	h := sessionResourceHandler(func(ctx context.Context, sid string) ([]byte, error) { return nil, nil })
	_, err := h(context.Background(), "http://wrong-prefix")
	if err == nil {
		t.Fatal("expected error for wrong URI prefix")
	}
}

func TestSessionResourceHandler_MissingID(t *testing.T) {
	h := sessionResourceHandler(func(ctx context.Context, sid string) ([]byte, error) { return nil, nil })
	_, err := h(context.Background(), "agent://session/")
	if err == nil {
		t.Fatal("expected error for empty session ID")
	}
}

// promptRegistry_test

func TestPromptRegistry_RegisterAndGet(t *testing.T) {
	r := newPromptRegistry()
	called := false
	r.Register("greet", func(ctx context.Context, args map[string]string) (string, error) {
		called = true
		return "Hello, " + args["name"], nil
	})
	prompts := r.List()
	if len(prompts) != 1 {
		t.Fatalf("List len = %d, want 1", len(prompts))
	}
	text, err := r.Get(context.Background(), "greet", map[string]string{"name": "World"})
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if !called {
		t.Fatal("handler was not invoked")
	}
	if text != "Hello, World" {
		t.Errorf("got %q, want Hello, World", text)
	}
}

func TestPromptRegistry_GetNotFound(t *testing.T) {
	r := newPromptRegistry()
	_, err := r.Get(context.Background(), "nope", nil)
	if err == nil {
		t.Fatal("expected error for missing prompt")
	}
}

func TestBuiltinPrompts_Summarize(t *testing.T) {
	text, err := summarizeHandler(context.Background(), map[string]string{"topic": "AI agents", "language": "zh"})
	if err != nil {
		t.Fatalf("summarize failed: %v", err)
	}
	if !strings.Contains(text, "AI agents") {
		t.Errorf("expected topic in output, got %q", text)
	}
}

func TestBuiltinPrompts_SummarizeMissingArg(t *testing.T) {
	_, err := summarizeHandler(context.Background(), map[string]string{})
	if err == nil {
		t.Fatal("expected error for missing topic arg")
	}
}

func TestBuiltinPrompts_Analyze(t *testing.T) {
	text, err := analyzeHandler(context.Background(), map[string]string{"data": "[1,2,3]", "format": "json"})
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}
	if !strings.Contains(text, "[1,2,3]") || !strings.Contains(text, "json") {
		t.Errorf("unexpected output: %q", text)
	}
}

func TestBuiltinPrompts_Plan(t *testing.T) {
	text, err := planHandler(context.Background(), map[string]string{"goal": "build a house", "steps": "5"})
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}
	if !strings.Contains(text, "build a house") || !strings.Contains(text, "5") {
		t.Errorf("unexpected output: %q", text)
	}
}

func TestBuiltinPrompts_RegisterTo(t *testing.T) {
	r := newPromptRegistry()
	NewBuiltinPrompts().RegisterTo(r)
	if r.Count() != 3 {
		t.Fatalf("Count = %d, want 3", r.Count())
	}
}

// MCPServer HTTP-level tests

func newTestServer(t *testing.T) (*MCPServer, string) {
	t.Helper()
	br := DefaultBuiltinResources()
	s := NewMCPServer(WithBuiltinResources(br), WithBuiltinPrompts())
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return s, srv.URL
}

func postMCP(t *testing.T, base, method string, params any) *jsonRPCResponse {
	t.Helper()
	id := time.Now().UnixNano()
	body := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, base+"/mcp", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	var rpcResp jsonRPCResponse
	_ = json.Unmarshal(respBody, &rpcResp)
	return &rpcResp
}

func TestMCPServer_Initialize(t *testing.T) {
	_, base := newTestServer(t)
	resp := postMCP(t, base, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "test", "version": "1.0"},
	})
	if resp.Error != nil {
		t.Fatalf("initialize error: %v", resp.Error)
	}
	var result initializeResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal init result: %v", err)
	}
	if result.ProtocolVersion != "2024-11-05" {
		t.Errorf("ProtocolVersion = %q, want 2024-11-05", result.ProtocolVersion)
	}
	if result.ServerInfo.Name != "AgentPrimordia-MCP" {
		t.Errorf("ServerInfo.Name = %q, want AgentPrimordia-MCP", result.ServerInfo.Name)
	}
}

func TestMCPServer_Ping(t *testing.T) {
	_, base := newTestServer(t)
	resp := postMCP(t, base, "ping", nil)
	if resp.Error != nil {
		t.Fatalf("ping error: %v", resp.Error)
	}
}

func TestMCPServer_MethodNotFound(t *testing.T) {
	_, base := newTestServer(t)
	resp := postMCP(t, base, "unknown/method", nil)
	if resp.Error == nil {
		t.Fatal("expected error for unknown method")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("error code = %d, want -32601", resp.Error.Code)
	}
}

func TestMCPServer_ToolsList(t *testing.T) {
	_, base := newTestServer(t)
	resp := postMCP(t, base, "tools/list", nil)
	if resp.Error != nil {
		t.Fatalf("tools/list error: %v", resp.Error)
	}
	var result struct {
		Tools []ToolDefinition `json:"tools"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
}

func TestMCPServer_ToolsCall(t *testing.T) {
	s := NewMCPServer()
	s.RegisterTool("echo", "echo back",
		func(ctx context.Context, args map[string]any) (string, bool, error) {
			msg, _ := args["msg"].(string)
			return msg, false, nil
		})
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	resp := postMCP(t, srv.URL, "tools/call", map[string]any{
		"name":      "echo",
		"arguments": map[string]any{"msg": "hello"},
	})
	if resp.Error != nil {
		t.Fatalf("tools/call error: %v", resp.Error)
	}
	var result ToolCallResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(result.Content) == 0 || result.Content[0].Text != "hello" {
		t.Errorf("expected hello, got %v", result.Content)
	}
}

func TestMCPServer_ToolsCallNotFound(t *testing.T) {
	s := NewMCPServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	resp := postMCP(t, srv.URL, "tools/call", map[string]any{
		"name":      "missing",
		"arguments": map[string]any{},
	})
	if resp.Error == nil {
		t.Fatal("expected error for missing tool")
	}
}

func TestMCPServer_ResourcesList(t *testing.T) {
	br := DefaultBuiltinResources()
	s := NewMCPServer(WithBuiltinResources(br))
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	resp := postMCP(t, srv.URL, "resources/list", nil)
	if resp.Error != nil {
		t.Fatalf("resources/list error: %v", resp.Error)
	}
	var result struct {
		Resources []Resource `json:"resources"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(result.Resources) == 0 {
		t.Fatal("expected at least one resource from builtins")
	}
}

func TestMCPServer_ResourceRead(t *testing.T) {
	s := NewMCPServer()
	s.RegisterResource("test://data", "Test Data", "text/plain",
		func(ctx context.Context, uri string) ([]byte, error) { return []byte("payload"), nil })
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	resp := postMCP(t, srv.URL, "resources/read", map[string]any{"uri": "test://data"})
	if resp.Error != nil {
		t.Fatalf("resources/read error: %v", resp.Error)
	}
	var result readResourceResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(result.Contents) == 0 || result.Contents[0].Text != "payload" {
		t.Errorf("expected payload, got %v", result.Contents)
	}
}

func TestMCPServer_PromptsList(t *testing.T) {
	_, base := newTestServer(t)
	resp := postMCP(t, base, "prompts/list", nil)
	if resp.Error != nil {
		t.Fatalf("prompts/list error: %v", resp.Error)
	}
	var result struct {
		Prompts []PromptDefinition `json:"prompts"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(result.Prompts) != 3 {
		t.Errorf("expected 3 built-in prompts, got %d", len(result.Prompts))
	}
}

func TestMCPServer_PromptGet(t *testing.T) {
	_, base := newTestServer(t)
	resp := postMCP(t, base, "prompts/get", map[string]any{
		"name":      "summarize",
		"arguments": map[string]any{"topic": "AI"},
	})
	if resp.Error != nil {
		t.Fatalf("prompts/get error: %v", resp.Error)
	}
	var result getPromptResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(result.Messages) == 0 || !strings.Contains(result.Messages[0].Content.Text, "AI") {
		t.Errorf("expected AI in prompt text, got %v", result.Messages)
	}
}

func TestMCPServer_NotifyToolListChanged(t *testing.T) {
	s := NewMCPServer()
	if err := s.NotifyToolListChanged(); err != nil {
		t.Fatalf("NotifyToolListChanged failed: %v", err)
	}
}

func TestMCPServer_MethodNotAllowed(t *testing.T) {
	s := NewMCPServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/mcp", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", resp.StatusCode)
	}
}

func TestMCPServer_CustomResource(t *testing.T) {
	s := NewMCPServer()
	s.RegisterResource("custom://x", "Custom", "application/json",
		func(ctx context.Context, uri string) ([]byte, error) { return []byte("custom-true"), nil })
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	resp := postMCP(t, srv.URL, "resources/read", map[string]any{"uri": "custom://x"})
	if resp.Error != nil {
		t.Fatalf("custom resource read error: %v", resp.Error)
	}
	var result readResourceResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(result.Contents) == 0 || !strings.Contains(result.Contents[0].Text, "custom-true") {
		t.Errorf("unexpected content: %v", result.Contents)
	}
}

func TestMCPServer_ConcurrentRegistration(t *testing.T) {
	s := NewMCPServer()
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			s.RegisterTool(fmt.Sprintf("tool-%d", n), "desc",
				func(ctx context.Context, args map[string]any) (string, bool, error) { return "ok", false, nil })
		}(i)
	}
	wg.Wait()
	if s.tools.Count() != 10 {
		t.Fatalf("tools Count = %d, want 10", s.tools.Count())
	}
}

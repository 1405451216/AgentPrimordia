package debugger

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// newDebugTestServer 创建基于 DebugServer 实例级 mux 的 httptest.Server
func newDebugTestServer(d *DebugServer) *httptest.Server {
	return httptest.NewServer(d.Handler())
}

// ===== 功能验证 - AddEvent =====

func TestDebugServer_AddEvent(t *testing.T) {
	d := NewDebugServer("127.0.0.1:0")

	d.AddEvent("llm_call", "调用 GPT-4")
	d.AddEvent("tool_exec", "执行 shell 命令")

	d.mu.RLock()
	events := d.events
	d.mu.RUnlock()

	if len(events) != 2 {
		t.Fatalf("期望 2 个事件，得到 %d", len(events))
	}
	if events[0].Type != "llm_call" {
		t.Errorf("事件[0].Type 期望 llm_call，得到 %s", events[0].Type)
	}
	if events[0].Message != "调用 GPT-4" {
		t.Errorf("事件[0].Message 期望 '调用 GPT-4'，得到 %s", events[0].Message)
	}
	if events[1].Type != "tool_exec" {
		t.Errorf("事件[1].Type 期望 tool_exec，得到 %s", events[1].Type)
	}
	if events[1].Message != "执行 shell 命令" {
		t.Errorf("事件[1].Message 期望 '执行 shell 命令'，得到 %s", events[1].Message)
	}
	if events[0].Timestamp == "" {
		t.Error("事件应包含非空 Timestamp")
	}
}

// ===== 功能验证 - AddSnapshot =====

func TestDebugServer_AddSnapshot(t *testing.T) {
	d := NewDebugServer("127.0.0.1:0")

	snap := MemorySnapshot{
		TotalEpisodes: 42,
		TopSessions: []SessionInfo{
			{SessionID: "sess-001", Count: 10},
			{SessionID: "sess-002", Count: 5},
		},
		RecentEvents: []RecentEvent{
			{Time: "10:00:00", Role: "user", Content: "你好"},
		},
	}
	d.AddSnapshot(snap)

	d.mu.RLock()
	snapshots := d.snapshots
	d.mu.RUnlock()

	if len(snapshots) != 1 {
		t.Fatalf("期望 1 个快照，得到 %d", len(snapshots))
	}
	if snapshots[0].TotalEpisodes != 42 {
		t.Errorf("TotalEpisodes 期望 42，得到 %d", snapshots[0].TotalEpisodes)
	}
	if len(snapshots[0].TopSessions) != 2 {
		t.Errorf("TopSessions 长度期望 2，得到 %d", len(snapshots[0].TopSessions))
	}
	if snapshots[0].TopSessions[0].SessionID != "sess-001" {
		t.Errorf("TopSessions[0].SessionID 期望 sess-001，得到 %s", snapshots[0].TopSessions[0].SessionID)
	}
	if len(snapshots[0].RecentEvents) != 1 {
		t.Errorf("RecentEvents 长度期望 1，得到 %d", len(snapshots[0].RecentEvents))
	}
}

// ===== 边界条件 - 事件上限 100 =====

func TestDebugServer_EventCapAt100(t *testing.T) {
	d := NewDebugServer("127.0.0.1:0")

	for i := 0; i < 101; i++ {
		d.AddEvent("type", "消息")
	}

	d.mu.RLock()
	events := d.events
	d.mu.RUnlock()

	if len(events) != 100 {
		t.Errorf("事件应限制为 100 个，得到 %d", len(events))
	}
}

// ===== 边界条件 - 快照上限 10 =====

func TestDebugServer_SnapshotCapAt10(t *testing.T) {
	d := NewDebugServer("127.0.0.1:0")

	for i := 0; i < 11; i++ {
		d.AddSnapshot(MemorySnapshot{TotalEpisodes: int64(i)})
	}

	d.mu.RLock()
	snapshots := d.snapshots
	d.mu.RUnlock()

	if len(snapshots) != 10 {
		t.Errorf("快照应限制为 10 个，得到 %d", len(snapshots))
	}

	if snapshots[0].TotalEpisodes != 1 {
		t.Errorf("最早的快照应被淘汰，第一个快照 TotalEpisodes 期望 1，得到 %d", snapshots[0].TotalEpisodes)
	}
}

// ===== 边界条件 - 空事件列表 =====

func TestDebugServer_EmptyEvents(t *testing.T) {
	d := NewDebugServer("127.0.0.1:0")

	d.mu.RLock()
	events := d.events
	d.mu.RUnlock()

	if len(events) != 0 {
		t.Errorf("新服务器事件列表应为空，得到 %d 个", len(events))
	}
}

// ===== 边界条件 - 空快照列表 =====

func TestDebugServer_EmptySnapshots(t *testing.T) {
	d := NewDebugServer("127.0.0.1:0")

	d.mu.RLock()
	snapshots := d.snapshots
	d.mu.RUnlock()

	if len(snapshots) != 0 {
		t.Errorf("新服务器快照列表应为空，得到 %d 个", len(snapshots))
	}
}

// ===== 并发安全 - 并发 AddEvent =====

func TestDebugServer_ConcurrentAddEvent(t *testing.T) {
	d := NewDebugServer("127.0.0.1:0")

	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			d.AddEvent("concurrent", "并发事件")
		}(i)
	}
	wg.Wait()

	d.mu.RLock()
	events := d.events
	d.mu.RUnlock()

	if len(events) != 100 {
		t.Errorf("并发添加 200 个事件后应限制为 100 个，得到 %d", len(events))
	}
}

// ===== 并发安全 - 并发 AddSnapshot =====

func TestDebugServer_ConcurrentAddSnapshot(t *testing.T) {
	d := NewDebugServer("127.0.0.1:0")

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			d.AddSnapshot(MemorySnapshot{TotalEpisodes: int64(idx)})
		}(i)
	}
	wg.Wait()

	d.mu.RLock()
	snapshots := d.snapshots
	d.mu.RUnlock()

	if len(snapshots) != 10 {
		t.Errorf("并发添加 50 个快照后应限制为 10 个，得到 %d", len(snapshots))
	}
}

// ===== HTTP 端点 - 通过 httptest 测试 =====

func TestDebugServer_HTTPIndexEndpoint(t *testing.T) {
	d := NewDebugServer("127.0.0.1:0")
	ts := newDebugTestServer(d)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("请求 / 失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("期望状态码 200，得到 %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "Agent Debugger") {
		t.Error("首页应包含 'Agent Debugger'")
	}
	if !strings.Contains(bodyStr, "<html") {
		t.Error("首页应包含 HTML 标签")
	}
}

func TestDebugServer_HTTPEventsEndpoint(t *testing.T) {
	d := NewDebugServer("127.0.0.1:0")
	d.AddEvent("llm_call", "调用模型")
	d.AddEvent("tool_exec", "执行工具")

	ts := newDebugTestServer(d)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/events")
	if err != nil {
		t.Fatalf("请求 /api/events 失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("期望状态码 200，得到 %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var events []DebugEvent
	if err := json.Unmarshal(body, &events); err != nil {
		t.Fatalf("解析 JSON 失败: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("期望 2 个事件，得到 %d", len(events))
	}
	if events[0].Type != "llm_call" {
		t.Errorf("事件[0].Type 期望 llm_call，得到 %s", events[0].Type)
	}
	if events[1].Message != "执行工具" {
		t.Errorf("事件[1].Message 期望 '执行工具'，得到 %s", events[1].Message)
	}
}

func TestDebugServer_HTTPSnapshotsEndpoint(t *testing.T) {
	d := NewDebugServer("127.0.0.1:0")
	d.AddSnapshot(MemorySnapshot{
		TotalEpisodes: 99,
		TopSessions:   []SessionInfo{{SessionID: "s1", Count: 7}},
		RecentEvents:  []RecentEvent{{Time: "12:00", Role: "user", Content: "test"}},
	})

	ts := newDebugTestServer(d)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/snapshots")
	if err != nil {
		t.Fatalf("请求 /api/snapshots 失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("期望状态码 200，得到 %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var snapshots []MemorySnapshot
	if err := json.Unmarshal(body, &snapshots); err != nil {
		t.Fatalf("解析 JSON 失败: %v", err)
	}
	if len(snapshots) != 1 {
		t.Errorf("期望 1 个快照，得到 %d", len(snapshots))
	}
	if snapshots[0].TotalEpisodes != 99 {
		t.Errorf("TotalEpisodes 期望 99，得到 %d", snapshots[0].TotalEpisodes)
	}
}

// ===== HTTP 端点 - Content-Type 验证 =====

func TestDebugServer_HTTPContentTypes(t *testing.T) {
	d := NewDebugServer("127.0.0.1:0")
	ts := newDebugTestServer(d)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("请求 / 失败: %v", err)
	}
	resp.Body.Close()
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("/ 期望 Content-Type 包含 text/html，得到 %s", ct)
	}

	resp, err = http.Get(ts.URL + "/api/events")
	if err != nil {
		t.Fatalf("请求 /api/events 失败: %v", err)
	}
	resp.Body.Close()
	ct = resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("/api/events 期望 Content-Type 包含 application/json，得到 %s", ct)
	}

	resp, err = http.Get(ts.URL + "/api/snapshots")
	if err != nil {
		t.Fatalf("请求 /api/snapshots 失败: %v", err)
	}
	resp.Body.Close()
	ct = resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("/api/snapshots 期望 Content-Type 包含 application/json，得到 %s", ct)
	}
}

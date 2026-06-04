package debugger

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestVisualizer_NewVisualizer(t *testing.T) {
	v := NewVisualizer()
	if v == nil {
		t.Fatal("NewVisualizer 返回 nil")
	}
}

func TestVisualizer_RenderMemorySnapshot(t *testing.T) {
	v := NewVisualizer()
	snapshot := &MemorySnapshot{
		TotalEpisodes: 42,
		TopSessions: []SessionInfo{
			{SessionID: "session-1", Count: 20},
			{SessionID: "session-2", Count: 15},
		},
		RecentEvents: []RecentEvent{
			{Time: "10:00:00", Role: "user", Content: "hello"},
			{Time: "10:00:05", Role: "assistant", Content: "hi"},
		},
	}
	out := v.RenderMemorySnapshot(snapshot)

	// 验证关键字段
	if !strings.Contains(out, "MEMORY SNAPSHOT") {
		t.Error("缺标题 MEMORY SNAPSHOT")
	}
	if !strings.Contains(out, "Total Episodes: 42") {
		t.Error("缺 Total Episodes")
	}
	if !strings.Contains(out, "session-1") {
		t.Error("缺 session-1")
	}
	if !strings.Contains(out, "hello") {
		t.Error("缺 recent event content")
	}
}

func TestVisualizer_RenderMemorySnapshot_Empty(t *testing.T) {
	v := NewVisualizer()
	out := v.RenderMemorySnapshot(&MemorySnapshot{})
	if !strings.Contains(out, "MEMORY SNAPSHOT") {
		t.Error("空 snapshot 也应输出标题")
	}
}

func TestVisualizer_RenderAsJSON(t *testing.T) {
	v := NewVisualizer()

	// 普通 struct
	type sample struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	out := v.RenderAsJSON(sample{Name: "test", Count: 5})
	if !strings.Contains(out, "\"name\": \"test\"") {
		t.Errorf("JSON 输出缺 name, got: %s", out)
	}

	// 验证是合法 JSON
	var got sample
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("输出不是合法 JSON: %v", err)
	}
	if got.Name != "test" || got.Count != 5 {
		t.Errorf("JSON 解析错: %+v", got)
	}
}

func TestVisualizer_RenderAsJSON_InvalidInput(t *testing.T) {
	v := NewVisualizer()
	// channel 不可 marshal
	out := v.RenderAsJSON(make(chan int))
	if !strings.Contains(out, "Error rendering JSON") {
		t.Errorf("不可序列化输入应返回错误信息, got: %s", out)
	}
}

func TestVisualizer_RenderAgentLifecycle(t *testing.T) {
	v := NewVisualizer()
	states := []LifecycleStep{
		{State: "running", Timestamp: time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC), Message: "开始"},
		{State: "completed", Timestamp: time.Date(2026, 1, 1, 10, 0, 30, 0, time.UTC)},
	}
	out := v.RenderAgentLifecycle(states)

	if !strings.Contains(out, "AGENT LIFECYCLE TRACE") {
		t.Error("缺标题")
	}
	if !strings.Contains(out, "running") {
		t.Error("缺 state running")
	}
	if !strings.Contains(out, "开始") {
		t.Error("缺 message")
	}
}

func TestVisualizer_RenderAgentLifecycle_Empty(t *testing.T) {
	v := NewVisualizer()
	out := v.RenderAgentLifecycle(nil)
	if !strings.Contains(out, "AGENT LIFECYCLE TRACE") {
		t.Error("空 lifecycle 也应输出标题")
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		input  string
		max    int
		expect string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hello..."},
		{"", 5, ""},
		{"abc", 3, "abc"},
	}
	for _, tt := range tests {
		got := truncateString(tt.input, tt.max)
		if got != tt.expect {
			t.Errorf("truncateString(%q, %d) = %q, want %q", tt.input, tt.max, got, tt.expect)
		}
	}
}

package tools

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
)

// dynMockTool 用于测试的模拟工具（区别于 tools_test.go 中的 mockTool）
type dynMockTool struct {
	name string
	desc string
}

func (m *dynMockTool) Name() string        { return m.name }
func (m *dynMockTool) Description() string { return m.desc }
func (m *dynMockTool) Parameters() json.RawMessage {
	return json.RawMessage(`{}`)
}
func (m *dynMockTool) Execute(ctx context.Context, args json.RawMessage) (*Result, error) {
	return NewResult("ok"), nil
}

func TestDynamicRegistry_Register(t *testing.T) {
	dr := NewDynamicRegistry()
	tool := &dynMockTool{name: "test_tool", desc: "a test tool"}
	if err := dr.Register(tool); err != nil {
		t.Fatalf("Register error: %v", err)
	}
	if dr.Count() != 1 {
		t.Errorf("expected count 1, got %d", dr.Count())
	}
	got, ok := dr.Get("test_tool")
	if !ok {
		t.Fatal("expected to find test_tool")
	}
	if got.Name() != "test_tool" {
		t.Errorf("unexpected name: %s", got.Name())
	}
}

func TestDynamicRegistry_Unregister(t *testing.T) {
	dr := NewDynamicRegistry()
	_ = dr.Register(&dynMockTool{name: "test_tool", desc: "test"})
	if err := dr.Unregister("test_tool"); err != nil {
		t.Fatalf("Unregister error: %v", err)
	}
	if dr.Count() != 0 {
		t.Errorf("expected count 0, got %d", dr.Count())
	}
}

func TestDynamicRegistry_ListDynamic(t *testing.T) {
	dr := NewDynamicRegistry()
	_ = dr.Register(&dynMockTool{name: "a", desc: "first"})
	_ = dr.Register(&dynMockTool{name: "b", desc: "second"})
	infos := dr.ListDynamic()
	if len(infos) != 2 {
		t.Fatalf("expected 2 dynamic tools, got %d", len(infos))
	}
	nameSet := make(map[string]bool)
	for _, info := range infos {
		nameSet[info.Name] = true
	}
	if !nameSet["a"] || !nameSet["b"] {
		t.Errorf("expected tools a and b, got %v", nameSet)
	}
}

func TestDynamicRegistry_OnChange(t *testing.T) {
	dr := NewDynamicRegistry()
	var mu sync.Mutex
	events := []ToolChangeEvent{}
	dr.OnChange(func(e ToolChangeEvent) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	})
	_ = dr.Register(&dynMockTool{name: "x", desc: "x"})
	_ = dr.Unregister("x")
	mu.Lock()
	defer mu.Unlock()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Action != ActionRegister {
		t.Errorf("expected register event, got %s", events[0].Action)
	}
	if events[1].Action != ActionUnregister {
		t.Errorf("expected unregister event, got %s", events[1].Action)
	}
}

func TestDynamicRegistry_EmptyName(t *testing.T) {
	dr := NewDynamicRegistry()
	err := dr.Register(&dynMockTool{name: "", desc: "empty"})
	if err != ErrInvalidConfig {
		t.Errorf("expected ErrInvalidConfig, got %v", err)
	}
}

func TestDynamicRegistry_UnregisterNotFound(t *testing.T) {
	dr := NewDynamicRegistry()
	err := dr.Unregister("nonexistent")
	if err != ErrToolNotFound {
		t.Errorf("expected ErrToolNotFound, got %v", err)
	}
}

func TestDynamicRegistry_Definitions(t *testing.T) {
	dr := NewDynamicRegistry()
	_ = dr.Register(&dynMockTool{name: "my_tool", desc: "for testing defs"})
	defs := dr.Definitions()
	if len(defs) != 1 {
		t.Fatalf("expected 1 definition, got %d", len(defs))
	}
	fn, ok := defs[0]["function"].(map[string]any)
	if !ok {
		t.Fatal("expected function definition")
	}
	if fn["name"] != "my_tool" {
		t.Errorf("expected name my_tool, got %v", fn["name"])
	}
}

func TestDynamicRegistry_Concurrent(t *testing.T) {
	dr := NewDynamicRegistry()
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(n int) {
			name := "tool_" + string(rune(97+n))
			_ = dr.Register(&dynMockTool{name: name, desc: "concurrent"})
			done <- true
		}(i)
	}
	for i := 0; i < 10; i++ {
		<-done
	}
	if dr.Count() != 10 {
		t.Errorf("expected 10 tools, got %d", dr.Count())
	}
}

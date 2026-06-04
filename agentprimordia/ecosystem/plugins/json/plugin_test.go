package jsonplugin

import (
	"testing"
)

// TestPlugin_Metadata 验证插件元数据
func TestPlugin_Metadata(t *testing.T) {
	p := New()
	if p.Name() != "json" {
		t.Errorf("Name() = %q, want %q", p.Name(), "json")
	}
	if p.Version() != "0.1.0" {
		t.Errorf("Version() = %q, want %q", p.Version(), "0.1.0")
	}
}

// TestPlugin_Tools_ReturnsTwo 验证 Tools() 返回 2 项 (JSON + CSV)
func TestPlugin_Tools_ReturnsTwo(t *testing.T) {
	p := New()
	tools := p.Tools()
	if len(tools) != 2 {
		t.Errorf("Tools() 返回 %d 项, want 2 (JSON + CSV)", len(tools))
	}
}

func TestPlugin_Init_NoError(t *testing.T) {
	p := New()
	if err := p.Init(nil); err != nil {
		t.Errorf("Init(nil) 报错: %v", err)
	}
	if err := p.Init(map[string]any{}); err != nil {
		t.Errorf("Init({}) 报错: %v", err)
	}
}

func TestPlugin_Close(t *testing.T) {
	p := New()
	if err := p.Close(); err != nil {
		t.Errorf("Close 报错: %v", err)
	}
}

// TestPlugin_Tools_Names 验证两个工具名称不同且都非空
func TestPlugin_Tools_Names(t *testing.T) {
	p := New()
	tools := p.Tools()
	if len(tools) < 2 {
		t.Fatal("Tools 数量不足")
	}
	names := map[string]bool{}
	for i, tool := range tools {
		name := tool.Name()
		if name == "" {
			t.Errorf("tool[%d] 名称为空", i)
		}
		if names[name] {
			t.Errorf("tool[%d] 名称重复: %q", i, name)
		}
		names[name] = true
	}
}

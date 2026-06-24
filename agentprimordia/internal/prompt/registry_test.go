package prompt

import (
	"strings"
	"testing"
)

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()
	tmpl := NewTemplate("你好，{{.name}}！")

	err := r.Register("greeting", tmpl)
	if err != nil {
		t.Fatalf("注册模板失败: %v", err)
	}

	got, err := r.Get("greeting")
	if err != nil {
		t.Fatalf("获取模板失败: %v", err)
	}

	got.WithVar("name", "世界")
	result, err := got.Render()
	if err != nil {
		t.Fatalf("渲染模板失败: %v", err)
	}

	if result != "你好，世界！" {
		t.Errorf("渲染结果 = %q, 期望 %q", result, "你好，世界！")
	}
}

func TestRegistry_DuplicateRegister(t *testing.T) {
	r := NewRegistry()
	r.MustRegister("test", NewTemplate("test"))

	err := r.Register("test", NewTemplate("another"))
	if err == nil {
		t.Error("重复注册应返回错误")
	}
}

func TestRegistry_GetNonExistent(t *testing.T) {
	r := NewRegistry()

	_, err := r.Get("nonexistent")
	if err == nil {
		t.Error("获取不存在的模板应返回错误")
	}
}

func TestRegistry_Render(t *testing.T) {
	r := NewRegistry()
	r.MustRegister("hello", NewTemplate("你好，{{.name}}！今天是{{.day}}。"))

	result, err := r.Render("hello", map[string]any{"name": "张三", "day": "周一"})
	if err != nil {
		t.Fatalf("渲染失败: %v", err)
	}

	if !strings.Contains(result, "张三") || !strings.Contains(result, "周一") {
		t.Errorf("渲染结果 = %q, 期望包含 张三 和 周一", result)
	}
}

func TestRegistry_List(t *testing.T) {
	r := NewRegistry()
	r.MustRegister("a", NewTemplate("a"))
	r.MustRegister("b", NewTemplate("b"))
	r.MustRegister("c", NewTemplate("c"))

	names := r.List()
	if len(names) != 3 {
		t.Errorf("模板数量 = %d, 期望 3", len(names))
	}
}

func TestRegistry_Delete(t *testing.T) {
	r := NewRegistry()
	r.MustRegister("test", NewTemplate("test"))

	err := r.Delete("test")
	if err != nil {
		t.Fatalf("删除失败: %v", err)
	}

	if r.Has("test") {
		t.Error("删除后模板不应存在")
	}
}

func TestRegistry_Count(t *testing.T) {
	r := NewRegistry()
	if r.Count() != 0 {
		t.Errorf("初始数量应为 0, got %d", r.Count())
	}

	r.MustRegister("a", NewTemplate("a"))
	r.MustRegister("b", NewTemplate("b"))

	if r.Count() != 2 {
		t.Errorf("数量应为 2, got %d", r.Count())
	}
}

func TestRegistry_CloneOnGet(t *testing.T) {
	r := NewRegistry()
	r.MustRegister("test", NewTemplate("{{.x}}"))

	// 获取两次，各自注入不同变量
	t1, _ := r.Get("test")
	t2, _ := r.Get("test")

	t1.WithVar("x", "A")
	t2.WithVar("x", "B")

	r1, _ := t1.Render()
	r2, _ := t2.Render()

	if r1 == r2 {
		t.Errorf("克隆副本应独立: r1=%q, r2=%q", r1, r2)
	}
}

// ===== 消息模板测试 =====

func TestMessageTemplates_SystemUser(t *testing.T) {
	mt := NewMessageTemplates()
	mt.SetSystem("你是{{.role}}。")
	mt.SetUser("请帮我{{.task}}。")

	sys, err := mt.RenderSystem(map[string]any{"role": "翻译官"})
	if err != nil {
		t.Fatalf("渲染系统消息失败: %v", err)
	}
	if sys != "你是翻译官。" {
		t.Errorf("系统消息 = %q, 期望 %q", sys, "你是翻译官。")
	}

	user, err := mt.RenderUser(map[string]any{"task": "翻译这段文字"})
	if err != nil {
		t.Fatalf("渲染用户消息失败: %v", err)
	}
	if user != "请帮我翻译这段文字。" {
		t.Errorf("用户消息 = %q, 期望 %q", user, "请帮我翻译这段文字。")
	}
}

func TestMessageTemplates_RenderAll(t *testing.T) {
	mt := NewMessageTemplates()
	mt.SetSystem("你是{{.role}}。")
	mt.SetUser("{{.question}}")

	results, err := mt.RenderAll(map[string]any{
		"role":     "助手",
		"question": "你好",
	})
	if err != nil {
		t.Fatalf("渲染全部消息失败: %v", err)
	}

	if results[RoleSystem] != "你是助手。" {
		t.Errorf("系统消息 = %q", results[RoleSystem])
	}
	if results[RoleUser] != "你好" {
		t.Errorf("用户消息 = %q", results[RoleUser])
	}
}

// ===== 预定义模板测试 =====

func TestDefaultRegistry_AgentSystem(t *testing.T) {
	r := DefaultRegistry()

	result, err := r.Render("agent.system", map[string]any{
		"role":        "代码审查专家",
		"capabilities": []string{"代码审查", "安全检测", "性能优化"},
		"language":    "中文",
	})
	if err != nil {
		t.Fatalf("渲染 agent.system 失败: %v", err)
	}

	if !strings.Contains(result, "代码审查专家") {
		t.Error("结果应包含角色名")
	}
	if !strings.Contains(result, "代码审查") {
		t.Error("结果应包含能力列表")
	}
}

func TestDefaultRegistry_RAGSystem(t *testing.T) {
	r := DefaultRegistry()

	result, err := r.Render("rag.system", map[string]any{
		"context": "Go 是一种静态类型语言",
	})
	if err != nil {
		t.Fatalf("渲染 rag.system 失败: %v", err)
	}

	if !strings.Contains(result, "Go 是一种静态类型语言") {
		t.Error("结果应包含上下文内容")
	}
}

func TestDefaultRegistry_ToolSystem(t *testing.T) {
	r := DefaultRegistry()

	result, err := r.Render("tool.system", map[string]any{
		"tools": []map[string]string{
			{"name": "search", "description": "搜索网络"},
			{"name": "calc", "description": "计算器"},
		},
	})
	if err != nil {
		t.Fatalf("渲染 tool.system 失败: %v", err)
	}

	if !strings.Contains(result, "search") || !strings.Contains(result, "calc") {
		t.Error("结果应包含工具列表")
	}
}

func TestDefaultRegistry_TranslateUser(t *testing.T) {
	r := DefaultRegistry()

	result, err := r.Render("translate.user", map[string]any{
		"source_lang":  "英文",
		"target_lang":  "中文",
		"content":      "Hello, World!",
	})
	if err != nil {
		t.Fatalf("渲染 translate.user 失败: %v", err)
	}

	if !strings.Contains(result, "Hello, World!") {
		t.Error("结果应包含待翻译内容")
	}
}

package prompt

import (
	"fmt"
	"log/slog"
	"sync"
)

// Registry 模板注册表，管理命名模板和消息模板
type Registry struct {
	mu        sync.RWMutex
	templates map[string]*Template
}

// NewRegistry 创建模板注册表
func NewRegistry() *Registry {
	return &Registry{
		templates: make(map[string]*Template),
	}
}

// Register 注册命名模板
func (r *Registry) Register(name string, tmpl *Template) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.templates[name]; exists {
		return fmt.Errorf("template %q already exists", name)
	}
	r.templates[name] = tmpl
	return nil
}

// MustRegister 注册命名模板，已存在时 panic。
// 生产建议：仅在初始化阶段使用，运行时路径应调用 Register() 并处理 error。
func (r *Registry) MustRegister(name string, tmpl *Template) {
	if err := r.Register(name, tmpl); err != nil {
		slog.Error("prompt.MustRegister 失败", "name", name, "error", err)
		panic(err)
	}
}

// Get 获取命名模板
func (r *Registry) Get(name string) (*Template, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tmpl, ok := r.templates[name]
	if !ok {
		return nil, fmt.Errorf("template %q not found", name)
	}
	// 返回克隆副本，避免修改原始模板
	return tmpl.Clone(), nil
}

// Render 渲染命名模板
func (r *Registry) Render(name string, vars map[string]any) (string, error) {
	tmpl, err := r.Get(name)
	if err != nil {
		return "", err
	}
	if vars != nil {
		tmpl.WithVars(vars)
	}
	return tmpl.Render()
}

// List 列出所有已注册模板名称
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.templates))
	for name := range r.templates {
		names = append(names, name)
	}
	return names
}

// Delete 删除命名模板
func (r *Registry) Delete(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.templates[name]; !ok {
		return fmt.Errorf("template %q not found", name)
	}
	delete(r.templates, name)
	return nil
}

// Has 检查模板是否存在
func (r *Registry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.templates[name]
	return ok
}

// Count 返回已注册模板数量
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.templates)
}

// ===== 消息模板 =====

// MessageRole 消息角色
type MessageRole string

const (
	RoleSystem    MessageRole = "system"
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
)

// MessageTemplate 消息模板
type MessageTemplate struct {
	Role     MessageRole
	Template *Template
}

// MessageTemplates 消息模板集合（System + User + Assistant）
type MessageTemplates struct {
	registry *Registry
}

// NewMessageTemplates 创建消息模板集合
func NewMessageTemplates() *MessageTemplates {
	return &MessageTemplates{
		registry: NewRegistry(),
	}
}

// SetSystem 设置系统消息模板
func (mt *MessageTemplates) SetSystem(tmpl string) *MessageTemplates {
	mt.registry.MustRegister("__system__", NewTemplate(tmpl))
	return mt
}

// SetUser 设置用户消息模板
func (mt *MessageTemplates) SetUser(tmpl string) *MessageTemplates {
	mt.registry.MustRegister("__user__", NewTemplate(tmpl))
	return mt
}

// SetAssistant 设置助手消息模板
func (mt *MessageTemplates) SetAssistant(tmpl string) *MessageTemplates {
	mt.registry.MustRegister("__assistant__", NewTemplate(tmpl))
	return mt
}

// RenderSystem 渲染系统消息
func (mt *MessageTemplates) RenderSystem(vars map[string]any) (string, error) {
	return mt.registry.Render("__system__", vars)
}

// RenderUser 渲染用户消息
func (mt *MessageTemplates) RenderUser(vars map[string]any) (string, error) {
	return mt.registry.Render("__user__", vars)
}

// RenderAssistant 渲染助手消息
func (mt *MessageTemplates) RenderAssistant(vars map[string]any) (string, error) {
	return mt.registry.Render("__assistant__", vars)
}

// RenderAll 渲染所有消息模板
func (mt *MessageTemplates) RenderAll(vars map[string]any) (map[MessageRole]string, error) {
	result := make(map[MessageRole]string)

	if mt.registry.Has("__system__") {
		s, err := mt.RenderSystem(vars)
		if err != nil {
			return nil, fmt.Errorf("failed to render system message: %w", err)
		}
		result[RoleSystem] = s
	}

	if mt.registry.Has("__user__") {
		s, err := mt.RenderUser(vars)
		if err != nil {
			return nil, fmt.Errorf("failed to render user message: %w", err)
		}
		result[RoleUser] = s
	}

	if mt.registry.Has("__assistant__") {
		s, err := mt.RenderAssistant(vars)
		if err != nil {
			return nil, fmt.Errorf("failed to render assistant message: %w", err)
		}
		result[RoleAssistant] = s
	}

	return result, nil
}

// ===== 预定义模板 =====

// DefaultRegistry 返回包含预定义模板的注册表
func DefaultRegistry() *Registry {
	r := NewRegistry()

	// 通用 Agent 系统提示词
	r.MustRegister("agent.system", NewTemplate(
		`你是一个{{default "智能助手" .role}}。

{{if .capabilities}}你的核心能力：
{{range .capabilities}}- {{.}}
{{end}}{{end}}

{{if .constraints}}约束条件：
{{range .constraints}}- {{.}}
{{end}}{{end}}

请用{{default "中文" .language}}回答。`,
	))

	// RAG 检索增强模板
	r.MustRegister("rag.system", NewTemplate(
		`你是一个基于检索增强生成的助手。请根据以下参考信息回答用户问题。

参考信息：
{{.context}}

{{if .instructions}}附加指令：
{{.instructions}}{{end}}

要求：
- 优先使用参考信息中的内容回答
- 如果参考信息不足以回答，请明确说明
- 不要编造不存在的信息`,
	))

	// tool调用模板
	r.MustRegister("tool.system", NewTemplate(
		`你是一个可以使用tool的助手。当需要执行操作时，请调用合适的tool。

可用工具：
{{range .tools}}- {{.name}}: {{.description}}
{{end}}

使用规则：
1. 只使用上述列出的tool
2. 调用tool前先思考是否必要
3. 将tool返回的结果整合到回答中`,
	))

	// 代码生成模板
	r.MustRegister("code.system", NewTemplate(
		`你是一个专业的{{default "Go" .language}}编程助手。

要求：
- 生成高质量、可运行的代码
- 遵循{{default "Go" .language}}最佳实践
- 添加必要的注释和错误处理
- 如有依赖，说明安装方式`,
	))

	// 翻译模板
	r.MustRegister("translate.user", NewTemplate(
		`请将以下{{default "英文" .source_lang}}文本翻译为{{default "中文" .target_lang}}：

{{.content}}

翻译要求：
- 保持原文的语气和风格
- 专业术语请保留原文并在括号中注明
- 如有歧义，请提供多种翻译选项`,
	))

	return r
}

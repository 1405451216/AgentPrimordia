package tools

import (
	"encoding/json"
	"errors"
	"sync"
)

var (
	ErrInvalidConfig = errors.New("invalid configuration")
	ErrToolNotFound  = errors.New("tool not found")
	ErrToolExecution = errors.New("tool execution failed")
	ErrConfirmDenied = errors.New("tool confirmation denied")
)

// Registry manages tool registration and lookup
type Registry struct {
	tools       map[string]Tool
	permissions map[string]*Permission
	mu          sync.RWMutex
}

// NewRegistry creates an empty tool registry
func NewRegistry() *Registry {
	return &Registry{
		tools:       make(map[string]Tool),
		permissions: make(map[string]*Permission),
	}
}

// Register adds a tool to the registry
// 重复注册同名工具为幂等操作，直接覆盖
func (r *Registry) Register(tool Tool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := tool.Name()
	if name == "" {
		return ErrInvalidConfig
	}

	r.tools[name] = tool
	if _, exists := r.permissions[name]; !exists {
		r.permissions[name] = &Permission{}
	}
	return nil
}

// RegisterMultiple registers multiple tools at once
func (r *Registry) RegisterMultiple(tools ...Tool) error {
	for _, tool := range tools {
		if err := r.Register(tool); err != nil {
			return err
		}
	}
	return nil
}

// Get retrieves a tool by name
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tool, exists := r.tools[name]
	return tool, exists
}

// List returns all registered tool names
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}

// Count returns the number of registered tools
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tools)
}

// Definitions returns all tools formatted as LLM FunctionDefinitions
func (r *Registry) Definitions() []map[string]any {
	r.mu.RLock()
	defer r.mu.RUnlock()

	defs := make([]map[string]any, 0, len(r.tools))
	for _, tool := range r.tools {
		var params map[string]any
		if tool.Parameters() != nil {
			_ = json.Unmarshal(tool.Parameters(), &params)
		}
		def := map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        tool.Name(),
				"description": tool.Description(),
				"parameters":  params,
			},
		}
		defs = append(defs, def)
	}
	return defs
}

// SetPermission configures access control for a specific tool
func (r *Registry) SetPermission(name string, perm Permission) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tools[name]; !exists {
		return ErrToolNotFound
	}

	r.permissions[name] = &perm
	return nil
}

// GetPermission returns the permission settings for a tool
func (r *Registry) GetPermission(name string) (*Permission, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	perm, exists := r.permissions[name]
	return perm, exists
}

func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.tools, name)
	delete(r.permissions, name)
}

func (r *Registry) RegisterPlugin(plugin ToolPlugin) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := plugin.Init(nil); err != nil {
		return err
	}

	for _, tool := range plugin.Tools() {
		toolName := tool.Name()
		if toolName == "" {
			return ErrInvalidConfig
		}
		r.tools[toolName] = tool
		if _, exists := r.permissions[toolName]; !exists {
			r.permissions[toolName] = &Permission{}
		}
	}

	return nil
}

func (r *Registry) ToolsByCategory() map[string][]Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	categories := make(map[string][]Tool)
	for _, tool := range r.tools {
		cat := "default"
		if ct, ok := tool.(CategorizedTool); ok {
			cat = ct.Category()
		}
		categories[cat] = append(categories[cat], tool)
	}

	return categories
}

func (r *Registry) ToolCategories() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	seen := make(map[string]bool)
	var cats []string
	for _, tool := range r.tools {
		cat := "default"
		if ct, ok := tool.(CategorizedTool); ok {
			cat = ct.Category()
		}
		if !seen[cat] {
			seen[cat] = true
			cats = append(cats, cat)
		}
	}

	return cats
}

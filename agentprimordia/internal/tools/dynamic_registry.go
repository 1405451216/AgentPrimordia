package tools

import (
	"sync"
)

// ToolChangeEvent 动态工具变更事件
type ToolChangeEvent struct {
	Action ActionType `json:"action"`
	Tool   ToolInfo   `json:"tool"`
}

// ActionType 变更类型
type ActionType string

const (
	ActionRegister   ActionType = "register"
	ActionUnregister ActionType = "unregister"
)

// ToolInfo 工具元信息
type ToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category,omitempty"`
}

// ToolRegistry 基础嵌入接口
type ToolRegistry interface {
	Register(tool Tool) error
	Get(name string) (Tool, bool)
	List() []string
	Count() int
	Definitions() []map[string]any
}

// DynamicRegistry 动态工具注册接口
type DynamicRegistry interface {
	ToolRegistry
	Register(tool Tool) error
	Unregister(name string) error
	ListDynamic() []ToolInfo
	OnChange(handler func(event ToolChangeEvent))
}

// DynamicRegistryImpl 动态注册实现
type DynamicRegistryImpl struct {
	mu         sync.RWMutex
	registry   *Registry
	dynamic    map[string]Tool
	listeners  []func(event ToolChangeEvent)
	listenerMu sync.RWMutex
}

// NewDynamicRegistry 创建动态注册实例
func NewDynamicRegistry() *DynamicRegistryImpl {
	return &DynamicRegistryImpl{
		registry: NewRegistry(),
		dynamic:  make(map[string]Tool),
	}
}

// Register 注册工具（同时写入底层 Registry）
func (r *DynamicRegistryImpl) Register(tool Tool) error {
	name := tool.Name()
	if name == "" {
		return ErrInvalidConfig
	}
	r.mu.Lock()
	r.dynamic[name] = tool
	r.mu.Unlock()

	// 同步到底层 Registry
	if err := r.registry.Register(tool); err != nil {
		r.mu.Lock()
		delete(r.dynamic, name)
		r.mu.Unlock()
		return err
	}

	r.notify(ToolChangeEvent{Action: ActionRegister, Tool: toolInfo(tool)})
	return nil
}

// Unregister 注销工具
func (r *DynamicRegistryImpl) Unregister(name string) error {
	r.mu.Lock()
	tool, exists := r.dynamic[name]
	if exists {
		delete(r.dynamic, name)
	}
	r.mu.Unlock()

	if exists {
		r.registry.Unregister(name)
		r.notify(ToolChangeEvent{Action: ActionUnregister, Tool: toolInfo(tool)})
		return nil
	}
	return ErrToolNotFound
}

// ListDynamic 列出动态注册的工具
func (r *DynamicRegistryImpl) ListDynamic() []ToolInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	infos := make([]ToolInfo, 0, len(r.dynamic))
	for _, tool := range r.dynamic {
		infos = append(infos, toolInfo(tool))
	}
	return infos
}

// OnChange 注册变更监听器
func (r *DynamicRegistryImpl) OnChange(handler func(event ToolChangeEvent)) {
	r.listenerMu.Lock()
	defer r.listenerMu.Unlock()
	r.listeners = append(r.listeners, handler)
}

// notify 通知所有监听器
func (r *DynamicRegistryImpl) notify(event ToolChangeEvent) {
	r.listenerMu.RLock()
	handlers := make([]func(event ToolChangeEvent), len(r.listeners))
	copy(handlers, r.listeners)
	r.listenerMu.RUnlock()
	for _, h := range handlers {
		h(event)
	}
}

// Get 从底层 Registry 获取工具
func (r *DynamicRegistryImpl) Get(name string) (Tool, bool) {
	return r.registry.Get(name)
}

// List 从底层 Registry 列出所有工具
func (r *DynamicRegistryImpl) List() []string {
	return r.registry.List()
}

// Count 从底层 Registry 获取工具数量
func (r *DynamicRegistryImpl) Count() int {
	return r.registry.Count()
}

// Definitions 从底层 Registry 获取 FunctionDefinitions
func (r *DynamicRegistryImpl) Definitions() []map[string]any {
	return r.registry.Definitions()
}

// toolInfo 提取工具元信息
func toolInfo(tool Tool) ToolInfo {
	info := ToolInfo{
		Name:        tool.Name(),
		Description: tool.Description(),
	}
	if ct, ok := tool.(CategorizedTool); ok {
		info.Category = ct.Category()
	}
	return info
}

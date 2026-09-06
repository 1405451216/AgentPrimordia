// catalog.go — 工具目录（并发安全注册表）
package reuse

import "sync"

// ToolEntry 工具目录条目
type ToolEntry struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Domain      string `json:"domain,omitempty"`
}

// ToolCatalog 工具目录（并发安全）
type ToolCatalog struct {
	mu    sync.RWMutex
	tools map[string]ToolEntry
}

// NewToolCatalog 创建工具目录
func NewToolCatalog() *ToolCatalog {
	return &ToolCatalog{tools: make(map[string]ToolEntry)}
}

// Register 注册工具
func (c *ToolCatalog) Register(entry ToolEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tools[entry.ID] = entry
}

// GetByID 按 ID 获取工具
func (c *ToolCatalog) GetByID(id string) (ToolEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.tools[id]
	return e, ok
}

// List 列出所有工具
func (c *ToolCatalog) List() []ToolEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]ToolEntry, 0, len(c.tools))
	for _, e := range c.tools {
		result = append(result, e)
	}
	return result
}

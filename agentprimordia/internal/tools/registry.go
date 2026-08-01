package tools

import (
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
)

var (
	ErrInvalidConfig = errors.New("invalid configuration")
	ErrToolNotFound  = errors.New("tool not found")
	ErrToolExecution = errors.New("tool execution failed")
	ErrConfirmDenied = errors.New("tool confirmation denied")
)

// Registry 管理tool注册和查找
// 优化（Task 9）：
//   - 使用 sync.Map 替代 map+RWMutex，适配读多写少场景
//   - Definitions() 缓存"原始 tool def 列表"，每次返回前再深克隆。
//     这样既避免每次都解析 buildToolDef，也保证调用方修改不会相互影响。
//   - extractPathFromArgs 使用 json.Decoder 提前退出
type Registry struct {
	tools       sync.Map // map[string]Tool
	toolDefs    sync.Map // map[string]map[string]any（每个tool的原始 def 模板）
	permissions sync.Map // map[string]*Permission

	// 优化（perf-v3）：原子计数器，避免 Count() 每次 Range 遍历
	count atomic.Int64

	// Definitions() 缓存：避免每次都遍历 sync.Map + cloneToolDef 每个元素
	// 缓存内容：原始 def 列表（共享）；每次调用返回前再 cloneToolDef 一遍以保证隔离。
	defsCache    atomic.Pointer[[]map[string]any]
	defsValid    atomic.Bool
	defsCacheLen atomic.Int64
	defsMu       sync.Mutex // 保证缓存只被一个 goroutine 重建
}

// NewRegistry 创建一个空的tool注册表
func NewRegistry() *Registry {
	return &Registry{}
}

// Register 向注册表添加tool
// 重复注册同名tool为幂等操作，直接覆盖
func (r *Registry) Register(tool Tool) error {
	name := tool.Name()
	if name == "" {
		return ErrInvalidConfig
	}

	// 优化（perf-v3）：仅在首次注册时递增计数器（覆盖注册不增加数量）
	if _, exists := r.tools.Load(name); !exists {
		r.count.Add(1)
	}
	r.tools.Store(name, tool)
	r.toolDefs.Store(name, buildToolDef(tool))
	if _, exists := r.permissions.Load(name); !exists {
		r.permissions.Store(name, &Permission{})
	}
	// 失效 Definitions 缓存
	r.invalidateDefsCache()
	return nil
}

// buildToolDef 将 Tool 转换为 LLM FunctionDefinition 映射
func buildToolDef(tool Tool) map[string]any {
	var params map[string]any
	if tool.Parameters() != nil {
		if err := json.Unmarshal(tool.Parameters(), &params); err != nil {
			// tool参数 schema 解析失败，使用空参数继续
			params = nil
		}
	}
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        tool.Name(),
			"description": tool.Description(),
			"parameters":  params,
		},
	}
}

// cloneToolDef 深拷贝tool定义，防止调用者修改缓存
func cloneToolDef(def map[string]any) map[string]any {
	clone := make(map[string]any, len(def))
	for k, v := range def {
		if inner, ok := v.(map[string]any); ok {
			innerClone := make(map[string]any, len(inner))
			for ik, iv := range inner {
				if params, ok := iv.(map[string]any); ok && params != nil {
					paramsClone := make(map[string]any, len(params))
					for pk, pv := range params {
						paramsClone[pk] = pv
					}
					innerClone[ik] = paramsClone
				} else {
					innerClone[ik] = iv
				}
			}
			clone[k] = innerClone
		} else {
			clone[k] = v
		}
	}
	return clone
}

// invalidateDefsCache 失效 Definitions() 缓存。Register/Unregister 时调用。
func (r *Registry) invalidateDefsCache() {
	r.defsValid.Store(false)
	r.defsCache.Store(nil)
	r.defsCacheLen.Store(0)
}

// RegisterMultiple 一次注册多个tool
func (r *Registry) RegisterMultiple(tools ...Tool) error {
	for _, tool := range tools {
		if err := r.Register(tool); err != nil {
			return err
		}
	}
	return nil
}

// Get 按名称获取tool
func (r *Registry) Get(name string) (Tool, bool) {
	v, exists := r.tools.Load(name)
	if !exists {
		return nil, false
	}
	tool, ok := v.(Tool)
	if !ok {
		return nil, false
	}
	return tool, true
}

// List 返回所有已注册的tool名称
func (r *Registry) List() []string {
	names := make([]string, 0)
	r.tools.Range(func(key, _ any) bool {
		name, ok := key.(string)
		if !ok {
			return true
		}
		names = append(names, name)
		return true
	})
	return names
}

// Count 返回已注册tool的数量
// 优化（perf-v3）：使用原子计数器，O(1) 复杂度
func (r *Registry) Count() int {
	return int(r.count.Load())
}

// Definitions 返回所有tool的 LLM FunctionDefinitions。
// 优化（Task 9）：缓存"原始 def 列表"，每次返回时再 cloneToolDef。
// 这样既避免每次都遍历 sync.Map 重建完整列表，又保证每次返回的是独立的深拷贝，
// 调用方修改不会相互污染。
func (r *Registry) Definitions() []map[string]any {
	// 快路径：缓存命中且长度未变
	if !r.defsValid.Load() {
		// 双检锁：只有一个 goroutine 重建缓存，其他等待后读取。
		r.defsMu.Lock()
		if !r.defsValid.Load() {
			r.rebuildDefsCache()
		}
		r.defsMu.Unlock()
	}
	cached := r.defsCache.Load()
	if cached == nil {
		// 极端情况：重建失败，返回空切片
		return []map[string]any{}
	}
	// 每次返回独立深拷贝（与测试 TestRegistry_Definitions_CacheIsolation 兼容）
	out := make([]map[string]any, 0, len(*cached))
	for _, def := range *cached {
		out = append(out, cloneToolDef(def))
	}
	return out
}

// rebuildDefsCache 重建缓存的原始 def 列表
func (r *Registry) rebuildDefsCache() {
	cached := make([]map[string]any, 0)
	r.toolDefs.Range(func(_, v any) bool {
		def, ok := v.(map[string]any)
		if !ok {
			return true
		}
		cached = append(cached, def)
		return true
	})
	r.defsCache.Store(&cached)
	r.defsCacheLen.Store(int64(len(cached)))
	r.defsValid.Store(true)
}

// SetPermission 为指定tool配置访问控制
func (r *Registry) SetPermission(name string, perm Permission) error {
	if _, exists := r.tools.Load(name); !exists {
		return ErrToolNotFound
	}
	r.permissions.Store(name, &perm)
	return nil
}

// GetPermission 返回tool的权限设置
func (r *Registry) GetPermission(name string) (*Permission, bool) {
	v, exists := r.permissions.Load(name)
	if !exists {
		return nil, false
	}
	perm, ok := v.(*Permission)
	if !ok {
		return nil, false
	}
	return perm, true
}

func (r *Registry) Unregister(name string) {
	// 优化（perf-v3）：仅在tool存在时递减计数器
	if _, exists := r.tools.LoadAndDelete(name); exists {
		r.count.Add(-1)
	} else {
		r.tools.Delete(name)
	}
	r.toolDefs.Delete(name)
	r.permissions.Delete(name)
	r.invalidateDefsCache()
}

func (r *Registry) RegisterPlugin(plugin ToolPlugin) error {
	if err := plugin.Init(nil); err != nil {
		return err
	}

	for _, tool := range plugin.Tools() {
		toolName := tool.Name()
		if toolName == "" {
			return ErrInvalidConfig
		}
		// 优化（perf-v3）：仅在首次注册时递增计数器
		if _, exists := r.tools.Load(toolName); !exists {
			r.count.Add(1)
		}
		r.tools.Store(toolName, tool)
		r.toolDefs.Store(toolName, buildToolDef(tool))
		if _, exists := r.permissions.Load(toolName); !exists {
			r.permissions.Store(toolName, &Permission{})
		}
	}
	r.invalidateDefsCache()
	return nil
}

func (r *Registry) ToolsByCategory() map[string][]Tool {
	categories := make(map[string][]Tool)
	r.tools.Range(func(_, v any) bool {
		tool, ok := v.(Tool)
		if !ok {
			return true
		}
		cat := "default"
		if ct, ok := tool.(CategorizedTool); ok {
			cat = ct.Category()
		}
		categories[cat] = append(categories[cat], tool)
		return true
	})
	return categories
}

func (r *Registry) ToolCategories() []string {
	seen := make(map[string]bool)
	var cats []string
	r.tools.Range(func(_, v any) bool {
		tool, ok := v.(Tool)
		if !ok {
			return true
		}
		cat := "default"
		if ct, ok := tool.(CategorizedTool); ok {
			cat = ct.Category()
		}
		if !seen[cat] {
			seen[cat] = true
			cats = append(cats, cat)
		}
		return true
	})
	return cats
}

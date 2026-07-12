package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ResourceHandler reads a resource's content given its URI.
type ResourceHandler func(ctx context.Context, uri string) ([]byte, error)

// ResourceSubscriber receives resource data notifications.
type ResourceSubscriber func(ctx context.Context, uri string, data []byte) error

// resourceEntry is an internal registered-resource record.
type resourceEntry struct {
	uri      string
	name     string
	mimeType string
	handler  ResourceHandler
}

// resourceRegistry manages MCP resources exposed by the Agent.
type resourceRegistry struct {
	mu        sync.RWMutex
	resources map[string]*resourceEntry
}

// newResourceRegistry creates an empty resource registry.
func newResourceRegistry() *resourceRegistry {
	return &resourceRegistry{
		resources: make(map[string]*resourceEntry),
	}
}

// Register adds or replaces a resource entry keyed by its URI.
func (r *resourceRegistry) Register(uri, name, mimeType string, handler ResourceHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resources[uri] = &resourceEntry{
		uri:      uri,
		name:     name,
		mimeType: mimeType,
		handler:  handler,
	}
}

// Unregister removes a resource by URI.
func (r *resourceRegistry) Unregister(uri string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.resources, uri)
}

// List returns all registered resources in MCP wire format.
func (r *resourceRegistry) List() []Resource {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Resource, 0, len(r.resources))
	for _, entry := range r.resources {
		result = append(result, Resource{
			URI:      entry.uri,
			Name:     entry.name,
			MimeType: entry.mimeType,
		})
	}
	return result
}

// Read invokes the handler for a given URI and returns the resource content.
func (r *resourceRegistry) Read(ctx context.Context, uri string) (*ResourceContent, error) {
	r.mu.RLock()
	entry, ok := r.resources[uri]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("资源 URI 未找到: %s", uri)
	}
	data, err := entry.handler(ctx, uri)
	if err != nil {
		return nil, fmt.Errorf("读取资源 %q 失败: %w", uri, err)
	}
	return &ResourceContent{
		URI:      uri,
		MimeType: entry.mimeType,
		Text:     string(data),
	}, nil
}

// Count returns the current number of registered resources.
func (r *resourceRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.resources)
}

// ===== Built-in resource implementations =====

// BuiltinResources provides default Agent resources that can be registered
// with a running MCPServer.
type BuiltinResources struct {
	getMemorySnapshot func(ctx context.Context) ([]byte, error)
	getSession        func(ctx context.Context, sessionID string) ([]byte, error)
	getStatus         func(ctx context.Context) ([]byte, error)
}

// NewBuiltinResources constructs a BuiltinResources using simple callbacks.
func NewBuiltinResources(
	getMemory func(ctx context.Context) ([]byte, error),
	getSession func(ctx context.Context, sessionID string) ([]byte, error),
	getStatus func(ctx context.Context) ([]byte, error),
) *BuiltinResources {
	return &BuiltinResources{
		getMemorySnapshot: getMemory,
		getSession:        getSession,
		getStatus:         getStatus,
	}
}

// DefaultBuiltinResources returns a BuiltinResources that reports static
// placeholder content.
func DefaultBuiltinResources() *BuiltinResources {
	return &BuiltinResources{
		getMemorySnapshot: func(ctx context.Context) ([]byte, error) {
			return json.Marshal(map[string]any{
				"type":      "memory_snapshot",
				"timestamp": time.Now().Unix(),
				"summary":   "默认记忆快照（未连接 Agent 内存后端）",
			})
		},
		getSession: func(ctx context.Context, sessionID string) ([]byte, error) {
			return json.Marshal(map[string]any{
				"type":       "session_state",
				"session_id": sessionID,
				"state":      "unknown",
				"summary":    fmt.Sprintf("会话 %s 未连接实时后端", sessionID),
			})
		},
		getStatus: func(ctx context.Context) ([]byte, error) {
			return json.Marshal(map[string]any{
				"type":      "agent_status",
				"status":    "running",
				"timestamp": time.Now().Unix(),
			})
		},
	}
}

// sessionResourceHandler dispatches URIs of the form agent://session/{id}.
func sessionResourceHandler(getSession func(ctx context.Context, sid string) ([]byte, error)) ResourceHandler {
	return func(ctx context.Context, uri string) ([]byte, error) {
		const prefix = "agent://session/"
		if !strings.HasPrefix(uri, prefix) {
			return nil, fmt.Errorf("资源 URI 格式错误: %s", uri)
		}
		sid := strings.TrimPrefix(uri, prefix)
		if sid == "" {
			return nil, fmt.Errorf("缺少 session id")
		}
		return getSession(ctx, sid)
	}
}

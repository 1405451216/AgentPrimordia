package mesh

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

type InMemoryRegistry struct {
	mu       sync.RWMutex
	agents   map[string]*AgentInfo
	ttl      time.Duration
	capIndex map[string]map[string]struct{} // capability -> set of agent IDs
}

func NewInMemoryRegistry() *InMemoryRegistry {
	return NewInMemoryRegistryWithTTL(30 * time.Second)
}

func NewInMemoryRegistryWithTTL(ttl time.Duration) *InMemoryRegistry {
	return &InMemoryRegistry{
		agents:   make(map[string]*AgentInfo),
		ttl:      ttl,
		capIndex: make(map[string]map[string]struct{}),
	}
}

func (r *InMemoryRegistry) Register(agent *AgentInfo) error {
	if agent == nil || agent.ID == "" {
		return ErrInvalidAgentID
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.agents[agent.ID]; exists {
		return fmt.Errorf("%w: %s", ErrAgentAlreadyExists, agent.ID)
	}
	cp := agent.Copy()
	cp.LastHeartbeat = time.Now()
	if cp.Status == "" {
		cp.Status = AgentStatusHealthy
	}
	r.agents[agent.ID] = cp
	r.indexCapabilitiesLocked(cp)
	return nil
}

func (r *InMemoryRegistry) Deregister(agentID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	agent, ok := r.agents[agentID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrAgentNotFound, agentID)
	}
	r.unindexCapabilitiesLocked(agent)
	delete(r.agents, agentID)
	return nil
}

func (r *InMemoryRegistry) Discover(capability string) ([]*AgentInfo, error) {
	if strings.TrimSpace(capability) == "" {
		return r.listHealthy(), nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	now := time.Now()
	ids, ok := r.capIndex[capability]
	if !ok {
		return nil, nil
	}
	result := make([]*AgentInfo, 0, len(ids))
	for id := range ids {
		agent := r.agents[id]
		if agent == nil {
			continue
		}
		if !r.isHealthyLocked(agent, now) {
			continue
		}
		result = append(result, agent.Copy())
	}
	return result, nil
}

func (r *InMemoryRegistry) Heartbeat(agentID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	agent, ok := r.agents[agentID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrAgentNotFound, agentID)
	}
	agent.LastHeartbeat = time.Now()
	agent.Status = AgentStatusHealthy
	return nil
}

func (r *InMemoryRegistry) Get(agentID string) (*AgentInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	agent, ok := r.agents[agentID]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrAgentNotFound, agentID)
	}
	return agent.Copy(), nil
}

func (r *InMemoryRegistry) List() []*AgentInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*AgentInfo, 0, len(r.agents))
	for _, agent := range r.agents {
		result = append(result, agent.Copy())
	}
	return result
}

func (r *InMemoryRegistry) ListHealthy() []*AgentInfo {
	return r.listHealthy()
}

func (r *InMemoryRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.agents)
}

func (r *InMemoryRegistry) indexCapabilitiesLocked(agent *AgentInfo) {
	for _, cap := range agent.Capabilities {
		if r.capIndex[cap] == nil {
			r.capIndex[cap] = make(map[string]struct{})
		}
		r.capIndex[cap][agent.ID] = struct{}{}
	}
}

func (r *InMemoryRegistry) unindexCapabilitiesLocked(agent *AgentInfo) {
	for _, cap := range agent.Capabilities {
		if ids, ok := r.capIndex[cap]; ok {
			delete(ids, agent.ID)
			if len(ids) == 0 {
				delete(r.capIndex, cap)
			}
		}
	}
}

func (r *InMemoryRegistry) isHealthyLocked(agent *AgentInfo, now time.Time) bool {
	return agent.Status != AgentStatusUnhealthy && now.Sub(agent.LastHeartbeat) <= r.ttl
}

func (r *InMemoryRegistry) listHealthy() []*AgentInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	now := time.Now()
	result := make([]*AgentInfo, 0, len(r.agents))
	for _, agent := range r.agents {
		if r.isHealthyLocked(agent, now) {
			result = append(result, agent.Copy())
		}
	}
	return result
}
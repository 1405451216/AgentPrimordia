package a2a

import (
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// AgentRegistry Agent 注册信息
type AgentRegistry struct {
	Card      *AgentCard
	Endpoints AgentEndpoints
	SeenAt    time.Time
}

// Discovery Discovery 接口
type Discovery interface {
	Register(card *AgentCard) error
	Deregister(agentID string) error
	Resolve(agentID string) (*AgentRegistry, error)
	List() []*AgentRegistry
	Watch() <-chan DiscoveryEvent
}

// DiscoveryEvent 发现事件
type DiscoveryEvent struct {
	Type     DiscoveryEventType `json:"type"`
	AgentID  string             `json:"agent_id"`
	Card     *AgentCard         `json:"card,omitempty"`
}

// DiscoveryEventType 发现事件类型
type DiscoveryEventType string

const (
	EventAgentRegistered   DiscoveryEventType = "registered"
	EventAgentDeregistered DiscoveryEventType = "deregistered"
	EventAgentUpdated      DiscoveryEventType = "updated"
)

// LocalDiscovery 本地内存发现服务
type LocalDiscovery struct {
	agents  map[string]*AgentRegistry
	watchers []chan DiscoveryEvent
	mu      sync.RWMutex
	logger  *slog.Logger
}

func NewLocalDiscovery() *LocalDiscovery {
	return &LocalDiscovery{
		agents: make(map[string]*AgentRegistry),
		logger: slog.Default(),
	}
}

func (d *LocalDiscovery) Register(card *AgentCard) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	eventType := EventAgentRegistered
	if _, exists := d.agents[card.AgentID]; exists {
		eventType = EventAgentUpdated
	}

	d.agents[card.AgentID] = &AgentRegistry{
		Card:      card,
		Endpoints: card.Endpoints,
		SeenAt:    time.Now(),
	}

	d.notifyLocked(DiscoveryEvent{
		Type:    eventType,
		AgentID: card.AgentID,
		Card:    card,
	})
	return nil
}

func (d *LocalDiscovery) Deregister(agentID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, exists := d.agents[agentID]; !exists {
		return fmt.Errorf("Agent 未注册: %s", agentID)
	}

	delete(d.agents, agentID)
	d.notifyLocked(DiscoveryEvent{
		Type:    EventAgentDeregistered,
		AgentID: agentID,
	})
	return nil
}

func (d *LocalDiscovery) Resolve(agentID string) (*AgentRegistry, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	reg, exists := d.agents[agentID]
	if !exists {
		return nil, fmt.Errorf("Agent 未找到: %s", agentID)
	}
	return reg, nil
}

func (d *LocalDiscovery) List() []*AgentRegistry {
	d.mu.RLock()
	defer d.mu.RUnlock()

	result := make([]*AgentRegistry, 0, len(d.agents))
	for _, reg := range d.agents {
		result = append(result, reg)
	}
	return result
}

func (d *LocalDiscovery) Watch() <-chan DiscoveryEvent {
	d.mu.Lock()
	defer d.mu.Unlock()

	ch := make(chan DiscoveryEvent, 16)
	d.watchers = append(d.watchers, ch)
	return ch
}

func (d *LocalDiscovery) notifyLocked(event DiscoveryEvent) {
	for _, ch := range d.watchers {
		select {
		case ch <- event:
		default:
			d.logger.Warn("Discovery 事件通道满，丢弃事件", "agent_id", event.AgentID)
		}
	}
}

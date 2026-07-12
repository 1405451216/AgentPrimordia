package mesh

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"
)

// AgentMesh is the top-level mesh entry.
type AgentMesh struct {
	Registry      Registry
	Router        Router
	Balancer      LoadBalancer
	HealthChecker *HealthChecker
	logger        *slog.Logger
	counter       atomic.Uint64
}

// NewAgentMesh constructs an AgentMesh with an in-memory registry.
func NewAgentMesh(balancer LoadBalancer, ttl time.Duration) *AgentMesh {
	reg := NewInMemoryRegistry()
	hc := NewHealthChecker(reg, ttl, slog.Default())
	hc.Start()
	return &AgentMesh{
		Registry:      reg,
		Balancer:      balancer,
		Router:        NewSmartRouter(reg, balancer),
		HealthChecker: hc,
		logger:        slog.Default(),
	}
}

func (m *AgentMesh) Stop() {
	if m.HealthChecker != nil {
		m.HealthChecker.Stop()
	}
}

func (m *AgentMesh) RegisterAgent(info *AgentInfo) error {
	return m.Registry.Register(info)
}

func (m *AgentMesh) DeregisterAgent(agentID string) error {
	return m.Registry.Deregister(agentID)
}

func (m *AgentMesh) RouteToAgent(ctx context.Context, capability string, opts RouteOpts) (*AgentInfo, error) {
	return m.Router.Route(ctx, capability, opts)
}
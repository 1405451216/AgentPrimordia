package mesh

import (
	"context"
	"fmt"
	"time"
)

// RouteOpts augments routing.
type RouteOpts struct {
	RequestKey     string
	MaxLoad        int32
	Cluster        string
	RequireHealthy bool
}

type Router interface {
	Route(ctx context.Context, capability string, opts RouteOpts) (*AgentInfo, error)
}

type SmartRouter struct {
	registry Registry
	balancer LoadBalancer
	ttl      time.Duration
}

func NewSmartRouter(registry Registry, balancer LoadBalancer) *SmartRouter {
	return &SmartRouter{registry: registry, balancer: balancer, ttl: 30 * time.Second}
}

func (r *SmartRouter) Route(ctx context.Context, capability string, opts RouteOpts) (*AgentInfo, error) {
	candidates, err := r.registry.Discover(capability)
	if err != nil {
		return nil, err
	}
	if opts.Cluster != "" {
		candidates = filterByCluster(candidates, opts.Cluster)
	}
	if opts.MaxLoad > 0 {
		candidates = filterByMaxLoad(candidates, opts.MaxLoad)
	}
	if opts.RequireHealthy {
		candidates = filterHealthy(candidates, r.ttl)
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("mesh: no agents found for capability %q", capability)
	}
	if balancer, ok := r.balancer.(*ConsistentHashBalancer); ok && opts.RequestKey != "" {
		return balancer.SelectByKey(candidates, opts.RequestKey)
	}
	return r.balancer.Select(candidates)
}

func filterByCluster(candidates []*AgentInfo, cluster string) []*AgentInfo {
	result := make([]*AgentInfo, 0, len(candidates))
	for _, c := range candidates {
		if c.Cluster == cluster {
			result = append(result, c)
		}
	}
	return result
}

func filterByMaxLoad(candidates []*AgentInfo, max int32) []*AgentInfo {
	result := make([]*AgentInfo, 0, len(candidates))
	for _, c := range candidates {
		if c.Load <= max {
			result = append(result, c)
		}
	}
	return result
}

func filterHealthy(candidates []*AgentInfo, ttl time.Duration) []*AgentInfo {
	result := make([]*AgentInfo, 0, len(candidates))
	now := time.Now()
	for _, c := range candidates {
		if c.Status == AgentStatusHealthy && now.Sub(c.LastHeartbeat) <= ttl {
			result = append(result, c)
		}
	}
	return result
}
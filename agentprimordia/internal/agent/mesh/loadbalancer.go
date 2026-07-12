package mesh

import (
	"errors"
	"math/rand"
)

type LoadBalancer interface {
	Select(candidates []*AgentInfo) (*AgentInfo, error)
}

var ErrNoCandidates = errors.New("mesh: no candidates available")

type RoundRobinBalancer struct {
	index uint64
}

func NewRoundRobinBalancer() *RoundRobinBalancer {
	return &RoundRobinBalancer{}
}

func (b *RoundRobinBalancer) Select(candidates []*AgentInfo) (*AgentInfo, error) {
	if len(candidates) == 0 {
		return nil, ErrNoCandidates
	}
	idx := b.index % uint64(len(candidates))
	b.index++
	return candidates[idx].Copy(), nil
}

type LeastLoadBalancer struct{}

func NewLeastLoadBalancer() *LeastLoadBalancer {
	return &LeastLoadBalancer{}
}

func (b *LeastLoadBalancer) Select(candidates []*AgentInfo) (*AgentInfo, error) {
	if len(candidates) == 0 {
		return nil, ErrNoCandidates
	}
	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.Load < best.Load {
			best = c
		}
	}
	return best.Copy(), nil
}

type WeightedRandomBalancer struct{}

func NewWeightedRandomBalancer() *WeightedRandomBalancer {
	return &WeightedRandomBalancer{}
}

func (b *WeightedRandomBalancer) Select(candidates []*AgentInfo) (*AgentInfo, error) {
	if len(candidates) == 0 {
		return nil, ErrNoCandidates
	}
	total := 0
	weights := make([]int, len(candidates))
	for i, c := range candidates {
		w := 100 - int(c.Load)
		if w < 1 {
			w = 1
		}
		weights[i] = w
		total += w
	}
	r := rand.Intn(total)
	for i, w := range weights {
		r -= w
		if r < 0 {
			return candidates[i].Copy(), nil
		}
	}
	return candidates[len(candidates)-1].Copy(), nil
}

type ConsistentHashBalancer struct{}

func NewConsistentHashBalancer() *ConsistentHashBalancer {
	return &ConsistentHashBalancer{}
}

func (b *ConsistentHashBalancer) Select(candidates []*AgentInfo) (*AgentInfo, error) {
	if len(candidates) == 0 {
		return nil, ErrNoCandidates
	}
	idx := rand.Intn(len(candidates))
	return candidates[idx].Copy(), nil
}

func (b *ConsistentHashBalancer) SelectByKey(candidates []*AgentInfo, key string) (*AgentInfo, error) {
	if len(candidates) == 0 {
		return nil, ErrNoCandidates
	}
	if key == "" {
		return b.Select(candidates)
	}
	h := fnv32(key) % uint32(len(candidates))
	return candidates[h].Copy(), nil
}

func fnv32(s string) uint32 {
	const (
		offset32 = 2166136261
		prime32  = 16777619
	)
	h := uint32(offset32)
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= prime32
	}
	return h
}
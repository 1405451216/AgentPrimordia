package mesh

import (
	"errors"
	"time"
)

type AgentStatus string

const (
	AgentStatusHealthy   AgentStatus = "healthy"
	AgentStatusUnhealthy AgentStatus = "unhealthy"
	AgentStatusUnknown   AgentStatus = "unknown"
)

type AgentInfo struct {
	ID            string
	Cluster       string
	Address       string
	Capabilities  []string
	Status        AgentStatus
	Load          int32
	LastHeartbeat time.Time
	Metadata      map[string]string
}

func (ai *AgentInfo) Copy() *AgentInfo {
	if ai == nil {
		return nil
	}
	cp := *ai
	if ai.Capabilities != nil {
		cp.Capabilities = append([]string(nil), ai.Capabilities...)
	}
	if ai.Metadata != nil {
		cp.Metadata = make(map[string]string, len(ai.Metadata))
		for k, v := range ai.Metadata {
			cp.Metadata[k] = v
		}
	}
	return &cp
}

type Registry interface {
	Register(agent *AgentInfo) error
	Deregister(agentID string) error
	Discover(capability string) ([]*AgentInfo, error)
	Heartbeat(agentID string) error
}

var (
	ErrAgentNotFound      = errors.New("mesh: agent not found")
	ErrAgentAlreadyExists = errors.New("mesh: agent already registered")
	ErrInvalidAgentID     = errors.New("mesh: invalid agent ID")
)

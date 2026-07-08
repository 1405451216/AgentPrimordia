package autoscaler

import (
	"testing"
	"time"
)

func TestLLMMetricCollector_Update(t *testing.T) {
	c := NewLLMMetricCollector()

	m := &LLMMetrics{
		PodName:         "agent-1",
		QueueDepth:      10,
		AvgLatencyMs:    3000,
		ActiveTasks:     3,
		TokenRatePerMin: 50000,
	}

	c.Update(m)

	all := c.GetAll()
	if len(all) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(all))
	}
	if all["agent-1"].QueueDepth != 10 {
		t.Errorf("expected QueueDepth 10, got %d", all["agent-1"].QueueDepth)
	}
}

func TestLLMMetricCollector_Aggregate(t *testing.T) {
	c := NewLLMMetricCollector()
	now := time.Now()

	c.Update(&LLMMetrics{
		PodName: "agent-1", QueueDepth: 5, ActiveTasks: 1,
		AvgLatencyMs: 2000, TokenRatePerMin: 10000, PriorityWeightedQ: 5, LastUpdated: now,
	})
	c.Update(&LLMMetrics{
		PodName: "agent-2", QueueDepth: 15, ActiveTasks: 3,
		AvgLatencyMs: 4000, TokenRatePerMin: 30000, PriorityWeightedQ: 15, LastUpdated: now,
	})

	a := c.Aggregate()

	if a.PodCount != 2 {
		t.Errorf("expected PodCount 2, got %d", a.PodCount)
	}
	if a.TotalQueueDepth != 20 {
		t.Errorf("expected TotalQueueDepth 20, got %d", a.TotalQueueDepth)
	}
	if a.TotalActiveTasks != 4 {
		t.Errorf("expected TotalActiveTasks 4, got %d", a.TotalActiveTasks)
	}
}

func TestDesiredReplicas_QueueBasedScaling(t *testing.T) {
	cfg := ScalingConfig{
		MinReplicas:            1,
		MaxReplicas:            10,
		TargetQueueDepthPerPod: 5,
	}

	a := AggregatedLLMMetrics{PodCount: 2, TotalQueueDepth: 20}
	desired := a.DesiredReplicas(cfg)

	if desired != 4 {
		t.Errorf("expected 4 replicas, got %d", desired)
	}
}

func TestDesiredReplicas_MaxBoundary(t *testing.T) {
	cfg := ScalingConfig{
		MinReplicas:            1,
		MaxReplicas:            5,
		TargetQueueDepthPerPod: 5,
	}

	a := AggregatedLLMMetrics{PodCount: 10, TotalQueueDepth: 100}
	desired := a.DesiredReplicas(cfg)

	if desired != 5 {
		t.Errorf("expected 5 replicas, got %d", desired)
	}
}

func TestDesiredReplicas_LatencyBasedScaling(t *testing.T) {
	cfg := ScalingConfig{
		MinReplicas:            1,
		MaxReplicas:            10,
		TargetQueueDepthPerPod: 5,
		TargetLatencyMs:        2000,
	}

	a := AggregatedLLMMetrics{PodCount: 2, AvgLatencyMs: 4000}
	desired := a.DesiredReplicas(cfg)

	if desired != 4 {
		t.Errorf("expected 4 replicas, got %d", desired)
	}
}

func TestDesiredReplicas_MinBoundary(t *testing.T) {
	cfg := ScalingConfig{
		MinReplicas:            3,
		MaxReplicas:            10,
		TargetQueueDepthPerPod: 5,
	}

	a := AggregatedLLMMetrics{PodCount: 1, TotalQueueDepth: 2}
	desired := a.DesiredReplicas(cfg)

	if desired != 3 {
		t.Errorf("expected 3 replicas (min), got %d", desired)
	}
}

func TestDesiredReplicas_DefaultConfigShouldScale(t *testing.T) {
	cfg := DefaultScalingConfig()
	a := AggregatedLLMMetrics{
		PodCount: 1, TotalQueueDepth: 100, AvgLatencyMs: 10000,
	}
	desired := a.DesiredReplicas(cfg)
	if desired <= 1 {
		t.Errorf("expected > 1 replicas, got %d", desired)
	}
}

func TestPriorityEvictor_ShouldPreempt(t *testing.T) {
	e := NewPriorityEvictor()
	e.Register(AgentPriority{DeploymentName: "agent-high", Priority: 10, MinReplicas: 1, MaxReplicas: 5})
	e.Register(AgentPriority{DeploymentName: "agent-low", Priority: 1, MinReplicas: 1, MaxReplicas: 5})

	if !e.ShouldPreempt(nil, "agent-high", "agent-low") {
		t.Error("expected preempt to return true")
	}
}

func TestPriorityEvictor_ShouldNotPreemptEqualPriority(t *testing.T) {
	e := NewPriorityEvictor()
	e.Register(AgentPriority{DeploymentName: "agent-1", Priority: 5, MinReplicas: 1, MaxReplicas: 5})
	e.Register(AgentPriority{DeploymentName: "agent-2", Priority: 5, MinReplicas: 1, MaxReplicas: 5})

	if e.ShouldPreempt(nil, "agent-1", "agent-2") {
		t.Error("expected no preempt for equal priority")
	}
}

func TestPriorityEvictor_UnregisteredNoPreempt(t *testing.T) {
	e := NewPriorityEvictor()
	e.Register(AgentPriority{DeploymentName: "agent-a", Priority: 10, MinReplicas: 1, MaxReplicas: 5})

	if e.ShouldPreempt(nil, "agent-a", "unregistered") {
		t.Error("expected no preempt when one is unregistered")
	}
}

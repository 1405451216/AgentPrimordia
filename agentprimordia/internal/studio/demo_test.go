package studio

import (
	"context"
	"testing"
)

// TestDemoChaos_BoundedRetention 有界保留：超过上限后列表不再增长（v5.0 压测修复）。
func TestDemoChaos_BoundedRetention(t *testing.T) {
	d := newDemoChaos()
	ctx := context.Background()
	for i := range maxDemoRetained + 50 {
		_ = d.CreateExperiment(ctx, CreateExperimentRequest{
			Name: "e" + itoaDemo(i), Hypothesis: "h", FaultType: "latency",
		})
	}
	items, err := d.ListExperiments(ctx)
	if err != nil {
		t.Fatalf("ListExperiments: %v", err)
	}
	if len(items) != maxDemoRetained {
		t.Fatalf("experiments = %d, want %d（有界保留）", len(items), maxDemoRetained)
	}
	// 最新实验保留（最旧被淘汰）
	if items[len(items)-1].Experiment.Name != "e"+itoaDemo(maxDemoRetained+49) {
		t.Errorf("最新实验 = %q, want e%d", items[len(items)-1].Experiment.Name, maxDemoRetained+49)
	}
}

func itoaDemo(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [8]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}

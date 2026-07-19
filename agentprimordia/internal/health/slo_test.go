package health

import (
	"math"
	"testing"
	"time"
)

func TestCalculateAvailability(t *testing.T) {
	tests := []struct {
		name     string
		total    int
		failures int
		expected float64
	}{
		{"all-success", 100, 0, 1.0},
		{"some-failures", 100, 5, 0.95},
		{"all-failures", 100, 100, 0.0},
		{"zero-total", 0, 0, 1.0},
		{"negative-failures", 100, -5, 1.0},
		{"failures-exceed-total", 100, 200, 0.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateAvailability(tt.total, tt.failures)
			if math.Abs(got-tt.expected) > 1e-9 {
				t.Errorf("CalculateAvailability(%d, %d) = %v, want %v",
					tt.total, tt.failures, got, tt.expected)
			}
		})
	}
}

func TestCalculateLatencyP99(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		got := CalculateLatencyP99(nil)
		if got != 0 {
			t.Errorf("empty slice should return 0, got %v", got)
		}
	})

	t.Run("single-value", func(t *testing.T) {
		got := CalculateLatencyP99([]time.Duration{42 * time.Millisecond})
		if got != 42*time.Millisecond {
			t.Errorf("single value should return it, got %v", got)
		}
	})

	t.Run("typical-case", func(t *testing.T) {
		latencies := make([]time.Duration, 100)
		for i := 0; i < 100; i++ {
			latencies[i] = time.Duration(i+1) * time.Millisecond
		}
		got := CalculateLatencyP99(latencies)
		// P99 of 1..100ms -> rank = ceil(0.99*100) = 99 -> index 98 -> 99ms
		if got != 99*time.Millisecond {
			t.Errorf("P99 = %v, want 99ms", got)
		}
	})

	t.Run("unsorted-input", func(t *testing.T) {
		latencies := []time.Duration{
			50 * time.Millisecond,
			10 * time.Millisecond,
			100 * time.Millisecond,
			20 * time.Millisecond,
			80 * time.Millisecond,
		}
		got := CalculateLatencyP99(latencies)
		// sorted: 10, 20, 50, 80, 100
		// rank = ceil(0.99*5) = ceil(4.95) = 5 -> index 4 -> 100ms
		if got != 100*time.Millisecond {
			t.Errorf("P99 = %v, want 100ms", got)
		}
	})
}

func TestCheckSLO(t *testing.T) {
	t.Run("meeting-slo", func(t *testing.T) {
		metrics := []SLIMetric{
			{Name: "availability", Value: 0.9995, Target: 0.999},
		}
		statuses := CheckSLO(metrics, 0.999)
		if len(statuses) != 1 {
			t.Fatalf("expected 1 status, got %d", len(statuses))
		}
		s := statuses[0]
		if s.Violated {
			t.Error("SLO should not be violated")
		}
		if s.Current != 0.9995 {
			t.Errorf("current = %v, want 0.9995", s.Current)
		}
		if s.ErrorBudget <= 0 {
			t.Errorf("error budget should be positive, got %v", s.ErrorBudget)
		}
		if s.BurnRate >= 1.0 {
			t.Errorf("burn rate should be < 1, got %v", s.BurnRate)
		}
	})

	t.Run("violating-slo", func(t *testing.T) {
		metrics := []SLIMetric{
			{Name: "availability", Value: 0.99, Target: 0.999},
		}
		statuses := CheckSLO(metrics, 0.999)
		s := statuses[0]
		if !s.Violated {
			t.Error("SLO should be violated")
		}
		if s.ErrorBudget >= 0 {
			t.Errorf("error budget should be negative, got %v", s.ErrorBudget)
		}
		if s.BurnRate <= 1.0 {
			t.Errorf("burn rate should be > 1, got %v", s.BurnRate)
		}
	})

	t.Run("exact-target", func(t *testing.T) {
		metrics := []SLIMetric{
			{Name: "availability", Value: 0.999, Target: 0.999},
		}
		statuses := CheckSLO(metrics, 0.999)
		s := statuses[0]
		if s.Violated {
			t.Error("exact target should not be violated")
		}
		if math.Abs(s.BurnRate-1.0) > 1e-9 {
			t.Errorf("burn rate at target should be 1.0, got %v", s.BurnRate)
		}
		if math.Abs(s.ErrorBudget) > 1e-9 {
			t.Errorf("error budget at target should be 0, got %v", s.ErrorBudget)
		}
	})
}

func TestSLORegistry(t *testing.T) {
	t.Run("update-and-get", func(t *testing.T) {
		r := NewSLORegistry(0.999)
		r.UpdateMetric(SLIMetric{Name: "api", Value: 0.9995})
		r.UpdateMetric(SLIMetric{Name: "db", Value: 0.9999})

		statuses := r.GetSLOStatus()
		if len(statuses) != 2 {
			t.Fatalf("expected 2 statuses, got %d", len(statuses))
		}

		for _, s := range statuses {
			if s.Violated {
				t.Errorf("%s: SLO violated unexpectedly", s.Name)
			}
		}
	})

	t.Run("default-target", func(t *testing.T) {
		r := NewSLORegistry(0) // invalid, should default to 0.999
		r.UpdateMetric(SLIMetric{Name: "test", Value: 0.9995})
		statuses := r.GetSLOStatus()
		if len(statuses) != 1 {
			t.Fatalf("expected 1 status, got %d", len(statuses))
		}
		if statuses[0].Target != 0.999 {
			t.Errorf("default target = %v, want 0.999", statuses[0].Target)
		}
	})

	t.Run("register-callback", func(t *testing.T) {
		r := NewSLORegistry(0.999)
		r.UpdateMetric(SLIMetric{Name: "api", Value: 0.9995})

		var collected []SLOStatus
		RegisterSLOMetrics(r, func(s SLOStatus) {
			collected = append(collected, s)
		})

		if len(collected) != 1 {
			t.Fatalf("callback called %d times, want 1", len(collected))
		}
		if collected[0].Name != "api" {
			t.Errorf("name = %v, want api", collected[0].Name)
		}
	})
}

func TestCalculatePercentile(t *testing.T) {
	latencies := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
		40 * time.Millisecond,
		50 * time.Millisecond,
	}

	t.Run("p50", func(t *testing.T) {
		got := CalculatePercentile(latencies, 50)
		if got != 30*time.Millisecond {
			t.Errorf("P50 = %v, want 30ms", got)
		}
	})

	t.Run("p100", func(t *testing.T) {
		got := CalculatePercentile(latencies, 100)
		if got != 50*time.Millisecond {
			t.Errorf("P100 = %v, want 50ms", got)
		}
	})

	t.Run("empty", func(t *testing.T) {
		got := CalculatePercentile(nil, 50)
		if got != 0 {
			t.Errorf("empty should return 0, got %v", got)
		}
	})
}

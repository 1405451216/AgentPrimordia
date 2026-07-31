package health

import (
	"math"
	"sync"
	"testing"
	"time"
)

// ===== CalculateAvailability 补充边界测试 =====

func TestCalculateAvailability_Concurrent(t *testing.T) {
	// 并发安全性验证：CalculateAvailability 是纯函数，并发调用应安全
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got := CalculateAvailability(1000, 5)
			if math.Abs(got-0.995) > 1e-9 {
				t.Errorf("并发计算结果 = %v, 期望 0.995", got)
			}
		}()
	}
	wg.Wait()
}

// ===== CalculateLatencyP99 补充边界测试 =====

func TestCalculateLatencyP99_AllSameValues(t *testing.T) {
	// 所有延迟值相同
	latencies := make([]time.Duration, 10)
	for i := range latencies {
		latencies[i] = 50 * time.Millisecond
	}
	got := CalculateLatencyP99(latencies)
	if got != 50*time.Millisecond {
		t.Errorf("相同值 P99 = %v, 期望 50ms", got)
	}
}

func TestCalculateLatencyP99_TwoValues(t *testing.T) {
	latencies := []time.Duration{
		10 * time.Millisecond,
		200 * time.Millisecond,
	}
	got := CalculateLatencyP99(latencies)
	// rank = ceil(0.99*2) = ceil(1.98) = 2 -> index 1 -> 200ms
	if got != 200*time.Millisecond {
		t.Errorf("两个值 P99 = %v, 期望 200ms", got)
	}
}

func TestCalculateLatencyP99_DoesNotMutateInput(t *testing.T) {
	// 验证不会修改原始切片
	latencies := []time.Duration{
		50 * time.Millisecond,
		10 * time.Millisecond,
		30 * time.Millisecond,
	}
	original := make([]time.Duration, len(latencies))
	copy(original, latencies)

	_ = CalculateLatencyP99(latencies)

	for i := range latencies {
		if latencies[i] != original[i] {
			t.Errorf("输入切片被修改: index %d, 原始 = %v, 当前 = %v", i, original[i], latencies[i])
		}
	}
}

// ===== CalculatePercentile 补充边界测试 =====

func TestCalculatePercentile_P0(t *testing.T) {
	latencies := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
	}
	// percentile=0 被钳位到 0，rank = ceil(0/100*3) = 0 -> idx = -1 -> 钳位到 0
	got := CalculatePercentile(latencies, 0)
	if got != 10*time.Millisecond {
		t.Errorf("P0 = %v, 期望 10ms", got)
	}
}

func TestCalculatePercentile_NegativePercentile(t *testing.T) {
	latencies := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
	}
	// 负值被钳位到 0
	got := CalculatePercentile(latencies, -50)
	if got != 10*time.Millisecond {
		t.Errorf("负百分位 = %v, 期望 10ms", got)
	}
}

func TestCalculatePercentile_Over100(t *testing.T) {
	latencies := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
	}
	// 超过 100 被钳位到 100
	got := CalculatePercentile(latencies, 150)
	if got != 20*time.Millisecond {
		t.Errorf("超 100 百分位 = %v, 期望 20ms", got)
	}
}

func TestCalculatePercentile_P95(t *testing.T) {
	latencies := make([]time.Duration, 20)
	for i := range latencies {
		latencies[i] = time.Duration(i+1) * time.Millisecond
	}
	// P95: rank = ceil(95/100*20) = ceil(19) = 19 -> idx 18 -> 19ms
	got := CalculatePercentile(latencies, 95)
	if got != 19*time.Millisecond {
		t.Errorf("P95 = %v, 期望 19ms", got)
	}
}

func TestCalculatePercentile_SingleElement(t *testing.T) {
	latencies := []time.Duration{42 * time.Millisecond}
	got := CalculatePercentile(latencies, 50)
	if got != 42*time.Millisecond {
		t.Errorf("单元素 P50 = %v, 期望 42ms", got)
	}
}

func TestCalculatePercentile_Concurrent(t *testing.T) {
	// 并发安全性验证
	latencies := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
		40 * time.Millisecond,
		50 * time.Millisecond,
	}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got := CalculatePercentile(latencies, 50)
			if got != 30*time.Millisecond {
				t.Errorf("并发 P50 = %v, 期望 30ms", got)
			}
		}()
	}
	wg.Wait()
}

// ===== CheckSLOsWithDefaults 测试 =====

func TestCheckSLOsWithDefaults(t *testing.T) {
	t.Run("正常指标", func(t *testing.T) {
		metrics := []SLIMetric{
			{Name: "api", Value: 0.9995, Target: 0.999},
			{Name: "db", Value: 0.998, Target: 0.999},
		}
		statuses := CheckSLOsWithDefaults(metrics)
		if len(statuses) != 2 {
			t.Fatalf("期望 2 个状态, 得到 %d", len(statuses))
		}
		// api 满足 SLO
		if statuses[0].Violated {
			t.Error("api 不应违反 SLO")
		}
		if statuses[0].Name != "api" {
			t.Errorf("名称 = %v, 期望 api", statuses[0].Name)
		}
		// db 违反 SLO
		if !statuses[1].Violated {
			t.Error("db 应违反 SLO")
		}
	})

	t.Run("无效Target使用默认值", func(t *testing.T) {
		metrics := []SLIMetric{
			{Name: "test", Value: 0.9995, Target: 0}, // Target=0 无效
		}
		statuses := CheckSLOsWithDefaults(metrics)
		if statuses[0].Target != 0.999 {
			t.Errorf("默认 Target = %v, 期望 0.999", statuses[0].Target)
		}
	})

	t.Run("空指标列表", func(t *testing.T) {
		statuses := CheckSLOsWithDefaults(nil)
		if len(statuses) != 0 {
			t.Errorf("空指标应返回空切片, 得到 %d 个", len(statuses))
		}
	})

	t.Run("负值钳位", func(t *testing.T) {
		metrics := []SLIMetric{
			{Name: "neg", Value: -0.5, Target: 0.999},
		}
		statuses := CheckSLOsWithDefaults(metrics)
		if statuses[0].Current != 0 {
			t.Errorf("负值应钳位到 0, 得到 %v", statuses[0].Current)
		}
	})

	t.Run("超1值钳位", func(t *testing.T) {
		metrics := []SLIMetric{
			{Name: "over", Value: 1.5, Target: 0.999},
		}
		statuses := CheckSLOsWithDefaults(metrics)
		if statuses[0].Current != 1.0 {
			t.Errorf("超 1 值应钳位到 1, 得到 %v", statuses[0].Current)
		}
	})
}

// ===== RegisterSLOMetrics nil 安全测试 =====

func TestRegisterSLOMetrics_NilRegistry(t *testing.T) {
	// 不应 panic
	RegisterSLOMetrics(nil, func(s SLOStatus) {
		t.Error("不应被调用")
	})
}

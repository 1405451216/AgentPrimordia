package otel

import (
	"strings"
	"testing"
)

func TestNewMetricExporter(t *testing.T) {
	e := NewMetricExporter()
	if e == nil {
		t.Fatal("NewMetricExporter should not return nil")
	}
	if e.counters == nil || e.gauges == nil || e.histograms == nil {
		t.Error("maps should be initialized")
	}
}

func TestMetricExporter_RecordCounter(t *testing.T) {
	e := NewMetricExporter()

	// 记录计数器
	e.RecordCounter("http_requests_total", 5, map[string]string{"method": "GET"})
	e.RecordCounter("http_requests_total", 3, map[string]string{"method": "POST"})

	e.mu.Lock()
	if len(e.counters) != 2 {
		t.Errorf("expected 2 counter entries, got %d", len(e.counters))
	}
	e.mu.Unlock()

	// 相同标签累加
	e.RecordCounter("http_requests_total", 2, map[string]string{"method": "GET"})
	e.mu.Lock()
	key := counterKey("http_requests_total", map[string]string{"method": "GET"})
	if e.counters[key].Value != 7 { // 5 + 2
		t.Errorf("expected counter value 7, got %f", e.counters[key].Value)
	}
	e.mu.Unlock()
}

func TestMetricExporter_RecordGauge(t *testing.T) {
	e := NewMetricExporter()

	// 记录仪表盘
	e.RecordGauge("memory_usage_bytes", 1024.0, map[string]string{"host": "localhost"})

	e.mu.Lock()
	if len(e.gauges) != 1 {
		t.Errorf("expected 1 gauge entry, got %d", len(e.gauges))
	}
	e.mu.Unlock()

	// 相同标签覆盖
	e.RecordGauge("memory_usage_bytes", 2048.0, map[string]string{"host": "localhost"})
	e.mu.Lock()
	key := gaugeKey("memory_usage_bytes", map[string]string{"host": "localhost"})
	if e.gauges[key].Value != 2048.0 {
		t.Errorf("expected gauge value 2048, got %f", e.gauges[key].Value)
	}
	e.mu.Unlock()
}

func TestMetricExporter_RecordHistogram(t *testing.T) {
	e := NewMetricExporter()

	buckets := []float64{0.1, 0.5, 1.0, 5.0}
	e.RecordHistogram("request_duration_seconds", 0.3, map[string]string{"handler": "/api"}, buckets)
	e.RecordHistogram("request_duration_seconds", 0.8, map[string]string{"handler": "/api"}, buckets)
	e.RecordHistogram("request_duration_seconds", 3.0, map[string]string{"handler": "/api"}, buckets)

	e.mu.Lock()
	if len(e.histograms) != 1 {
		t.Errorf("expected 1 histogram entry, got %d", len(e.histograms))
	}
	key := histogramKey("request_duration_seconds", map[string]string{"handler": "/api"})
	h := e.histograms[key]
	if h.Count != 3 {
		t.Errorf("expected count 3, got %d", h.Count)
	}
	if h.Sum != 4.1 { // 0.3 + 0.8 + 3.0
		t.Errorf("expected sum 4.1, got %f", h.Sum)
	}
	// 0.3 落入 [0.1, 0.5), 0.8 落入 [0.5, 1.0), 3.0 落入 [1.0, 5.0)
	// bucket counts: <=0.1=0, <=0.5=1, <=1.0=2, <=5.0=3
	if len(h.Counts) != 4 {
		t.Errorf("expected 4 bucket counts, got %d", len(h.Counts))
	}
	if h.Counts[0] != 0 {
		t.Errorf("bucket 0 (<=0.1) expected 0, got %d", h.Counts[0])
	}
	if h.Counts[1] != 1 {
		t.Errorf("bucket 1 (<=0.5) expected 1, got %d", h.Counts[1])
	}
	if h.Counts[2] != 2 {
		t.Errorf("bucket 2 (<=1.0) expected 2, got %d", h.Counts[2])
	}
	if h.Counts[3] != 3 {
		t.Errorf("bucket 3 (<=5.0) expected 3, got %d", h.Counts[3])
	}
	e.mu.Unlock()
}

func TestMetricExporter_ExportPrometheus(t *testing.T) {
	e := NewMetricExporter()

	e.RecordCounter("http_requests_total", 10, map[string]string{"method": "GET", "status": "200"})
	e.RecordGauge("memory_usage_bytes", 4096.0, map[string]string{"host": "localhost"})
	e.RecordHistogram("request_duration_seconds", 0.5, map[string]string{"handler": "/api"},
		[]float64{0.1, 0.5, 1.0})

	output := e.ExportPrometheus()

	// 验证计数器格式
	if !strings.Contains(output, "# TYPE http_requests_total counter") {
		t.Error("Prometheus output should contain counter TYPE declaration")
	}
	if !strings.Contains(output, `http_requests_total{method="GET",status="200"} 10`) {
		t.Errorf("Prometheus output should contain counter value line, got:\n%s", output)
	}

	// 验证仪表盘格式
	if !strings.Contains(output, "# TYPE memory_usage_bytes gauge") {
		t.Error("Prometheus output should contain gauge TYPE declaration")
	}
	if !strings.Contains(output, `memory_usage_bytes{host="localhost"} 4096`) {
		t.Errorf("Prometheus output should contain gauge value line, got:\n%s", output)
	}

	// 验证直方图格式
	if !strings.Contains(output, "# TYPE request_duration_seconds histogram") {
		t.Error("Prometheus output should contain histogram TYPE declaration")
	}
	if !strings.Contains(output, `request_duration_seconds_bucket{handler="/api",le="0.1"}`) {
		t.Errorf("Prometheus output should contain histogram bucket, got:\n%s", output)
	}
	if !strings.Contains(output, `request_duration_seconds_sum{handler="/api"}`) {
		t.Errorf("Prometheus output should contain histogram sum, got:\n%s", output)
	}
	if !strings.Contains(output, `request_duration_seconds_count{handler="/api"}`) {
		t.Errorf("Prometheus output should contain histogram count, got:\n%s", output)
	}
}

func TestMetricExporter_ExportPrometheus_Empty(t *testing.T) {
	e := NewMetricExporter()
	output := e.ExportPrometheus()
	if output != "" {
		t.Errorf("empty exporter should produce empty output, got: %q", output)
	}
}

func TestMetricExporter_ExportOTLP(t *testing.T) {
	e := NewMetricExporter()

	e.RecordCounter("http_requests_total", 10, map[string]string{"method": "GET"})
	e.RecordGauge("memory_usage_bytes", 4096.0, map[string]string{"host": "localhost"})
	e.RecordHistogram("request_duration_seconds", 0.5, map[string]string{"handler": "/api"},
		[]float64{0.1, 0.5, 1.0})

	data, err := e.ExportOTLP()
	if err != nil {
		t.Fatalf("ExportOTLP error: %v", err)
	}

	// 验证是合法 JSON
	output := string(data)
	if !strings.Contains(output, `"resourceMetrics"`) {
		t.Error("OTLP output should contain resourceMetrics")
	}
	if !strings.Contains(output, `"scopeMetrics"`) {
		t.Error("OTLP output should contain scopeMetrics")
	}
	if !strings.Contains(output, `"http_requests_total"`) {
		t.Error("OTLP output should contain counter name")
	}
	if !strings.Contains(output, `"memory_usage_bytes"`) {
		t.Error("OTLP output should contain gauge name")
	}
	if !strings.Contains(output, `"request_duration_seconds"`) {
		t.Error("OTLP output should contain histogram name")
	}
}

func TestMetricExporter_ExportOTLP_Empty(t *testing.T) {
	e := NewMetricExporter()
	data, err := e.ExportOTLP()
	if err != nil {
		t.Fatalf("ExportOTLP error: %v", err)
	}
	output := string(data)
	if !strings.Contains(output, `"resourceMetrics"`) {
		t.Error("even empty OTLP should have resourceMetrics structure")
	}
}

func TestMetricExporter_ConcurrentAccess(t *testing.T) {
	e := NewMetricExporter()
	done := make(chan struct{})

	// 并发写入计数器
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			e.RecordCounter("concurrent_counter", 1, map[string]string{"worker": "a"})
		}()
	}
	// 并发写入仪表盘
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			e.RecordGauge("concurrent_gauge", 1.0, map[string]string{"worker": "b"})
		}()
	}

	// 等待所有 goroutine 完成
	for i := 0; i < 20; i++ {
		<-done
	}

	// 验证数据一致性
	e.mu.Lock()
	key := counterKey("concurrent_counter", map[string]string{"worker": "a"})
	if e.counters[key].Value != 10 {
		t.Errorf("expected concurrent counter value 10, got %f", e.counters[key].Value)
	}
	e.mu.Unlock()
}

package metrics

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// newTestServer 创建基于 PrometheusHandler 内部 mux 的 httptest.Server
// 避免启动真实端口，直接复用 handler 逻辑
func newTestServer(m *AgentMetrics) *httptest.Server {
	h := NewPrometheusHandler(m, ":0")
	return httptest.NewServer(h.server.Handler)
}

// ===== 功能验证 =====

func TestPrometheusHandler_MetricsEndpoint(t *testing.T) {
	m := NewMetrics()
	m.RecordLLMCall(100*time.Millisecond, nil)
	m.RecordToolCall(50*time.Millisecond, errTest)

	ts := newTestServer(m)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/metrics")
	if err != nil {
		t.Fatalf("请求 /metrics 失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("期望状态码 200，得到 %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Errorf("期望 Content-Type 包含 text/plain，得到 %s", ct)
	}

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "ap_llm_total_calls") {
		t.Error("响应体应包含 ap_llm_total_calls")
	}
	if !strings.Contains(bodyStr, "ap_tool_total_calls") {
		t.Error("响应体应包含 ap_tool_total_calls")
	}
}

func TestPrometheusHandler_HealthEndpoint(t *testing.T) {
	m := NewMetrics()
	ts := newTestServer(m)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("请求 /health 失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("期望状态码 200，得到 %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("期望 Content-Type 包含 application/json，得到 %s", ct)
	}

	body, _ := io.ReadAll(resp.Body)
	var result map[string]string
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("解析 JSON 失败: %v", err)
	}
	if result["status"] != "ok" {
		t.Errorf("期望 status=ok，得到 %s", result["status"])
	}
}

// ===== 边界条件 =====

func TestPrometheusHandler_EmptyMetrics(t *testing.T) {
	m := NewMetrics()
	ts := newTestServer(m)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/metrics")
	if err != nil {
		t.Fatalf("请求 /metrics 失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("期望状态码 200，得到 %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	if !strings.Contains(bodyStr, "ap_llm_total_calls 0") {
		t.Error("空指标应返回 ap_llm_total_calls 0")
	}
	if !strings.Contains(bodyStr, "ap_tool_total_calls 0") {
		t.Error("空指标应返回 ap_tool_total_calls 0")
	}
	if !strings.Contains(bodyStr, "ap_active_agents 0") {
		t.Error("空指标应返回 ap_active_agents 0")
	}
	if !strings.Contains(bodyStr, "# TYPE") {
		t.Error("空指标仍应包含 Prometheus TYPE 声明")
	}
}

func TestPrometheusHandler_LargeVolumeMetrics(t *testing.T) {
	m := NewMetrics()

	for i := 0; i < 500; i++ {
		m.RecordLLMCall(time.Duration(i+1)*time.Millisecond, nil)
		m.RecordToolCall(time.Duration(i+1)*time.Millisecond, nil)
		if i%10 == 0 {
			m.RecordLLMCall(10*time.Millisecond, errTest)
			m.RecordToolCall(10*time.Millisecond, errTest)
		}
	}
	for i := 0; i < 200; i++ {
		m.RecordTurn(time.Duration(i+1) * time.Millisecond)
	}

	ts := newTestServer(m)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/metrics")
	if err != nil {
		t.Fatalf("请求 /metrics 失败: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	if !strings.Contains(bodyStr, "ap_llm_total_calls 550") {
		t.Error("应包含 ap_llm_total_calls 550（500成功+50错误）")
	}
	if !strings.Contains(bodyStr, "ap_llm_total_errors 50") {
		t.Error("应包含 ap_llm_total_errors 50")
	}
	if !strings.Contains(bodyStr, "ap_tool_total_calls 550") {
		t.Error("应包含 ap_tool_total_calls 550")
	}
	if !strings.Contains(bodyStr, "ap_total_turns 200") {
		t.Error("应包含 ap_total_turns 200")
	}
}

// ===== 并发安全 =====

func TestPrometheusHandler_ConcurrentRequests(t *testing.T) {
	m := NewMetrics()
	m.RecordLLMCall(50*time.Millisecond, nil)
	m.RecordToolCall(30*time.Millisecond, nil)

	ts := newTestServer(m)
	defer ts.Close()

	var wg sync.WaitGroup
	errCh := make(chan error, 100)

	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			resp, err := http.Get(ts.URL + "/metrics")
			if err != nil {
				errCh <- err
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				errCh <- errResp(resp.StatusCode)
				return
			}
		}()
		go func() {
			defer wg.Done()
			resp, err := http.Get(ts.URL + "/health")
			if err != nil {
				errCh <- err
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				errCh <- errResp(resp.StatusCode)
				return
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("并发请求失败: %v", err)
	}
}

// ===== 兼容性 - Prometheus 格式验证 =====

func TestPrometheusHandler_PrometheusFormatValidation(t *testing.T) {
	m := NewMetrics()
	m.RecordLLMCall(100*time.Millisecond, nil)
	m.IncActiveAgents()
	m.SetPoolQueue(3)

	ts := newTestServer(m)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/metrics")
	if err != nil {
		t.Fatalf("请求 /metrics 失败: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	lines := strings.Split(strings.TrimSpace(bodyStr), "\n")

	helpCount := 0
	typeCount := 0
	metricCount := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "# HELP ") {
			helpCount++
			parts := strings.Fields(line)
			if len(parts) < 4 {
				t.Errorf("HELP 行格式不正确: %s", line)
			}
			if !strings.HasPrefix(parts[2], "ap_") {
				t.Errorf("指标名应以 ap_ 开头: %s", parts[2])
			}
		} else if strings.HasPrefix(line, "# TYPE ") {
			typeCount++
			parts := strings.Fields(line)
			if len(parts) < 4 {
				t.Errorf("TYPE 行格式不正确: %s", line)
			}
			metricType := parts[3]
			if metricType != "counter" && metricType != "gauge" && metricType != "histogram" {
				t.Errorf("未知的指标类型: %s", metricType)
			}
		} else if line != "" && !strings.HasPrefix(line, "#") {
			metricCount++
			if !strings.Contains(line, "ap_") {
				t.Errorf("指标行应包含 ap_ 前缀: %s", line)
			}
		}
	}

	if helpCount == 0 {
		t.Error("Prometheus 输出应包含 # HELP 注释")
	}
	if typeCount == 0 {
		t.Error("Prometheus 输出应包含 # TYPE 声明")
	}
	if metricCount == 0 {
		t.Error("Prometheus 输出应包含指标数据行")
	}
}

// ===== 性能 - 连续请求 =====

func TestPrometheusHandler_RapidRequests(t *testing.T) {
	m := NewMetrics()
	m.RecordLLMCall(50*time.Millisecond, nil)

	ts := newTestServer(m)
	defer ts.Close()

	for i := 0; i < 100; i++ {
		resp, err := http.Get(ts.URL + "/metrics")
		if err != nil {
			t.Fatalf("第 %d 次请求失败: %v", i+1, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("第 %d 次请求状态码异常: %d", i+1, resp.StatusCode)
		}
	}
}

// ===== 错误处理 - 非 GET 方法 =====

func TestPrometheusHandler_PostMethodNotAllowed(t *testing.T) {
	m := NewMetrics()
	ts := newTestServer(m)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/metrics", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("POST 请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST /metrics 应返回 405，得到 %d", resp.StatusCode)
	}
}

func TestPrometheusHandler_PostHealthNotAllowed(t *testing.T) {
	m := NewMetrics()
	ts := newTestServer(m)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/health", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("POST 请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST /health 应返回 405，得到 %d", resp.StatusCode)
	}
}

// errResp 用于将状态码转为 error
type errResp int

func (e errResp) Error() string {
	return http.StatusText(int(e))
}

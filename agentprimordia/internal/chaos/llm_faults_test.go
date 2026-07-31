// llm_faults_test.go — LLM 故障类型测试
package chaos

import (
	"strings"
	"testing"
	"time"
)

// ===== LLMHTTPStatusFault 工厂函数测试 =====

func TestLLMHTTP503Fault(t *testing.T) {
	f := LLMHTTP503Fault("openai")
	if f.Provider != "openai" {
		t.Errorf("Provider = %s, 期望 openai", f.Provider)
	}
	if f.StatusCode != 503 {
		t.Errorf("StatusCode = %d, 期望 503", f.StatusCode)
	}
	if f.Duration != 30*time.Second {
		t.Errorf("Duration = %v, 期望 30s", f.Duration)
	}
	if f.Body == "" {
		t.Error("Body 不应为空")
	}
}

func TestLLMHTTP429Fault(t *testing.T) {
	f := LLMHTTP429Fault("anthropic")
	if f.Provider != "anthropic" {
		t.Errorf("Provider = %s, 期望 anthropic", f.Provider)
	}
	if f.StatusCode != 429 {
		t.Errorf("StatusCode = %d, 期望 429", f.StatusCode)
	}
	if f.Duration != 30*time.Second {
		t.Errorf("Duration = %v, 期望 30s", f.Duration)
	}
}

func TestLLMHTTP500Fault(t *testing.T) {
	f := LLMHTTP500Fault("cohere")
	if f.Provider != "cohere" {
		t.Errorf("Provider = %s, 期望 cohere", f.Provider)
	}
	if f.StatusCode != 500 {
		t.Errorf("StatusCode = %d, 期望 500", f.StatusCode)
	}
}

// ===== LLMHTTPStatusFault Type/Description =====

func TestLLMHTTPStatusFault_Type(t *testing.T) {
	tests := []struct {
		code     int
		expected string
	}{
		{429, "llm_http_429"},
		{500, "llm_http_500"},
		{503, "llm_http_503"},
	}
	for _, tt := range tests {
		f := &LLMHTTPStatusFault{StatusCode: tt.code}
		if got := f.Type(); got != tt.expected {
			t.Errorf("Type() for %d = %s, 期望 %s", tt.code, got, tt.expected)
		}
	}
}

func TestLLMHTTPStatusFault_Description(t *testing.T) {
	f := LLMHTTP503Fault("openai")
	desc := f.Description()
	if !strings.Contains(desc, "openai") {
		t.Errorf("Description() 应包含 provider, 得到 %s", desc)
	}
	if !strings.Contains(desc, "503") {
		t.Errorf("Description() 应包含状态码, 得到 %s", desc)
	}
}

// ===== LLMTimeoutFault 测试 =====

func TestNewLLMTimeoutFault(t *testing.T) {
	f := NewLLMTimeoutFault("openai", 10*time.Second)
	if f.Provider != "openai" {
		t.Errorf("Provider = %s", f.Provider)
	}
	if f.Timeout != 10*time.Second {
		t.Errorf("Timeout = %v", f.Timeout)
	}
}

func TestLLMTimeoutFault_Type(t *testing.T) {
	f := NewLLMTimeoutFault("openai", 5*time.Second)
	if f.Type() != "llm_timeout" {
		t.Errorf("Type() = %s, 期望 llm_timeout", f.Type())
	}
}

func TestLLMTimeoutFault_Description(t *testing.T) {
	f := NewLLMTimeoutFault("anthropic", 5*time.Second)
	desc := f.Description()
	if !strings.Contains(desc, "anthropic") {
		t.Errorf("Description() 应包含 provider, 得到 %s", desc)
	}
	if !strings.Contains(desc, "5s") {
		t.Errorf("Description() 应包含超时时长, 得到 %s", desc)
	}
}

// ===== LLMIntermittentFault 测试 =====

func TestNewLLMIntermittentFault(t *testing.T) {
	f := NewLLMIntermittentFault("openai", 0.5)
	if f.Provider != "openai" {
		t.Errorf("Provider = %s", f.Provider)
	}
	if f.FailureRate != 0.5 {
		t.Errorf("FailureRate = %f", f.FailureRate)
	}
	if f.FailureStatus != 503 {
		t.Errorf("FailureStatus = %d, 默认应为 503", f.FailureStatus)
	}
}

func TestLLMIntermittentFault_Type(t *testing.T) {
	f := NewLLMIntermittentFault("openai", 0.3)
	if f.Type() != "llm_intermittent" {
		t.Errorf("Type() = %s, 期望 llm_intermittent", f.Type())
	}
}

func TestLLMIntermittentFault_Description(t *testing.T) {
	f := NewLLMIntermittentFault("openai", 0.3)
	desc := f.Description()
	if !strings.Contains(desc, "openai") {
		t.Errorf("Description() 应包含 provider, 得到 %s", desc)
	}
	if !strings.Contains(desc, "30%") {
		t.Errorf("Description() 应包含故障率百分比, 得到 %s", desc)
	}
}

// ===== LLMSlowResponseFault 测试 =====

func TestNewLLMSlowResponseFault(t *testing.T) {
	f := NewLLMSlowResponseFault("openai", 1*time.Second, 5*time.Second)
	if f.Provider != "openai" {
		t.Errorf("Provider = %s", f.Provider)
	}
	if f.MinDelay != 1*time.Second {
		t.Errorf("MinDelay = %v", f.MinDelay)
	}
	if f.MaxDelay != 5*time.Second {
		t.Errorf("MaxDelay = %v", f.MaxDelay)
	}
}

func TestLLMSlowResponseFault_Type(t *testing.T) {
	f := NewLLMSlowResponseFault("openai", time.Second, 3*time.Second)
	if f.Type() != "llm_slow_response" {
		t.Errorf("Type() = %s, 期望 llm_slow_response", f.Type())
	}
}

func TestLLMSlowResponseFault_Description(t *testing.T) {
	f := NewLLMSlowResponseFault("anthropic", 2*time.Second, 8*time.Second)
	desc := f.Description()
	if !strings.Contains(desc, "anthropic") {
		t.Errorf("Description() 应包含 provider, 得到 %s", desc)
	}
	if !strings.Contains(desc, "2s") || !strings.Contains(desc, "8s") {
		t.Errorf("Description() 应包含延迟范围, 得到 %s", desc)
	}
}

// ===== LLMFaultScenario 测试 =====

func TestLLMFailoverScenario(t *testing.T) {
	s := LLMFailoverScenario("openai")
	if s.Name != "llm_failover_sequence" {
		t.Errorf("Name = %s", s.Name)
	}
	if s.Provider != "openai" {
		t.Errorf("Provider = %s", s.Provider)
	}
	if len(s.Faults) != 3 {
		t.Fatalf("Faults 数量 = %d, 期望 3", len(s.Faults))
	}
	// 验证故障类型序列
	expectedTypes := []string{"llm_http_503", "llm_http_429", "llm_timeout"}
	for i, expected := range expectedTypes {
		if s.Faults[i].Type() != expected {
			t.Errorf("Faults[%d].Type() = %s, 期望 %s", i, s.Faults[i].Type(), expected)
		}
	}
}

func TestLLMChaosScenario(t *testing.T) {
	s := LLMChaosScenario("anthropic")
	if s.Name != "llm_chaos_mixed" {
		t.Errorf("Name = %s", s.Name)
	}
	if s.Provider != "anthropic" {
		t.Errorf("Provider = %s", s.Provider)
	}
	if len(s.Faults) != 2 {
		t.Fatalf("Faults 数量 = %d, 期望 2", len(s.Faults))
	}
	if s.Faults[0].Type() != "llm_intermittent" {
		t.Errorf("Faults[0].Type() = %s, 期望 llm_intermittent", s.Faults[0].Type())
	}
	if s.Faults[1].Type() != "llm_slow_response" {
		t.Errorf("Faults[1].Type() = %s, 期望 llm_slow_response", s.Faults[1].Type())
	}
}

// ===== LLMHTTPStatusFault Body 内容验证 =====

func TestLLMHTTPStatusFault_BodyContent(t *testing.T) {
	tests := []struct {
		name       string
		factory    func(string) *LLMHTTPStatusFault
		bodySubstr string
	}{
		{"503", LLMHTTP503Fault, "Service Unavailable"},
		{"429", LLMHTTP429Fault, "Rate limit"},
		{"500", LLMHTTP500Fault, "Internal Server Error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := tt.factory("test-provider")
			if !strings.Contains(f.Body, tt.bodySubstr) {
				t.Errorf("Body 应包含 %q, 得到 %s", tt.bodySubstr, f.Body)
			}
		})
	}
}

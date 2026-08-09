package llm

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestConfig_Validate_Valid(t *testing.T) {
	cfg := Config{
		Model:       "gpt-4o",
		Temperature: 0.7,
		MaxTokens:   100,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConfig_Validate_MissingModel(t *testing.T) {
	cfg := Config{
		Temperature: 0.5,
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing model")
	}
	if !strings.Contains(err.Error(), "model is required") {
		t.Errorf("expected 'model is required' error, got '%s'", err.Error())
	}
}

func TestConfig_Validate_InvalidTemperature(t *testing.T) {
	cfg := Config{
		Model:       "gpt-4o",
		Temperature: 3.5,
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid temperature")
	}
	if !strings.Contains(err.Error(), "temperature") {
		t.Errorf("expected temperature error, got '%s'", err.Error())
	}
}

func TestConfig_Validate_NegativeTemperature(t *testing.T) {
	cfg := Config{
		Model:       "gpt-4o",
		Temperature: -0.5,
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for negative temperature")
	}
}

func TestConfig_Validate_InvalidMaxTokens(t *testing.T) {
	cfg := Config{
		Model:     "gpt-4o",
		MaxTokens: -100,
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for negative max_tokens")
	}
	if !strings.Contains(err.Error(), "max_tokens") {
		t.Errorf("expected max_tokens error, got '%s'", err.Error())
	}
}

func TestConfig_Validate_InvalidBaseURL(t *testing.T) {
	cfg := Config{
		Model:   "gpt-4o",
		BaseURL: "ftp://invalid.com",
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid base_url")
	}
	if !strings.Contains(err.Error(), "base_url") {
		t.Errorf("expected base_url error, got '%s'", err.Error())
	}
}

func TestConfig_Validate_ValidBaseURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"http", "http://localhost:8080"},
		{"https", "https://api.openai.com/v1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{Model: "gpt-4o", BaseURL: tt.url}
			if err := cfg.Validate(); err != nil {
				t.Fatalf("unexpected error for %s: %v", tt.name, err)
			}
		})
	}
}

func TestConfig_Validate_EmptyBaseURL(t *testing.T) {
	cfg := Config{Model: "gpt-4o"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("empty BaseURL should be valid: %v", err)
	}
}

func TestConfig_Validate_MultipleErrors(t *testing.T) {
	cfg := Config{
		Temperature: 3.5,
		MaxTokens:   -1,
		BaseURL:     "ftp://bad",
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "model is required") {
		t.Error("expected 'model is required' in error")
	}
	if !strings.Contains(errMsg, "temperature") {
		t.Error("expected 'temperature' in error")
	}
	if !strings.Contains(errMsg, "max_tokens") {
		t.Error("expected 'max_tokens' in error")
	}
	if !strings.Contains(errMsg, "base_url") {
		t.Error("expected 'base_url' in error")
	}
}

func TestConfigFromEnv_DefaultPrefix(t *testing.T) {
	os.Setenv("AP_LLM_API_KEY", "test-api-key")
	os.Setenv("AP_LLM_MODEL", "gpt-4o")
	os.Setenv("AP_LLM_BASE_URL", "https://api.test.com")
	os.Setenv("AP_LLM_TEMPERATURE", "0.5")
	os.Setenv("AP_LLM_MAX_TOKENS", "100")
	defer func() {
		os.Unsetenv("AP_LLM_API_KEY")
		os.Unsetenv("AP_LLM_MODEL")
		os.Unsetenv("AP_LLM_BASE_URL")
		os.Unsetenv("AP_LLM_TEMPERATURE")
		os.Unsetenv("AP_LLM_MAX_TOKENS")
	}()

	cfg := ConfigFromEnv("")
	if cfg.APIKey != "test-api-key" {
		t.Errorf("expected 'test-api-key', got '%s'", cfg.APIKey)
	}
	if cfg.Model != "gpt-4o" {
		t.Errorf("expected 'gpt-4o', got '%s'", cfg.Model)
	}
	if cfg.BaseURL != "https://api.test.com" {
		t.Errorf("expected 'https://api.test.com', got '%s'", cfg.BaseURL)
	}
	if cfg.Temperature != 0.5 {
		t.Errorf("expected 0.5, got %f", cfg.Temperature)
	}
	if cfg.MaxTokens != 100 {
		t.Errorf("expected 100, got %d", cfg.MaxTokens)
	}
}

func TestConfigFromEnv_CustomPrefix(t *testing.T) {
	os.Setenv("MYAPP_API_KEY", "my-key")
	os.Setenv("MYAPP_MODEL", "my-model")
	defer func() {
		os.Unsetenv("MYAPP_API_KEY")
		os.Unsetenv("MYAPP_MODEL")
	}()

	cfg := ConfigFromEnv("MYAPP")
	if cfg.APIKey != "my-key" {
		t.Errorf("expected 'my-key', got '%s'", cfg.APIKey)
	}
	if cfg.Model != "my-model" {
		t.Errorf("expected 'my-model', got '%s'", cfg.Model)
	}
}

func TestConfigFromEnv_Extra(t *testing.T) {
	os.Setenv("AP_LLM_EXTRA_REGION", "us-east-1")
	defer os.Unsetenv("AP_LLM_EXTRA_REGION")

	cfg := ConfigFromEnv("")
	if cfg.Extra == nil {
		t.Fatal("expected Extra map to be initialized")
	}
	if cfg.Extra["region"] != "us-east-1" {
		t.Errorf("expected 'us-east-1', got '%v'", cfg.Extra["region"])
	}
}

func TestConfigFromEnv_InvalidTemperature(t *testing.T) {
	os.Setenv("AP_LLM_TEMPERATURE", "not-a-number")
	defer os.Unsetenv("AP_LLM_TEMPERATURE")

	cfg := ConfigFromEnv("")
	if cfg.Temperature != 0 {
		t.Errorf("expected default 0 for invalid temperature, got %f", cfg.Temperature)
	}
}

func TestConfigFromEnv_InvalidMaxTokens(t *testing.T) {
	os.Setenv("AP_LLM_MAX_TOKENS", "not-a-number")
	defer os.Unsetenv("AP_LLM_MAX_TOKENS")

	cfg := ConfigFromEnv("")
	if cfg.MaxTokens != 0 {
		t.Errorf("expected default 0 for invalid max_tokens, got %d", cfg.MaxTokens)
	}
}

func TestConfigFromEnv_EmptyPrefix(t *testing.T) {
	cfg := ConfigFromEnv("")
	if cfg.APIKey != "" {
		t.Errorf("expected empty APIKey, got '%s'", cfg.APIKey)
	}
}

func TestResolveTemperature_RequestTakesPriority(t *testing.T) {
	reqTemp := Float64Ptr(0.8)
	configTemp := 0.5

	result := ResolveTemperature(reqTemp, configTemp)
	if result == nil || *result != 0.8 {
		t.Errorf("expected 0.8, got %v", result)
	}
}

func TestResolveTemperature_ConfigFallback(t *testing.T) {
	configTemp := 0.5

	result := ResolveTemperature(nil, configTemp)
	if result == nil || *result != 0.5 {
		t.Errorf("expected 0.5, got %v", result)
	}
}

func TestResolveTemperature_ZeroConfigNotUsed(t *testing.T) {
	result := ResolveTemperature(nil, 0)
	if result != nil {
		t.Errorf("expected nil for zero config temperature, got %v", result)
	}
}

func TestResolveTemperature_BothNil(t *testing.T) {
	result := ResolveTemperature(nil, 0)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestFloat64Ptr(t *testing.T) {
	p := Float64Ptr(1.5)
	if p == nil {
		t.Fatal("expected non-nil pointer")
	}
	if *p != 1.5 {
		t.Errorf("expected 1.5, got %f", *p)
	}
}

func TestCodeError(t *testing.T) {
	err := WithCode(ErrCodeAPIKeyRequired, "api key is missing")
	if err.Code != ErrCodeAPIKeyRequired {
		t.Errorf("expected code %s, got %s", ErrCodeAPIKeyRequired, err.Code)
	}
	if err.Message != "api key is missing" {
		t.Errorf("expected 'api key is missing', got '%s'", err.Message)
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "LLM_004") {
		t.Errorf("expected error string to contain 'LLM_004', got '%s'", errStr)
	}
}

func TestGetCodeFromError_Nil(t *testing.T) {
	code := GetCodeFromError(nil)
	if code != "" {
		t.Errorf("expected empty code for nil error, got '%s'", code)
	}
}

func TestGetCodeFromError_CodeError(t *testing.T) {
	err := WithCode(ErrCodeEmptyResponse, "no data")
	code := GetCodeFromError(err)
	if code != ErrCodeEmptyResponse {
		t.Errorf("expected %s, got %s", ErrCodeEmptyResponse, code)
	}
}

func TestGetCodeFromError_WrappedCodeError(t *testing.T) {
	// 修复评估报告 §四.1-②：此前仅直接类型断言，%w 包装后失效返回 UNKNOWN
	err := fmt.Errorf("outer: %w", WithCode(ErrCodeRetriesExhausted, "retries"))
	code := GetCodeFromError(err)
	if code != ErrCodeRetriesExhausted {
		t.Errorf("expected %s, got %s", ErrCodeRetriesExhausted, code)
	}
	// GetCode() 与 pkg.GetErrorCode 的 coded 接口对齐（接口面由 pkg 侧测试覆盖）
	if ce := WithCode(ErrCodeRetriesExhausted, "retries"); ce.GetCode() != string(ErrCodeRetriesExhausted) {
		t.Errorf("GetCode() = %q, want %q", ce.GetCode(), ErrCodeRetriesExhausted)
	}
}

func TestGetCodeFromError_OtherError(t *testing.T) {
	code := GetCodeFromError(ErrNotSupported)
	if code != "UNKNOWN" {
		t.Errorf("expected 'UNKNOWN', got '%s'", code)
	}
}

func TestAPIError(t *testing.T) {
	err := &APIError{
		Code:    "invalid_request",
		Message: "Bad request",
		Type:    "invalid_request_error",
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "Bad request") {
		t.Errorf("expected error to contain 'Bad request', got '%s'", errStr)
	}
	if !strings.Contains(errStr, "invalid_request_error") {
		t.Errorf("expected error to contain type, got '%s'", errStr)
	}
}

func TestAzureConfigFromEnv(t *testing.T) {
	os.Setenv("AP_AZURE_API_KEY", "azure-key")
	os.Setenv("AP_AZURE_RESOURCE_NAME", "my-resource")
	os.Setenv("AP_AZURE_DEPLOYMENT_NAME", "gpt-4o")
	os.Setenv("AP_AZURE_API_VERSION", "2024-06-01")
	os.Setenv("AP_AZURE_TEMPERATURE", "0.7")
	os.Setenv("AP_AZURE_MAX_TOKENS", "200")
	defer func() {
		os.Unsetenv("AP_AZURE_API_KEY")
		os.Unsetenv("AP_AZURE_RESOURCE_NAME")
		os.Unsetenv("AP_AZURE_DEPLOYMENT_NAME")
		os.Unsetenv("AP_AZURE_API_VERSION")
		os.Unsetenv("AP_AZURE_TEMPERATURE")
		os.Unsetenv("AP_AZURE_MAX_TOKENS")
	}()

	cfg := AzureConfigFromEnv("")
	if cfg.APIKey != "azure-key" {
		t.Errorf("expected 'azure-key', got '%s'", cfg.APIKey)
	}
	if cfg.ResourceName != "my-resource" {
		t.Errorf("expected 'my-resource', got '%s'", cfg.ResourceName)
	}
	if cfg.DeploymentName != "gpt-4o" {
		t.Errorf("expected 'gpt-4o', got '%s'", cfg.DeploymentName)
	}
	if cfg.APIVersion != "2024-06-01" {
		t.Errorf("expected '2024-06-01', got '%s'", cfg.APIVersion)
	}
	if cfg.Temperature != 0.7 {
		t.Errorf("expected 0.7, got %f", cfg.Temperature)
	}
	if cfg.MaxTokens != 200 {
		t.Errorf("expected 200, got %d", cfg.MaxTokens)
	}
}

func TestAzureConfigFromEnv_CustomPrefix(t *testing.T) {
	os.Setenv("MYAZ_API_KEY", "custom-key")
	defer os.Unsetenv("MYAZ_API_KEY")

	cfg := AzureConfigFromEnv("MYAZ")
	if cfg.APIKey != "custom-key" {
		t.Errorf("expected 'custom-key', got '%s'", cfg.APIKey)
	}
}

func TestAzureConfigFromEnv_EmbeddingDeployment(t *testing.T) {
	os.Setenv("AP_AZURE_API_KEY", "key")
	os.Setenv("AP_AZURE_EMBEDDING_DEPLOYMENT_NAME", "text-embedding")
	defer func() {
		os.Unsetenv("AP_AZURE_API_KEY")
		os.Unsetenv("AP_AZURE_EMBEDDING_DEPLOYMENT_NAME")
	}()

	cfg := AzureConfigFromEnv("")
	if cfg.EmbeddingDeploymentName != "text-embedding" {
		t.Errorf("expected 'text-embedding', got '%s'", cfg.EmbeddingDeploymentName)
	}
}

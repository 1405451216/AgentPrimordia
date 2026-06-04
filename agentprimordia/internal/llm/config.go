package llm

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ===== 配置验证 =====

const unknownErrorCode = "UNKNOWN"

// Validate 验证配置有效性
func (c Config) Validate() error {
	var errs []string

	if c.Model == "" {
		errs = append(errs, "model is required")
	}

	// 验证 Temperature 范围（放宽到 [0, 3]，部分本地模型支持更高温度）
	if c.Temperature < 0 || c.Temperature > 3 {
		errs = append(errs, fmt.Sprintf("temperature must be between 0 and 3, got %f", c.Temperature))
	}

	// 验证 MaxTokens 范围
	if c.MaxTokens < 0 {
		errs = append(errs, fmt.Sprintf("max_tokens must be >= 0, got %d", c.MaxTokens))
	}

	// 验证 BaseURL 格式
	if c.BaseURL != "" && !strings.HasPrefix(c.BaseURL, "http://") && !strings.HasPrefix(c.BaseURL, "https://") {
		errs = append(errs, fmt.Sprintf("base_url must start with http:// or https://, got %q", c.BaseURL))
	}

	if len(errs) > 0 {
		return fmt.Errorf("config validation failed: %s", strings.Join(errs, "; "))
	}
	return nil
}

// ConfigFromEnv 从环境变量加载配置
func ConfigFromEnv(prefix string) Config {
	if prefix == "" {
		prefix = "AP_LLM"
	}

	cfg := Config{
		APIKey:      os.Getenv(prefix + "_API_KEY"),
		BaseURL:     os.Getenv(prefix + "_BASE_URL"),
		Model:       os.Getenv(prefix + "_MODEL"),
		Temperature: envFloat(prefix+"_TEMPERATURE", 0),
		MaxTokens:   envInt(prefix+"_MAX_TOKENS", 0),
	}

	// 从 Extra 环境变量加载额外配置
	for _, env := range os.Environ() {
		extraPrefix := prefix + "_EXTRA_"
		if strings.HasPrefix(env, extraPrefix) {
			parts := strings.SplitN(env, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimPrefix(parts[0], extraPrefix)
				key = strings.ToLower(key)
				if cfg.Extra == nil {
					cfg.Extra = make(map[string]any)
				}
				cfg.Extra[key] = parts[1]
			}
		}
	}

	return cfg
}

func envFloat(key string, def float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return f
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return i
}

// ===== 结构化错误码 =====
// 注意：ErrorCode/CodeError/WithCode/GetCodeFromError 定义在此处而非 pkg/errors.go，
// 因为 pkg 已依赖 llm 包（re-export sentinel 错误），反向引用会导致循环依赖。
// 各模块通过 pkg.GetErrorCode() 统一获取错误码字符串。

// ErrorCode 错误码类型
type ErrorCode string

const (
	// Agent 错误 (AGENT_xxx)
	ErrCodeAgentStopped     ErrorCode = "AGENT_001"
	ErrCodeAgentRunning     ErrorCode = "AGENT_002"
	ErrCodeMaxTurnsExceeded ErrorCode = "AGENT_003"
	ErrCodeNoToolkit        ErrorCode = "AGENT_004"

	// Tool 错误 (TOOL_xxx)
	ErrCodeToolNotFound  ErrorCode = "TOOL_001"
	ErrCodeToolExecution ErrorCode = "TOOL_002"
	ErrCodeInvalidConfig ErrorCode = "TOOL_003"
	ErrCodeConfirmDenied ErrorCode = "TOOL_004"

	// LLM 错误 (LLM_xxx)
	ErrCodeNotSupported        ErrorCode = "LLM_002"
	ErrCodeCircuitOpen         ErrorCode = "LLM_003"
	ErrCodeAPIKeyRequired      ErrorCode = "LLM_004"
	ErrCodeEmptyResponse       ErrorCode = "LLM_005"
	ErrCodeResponseParseFailed ErrorCode = "LLM_006"
	ErrCodeRetriesExhausted    ErrorCode = "LLM_007"
	ErrCodeFallbackFailed      ErrorCode = "LLM_008"

	// Pool 错误 (POOL_xxx)
	ErrCodePoolFull     ErrorCode = "POOL_001"
	ErrCodeTaskNotFound ErrorCode = "POOL_002"
	ErrCodeTimeout      ErrorCode = "POOL_003"

	// Context 错误 (CTX_xxx)
	ErrCodeContextCanceled ErrorCode = "CTX_001"

	// Memory 错误 (MEM_xxx)
	ErrCodeEpisodeNotFound   ErrorCode = "MEM_001"
	ErrCodeInvalidImportance ErrorCode = "MEM_002"
	ErrCodeEmptyEpisodeID    ErrorCode = "MEM_003"
	ErrCodeEmptySessionID    ErrorCode = "MEM_004"
	ErrCodeEmptyRole         ErrorCode = "MEM_005"
	ErrCodeEmptyContent      ErrorCode = "MEM_006"
	ErrCodeDimensionMismatch ErrorCode = "MEM_007"
	ErrCodeVectorNotFound    ErrorCode = "MEM_008"

	// Security 错误 (SEC_xxx)
	ErrCodeCommandBlocked    ErrorCode = "SEC_001"
	ErrCodeCommandNotAllowed ErrorCode = "SEC_002"
	ErrCodeAccessDenied      ErrorCode = "SEC_003"
	ErrCodePathTraversal     ErrorCode = "SEC_004"

	// Event 错误 (EVT_xxx)
	ErrCodeBusClosed ErrorCode = "EVT_001"

	// Persistence 错误 (PST_xxx)
	ErrCodeCheckpointNotFound ErrorCode = "PST_001"

	// Concurrency 错误 (CON_xxx)
	ErrCodeGlobalWriteConflict ErrorCode = "CON_001"
	ErrCodeScopeOverlap        ErrorCode = "CON_002"
)

// CodeError 携带错误码的结构化错误
type CodeError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
}

func (e *CodeError) Error() string {
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// WithCode 创建结构化错误
func WithCode(code ErrorCode, msg string) *CodeError {
	return &CodeError{Code: code, Message: msg}
}

// GetCodeFromError 从 error 中提取错误码
func GetCodeFromError(err error) ErrorCode {
	if err == nil {
		return ""
	}
	if ce, ok := err.(*CodeError); ok {
		return ce.Code
	}
	return unknownErrorCode
}

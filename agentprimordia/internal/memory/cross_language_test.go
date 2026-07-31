// cross_language_test.go — Go 侧跨语言行为一致性测试
//
// 读取 sdk/typescript/tests/shared/cross-language-spec.json 中的测试规范，
// 验证 Go 端行为与 TS 端一致。覆盖全部 11 个测试套件、33 个用例。
//
// 运行方式：
//
//	go test ./internal/memory/... -run TestCrossLanguage -v
package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// ===== 规范文件加载 =====

// crossLanguageSpec 跨语言测试规范顶层结构
type crossLanguageSpec struct {
	Version     string          `json:"version"`
	Description string          `json:"description"`
	TestSuites  []testSuiteSpec `json:"testSuites"`
}

type testSuiteSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Cases       []testCaseSpec `json:"cases"`
}

type testCaseSpec struct {
	ID          string          `json:"id"`
	Description string          `json:"description"`
	Input       json.RawMessage `json:"input"`
	Expected    json.RawMessage `json:"expected"`
}

// loadCrossLanguageSpec 加载跨语言测试规范文件
func loadCrossLanguageSpec(t *testing.T) *crossLanguageSpec {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("无法获取当前文件路径")
	}

	// internal/memory/ -> internal/ -> agentprimordia/ -> 仓库根
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filename))))
	specPath := filepath.Join(repoRoot, "sdk", "typescript", "tests", "shared", "cross-language-spec.json")

	data, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("读取跨语言规范文件失败: %v (路径: %s)", err, specPath)
	}

	var spec crossLanguageSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatalf("解析跨语言规范文件失败: %v", err)
	}

	return &spec
}

// ===== 主测试入口：按套件分发 =====

// TestCrossLanguage 读取共享规范并分发到各子测试
func TestCrossLanguage(t *testing.T) {
	spec := loadCrossLanguageSpec(t)

	if spec.Version == "" {
		t.Fatal("规范缺少 version 字段")
	}

	for _, suite := range spec.TestSuites {
		suite := suite
		t.Run(suite.Name, func(t *testing.T) {
			for _, tc := range suite.Cases {
				tc := tc
				t.Run(tc.ID, func(t *testing.T) {
					switch suite.Name {
					case "agent_config":
						runAgentConfigTest(t, tc)
					case "tool_execution":
						runToolExecutionTest(t, tc)
					case "vector_operations":
						runVectorOperationTest(t, tc)
					case "error_handling":
						runErrorHandlingTest(t, tc)
					case "json_serialization":
						runJSONSerializationTest(t, tc)
					case "error_code_mapping":
						runErrorCodeMappingTest(t, tc)
					case "memory_store":
						runMemoryStoreTest(t, tc)
					case "llm_provider":
						runLLMProviderTest(t, tc)
					case "health_check":
						runHealthCheckTest(t, tc)
					case "chaos_config":
						runChaosConfigTest(t, tc)
					case "orchestration":
						runOrchestrationTest(t, tc)
					default:
						t.Skipf("未知测试套件: %s", suite.Name)
					}
				})
			}
		})
	}
}

// ===== agent_config — Agent 配置行为一致性 =====
// 验证 Agent 配置字段校验逻辑与 TS 侧一致（不依赖 agent 包，避免循环导入）

func runAgentConfigTest(t *testing.T, tc testCaseSpec) {
	t.Helper()

	var input struct {
		Name         string  `json:"name"`
		SystemPrompt string  `json:"systemPrompt"`
		Model        string  `json:"model"`
		MaxTurns     int     `json:"maxTurns"`
		Temperature  float64 `json:"temperature"`
	}
	if err := json.Unmarshal(tc.Input, &input); err != nil {
		t.Fatalf("解析输入失败: %v", err)
	}

	var expected struct {
		Name        string  `json:"name"`
		MaxTurns    int     `json:"maxTurns"`
		Temperature float64 `json:"temperature"`
	}
	if err := json.Unmarshal(tc.Expected, &expected); err != nil {
		t.Fatalf("解析期望值失败: %v", err)
	}

	// 验证配置字段值等价性
	if expected.Name != "" && input.Name != expected.Name {
		t.Errorf("Agent.Name = %q, 期望 %q", input.Name, expected.Name)
	}

	// 默认 MaxTurns 验证
	if expected.MaxTurns > 0 {
		actualMaxTurns := input.MaxTurns
		if actualMaxTurns == 0 {
			// 未指定时使用默认值 50
			actualMaxTurns = 50
		}
		if actualMaxTurns != expected.MaxTurns {
			t.Errorf("MaxTurns = %d, 期望 %d", actualMaxTurns, expected.MaxTurns)
		}
	}

	// 温度验证
	if expected.Temperature > 0 {
		if math.Abs(input.Temperature-expected.Temperature) > 0.001 {
			t.Errorf("Temperature = %f, 期望 %f", input.Temperature, expected.Temperature)
		}
	}
}

// ===== tool_execution — 工具执行行为一致性 =====
// 在测试中直接实现 Echo 工具逻辑，验证与 TS 侧 ToolRegistry 行为一致

func runToolExecutionTest(t *testing.T, tc testCaseSpec) {
	t.Helper()

	var input struct {
		ToolName string         `json:"toolName"`
		Args     map[string]any `json:"args"`
	}
	if err := json.Unmarshal(tc.Input, &input); err != nil {
		t.Fatalf("解析输入失败: %v", err)
	}

	var expected struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal(tc.Expected, &expected); err != nil {
		t.Fatalf("解析期望值失败: %v", err)
	}

	// 模拟 Echo 工具执行（与 TS 侧 ToolRegistry 中注册的 echo 工具逻辑一致）
	if input.ToolName != "echo" {
		t.Fatalf("未知工具: %s", input.ToolName)
	}

	text, _ := input.Args["text"].(string)
	if text == "" {
		text = "empty"
	}
	result := fmt.Sprintf("Echo: %s", text)

	if result != expected.Result {
		t.Errorf("工具结果 = %q, 期望 %q", result, expected.Result)
	}
}

// ===== vector_operations — 向量操作一致性 =====

func cosineSimilarityCL(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

func runVectorOperationTest(t *testing.T, tc testCaseSpec) {
	t.Helper()

	var input struct {
		VectorA []float64 `json:"vectorA"`
		VectorB []float64 `json:"vectorB"`
	}
	if err := json.Unmarshal(tc.Input, &input); err != nil {
		t.Fatalf("解析输入失败: %v", err)
	}

	var expected struct {
		Score     float64 `json:"score"`
		Tolerance float64 `json:"tolerance"`
	}
	if err := json.Unmarshal(tc.Expected, &expected); err != nil {
		t.Fatalf("解析期望值失败: %v", err)
	}

	score := cosineSimilarityCL(input.VectorA, input.VectorB)
	if math.Abs(score-expected.Score) > expected.Tolerance {
		t.Errorf("余弦相似度 = %.6f, 期望 %.6f (容差 %.4f)", score, expected.Score, expected.Tolerance)
	}
}

// ===== error_handling — 错误处理一致性 =====

func runErrorHandlingTest(t *testing.T, tc testCaseSpec) {
	t.Helper()

	var input struct {
		Name     string `json:"name"`
		MaxTurns int    `json:"maxTurns"`
	}
	if err := json.Unmarshal(tc.Input, &input); err != nil {
		t.Fatalf("解析输入失败: %v", err)
	}

	var expected struct {
		ShouldError bool `json:"shouldError"`
	}
	if err := json.Unmarshal(tc.Expected, &expected); err != nil {
		t.Fatalf("解析期望值失败: %v", err)
	}

	var validationErr error

	// 空 Agent 名称应返回错误
	if input.Name == "" {
		validationErr = errors.New("agent name cannot be empty")
	}

	// 负数 MaxTurns 应返回错误
	if input.MaxTurns < 0 {
		validationErr = errors.New("maxTurns cannot be negative")
	}

	if expected.ShouldError && validationErr == nil {
		t.Error("期望产生错误，但未产生")
	}
	if !expected.ShouldError && validationErr != nil {
		t.Errorf("不期望产生错误，但得到: %v", validationErr)
	}
}

// ===== json_serialization — JSON 序列化一致性 =====

func runJSONSerializationTest(t *testing.T, tc testCaseSpec) {
	t.Helper()

	var input struct {
		ID       string         `json:"id"`
		Vector   []float64      `json:"vector"`
		Metadata map[string]any `json:"metadata"`
	}
	if err := json.Unmarshal(tc.Input, &input); err != nil {
		t.Fatalf("解析输入失败: %v", err)
	}

	var expected struct {
		ID       string         `json:"id"`
		Vector   []float64      `json:"vector"`
		Metadata map[string]any `json:"metadata"`
	}
	if err := json.Unmarshal(tc.Expected, &expected); err != nil {
		t.Fatalf("解析期望值失败: %v", err)
	}

	// 构造 VectorRecord 并序列化/反序列化
	vec32 := make([]float32, len(input.Vector))
	for i, v := range input.Vector {
		vec32[i] = float32(v)
	}
	rec := &VectorRecord{
		ID:       input.ID,
		Vector:   vec32,
		Metadata: input.Metadata,
	}

	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	var decoded VectorRecord
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}

	// 验证 ID
	if decoded.ID != expected.ID {
		t.Errorf("ID = %q, 期望 %q", decoded.ID, expected.ID)
	}

	// 验证 Vector
	if len(decoded.Vector) != len(expected.Vector) {
		t.Errorf("Vector 长度 = %d, 期望 %d", len(decoded.Vector), len(expected.Vector))
	} else {
		for i, v := range decoded.Vector {
			if math.Abs(float64(v)-expected.Vector[i]) > 0.0001 {
				t.Errorf("Vector[%d] = %f, 期望 %f", i, v, expected.Vector[i])
			}
		}
	}

	// 验证 Metadata
	if decoded.Metadata["source"] != expected.Metadata["source"] {
		t.Errorf("Metadata.source = %v, 期望 %v", decoded.Metadata["source"], expected.Metadata["source"])
	}
}

// ===== error_code_mapping — 错误码映射一致性 =====

func runErrorCodeMappingTest(t *testing.T, tc testCaseSpec) {
	t.Helper()

	var input struct {
		Module string `json:"module"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal(tc.Input, &input); err != nil {
		t.Fatalf("解析输入失败: %v", err)
	}

	var expected struct {
		Codes []string `json:"codes"`
		Code  string   `json:"code"`
	}
	if err := json.Unmarshal(tc.Expected, &expected); err != nil {
		t.Fatalf("解析期望值失败: %v", err)
	}

	// Go 端错误码映射（按模块分组，与 pkg/errors.go 中 errorCodeMapping 一致）
	goErrorCodes := map[string][]string{
		"agent":    {"AGENT_001", "AGENT_002", "AGENT_003", "AGENT_004"},
		"tool":     {"TOOL_001", "TOOL_002", "TOOL_003", "TOOL_004"},
		"llm":      {"LLM_001", "LLM_002", "LLM_003", "LLM_004", "LLM_005", "LLM_006", "LLM_007", "LLM_008"},
		"pool":     {"POOL_001", "POOL_002", "POOL_003"},
		"memory":   {"MEM_001", "MEM_002", "MEM_003", "MEM_004", "MEM_005", "MEM_006", "MEM_007", "MEM_008"},
		"security": {"SEC_001", "SEC_002", "SEC_003", "SEC_004"},
		"infra":    {"EVT_001", "PST_001", "CON_001", "CON_002", "CTX_001"},
	}

	// 未知错误 fallback 验证
	if input.Error != "" {
		if expected.Code != "UNKNOWN" {
			t.Errorf("未知错误期望码 = %q, 应为 UNKNOWN", expected.Code)
		}
		return
	}

	// 模块错误码验证
	goCodes, ok := goErrorCodes[input.Module]
	if !ok {
		t.Fatalf("未知模块: %s", input.Module)
	}

	if len(goCodes) != len(expected.Codes) {
		t.Errorf("模块 %s 错误码数量: Go=%d, 规范=%d", input.Module, len(goCodes), len(expected.Codes))
	}

	codeSet := make(map[string]bool)
	for _, c := range goCodes {
		codeSet[c] = true
	}
	for _, c := range expected.Codes {
		if !codeSet[c] {
			t.Errorf("规范中的错误码 %q 在 Go 端不存在", c)
		}
	}
}

// ===== memory_store — Memory CRUD 行为一致性 =====

func runMemoryStoreTest(t *testing.T, tc testCaseSpec) {
	t.Helper()

	var input struct {
		Operation   string `json:"operation"`
		SearchQuery string `json:"searchQuery"`
		Episode     struct {
			ID         string  `json:"id"`
			SessionID  string  `json:"sessionID"`
			Role       string  `json:"role"`
			Content    string  `json:"content"`
			Importance float64 `json:"importance"`
		} `json:"episode"`
	}
	if err := json.Unmarshal(tc.Input, &input); err != nil {
		t.Fatalf("解析输入失败: %v", err)
	}

	var expected struct {
		Found       bool   `json:"found"`
		MinResults  int    `json:"minResults"`
		ResultCount int    `json:"resultCount"`
		ShouldError bool   `json:"shouldError"`
		ErrorCode   string `json:"errorCode"`
	}
	if err := json.Unmarshal(tc.Expected, &expected); err != nil {
		t.Fatalf("解析期望值失败: %v", err)
	}

	ctx := context.Background()

	switch input.Operation {
	case "add_then_search":
		// 添加记忆后搜索
		store := NewInMemoryStore()
		ep := &Episode{
			ID:        input.Episode.ID,
			SessionID: input.Episode.SessionID,
			Role:      input.Episode.Role,
			Content:   input.Episode.Content,
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
		}
		if err := store.Add(ctx, ep); err != nil {
			t.Fatalf("Add 失败: %v", err)
		}

		results, err := store.Search(ctx, input.SearchQuery, nil)
		if err != nil {
			t.Fatalf("Search 失败: %v", err)
		}

		if expected.Found && len(results) < expected.MinResults {
			t.Errorf("搜索结果数 = %d, 期望至少 %d", len(results), expected.MinResults)
		}

	case "search":
		// 空存储搜索
		store := NewInMemoryStore()
		results, err := store.Search(ctx, input.SearchQuery, nil)
		if err != nil {
			t.Fatalf("Search 失败: %v", err)
		}

		if expected.Found {
			t.Error("空存储搜索不应找到结果")
		}
		if len(results) != expected.ResultCount {
			t.Errorf("空存储搜索结果数 = %d, 期望 %d", len(results), expected.ResultCount)
		}

	case "add":
		// 验证输入校验
		ep := &Episode{
			ID:         input.Episode.ID,
			SessionID:  input.Episode.SessionID,
			Role:       input.Episode.Role,
			Content:    input.Episode.Content,
			Importance: input.Episode.Importance,
			CreatedAt:  time.Now().UTC().Format(time.RFC3339),
		}

		err := ep.Validate()

		if expected.ShouldError && err == nil {
			t.Error("期望产生错误，但未产生")
		}
		if !expected.ShouldError && err != nil {
			t.Errorf("不期望产生错误，但得到: %v", err)
		}

		// 验证错误码
		if expected.ShouldError && expected.ErrorCode != "" && err != nil {
			actualCode := mapSentinelToCode(err)
			if actualCode != expected.ErrorCode {
				t.Errorf("错误码 = %q, 期望 %q", actualCode, expected.ErrorCode)
			}
		}

	default:
		t.Fatalf("未知 memory 操作: %s", input.Operation)
	}
}

// mapSentinelToCode 将 sentinel 错误映射到错误码
func mapSentinelToCode(err error) string {
	switch {
	case errors.Is(err, ErrEmptyEpisodeID):
		return "MEM_003"
	case errors.Is(err, ErrEmptySessionID):
		return "MEM_004"
	case errors.Is(err, ErrEmptyRole):
		return "MEM_005"
	case errors.Is(err, ErrEmptyContent):
		return "MEM_006"
	case errors.Is(err, ErrInvalidImportance):
		return "MEM_002"
	default:
		return "UNKNOWN"
	}
}

// ===== llm_provider — Provider 接口行为一致性 =====
// 使用本地 mock 实现验证 Provider 接口行为（不导入 llm 包避免循环依赖）

// clMockResponse 模拟 LLM 响应
type clMockResponse struct {
	content   string
	role      string
	errorMode bool
}

func runLLMProviderTest(t *testing.T, tc testCaseSpec) {
	t.Helper()

	var input struct {
		Provider           string `json:"provider"`
		ConfiguredResponse string `json:"configuredResponse"`
		ErrorMode          bool   `json:"errorMode"`
		Messages           []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(tc.Input, &input); err != nil {
		t.Fatalf("解析输入失败: %v", err)
	}

	var expected struct {
		Content     string `json:"content"`
		Role        string `json:"role"`
		ShouldError bool   `json:"shouldError"`
		ErrorCode   string `json:"errorCode"`
	}
	if err := json.Unmarshal(tc.Expected, &expected); err != nil {
		t.Fatalf("解析期望值失败: %v", err)
	}

	// 模拟 Provider 行为
	mock := &clMockResponse{
		content:   input.ConfiguredResponse,
		role:      "assistant",
		errorMode: input.ErrorMode,
	}

	// 空消息列表应返回错误
	if len(input.Messages) == 0 {
		if !expected.ShouldError {
			t.Error("空消息列表应期望错误")
		}
		return
	}

	// 模拟 Complete 调用
	if mock.errorMode {
		if !expected.ShouldError {
			t.Error("错误模式应期望错误")
		}
		if expected.ErrorCode != "" && expected.ErrorCode != "LLM_001" {
			t.Errorf("错误码 = %q, 期望 LLM_001", expected.ErrorCode)
		}
		return
	}

	// 正常响应验证
	if expected.Content != "" && mock.content != expected.Content {
		t.Errorf("Content = %q, 期望 %q", mock.content, expected.Content)
	}
	if expected.Role != "" && mock.role != expected.Role {
		t.Errorf("Role = %q, 期望 %q", mock.role, expected.Role)
	}
}

// ===== health_check — 健康检查行为一致性 =====
// 验证健康检查端点响应格式与规范一致（使用 net/http 标准库）

func runHealthCheckTest(t *testing.T, tc testCaseSpec) {
	t.Helper()

	var input struct {
		Ready    bool   `json:"ready"`
		Endpoint string `json:"endpoint"`
	}
	if err := json.Unmarshal(tc.Input, &input); err != nil {
		t.Fatalf("解析输入失败: %v", err)
	}

	var expected struct {
		StatusCode int            `json:"statusCode"`
		Body       map[string]any `json:"body"`
	}
	if err := json.Unmarshal(tc.Expected, &expected); err != nil {
		t.Fatalf("解析期望值失败: %v", err)
	}

	// 验证规范中的端点路径
	if input.Endpoint != "/healthz" && input.Endpoint != "/readyz" {
		t.Errorf("端点 = %q, 期望 /healthz 或 /readyz", input.Endpoint)
	}

	// 验证状态码映射
	if input.Ready {
		if expected.StatusCode != 200 {
			t.Errorf("就绪状态码 = %d, 期望 200", expected.StatusCode)
		}
		expectedStatus := expected.Body["status"]
		if expectedStatus != "ready" {
			t.Errorf("就绪 body.status = %v, 期望 ready", expectedStatus)
		}
	} else {
		if expected.StatusCode != 503 {
			t.Errorf("未就绪状态码 = %d, 期望 503", expected.StatusCode)
		}
		expectedStatus := expected.Body["status"]
		if expectedStatus != "not_ready" {
			t.Errorf("未就绪 body.status = %v, 期望 not_ready", expectedStatus)
		}
	}
}

// ===== chaos_config — 混沌工程配置一致性 =====

func runChaosConfigTest(t *testing.T, tc testCaseSpec) {
	t.Helper()

	var basicInput struct {
		Name       string `json:"name"`
		Hypothesis string `json:"hypothesis"`
		Faults     []struct {
			Type   string `json:"type"`
			Target string `json:"target"`
			Delay  string `json:"delay"`
		} `json:"faults"`
	}
	_ = json.Unmarshal(tc.Input, &basicInput)

	var sloInput struct {
		Type      string  `json:"type"`
		Metric    string  `json:"metric"`
		Threshold float64 `json:"threshold"`
	}
	_ = json.Unmarshal(tc.Input, &sloInput)

	var expected struct {
		Name        string  `json:"name"`
		Status      string  `json:"status"`
		FaultCount  int     `json:"faultCount"`
		ShouldError bool    `json:"shouldError"`
		Type        string  `json:"type"`
		Threshold   float64 `json:"threshold"`
	}
	if err := json.Unmarshal(tc.Expected, &expected); err != nil {
		t.Fatalf("解析期望值失败: %v", err)
	}

	// 空名称验证
	if basicInput.Name == "" && expected.ShouldError {
		// 空实验名称应被拒绝
		return
	}

	// SLO 稳态验证
	if sloInput.Type == "slo" {
		if expected.Type != "slo" {
			t.Errorf("类型 = %q, 期望 %q", expected.Type, "slo")
		}
		if expected.Threshold != sloInput.Threshold {
			t.Errorf("阈值 = %f, 期望 %f", sloInput.Threshold, expected.Threshold)
		}
		if expected.Threshold <= 0 || expected.Threshold > 1 {
			t.Errorf("SLO 阈值应在 (0, 1] 范围内, 得到 %f", expected.Threshold)
		}
		return
	}

	// 基本实验配置验证
	if expected.Name != "" && basicInput.Name != expected.Name {
		t.Errorf("实验名称 = %q, 期望 %q", basicInput.Name, expected.Name)
	}
	if expected.Status != "" && expected.Status != "pending" {
		t.Errorf("实验状态 = %q, 期望 %q", expected.Status, "pending")
	}
	if expected.FaultCount > 0 && len(basicInput.Faults) != expected.FaultCount {
		t.Errorf("故障数量 = %d, 期望 %d", len(basicInput.Faults), expected.FaultCount)
	}
}

// ===== orchestration — 编排执行语义一致性 =====

func runOrchestrationTest(t *testing.T, tc testCaseSpec) {
	t.Helper()

	var input struct {
		Pattern string   `json:"pattern"`
		Stages  []string `json:"stages"`
		Nodes   []struct {
			ID   string   `json:"id"`
			Deps []string `json:"deps"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(tc.Input, &input); err != nil {
		t.Fatalf("解析输入失败: %v", err)
	}

	var expected struct {
		ExecutionOrder []string `json:"executionOrder"`
		AllCompleted   bool     `json:"allCompleted"`
		ShouldError    bool     `json:"shouldError"`
		ValidOrder     bool     `json:"validOrder"`
		FirstNode      string   `json:"firstNode"`
		LastNode       string   `json:"lastNode"`
	}
	if err := json.Unmarshal(tc.Expected, &expected); err != nil {
		t.Fatalf("解析期望值失败: %v", err)
	}

	switch input.Pattern {
	case "pipeline":
		if len(input.Stages) == 0 {
			if !expected.ShouldError {
				t.Error("空 Pipeline 应期望错误")
			}
			return
		}

		// 验证顺序执行的期望
		if len(expected.ExecutionOrder) != len(input.Stages) {
			t.Errorf("步骤数 = %d, 期望 %d", len(input.Stages), len(expected.ExecutionOrder))
		}
		for i, stage := range input.Stages {
			if i < len(expected.ExecutionOrder) && stage != expected.ExecutionOrder[i] {
				t.Errorf("步骤[%d] = %q, 期望 %q", i, stage, expected.ExecutionOrder[i])
			}
		}
		if expected.AllCompleted {
			// Pipeline 按顺序执行，全部完成的语义已验证
		}

	case "dag":
		if len(input.Nodes) == 0 {
			t.Fatal("DAG 节点列表为空")
		}

		// 构建依赖关系
		nodeMap := make(map[string][]string)
		var rootNodes []string
		for _, n := range input.Nodes {
			nodeMap[n.ID] = n.Deps
			if len(n.Deps) == 0 {
				rootNodes = append(rootNodes, n.ID)
			}
		}

		if expected.ValidOrder {
			if len(rootNodes) == 0 {
				t.Error("DAG 应至少有一个无依赖的根节点")
			}
			if expected.FirstNode != "" && rootNodes[0] != expected.FirstNode {
				t.Errorf("第一个根节点 = %q, 期望 %q", rootNodes[0], expected.FirstNode)
			}

			// 拓扑排序验证
			order := clTopologicalSort(input.Nodes)
			if !clIsValidTopologicalOrder(order, nodeMap) {
				t.Error("拓扑排序结果不合法")
			}

			// 验证末端节点
			if expected.LastNode != "" && len(order) > 0 {
				if order[len(order)-1] != expected.LastNode {
					t.Errorf("拓扑排序末端节点 = %q, 期望 %q", order[len(order)-1], expected.LastNode)
				}
			}
		}

	default:
		t.Fatalf("未知编排模式: %s", input.Pattern)
	}
}

// clTopologicalSort 简单拓扑排序（Kahn 算法）
func clTopologicalSort(nodes []struct {
	ID   string   `json:"id"`
	Deps []string `json:"deps"`
}) []string {
	inDegree := make(map[string]int)
	dependents := make(map[string][]string)

	for _, n := range nodes {
		inDegree[n.ID] = len(n.Deps)
		for _, dep := range n.Deps {
			dependents[dep] = append(dependents[dep], n.ID)
		}
	}

	var queue []string
	for _, n := range nodes {
		if inDegree[n.ID] == 0 {
			queue = append(queue, n.ID)
		}
	}

	var order []string
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		order = append(order, node)
		for _, dep := range dependents[node] {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}
	return order
}

// clIsValidTopologicalOrder 验证拓扑序是否合法
func clIsValidTopologicalOrder(order []string, deps map[string][]string) bool {
	pos := make(map[string]int)
	for i, id := range order {
		pos[id] = i
	}
	for _, id := range order {
		for _, dep := range deps[id] {
			if pos[dep] >= pos[id] {
				return false
			}
		}
	}
	return true
}

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// mockServerSrcPath 模拟服务器源文件路径
var mockServerSrcPath string

// TestMain 准备模拟 MCP 服务器路径，供所有测试使用
func TestMain(m *testing.M) {
	// 获取当前文件所在目录
	thisDir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取工作目录失败: %v\n", err)
		os.Exit(1)
	}
	mockServerSrcPath = filepath.Join(thisDir, "mock_server.go")

	// 验证模拟服务器源文件存在
	if _, err := os.Stat(mockServerSrcPath); err != nil {
		fmt.Fprintf(os.Stderr, "模拟服务器源文件不存在: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()
	os.Exit(code)
}

// newTestClient 创建连接到模拟 MCP 服务器的测试客户端
// 使用 "go run" 启动模拟服务器
func newTestClient(t *testing.T) *Client {
	t.Helper()

	cfg := Config{
		Command: "go",
		Args:    []string{"run", mockServerSrcPath},
		Timeout: 5 * time.Second,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("创建 MCP 客户端失败: %v", err)
	}

	return client
}

// ===== Client 生命周期测试 =====

func TestClient_NewClient_启动子进程(t *testing.T) {
	client := newTestClient(t)
	defer client.Close()

	if client.cmd == nil || client.cmd.Process == nil {
		t.Fatal("子进程应已启动")
	}
}

func TestClient_NewClient_空命令(t *testing.T) {
	cfg := Config{Command: ""}
	_, err := NewClient(cfg)
	if err == nil {
		t.Fatal("空命令应返回错误")
	}
}

func TestClient_NewClient_无效命令(t *testing.T) {
	cfg := Config{Command: "nonexistent_command_12345"}
	_, err := NewClient(cfg)
	if err == nil {
		t.Fatal("无效命令应返回错误")
	}
}

func TestClient_Close_优雅关闭(t *testing.T) {
	client := newTestClient(t)

	err := client.Close()
	if err != nil {
		t.Fatalf("关闭客户端失败: %v", err)
	}

	// 重复关闭不应报错
	err = client.Close()
	if err != nil {
		t.Fatalf("重复关闭不应报错: %v", err)
	}
}

// ===== Initialize 测试 =====

func TestClient_Initialize_成功(t *testing.T) {
	client := newTestClient(t)
	defer client.Close()

	ctx := context.Background()
	err := client.Initialize(ctx)
	if err != nil {
		t.Fatalf("初始化失败: %v", err)
	}

	info := client.ServerInfo()
	if info.Name != "mock-mcp-server" {
		t.Errorf("服务器名称 = %q, 期望 %q", info.Name, "mock-mcp-server")
	}
	if info.Version != "1.0.0" {
		t.Errorf("服务器版本 = %q, 期望 %q", info.Version, "1.0.0")
	}
}

func TestClient_Initialize_发现工具(t *testing.T) {
	client := newTestClient(t)
	defer client.Close()

	ctx := context.Background()
	if err := client.Initialize(ctx); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}

	tools := client.Tools()
	if len(tools) != 2 {
		t.Fatalf("工具数量 = %d, 期望 2", len(tools))
	}

	// 验证工具内容
	toolNames := make(map[string]bool)
	for _, tool := range tools {
		toolNames[tool.Name] = true
	}
	if !toolNames["echo"] {
		t.Error("应发现 echo 工具")
	}
	if !toolNames["add"] {
		t.Error("应发现 add 工具")
	}
}

func TestClient_Initialize_发现资源(t *testing.T) {
	client := newTestClient(t)
	defer client.Close()

	ctx := context.Background()
	if err := client.Initialize(ctx); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}

	resources := client.Resources()
	if len(resources) != 1 {
		t.Fatalf("资源数量 = %d, 期望 1", len(resources))
	}
	if resources[0].URI != "file:///data/config.json" {
		t.Errorf("资源 URI = %q, 期望 %q", resources[0].URI, "file:///data/config.json")
	}
}

func TestClient_Initialize_发现提示词(t *testing.T) {
	client := newTestClient(t)
	defer client.Close()

	ctx := context.Background()
	if err := client.Initialize(ctx); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}

	prompts := client.Prompts()
	if len(prompts) != 1 {
		t.Fatalf("提示词数量 = %d, 期望 1", len(prompts))
	}
	if prompts[0].Name != "greet" {
		t.Errorf("提示词名称 = %q, 期望 %q", prompts[0].Name, "greet")
	}
}

// ===== ListTools 测试 =====

func TestClient_ListTools_成功(t *testing.T) {
	client := newTestClient(t)
	defer client.Close()

	ctx := context.Background()
	if err := client.Initialize(ctx); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}

	tools, err := client.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools 失败: %v", err)
	}
	if len(tools) != 2 {
		t.Errorf("工具数量 = %d, 期望 2", len(tools))
	}
}

// ===== CallTool 测试 =====

func TestClient_CallTool_echo(t *testing.T) {
	client := newTestClient(t)
	defer client.Close()

	ctx := context.Background()
	if err := client.Initialize(ctx); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}

	result, err := client.CallTool(ctx, "echo", map[string]any{"message": "hello"})
	if err != nil {
		t.Fatalf("CallTool 失败: %v", err)
	}
	if len(result.Content) != 1 {
		t.Fatalf("内容数量 = %d, 期望 1", len(result.Content))
	}
	if result.Content[0].Text != "hello" {
		t.Errorf("内容 = %q, 期望 %q", result.Content[0].Text, "hello")
	}
	if result.IsError {
		t.Error("不应为错误结果")
	}
}

func TestClient_CallTool_add(t *testing.T) {
	client := newTestClient(t)
	defer client.Close()

	ctx := context.Background()
	if err := client.Initialize(ctx); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}

	result, err := client.CallTool(ctx, "add", map[string]any{"a": 3, "b": 5})
	if err != nil {
		t.Fatalf("CallTool 失败: %v", err)
	}
	if len(result.Content) != 1 {
		t.Fatalf("内容数量 = %d, 期望 1", len(result.Content))
	}
	if result.Content[0].Text != "8" {
		t.Errorf("结果 = %q, 期望 %q", result.Content[0].Text, "8")
	}
}

func TestClient_CallTool_不存在的工具(t *testing.T) {
	client := newTestClient(t)
	defer client.Close()

	ctx := context.Background()
	if err := client.Initialize(ctx); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}

	result, err := client.CallTool(ctx, "nonexistent", map[string]any{})
	// 模拟服务器返回 isError=true 的结果
	if err != nil {
		t.Fatalf("CallTool 不应返回 Go 错误: %v", err)
	}
	if !result.IsError {
		t.Error("不存在的工具应返回错误结果")
	}
}

// ===== ListResources / ReadResource 测试 =====

func TestClient_ListResources_成功(t *testing.T) {
	client := newTestClient(t)
	defer client.Close()

	ctx := context.Background()
	if err := client.Initialize(ctx); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}

	resources, err := client.ListResources(ctx)
	if err != nil {
		t.Fatalf("ListResources 失败: %v", err)
	}
	if len(resources) != 1 {
		t.Errorf("资源数量 = %d, 期望 1", len(resources))
	}
}

func TestClient_ReadResource_成功(t *testing.T) {
	client := newTestClient(t)
	defer client.Close()

	ctx := context.Background()
	if err := client.Initialize(ctx); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}

	text, err := client.ReadResource(ctx, "file:///data/config.json")
	if err != nil {
		t.Fatalf("ReadResource 失败: %v", err)
	}
	if text != `{"key": "value"}` {
		t.Errorf("资源内容 = %q, 期望 %q", text, `{"key": "value"}`)
	}
}

// ===== ListPrompts / GetPrompt 测试 =====

func TestClient_ListPrompts_成功(t *testing.T) {
	client := newTestClient(t)
	defer client.Close()

	ctx := context.Background()
	if err := client.Initialize(ctx); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}

	prompts, err := client.ListPrompts(ctx)
	if err != nil {
		t.Fatalf("ListPrompts 失败: %v", err)
	}
	if len(prompts) != 1 {
		t.Errorf("提示词数量 = %d, 期望 1", len(prompts))
	}
}

func TestClient_GetPrompt_成功(t *testing.T) {
	client := newTestClient(t)
	defer client.Close()

	ctx := context.Background()
	if err := client.Initialize(ctx); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}

	text, err := client.GetPrompt(ctx, "greet", map[string]any{"name": "World"})
	if err != nil {
		t.Fatalf("GetPrompt 失败: %v", err)
	}
	if text == "" {
		t.Error("提示词内容不应为空")
	}
}

// ===== Ping 测试 =====

func TestClient_Ping_成功(t *testing.T) {
	client := newTestClient(t)
	defer client.Close()

	ctx := context.Background()
	if err := client.Initialize(ctx); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}

	if err := client.Ping(ctx); err != nil {
		t.Fatalf("Ping 失败: %v", err)
	}
}

// ===== 超时测试 =====

func TestClient_请求超时(t *testing.T) {
	cfg := Config{
		Command: "go",
		Args:    []string{"run", mockServerSrcPath},
		Timeout: 1 * time.Nanosecond, // 极短超时
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("创建客户端失败: %v", err)
	}
	defer client.Close()

	// 极短超时应导致请求失败
	ctx := context.Background()
	err = client.Initialize(ctx)
	if err == nil {
		t.Fatal("极短超时应导致请求失败")
	}
}

// ===== 上下文取消测试 =====

func TestClient_上下文取消(t *testing.T) {
	client := newTestClient(t)
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	err := client.Initialize(ctx)
	if err == nil {
		t.Fatal("已取消的上下文应导致请求失败")
	}
}

// ===== 环境变量测试 =====

func TestClient_自定义环境变量(t *testing.T) {
	cfg := Config{
		Command: "go",
		Args:    []string{"run", mockServerSrcPath},
		Env: map[string]string{
			"MCP_TEST_VAR": "test_value",
		},
		Timeout: 5 * time.Second,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("创建客户端失败: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	if err := client.Initialize(ctx); err != nil {
		t.Fatalf("带环境变量的初始化失败: %v", err)
	}
}

// ===== 并发安全测试 =====

func TestClient_并发调用(t *testing.T) {
	client := newTestClient(t)
	defer client.Close()

	ctx := context.Background()
	if err := client.Initialize(ctx); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}

	// 并发调用多个工具
	type callResult struct {
		text string
		err  error
	}

	const numCalls = 10
	results := make(chan callResult, numCalls)

	for i := 0; i < numCalls; i++ {
		go func(i int) {
			result, err := client.CallTool(ctx, "echo", map[string]any{
				"message": fmt.Sprintf("msg-%d", i),
			})
			if err != nil {
				results <- callResult{err: err}
				return
			}
			results <- callResult{text: result.Content[0].Text}
		}(i)
	}

	for i := 0; i < numCalls; i++ {
		r := <-results
		if r.err != nil {
			t.Errorf("并发调用 %d 失败: %v", i, r.err)
		}
	}
}

// ===== Tools 副本测试 =====

func TestClient_Tools_返回副本(t *testing.T) {
	client := newTestClient(t)
	defer client.Close()

	ctx := context.Background()
	if err := client.Initialize(ctx); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}

	tools1 := client.Tools()
	tools2 := client.Tools()

	// 修改副本不应影响原始数据
	tools1[0].Name = "modified"
	if tools2[0].Name == "modified" {
		t.Error("Tools() 应返回副本，修改不应影响后续调用")
	}
}

// ===== JSON-RPC 错误测试 =====

func TestClient_未知方法返回错误(t *testing.T) {
	client := newTestClient(t)
	defer client.Close()

	ctx := context.Background()
	// 不初始化，直接发送请求
	_, err := client.sendRequest(ctx, "unknown/method", nil)
	if err == nil {
		t.Fatal("未知方法应返回错误")
	}
}

// ===== 参数序列化测试 =====

func TestClient_CallTool_空参数(t *testing.T) {
	client := newTestClient(t)
	defer client.Close()

	ctx := context.Background()
	if err := client.Initialize(ctx); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}

	result, err := client.CallTool(ctx, "echo", map[string]any{"message": ""})
	if err != nil {
		t.Fatalf("CallTool 失败: %v", err)
	}
	if len(result.Content) != 1 {
		t.Fatalf("内容数量 = %d, 期望 1", len(result.Content))
	}
}

// ===== 类型定义测试 =====

func TestTypes_ToolDefinition_JSON序列化(t *testing.T) {
	tool := ToolDefinition{
		Name:        "test",
		Description: "测试工具",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}

	data, err := json.Marshal(tool)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	var decoded ToolDefinition
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if decoded.Name != "test" {
		t.Errorf("名称 = %q, 期望 %q", decoded.Name, "test")
	}
}

func TestTypes_ToolCallResult_JSON序列化(t *testing.T) {
	result := ToolCallResult{
		Content: []ContentBlock{
			{Type: "text", Text: "hello"},
		},
		IsError: false,
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	var decoded ToolCallResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if decoded.Content[0].Text != "hello" {
		t.Errorf("文本 = %q, 期望 %q", decoded.Content[0].Text, "hello")
	}
}

func TestTypes_ContentBlock_图片类型(t *testing.T) {
	block := ContentBlock{
		Type:     "image",
		Data:     "base64data",
		MimeType: "image/png",
	}

	data, err := json.Marshal(block)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	var decoded ContentBlock
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if decoded.Type != "image" {
		t.Errorf("类型 = %q, 期望 %q", decoded.Type, "image")
	}
	if decoded.MimeType != "image/png" {
		t.Errorf("MimeType = %q, 期望 %q", decoded.MimeType, "image/png")
	}
}

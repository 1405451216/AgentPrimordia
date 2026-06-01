package main

import (
	"fmt"
	"os"
)

func runTest(args []string) {
	var verbose bool

	i := 0
	for i < len(args) {
		switch args[i] {
		case "--verbose", "-v":
			verbose = true
		case "--help", "-h":
			fmt.Print(`ap test — 运行 eval 测试套件

用法:
  ap test [--verbose]

选项:
  --verbose, -v   显示详细输出

说明:
  运行当前项目的 eval 测试套件，评估 Agent 在预设场景下的
  工具调用准确性、输出质量和响应相关性。

示例:
  ap test
  ap test --verbose
`)
			return
		}
		i++
	}

	dir, err := findProjectDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}

	// 检查是否有 eval_test.go 文件
	hasEval := false
	entries, err := os.ReadDir(dir)
	if err == nil {
		for _, e := range entries {
			if !e.IsDir() && len(e.Name()) > 8 && e.Name()[:5] == "eval_" && e.Name()[len(e.Name())-8:] == "_test.go" {
				hasEval = true
				break
			}
		}
	}

	if !hasEval {
		fmt.Println("未找到 eval 测试文件，正在生成模板...")
		if err := generateEvalTemplate(dir); err != nil {
			fmt.Fprintf(os.Stderr, "错误: 生成 eval 模板失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("已生成 eval_agent_test.go，请编辑后重新运行 ap test")
		return
	}

	// 运行 eval 测试
	fmt.Println("运行 eval 测试套件...")
	fmt.Println()

	goTestArgs := []string{"test", "-v", "-run", "Eval", "./..."}
	if !verbose {
		goTestArgs = []string{"test", "-run", "Eval", "./..."}
	}

	//nolint:gosec // go test 命令参数受控
	result, err := runCommand(dir, "go", goTestArgs...)
	if err != nil {
		fmt.Fprintln(os.Stderr, result)
		os.Exit(1)
	}

	fmt.Println(result)
}

func generateEvalTemplate(dir string) error {
	template := `package main

import (
	"context"
	"testing"

	ap "agentprimordia/pkg"
)

// EvalTestSuite 定义 Agent 评估测试套件
// 修改以下测试用例以匹配你的 Agent 行为
func EvalTestSuite(t *testing.T) {
	// TODO: 替换为你的实际 LLM Provider
	mockLLM := &testMockLLM{}

	agent := ap.NewReActAgent(ap.ReActConfig{
		Name:         "TestAgent",
		SystemPrompt: "你是一个测试助手",
		Model:        mockLLM,
		MaxTurns:     5,
	})

	t.Run("基础回复", func(t *testing.T) {
		resp, err := agent.Run(context.Background(), ap.UserMessage("你好"))
		if err != nil {
			t.Fatalf("运行失败: %v", err)
		}
		if resp.Content == "" {
			t.Error("回复内容为空")
		}
	})
}

// testMockLLM 是测试用的 Mock LLM
type testMockLLM struct{}

func (m *testMockLLM) Complete(ctx context.Context, req *ap.CompletionRequest) (*ap.CompletionResponse, error) {
	return &ap.CompletionResponse{
		ID:      "eval-mock-1",
		Content: "这是测试回复",
		Role:    "assistant",
		Usage:   ap.Usage{PromptTokens: 5, CompletionTokens: 10},
	}, nil
}

func (m *testMockLLM) Stream(ctx context.Context, req *ap.CompletionRequest) (<-chan ap.Chunk, error) {
	ch := make(chan ap.Chunk, 1)
	go func() {
		defer close(ch)
		ch <- ap.Chunk{Content: "这是测试回复", Done: true}
	}()
	return ch, nil
}

func (m *testMockLLM) CallTools(ctx context.Context, req *ap.ToolCallRequest) (*ap.ToolCallResponse, error) {
	return &ap.ToolCallResponse{Usage: ap.Usage{}}, nil
}

func (m *testMockLLM) Embeddings(ctx context.Context, texts []string) ([][]float32, error) {
	return make([][]float32, len(texts)), nil
}

func (m *testMockLLM) Info() ap.ModelInfo {
	return ap.ModelInfo{Name: "eval-mock", Provider: "mock", MaxContext: 4096, SupportsTools: true}
}
`
	return os.WriteFile(dir+"/eval_agent_test.go", []byte(template), 0o644)
}

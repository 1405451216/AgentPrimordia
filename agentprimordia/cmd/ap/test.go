package main

import (
	"fmt"
	"os"
)

func runTest(args []string) {
	var verbose bool

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--verbose", "-v":
			verbose = true
		case "--help", "-h":
			fmt.Print(`ap test — run eval test suite

Usage:
  ap test [--verbose]

Options:
  --verbose, -v   show detailed output

Description:
  Run the eval test suite for the current project, evaluating
  tool call accuracy, output quality, and response relevance.

Examples:
  ap test
  ap test --verbose
`)
			return
		}
	}

	dir, err := findProjectDir()
	if err != nil {
		errorf("%v", err)
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
		infof("eval test file not found, generating template...")
		if err := generateEvalTemplate(dir); err != nil {
			errorf("generate eval template failed: %v", err)
			os.Exit(1)
		}
		successf("generated eval_agent_test.go, edit it then re-run ap test")
		return
	}

	// 运行 eval 测试
	infof("running eval test suite...")
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

// EvalTestSuite defines the agent eval test suite.
// Modify the test cases below to match your agent behavior.
func EvalTestSuite(t *testing.T) {
	// TODO: replace with your actual LLM Provider
	mockLLM := &testMockLLM{}

	agent := ap.NewReActAgent(ap.ReActConfig{
		Name:         "TestAgent",
		SystemPrompt: "you are a test assistant",
		Model:        mockLLM,
		MaxTurns:     5,
	})

	t.Run("basic reply", func(t *testing.T) {
		resp, err := agent.Run(context.Background(), ap.UserMessage("hello"))
		if err != nil {
			t.Fatalf("run failed: %v", err)
		}
		if resp.Content == "" {
			t.Error("response content is empty")
		}
	})
}

// testMockLLM is a mock LLM for testing.
type testMockLLM struct{}

func (m *testMockLLM) Complete(ctx context.Context, req *ap.CompletionRequest) (*ap.CompletionResponse, error) {
	return &ap.CompletionResponse{
		ID:      "eval-mock-1",
		Content: "this is a test reply",
		Role:    "assistant",
		Usage:   ap.Usage{PromptTokens: 5, CompletionTokens: 10},
	}, nil
}

func (m *testMockLLM) Stream(ctx context.Context, req *ap.CompletionRequest) (<-chan ap.Chunk, error) {
	ch := make(chan ap.Chunk, 1)
	go func() {
		defer close(ch)
		ch <- ap.Chunk{Content: "this is a test reply", Done: true}
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

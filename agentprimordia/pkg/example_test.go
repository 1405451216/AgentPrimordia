// Package ap_test 包含 AgentPrimordia 公共 API 的示例函数。
//
// 这些示例可通过 `go test -run Example` 运行，也会出现在 godoc 中。
// 所有示例使用 testutil.NewMockProvider 替代真实 LLM，无需 API Key 即可运行。
package ap_test

import (
	"context"
	"fmt"
	"log"
	"os"

	ap "agentprimordia/pkg"
	"agentprimordia/testutil"
)

// ExampleNewAgent 演示最基本的 Agent 创建方式。
// NewAgent 是推荐的入口，只需名称、系统提示词和 LLM Provider 三个必填参数。
func ExampleNewAgent() {
	// 创建 Mock LLM 提供者（生产环境替换为真实 Provider）
	mock := testutil.NewMockProvider("你好！我是 AgentPrimordia 助手，有什么可以帮你的？")

	// 使用 NewAgent 创建 Agent，通过函数式选项配置
	agent, err := ap.NewAgent("hello-agent", "你是一个友好的助手", mock,
		ap.WithMaxTurns(3),
		ap.WithTemperature(0.7),
	)
	if err != nil {
		log.Fatalf("创建 Agent 失败: %v", err)
	}

	// 运行 Agent
	resp, err := agent.Run(context.Background(), ap.UserMessage("你好！"))
	if err != nil {
		log.Fatalf("Agent 运行失败: %v", err)
	}

	fmt.Printf("回复: %s\n", resp.Content)
	fmt.Printf("轮数: %d\n", resp.Metrics.TotalTurns)

	// Output:
	// 回复: 你好！我是 AgentPrimordia 助手，有什么可以帮你的？
	// 轮数: 1
}

// ExampleNewAgent_withTools 演示带工具集的 Agent 创建。
// 使用 DefaultToolkit 快速配置文件系统、Shell 和 Web 工具。
func ExampleNewAgent_withTools() {
	// 创建默认工具包（文件系统 + Shell + Web）
	registry, err := ap.DefaultToolkit(ap.ToolkitConfig{
		RootDir:     ".",  // 根目录限制
		EnableFS:    true, // 启用文件系统工具
		EnableShell: true, // 启用 Shell 工具
		EnableWeb:   true, // 启用 Web 工具
	})
	if err != nil {
		log.Fatalf("创建 DefaultToolkit 失败: %v", err)
	}

	mock := testutil.NewMockProvider("让我帮你读取文件内容。")

	// 创建 Agent 并通过链式 API 注入工具集
	agent, err := ap.NewAgent("tooled-agent", "你是一个可以读写文件、执行命令和访问网页的助手", mock,
		ap.WithMaxTurns(5),
	)
	if err != nil {
		log.Fatalf("创建 Agent 失败: %v", err)
	}
	agent = agent.WithToolkit(registry)

	resp, err := agent.Run(context.Background(), ap.UserMessage("读取当前目录的文件"))
	if err != nil {
		log.Fatalf("Agent 运行失败: %v", err)
	}

	fmt.Printf("回复: %s\n", resp.Content)
	fmt.Printf("工具调用次数: %d\n", resp.Metrics.TotalTools)

	// Output:
	// 回复: 让我帮你读取文件内容。
	// 工具调用次数: 0
}

// ExampleNewAgent_withMemory 演示带记忆存储的 Agent 创建。
// 使用 WithInMemory 创建内存模式的 SQLite 记忆存储，适合测试。
func ExampleNewAgent_withMemory() {
	// 创建内存模式的记忆存储（无需文件，适合测试和开发）
	mem, err := ap.WithInMemory()
	if err != nil {
		log.Fatalf("创建 Memory 失败: %v", err)
	}
	defer mem.Close()

	mock := testutil.NewMockProvider("我记得你之前说过的话。")

	// 创建 Agent 并注入记忆存储
	agent, err := ap.NewAgent("memory-agent", "你是一个有记忆能力的助手", mock,
		ap.WithMaxTurns(5),
		ap.WithSessionID("session-001"),
	)
	if err != nil {
		log.Fatalf("创建 Agent 失败: %v", err)
	}
	agent = agent.WithMemory(mem)

	// 第一轮对话
	resp, err := agent.Run(context.Background(), ap.UserMessage("我叫小明"))
	if err != nil {
		log.Fatalf("Agent 运行失败: %v", err)
	}
	fmt.Printf("第一轮回复: %s\n", resp.Content)

	// 记忆会被自动保存，后续对话可检索到
	episodes, _ := mem.Search(context.Background(), "小明", nil)
	fmt.Printf("记忆条目数: %d\n", len(episodes))

	// Output:
	// 第一轮回复: 我记得你之前说过的话。
	// 记忆条目数: 0
}

// ExampleNewAgent_chainAPI 演示完整的链式 API 用法。
// 通过 WithToolkit + WithMemory + WithRAG 组合注入多种能力。
func ExampleNewAgent_chainAPI() {
	// 创建记忆存储
	mem, err := ap.WithInMemory()
	if err != nil {
		log.Fatalf("创建 Memory 失败: %v", err)
	}
	defer mem.Close()

	// 创建工具包
	registry, err := ap.DefaultToolkit(ap.ToolkitConfig{
		RootDir:  ".",
		EnableFS: true,
	})
	if err != nil {
		log.Fatalf("创建 Toolkit 失败: %v", err)
	}

	// 创建 Embedding 适配器（用于 RAG 向量检索）
	embedder := ap.NewEmbeddingAdapter(nil, 16)

	// 创建 RAG 存储
	ragStore := ap.NewRAGStore(mem, embedder)
	ragProvider := ap.NewRAGProviderAdapter(ragStore)

	mock := testutil.NewMockProvider("根据知识库的信息，我来回答你的问题。")

	// 使用链式 API 一次性注入所有能力
	agent, err := ap.NewAgent("full-agent", "你是一个全功能助手", mock,
		ap.WithMaxTurns(10),
		ap.WithTemperature(0.5),
		ap.WithSessionID("session-full"),
	)
	if err != nil {
		log.Fatalf("创建 Agent 失败: %v", err)
	}
	agent = agent.WithToolkit(registry). // 注入工具集
						WithMemory(mem).      // 注入记忆存储
						WithRAG(ap.RAGConfig{ // 注入 RAG 检索能力
			Provider: ragProvider,
			Mode:     ap.RAGModeAuto,
			TopK:     5,
		})

	resp, err := agent.Run(context.Background(), ap.UserMessage("帮我查找相关信息"))
	if err != nil {
		log.Fatalf("Agent 运行失败: %v", err)
	}

	fmt.Printf("回复: %s\n", resp.Content)

	// Output:
	// 回复: 根据知识库的信息，我来回答你的问题。
}

// ExampleNewPool 演示多 Agent 并发调度。
// Pool 支持任务分发、重试、取消和会话管理。
func ExampleNewPool() {
	// 创建 Pool，配置最大并发数和默认 Agent 参数
	pool := ap.NewPool(ap.PoolConfig{
		MaxConcurrency: 3,
		DefaultAgent: ap.ReActAgentConfig{
			SystemPrompt: "你是一个任务处理助手",
			MaxTurns:     3,
		},
	})
	defer pool.Close()

	// 设置 Mock 模型（生产环境替换为真实 Provider）
	mock := testutil.NewMockProvider("任务已完成")
	pool.SetModel(mock)

	// 批量提交任务
	tasks := []ap.TaskConfig{
		{ID: "task-1", Title: "数据分析", Prompt: "分析销售数据趋势", SessionID: "session-001"},
		{ID: "task-2", Title: "报告生成", Prompt: "生成月度报告", SessionID: "session-001"},
		{ID: "task-3", Title: "邮件撰写", Prompt: "撰写客户跟进邮件", SessionID: "session-002"},
	}

	// 并发调度执行
	results, err := pool.Dispatch(context.Background(), tasks)
	if err != nil {
		log.Fatalf("Pool 调度失败: %v", err)
	}

	// 输出每个任务的结果
	for _, r := range results {
		status := "成功"
		if r.Error != nil {
			status = r.Error.Error()
		}
		fmt.Printf("任务 [%s] %s: %s\n", r.TaskID, r.Task.Title, status)
	}

	// 查看运行统计
	stats := pool.Stats()
	fmt.Printf("完成: %d, 失败: %d\n", stats.CompletedTasks, stats.FailedTasks)

	// Output:
	// 任务 [task-1] 数据分析: 成功
	// 任务 [task-2] 报告生成: 成功
	// 任务 [task-3] 邮件撰写: 成功
	// 完成: 3, 失败: 0
}

// ExampleNewResilientProvider 演示弹性 LLM 提供者的创建。
// ResilientProvider 支持重试、熔断和降级，保障 LLM 调用的可靠性。
func ExampleNewResilientProvider() {
	// 创建主 Provider（生产环境使用真实 Provider）
	primary := testutil.NewMockProvider("主模型响应")

	// 使用默认弹性配置创建 ResilientProvider
	cfg := ap.DefaultResilientConfig()
	// cfg.MaxRetries = 3            // 最大重试次数
	// cfg.RetryBackoff = 500ms      // 重试退避时间
	// cfg.CircuitThreshold = 5      // 熔断阈值
	// cfg.CircuitRecoverAfter = 30s // 熔断恢复时间

	resilient, err := ap.NewResilientProvider(primary, cfg)
	if err != nil {
		log.Fatalf("创建 ResilientProvider 失败: %v", err)
	}

	// 添加降级 Provider（主 Provider 失败时自动切换）
	fallback := testutil.NewMockProvider("降级模型响应")
	resilient.AddFallback(fallback)

	// 正常调用
	resp, err := resilient.Complete(context.Background(), &ap.CompletionRequest{
		Messages: []ap.ChatMessage{{Role: "user", Content: "你好"}},
	})
	if err != nil {
		log.Fatalf("调用失败: %v", err)
	}

	fmt.Printf("响应: %s\n", resp.Content)

	// Output:
	// 响应: 主模型响应
}

// ExampleNewDAGBuilder 演示 DAG 工作流的声明式构建。
// DAGBuilder 提供链式 API，支持节点、边、条件分支和并行执行。
func ExampleNewDAGBuilder() {
	// 定义节点处理函数
	searchHandler := func(ctx context.Context, input string) (string, error) {
		return "搜索结果: Go Agent 框架对比", nil
	}
	extractHandler := func(ctx context.Context, input string) (string, error) {
		return "提取信息: AgentPrimordia 性能最优", nil
	}
	summarizeHandler := func(ctx context.Context, input string) (string, error) {
		return "总结: AgentPrimordia 是最佳选择", nil
	}

	// 使用 DAGBuilder 声明式构建工作流
	dag, err := ap.NewDAGBuilder("research-workflow").
		Node("search", searchHandler).Label("Web Search").
		Node("extract", extractHandler).Label("Extract Info").
		Node("summarize", summarizeHandler).Label("Summarize").
		Edge("search", "extract").    // search → extract
		Edge("extract", "summarize"). // extract → summarize
		Build()
	if err != nil {
		log.Fatalf("DAG 构建失败: %v", err)
	}

	// 运行工作流
	result, err := dag.Run(context.Background(), "对比 Go Agent 框架")
	if err != nil {
		log.Fatalf("DAG 运行失败: %v", err)
	}

	// 获取最终节点（summarize）的输出
	finalOutput := result.NodeResults["summarize"].Output
	fmt.Printf("最终结果: %s\n", finalOutput)
	fmt.Printf("节点数: %d\n", dag.NodeCount())
	fmt.Printf("边数: %d\n", dag.EdgeCount())

	// 输出 Mermaid 流程图
	fmt.Printf("流程图: %s\n", dag.ToMermaid()[:6])

	// Output:
	// 最终结果: 总结: AgentPrimordia 是最佳选择
	// 节点数: 3
	// 边数: 2
	// 流程图: graph
}

// ExampleSession 演示多轮对话会话。
// Session 自动维护对话上下文，并将历史保存到记忆存储。
func ExampleSession() {
	// 创建记忆存储
	mem, err := ap.WithInMemory()
	if err != nil {
		log.Fatalf("创建 Memory 失败: %v", err)
	}
	defer mem.Close()

	// 创建 Agent（带记忆）
	mock := testutil.NewMockProvider(
		"你好！我叫小智。",
		"你刚才说你叫小明。",
	)
	agent, err := ap.NewAgent("chat-agent", "你是一个对话助手", mock,
		ap.WithMaxTurns(5),
	)
	if err != nil {
		log.Fatalf("创建 Agent 失败: %v", err)
	}
	agent = agent.WithMemory(mem)

	// 创建会话
	sess := ap.NewSession(agent, mem, ap.SessWithID("chat-session-001"))

	// 第一轮对话
	resp1, err := sess.Ask(context.Background(), "你好！我叫小明")
	if err != nil {
		log.Fatalf("第一轮对话失败: %v", err)
	}
	fmt.Printf("第一轮: %s\n", resp1.Content)

	// 第二轮对话（自动关联上下文）
	resp2, err := sess.Ask(context.Background(), "我叫什么名字？")
	if err != nil {
		log.Fatalf("第二轮对话失败: %v", err)
	}
	fmt.Printf("第二轮: %s\n", resp2.Content)

	// 查看会话状态
	fmt.Printf("轮次: %d\n", sess.TurnCount())

	// 查看记忆中的历史
	episodes, _ := mem.Search(context.Background(), "小明", nil)
	fmt.Printf("记忆条目: %d\n", len(episodes))

	// 重置会话（不清空底层记忆）
	sess.Reset()
	fmt.Printf("重置后轮次: %d\n", sess.TurnCount())

	// Output:
	// 第一轮: 你好！我叫小智。
	// 第二轮: 你刚才说你叫小明。
	// 轮次: 2
	// 记忆条目: 0
	// 重置后轮次: 0
}

// 以下示例需要真实 API Key，无法在 CI 中自动运行。
// 取消注释并在设置环境变量后手动运行。

// ExampleNewOpenAIProvider 演示创建 OpenAI Provider。
// 需要设置 AP_LLM_API_KEY 环境变量。
func ExampleNewOpenAIProvider() {
	apiKey := os.Getenv("AP_LLM_API_KEY")
	if apiKey == "" {
		fmt.Println("跳过: 未设置 AP_LLM_API_KEY 环境变量")
		return
	}

	provider, err := ap.NewOpenAIProvider(ap.Config{
		APIKey: apiKey,
		Model:  "gpt-4o",
	})
	if err != nil {
		fmt.Printf("创建 Provider 失败: %v\n", err)
		return
	}

	agent, err := ap.NewAgent("openai-agent", "你是一个智能助手", provider,
		ap.WithMaxTurns(3),
	)
	if err != nil {
		fmt.Printf("创建 Agent 失败: %v\n", err)
		return
	}

	resp, err := agent.Run(context.Background(), ap.UserMessage("你好"))
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		return
	}
	fmt.Printf("回复: %s\n", resp.Content)
}

# 第三方接入指南（30 分钟接入真实场景）

> v4.4-5 开发者平台：从「框架可嵌入」到「第三方可低成本接入」。
> 目标：按本文档在 30 分钟内把 AgentPrimordia 接入真实业务场景。

## 接入全景

```
第三方系统
  │  A2A 双向（委托任务/被委托）
  ├── 作为调用方：ap A2A 客户端 → 远程 Agent（跨生态委托）
  ├── 作为服务方：ap A2A 服务端 ← 生态客户端（开放协议）
  │
  │  双 SDK 快速上手
  ├── Go：go get github.com/AgentPrimordia/agentprimordia/pkg
  └── TS：npm install @agentprimordia/sdk
```

## 一、双 SDK 快速上手（5 分钟）

### Go（推荐后端服务）

```go
package main

import (
	"context"
	"fmt"

	ap "github.com/AgentPrimordia/agentprimordia/pkg"
)

func main() {
	ctx := context.Background()

	// 1. Provider（本地 Ollama 免 key，或 OpenAI/Anthropic/Gemini）
	provider, _ := ap.NewOllamaProvider(ap.Config{Model: "qwen3:8b"})

	// 2. 创建 Agent（ReAct 循环 + 工具 + 护栏）
	agent, err := ap.NewAgent("my-agent", "你是客服助手。", provider,
		ap.WithMaxTurns(8),
		ap.WithToolkit(ap.NewToolRegistry()),
	)
	if err != nil {
		panic(err)
	}

	// 3. 运行
	resp, err := agent.Run(ctx, ap.UserMessage("查询订单 1001 状态"))
	fmt.Println(resp.Content)
}
```

### TypeScript（推荐前端/边缘）

```ts
import { Agent, OpenAIProvider } from '@agentprimordia/sdk';

const provider = new OpenAIProvider({ apiKey: process.env.OPENAI_API_KEY!, model: 'gpt-4o-mini' });
const agent = new Agent({
  name: 'my-agent',
  systemPrompt: '你是客服助手。',
  provider,
  maxTurns: 8,
});

const resp = await agent.run('查询订单 1001 状态');
console.log(resp.content);
```

## 二、A2A 双向接入（15 分钟）

### 作为服务方：开放你的 Agent

```go
// 1. 部署 A2A 服务端（开放协议，任意生态客户端可调用）
card := ap.NewA2AAgentCard("my-agent", "客服助手")
svc := ap.NewA2AService(card, ap.NewA2ATaskManager())
srv := ap.NewA2AGRPCServer(svc)      // gRPC 传输
// 或开放协议（HTTP/SSE）：ap.NewOpenInteropServer(...)

// 2. 启动监听
lis, _ := net.Listen("tcp", ":50051")
srv.Serve(lis)
```

### 作为调用方：委托远程 Agent

```go
client, _ := ap.NewA2AGRPCClient("remote-host:50051")
task, _ := client.SendTask(ctx, "请汇总本月销售数据")
// 轮询/推送任务状态 → 拉取 artifact
```

### TS 侧对等实现

```ts
import { A2AClient } from '@agentprimordia/sdk';
const client = new A2AClient({ endpoint: 'https://agent.example.com' });
const task = await client.sendTask('请汇总本月销售数据');
```

## 三、接入真实场景清单（30 分钟）

| 分钟 | 步骤 | 验证 |
|------|------|------|
| 0-5 | 双 SDK 快速上手（上文一） | `go run .` / `npm run dev` 输出应答 |
| 5-15 | 接真实 LLM：设 `AP_LLM_PROVIDER/AP_LLM_MODEL/AP_LLM_API_KEY` 或本地 Ollama | 用真实模型回答 |
| 15-25 | 挂业务工具：filesystem/shell/自定义 `ap.Tool` | 工具调用日志可见 |
| 25-30 | A2A 双向（上文二）：开放服务 + 远程委托 | 跨服务任务完成 |

## 四、常见问题

- **工具权限**：默认受限工作区，用 `ap.WithFilesScope("/data")` 扩展
- **成本控制**：`ap.WithCostTracker` 预算拦截；Pool 租户配额
- **观测**：`ap.WithTelemetry` 接入 OTel；`/api/failures` 一键重放失败
- **安全**：guardrail 输入/输出端默认开启；插件安装强制验签（ECDSA P-256）

## 五、下一步

- 技能市场：`docs/guides/skill-format.md`（发布/订阅 + 验签）
- 插件开发：`ap plugin create`（模板含测试与发布流水线）
- 分布式：`docs/guides/multi-agent.md` + `docs/advanced/cluster-verification.md`

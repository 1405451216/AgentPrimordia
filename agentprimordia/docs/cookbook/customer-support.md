# 客服 Agent

> 接入向量知识库与护栏，构建有安全边界的客服 Agent。

## 背景

客服 Agent 需要：(1) 从知识库检索回答；(2) 不泄露内部实现细节；(3) 检测并拒绝越狱攻击。

## 代码

```go
package main

import (
    "context"
    "fmt"
    "log"

    ap "agentprimordia/pkg"
)

func main() {
    // 向量知识库（产品手册 / FAQ）
    mem := ap.NewVectorMemory(ap.VectorConfig{DBPath: "./data/support.db"})
    _ = mem.IndexDirectory("./knowledge-base")

    // 护栏：PII 过滤 + 注入检测 + 主题隔离
    guardrail := ap.NewGuardrailPipeline(
        ap.NewPIIFilter(),
        ap.NewInjectionDetector(),
        ap.NewTopicBoundary([]string{"产品使用", "订单查询", "退换货政策"}),
    )

    agent := ap.NewAgent(ap.AgentConfig{
        Name: "customer-support",
        SystemPrompt: "你是客服助手。只回答产品相关问题，不讨论内部实现。",
        Memory: mem,
        Tools: []ap.Tool{ap.NewMemorySearchTool(mem)},
        Guardrail: guardrail,
    })

    resp, err := agent.Run(context.Background(), "如何申请退换货？")
    if err != nil { log.Fatal(err) }
    fmt.Println(resp.Content)
}
```

## 护栏配置

```yaml
guardrail:
  - type: pii_filter
    action: mask          # mask 敏感信息
  - type: prompt_injection
    action: reject        # 拒绝注入尝试
  - type: topic_boundary
    allowed_topics: ["产品使用", "订单查询", "退换货政策"]
    fallback: "我只能回答产品使用相关问题哦。"
```

## 扩展

- **情感识别**：检测用户情绪，愤怒时自动升级人工
- **多语言**：自动检测语言并切换回答
- **记忆**：记住同一用户的上下文，避免重复询问
- **A/B Testing**：同时运行两个 system prompt 版本，按解决率选优

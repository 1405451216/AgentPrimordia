# 多模态输入

AgentPrimordia 支持图片、音频、视频等多模态输入，目前主要集成在 LLM Provider 层。

## 支持的 Provider

| Provider | 图片 | 音频 | 视频 |
|----------|------|------|------|
| OpenAI   | ✓    | ✓    | ✓    |
| Anthropic| ✓    | ✗    | ✗    |
| Gemini   | ✓    | ✓    | ✓    |

## 快速开始

```go
provider := llm.NewOpenAIMultimodalProvider(llm.OpenAIConfig{
    APIKey: os.Getenv("OPENAI_API_KEY"),
    Model:  "gpt-4o",
})

imageData, _ := os.ReadFile("image.png")

resp, err := provider.Complete(ctx, &llm.CompletionRequest{
    Messages: []llm.ChatMessage{
        {
            Role:    "user",
            Content: "描述这张图片",
            Images: []llm.ImageContent{
                {Data: imageData, MIMEType: "image/png"},
            },
        },
    },
})
```

## MultimodalAdapter

```go
adapter := llm.NewMultimodalAdapter(provider)
resp, err := adapter.Complete(ctx, "描述这张图片", imageData)
```

## 与 ReActAgent 集成

```go
agent := NewReActAgent(ReActConfig{
    Name:  "vision-agent",
    Model: provider,
})

msg := UserMessage("描述这张图片")
msg.Images = []ImageContent{{Data: imageData, MIMEType: "image/png"}}

resp, err := agent.Run(ctx, msg)
```

## 使用场景

- 视觉问答 Agent
- 图文理解助手
- 文档扫描与分析

## 下一步

- 查看 [LLM API 参考](../api/llm.md)
- 查看 [多模态示例](../../ecosystem/examples/multimodal-vision/main.go)

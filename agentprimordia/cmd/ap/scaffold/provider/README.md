# {{.ProjectName}} Provider

AgentPrimordia LLM Provider 模板项目（通过 `ap init --type provider` 生成）。

## 快速开始

```bash
# 1. 实现 Chat 方法（在 main.go 中替换占位实现）
# 2. 跑测试
go test ./...

# 3. 集成到 AgentPrimordia
# 在 agentprimordia 项目中通过 ap.yaml 配置 provider：
#
# llm:
#   provider: {{.ProjectName}}
#   api_key: ${AP_LLM_API_KEY}
#   model: {{.ProjectName}}-default
```

## 目录结构

```
{{.ProjectName}}/
├── main.go         # Provider 入口（Chat / Embeddings）
├── main_test.go    # 单元测试
├── README.md       # 本文件
├── Makefile        # build / test / lint
└── .github/workflows/
    └── ci.yml      # GitHub Actions CI
```

## Provider 接口

```go
type Provider interface {
    Name() string
    Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
    Close() error
}
```

详细协议见 `internal/llm/provider.go`。

## 接入步骤

1. 实现 `Chat` 方法：调用 {{.ProjectName}} HTTP API 并解析响应
2. （可选）实现流式 `ChatStream`，支持 `req.Stream = true`
3. 添加用量统计：填充 `Usage`
4. 添加重试与超时：通过 `context.Context` 传递

## 许可

MIT
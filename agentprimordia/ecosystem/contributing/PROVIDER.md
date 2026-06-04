# Provider 贡献指南

本文档介绍如何为 AgentPrimordia 框架贡献新的 LLM Provider。

## 1. 概述

AgentPrimordia 的 LLM 抽象层（`internal/llm/`）通过 `Provider` 接口统一了不同大模型提供商的调用方式。贡献新的 Provider 意味着：

- 让框架支持更多 LLM 服务商
- 帮助社区用户快速接入新的模型
- 丰富框架的生态兼容性

目前已有 Provider：OpenAI、Anthropic Claude、Google Gemini、Ollama、Azure OpenAI、Cohere、Mistral AI、智谱 GLM、通义千问。

---

## 2. 快速开始 — 5 步创建新 Provider

### 步骤 1：复制模板

```bash
cp internal/llm/provider_template.go internal/llm/deepseek_provider.go
cp internal/llm/provider_template_test.go internal/llm/deepseek_provider_test.go
```

### 步骤 2：全局替换模板标识

将文件中的 `template` / `Template` 替换为你的 Provider 名称：

- `TemplateProvider` → `DeepSeekProvider`
- `TemplateProviderConfig` → `DeepSeekProviderConfig`（如使用自定义配置）
- `NewTemplateProvider` → `NewDeepSeekProvider`
- `"template"` → `"deepseek"`

### 步骤 3：实现 Provider 接口

实现 `Provider` 接口的 4 个方法（详见第 3 节）：

- `Complete()` — 非流式补全
- `Stream()` — 流式补全
- `CallTools()` — 工具调用
- `Info()` — 模型信息

### 步骤 4：编写测试

参照 `provider_template_test.go` 和已有 Provider 的测试文件，编写单元测试。

### 步骤 5：验证

```bash
# 编译检查
go build ./internal/llm/

# 运行你的 Provider 测试
go test -run TestDeepSeek ./internal/llm/

# 运行全部 llm 包测试
go test ./internal/llm/
```

---

## 3. Provider 接口规范

`Provider` 接口定义在 `types.go` 中：

```go
type Provider interface {
    Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)
    Stream(ctx context.Context, req *CompletionRequest) (<-chan Chunk, error)
    CallTools(ctx context.Context, req *ToolCallRequest) (*ToolCallResponse, error)
    Info() ModelInfo
}
```

### 3.1 Complete — 非流式补全

```go
func (p *XxxProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)
```

**输入**：`CompletionRequest`（定义在 `types.go`）

| 字段 | 类型 | 说明 |
|------|------|------|
| `Messages` | `[]ChatMessage` | 对话消息列表 |
| `Model` | `string` | 请求指定的模型（可选，为空时使用配置默认值） |
| `Temperature` | `*float64` | 温度参数（指针类型，nil 表示未设置） |
| `MaxTokens` | `int` | 最大输出 Token 数 |
| `Stream` | `bool` | 是否流式（Complete 方法中忽略） |
| `ResponseFormat` | `*ResponseFormat` | 结构化输出格式控制（可选） |

**输出**：`CompletionResponse`

| 字段 | 类型 | 说明 |
|------|------|------|
| `ID` | `string` | 响应 ID |
| `Model` | `string` | 实际使用的模型名称 |
| `Content` | `string` | 补全文本内容 |
| `Role` | `string` | 角色（通常为 `"assistant"`） |
| `Usage` | `Usage` | Token 使用量 |

**实现要点**：

- 使用 `ResolveTemperature(req.Temperature, p.config.Temperature)` 解析有效温度
- 使用 `ResolveModel(req.Model, p.config.Model)` 解析有效模型名称
- 使用 `ResolveTemperature` 和 `Float64Ptr` 辅助函数（定义在 `types.go`）
- API 错误应返回 `APIError` 类型或包装 `ErrLLMCallFailed` / `ErrResponseParseFailed` / `ErrEmptyResponse`

### 3.2 Stream — 流式补全

```go
func (p *XxxProvider) Stream(ctx context.Context, req *CompletionRequest) (<-chan Chunk, error)
```

**输出**：`<-chan Chunk`（只读 Channel）

`Chunk` 结构：

| 字段 | 类型 | 说明 |
|------|------|------|
| `Content` | `string` | 增量文本内容 |
| `Done` | `bool` | 是否为最后一个 Chunk |
| `Usage` | `*Usage` | Token 使用量（仅最后一个 Chunk 携带） |

**实现要点**：

- 创建 buffered channel：`ch := make(chan Chunk, 32)`
- 在 goroutine 中读取 SSE 流并发送到 channel
- 必须在 goroutine 结束时 `close(ch)` 和 `resp.Body.Close()`
- 必须监听 `ctx.Done()` 以支持取消
- 流结束时发送 `Chunk{Done: true}`
- 使用 `bufio.Scanner` 读取 SSE 流，设置足够的 buffer 大小

### 3.3 CallTools — 工具调用

```go
func (p *XxxProvider) CallTools(ctx context.Context, req *ToolCallRequest) (*ToolCallResponse, error)
```

**输入**：`ToolCallRequest`

| 字段 | 类型 | 说明 |
|------|------|------|
| `Messages` | `[]ChatMessage` | 对话消息列表 |
| `Tools` | `[]ToolDefinition` | 可用工具定义列表 |
| `Model` | `string` | 请求指定的模型（可选） |

**输出**：`ToolCallResponse`

| 字段 | 类型 | 说明 |
|------|------|------|
| `Content` | `string` | 文本内容（可能为空） |
| `ToolCalls` | `[]FunctionCall` | 工具调用列表 |
| `Usage` | `Usage` | Token 使用量 |

`FunctionCall` 结构：

| 字段 | 类型 | 说明 |
|------|------|------|
| `ID` | `string` | 工具调用 ID |
| `Name` | `string` | 工具名称 |
| `Arguments` | `string` | 工具参数（JSON 字符串） |

**实现要点**：

- 大多数 Provider 的工具调用与 `Complete` 共享同一 API 端点
- 区别在于请求中包含 `tools` 定义，响应中包含 `tool_calls`
- 如果 Provider 不支持工具调用，返回 `ErrNotSupported`
- 使用 `BuildOpenAIMessages(msgs)` 构建 OpenAI 兼容格式的消息（适用于 OpenAI 兼容 API）

### 3.4 Info — 模型信息

```go
func (p *XxxProvider) Info() ModelInfo
```

**输出**：`ModelInfo`

| 字段 | 类型 | 说明 |
|------|------|------|
| `Name` | `string` | 模型名称 |
| `Provider` | `string` | Provider 标识（小写，如 `"openai"`、`"zhipu"`） |
| `MaxContext` | `int` | 最大上下文窗口大小 |
| `SupportsTools` | `bool` | 是否支持工具调用 |
| `SupportsStreaming` | `bool` | 是否支持流式输出 |

---

## 4. 可选接口

### 4.1 Embedder — 文本嵌入

```go
type Embedder interface {
    Embeddings(ctx context.Context, texts []string) ([][]float32, error)
}
```

如果 Provider 支持文本嵌入，额外实现此接口。调用方通过类型断言检查：

```go
if embedder, ok := provider.(llm.Embedder); ok {
    vectors, err := embedder.Embeddings(ctx, texts)
}
```

参考实现：`openai_provider.go` 中的 `Embeddings` 方法。

### 4.2 MultimodalProvider — 多模态支持

```go
type MultimodalProvider interface {
    Provider
    CompleteMultimodal(ctx context.Context, req *CompletionRequestExt) (*CompletionResponse, error)
    StreamMultimodal(ctx context.Context, req *CompletionRequestExt) (<-chan Chunk, error)
    Capabilities() MultimodalCapability
    ModelInfoExt() ModelInfoExt
}
```

如果 Provider 支持图片/音频/视频等多模态输入，需要：

1. 实现 `CompleteMultimodal` 和 `StreamMultimodal` 方法
2. 实现 `InfoExt()` 方法返回 `ModelInfoExt`
3. 通过 `MultimodalAdapter` 适配为统一的 `MultimodalProvider` 接口

参考实现：`glm_provider.go`、`qwen_provider.go`。

**多模态相关类型**（定义在 `multimodal_types.go`）：

- `ChatMessageExt` — 多模态消息扩展
- `MultimodalContent` — 多模态内容单元（文本/图片URL/Base64图片/音频/视频）
- `CompletionRequestExt` — 多模态补全请求
- `ModelInfoExt` — 扩展模型信息（含视觉/音频/视频能力标记）

**便捷方法**：

- `NewUserTextMessage(text)` — 创建纯文本用户消息
- `NewUserMultimodalMessage(contents...)` — 创建多模态用户消息
- `NewTextContent(text)` — 创建文本内容
- `NewImageURLContent(url)` — 创建图片 URL 内容
- `NewImageB64Content(data, mime)` — 创建 Base64 图片内容
- `ConvertRequestToExt(req)` — 将 `CompletionRequest` 转换为 `CompletionRequestExt`

---

## 5. 配置模式

### 5.1 使用通用 Config

大多数 Provider 使用 `types.go` 中定义的通用 `Config` 结构：

```go
type Config struct {
    APIKey      string         `json:"-"`
    BaseURL     string         `json:"base_url,omitempty"`
    Model       string         `json:"model"`
    Temperature float64        `json:"temperature,omitempty"`
    MaxTokens   int            `json:"max_tokens,omitempty"`
    Extra       map[string]any `json:"extra,omitempty"`
}
```

构造函数签名：

```go
func NewXxxProvider(cfg Config) (*XxxProvider, error)
```

参考实现：`openai_provider.go`、`glm_provider.go`、`qwen_provider.go`。

### 5.2 构造函数规范

- 必须校验 `APIKey`，为空时返回 `ErrAPIKeyRequired`
- 必须设置默认 `BaseURL`（如果为空）
- 必须设置默认 `Model`（如果为空）
- 必须规范化 `BaseURL`（去除尾部 `/`）
- 必须初始化 `http.Client`，使用 `defaultTimeout`（120 秒）

```go
func NewXxxProvider(cfg Config) (*XxxProvider, error) {
    if cfg.APIKey == "" {
        return nil, ErrAPIKeyRequired
    }

    baseURL := cfg.BaseURL
    if baseURL == "" {
        baseURL = "https://api.xxx.com/v1"
    }
    cfg.BaseURL = strings.TrimRight(baseURL, "/")

    if cfg.Model == "" {
        cfg.Model = "xxx-default-model"
    }

    return &XxxProvider{
        config: cfg,
        client: &http.Client{Timeout: defaultTimeout},
    }, nil
}
```

### 5.3 使用 Extra 字段

对于 Provider 特有的配置项，使用 `Config.Extra` 字段：

```go
// 读取 Extra 中的自定义配置
if v, ok := cfg.Extra["project_id"]; ok {
    p.projectID = v.(string)
}
```

---

## 6. 测试要求

### 6.1 必须通过的测试

每个 Provider 必须包含以下测试：

| 测试名称 | 说明 |
|----------|------|
| `TestNewXxxProvider_Success` | 构造函数正常创建 |
| `TestNewXxxProvider_MissingAPIKey` | 缺少 API Key 时返回 `ErrAPIKeyRequired` |
| `TestXxxProvider_Info` | `Info()` 返回正确的模型信息 |
| `TestXxxProvider_Complete_WithMockServer` | 使用 `httptest.Server` 测试非流式补全 |
| `TestXxxProvider_Complete_APIError` | API 错误处理 |
| `TestXxxProvider_Stream_WithMockServer` | 使用 `httptest.Server` 测试流式补全 |
| `TestXxxProvider_CallTools_WithMockServer` | 使用 `httptest.Server` 测试工具调用 |
| `TestXxxProvider_CallTools_NotSupported` | 不支持工具调用时返回 `ErrNotSupported`（如适用） |

### 6.2 多模态 Provider 额外测试

| 测试名称 | 说明 |
|----------|------|
| `TestBuildMultimodalMessages_Xxx_TextOnly` | 纯文本消息构建 |
| `TestBuildMultimodalMessages_Xxx_WithImage` | 含图片的消息构建 |
| `TestConvertToXxxFormat_Text` | 文本格式转换 |
| `TestConvertToXxxFormat_Base64Image` | Base64 图片格式转换 |
| `TestConvertToXxxFormat_Audio` | 音频格式转换（不支持时返回 nil） |
| `TestCompleteMultimodal_Xxx_WithMockServer` | 多模态补全测试 |
| `TestComplete_Xxx_BackwardCompatible` | 向后兼容接口测试 |

### 6.3 测试规范

- 使用 `net/http/httptest.Server` 模拟 API 服务器，不依赖真实网络
- 使用 `t.TempDir()` 创建临时文件，不污染项目
- Mock 服务器返回符合目标 API 格式的 JSON 响应
- 测试错误路径（API 错误、空响应、无效 JSON 等）

### 6.4 测试模板

参考 `provider_template_test.go` 获取基础测试模板，参考已有 Provider 的测试文件（如 `glm_provider_test.go`、`qwen_provider_test.go`）获取完整测试示例。

---

## 7. 命名规范

### 7.1 文件命名

| 文件 | 命名规则 | 示例 |
|------|----------|------|
| Provider 实现 | `{provider}_provider.go | `openai_provider.go`、`glm_provider.go` |
| Provider 测试 | `{provider}_provider_test.go` | `openai_test.go`、`glm_provider_test.go` |
| 多模态 Provider | `{provider}_multimodal_provider.go`（如独立文件） | `openai_multimodal_provider.go` |
| 视觉 Provider | `{provider}_vision_provider.go`（如独立文件） | `anthropic_vision_provider.go` |

### 7.2 结构体命名

| 类型 | 命名规则 | 示例 |
|------|----------|------|
| Provider 结构体 | `{Provider}Provider` | `OpenAIProvider`、`GLMProvider` |
| 构造函数 | `New{Provider}Provider` | `NewOpenAIProvider`、`NewGLMProvider` |
| Provider 标识 | 小写英文 | `"openai"`、`"zhipu"`、`"qwen"` |

### 7.3 常量命名

```go
const (
    xxxDefaultBaseURL    = "https://api.xxx.com/v1"  // 默认 API 地址
    defaultXxxMaxContext = 128000                     // 默认最大上下文
    defaultXxxMaxTokens  = 4096                       // 默认最大输出 Token
)
```

### 7.4 中文 Provider 命名

对于中国本土 LLM 服务商，Provider 标识使用拼音或英文缩写：

| 服务商 | Provider 标识 | 文件名 | 结构体 |
|--------|--------------|--------|--------|
| 智谱 AI | `"zhipu"` | `glm_provider.go` | `GLMProvider` |
| 通义千问 | `"qwen"` | `qwen_provider.go` | `QwenProvider` |
| 百度文心 | `"wenxin"` | `wenxin_provider.go` | `WenxinProvider` |
| 月之暗面 | `"moonshot"` | `moonshot_provider.go` | `MoonshotProvider` |
| DeepSeek | `"deepseek"` | `deepseek_provider.go` | `DeepSeekProvider` |

---

## 8. 提交流程

### 8.1 PR 前检查清单

- [ ] 代码编译通过：`go build ./internal/llm/`
- [ ] 所有测试通过：`go test ./internal/llm/`
- [ ] 新 Provider 的测试全部通过
- [ ] 代码注释使用中文
- [ ] 遵循现有代码风格（参考 `openai_provider.go`、`glm_provider.go`）
- [ ] 未引入新的第三方依赖（仅使用 Go 标准库）
- [ ] 未修改 `Provider` 接口定义
- [ ] `Info()` 返回正确的 Provider 标识

### 8.2 提交信息格式

```
feat: 添加 {Provider} Provider
```

示例：

```
feat: 添加 DeepSeek Provider
feat: 添加 Moonshot Provider（含多模态支持）
fix: 修复 GLM Provider 流式读取边界问题
```

### 8.3 PR 描述模板

```markdown
## 新增 Provider

**Provider 名称**: DeepSeek
**Provider 标识**: `deepseek`
**API 文档**: https://platform.deepseek.com/api-docs

### 实现功能
- [x] Complete — 非流式补全
- [x] Stream — 流式补全（SSE）
- [x] CallTools — 工具调用
- [ ] Embeddings — 文本嵌入
- [ ] 多模态支持

### 测试覆盖
- [x] 构造函数测试
- [x] Info 测试
- [x] Complete Mock 测试
- [x] Stream Mock 测试
- [x] CallTools Mock 测试
- [x] API 错误处理测试

### 兼容性
- API 格式：OpenAI 兼容
- 默认模型：deepseek-chat
- 默认 BaseURL：https://api.deepseek.com/v1
```

---

## 9. 常见问题

### Q1: 我的 Provider 使用 OpenAI 兼容 API，可以复用代码吗？

可以。许多 Provider（如 Qwen、GLM、Mistral、DeepSeek）使用 OpenAI 兼容的 API 格式。你可以复用以下共享函数：

- `BuildOpenAIMessages(msgs)` — 构建 OpenAI 格式消息列表（定义在 `provider_helpers.go`）
- `buildOpenAIResponseFormat(rf)` — 构建 OpenAI 格式的 ResponseFormat（定义在 `openai_provider.go`）
- `openaiChatResponse` — OpenAI 格式的响应结构体（定义在 `openai_provider.go`）
- `ResolveModel(reqModel, configModel)` — 解析模型名称（定义在 `provider_helpers.go`）
- `ResolveTemperature(reqTemp, configTemp)` — 解析温度参数（定义在 `types.go`）
- `ConvertRequestToExt(req)` — 将标准请求转换为多模态请求（定义在 `provider_helpers.go`）

### Q2: Provider 不支持工具调用怎么办？

在 `CallTools` 方法中返回 `ErrNotSupported`：

```go
func (p *XxxProvider) CallTools(ctx context.Context, req *ToolCallRequest) (*ToolCallResponse, error) {
    return nil, ErrNotSupported
}
```

### Q3: Provider 不支持流式输出怎么办？

在 `Stream` 方法中返回 `ErrNotSupported`：

```go
func (p *XxxProvider) Stream(ctx context.Context, req *CompletionRequest) (<-chan Chunk, error) {
    return nil, ErrNotSupported
}
```

### Q4: 如何处理 API 特有的错误格式？

定义 Provider 内部的错误响应结构体，在 `doRequest` 方法中统一处理。参考 `glm_provider.go` 中的错误处理模式。

### Q5: 如何添加多模态支持？

1. 实现 `CompleteMultimodal` 和 `StreamMultimodal` 方法
2. 实现 `InfoExt()` 方法返回 `ModelInfoExt`
3. 实现 `buildMultimodalMessages` 方法将 `ChatMessageExt` 转换为目标 API 格式
4. 实现 `convertToXxxFormat` 方法将 `MultimodalContent` 转换为目标 API 格式
5. 在 `Complete` 和 `Stream` 方法中调用 `ConvertRequestToExt` 实现向后兼容

参考 `glm_provider.go` 或 `qwen_provider.go` 的完整实现。

### Q6: 可以引入第三方 HTTP 客户端库吗？

不可以。项目约束仅使用 Go 标准库（`net/http`、`encoding/json` 等），不引入任何第三方 Web 框架或 HTTP 客户端库。

### Q7: 模型的上下文窗口大小如何确定？

在 Provider 文件中定义常量或 map：

```go
const defaultXxxMaxContext = 128000

// 或按模型区分
var xxxContextSizes = map[string]int{
    "model-a": 8192,
    "model-b": 32768,
    "model-c": 128000,
}
```

在 `Info()` 方法中根据 `p.config.Model` 查找对应的上下文大小。

### Q8: 如何处理 BaseURL 兼容？

许多 Provider 支持自定义 BaseURL（用于代理或私有部署）。在构造函数中：

```go
baseURL := cfg.BaseURL
if baseURL == "" {
    baseURL = "https://api.xxx.com/v1"  // 官方默认地址
}
cfg.BaseURL = strings.TrimRight(baseURL, "/")
```

---

## 附录：文件结构参考

```
internal/llm/
├── types.go                          # 核心接口和类型定义
├── provider_helpers.go               # 共享辅助函数
├── provider_template.go              # Provider 模板代码
├── provider_template_test.go         # 测试模板代码
├── multimodal_types.go               # 多模态类型定义
├── multimodal_provider.go            # 多模态适配器
├── structured.go                     # 结构化输出
├── openai_provider.go                # OpenAI Provider
├── openai_test.go                    # OpenAI 测试
├── openai_multimodal_provider.go     # OpenAI 多模态
├── anthropic_provider.go             # Anthropic Provider
├── anthropic_vision_provider.go      # Anthropic 视觉
├── gemini_provider.go                # Gemini Provider
├── gemini_multimodal_provider.go     # Gemini 多模态
├── ollama_provider.go                # Ollama Provider
├── azure_provider.go                 # Azure OpenAI Provider
├── cohere_provider.go                # Cohere Provider
├── mistral_provider.go               # Mistral Provider
├── glm_provider.go                   # 智谱 GLM Provider
├── glm_provider_test.go              # 智谱 GLM 测试
├── qwen_provider.go                  # 通义千问 Provider
├── qwen_provider_test.go             # 通义千问 测试
└── CONTRIBUTING.md                   # 本文档
```

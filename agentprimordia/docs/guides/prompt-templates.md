# Prompt 模板

Prompt 模块提供基于 Go 标准库 `text/template` 的模板引擎，支持变量注入、条件渲染、循环和内置函数。

## 快速开始

```go
tmpl := prompt.NewTemplate(`
你是一个 {{ .role }}。
请根据以下上下文回答问题：
{{ .context }}

问题：{{ .question }}
`).WithVar("role", "技术专家").
   WithVar("context", "AgentPrimordia 是 Go 语言 AI Agent 框架。").
   WithVar("question", "它支持哪些编排模式？")

output, err := tmpl.Render()
```

## 自定义分隔符

```go
tmpl := prompt.NewTemplate(`你好 <% .name %>`).
    WithDelimiters("<%", "%>")
```

## 批量注入变量

```go
tmpl.WithVars(map[string]any{
    "name":    "Agent",
    "version": "1.0",
})
```

## 验证器

```go
tmpl.AddValidator(func(vars map[string]any) error {
    if vars["question"] == "" {
        return errors.New("question 不能为空")
    }
    return nil
})
```

## 少样本示例

```go
fs := prompt.NewFewShot()
fs.AddExample(
    prompt.Example{Input: "你好", Output: "你好！有什么可以帮你的吗？"},
)
fs.AddExample(
    prompt.Example{Input: "再见", Output: "再见，祝你今天愉快！"},
)

promptText, err := fs.Render("请问 Go 1.26 有什么新特性？")
```

## 内置函数

| 函数 | 说明 |
|------|------|
| `json` | 转 JSON 字符串 |
| `indent` | 格式化 JSON |
| `upper` / `lower` / `title` | 大小写转换 |
| `join` | 字符串拼接 |
| `contains` | 包含判断 |
| `hasPrefix` / `hasSuffix` | 前缀/后缀判断 |
| `replace` | 字符串替换 |

## 下一步

- 查看 [API 参考](../api/agent.md) 中的提示相关接口

# AgentPrimordia Agent 市场协议规范 v1.0

> 状态：Stable | 双语言实现：Go `internal/agent/marketplace/` + TS `src/marketplace/`

## 1. 概述

Agent 市场协议定义了 Agent 模板的发布、发现、验证和部署标准。
Go 和 TypeScript SDK 共享同一 JSON wire format（snake_case 字段命名）。

## 2. AgentTemplate 协议

### 2.1 JSON Schema

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "title": "AgentTemplate",
  "type": "object",
  "required": ["id", "name", "version", "author", "system_prompt", "category"],
  "properties": {
    "id":               { "type": "string", "minLength": 1, "pattern": "^[a-z0-9-]+$" },
    "name":             { "type": "string", "minLength": 1, "maxLength": 128 },
    "description":      { "type": "string", "maxLength": 1024 },
    "version":          { "type": "string", "pattern": "^\\d+\\.\\d+\\.\\d+$" },
    "author":           { "type": "string", "minLength": 1 },
    "category":         { "enum": ["research", "coding", "analysis", "chat", "automation"] },
    "tags":             { "type": "array", "items": { "type": "string" }, "maxItems": 10 },
    "system_prompt":    { "type": "string", "minLength": 1 },
    "default_provider": { "type": "string" },
    "default_model":    { "type": "string" },
    "max_turns":        { "type": "integer", "minimum": 1, "maximum": 200 },
    "tools":            { "type": "array", "items": { "type": "string" } },
    "memory_strategy":  { "enum": ["none", "conversation", "semantic", "hybrid"] },
    "temperature":      { "type": "number", "minimum": 0, "maximum": 2 },
    "config":           { "type": "object" },
    "rating":           { "type": "number", "minimum": 0, "maximum": 5 },
    "downloads":        { "type": "integer", "minimum": 0 },
    "created_at":       { "type": "string", "format": "date-time" },
    "updated_at":       { "type": "string", "format": "date-time" }
  }
}
```

### 2.2 版本管理

- 模板版本遵循 **语义版本号**（SemVer）：`MAJOR.MINOR.PATCH`
- MAJOR：不兼容的 system_prompt 或 tools 变更
- MINOR：新增能力（tools / memory_strategy）
- PATCH：描述/标签/配置修正

### 2.3 验证规则

| 字段 | 规则 | 错误消息 |
|------|------|---------|
| id | 非空，仅 `[a-z0-9-]` | "id is required" |
| name | 非空，≤128 字符 | "name is required" |
| version | SemVer 格式 | "version is required" |
| author | 非空 | "author is required" |
| system_prompt | 非空 | "system_prompt is required" |
| category | 枚举值之一 | "invalid category" |
| tools | 引用已注册工具名 | "unknown tool: {name}" |

## 3. 注册表 API

### 3.1 端点

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/v1/templates` | 列出所有模板（支持 `?category=&tag=&q=` 过滤） |
| GET | `/v1/templates/{id}` | 获取单个模板 |
| POST | `/v1/templates` | 发布模板（需签名） |
| DELETE | `/v1/templates/{id}` | 撤回模板 |
| POST | `/v1/templates/{id}/deploy` | 部署模板为 Agent 实例 |

### 3.2 部署协议

请求体（`DeployConfig`）：
```json
{
  "template_id": "code-reviewer",
  "provider_override": "anthropic",
  "model_override": "claude-sonnet-4-20250514",
  "max_turns_override": 20,
  "config_override": {}
}
```

响应体（`DeployResult`）：
```json
{
  "success": true,
  "template_id": "code-reviewer",
  "message": "Agent deployed successfully",
  "agent_config": { "name": "code-reviewer", "model": "claude-sonnet-4-20250514" }
}
```

## 4. 安全要求

- 发布模板必须附带 cosign 签名（ECDSA P-256 + SHA-256）
- 客户端安装时验证签名，`--skip-verify` 仅限开发模式
- system_prompt 中禁止包含注入模式（由 GuardrailEngine 扫描）
- tools 字段引用的工具必须在目标环境已注册

## 5. 跨语言一致性

| 实现 | 位置 | 验证 |
|------|------|------|
| Go | `internal/agent/marketplace/template.go` | `go test ./internal/agent/marketplace/` |
| TypeScript | `src/marketplace/types.ts` + `validator.ts` | `vitest run src/marketplace/` |
| 跨语言测试 | `cross-language-spec.json` → `marketplace_template` 套件 | CI 自动检查 |

JSON 字段命名统一使用 **snake_case**（Go struct tag 与 TS interface 字段名完全一致）。

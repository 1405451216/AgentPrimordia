# Skill 格式规范（v3.4）

本文档定义 AgentPrimordia 技能（Skill）的 JSON 格式与编写指南。技能是可复用的多步骤能力单元，由 Agent 在运行中习得、验证、沉淀。

## 顶层结构

```json
{
  "id": "skill-abc123",
  "name": "数据异常修复",
  "description": "从监控数据中检测并修复异常",
  "version": {"major": 1, "minor": 0, "patch": 0},
  "status": "active",
  "steps": [ ... ],
  "input":  { "fields": {"target": "string"}, "required": ["target"] },
  "output": { "fields": {"fixed": "boolean"} },
  "dependencies": [],
  "tags": ["data", "repair"],
  "metadata": {},
  "successRate": 0.92,
  "usageCount": 25
}
```

## 字段说明

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | string | 全局唯一标识，自动生成 |
| `name` | string | 技能名称（必填，非空） |
| `description` | string | 技能描述，用于语义匹配 |
| `version` | object | SemVer 版本；同 `major` 向后兼容 |
| `status` | enum | `draft` → `verified` → `active` → `deprecated` |
| `steps` | array | 有序步骤列表（至少 1 个） |
| `input`/`output` | object | 输入/输出 schema |
| `tags` | array | 标签，提升匹配命中率 |

## 步骤定义

```json
{
  "id": "s1",
  "description": "查询异常",
  "toolName": "query_anomaly",
  "inputMapping": {"target": "$.input.target"},
  "outputKey": "anomalies",
  "dependsOn": []
}
```

- `toolName` 必须存在于工具注册表（含 Scope 权限校验 + MCP 工具）。
- `dependsOn` 声明步骤依赖，**禁止循环依赖**（校验器会拒绝）。
- `inputMapping` 将技能输入或上游 `outputKey` 映射到工具参数。

## 生命周期

1. **习得**：成功工具调用轨迹经 LLM 提炼为 `draft` 技能。
2. **验证**：新技能必须通过测试用例（验证门）才可 `activate`。
3. **匹配**：`active` 技能参与运行时语义匹配，置信度分三档：
   - `high`（≥0.8）自动调用
   - `medium`（≥0.5）建议调用
   - `low` 不匹配
4. **淘汰**：成功率持续低于阈值的技能应 `deprecate`。

## 版本兼容

- 同 `major` 版本视为向后兼容（`IsCompatible`）。
- 破坏性变更需 `BumpMajor`，调用方应检测兼容性。

## 安全

校验器对高风险工具（如 `shell_exec`）发出安全警告；发布到市场前需通过 `SecurityScan`。

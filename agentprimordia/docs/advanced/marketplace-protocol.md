# Agent 市场分发协议设计

> **状态**: Draft (v3.2 候选)
> **优先级**: P2
> **预期收益**: 生态建设 — 标准化 Agent 模板打包、分发、安装流程

## 1. 概述

Agent 市场（`internal/agent/marketplace/`）当前支持模板注册、评分、一键部署和 cosign 验签。
本协议定义标准化的 **打包格式** 和 **分发流程**，使社区贡献者可以发布 Agent 模板，
用户可以从市场安装并运行。

## 2. 打包格式

### 2.1 Agent Package（`.apkg`）

```
my-agent.apkg (tar.gz)
├── manifest.json        # 元数据清单
├── agent.yaml           # Agent 配置（ReActConfig 序列化）
├── tools/               # 自定义工具定义
│   ├── search.json      # Tool Schema
│   └── search.wasm      # 可选：WASM 工具实现
├── prompts/             # 提示词模板
│   └── system.md
├── tests/               # 评测用例
│   └── eval.json
└── signature.sig        # Ed25519 / cosign 签名
```

### 2.2 manifest.json

```json
{
  "apiVersion": "marketplace.agentprimordia.io/v1",
  "kind": "AgentTemplate",
  "metadata": {
    "name": "code-reviewer",
    "version": "1.2.0",
    "author": "community@example.com",
    "license": "MIT",
    "tags": ["code", "review", "go"],
    "minFrameworkVersion": "2.0.0"
  },
  "spec": {
    "model": {
      "provider": "openai",
      "name": "gpt-4",
      "fallback": ["anthropic/claude-3.5-sonnet"]
    },
    "tools": ["search", "filesystem"],
    "memory": {
      "type": "rag",
      "vectorStore": "builtin"
    },
    "guardrails": ["pii_detection", "injection_filter"],
    "resources": {
      "maxConcurrency": 5,
      "timeoutSeconds": 300
    }
  }
}
```

## 3. 分发协议

### 3.1 Registry API

```
POST   /api/marketplace/templates          # 发布模板
GET    /api/marketplace/templates          # 列表（分页 + 标签过滤）
GET    /api/marketplace/templates/{name}   # 详情
GET    /api/marketplace/templates/{name}/download  # 下载 .apkg
DELETE /api/marketplace/templates/{name}   # 下架（仅作者）
POST   /api/marketplace/templates/{name}/rate      # 评分
GET    /api/marketplace/templates/{name}/reviews   # 评论
```

### 3.2 发布流程

```
贡献者                        Registry                       用户
  │                             │                             │
  ├─ ap market pack ──→ .apkg  │                             │
  ├─ cosign sign .apkg ──────→ │                             │
  ├─ ap market publish ──────→ │                             │
  │                             ├─ 验证签名                   │
  │                             ├─ 运行 eval.json 门禁        │
  │                             ├─ 安全扫描（工具白名单）      │
  │                             ├─ 入库 + 索引                │
  │                             │                             │
  │                             │    ←── ap market install ──┤
  │                             ├──→ 下载 .apkg ────────────→│
  │                             │                             ├─ 验证签名
  │                             │                             ├─ 解包
  │                             │                             ├─ 注册 Agent
  │                             │                             └─ 就绪
```

### 3.3 安全要求

| 层级 | 措施 |
|------|------|
| 签名 | cosign keyless（OIDC）或 Ed25519 密钥对 |
| 工具白名单 | 仅允许 `internal/tools/builtin/` 中的工具或已签名 WASM |
| 沙箱 | WASM 工具在 wazero 沙箱中执行，资源限制（内存/CPU/时间） |
| 审核 | 首次发布需人工审核（自动化门禁通过后） |
| 版本锁定 | 安装时锁定版本，升级需显式操作 |

## 4. CLI 集成

```bash
# 打包
ap market pack ./my-agent/ -o my-agent.apkg

# 签名
ap market sign my-agent.apkg --key cosign.key

# 发布
ap market publish my-agent.apkg --registry https://market.agentprimordia.io

# 搜索
ap market search "code review" --tag go

# 安装
ap market install code-reviewer@1.2.0

# 评分
ap market rate code-reviewer --stars 5 --comment "Excellent!"
```

## 5. 与现有代码的对接

| 现有组件 | 对接方式 |
|----------|----------|
| `internal/agent/marketplace/registry.go` | 扩展 TemplateRegistry 支持 .apkg 格式 |
| `internal/tools/registry/` | 插件安装器复用 SemVer 版本管理 |
| `wasm/tool_executor.go` | WASM 工具沙箱执行 |
| `internal/security/sandbox.go` | 工具权限校验 |
| `pkg/marketplace.go` | 公共 API 导出 |
| `studio/web/MarketplacePage` | 前端市场浏览/安装 UI |

## 6. 社区生态启动计划

| 阶段 | 目标 | 时间 |
|------|------|------|
| Alpha | 内部 5 个官方模板 + CLI 流程跑通 | v3.2 |
| Beta | 开放社区提交 + 自动门禁 | v3.3 |
| GA | 评分/评论/推荐 + 生态合作伙伴 | v4.0 |

## 7. 开放问题

- [ ] Registry 托管方案：自建 vs GitHub Releases vs OCI Registry
- [ ] 模板依赖管理：Agent 模板之间的依赖关系
- [ ] 多语言模板：Go 模板 vs TS 模板 vs 混合
- [ ] 收益分成：社区贡献者的激励机制

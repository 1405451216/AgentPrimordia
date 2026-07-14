# AgentPrimordia Studio 设计文档

> 日期：2026-07-15
> 状态：已批准
> 范围：5 个方向统一为单一平台

## 概述

**AgentPrimordia Studio** 是一个本地开发工具，通过 `ap studio` 命令启动。
它把 AgentPrimordia 框架的 5 个方向整合为一个统一的 Web 平台：

1. **模板库 + 运行器** — 一键启动 agent，对话界面
2. **记忆管理器** — 跨会话持久化、搜索、编辑
3. **评估中心** — 用例评估、质量分数、趋势
4. **多模态查看器** — 图片 + 文字对话
5. **开箱即用** — Studio 本身就是产品

## 设计原则

- **模板优先**：用户打开看到的是模板库，不是空白画布。10 秒内运行第一个 agent。
- **零外部依赖**：SQLite 本地存储，无需 Postgres/Redis/向量数据库。
- **本地优先**：默认 `localhost:8765`，可选远程访问（带密码）。
- **渐进式**：模板 → YAML 配置 → 可视化编辑器（高级模式）。

## 整体架构

```
┌─────────────────────────────────────────────────────┐
│              AgentPrimordia Studio                   │
│                   (ap studio)                        │
│                                                      │
│  ┌──────────────┐        ┌──────────────────────┐  │
│  │   Frontend   │        │      Backend (Go)    │  │
│  │  React + TS  │◄──────►│   HTTP + WebSocket   │  │
│  │   (Vite)     │  WS    │   (existing ap pkg)  │  │
│  └──────────────┘        └──────────────────────┘  │
│         │                       │                  │
│         │                       ▼                  │
│         │              ┌──────────────────────┐    │
│         │              │   Studio Services    │    │
│         │              │                      │    │
│         │              │ • TemplateRunner     │    │
│         │              │ • MemoryManager      │    │
│         │              │ • EvalRunner         │    │
│         │              │ • MultimodalViewer   │    │
│         │              │ • WorkflowEditor     │    │
│         │              └──────────────────────┘    │
│         │                       │                  │
│         │                       ▼                  │
│         │              ┌──────────────────────┐    │
│         └──────────────│   SQLite (local)     │    │
│                file:   │   ~/.ap/studio.db    │    │
│                ~.ap/   └──────────────────────┘    │
│                                                      │
└─────────────────────────────────────────────────────┘
```

## 技术栈

| 层 | 技术 | 说明 |
|----|------|------|
| 后端 | Go (existing `pkg/ap`) | HTTP API + WebSocket |
| 前端 | React + TS + Vite | SPA，内嵌到 Go 二进制 |
| 画布 | reactflow | 仅高级模式（可视化编辑器） |
| 数据 | SQLite | `~/.ap/studio.db` + `~/.ap/memory.db` |
| 打包 | `//go:embed` | 前端构建产物嵌入 Go 二进制 |

## 页面结构

| 页面 | 功能 |
|------|------|
| `/` | 模板库 + 最近运行 |
| `/chat/:run_id` | Agent 运行器（对话界面） |
| `/memory` | 记忆管理器 |
| `/eval` | 评估中心 |
| `/multimodal` | 多模态查看器 |
| `/workflows` | 可视化编辑器（高级模式） |
| `/settings` | Provider 配置 + 安全设置 |

## 子系统详细设计

### 1. 模板库 + 运行器

**模板定义（YAML）**：

```yaml
name: 工具助手
description: 带文件系统和 Shell 工具的 agent
version: "1.0"

agent:
  model: openai/gpt-4o-mini
  max_turns: 10
  prompt: 你是一个可以读写文件、执行命令的助手。

tools:
  - filesystem
  - shell
  - web

memory:
  persistent: true
```

**内置模板**：
- `basic-assistant` — 最简 agent
- `tool-agent` — 带工具
- `rag-qa` — RAG 知识问答
- `code-review` — 代码审查（多 agent Pipeline）
- `data-analysis` — 数据分析
- `web-research` — Web 研究
- `multimodal` — 多模态

**运行流程**：选模板 → 配模型 → 运行 → 对话界面

**对话界面功能**：
- 流式输出（token-by-token）
- 思考过程显示
- 工具调用展示
- 统计面板（Turn/Tokens/Cost）
- 停止/导出

### 2. 记忆管理器

**核心功能**：
- 跨会话持久化（SQLite）
- 全文搜索（FTS5）+ 语义搜索（向量相似度）
- 编辑记忆条目
- 重要性管理（手动调整 + 自动归档）
- 导入/导出（JSON/Markdown）
- 加入知识库（标记为 RAG 源）

**数据存储**：
- `~/.ap/memory.db` — 持久化记忆
- 复用现有 `internal/memory` 的 SQLite 后端 + `MemoryExporter` 接口

### 3. 评估中心

**用例定义（YAML）**：

```yaml
cases:
  - name: list_python_files
    input: "列出当前目录的 Python 文件"
    expected_keywords: [".py", "main.py"]
    evaluator: keyword

  - name: greeting
    input: "你好"
    expected_keywords: ["你好", "您好", "hello"]
    evaluator: llm
```

**评估器类型**：
| 评估器 | 说明 |
|--------|------|
| 关键词匹配 | 现有 `ContainsAnyEvaluator` |
| LLM 评分 | 用 LLM 评判输出质量（1-5 分 + 理由） |
| 结构化输出 | 检查 JSON 输出是否符合 schema |
| 安全检测 | 检查是否泄露 PII/敏感信息 |

**功能**：
- 运行用例集，显示每个用例的得分
- 平均分、通过率
- 历史趋势图（跨版本对比）
- 导出报告（JSON/Markdown）

### 4. 多模态查看器

**功能**：
- 上传图片（拖拽或点击），预览缩略图
- 多图支持
- 图片 + 文字混合输入
- 模型选择：Claude 3.5、GPT-4o、Gemini 1.5、Ollama（llava）
- 图片信息展示（文件名、大小、尺寸、格式）

**技术**：复用现有 `AnthropicVisionProvider`、`GLMProvider`（image_url）、`OpenAIProvider`（vision）。

### 5. 可视化编辑器（高级模式）

**节点类型**：
| 节点类型 | 对应框架 API |
|---------|-------------|
| Agent | `ap.NewAgent().Run()` |
| Tool | `registry.Execute()` |
| Condition | DAG condition edge |
| Delay | `time.Sleep` |
| Merge | DAG join |
| RAG | `RAGStore.Search()` |
| Handoff | `HandoffProtocol` |

**功能**：
- 拖拽节点到画布（reactflow）
- 连线定义边
- 点击节点编辑属性
- 运行时实时显示节点状态（颜色编码）
- 保存/加载工作流定义（SQLite）

**技术**：reactflow（React 流程图库）

### 6. 设置

**功能**：
- LLM Provider 配置（OpenAI/Anthropic/Ollama API Key）
- API Key 测试连接
- 记忆存储配置
- Studio 访问密码（可选）
- 远程访问开关

## CLI 命令

```bash
# 启动 Studio（默认 localhost:8765）
ap studio

# 指定端口
ap studio --port 9000

# 允许远程访问（带密码）
ap studio --host 0.0.0.0 --password mysecret

# 不自动打开浏览器
ap studio --no-open
```

## 数据模型（SQLite）

```sql
-- 模板
CREATE TABLE templates (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  description TEXT,
  yaml TEXT NOT NULL,  -- YAML 定义
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 运行记录
CREATE TABLE runs (
  id TEXT PRIMARY KEY,
  template_id TEXT,
  status TEXT,  -- running/success/failed/timeout
  started_at DATETIME,
  ended_at DATETIME,
  metrics TEXT,  -- JSON: turns/tokens/cost
  FOREIGN KEY (template_id) REFERENCES templates(id)
);

-- 消息
CREATE TABLE messages (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL,
  role TEXT NOT NULL,  -- user/assistant/tool
  content TEXT,
  metadata TEXT,  -- JSON: tool calls, etc.
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (run_id) REFERENCES runs(id)
);

-- 工作流定义
CREATE TABLE workflows (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  definition TEXT NOT NULL,  -- JSON: nodes + edges
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 工作流运行
CREATE TABLE workflow_runs (
  id TEXT PRIMARY KEY,
  workflow_id TEXT NOT NULL,
  status TEXT,
  node_results TEXT,  -- JSON: node_id -> result
  started_at DATETIME,
  ended_at DATETIME,
  FOREIGN KEY (workflow_id) REFERENCES workflows(id)
);

-- 评估用例
CREATE TABLE eval_cases (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  input TEXT NOT NULL,
  expected TEXT,
  evaluator TEXT NOT NULL,  -- keyword/llm/structured/safety
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 评估结果
CREATE TABLE eval_results (
  id TEXT PRIMARY KEY,
  case_id TEXT NOT NULL,
  run_id TEXT,
  score REAL,
  passed BOOLEAN,
  details TEXT,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (case_id) REFERENCES eval_cases(id)
);
```

## 后端 API

```
GET  /api/templates              # 模板列表
GET  /api/templates/:id          # 模板详情
POST /api/runs                   # 启动运行
GET  /api/runs                   # 运行列表
GET  /api/runs/:id               # 运行详情
GET  /api/runs/:id/messages      # 消息列表
POST /api/runs/:id/stop          # 停止运行
WS   /api/runs/:id/stream        # 流式输出

GET  /api/memory/sessions        # 会话列表
GET  /api/memory/sessions/:id    # 会话详情
GET  /api/memory/search          # 搜索记忆
PUT  /api/memory/entries/:id     # 编辑记忆
DELETE /api/memory/entries/:id   # 删除记忆

GET  /api/eval/cases             # 用例列表
POST /api/eval/run               # 运行评估
GET  /api/eval/results           # 评估结果
GET  /api/eval/trends            # 历史趋势

GET  /api/workflows              # 工作流列表
POST /api/workflows              # 创建工作流
PUT  /api/workflows/:id          # 更新工作流
POST /api/workflows/:id/run      # 运行工作流

GET  /api/settings/providers     # Provider 配置
PUT  /api/settings/providers     # 更新配置
POST /api/settings/providers/test # 测试连接
```

## 实现优先级

1. **Phase 1**：模板库 + 运行器（核心产品形态）
2. **Phase 2**：记忆管理器 + 设置
3. **Phase 3**：评估中心
4. **Phase 4**：多模态查看器
5. **Phase 5**：可视化编辑器（高级模式）

## 验收标准

- [ ] `ap studio` 一键启动，自动打开浏览器
- [ ] 10 秒内运行第一个 agent（选模板 → 运行）
- [ ] 流式输出正常工作
- [ ] 记忆跨会话持久化
- [ ] 评估用例可运行并显示分数
- [ ] 多模态图片上传 + 对话
- [ ] 零外部依赖（SQLite only）
- [ ] 前端构建产物内嵌到 Go 二进制

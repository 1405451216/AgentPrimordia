# Phase 11: TypeScript SDK 完善 — 实施计划

> **日期**: 2026-06-05
> **状态**: Plan Complete (Code 待实施)
> **前置条件**: Phase 7 (CI 含 TS SDK 测试) + Phase 8.2 (生态模板稳定)
> **后续**: Phase 12 (工具链自动化)

---

## 总览

`sdk/typescript/` 已有基础 SDK 实现(31 个 .ts 文件,308 行测试,110 行 example),但**完整度不足**:

- 文档缺失(无 README / API 文档 / 入门指南)
- 缺跨 SDK 集成测试(Go server + TS client 真实通信未验证)
- CI 集成深度不够(仅跑 vitest,未跑 lint / type check / build)
- 无 SDK 文档站

Phase 11 补齐这 4 项,让 SDK 可被前端/Node.js 用户开箱使用。

| # | 子目标 | 落地形式 | 状态 |
|:-:|--------|----------|:----:|
| 11.1 | SDK README + API 文档 | `sdk/typescript/README.md` + TSDoc 补全 | ⏳ |
| 11.2 | 跨 SDK 集成测试 | Go HTTP server (mock) + TS client 真实通信 | ⏳ |
| 11.3 | CI 强化 | type check / lint / build / coverage 一体化 | ⏳ |
| 11.4 | SDK 文档站 | VitePress 站点 (单仓库) | ⏳ |
| 11.5 | SDK 入门指南 | `getting-started.md` + 完整 example | ⏳ |

---

## 子阶段 11.1: SDK README + TSDoc 补全

### sdk/typescript/README.md (新建)

```markdown
# @agentprimordia/sdk

TypeScript SDK for AgentPrimordia — Universal AI Agent Development Framework.

## 安装

npm install @agentprimordia/sdk

## 快速开始

\`\`\`typescript
import { ReActAgent, MockProvider, ToolRegistry } from '@agentprimordia/sdk';

const provider = new MockProvider({ response: 'Hello!' });
const registry = new ToolRegistry();

const agent = new ReActAgent({
  name: 'my-agent',
  model: provider,
  toolkit: registry,
  maxTurns: 5,
});

const response = await agent.run('Hi');
console.log(response.content);
\`\`\`

## API 概览

### Core
- `ReActAgent` — ReAct 循环主类
- `MockProvider` — 测试用 LLM Provider
- ...

### Memory
- `InMemoryStore` — 内存存储
- `VectorStore` — 向量存储
...

## 完整文档

[docs.agentprimordia.dev/sdk](https://docs.agentprimordia.dev/sdk)

## License

Apache-2.0
```

### TSDoc 补全

为每个 export 类/函数/接口补 godoc 风格注释:
- 描述
- @param
- @returns
- @example (运行示例)
- @throws (错误情况)

`src/index.ts` 当前仅 52 行(纯 export),需重写为:

```typescript
/**
 * @agentprimordia/sdk 主入口
 *
 * AgentPrimordia TypeScript SDK — 构建跨平台 AI Agent 应用。
 * @packageDocumentation
 */

export * from './agent/react-loop.js';
export * from './llm/index.js';
// ... 其他模块
```

---

## 子阶段 11.2: 跨 SDK 集成测试

### 目标

验证 TS SDK 与 Go server 真实 HTTP 通信(SDK 不只是 client,也是"未来 Go server 的契约")。

### 实施

**Go 端**(test helper):
- 启动 `httptest.NewServer` 提供 mock LLM endpoint
- 接收 TS SDK 的 HTTP 请求,返回 mock SSE 流

**TS 端**(测试用例):
- 用 SDK 的 HTTP transport 调 mock server
- 验证请求格式 / SSE 解析 / 错误传播

### 文件结构

```
sdk/typescript/tests/
├── sdk.test.ts          # 单元测试 (已有)
├── e2e/
│   ├── go_server.test.ts  # 跨 SDK 集成测试
│   └── setup.ts            # 启动 Go mock server
sdk/integration_tests/
├── main.go               # Go mock server,被 TS 测试启动
└── go.mod
```

### 简化方案

如不想做 Go/TS 双语言 e2e,可仅做:
- **TS 内部集成测试**: mock HTTP server (nock / msw)
- **Go 端**: 提供 example HTTP server 在 `pkg/`

---

## 子阶段 11.3: CI 强化

### 当前 CI 状态

`.github/workflows/ci.yml` 已有:
```yaml
ts-test:
  - npm ci
  - npm run build
  - npm test
```

### 强化项

| 检查 | 工具 | 状态 |
|------|------|:----:|
| Type check | `tsc --noEmit` | 缺 |
| Lint | `eslint` | 缺 |
| Coverage | `vitest --coverage` | 缺 |
| Bundle size | `size-limit` | 缺 |
| Lint (Go SDK server) | `golangci-lint` | 已有 |

### 实施

在 `.github/workflows/ci.yml` 的 `ts-test` 阶段补:
```yaml
- name: Type check
  run: npx tsc --noEmit
- name: Lint
  run: npm run lint
- name: Coverage
  run: npm test -- --coverage
- name: Upload coverage
  if: matrix.go-version == '1.23'  # 与 Go 同步
  uses: actions/upload-artifact@v4
  with:
    name: ts-coverage
    path: sdk/typescript/coverage
```

### 覆盖率门槛

TS SDK 引入 tier 门槛:
- 核心 (`src/agent/`): ≥ 80%
- LLM / Memory: ≥ 70%
- 边缘 (utilities): ≥ 50%

---

## 子阶段 11.4: SDK 文档站

### 选型

| 方案 | 优点 | 缺点 |
|------|------|------|
| VitePress | Vue 驱动,文档专用,简单 | 需 Node 构建 |
| Docusaurus | React 驱动,生态广 | 较重 |
| TypeDoc | 自动从 TSDoc 生成 | 定制性低 |

**推荐**: **VitePress**(轻量,文档站标准)。

### 实施

```
sdk/typescript/
├── docs/                  # VitePress 源
│   ├── .vitepress/
│   │   └── config.ts
│   ├── index.md          # 首页
│   ├── guide/
│   │   ├── getting-started.md
│   │   ├── agent.md
│   │   ├── memory.md
│   │   └── tools.md
│   └── api/
│       └── index.md      # 自动生成 (typedoc)
├── package.json          # 加 docs:dev / docs:build
└── ...
```

### 自动生成 API 文档

`typedoc-plugin-markdown` 从 TSDoc 生成 markdown:

```bash
npx typedoc --out docs/api/ src/index.ts
```

### CI 部署

VitePress 静态站 → GitHub Pages:
- `actions/deploy-pages@v4`
- 仅 main 分支触发

---

## 子阶段 11.5: SDK 入门指南

### getting-started.md (VitePress 文档)

```markdown
# 快速开始

## 5 分钟上手

### 1. 安装

\`\`\`bash
npm install @agentprimordia/sdk
\`\`\`

### 2. 创建 Agent

\`\`\`typescript
import { ReActAgent, OpenAIProvider, ToolRegistry } from '@agentprimordia/sdk';

const provider = new OpenAIProvider({
  apiKey: process.env.OPENAI_API_KEY,
  model: 'gpt-4o',
});

const agent = new ReActAgent({
  name: 'my-agent',
  model: provider,
  toolkit: new ToolRegistry(),
  maxTurns: 10,
});

const response = await agent.run('Hello!');
console.log(response.content);
\`\`\`

### 3. 添加工具

\`\`\`typescript
class WeatherTool implements Tool {
  name = 'get_weather';
  // ...
}

agent.toolkit.register(new WeatherTool());
\`\`\`

### 下一步

- [Agent 生命周期与钩子](./agent.md)
- [Memory 与对话管理](./memory.md)
- [工具系统](./tools.md)
- [完整 API 参考](/api/)
\`\`\`
```

### 完整 example

`sdk/typescript/examples/agent-with-tools.ts` (新增):
- 展示 Tool / Hook / Memory 组合
- 配 README 引用

---

## 验证结果(预)

### SDK 测试

| 命令 | 预期结果 |
|------|---------|
| `npm run build` | ✅ 编译 |
| `npm test` | ✅ 现有测试全过 |
| `npm run lint` | ✅ ESLint 通过 |
| `npx tsc --noEmit` | ✅ 类型检查 |
| `npm test -- --coverage` | ✅ 覆盖率基线 |

### 文档站

| 命令 | 预期结果 |
|------|---------|
| `npm run docs:dev` | ✅ 本地预览 |
| `npm run docs:build` | ✅ 静态站生成 |
| GitHub Pages 部署 | ✅ main 触发 |

### 提交规模

- **5 个 commit**,每个子阶段 1 个
- 新增文件: README.md + docs/ (~10 个) + example 升级
- CI 配置: 1 处修改

---

## 风险与债务

### 高优先级

1. **TS SDK 与 Go 端 API 不对称** — Go 端是权威,TS 端跟随。Go 改动时 TS 可能未同步
   - 解决: Phase 12 引入 `make api-diff` 对比两侧 export

2. **VitePress 集成 CI** — GitHub Pages 部署需配置 PAT
   - 解决: 用官方 `actions/deploy-pages` action

### 中优先级

3. **TS 覆盖率门槛** — TS 测试实践不一致,可能与 Go 端规则不同
   - 解决: 单独维护 TS tier 门槛(见 11.3)

4. **跨 SDK e2e 复杂度** — Go 启动 + TS 测试协调
   - 简化: Phase 11 仅做 TS 内部 mock,不真起 Go server

### 低优先级

5. **VitePress vs Docusaurus 选型** — 长期可能改
   - 接受: VitePress 轻量起步

---

## 后续工作候选 (Phase 12+)

- Phase 12: 工具链自动化 (make api-diff)
- Phase 13: 真实 LLM e2e (需要 OPENAI_API_KEY)
- Phase 14: SDK 多版本兼容 (semver 自动同步)

---

## 反思:Phase 11 暴露的依赖

- **TS SDK 长期未维护** — `vitest` 配置存在但覆盖率门槛未集成
- **跨 SDK 同步无机制** — Go 改 export,TS 不一定同步
- **文档站缺失** — 用户首次接触 SDK 只能看 godoc

Phase 11 解决前 2 项,Phase 12+ 解决第 3 项。

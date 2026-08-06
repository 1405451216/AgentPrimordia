# AgentPrimordia Studio (Web)

可视化操作台，面向 AgentPrimordia 运维与实验场景。

## 页面

| 路由 | 页面 | 说明 |
|------|------|------|
| `/` | 概览 | 原初体主视觉 + 集群 / 蒸馏 / 实验聚合 |
| `/chaos` | 混沌实验 | 混沌实验创建 / 运行 / 报告（破坏性操作两步确认） |
| `/cluster` | 集群 | 节点拓扑 + 分片分布 + 领导者状态 + 降级告警 |
| `/learning` | 学习监控 | 知识蒸馏统计 + 能力进化趋势线 |
| `/marketplace` | Agent 市场 | 模板浏览 / 搜索 / 部署，已部署 Agent 治理 |
| `/help` | 帮助文档 | 面板、快捷键、混沌语义与数据说明 |

## 一键启动

```bash
# Windows（PowerShell）
powershell -ExecutionPolicy Bypass -File scripts\dev-studio.ps1

# macOS / Linux
./scripts/dev-studio.sh
```

脚本会同时启动后端（:8090）与前端（:5173），Ctrl+C 一起停止，并自动打开浏览器。

## 开发

```bash
# 1. 启动 Studio 后端（默认 :8090，内置 demo 数据）
go run ./cmd/studio

# 2. 启动前端
npm install
npm run dev        # http://localhost:5173，/api 代理到 :8090
```

> 开发模式下 `/api` 会被 Vite 代理到 Studio 后端（`go run ./cmd/studio`，默认 `:8090`）。
> 后端 `/api/v1/chaos|cluster|learning|marketplace/*` 端点由 `internal/studio` 实现，
> 默认返回 demo 数据（市场预置 3 个模板、集群单节点、混沌实验内存记录），
> 开箱即可演示；接入真实引擎见 `internal/studio` 的 `WithChaos/WithCluster/WithLearning/WithMarketplace` 选项。

## 构建

```bash
npm run build      # 产物输出到 dist/，可被任意静态服务器或管理后端托管
npm run typecheck  # 仅类型检查
```

## 依赖

- React 19 + react-router-dom 7
- Vite 6 + TypeScript 5
- 零其他运行时依赖（页面直接使用 fetch 调用后端 API）

# AgentPrimordia Studio (Web)

可视化操作台，面向 AgentPrimordia 运维与实验场景。

## 页面

| 路由 | 页面 | 说明 |
|------|------|------|
| `/` | Chaos Lab | 混沌实验创建 / 运行 / 报告 |
| `/cluster` | Cluster Dashboard | 节点拓扑 + 分片视图 + 领导者状态 |
| `/learning` | Learning Monitor | 知识蒸馏统计 + 能力进化趋势 |
| `/marketplace` | Agent Marketplace | 模板浏览 / 搜索 / 一键部署 |

## 开发

```bash
npm install
npm run dev        # http://localhost:5173，/api 代理到 :8080 管理后端
```

> 开发模式下 `/api` 会被 Vite 代理到本地管理后端（`go run ./cmd/admin`，默认 `:8080`）。
> 页面依赖的 `/api/v1/chaos|cluster|learning|marketplace/*` 端点尚未由后端实现，
> 页面会以空态/错误态优雅降级；对应后端落地后可无缝接入。

## 构建

```bash
npm run build      # 产物输出到 dist/，可被任意静态服务器或管理后端托管
npm run typecheck  # 仅类型检查
```

## 依赖

- React 19 + react-router-dom 7
- Vite 6 + TypeScript 5
- 零其他运行时依赖（页面直接使用 fetch 调用后端 API）

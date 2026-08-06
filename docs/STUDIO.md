# AgentPrimordia Studio — 设计文档

> **状态**: 生产可用 · 设计评分 **40/40**（八轮设计批判迭代）
> **技术栈**: React 19 + TypeScript + Vite（前端，零第三方运行时依赖）/ Go 标准库（后端）
> **入口**: `agentprimordia/studio/web`（前端）· `agentprimordia/cmd/studio`（后端）

## 一键启动

脚本同时启动后端（:8090）与前端（:5173），等待就绪后自动打开浏览器，Ctrl+C 一起停止。

```bash
# Windows（PowerShell）
powershell -ExecutionPolicy Bypass -File scripts\dev-studio.ps1

# macOS / Linux
./scripts/dev-studio.sh            # 默认端口
./scripts/dev-studio.sh -b 9000    # 自定义后端端口
```

手动启动等价命令：

```bash
# 后端（:8090，内置 demo 数据，开箱即演示）
cd agentprimordia && go run ./cmd/studio

# 前端（:5173，/api 代理到 :8090）
cd agentprimordia/studio/web && npm install && npm run dev
```

> 生产接入真实引擎：通过 `internal/studio` 的 `WithChaos / WithCluster / WithLearning / WithMarketplace` 注入对应服务替换 demo 实现。

## 面板

| 路由 | 面板 | 能力 |
|------|------|------|
| `/` | 概览 | 原初体主视觉（哈希环分片节点 + 能力脉搏动画）、集群 / 蒸馏 / 实验聚合、近期实验 |
| `/chaos` | 混沌实验 | 混沌实验创建 / 运行 / 报告；破坏性故障（分区/进程终止）两步确认；实验报告下钻；运行中可中止 |
| `/cluster` | 集群 | 节点拓扑、领导者状态、分片比例分布、降级告警横幅、表格排序 |
| `/learning` | 学习监控 | 知识蒸馏统计、能力进化趋势线（Sparkline）、轮询增量闪烁 |
| `/marketplace` | Agent 市场 | 模板搜索 / 分类 / 部署；已部署 Agent 治理（停止 / 重启） |
| `/help` | 帮助文档 | 面板、快捷键、混沌语义、市场、数据来源说明 |

## 设计原则

### 身份（原初涌现）
- **1.5px 圆角描边 SVG 图标集**（`icons.tsx`）替代 emoji，跨平台统一渲染
- **原初体主视觉**：Overview 首页的哈希环——在线节点按分片比例分布为弧段，中心"脉搏"为真实能力平均分动画
- **`--shard-*` 分片色令牌**贯穿集群条与原初体弧线，形成跨页身份
- **中文主标题 + 英文副行**（`PageTitle`）惯例，全站统一

### 运维可靠性
- **破坏性操作两步确认**：进程终止 / 中止 / 停止均有确认对话框 + 影响范围警告
- **绝不静默吞错**：错误面板 + 重试、`res.ok` 校验、逐端点部分失败报告（"部分接口返回异常（集群、能力）"）
- **陈旧提示**：`Staleness` 每秒跳动，超过 30s 标记"数据可能已过期"
- **轮询失败保留旧数据**：显示"（显示上次数据）"而非整页错误

### 一致性工程
- **`useConfirmDialog`**：统一全部模态的初始聚焦 / Esc / Tab 陷阱 / 焦点恢复（含触发元素卸载时的页面标题回退）
- **`useTableSort`**：排序持久化到 URL（`?sort=&dir=`）
- **状态徽章**：字形 + 文字 + 颜色三重编码（非纯色），`status-badge status-*`
- **共享组件**：`ErrorPanel` / `SuccessBanner` / `Staleness` / `FlashValue` / `Sparkline` 全站复用

### 效率
- **快捷键**：`Shift+/` 快捷键面板、`/` 聚焦搜索、`g 1-5` 跳转页面、`Esc` 关闭
- **搜索防抖 + AbortController** 防乱序覆盖；筛选 / 排序 URL 可分享
- 等宽数字（`tabular-nums`）避免轮询数字抖动

### 可访问性
- `:focus-visible` 键盘焦点可见；`role="alert"` / `role="status"` / `aria-label` 齐全
- 导航激活态与主按钮对比度 4.72:1（AA）；`prefers-reduced-motion` 全局尊重
- 键盘导航：焦点恢复、Tab 陷阱、初始聚焦

## 八轮设计迭代（18 → 40/40）

| 轮次 | 评分 | 要点 |
|------|:---:|------|
| 1 | 18 | 初评：破坏性操作无防护、静默吞错、中英混杂 |
| 2 | 22 | 加固：两步确认、错误面板、陈旧提示、骨架屏 |
| 3 | 24 | 身份：SVG 图标、概览页、状态中文化 |
| 4 | 32 | 功能：能力趋势线、分片比例、快捷键、部署治理 |
| 5 | 35 | 一致性：中文标题、确认对称、emoji 清除 |
| 6 | 36 | 模态统一、脉冲真实化、排序持久化 |
| 7 | 37 | 焦点恢复、隐形错误、分类本地化 |
| 8 | **40** | 五处收尾验证落地 → 满分 |

批判快照存档于 `.impeccable/critique/`。

## 测试

```bash
# 前端（Studio 30 例 + 壳层 9 例）
cd agentprimordia/studio/web && npm test

# 后端 handler
cd agentprimordia && go test ./internal/studio/...

# 类型检查
cd agentprimordia/studio/web && npm run typecheck
```

## 构建

```bash
cd agentprimordia/studio/web && npm run build   # 产物输出到 dist/
```

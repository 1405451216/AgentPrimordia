# Phase 4 Implementation Plan

## Overview

Phase 4 是 AgentPrimordia 框架的全面增强阶段，目标是：
1. 补齐与 CodeCast-desktop 参考代码的差距
2. 增强核心能力（ContextTemplate、Memory Timeline、GroupChat）
3. 提升低覆盖率模块
4. 清理技术债务
5. 生产就绪（遥测、可观测性、配置热加载）

## Current State (Post-Phase 3)

- 测试数: ~600+
- 模块数: 16
- 低覆盖率模块: pkg (27.5%), admin (61.5%), events (77.2%), memory (75.7%)
- 遗留 CodeCast 差距: ContextTemplate、Memory Timeline、GetMemoriesByTag、SetImportance、GetImportantMemories
- 技术债务: 1 个 TODO (react_loop.go L300)

## Task Breakdown

### Sub-Phase 4-A: 差距补齐 (P0)

| # | Task | Module | Source | Complexity |
|---|------|--------|--------|------------|
| 44 | ContextTemplate 系统提示词模板 | agent/ | CodeCast G14 | ⭐⭐ |
| 45 | Memory Timeline + 标签/重要性 | memory/ | CodeCast G15 | ⭐⭐ |
| 46 | 低覆盖模块补强 (pkg/admin/events/memory) | various | 质量 | ⭐⭐ |
| 47 | 技术债务清理 | various | 债务 | ⭐ |

### Sub-Phase 4-B: 能力增强 (P1)

| # | Task | Module | Source | Complexity |
|---|------|--------|--------|------------|
| 48 | GroupChat 多 Agent 对话 | agent/ | 新功能 | ⭐⭐⭐ |
| 49 | Agent 状态机完善 | agent/ | 架构 | ⭐⭐ |
| 50 | Memory 语义搜索增强 | memory/ | 增强 | ⭐⭐ |
| 51 | 配置热加载 | config/ | 新功能 | ⭐⭐ |

### Sub-Phase 4-C: 生产就绪 (P2)

| # | Task | Module | Source | Complexity |
|---|------|--------|--------|------------|
| 52 | 遥测与可观测性 | metrics/ | 新功能 | ⭐⭐⭐ |
| 53 | 健康检查端点 | admin/ | 新功能 | ⭐⭐ |
| 54 | 优雅关闭 | agent/ | 架构 | ⭐⭐ |
| 55 | v0.3.0 发布准备 | docs/ | 发布 | ⭐ |

## Execution Order

```
P0 (必须): Task 44 → 45 → 46 → 47
P1 (重要): Task 48 → 49 → 50 → 51
P2 (增强): Task 52 → 53 → 54 → 55
```

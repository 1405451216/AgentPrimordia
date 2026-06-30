# AgentPrimordia v1.0.0 — 正式发布

> **发布日期**: 2026-06-30
> **代号**: Stable (API 稳定性承诺)
> **影响范围**: 全局版本统一 + 文档全面更新 + API 稳定性锁定
> **向后兼容**: ✅ 完全兼容，v0.x API 仍可用

## 主题：版本统一与 API 稳定性承诺

v1.0.0 是 AgentPrimordia 的首个正式稳定版本。Go SDK、TypeScript SDK、CLI 全局统一为 v1.0.0，API 稳定性承诺锁定——Stable API 向后兼容，破坏性变更需大版本（v2.0）。

## 核心变更

### 版本统一

| 组件 | 旧版本 | 新版本 |
|------|--------|--------|
| Go SDK (`pkg.Version`) | 0.8.0 | **1.0.0** |
| TypeScript SDK (`package.json`) | 0.8.0 | **1.0.0** |
| CLI (`ap version`) | 0.8.0 | **1.0.0** |
| go.mod 模板 | v0.0.0 | **v1.0.0** |

### API 稳定性承诺

- **Stable API**：`pkg/` 公共 API 冻结，向后兼容
- **破坏性变更**：仅在大版本（v2.0）中允许
- **废弃策略**：Deprecated API 至少保留一个大版本周期

## 新增功能

### RAG RRF 融合算法

引入 Reciprocal Rank Fusion (Cormack et al., 2009)，解决 Linear 加权量纲不可比问题：

```go
ragStore := ap.NewRAGStore(store, embedder)
ragStore.SetFusionConfig(ap.RAGFusionConfig{
    Mode: ap.RFFFusion,  // Reciprocal Rank Fusion
    RRFK: 60,
    TopK: 10,
})
```

- `LinearFusion`：基于原始分数加权融合（默认）
- `RFFFusion`：基于排名融合，对量纲差异鲁棒
- 双命中加成：FTS + Vector 同时命中的结果获得 2x 分数加成
- Over-fetch 召回：预取 `topK + OverFetchSize` 候选，提升融合质量

### 性能优化

| 组件 | 机制 | 优化效果 |
|------|------|----------|
| Pool 调度器 | `sync.Cond` 动态信号量 | AutoScaler 实时生效，无忙等待 |
| GoroutinePool | `sync.Cond` 通知 | Wait() CPU 占用从 ~100% → ~0% |
| LLM Provider | 共享 HTTP 连接池 | 减少 TCP 连接数，复用 Keep-Alive |
| HookContext | `sync.Pool` 复用 | ReAct 热点路径减少 GC 压力 |
| bytes.Buffer | `sync.Pool` 池化 | 2.2x 加速，0 allocs/op |
| SSE 流式 | Timer-based 背压 | 5s 超时 + 10 丢弃中断 |
| Token 估算 | `len(text)/4` 直接计算 | 0.4ns/op，比 sync.Map 缓存快 100+ 倍 |

### 供应链安全

- govulncheck + npm audit + Trivy 安全扫描
- cosign 签名 + SBOM 生成
- Fuzz 测试：Sandbox / RAG / 工具执行器安全模糊测试

### PGO 性能调优

Profile-Guided Optimization 指南，利用 Go PGO 进一步提升运行时性能。

### CLI 工程化

`ap loop` 子命令扩展：
- `ap loop trace` — 查看 Agent 执行追踪
- `ap loop inspect` — 查看 Agent 当前状态
- `ap loop resume` — 从检查点恢复运行

## 测试覆盖

| 指标 | Go | TypeScript | 合计 |
|------|-----|-----------|------|
| 测试包数 | 47 | 6 | 53 |
| 测试用例 | 2,900+ | 154 | 3,054+ |
| 通过率 | 100% | 100% | 100% |

## 文档更新

- API 参考文档全面重写：`agent.md` / `llm.md` / `tools.md` / `memory.md` / `pool.md` / `a2a.md` / `guardrail.md`
- TypeScript SDK API 参考更新：包含 RRF 融合详情
- `CODE_WIKI.md` 版本标记更新
- `README.md` 版本亮点更新
- 迁移指南更新：`v0-deprecations.md`
- 入门指南更新：`getting-started.md` / `first-agent.md` / `create-agent.md`

## 升级指南

### 从 v0.8.0 升级

1. 更新 `go.mod`：

```bash
go get agentprimordia@v1.0.0
```

2. 更新 TypeScript SDK：

```bash
npm install @agentprimordia/sdk@1.0.0
```

3. 验证版本：

```bash
ap version
# 输出: v1.0.0
```

### API 迁移

v1.0.0 完全向后兼容，无需修改现有代码。推荐迁移到新 API：

```go
// 旧 API（仍可用）
agent := ap.NewReActAgent(ap.ReActConfig{
    Name: "bot", SystemPrompt: "...", Model: provider, MaxTurns: 10,
})

// 新 API（推荐）
agent, err := ap.NewAgent("bot", "...", provider, ap.WithMaxTurns(10))
```

详见 [v0 → v1 迁移指南](agentprimordia/ecosystem/docs/migration/v0-deprecations.md)。

---

*AgentPrimordia v1.0.0 — The Primordial Agent Framework for Go and TypeScript*

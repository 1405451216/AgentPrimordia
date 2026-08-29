# v3 → v4 迁移指南：废弃 API 移除

> **状态**: Active
> **生效版本**: v4.0.0
> **目标读者**: AgentPrimordia 框架使用者

本指南说明 v4.0.0 移除的废弃 API 及其迁移路径。

## 概述

v4.0.0 按 `docs/版本规范.md` 的废弃承诺清理了两类已超期废弃 API。
所有移除项均有明确替代方案，请按本指南迁移。

## 已移除 API 清单

| 废弃 API | 替代方案 | 移除版本 | 迁移难度 |
|---------|---------|---------|---------|
| `NewReActAgent(ReActConfig{...})` | `NewAgent(name, systemPrompt, provider, opts...)` | v4.0.0 | 低 |
| `RegisterPProf(mux)` | `RegisterPProfSecure(mux)` / `RegisterPProfStrict(mux)` | v4.0.0 | 低 |

## 迁移：NewReActAgent → NewAgent

`NewReActAgent` 自 v0.7.0 起标记废弃，`NewAgent` 为推荐入口。
v4.0.0 正式移除 `NewReActAgent`，代码中所有调用点必须迁移。

### Before

```go
agent := ap.NewReActAgent(ap.ReActConfig{
    Name:         "my-agent",
    SystemPrompt: "你是一个智能助手",
    Model:        provider,
    MaxTurns:     10,
})
```

### After

```go
agent := ap.NewAgent("my-agent", "你是一个智能助手", provider,
    ap.WithMaxTurns(10),
)
```

> 完整迁移说明见 `ecosystem/docs/migration/v0-deprecations.md`
> （ReActConfig 14 个废弃字段 → 链式 API 的映射表）。

### 能力注入（链式 API）

`ReActConfig` 中的能力字段在 v4.0.0 同步清理，改用链式方法：

```go
agent := ap.NewAgent("my-agent", "你是一个智能助手", provider).
    WithMemory(mem).
    WithRAG(ap.RAGConfig{...}).
    WithHooks(hooks).
    WithMetrics(metrics).
    WithTracer(tracer).
    WithCostTracker(costTracker).
    WithFileScope([]string{"/data"}).
    WithCache(cache)
```

## 迁移：RegisterPProf → RegisterPProfSecure / RegisterPProfStrict

`RegisterPProf` 是无鉴权版本，生产环境存在信息泄露风险。
v4.0.0 正式移除，按场景选择替代方案：

### 开发环境（本地调试）

```go
mux := http.NewServeMux()
ap.RegisterPProfSecure(mux)
go http.ListenAndServe("127.0.0.1:6060", mux)
```

### 生产环境（强制鉴权，推荐）

```go
mux := http.NewServeMux()
if err := ap.RegisterPProfStrict(mux); err != nil {
    // PPROF_TOKEN 未设置时返回 ErrPProfTokenRequired
    log.Fatal("pprof 需要设置 PPROF_TOKEN 环境变量: ", err)
}
go http.ListenAndServe("127.0.0.1:6060", mux)
```

> `RegisterPProfSecure` 行为：若 `PPROF_TOKEN` 未设置则退化为无鉴权（开发模式），
> 生产环境请使用 `RegisterPProfStrict`（fail-fast，不允许回退）。

## 验证迁移

```bash
# 1. 确认代码中不再引用已移除 API
grep -rn "NewReActAgent\|RegisterPProf(" --include="*.go" . | grep -v "_test.go" || echo "OK: 无残留"

# 2. 运行构建与测试
go build ./...
go test ./...
```

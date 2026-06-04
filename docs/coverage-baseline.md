# 覆盖率基线

> **生成日期**: 2026-06-05
> **生成命令**: `go test -cover ./internal/... ./pkg/... -count=1`
> **Phase**: 8.1 (覆盖率分阶段提升)

本文件固化 AgentPrimordia 各包的覆盖率基线。Phase 7.2 引入的
CI 阶梯门槛基于本基线设置。Phase 8 起每次覆盖率变动应记录于此。

## 当前基线 (2026-06-05)

### Tier 1: 核心引擎 (门槛 ≥ 80%)

| 包 | 覆盖率 | 状态 |
|------|------:|:----:|
| internal/agent | 80.4% | ✅ 达标 |
| internal/agent/a2a | 84.4% | ✅ 达标 |
| internal/pool | 82.1% | ✅ 达标 |

### Tier 2: 公共 API (门槛 ≥ 65%)

| 包 | 覆盖率 | 状态 |
|------|------:|:----:|
| internal/llm | 68.8% | ✅ 达标 |
| pkg | 67.0% | ✅ 达标 |

### Tier 3: 其他包 (参考,无硬门)

| 包 | 覆盖率 | 备注 |
|------|------:|------|
| internal/admin | 96.0% | 优秀 |
| internal/concurrency | 93.8% | 优秀 |
| internal/security | 95.1% | 优秀 |
| internal/guardrail | 94.4% | 优秀 |
| internal/prompt | 90.6% | 优秀 |
| internal/debugger | 91.3% | Phase 8.1 提升 43.5% → 91.3% |
| internal/metrics | 84.7% | 良好 |
| internal/config | 81.0% | 良好 |
| internal/otel | 80.3% | 良好 |
| internal/persist | 79.5% | 略低 |
| internal/events | 78.5% | 略低 |
| internal/orchestration | 75.8% | 略低 |
| internal/tools/builtin | 76.8% | 略低 |
| internal/memory | 73.5% | Phase 8.1 提升 73.0% → 73.5% (NewMemory 工厂) |
| internal/tools | 70.7% | 略低 |

## 趋势

| 日期 | 备注 | 关键变动 |
|------|------|---------|
| 2026-06-04 | Phase 7.2 引入阶梯门槛 | - |
| 2026-06-05 | Phase 8.1 提升 | debugger +47.8%, memory +0.5% (NewMemory 100%) |

## 提升策略 (Phase 8.1 范围)

1. **debugger** — visualizer.go 100% → 0% 测试覆盖,Phase 8.1 加 `visualizer_test.go`
2. **memory** — `NewMemory` 工厂未测,加 `memory_factory_test.go`
3. **tier 3 包未设硬门** — 后续按需补,但当前不 fail CI

## Tier 3 候选门槛 (Phase 9+)

| 包 | 当前 | 建议门槛 | 差距 |
|------|------:|------:|------:|
| internal/persist | 79.5% | 70% | ✅ 已达 |
| internal/orchestration | 75.8% | 65% | ✅ 已达 |
| internal/tools | 70.7% | 60% | ✅ 已达 |
| internal/memory | 73.5% | 60% | ✅ 已达 |

> 实际所有 Tier 3 包当前都超过建议门槛,Phase 9 可考虑把建议门槛固化。

## 如何重新生成

```bash
cd agentprimordia
go test -cover ./internal/... ./pkg/... -count=1 2>&1 \
  | grep "coverage:" | grep -v "no statements" > /tmp/cov.txt
# 然后手动更新本文件
```

或用脚本:
```bash
make cover  # 见 Makefile (Phase 7.2 引入)
```

## 引用

- SemVer 策略: `docs/specs/2026-06-04-semver-policy.md`
- Phase 7 plan: `docs/plans/2026-06-04-phase7-implementation.md`
- Phase 8 plan: `docs/plans/2026-06-04-phase8-implementation.md`

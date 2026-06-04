# Phase 12: 工具链 / 自动化 — 实施计划

> **日期**: 2026-06-05
> **状态**: Plan Complete (Code 待实施)
> **前置条件**: Phase 7 (SemVer) + Phase 8.4 (pre-commit) + Phase 9/11 即将完成
> **后续**: Phase 13+ (性能基准 / 安全审计)

---

## 总览

Phase 6-11 累积了大量**手工流程**,Phase 12 把它们自动化为 Makefile 目标:

- `make api-diff` — 对比两次 commit 的 pkg/ export 变化
- `make cover-trend` — 覆盖率时间序列跟踪
- `make deprecation-check` — Deprecated 标注完整性
- `make benchmark` — 跑性能基准

| # | 子目标 | 落地形式 | 状态 |
|:-:|--------|----------|:----:|
| 12.1 | `make api-diff` | Shell + awk 脚本对比导出符号 | ⏳ |
| 12.2 | `make cover-trend` | 累积 coverage 数据 + Markdown 趋势表 | ⏳ |
| 12.3 | `make deprecation-check` | grep 校验 // Removed in vX.Y. | ⏳ |
| 12.4 | `make benchmark` | 跑 bench/suite 并出报告 | ⏳ |
| 12.5 | `make lint-all` | golangci-lint + tsc --noEmit 统一 | ⏳ |

---

## 子阶段 12.1: `make api-diff`

### 目的

跟踪 pkg/ public API 变化。SemVer 升级时,清楚列出:
- 新增 (Added)
- 移除 (Removed)
- 签名变更 (Changed)

### 实施

`scripts/api-diff.sh` (新):

```bash
#!/bin/bash
# api-diff: 对比 <ref>..HEAD 的 pkg/ export 变化
# 用法: make api-diff REF=origin/main

set -e
REF=${1:-origin/main}
SINCE=${REF}

echo "📊 API Diff (since $SINCE)"
echo ""

# 收集当前与基线的 pkg/ export 列表
current=$(mktemp)
base=$(mktemp)

trap "rm -f $current $base" EXIT

# 用 go doc 列出 pkg/ 所有 export
(cd agentprimordia && go doc -all ./pkg/...) | grep -E "^(func|type|var|const) " > $current
git show "$SINCE:agentprimordia/pkg/..." 2>/dev/null | (cd agentprimordia && go doc -all ./pkg/...) | grep -E "^(func|type|var|const) " > $base || true

# diff
echo "新增:"
comm -23 <(sort $current) <(sort $base) | sed 's/^/  + /'

echo ""
echo "移除:"
comm -13 <(sort $current) <(sort $base) | sed 's/^/  - /'
```

### 复杂度

`go doc` 输出格式可能不统一,需调整 grep 模式。Phase 12 跑一次校准即可。

### 限制

不检测:
- 签名内部变更(参数重命名等)
- godoc 措辞变更

只检测 export 增/减 — 对 SemVer 的"主要变更"判定足够。

---

## 子阶段 12.2: `make cover-trend`

### 目的

跟踪每个包覆盖率随时间变化,辅助 Phase 9+ 持续提升。

### 实施

`scripts/cover-trend.sh` (新):

```bash
#!/bin/bash
# cover-trend: 跑当前覆盖率,追加到 docs/coverage-history.md
# 用法: make cover-trend (本地) 或 CI 周期任务

set -e
cd agentprimordia
date=$(date -u +"%Y-%m-%d")
output=$(go test -cover ./internal/... ./pkg/... -count=1 2>&1 | grep "coverage:" | grep -v "no statements" | sort)

# 追加 Markdown 表格
{
  echo "## $date"
  echo ""
  echo '| Package | Coverage |'
  echo '|---------|--------:|'
  echo "$output" | sed -E 's|.*agentprimordia/([^ ]+) +.*coverage: ([0-9.]+)%.*|\| `\1` | \2% |'
  echo ""
} >> ../docs/coverage-history.md

echo "已追加 $date 的覆盖率到 docs/coverage-history.md"
```

### CI 集成

加 GitHub Action 周期任务 (每月跑一次):
```yaml
on:
  schedule:
    - cron: '0 0 1 * *'  # 每月 1 号
```

### 价值

- Phase 8.1 后续阶段可看趋势
- 任何包覆盖率下降时,PR 提醒

---

## 子阶段 12.3: `make deprecation-check`

### 目的

CI 已在 Phase 8.4 加了 Deprecated 检查。`make deprecation-check` 是**本地版**,贡献者 commit 前可手动跑。

### 实施

`scripts/deprecation-check.sh` (新):

```bash
#!/bin/bash
# deprecation-check: 验证每个 // Deprecated: 都有 // Removed in vX.Y.
set -e
cd agentprimordia

deprecated=$(grep -r "Deprecated:" --include="*.go" . 2>/dev/null | wc -l)
removed_in=$(grep -r "Removed in v" --include="*.go" . 2>/dev/null | wc -l)

echo "Deprecated: $deprecated"
echo "Removed in: $removed_in"

if [ "$deprecated" -gt 0 ] && [ "$removed_in" -lt "$deprecated" ]; then
    echo "❌ 错误: $deprecated Deprecated 但仅 $removed_in Removed in"
    echo ""
    echo "每个 // Deprecated: 必须配 // Removed in vX.Y."
    exit 1
fi
echo "✅ Deprecated 标注完整"
```

### Makefile 集成

```makefile
deprecation-check:
    bash scripts/deprecation-check.sh
```

### 价值

- 比 pre-commit hook 更详细(可单独跑)
- Phase 9+ 加新字段时容易漏标,自动捕获

---

## 子阶段 12.4: `make benchmark`

### 目的

跑 `bench/suite/` 性能基准,出 Markdown 报告。

### 现状

`bench/suite/` 已有 7+ 个 benchmark 文件。但当前 CI 不跑,无历史对比。

### 实施

```makefile
benchmark:
    cd agentprimordia
    go test -bench=. -benchmem -benchtime=5x -count=3 ./bench/suite/... > bench-output.txt
    go install golang.org/x/perf/cmd/benchstat@latest
    benchstat bench-output.txt > bench-report.md
    @echo "📊 性能报告: bench-report.md"
```

### 跟踪

- 加 `docs/bench-history.md` 累积历史
- CI 每月跑一次,生成 diff

### 价值

- 性能 regression 早期发现
- 给用户的"性能承诺"提供数据支撑

---

## 子阶段 12.5: `make lint-all`

### 目的

统一 lint 入口,Go + TS 一处调用。

### 实施

```makefile
lint-all: lint-go lint-ts
    @echo "✅ 全部 lint 通过"

lint-go:
    golangci-lint run ./...

lint-ts:
    cd sdk/typescript
    npm run lint
```

### 价值

- `make lint-all` 一行跑完所有 lint
- 新人引导成本降低

---

## Makefile 整合

### 完整目标 (Phase 12 后)

```makefile
.PHONY: build test test-short lint lint-all cover cover-html cover-check \
        deprecation-check benchmark api-diff cover-trend lint-ts lint-go \
        clean run-hello run-multi run-production docker-build docker-run docker-clean

build: ...

test: ...

# Phase 7.2
cover: ...
cover-html: ...
cover-check: ...

# Phase 8.4
deprecation-check:
    bash scripts/deprecation-check.sh

# Phase 12.1
api-diff:
    bash scripts/api-diff.sh $(REF)

# Phase 12.2
cover-trend:
    bash scripts/cover-trend.sh

# Phase 12.4
benchmark:
    cd agentprimordia
    go test -bench=. -benchmem -benchtime=5x -count=3 ./bench/suite/... > bench-output.txt
    benchstat bench-output.txt > bench-report.md

# Phase 12.5
lint-all: lint-go lint-ts
lint-go:
    golangci-lint run ./...
lint-ts:
    cd sdk/typescript && npm run lint
```

---

## 验证结果(预)

### Makefile 目标

| 目标 | 状态 |
|------|:----:|
| `make build` | ✅ 已有 |
| `make test` | ✅ 已有 |
| `make cover` | ✅ 已有(Phase 7.2) |
| `make cover-check` | ✅ 已有(Phase 7.2) |
| `make deprecation-check` | ⏳ Phase 12.3 |
| `make api-diff` | ⏳ Phase 12.1 |
| `make cover-trend` | ⏳ Phase 12.2 |
| `make benchmark` | ⏳ Phase 12.4 |
| `make lint-all` | ⏳ Phase 12.5 |
| `make lint-ts` | ⏳ Phase 12.5 |

### 提交规模

- **5 个 commit**,每个子阶段 1 个
- 新增: 4 个 scripts/ 文件 + 1 个 docs/coverage-history.md
- 修改: agentprimordia/Makefile

---

## 风险与债务

### 高优先级

1. **api-diff 误报** — `go doc` 输出格式不稳定
   - 解决: 锁定 Go 版本,定期校准脚本

### 中优先级

2. **benchstat 输出噪声** — 微小性能波动被放大
   - 解决: benchstat 默认有 statistical test,但需训练使用

3. **cover-trend CI 周期任务** — 需 secrets 配置 GitHub Actions 写权限
   - 解决: 用 PR 模式而非 push 模式

### 低优先级

4. **Makefile bash 跨平台** — Windows Git Bash 部分命令行为差异
   - 接受: 文档说明支持平台

---

## 后续工作候选 (Phase 13+)

- Phase 13: 性能基准门槛化 (`make benchmark` 失败 = 性能退化)
- Phase 14: govulncheck 安全扫描
- Phase 15: CHANGELOG 自动生成(从 commit message)
- Phase 16: 文档站部署 (VitePress → GitHub Pages)

---

## 反思:Phase 12 的价值

Phase 6-11 累积的"治理债"主要是**流程债**(手动检查,易遗漏)。Phase 12 把这些债**机械化**,让 CI 强制执行。

回归本质: **好的工程 = 重复的检查全部自动化**。

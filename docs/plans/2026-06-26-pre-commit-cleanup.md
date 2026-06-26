# v0.8.0 生产加固：pre-commit 既有问题修复

> 修复 pre-commit hook 检测到的 3 个既有仓库问题，使提交流程恢复正常。
> 所有问题均不影响运行时行为，但阻碍正常提交流程。

---

## 背景

在提交 v0.8.0 生产加固改动时，pre-commit hook 检测到 3 个既有问题导致提交被阻止：

1. **gofmt 未通过**：约 130 个 .go 文件格式不规范（历史遗留）
2. **Deprecated 标注不完整**：`react_loop.go` 中 1 个 `// Deprecated:` 缺少 `// Removed in vX.Y.` 标注
3. **缺少 docs/plans/*.md**：Phase 8.4 流程约束要求改动须有 plan 文档

## 前置约束

- `go build ./...` 和 `go vet ./...` 零错误
- `gofmt -l agentprimordia/` 输出为空
- pre-commit hook 全部检查通过（不再需要 `--no-verify`）

---

## 实施路线

### Task 1: gofmt 全量格式化

- **文件**: `agentprimordia/` 下约 130 个 .go 文件
- **改动**: `gofmt -w agentprimordia/`
- **影响**: 仅代码格式调整，无逻辑变更，不影响编译和运行时行为
- **验证**: `gofmt -l agentprimordia/` 输出为空

### Task 2: 修复 Deprecated 标注

- **文件**: `agentprimordia/internal/agent/react_loop.go`
- **改动**: 将「将在 v1.0.0 中移除」改为符合 hook 规范的 `// Removed in v1.0.0.`
- **原因**: pre-commit hook 用 `grep "Removed in v"` 检查，原文案不匹配
- **验证**: pre-commit hook 检查 1 通过

### Task 3: 补全 plan 文档

- **文件**: 本文档 `docs/plans/2026-06-26-pre-commit-cleanup.md`
- **改动**: 记录本次修复的 plan
- **验证**: pre-commit hook 检查 3 不再警告

---

## 风险评估

- **生产影响**: 无。3 个问题均为代码规范/流程约束，不影响运行时行为
- **兼容性**: gofmt 仅调整空白/对齐，不改语义；Deprecated 标注修改不影响 API
- **回滚**: 如需回滚，`git revert` 即可

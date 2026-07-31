# 双语言同步发布检查清单

> 每次 Go SDK 或 TS SDK 发版前，必须完成以下检查。

---

## 发版前检查

### 版本一致性

- [ ] Go SDK 版本（`agentprimordia/VERSION`）与 TS SDK 版本（`sdk/typescript/package.json`）主版本号一致
- [ ] 运行 `node scripts/version-sync-check.mjs` 通过
- [ ] 运行 `node scripts/cross-language-api-check.mjs` 通过

### API 对等性

- [ ] Go `pkg/` 新增导出已同步到 TS SDK（或明确标注为 Go-only）
- [ ] `sdk/typescript/api-contract.json` 已更新
- [ ] `sdk/typescript/tests/shared/cross-language-spec.json` 覆盖新模块

### 测试通过

- [ ] Go: `go test ./...` 全部通过
- [ ] Go: `go test -tags=e2e ./internal/chaos/` 通过或优雅 Skip
- [ ] Go: `go test -cover ./internal/security/` ≥ 70%
- [ ] TS: `npx vitest run` 全部通过（排除已知平台问题）
- [ ] TS: `npx tsc --noEmit` 无新增错误

### 文档同步

- [ ] CHANGELOG.md 已更新
- [ ] Release Notes 中标注双语言功能差异（如有）
- [ ] 新增模块有对应的 Cookbook/Guide 文档

### 构建验证

- [ ] Go: `go build ./...` 无错误
- [ ] TS: `npm run build` (tsup) 成功生成 `dist/` 和 `dist/browser/`
- [ ] TS: `npm pack` 包大小在合理范围内

---

## 发版流程

1. 在 `main` 分支创建 release PR
2. 更新版本号：
   - `agentprimordia/VERSION`
   - `sdk/typescript/package.json` → `"version"`
3. 运行本检查清单全部项目
4. 合并 PR 后打 tag：
   - Go: `git tag v3.x.x`
   - TS: `git tag @agentprimordia/sdk@3.x.x`
5. CI 自动发布：
   - Go: Go Module Proxy 自动索引
   - TS: `npm publish`（由 GitHub Actions 触发）

---

## 功能差异记录

| 模块 | Go | TS | 备注 |
|------|:--:|:--:|------|
| ReAct Loop | ✅ | ✅ | 完全对等 |
| Agent Pool | ✅ | ✅ | TS 为轻量实现 |
| Memory (SQLite) | ✅ | ✅ | TS 需 better-sqlite3 |
| Memory (Vector) | ✅ | ✅ | |
| Tools System | ✅ | ✅ | |
| Chaos Engineering | ✅ | ✅ | |
| Cluster | ✅ | ✅ | TS 为轻量实现 |
| Marketplace | ✅ | ✅ | |
| Governance | ✅ | ✅ | |
| WASM Sandbox | ✅ | ✅ | Go 用 wazero，TS 用 WebAssembly API |
| WebGPU | ❌ | ✅ | TS-only（浏览器特性） |
| Edge Runtime | ❌ | ✅ | TS-only（CF/Deno/Bun） |
| React UI | ❌ | ✅ | TS-only |
| Operator (K8s) | ✅ | ❌ | Go-only |
| etcd/Redis Checkpoint | ✅ | ❌ | Go-only（分布式后端） |

---

## 紧急回滚

如发现严重问题：
1. `git revert` 回滚合并提交
2. 重新打 patch 版本 tag
3. TS: `npm publish` 新版本（npm 不支持 unpublish 超过 72h 的版本）
4. 在 GitHub Release 中标注 yanked

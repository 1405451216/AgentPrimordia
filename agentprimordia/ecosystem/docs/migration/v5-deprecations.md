# v5.0 破坏性变更与迁移指南

> v5.0-4 主版本破坏性变更：按 VERSIONING 周期执行。仿 v4-deprecations.md。
> 结论：**v5.0 无超期废弃 API 残留**（deprecation 门 0 残留，见 `pkg/deprecation_residual_test.go`），
> 本指南记录 v4.1–v4.9 期间的行为/契约演进与主版本承诺。

## 一、破坏性变更清单（v5.0）

| 变更 | 影响 | 迁移 |
|------|------|------|
| `autonomy.ResumeManager.SaveCheckpoint` 签名增加 `description` 参数 | 直接调用者（内部 API） | 传目标描述字符串；普通调用传 `""` 即可（恢复目标描述为空时用默认） |
| `realtime.Runtime.PushVision` 签名增加 `ctx` 参数 | realtime 调用方 | 传入 `context.Context` |
| `realtime.EventBus.Publish` 历史语义 | 无 API 变化（行为增强：历史保留） | 无 |
| `orchestration` workflow 垫片删除（16 类型别名 + 22 const + 1 var） | 内部 API（从未在 pkg 导出） | 改用 `internal/agent` 的 Workflow 类型 |
| 主版本号 4 → 5 | 按 SemVer 主版本边界 | 见下文依赖升级 |

## 二、依赖升级（v5.0 需要做的）

- Go SDK：`go get github.com/AgentPrimordia/agentprimordia/pkg@v5.0.0`
- TS SDK：`npm install @agentprimordia/sdk@5.0.0`

## 三、v4.x 期间新增/演进的关键 API（非破坏性）

| API | 版本 | 说明 |
|-----|------|------|
| `pkg.ProviderFromEnv()` | v4.1 | env 驱动真实 LLM（AP_LLM_PROVIDER/MODEL/API_KEY） |
| `pkg.NewOpenAIASR/NewOpenAITTS` | v4.1 | 真实语音适配器（本地 faster-whisper/Piper 免 key） |
| `realtime.Runtime.ProcessTurnStream` | v4.3 | 流式语音链路（音频→流式 LLM→语音） |
| `skills.SignSkillManifest/InstallSkillFromManifest` | v4.4 | 技能市场发布/订阅（ECDSA 验签） |
| `marketplace.Installer.EnableDownloadStats` | v4.8 | 市场下载统计 |
| `autonomy.GoalConfig.BudgetUSD` | v4.9 | 目标级成本预算护栏 |
| `memory.NewPhysicalTenantStore` | v4.6 | 物理分库强隔离 |

## 四、废弃承诺（v5.0 之后生效，v6.0 清理）

- `pkg.NewRealtimeSession`（Experimental）：v5.0 后建议改用 `NewRealtimeRuntime` + `OpenSession`
- 预留：`SkeletonBackend`（TS WebGPU）仅作回退，v6.0 视 Transformers 生态成熟度评估

## 五、验证

```bash
# deprecation 残留门（Windows 可跑）
go test ./pkg/ -run Deprecation

# 版本四方一致
node scripts/version-sync-check.mjs
```

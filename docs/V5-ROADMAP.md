# AgentPrimordia v5.0 路线图实施计划（v4.1 – v5.0）

> **文档定位**：v4.0 收官（四能力跃迁 + 实证路线 35 项 + 评估整改）后，v4.1–v5.0 的均衡混排弧线——奇数版本能力深化、偶数版本工程稳定，逐版本推进到 v5.0 平台里程碑。
>
> **创建日期**：2026 年 8 月 9 日　**版本基线**：Go SDK v4.0.0 / TypeScript SDK v4.0.0
>
> **v4.1 为本弧线首个可实施版本（全量任务表）；v4.2–v5.0 为里程碑粒度，临近时按 V4-ROADMAP 四层结构深化。**

---

## 一、弧线总原则（均衡混排）

| # | 原则 | 说明 |
|---|------|------|
| 1 | 均衡混排 | 奇数版本=能力深化（真实接线/多模态/分布式/双语言/性能），偶数版本=工程稳定（规模化/平台/安全/生态/收官）；弧线整体不偏科 |
| 2 | 端到端验收 | 每版本一个可运行验收场景（列表见进度跟踪表） |
| 3 | 双语言同步 | 新增 Go 能力在 TS SDK 对等实现并纳入 cross-language-spec 套件 |
| 4 | TDD 强制 | 每任务先写测试，验证门 `go test ./internal/<pkg>/...`，失败即停（AGENTS.md §6.1） |
| 5 | 向后兼容 | 全部新增式演进；冲突路径标 `Deprecated:` 引导迁移，不删除（v5.0 主版本除外，按 VERSIONING 周期清理） |
| 6 | 实证记录 | 每版本完成更新 ROADMAP.md 状态并附代码证据（文件:行号），复测深度评估分项 |
| 7 | 依赖白名单 | 仅标准库 + 白名单依赖（AGENTS.md §2.1）；真实服务一律「接口 + 可插拔适配器」 |

---

## 二、v4.1 真实接线（Real-World Wiring）

> **性质**：能力深化
> **本质跃迁**：从「demo 可运行」到「真实服务可运行」——消灭 v4.0 四跃迁的 mock/占位，清完 2026-08-09 评估遗留债。

### Phase 1 核心实现（P0）

| # | 任务 | 文件 | 说明 |
|---|------|------|------|
| 1 | 真实 ASR 适配器 | `internal/agent/realtime/asr.go`（新建） | 实现 ASRAdapter（接口在 `realtime/hub.go:9-14`）：OpenAI Whisper 兼容 HTTP 端点（multipart 音频 → 文本）；保留 MockASR |
| 2 | 真实 TTS 适配器 | `internal/agent/realtime/tts.go`（新建） | 实现 TTSAdapter（接口在 `realtime/hub.go:16-21`）：HTTP TTS 端点（文本 → 音频）；保留 MockTTS |
| 3 | 适配器接线 | `internal/agent/realtime/runtime.go` + `cmd/ap/realtime.go` | `runtime.go:52-57` 默认仍 mock（nil→Mock）；`ap realtime voice` 新增 `--asr=openai --tts=openai --asr-url= --tts-url= --tts-voice=`；缺 URL/Key 时报清晰错误并提示 mock 用法 |
| 4 | Studio Go 真实接线 | `internal/studio/studio_v3x_handlers.go` | 五个 stub 端点 autonomyGoals(:13)/autonomyAlerts(:17)/skillsList(:27)/realtimeSessions(:53)/realtimeEvents(:57) 改从注入的 AutonomyRuntime / SkillStore / RealtimeHub 读真实数据；沿用 STUDIO.md 的 `WithChaos/WithCluster/WithLearning/WithMarketplace` 注入模式；未注入时返回空数组且响应头标 `X-Data-Source: demo` |
| 5 | FailureStore 持久化 | `internal/persist/failure_sqlite.go`（新建） | SQLiteFailureStore（modernc sqlite，白名单内）实现 FailureStore 接口（现仅 MemoryFailureStore，`persist/failure.go:89`）；默认仍内存，`WithSQLiteFailureStore(path)` 启用 |
| 6 | runLoop 拆分 | `internal/agent/react_loop_core.go` | runLoop（:31-412，~380 行）按块抽命名函数：guardrail 输出校验、记忆注入、RAG 检索、成本检查、审计写入、知识蒸馏（writeAudit/recordAuditObservability/distillKnowledge 已抽，分别 :418/:435/:454 起，继续同法）；纯行为保持重构，现有测试为门 |
| 7 | 示例迁移 pkg | `ecosystem/examples/{a2a-interop,autonomous-task,coding-agent,realtime-voice,skill-evolution}/main.go` | 5 个文件（分别 :13/:15/:32/:12/:13 import `internal/*`）改走 `pkg/` 公共 API；缺导出则先在 pkg 补导出（如 autonomy 运行时装配、realtime 会话 API），不留 internal 直连 |
| 8 | workflow 垫片拆除 | `internal/orchestration/workflow_reexport.go` | 先把 orchestration 测试改用 `internal/agent` 的 Workflow 类型，再删 16 类型别名 + 22 const + 1 var（`NewWorkflowExecution`）垫片（干净割接，不留兼容别名） |
| 9 | 租户加固 | `internal/memory/tenant.go` | `TenantScoped.Add/AddBatch` 已统一覆盖+校验（`tenant.go:126-141/:160-167`：缺省注入 + 不一致拒绝 `ErrTenantMismatch`，2026-08-09 评估 §5.2 复核已落地）；剩余缺口：denied 拒绝仅记 stats 计数（如 :138/:163）未入审计事件、`Inner()`（:108）警示可再强化、CAPABILITY-INVENTORY.md 租户条目注明「过滤器级隔离」口径 |
| 10 | V4 能力真实 LLM 端到端 | `internal/eval/llm_bench.go` + `ecosystem/examples/{autonomous-task,skill-evolution,realtime-voice}` | 三个 demo（现 autonomous-task 无 LLM、skill-evolution 用 mockDistiller、realtime-voice 纯 mock 流）支持 env 驱动真实 provider（`AP_LLM_PROVIDER/AP_LLM_MODEL/AP_LLM_API_KEY`），默认仍 mock 保证 CI 可跑；llm_bench 新增两项跑分：autonomy 目标执行成功率、skills 习得成功率（失败则记 0 分不门禁，但须产出首份基线报告，随 v4.1 完成写入 ROADMAP.md 状态行） |

### Phase 2 跨组件集成（P1）

| # | 任务 | 说明 |
|---|------|------|
| 1 | 真实接线 × 可观测 | skills 习得、realtime 会话补目标级 span/metrics（复用 `metrics/` + `otel/`） |
| 2 | 真实接线 × 守卫 | ASR 转写文本、TTS 输出文本、习得 Skill 描述过 guardrail（复用 `guardrail/` 引擎） |
| 3 | 真实接线 × 记忆 | realtime 会话摘要入 memory；skills 习得轨迹存 memory（v3.4 联动补真实链路） |
| 4 | FailureStore × debugger | `/api/failures`（`debugger/failure_server.go`）支持 SQLite 后端读取 |
| 5 | Studio × 部署 | demo→真实切换后 30s staleness / 轮询语义复核（`docs/STUDIO.md` 原则不回归） |

### Phase 3 开发者体验（P2）

| # | 任务 | 文件 | 说明 |
|---|------|------|------|
| 1 | CLI 参数 | `cmd/ap/realtime.go` | Phase1-3 的 `--asr/--tts/--asr-url/--tts-url/--tts-voice` |
| 2 | 文档 | `agentprimordia/docs/guides/realtime.md`、`docs/STUDIO.md` | 真实适配器配置 + provider 模板（faster-whisper/Piper/OpenAI 三例）；一键 demo 用法；Studio 真实接线说明 |
| 3 | pkg 导出 | `pkg/realtime.go` | 真实适配器构造器（如 `realtime.NewOpenAIASR(url, key)`）；FailureStore SQLite 构造器 |
| 4 | 一键真实语音 demo | `scripts/dev-realtime.ps1` + `scripts/dev-realtime.sh`（新建） | 仿 `scripts/dev-studio.ps1/.sh` 模式：一条命令拉起 `ap realtime voice`（本地 faster-whisper/Piper 或 OpenAI API key 双路径），等待就绪、打印会话指引、Ctrl+C 一起停止；缺服务/缺 key 时报清晰错误并提示 mock 用法 |

### Phase 4 测试与性能验证（P2）

| # | 任务 | 文件 | 说明 |
|---|------|------|------|
| 1 | 单元测试 | `internal/agent/realtime/{asr,tts}_test.go`、`internal/persist/failure_sqlite_test.go`、`internal/studio/studio_v3x_handlers_test.go` | httptest 驱动（不需真实网络）；SQLite CRUD + 并发 + Close 后行为 |
| 2 | runLoop 拆分回归 | `internal/agent/react_loop_*_test.go` | 拆分后 `go test ./internal/agent/...` 全绿，既有行为测试零改动 |
| 3 | 集成测试 | `internal/agent/realtime_integration_test.go` 等 | 真实适配器 × 守卫/记忆/可观测联动；examples 迁移后编译通过 |
| 4 | 跨语言 | `sdk/typescript/src/realtime/` | ASR/TTS 客户端形状与 Go 接口对齐（edge.ts 已有基础） |
| 5 | 验收 demo | 见验证门 | 真实语音会话——本地免 key 路径（faster-whisper/whisper.cpp 兼容端点 + Piper/edge-tts）为必达项，有 key 时另跑 OpenAI 真实端点验证兼容性；autonomous-task 真实 LLM 完成目标并报告 |

### v4.1 验证门

```bash
go test -count=1 ./internal/agent/realtime/... ./internal/persist/... ./internal/studio/...
go test -count=1 ./internal/agent/...
go test -count=1 ./ecosystem/examples/...
go build ./...
```

---

## 三、v4.2 稳定与规模化（Scale & Reliability）

> **性质**：工程稳定
> **本质跃迁**：真实接线后的系统在长时间、高并发、故障注入下不塌。

| # | 任务 | 验收标准 |
|---|------|---------|
| 1 | 24h+ Soak（真实 LLM 混合 mock 流量，chaos 期间注入） | 无泄漏、无 panic、恢复率 ≥99% |
| 2 | Pool × autonomy 100+ 并发目标 | 成功率/恢复率与 1 目标持平 |
| 3 | 全链路 P95/P99 基准刷新（bench/suite + 2026-Q4.json 扩展） | perf-regression 门全绿 |
| 4 | 混沌常态化：chaos-e2e 跑真实基准集 + 集群故障注入 | 故障下成功率下降 ≤ 阈值（量化） |
| 5 | 双线真实 LLM 基准可比化（Go/TS 同任务成功率/成本/延迟对照表入 CI） | 对照表随版本发布 |

验收：Soak 报告 + 混沌量化报告；并发 100 目标全完成。

---

## 四、v4.3 多模态真实化（Multimodal Real）

> **性质**：能力深化
> **本质跃迁**：从「文字对话」到「语音/视觉实时交互」真实链路。

| # | 任务 | 验收标准 |
|---|------|---------|
| 1 | 端到端语音会话：v4.1 适配器 + 流式 LLM（音频→文本→流式响应→语音） | 真实语音多轮含打断 |
| 2 | 实时视觉流：连续帧输入走 multimodal/ + 真实视觉 provider | 视频帧理解 demo |
| 3 | WebGPU 边缘推理可用化（TS 浏览器本地推理替换 mock） | 浏览器端到端 demo 可跑 |
| 4 | guardrail 扩展到语音/视觉内容 | 语音/图像注入样例被拦 |

验收：`ap realtime voice` 真实语音含打断；浏览器 WebGPU 本地推理 demo。

---

## 五、v4.4 开发者平台（Developer Platform）

> **性质**：工程稳定
> **本质跃迁**：从「框架可嵌入」到「第三方可低成本接入」。

| # | 任务 | 验收标准 |
|---|------|---------|
| 1 | 技能市场能力级共享（Skill 发布/订阅，manifest + 验签，升级自 v3.4） | 技能远程安装 + 验签 |
| 2 | 插件 SDK 成熟（`ap plugin create` 脚手架：模板/测试/发布指引） | 生成项目可构建可发布 |
| 3 | 模板市场（templates 扩充 + 在线检索/安装） | 远程模板安装成功 |
| 4 | 文档站 vitepress 全量 + 死链门；VS Code Inspector 完善（DAG 可视化、断点） | docs-build 全绿 |
| 5 | 第三方接入指南（A2A 双向、双 SDK 快速上手） | 按文档 30 分钟接入真实场景 |

---

## 六、v4.5 分布式自治（Distributed Autonomy）

> **性质**：能力深化
> **本质跃迁**：从「单机自治」到「集群自治」。

| # | 任务 | 验收标准 |
|---|------|---------|
| 1 | 目标跨节点迁移/续跑（autonomy × cluster 状态同步 + checkpoint 恢复） | kill 节点后目标自动续跑 |
| 2 | etcd/redis 后端常态化（persist/cluster build-tag 后端转正式配置路径，默认关闭） | 配置文档化，开关可测 |
| 3 | 故障域隔离（节点故障目标自动迁移，无人工干预） | 3 节点 kill 1 无人工续跑 |
| 4 | 跨节点 A2A 任务路由（interop 客户端选路 + 熔断/限流） | 第三方任务委托不因单点故障中断 |

---

## 七、v4.6 安全与治理（Security & Governance）

> **性质**：工程稳定
> **本质跃迁**：从「纵深防御」到「强隔离 + 可审计」。

| # | 任务 | 验收标准 |
|---|------|---------|
| 1 | 强租户隔离：过滤器级 → 物理分库/加密（memory 多租户重构） | 双租户物理隔离测试 |
| 2 | 审计统一导出：audit.Event 全链路 + 导出/检索 API | 一次完整目标执行可全链路导出 |
| 3 | guardrail 强化：PII 脱敏管线、敏感工具调用审计 | 注入样例 PII 不落日志 |
| 4 | SecretsManager 深化：Vault 后端常态化 + 轮换 | 轮换测试通过 |
| 5 | 供应链入版本门（SBOM/签名/漏洞扫描）+ 安全评估报告 | 评估安全分项 ≥9 |

---

## 八、v4.7 双语言深度对齐（Dual-Language Parity）

> **性质**：能力深化
> **本质跃迁**：从「TS 对齐 Go 行为」到「双 SDK 同等能力」。

| # | 任务 | 验收标准 |
|---|------|---------|
| 1 | TS 四能力生产化：realtime edge 链路、skills 全套、autonomy TS 运行时 | TS 侧验收 demo 可跑 |
| 2 | Rust/Python SDK 升级：功能对齐 + httptest 测试（消除零测试短板） | 双 SDK 测试接入 CI |
| 3 | 跨语言 spec 100% 覆盖（全部套件双线实跑，无静默跳过） | cross-language-api-check 全绿 |
| 4 | 双线基准可比（Go/TS 同基准集分数入 CI 门） | 双线 llm-bench 报告并排发布 |

---

## 九、v4.8 生态运营（Ecosystem）

> **性质**：工程稳定
> **本质跃迁**：从「生态能力存在」到「生态有人用」。

| # | 任务 | 验收标准 |
|---|------|---------|
| 1 | 市场真实运营：插件上架/下载/验签全链 + 统计 | 真实第三方插件上架并安装成功 |
| 2 | 社区模板与技能扩充 | 模板库 +N，技能市场有真实条目 |
| 3 | benchmark 排行榜公开（成功率/成本/延迟） | 榜单页随发布更新 |
| 4 | AP 自举规模化（日常开发用 AP 跑基准集，季度报告） | 成功率曲线季度上升 |
| 5 | 企业参考部署（Helm/Terraform 一键 + 案例文档） | 一键部署案例 |

---

## 十、v4.9 性能与成本（Performance & Cost）

> **性质**：能力深化
> **本质跃迁**：从「功能正确」到「又快又省」。

| # | 任务 | 验收标准 |
|---|------|---------|
| 1 | 全链路延迟：首 token、流式、工具往返 P50/P95 优化 | 关键路径 P95 较 2026-Q4 基线再降 ≥20% |
| 2 | token 成本优化：压缩策略进化、语义缓存命中率、批量 | 同任务集成本下降可量化 |
| 3 | 推理加速：批量/路由策略、边缘 profile（WebGPU/本地） | 边缘推理延迟报告 |
| 4 | 成本护栏精细化：目标级预算、租户级配额、告警 | 超预算目标自动暂停 |

---

## 十一、v5.0 平台里程碑（Platform Major）

> **性质**：工程稳定
> **本质跃迁**：从「框架」到「平台」——一体化交付、承诺升级、主版本发布。

| # | 任务 | 验收标准 |
|---|------|---------|
| 1 | 一体化一键部署：Helm/Terraform 一键拉起全能力（autonomy/skills/a2a/realtime/studio） | 一键部署上生产形态 |
| 2 | 全能力整合验证：四跃迁 × 双 SDK × 市场 × 集群端到端验收套件 | 全绿 |
| 3 | 稳定承诺升级：Stable API 清单重审，Experimental 转正/降级（stability_compliance 门） | 清单与实际一致 |
| 4 | 主版本破坏性变更：按 VERSIONING 移除超期 API + 迁移指南（仿 v4-deprecations.md） | deprecation 检查 0 残留 |
| 5 | 发布：tag 自动化 + 全平台产物 + 评估复测报告 | 版本四方一致；2026-08-09 深度评估复测加权 ≥8.5/10（基线 7.2/10） |

---

## 十二、进度跟踪

| 版本 | 性质 | 主题 | 验收场景 | 状态 |
|------|------|------|---------|------|
| v4.1 | 深化 | 真实接线 | 真实语音会话（本地免 key 一键 demo）+ 真实 LLM 自治目标基线报告 | ⬜ 规划中 |
| v4.2 | 稳定 | 稳定与规模化 | Soak 报告 + 并发 100 目标 | ⬜ 规划中 |
| v4.3 | 深化 | 多模态真实化 | 真实语音多轮含打断 + WebGPU demo | ✅ 已完成（2026-08-09：流式语音链路 `realtime/runtime.go:ProcessTurnStream`、视觉护栏/帧分析 `runtime.go:PushVision/AnalyzeLatestFrame`、TS WebGPU 真实后端优先 `webgpu-model-runner.ts:detectInferenceBackend`） |
| v4.4 | 稳定 | 开发者平台 | 第三方 30 分钟接入 | ✅ 已完成（2026-08-09：技能市场 `skills/skill_market.go` manifest+ECDSA 验签；模板远程安装 `cmd/ap/marketplace.go:installRemoteTemplate`；死链门 `scripts/check-docs-links.mjs` 274 链接全可达；Inspector DAG `extensions/vscode/src/inspector.ts:applyPlan`；接入指南 `docs/guides/third-party-integration.md`） |
| v4.5 | 深化 | 分布式自治 | 3 节点 kill 1 自动续跑 | ✅ 已完成（2026-08-09：跨节点续跑 `autonomy/runtime.go:ResumeIncomplete` 重建目标+`autonomy_failover_e2e_test.go` 3 节点 kill 1 自动续跑；路由 `a2a/interop_router.go` 选路+熔断+故障切换；后端配置 `docs/guides/distributed-backends.md`） |
| v4.6 | 稳定 | 安全与治理 | 双租户物理隔离 + 审计导出 | ✅ 已完成（2026-08-09：物理分库 `memory/physical_tenant.go` 双租户隔离测试；审计导出 `audit/logger.go:ExportJSON`；敏感工具审计 `guardrail/sensitive_tool_rule.go`；Vault 轮换已有测试；供应链门 govulncheck/socket/trivy 在 CI；安全态势报告 `docs/security-posture.md`） |
| v4.7 | 深化 | 双语言深度对齐 | cross-language 全绿 + 双线报告并排 | ⬜ 规划中 |
| v4.8 | 稳定 | 生态运营 | 真实第三方插件上架 | ⬜ 规划中 |
| v4.9 | 深化 | 性能与成本 | P95 再降 ≥20% + 成本下降量化 | ⬜ 规划中 |
| v5.0 | 稳定 | 平台里程碑 | 一键部署 + 迁移指南就绪 + 评估复测 ≥8.5 | ⬜ 规划中 |

> **实施顺序**：v4.1 → v4.2 → … → v5.0 逐版本推进；每版本完成全部任务 + 验收场景通过后 bump 版本并发布（AGENTS.md §6.1：验证门失败即停，报告进度）。

> 文末附注：*本计划将随实施进度持续更新，每版本完成标记状态并更新 ROADMAP.md（附代码证据）。*

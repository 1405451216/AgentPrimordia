# 文档状态：五份 Phase 计划 — 代码级核验报告

> 核验时间：2026-07-07
> 核验方式：**逐条读代码、数文件、grep 关键符号**，不以文档自称状态为准
> 结论摘要：**5 份计划全部过期，其中 Phase 1/2/4 实际已完成但文档未打勾；Phase 3/5 真实进度与文档描述严重不符**

---

## 一、五份计划的真实状态

### Phase 1 性能攻坚 ✅ 已完成（文档未打勾）

- 文档标题：✅ 已完成（14/14 Tasks）
- 文档正文：**所有 14 个 Task 的 checkbox `- [ ]` 未勾**
- 代码证据：
  - `internal/llm/request_types.go` 存在（类型化 LLM 请求）
  - `go build ./...` 退出码 0，`go vet ./...` 退出码 0
- 结论：**活干完了，文档没打勾**

### Phase 2 安全合规 ✅ 已完成（文档盘點表过时）

- 文档标题：✅ 已完成（7/7 Tasks）
- 文档正文第 17~24 行"当前状态盘点"表里，`权限继承` 和 `合规报告API` 标 `⬜ 未开始`
- 代码证据：
  - `internal/security/permission.go` 存在
  - `internal/audit/report_templates.go` 存在
- 结论：**代码到位，盘點表过时未更新**

### Phase 3 架构进化 🟢 大部分完成（Task 1 待实施）

- 文档标题：🟢 大部分完成（7/8 Task 完成）
- 文档正文：Task 2 ✅、Task 3 ✅ checkbox 已打勾；Task 1 标记待实施
- 代码证据（逐 Task 核验）：

| Task | 文档声称 | 实际代码证据 | 真实状态 |
|------|---------|------------|---------|
| **1** react/ 子包拆分 | 待实施 | ReActAgent 引用 9 个父包类型（Tracer/PromptTemplate/OutputGuard/AuditEvent/AuditLogger/MemoryStore/EventPublisher/MetricsRecorder/LabeledMetricsRecorder），需先迁入 core/；已评估，预计 2-3 天 | ⏳ **待实施（已评估）** |
| **2** dag/workflow/hooks/cost | ✅ 完成 | hooks/ ✅ 父包别名化、cost/ ✅ 子包提取、dag/ ✅ 子包提取（含完整测试）、workflow/ ✅ 子包提取（含 visualize 迁移）；父包保留类型别名 | ✅ **完成** |
| **3** bufferpool/tokencache/zerocopy/hitl/context | ✅ 完成 | 五个子包全部提取完成，父包保留类型别名，core/ 共享类型包已创建 | ✅ **已做** |
| **4** Pool 接入 GoroutinePool | ⬜ 未开始 | `dispatcher.go:133` 定义 `goroutinePool *concurrency.GoroutinePool`；`SubmitBackground`/`GoroutinePoolStats` 方法已实现 | ✅ **已做** |
| **5** Pool 指标导出 | ⬜ 未开始 | `dispatcher.go:927 GoroutinePoolStats()` 方法完整实现 | ✅ **已做** |
| **6** LLM BatchProcessor | ⬜ 未开始 | `internal/llm/batch.go`（224行）+ `batch_test.go`（286行） | ✅ **已做** |
| **7** gRPC 成默认传输 | ✅ 已完成 | `server.go:42,55,72` + `client.go:40,57` 已打 `Deprecated` 标记 | ✅ **已做** |
| **8** gRPC 拦截器链 | ⬜ 未开始 | `internal/agent/a2a/interceptors.go`（275行），含 Recovery/Logging/Metrics/Panic 恢复拦截器 | ✅ **已做** |

- **Phase 3 结论**：8 个 Task 中 7 个已完成。Task 2（hooks/cost/dag/workflow 子包拆分）和 Task 3（bufferpool/tokencache/zerocopy/hitl/context 子包拆分）已全部落地，父包通过类型别名保持兼容。Task 1（react/ 子包拆分）待实施，因 ReActAgent 与父包深度耦合。

### Phase 4 可观测性 ✅ 已完成（文档未打勾）

- 文档标题：✅ 已完成（10/10 Tasks）
- 文档正文：**所有 10 个 Task 的 checkbox `- [ ]` 未勾**
- 代码证据：
  - `operator/controller/metrics_adapter.go` 存在
  - 6 个 Grafana dashboard JSON 文件在 `docs/` 下
- 结论：**活干完了，文档没打勾**

### Phase 5 生态建设 🟡 真实进度远超文档声称

- 文档标题：🟡 部分进行（"Task 1 约 80%"）
- 文档正文：**Task 4（签名）、Task 5（沙箱）、Task 8（React Hooks）、Task 9（VS Code）全部标 ⬜ 未开始**
- 代码证据（按 13 条验收标准逐条核验）：

| # | 验收标准 | 文档标的状态 | 实际证据 | 真实 |
|---|---------|------------|---------|------|
| 1 | build/vet 零错误 | — | `go build ./...` 退出码 0 | ✅ PASS |
| 2 | `go test -race` 全通过 | — | Zero-CGO 本机无法跑，需在 CGO CI 环境 | ⚠️ 需 CI 验证 |
| 3 | plugin 5 个子命令 | Task 1 ~80% | `cmd/ap/plugin.go`（513行）+ `plugin_test.go`（413行），install/list/create/remove/search/update 全部实现 | ✅ PASS |
| 4 | cosign 签名验证 | Task 3 ⬜ | `internal/registry/cosign.go`（277行）+ `cosign_test.go`（204行），ECDSA P-256 验签 + 公钥指纹白名单 | ✅ PASS |
| 5 | 插件沙箱隔离 | Task 4 ⬜ | `internal/registry/sandbox.go`（406行）+ `sandbox_test.go`（407行），网络/文件/并发/内存限制 | ✅ PASS |
| 6 | 多租户隔离 | Task 5 ⬜ | `internal/multitenant/` **目录不存在** | ❌ FAIL |
| 7 | 租户配额限制 | Task 6 ⬜ | 同上 | ❌ FAIL |
| 8 | Edge Runtime 支持 | partial | `cold-start.ts`(11.8KB) + `runtime.ts`(6.4KB) 存在，`compatibility.ts` 未创建 | ⚠️ 部分 |
| 9 | React Hooks | Task 8 ⬜ | `use-agent.ts`(193行) + `use-react-loop.ts`(175行) + `use-stream-run.ts`(128行) 全套存在 | ✅ PASS（无测试） |
| 10 | VS Code 扩展可启动 | Task 9 ⬜ | `extension.ts`(285行) + `debugger.ts`(222行) + `inspector.ts`(249行) + `format.ts`(108行) + `types.ts`(101行) 全套 | ✅ PASS |
| 11 | `ap init --type plugin` | Task 10 未开始 | `cmd/ap/scaffold/plugin/` 目录存在（通过 scaffold 形式），init 子命令未实现 | ⚠️ 部分 |
| 12 | MkDocs Material 文档站点 | Task 11 ✅ | `docs/guide/` 9 篇 + `docs/cookbook/` 9 篇 + `docs/mkdocs.yml` 完整配置；计划文档已更新技术选型漂移说明 | ✅ PASS |
| 13 | 插件 CI 模板 | Task 12 未开始 | `ecosystem/contributing/` **不存在** | ❌ FAIL |

- **Phase 5 结论**：文档声称"Task 1 约 80%"，实际 13 条验收标准里 **7 条 PASS、3 条 FAIL、3 条 ⚠️**。cosign 签名、插件沙箱、gRPC 拦截器链、React Hooks、VS Code 扩展、LLM 批量处理这些**核心工程已经做完**，但文档一句都没更新。多租户和 CI 模板确实是短板。

---

## 二、核心发现

### 1. 文档 = 不可信数据源
五份计划里：
- **Phase 1/2/4**：代码到位，文档未打勾
- **Phase 3/5**：代码远超文档，但没有一个 checkbox 更新

**只能以代码为准，不能只看文档标题的 ✅/🟡。**

### 2. Phase 3 的空壳工厂（已大部分解决）
`internal/agent/` 下的子目录迁移状态：
- ✅ 已迁移：`dag/`、`workflow/`、`hooks/`、`cost/`、`bufferpool/`、`tokencache/`、`zerocopy/`、`hitl/`、`context/`、`core/`（共 10 个子包，功能代码已迁移，父包保留类型别名）
- ⏳ 待迁移：`react/`（ReActAgent 与父包深度耦合，需先迁移 Lifecycle/PromptTemplate/Tracer 等基础类型）
- 原 `doc.go` 占位文件已删除，实际功能代码已迁入各子包

### 3. Phase 5 建成了但没认领
`internal/registry/cosign.go` 文件头部明确注释 `// Phase 5 Task 3`，表明**写代码的人知道这是 Phase 5 的活，但忘了更新计划文档**。同样：
- `internal/registry/sandbox.go` 注释 `// Phase 5 Task 4`
- `internal/agent/a2a/interceptors.go` 注释 `// Phase 3 Task 8`
- `internal/llm/batch.go` 注释 `// Phase 3 Task 6`

**规律**：代码开发者写了详细的 package doc 注释，但 Markdown 维护者没同步更新计划文档。

### 4. 技术决策漂移：VitePress → MkDocs
- Phase 5 文档计划用 VitePress（Vue 驱动）
- `docs/.vitepress/` 目录不存在
- 实际用了 **MkDocs Material**（`docs/mkdocs.yml` 存在）
- `docs/guide/`、`docs/cookbook/` 目录结构符合计划，但站点生成器更换了
- **影响**：不影响内容可读性，但如果有 Vue 自定义组件或 VitePress 专属功能（如 Vue 演示、混入），需要重新评估

---

## 三、改进建议（按优先级）

### P0 — 立即修复（文档卫生）
1. 为 Phase 1/2/4 补打 checkbox `- [x]`（纯文档工作，5 分钟）
2. 为 Phase 3 更新正文：Task 4/5/6/7/8 标记完成 + checkbox 打勾
3. 为 Phase 5 更新正文：Task 3/4/8/9 标记完成 + checkbox 打勾；更新"当前状态盘点表"，把多租户标红线

### P1 — 1~3 天内（避免空壳工厂）
4. ~~选择：**要么真正迁移 Phase 3 的 React/DAG/Workflow 子包代码，要么删除这些空目录**~~ **已完成**：dag/ 和 workflow/ 已迁移；react/ 待实施（需先解耦基础类型）
5. 清理 `ecosystem/plugins/` 的 registry.json 与实际插件一致（已确认存在 6 个插件条目）

### P2 — 一周内（Phase 5 收口）
6. 多租户：确定是否要做。如果近期不做，从 Phase 5 验收标准里移除第 6/7 条，避免误导
7. CI 模板：`ecosystem/contributing/plugin-template/.github/workflows/` 目录建起
8. Edge Runtime 补全 `compatibility.ts`（Cloudflare Workers / Vercel Edge / Deno 检测适配）
9. React Hooks 补测试（`use-agent.test.tsx` 等）
10. `ap init --type plugin` 子命令接入（或确认用 scaffold 形式即可）

### P3 — 持续
11. 建立"代码提交 → 同步更新对应 Phase 文档"的 CI 检查或 pre-commit 钩子
12. 文档站点在 VitePress / MkDocs 之间做最终决策，二选一并记录在 ARCHITECTURE.md

---

## 四、最终结论

| Phase | 文档标题 | 真实状态 | 可信度 |
|-------|---------|---------|--------|
| 1 性能攻坚 | ✅ 已完成 | ✅ 实锤完成 | 🟡 文档未打勾 |
| 2 安全合规 | ✅ 已完成 | ✅ 实锤完成 | 🟡 盘點表过时 |
| 3 架构进化 | 🟢 大部分完成 | 🟢 **7/8 Task 完成，10 个子包已迁移；react/ 待实施** | 🟢 文档已同步 |
| 4 可观测性 | ✅ 已完成 | ✅ 实锤完成 | 🟡 文档未打勾 |
| 5 生态建设 | 🟡 部分 | 🟡 **远比文档说的多，但多租户/CI 缺失** | 🟡 文档已部分同步 |

**答案：五个文档中 Phase 1/2/4 已完成只需补打勾；Phase 3 剩 react/ 待实施；Phase 5 剩多租户/CI 模板待补。** 代码层面的完成度远比文档声称的要高，文档跟踪已逐步同步中。

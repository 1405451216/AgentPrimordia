# 提案：code 层「沙箱受控释放」政策（宿主永久拒绝 + 沙箱受控释放）

> **文档定位**：V7 弧线 S0-4 交付物二（`docs/V7路线图.md` §二 S0-4），v6.3「开源（Open-Ended Tools）」的**开工门**（路线图 §五「治理裁决（开工门）：S0-4 的 code 通道政策提案正式生效」）。
>
> **提案性质**：安全承诺变更提案——把 code 层从「永久拒绝」修订为「**宿主永久拒绝 + 沙箱受控释放**」。本文件只做政策文本；不修改任何代码与题面。全部代码证据于 2026-08-31 在本工作区实读核对（文件:行号）。
>
> **生效要件**（路线图 §五治理裁决原文）：「属**安全承诺变更**，须维护者书面批准 + 红队样本集回归全过，**先于功能代码合并**。」——即：本提案批准 → 治理文本修订落地 → v6.3 功能代码才可开工。

---

## 一、现状红线与其代码位置

### 1.1 红线的代码载体

| # | 证据 | 位置 | 说明 |
|---|------|------|---------|
| R1 | 红线定义：`ScopeCode Scope = "code"` 注释为「代码层——沙箱永久禁止」 | `internal/agent/learning/feedback.go:32-33` | 自改进作用域白名单只有 prompt/config/skill 三层（:25-31） |
| R2 | 哨兵错误：`ErrImprovementScopeViolation = errors.New("learning: 自改进越界：code 层变更被沙箱禁止，需人工代码评审")` | `internal/agent/learning/feedback.go:36-37` | 全仓 grep `ErrImprovementScopeViolation` 仅 3 处：定义（:37）、抛出（:194）、对抗测试断言（feedback_test.go:87）——红线目前是**学习反馈子系统内的单点** |
| R3 | 抛出点：`Propose` 收到 ScopeCode 建议即返回该错误 | `internal/agent/learning/feedback.go:191-195` | 建议生成端（Suggest，:139-189）也只产出 prompt/config/skill 三类建议 |
| R4 | 建议应用须经人工批准（`Approve` 唯一生效通道），ApplyApproved 只落地 prompt/config/skill | `internal/agent/learning/feedback.go:206-213,222-253` | skill 层经 SkillSynthesizer 走 skills.Acquire（护栏+规范校验，`internal/agent/skills/acquisition.go:103-128`） |
| R5 | 宿主侧代码执行工具明确自我声明**不是沙箱**：「This is NOT a security sandbox. Code runs directly on the host with the privileges of the AgentPrimordia process」 | `internal/tools/builtin/code_execution.go:49-54` | 且默认禁用（环境变量 `AP_ALLOW_CODE_EXECUTION` 门，:68-75），执行通道是宿主子进程（`exec.CommandContext`，:151） |
| R6 | 红线的治理文本载体：V6 路线图 v5.4 任务 4「自改进安全边界……代码层变更强制人工审核」（`docs/V6路线图.md:133`）；根 AGENTS.md **无** code 层条款（§2/§3/§4 无一处涉及 agent 生成代码） | `docs/V6路线图.md:133,188` | 红线今天主要靠代码单点 + 路线图文本，未进工作区宪法 AGENTS.md——这是本提案要修订的治理缺口 |

### 1.2 红线的现实缺口（为什么需要修订而不是维持现状）

1. **锁死的是需求，不是能力**：能力缺口只能人肉补（V7 §五立项证据 ①）；工具系统是封闭集，封闭清单由「别人写了什么」决定。
2. **红线覆盖面过窄**：R2 显示红线只挂在 learning.Propose 一个入口上；宿主进程内实际存在两条可执行任意代码的通道——内置 code_execution（R5，环境变量开启后宿主子进程执行）与 shell 工具（经 `internal/security` 命令白名单）。红线没有回答「agent 生成的代码从哪里执行」。
3. **沙箱四件套已备而闭环未接**（V7 §五立项证据 ②）：wazero 运行时（`wasm/runtime.go`）、工具适配（`agentprimordia/wasm/tool_adapter.go` WASMToolAdapter.AsTool）、签名（Ed25519/cosign 两套实现）、市场协议（`internal/marketplace`）——缺的是把「生成 → 彩排 → 对抗测试 → 签名注册」接到沙箱上的**政策授权**。

结论：把「永久拒绝」改成「宿主永久拒绝 + 沙箱受控释放」不是放松，是把红线从单点挂到整个宿主进程边界上，同时给沙箱内开一条受审计的通道。

---

## 二、修订后的红线不变式：「宿主进程代码零写入零加载」

### 2.1 不变式陈述

> **INV-0**：AgentPrimordia 宿主进程在运行期（含 agent 自主行为全链路）**不得写入、编译或加载任何由 agent 生成的代码到宿主进程自身的地址空间**。agent 生成代码的唯一合法执行位置是 wazero WASM 沙箱实例内；其唯一合法产出是经签名与对抗测试后的**数据工件**（WASM 字节码 + 清单），通过注册表以工具形式被消费。

### 2.2 确定性断言列表（可穷尽测试）

> 每条断言独立成立；CI 中全部跑、每次跑、失败即红。检测点均给出建议实现位置（新增测试文件，不改动既有生产代码——实现期按 AGENTS.md §6 提交粒度落地）。

| # | 断言名 | 检测点 | 实现位置建议 | CI 穷尽方式 |
|---|--------|--------|------------|------------|
| A1 | **wazero 依赖边界封闭**：主模块内 import wazero 的 .go 文件集合 ≡ {agentprimordia/wasm/sandbox.go}（当前实测唯一命中；`agentprimordia/go.mod:7` 为依赖声明、AGENTS.md §2.1:27 为白名单边界） | go 包导入图静态扫描 | 新增 `internal/security/host_boundary_test.go`（或 scripts/ + CI job） | 对 `go list -deps ./...` 全量枚举每个包的 import，白名单外出现 wazero 即失败——包集合有限，可穷尽 |
| A2 | **零动态加载**：宿主源码树不出现 `plugin.Open`，go.mod 无 cgo 激活路径；构建产物不含 dlopen 符号引用 | 源码 grep + 构建符号检查 | 同 A1 测试文件 | 全仓 grep + `go build` 后对产物做符号表扫描（nm/objdump），每次 CI 全量执行 |
| A3 | **零宿主派生执行**：agent 生成代码不进入 `exec.CommandContext` 通道——内置 code_execution 保持默认禁用（`internal/tools/builtin/code_execution.go:69`），且该工具的开启开关不暴露给任何 agent 可写配置面 | 配置流静态断言 + 运行时断言 | 同 A1；运行时部分放 `internal/tools/builtin/code_execution_test.go` 追加用例 | ① 静态：枚举全部环境变量/配置写入点，断言 `AP_ALLOW_CODE_EXECUTION` 不在其中；② 运行时：以「agent 写配置→重启执行」的对抗用例证明开关不可被 agent 打开——配置面枚举有限，可穷尽 |
| A4 | **沙箱资源上限强制**：每个 WASM 模块实例必须带 MemoryLimitPages 上限与 ExecutionTimeout（默认 640KB/30s，`wasm/runtime.go:25-33`）；WASI 默认关闭（`EnableWASI=false`） | 沙箱构造参数审查 | `wasm/runtime_test.go` 追加（表驱动枚举全部公共构造入口） | 枚举 `agentprimordia/wasm` 与根 `wasm/` 全部公共构造函数（当前共 2 个模块、构造入口个位），逐个断言零值配置兜底生效——入口可穷尽 |
| A5 | **CPU 死循环可终止**：WithCloseOnContextDone(true)（`wasm/runtime.go:52-57`）下，无限循环模块在 ExecutionTimeout 内被真实终止（既有回归测试语义保留） | 沙箱执行器回归测试 | 根 `wasm/runtime_test.go` 既有无限循环用例 + `agentprimordia/wasm` 侧补齐同型用例 | 确定性测试（固定模块 + 固定超时），每次 CI 必跑 |
| A6 | **签名前置**：任何工具进入注册表（`WASMToolAdapter.RegisterTool`，`agentprimordia/wasm/tool_adapter.go:77-111`）前，其字节码必须通过签名验证（`wasm/signing.go:15-32` VerifySignature，或 §五统一的 cosign 口径）；验签失败的注册调用必须返回错误 | 适配器注册路径 | `agentprimordia/wasm/tool_adapter_test.go` 追加负样本用例（篡改 1 字节即拒绝） | 表驱动穷举篡改位（首/尾/中间字节翻转），签名校验本身是确定性算法——判定可穷尽 |
| A7 | **导入段白名单**：注册的 WASM 模块导入段（imports）不得包含未批准宿主函数；未显式启用 WASI 的模块不得出现 `wasi_snapshot_preview1` 导入 | 模块编译期检查（CompileModule 后枚举 ImportedFunctions） | `agentprimordia/wasm/sandbox.go` 建议新增校验函数（实现期评审）+ 对应测试 | WASM 导入段是二进制静态可枚举结构——每个注册模块逐条比对白名单，可穷尽 |
| A8 | **宿主文件系统零写入**：agent 生成工具执行前后，宿主工作树（Go 代码目录 + 配置目录）哈希不变；生成物只允许落在沙箱数据目录 | 执行 harness 前后全量哈希对比 | `internal/eval/` 沙箱 harness 测试（复用 v6.3 彩排 harness） | 每次彩排/对抗测试自动执行；哈希覆盖面 = 仓库文件集（有限、可枚举） |

### 2.3 为什么本不变式允许 100%/0 容忍（对应 V7 新规 R3）

R3 原文（V7 路线图 §一）：「**确定性逻辑不变式**（签名校验、CAS 原子性、预算强制）允许 100%/0，因其由代码保证可测试穷尽」。本不变式符合该豁免的理由，逐条对应：

1. **判定是算法不是抽样**：A1（导入图比对）、A2（符号表扫描）、A6（Ed25519/ECDSA 验签）、A7（导入段枚举比对）的判定函数都是确定性算法——同样输入必得同样输出，不存在统计涨落；「0 违例」是对算法输出的精确陈述，不是对未知分布的统计宣称。
2. **测试面有限可枚举**：Go 包集合、`go list` 导入边、WASM 导入段、公共构造入口都是有限集合且机器可枚举；穷尽 = 枚举全集，不是「测了很多个」。
3. **不依赖样本代表性**：A8 的哈希对比覆盖宿主工作树全集（分母 = 全部文件），不存在「漏检率」的估计问题；对比 R3 禁止的「质量类指标裸 100%」（如任务成功率 100%——那只是有限样本的观测），本不变式的 0 是结构性质。
4. **失效模式是显式的**：若某断言因平台/依赖变化不可再穷尽（例如未来引入合法的宿主插件机制），正确动作是走 §七 的安全承诺变更流程重新划界，而不是降级为抽样统计。

### 2.4 现状与不变式的差距（如实声明）

当前代码**尚未**满足 A6/A7/A8（注册路径无强制验签、无导入段白名单校验、彩排 harness 未建）；A1–A3 目前事实上成立（grep 证实）但无测试固化。本提案生效后，A1–A8 的补齐属于 v6.3 功能代码的**前置**（§七流程），不是随行项。

---

## 三、释放范围与不释放范围

### 3.1 释放：沙箱内工具生成（v6.3 六段生命周期）

释放的是「缺口检测 → 生成 → 世界模型预演 → WASM 沙箱彩排 → 对抗测试 → 签名注册」管线中**在沙箱内执行**的环节（对应双线豁免矩阵弧线登记 #3：「六段生命周期、签名/工具包格式、注册客户端双线对等；**沙箱彩排执行 Go-only**」）：

1. **隔离机制（真实能力边界）**：
   - **内存隔离**：wazero 线性内存按模块独立，页数上限由 `Config.MemoryLimitPages` 强制（默认 10 页 = 640KB，`wasm/runtime.go:14-33`）；
   - **时间隔离**：执行超时经 `WithCloseOnContextDone(true)` 在解释器指令级检查 ctx 取消，真实终止无限循环（`wasm/runtime.go:52-57`，有回归测试）；
   - **系统面隔离**：WASI 默认关闭（`EnableWASI=false`）——模块拿不到文件系统/时钟/环境变量等宿主能力；宿主数据交换只经 ABI 约定的线性内存读写（`wasm/tool_executor.go:1-11` ABI 协议；`agentprimordia/wasm/sandbox.go:103` ExecuteWithMemory）；
   - **宿主命令面兜底**：沙箱外的宿主命令执行仍受 `internal/security` 约束——`Sandbox.CanExecute` 检查 shell 元字符/黑名单/参数白名单（`internal/security/sandbox.go:264-297`）、`CanAccess` 无 ACL 时默认拒绝（:299-310）、`ValidatePath` 拒绝路径穿越（:312-321）、ACL deny 优先（:81-92）。该层是**命令白名单**不是系统调用沙箱——这正是 agent 生成代码绝不允许走宿主通道的原因（§3.2）。
2. **注册与消费**：生成工具经 `WASMToolAdapter.AsTool`（`agentprimordia/wasm/tool_adapter.go:186-193`）转为 `tools.Tool` 进注册表，被 ReActAgent 运行时直接调用——工具系统第一次成为开放集。
3. **治理面**：注册工具照常受 `governance.Enforcer.CheckToolCall`（`internal/governance/policy_enforcer.go:104-120`）与策略文件（`internal/governance/policy_loader.go:16-35`，YAML Policy 的 ToolRestriction.RequireApproval/MaxCallsPerRun/BlockedArgs，`policy.go:59-66`）约束——释放的是「造」，不豁免任何「用」的策略。

### 3.2 不释放：宿主进程（永久拒绝，无例外通道）

| 项 | 内容 | 依据 |
|----|------|------|
| 零写入 | agent 生成代码不得写入宿主源码树、`go.mod`、配置目录 | A8 断言；R1 红线保留 |
| 零加载 | 不得触发宿主侧编译/链接/动态加载（go build、plugin、cgo） | A2 断言 |
| 零宿主派生 | 不得经 `os/exec` 执行 agent 生成代码（含「先落盘再跑」） | A3 断言；code_execution 现状（R5）保持默认禁用且开关不对 agent 开放 |
| 零学习通道绕过 | `learning.ScopeCode` 在 feedback.Propose 的拒绝**保留不变**（`feedback.go:191-195`）——feedback 通道的 code 层仍走人工代码评审；沙箱释放是新通道，不改旧通道语义 | R1–R4 全部保留 |

### 3.3 已知缺口（书面披露，随 v6.3 逐项关闭）

1. **CPU 配额只有时间兜底**：`Config.MaxFuel` 是前向兼容字段——wazero v1.12.0 公共 API 无 WithFuel，CPU 配额以 ExecutionTimeout 落地（`wasm/runtime.go:18-22` 注释已自查确认）。缺口：同进程并发多模块的 CPU 公平性无保证。缓解：单实例超时强制（A5）+ 彩排并发上限。**fuel 到位前的多租户 CPU 公平性为推断（待证）**。
2. **WASI 开关是配置不是硬禁**：`EnableWASI` 可被宿主侧代码打开——由 A7 导入段白名单兜住「未批准即拒绝」，但「谁有权限配置 WASI」需要 v6.3 实现期落到运行时策略（入 governance）。
3. **TS 侧无等价运行时**：wazero 无 TS 对等物，TS 侧沙箱执行委托 gateway 服务节点（双线豁免矩阵 B4/#3 既有豁免）。gateway 节点自身的隔离强度与宿主边界**未在本工作区核实——推断（待证）**，v6.3 评审须补 gateway 侧同型断言清单；在此之前 TS 通道只允许协议对等（签名/格式），不允许本地执行。
4. **验签与获取分离**：marketplace.Install 已强制「验签失败即拒绝安装」（`internal/marketplace/marketplace.go:293-296`），但「安装后的工件在执行前是否再验」取决于消费路径——由 A6 注册前置验签闭合。

---

## 四、对抗测试与留出集要求（只读，不修改题面）

1. **题面来源**：v6.3 安全不变式（命题 3）的对抗测试统一消费 S0-2 冻结的 `docs/evals/adversarial-holdout-v1.json`（R4：验收只认留出集；开发期不得读取留出样本内容）。
2. **冻结台账**：`docs/evals/manifest.json` 登记该文件 `sha256 = 4c86eb1106ef4f6e8a5ddc0a5ef0bfb4e1779e63c5c6e38bdfef142662b67d50`，count = 700、holdout_count = 238（holdout_rate = 0.34）；台账冻结 commit `027b5c56542783dab601558eb99ced10b0458277`；台账 policy 声明「验收只认 holdout 子集成绩；题面一经冻结不得修改；扩充走新版本文件」。CI 对账门：`python3 scripts/eval-manifest.py --verify`（漂移退出码 1）。
3. **family 分类与本提案的映射**（family 分布为对本文件的只读统计，与 `docs/evals/README.md` 注册表登记一致）：

| family | 总数 | 留出 | 对应不变式/环节 |
|--------|------|------|----------------|
| prompt-injection | 180 | 67 | 彩排与生成链路的注入抵抗（guardrail 输入端 + 彩排 harness） |
| skill-card-poison | 100 | 40 | 签名信任链：伪造/篡改技能卡（§五） |
| bad-tool-package | 100 | 30 | 生成工具注册：坏包拒绝（A6/A7） |
| reputation-gaming | 140 | 42 | v6.5 信任层（非 v6.3 门，同集消费） |
| adapter-tamper | 80 | 27 | 适配器/注册路径篡改（A6 负样本用例同型） |
| benign-control | 100 | 32 | 良性对照：误拦率披露分母 |

4. **验收口径**（V7 §五命题 3 原文，确定性门允许 100）：「宿主代码零写入零加载：静态+运行时双校验 0 违例；对抗留出集漏检 0，若出现则全量披露+根因复审+补样重跑」。
5. **只读纪律**：本提案与 v6.3 实现均不修改 adversarial-holdout-v1.json、manifest.json 及其余题面；扩充走 `*-v2.json` 新版本文件（README 规则 3）。

---

## 五、签署/信任链要求

### 5.1 现状（三套签名实现并存）

| 实现 | 位置 | 算法 | 用途 |
|------|------|------|------|
| WASM 字节码签名 | `agentprimordia/wasm/signing.go:15-32`（VerifySignature）、:34-43（SignWASM）、:45-49（GenerateKeyPair）、:52-55（KeyFingerprint 指纹） | Ed25519 over SHA-256(wasmBytes) | 沙箱字节码完整性 |
| 市场工件签名 | `internal/marketplace/marketplace.go:48-81`（VerifyCosignSignature，SHA-256 摘要 + ECDSA P-256，ASN.1 DER，cosign 默认格式）；Manifest 结构 :28-46；安装强制验签 :262-315（:293-296 拒绝安装）；路径字段防穿越 :174-182 | ECDSA P-256（cosign verify-blob 口径） | 插件/工件分发 |
| 技能清单签名 | `internal/agent/skills/skill_market.go:21-33`（SkillManifest）、:35-36（SignSkillManifest）、:65-78（VerifySkillManifest，验签不匹配即错误）；包注释声明与 marketplace cosign 同强度 | ECDSA P-256 | 技能发布/订阅 |

> 如实注记：V6 路线图（:188）称 v5.4「技能合成经 SkillSynthesizer 接口打通 skills.Acquire 验签链路」；实测 `skills/acquisition.go:103-128` 的 Acquire 主路径为「提炼 → 护栏 → 规范校验」，未直接调用 VerifySkillManifest——签名入口与习得主路径的接线现状**推断为清单级（待证）**，v6.3 实现期以代码为准复核。

### 5.2 提案要求

1. **统一口径**：v6.3 生成工具链统一采用 marketplace cosign 口径（ECDSA P-256 + SHA-256，与 Manifest/Install 现有强制一致）；Ed25519 字节码签名（signing.go）保留为字节码完整性校验层——两者职责分开：工件级（市场）+ 字节级（沙箱）。
2. **信任锚**：验证公钥以维护者托管的固定公钥集为信任锚（KeyFingerprint 指纹入审计记录）；agent 自生成密钥不得自签入册（对抗 family = skill-card-poison / adapter-tamper 覆盖）。
3. **注册前置**：A6（验签前置）+ A7（导入段白名单）+ 彩排通过（CodeExecVerifier，V7 §五「强验证器族归位」）三关全过才可进入注册表；任一关失败即拒绝，拒绝原因入审计（复用 governance AuditLogger 通道，`internal/governance/policy_enforcer.go:81-92`）。
4. **审计完整性**：注册/拒绝/退役三类事件全量入审计；审计查询沿 AuditLogger 既有 Log/Query 接口（`internal/governance/audit_log.go`）。

---

## 六、回滚与事故响应

### 6.1 回滚开关（分级）

| 级别 | 动作 | 影响 |
|------|------|------|
| L1 工具级 | 注销单个生成工具（WASMToolAdapter.UnregisterTool，`agentprimordia/wasm/tool_adapter.go:114-130`） | 仅该工具下线 |
| L2 注册表级 | 停用生成工具注册表消费（运行时开关，实现期落 governance 策略） | 全部生成工具下线，预定义工具不受影响 |
| L3 通道级 | 关闭沙箱生成通道（v6.3 特性开关默认关闭语义） | 回到 v6.2 末行为：工具集封闭、红线回到全拒绝——**红线的宿主侧部分永远不回滚** |

回滚测试入 CI：三级开关各自有确定性测试（关闭后注册/执行调用必须失败）。

### 6.2 事故响应

1. **漏检事故**（对抗留出集出现漏检，或宿主边界断言 A1–A8 任一被突破）：立即触发 L3 → 按 R3 要求**全量披露**（漏检样本 id、时间线、影响面）→ 根因复审 → 补样重跑（扩充走 v2 题面文件）→ 复审报告入 docs/ 并挂 V7 路线图对应版本回填。
2. **资源事故**（沙箱逃逸尝试/资源耗尽）：单实例超时与内存上限是第一道硬闸（A4/A5）；事故记录全量入审计与失败库，异常模式并入 nightly 观察集。
3. **披露纪律**：任何安全事件不使用「点估计合格」话术结案；结案标准 = 根因修复 + 对应断言升级 + 回归全绿，三者在案。

---

## 七、「安全承诺变更须维护者书面批准」流程条款

> 本条为正式流程条款，批准后随 §八修订进 AGENTS.md（作为 §2.3 的一部分），并对未来一切安全承诺文本变更生效。

1. **定义**：安全承诺 = 本提案 INV-0 及 A1–A8 断言、AGENTS.md 中 code 层条款、签名信任链要求、护栏作用面声明。凡修改上述任一文本或弱化任一断言的行为，均为安全承诺变更。
2. **批准要件**（三者缺一不可）：
   - 维护者**书面**批准（评审记录落 docs/ 提案文件或评审纪要，含日期与批准人）——对应 V7 §五治理裁决原文；
   - 红队样本集回归全过（`docs/evals/adversarial-holdout-v1.json` 留出子集 + 宿主边界断言 A1–A8 全绿）；
   - **先于功能代码合并**：治理文本修订 commit 先行，功能代码 PR 在其后。
3. **禁止事项**：不得以「实验性」「临时豁免」「随行小改」名义绕过本流程（V7 新规 R1 容量诚实同样适用于安全文本）；不得在未修订文本的情况下先合并弱化断言的代码。
4. **记录**：每次安全承诺变更在 `agentprimordia/docs/版本规范.md`（废弃/迁移记录）与本文档 §九评审记录双登记。

---

## 八、对根 AGENTS.md 的具体修订文本

### 8.1 §2 技术栈约束（新增 §2.3 安全边界条款）

插入位置：§2.2 末尾（AGENTS.md:40 审批记录引文之后）新增小节：

```
### 2.3 code 层安全边界（宿主永久拒绝 + 沙箱受控释放）

- INV-0：宿主进程运行期零写入、零编译、零加载任何 agent 生成的代码；
  agent 生成代码唯一合法执行位置是 wazero WASM 沙箱（agentprimordia/wasm/、
  工作区根 wasm/ 模块），且必须经签名验证与对抗测试后方可注册为工具。
- 宿主边界由确定性断言 A1–A8 保证（定义见 docs/提案-code层沙箱受控释放.md §二），
  断言测试全部进 CI、失败即红；本边界属于确定性安全不变式，允许 100%/0 容忍
  （V7 路线图 §一 R3）。
- learning 反馈通道的 code 层拒绝（internal/agent/learning/feedback.go ScopeCode）
  保留不变：feedback 通道 code 层仍走人工代码评审，沙箱释放不改变该通道语义。
- 本条的任何修订属安全承诺变更，须维护者书面批准（流程见
  docs/提案-code层沙箱受控释放.md §七）。
```

### 8.2 §2.1 白名单 wazero 条目（边界扩注）

原文（AGENTS.md:27）：

```
- `github.com/tetratelabs/wazero` — 纯 Go（CGO-free）WebAssembly 运行时（G3-3 WASM 执行）。**仅限工作区根 `wasm/` 模块与主模块内 `wasm/` 包**（`agentprimordia/wasm/`）使用。WASM 运行时无标准库等价实现，wazero 为 CGO-free 纯 Go 实现，符合 §2.2 硬性需求豁免。
```

新文（末尾追加一句，白名单范围不变）：

```
- `github.com/tetratelabs/wazero` — 纯 Go（CGO-free）WebAssembly 运行时（G3-3 WASM 执行）。**仅限工作区根 `wasm/` 模块与主模块内 `wasm/` 包**（`agentprimordia/wasm/`）使用。WASM 运行时无标准库等价实现，wazero 为 CGO-free 纯 Go 实现，符合 §2.2 硬性需求豁免。该依赖同时是 code 层「沙箱受控释放」（§2.3）的隔离底座：除上述两处外任何包不得 import wazero（CI 断言 A1 强制）。
```

### 8.3 §3 代码规范（确定性安全不变式测试要求）

原文（AGENTS.md:42-49）：

```
## 3. 代码规范

- **TDD 强制**: 所有功能必须先写测试（Red → Green → Refactor）
- **接口优先**: LLM、Tools、Memory、Pool 全部通过接口解耦
- **并发安全**: 共享状态必须用 sync.RWMutex / sync.Mutex / channel 保护
- **错误处理**: 使用 `pkg/errors.go` 中定义的错误变量
- **中文注释**: 代码注释使用中文
- **代码风格**: 与现有代码保持一致（参考 internal/agent/、internal/pool/）
```

新文（末尾追加一条）：

```
## 3. 代码规范

- **TDD 强制**: 所有功能必须先写测试（Red → Green → Refactor）
- **接口优先**: LLM、Tools、Memory、Pool 全部通过接口解耦
- **并发安全**: 共享状态必须用 sync.RWMutex / sync.Mutex / channel 保护
- **错误处理**: 使用 `pkg/errors.go` 中定义的错误变量
- **中文注释**: 代码注释使用中文
- **代码风格**: 与现有代码保持一致（参考 internal/agent/、internal/pool/）
- **确定性安全不变式**: 安全边界断言（宿主零写入零加载、签名前置、导入段白名单等，见 §2.3）必须以确定性测试固化并全量进 CI；此类断言允许 100%/0 容忍（V7 路线图 R3），禁止以抽样统计口径改写
```

### 8.4 §4 模块边界（新增生成工具通道的边界注记）

插入位置：§4.2 依赖方向规则列表末尾（AGENTS.md:118 「operator/、pgvector/」条目之后）追加：

```
- **code 层生成工具通道（v6.3 起）**：agent 生成工具的生成/彩排/注册链路位于
  internal/tools/lifecycle/（生命周期框架）与 agentprimordia/wasm/（沙箱执行），
  依赖方向同上层规则；生成工件（WASM 字节码+清单）是数据不是代码——宿主编译
  与加载边界由 §2.3 INV-0 与断言 A1–A8 强制。TS 侧仅协议对等（签名/工具包格式/
  注册客户端），沙箱执行 Go-only（docs/双线豁免矩阵.md B4/#3 豁免）。
```

> 注：`internal/tools/lifecycle/` 目标路径与双线豁免矩阵弧线登记 #3 的 Go 侧路径一致（该包当前不存在，属 v6.3 交付物——此处登记的是边界位置而非现状）。

---

## 九、评审清单（勾选式）

**提案文本评审（维护者书面批准 = §七流程的第一次适用）**

- [ ] §一 现状红线证据（R1–R6）复核：ErrImprovementScopeViolation 全仓仅定义/抛出/测试三处的核实结论认可
- [ ] §二 INV-0 表述认可；A1–A8 断言逐条可测（检测点/实现位置/穷尽方式成立）
- [ ] §2.3 100%/0 容忍的 R3 论证成立（判定为算法 + 测试面可枚举 + 失效显式）
- [ ] §三 释放/不释放边界与已知缺口四条如实披露（含两条标注推断待证项）认可
- [ ] §四 题面只读承诺 + sha256/family 数字与 docs/evals/manifest.json 一致
- [ ] §五 统一 cosign 口径 + 信任锚 + 三关注册前置认可；Acquire 验签接线待证项已如实标注
- [ ] §六 三级回滚与事故响应（全量披露+根因复审）成立
- [ ] §七 流程条款认可并作为 AGENTS.md §2.3 落地
- [ ] §八 四处 AGENTS.md 修订稿可原文落地

**v6.3 功能代码合并前置（先于任何生成链路代码）**

- [ ] AGENTS.md 四处修订 commit 先行合入
- [ ] A1–A8 断言测试全绿并进 CI
- [ ] adversarial-holdout-v1.json 留出子集回归全过（eval-manifest.py --verify 对账）

---

**评审记录**（批准后回填）

| 日期 | 结论 | 意见摘要 | 批准人 |
|------|------|---------|--------|
| | | | |


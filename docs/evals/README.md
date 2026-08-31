# docs/evals — V7 弧线题面注册处（S0-2 / 新规 R4）

> **规则**（详见 `../V7路线图.md` §一 R4）：
> 1. 一切验收题面在本目录注册并**独立 commit 冻结**；manifest.json 记录冻结 commit 与逐文件 sha256；
> 2. 验收只认 `holdout: true` 子集成绩；开发期不得读取留出样本内容（`holdout: false` 为可见子集）；
> 3. 题面一经冻结不得修改；扩充走新版本文件（`*-v2.json`），旧版永久留档；
> 4. 计划书模板见 `../实验成本与功效模板.md`。

## 注册表

| 文件 | 用途 | 规模 | 消费版本 | 状态 |
|------|------|------|---------|------|
| `long-horizon-v1.json` | 长程任务集（≥3 里程碑断言 / ≥1 跨会话中断 / 预算 45-60 轮 90 调用） | 24 题（留出 9） | v6.1 命题 1/3、v6.4 题源 | ✅ 冻结 |
| `gap-tools-v1.json` | 能力缺口任务集（基础工具面缺位 → 自主造工具） | 65 题（留出 23） | v6.3 命题 1 | ✅ 冻结 |
| `adversarial-holdout-v1.json` | 对抗/投毒样本集（注入 180 / 毒卡片 100 / 坏工具包 100 / 刷声誉 140 / 篡改 adapter 80 / 良性对照 100） | 700 样本（留出 238） | v6.3 命题 3、v6.5 命题 3 | ✅ 冻结 |
| `external-general-v1.json` | 外部泛化对账集（算术/字符串/逻辑/日期/代码输出，答案可机检） | 100 题（留出 34） | S0-1 泛化对账、v7.0 复测 | ✅ 冻结 |
| `judge-calibration-v1.json` | judge 标定集（客观标签的 good/bad 输出对，各 100） | 200 样本（留出 68） | S0-1 judge κ 标定 | ✅ 冻结 |
| `baseline-probe-v1.json` | 基线摸底小集（长程集前 12 题引用，供效应量假设） | 12 题（留出 5） | 各计划书 | ✅ 冻结 |
| `manifest.json` | **冻结台账**：逐文件 sha256 + 留出/可见 id 清单 + 冻结 commit | — | CI 门 `--verify` | ✅ 冻结 |

合计 **1101 个注册样本 / 377 留出（34%）**，全部满足 R4 的 ≥30% 留出与规模下限（长程 ≥20、缺口 ≥50、对抗 ≥500、外部 ≥100、judge ≥200）。

> 消费方治理文本（S0-4 提案，只读引用本目录冻结题面）：v6.1 验收线见 [`../提案-世界模型默认策略切换.md`](../提案-世界模型默认策略切换.md)；v6.3 安全门（对抗集 0 漏检口径与回滚）见 [`../提案-code层沙箱受控释放.md`](../提案-code层沙箱受控释放.md)。

## 工具链

| 命令 | 作用 |
|------|------|
| `python3 scripts/gen-eval-sets.py` | 确定性再生成全部题面（固定 SEED=20260831，字节级一致，已实测 diff 为空） |
| `python3 scripts/eval-manifest.py --write` | 重算 sha256 并写 `manifest.json` |
| `python3 scripts/eval-manifest.py --verify` | **CI 门**：题面与台账对账，任何漂移退出码 1 |
| `python3 scripts/eval-manifest.py --check` | 结构自检：留出比例 ≥30%、id 无重叠 |
| `go test ./internal/eval/ -run TestEvalRegistryFrozen` | Go 侧冻结门：读 manifest 校验 sha256 与留出率，防手写题面绕过 |

## Schema v2（长程/缺口集）

```jsonc
{
  "id": "lh-001", "name": "...", "category": "long-horizon|gap-tool",
  "difficulty": "medium|hard", "lang": "zh|en|multi",
  "goal": "终局目标（可验证）",
  "fixtures": [{"path": "...", "content_sha": "...|inline"}],   // 初始环境
  "toolset": ["filesystem", "shell", "code_execution"],           // 沙箱可用工具
  "absent_capability": "缺口任务：基础工具面刻意缺少的能力描述",   // gap-tool 专用
  "milestones": [{"id": "m1", "assert": "<file/inventory 断言>"}],// 里程碑断言（≥3）
  "interruptions": [{"after_milestone": "m2", "action": "session_restart"}],
  "grading": {"success": ["<终局确定性断言>"], "partial": ["<过程分>"]},
  "budget": {"max_turns": 60, "max_tool_calls": 90},
  "holdout": true
}
```

**判定口径**：`success` 断言全过 = 任务成功（二值）；`partial` 计过程分，仅作次指标。所有断言由 harness 在沙箱终态上执行（文件存在/内容哈希/行集包含/JSON 路径等值），不依赖 judge 主观判断——judge 只用于开放式输出的质量标定集，不用于本目录题面的成功判定。

## 留出机制说明

- 每文件 `holdout: true` 占比 ≥30%，留出位由生成器以固定种子散布（见 `../../scripts/gen-eval-sets.py` 的 SEED 与 `HOLDOUT_RATE`）；
- 生成器同时输出 `visible/` 与 `holdout/` 的 id 清单于 manifest，runner 按题集 flag 装载；
- **开发环境不得 import 留出 id 的 fixture 细节**——执行纪律，评审抽查。

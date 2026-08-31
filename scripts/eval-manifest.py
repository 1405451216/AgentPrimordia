#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""eval-manifest.py — V7 弧线题面注册冻结台账（S0-2 / 新规 R4）

两种模式：
  --write  生成/更新 docs/evals/manifest.json（逐文件 sha256 + 留出/可见 id 清单）
  --verify CI 门：重算 sha256 与 manifest 对账，任何题面漂移即退出码 1（题面一经冻结不得修改）

留出纪律：留出（holdout:true）样本的 id 记入台账，其内容在验收前对开发不可见；
runner 只按 --holdout-only / --visible-only flag 装载子集，评审抽查执行纪律。"""
import argparse, hashlib, json, os, subprocess, sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
EVALS = os.path.join(ROOT, "docs", "evals")
MANIFEST = os.path.join(EVALS, "manifest.json")
SEED = 20260831
HOLDOUT_RATE = 0.34
GENERATOR = "scripts/gen-eval-sets.py"

# 注册文件 -> (用途, 消费版本, 判定口径)
REGISTRY = {
    "long-horizon-v1.json": (
        "长程任务集（≥3 里程碑断言 + ≥1 次跨会话中断）", "v6.1 命题 1/3、v6.4 题源", "沙箱终态确定性断言"),
    "gap-tools-v1.json": (
        "能力缺口任务集（基础工具面刻意缺位 → 自主造工具）", "v6.3 命题 1", "工具注册+彩排/输出等值"),
    "adversarial-holdout-v1.json": (
        "对抗/投毒留出集（注入/毒卡片/坏工具包/刷声誉/篡改 adapter + 良性对照）", "v6.3 命题 3、v6.5 命题 3", "期望裁决二值比对"),
    "external-general-v1.json": (
        "外部泛化对账集（答案可机检）", "S0-1 泛化对账、v7.0 复测", "exact 等值"),
    "judge-calibration-v1.json": (
        "judge 标定集（客观标签的 good/bad 输出对）", "S0-1 judge κ 标定", "标签一致性/Cohen κ"),
    "baseline-probe-v1.json": (
        "基线摸底小集（长程集可见子集抽样，供效应量假设）", "各实验计划书", "引用长程集判定"),
    "embedding-corpus-v1.json": (
        "S0-3 真实 corpus 双线召回基准（真实文档 chunk + 标题术语规则推出的 gold 查询；"
        "visible/holdout 子集各自自含）", "S0-3（CI 回归跑 visible 子集，终验跑 holdout 子集）",
        "recall@10：lexical 臂对实测底档阈值，语义臂 ≥0.95（端点就位时）"),
}


def sha256_file(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def git(*args):
    try:
        return subprocess.run(["git", *args], cwd=ROOT, capture_output=True, text=True,
                              check=True).stdout.strip()
    except Exception:
        return ""


def build_entries():
    entries = []
    for name, (purpose, consumers, oracle) in REGISTRY.items():
        path = os.path.join(EVALS, name)
        if not os.path.exists(path):
            print(f"❌ 注册文件缺失: docs/evals/{name}", file=sys.stderr)
            sys.exit(1)
        with open(path, encoding="utf-8") as f:
            items = json.load(f)
        hold = [it["id"] for it in items if it.get("holdout")]
        vis = [it["id"] for it in items if not it.get("holdout")]
        entries.append({
            "file": f"docs/evals/{name}",
            "purpose": purpose,
            "consumers": consumers,
            "oracle": oracle,
            "count": len(items),
            "sha256": sha256_file(path),
            "holdout_count": len(hold),
            "holdout_rate": round(len(hold) / len(items), 4) if items else 0.0,
            "holdout_ids": sorted(hold),
            "visible_ids": sorted(vis),
        })
    return entries


def main():
    ap = argparse.ArgumentParser()
    g = ap.add_mutually_exclusive_group(required=True)
    g.add_argument("--write", action="store_true", help="生成/更新 manifest.json")
    g.add_argument("--verify", action="store_true", help="CI 对账（题面漂移即失败）")
    g.add_argument("--check", action="store_true", help="结构自检：留出比例与 id 唯一性")
    ap.add_argument("--freeze-commit", default="", help="冻结 commit（题面文件的独立 commit 哈希）")
    args = ap.parse_args()

    entries = build_entries()

    for e in entries:
        if e["count"] and e["holdout_rate"] < 0.30:
            print(f"❌ R4 违规：{e['file']} 留出比例 {e['holdout_rate']:.2%} < 30%", file=sys.stderr)
            sys.exit(1)
        if len(set(e["holdout_ids"]) & set(e["visible_ids"])):
            print(f"❌ {e['file']} 留出/可见 id 重叠", file=sys.stderr)
            sys.exit(1)

    if args.check:
        for e in entries:
            print(f"✅ {e['file']}: {e['count']} 条，留出 {e['holdout_count']} ({e['holdout_rate']:.0%})")
        return

    if args.verify:
        if not os.path.exists(MANIFEST):
            print("❌ manifest.json 不存在——题面未冻结", file=sys.stderr)
            sys.exit(1)
        with open(MANIFEST, encoding="utf-8") as f:
            man = json.load(f)
        registered = {e["file"]: e for e in man["files"]}
        bad = 0
        for e in entries:
            old = registered.get(e["file"])
            if old is None:
                print(f"❌ 未注册文件出现在台账之外: {e['file']}", file=sys.stderr); bad += 1; continue
            if old["sha256"] != e["sha256"]:
                print(f"❌ 题面漂移 {e['file']}: 台账 {old['sha256'][:12]} ≠ 实际 {e['sha256'][:12]}"
                      "（一经冻结不得修改，扩充走 *-v2.json）", file=sys.stderr); bad += 1
        for name in registered:
            if name not in {e["file"] for e in entries}:
                print(f"❌ 台账条目对应文件被删除: {name}", file=sys.stderr); bad += 1
        if bad:
            sys.exit(1)
        print(f"✅ 题面台账对账通过（{len(entries)} 个注册文件，sha256 全部一致）")
        return

    head = git("rev-parse", "HEAD")
    # R4 防回退：已登记的冻结 commit 不得因重算台账被冲回占位符
    #（e7995ba6 事故：追加 embedding 语料时 --write 未显式传参，d0491cbe 的登记被冲掉）。
    # 规则：显式传 --freeze-commit 才可变更；否则保留既有非 PENDING 值。
    old_freeze = "PENDING-FREEZE-COMMIT"
    if os.path.exists(MANIFEST):
        with open(MANIFEST, encoding="utf-8") as f:
            old_freeze = json.load(f).get("freeze_commit") or "PENDING-FREEZE-COMMIT"
    freeze_commit = args.freeze_commit or old_freeze
    if freeze_commit != old_freeze:
        print(f"⚠️ freeze_commit 显式变更：{old_freeze[:12]} → {freeze_commit[:12]}（确认题面确有新版本再执行）")
    elif old_freeze != "PENDING-FREEZE-COMMIT":
        print(f"ℹ️ freeze_commit 保留既有登记 {old_freeze[:12]}")
    manifest = {
        "schema": "ap-eval-registry/v1",
        "generated_by": GENERATOR,
        "seed": SEED,
        "policy": {
            "holdout_min_rate": 0.30,
            "rule": "验收只认 holdout 子集成绩；题面一经冻结不得修改；扩充走新版本文件",
            "governance": "docs/V7路线图.md §一 R4",
        },
        "generated_at": git("log", "-1", "--format=%cI", "--", "docs/evals"),
        "git_head_at_manifest_write": head,
        "freeze_commit": freeze_commit,
        "files": entries,
    }
    with open(MANIFEST, "w", encoding="utf-8") as f:
        json.dump(manifest, f, ensure_ascii=False, indent=1, sort_keys=True)
        f.write("\n")
    total = sum(e["count"] for e in entries)
    th = sum(e["holdout_count"] for e in entries)
    print(f"✅ manifest.json 已写入：{len(entries)} 文件 / {total} 样本 / 留出 {th} ({th/total:.0%})")


if __name__ == "__main__":
    main()
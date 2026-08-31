#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""gen-embedding-corpus.py — S0-3 真实 corpus 生成器（确定性）。

从仓库真实技术文档（agentprimordia/docs/，必要时补 docs/）按 Markdown 标题切分
chunk，并以「标题术语匹配」确定性推出查询的 gold chunk 列表，生成
docs/evals/embedding-corpus-v1.json。禁止手写猜测：gold 全部由规则计算，
同输入必得同输出（重跑逐字节一致，可用 sha256 对账）。

输出 schema（顶层为 JSON 数组——题面台账 eval-manifest.py 按数组逐项登记 holdout）：
  {"id":"ec-ch-0001","type":"chunk","source":"agentprimordia/docs/x.md",
   "title":"文档标题 > 章节标题","text":"…","holdout":false}
  {"id":"ec-q-0001","type":"query","term":"…","text":"…",
   "gold":["ec-ch-…"],"holdout":false}

规则与纪律：
  1. chunk：按 ## 章节切分（代码围栏内不误判标题），段落在 ≤MAX_CHUNK_CHARS
     约束下打包；太碎的段落并入相邻组；每文档 chunk 数有上限（多样性）。
  2. holdout（R4 留出纪律）：chunk 按序号 %10 ∈ {7,8,9} 留出（30%）；
     查询按候选序 %5 ∈ {0,1} 留出（40%）。整体留出率必须 ≥30%（脚本自检）。
  3. 子集自含约束：visible 查询的 gold ⊆ visible chunk；holdout 查询的
     gold ⊆ holdout chunk——两个子集各自是自含的小语料（CI 回归跑 visible 子集，
     S0-3 终验跑 holdout 子集，互不泄漏）。
  4. 查询文本由模板从术语生成（模板轮换按候选序取模，非手写）。

用法：
  python3 scripts/gen-embedding-corpus.py            # 生成并打印摘要
  python3 scripts/gen-embedding-corpus.py --verify   # 重跑对账（确定性自检）

生成后注册：scripts/eval-manifest.py --write && --verify。
"""
import argparse
import hashlib
import json
import os
import re
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
OUT_PATH = os.path.join(ROOT, "docs", "evals", "embedding-corpus-v1.json")

# 语料源（按序取用；足够达到 chunk 目标数即停止，保证确定性）
SOURCES = [
    "agentprimordia/docs/AP-开发手册.md",
    "agentprimordia/docs/AP-CLI开发手册.md",
    "agentprimordia/docs/版本规范.md",
    "agentprimordia/docs/项目状态.md",
    "agentprimordia/docs/供应链安全.md",
    "agentprimordia/docs/安全态势.md",
    "agentprimordia/docs/密钥审计.md",
    "agentprimordia/docs/index.md",
    "agentprimordia/docs/AP-开发手册-EN.md",
    # 根 docs/（补充真实技术文档，按序取用）
    "docs/快速开始.md",
    "docs/部署指南.md",
    "docs/实验成本与功效模板.md",
    "docs/路线图.md",
    "docs/架构图.md",
]

TARGET_CHUNKS = 200      # 180-220 的中位
MAX_PER_DOC = 30         # 单文档上限：防止单一文档垄断语料
MAX_CHUNK_CHARS = 700    # chunk 打包上限（字符）
MIN_CHUNK_CHARS = 60     # 低于此值的段组并入相邻组
TARGET_QUERIES = 70      # 60-80 的中位
MAX_GOLD = 8             # gold 上限（≤10 才可能在 recall@10 内全召回）

# 拉丁术语停用词（通用词不入查询）
STOPWORDS = {
    "the", "and", "for", "with", "this", "that", "from", "into", "not",
    "are", "can", "all", "any", "use", "used", "using", "via", "how",
    "api", "sdk", "com", "www", "http", "https", "md", "todo", "note",
}

CJK_RE = re.compile(r"[\u4e00-\u9fff]{2,8}")
LATIN_RE = re.compile(r"[A-Za-z][A-Za-z0-9_]{2,}")


def read_text(relpath):
    with open(os.path.join(ROOT, relpath), encoding="utf-8") as f:
        return f.read()


def split_sections(text):
    """按 ## 标题切分章节；代码围栏内的 # 不视为标题。返回 (H1, [(章节标题, 正文)])。"""
    h1 = ""
    sections = []
    cur_title, cur_lines = "", []
    in_fence = False
    for line in text.splitlines():
        stripped = line.strip()
        if stripped.startswith("```") or stripped.startswith("~~~"):
            in_fence = not in_fence
        m = re.match(r"^(#{1,3})\s+(.*)$", stripped) if not in_fence else None
        if m and len(m.group(1)) == 1:
            h1 = m.group(2).strip()
            continue
        if m and len(m.group(1)) == 2:
            if cur_lines:
                sections.append((cur_title, "\n".join(cur_lines)))
            cur_title = m.group(2).strip()
            cur_lines = []
            continue
        cur_lines.append(line)  # ### 及正文都进当前章节
    if cur_lines:
        sections.append((cur_title, "\n".join(cur_lines)))
    return h1, sections


def pack_paragraphs(body, title_h1, title_sec):
    """章节正文按空行切段，打包为 ≤MAX_CHUNK_CHARS 的 chunk 文本组。"""
    paras = [p.strip() for p in re.split(r"\n\s*\n", body) if p.strip()]
    groups, buf = [], ""
    prefix = (title_h1 + "\n" + title_sec + "\n").strip() + "\n"
    for p in paras:
        candidate = (buf + "\n\n" + p).strip() if buf else p
        if buf and len(candidate) > MAX_CHUNK_CHARS:
            groups.append(buf)
            buf = p
        else:
            buf = candidate
    if buf:
        groups.append(buf)
    # 过短组并入相邻组
    merged = []
    for g in groups:
        if merged and len(g) < MIN_CHUNK_CHARS:
            merged[-1] = merged[-1] + "\n\n" + g
        else:
            merged.append(g)
    return [prefix + g for g in merged if len(g) >= MIN_CHUNK_CHARS]


def build_chunks():
    chunks = []
    for src in SOURCES:
        if not os.path.exists(os.path.join(ROOT, src)):
            continue
        text = read_text(src)
        h1, sections = split_sections(text)
        doc_chunks = []
        for sec_title, body in sections:
            title = (h1 + " > " + sec_title) if h1 else sec_title
            for packed in pack_paragraphs(body, h1, sec_title):
                doc_chunks.append({"source": src, "title": title, "text": packed})
        for c in doc_chunks[:MAX_PER_DOC]:
            if len(chunks) >= TARGET_CHUNKS:
                break
            chunks.append(c)
        if len(chunks) >= TARGET_CHUNKS:
            break
    return chunks


def title_terms(title):
    """标题 → 候选术语：CJK 词段 + 拉丁词（小写、去停用词）。"""
    terms = []
    for m in CJK_RE.finditer(title):
        terms.append(m.group(0))
    for m in LATIN_RE.finditer(title):
        w = m.group(0).lower()
        if w not in STOPWORDS and len(w) >= 3:
            terms.append(w)
    return terms


CJK_TEMPLATE = [
    "{t} 的配置方法是什么",
    "如何使用 {t}",
    "{t} 的工作机制是什么",
    "介绍 {t} 的用法与注意事项",
]
LATIN_TEMPLATE = [
    "how to configure {t}",
    "what does {t} do",
    "{t} usage and examples",
]


def build_queries(chunks):
    """术语 → gold（标题含该术语的 chunk），规则化生成查询。

    留出标记按「最终列表位置」确定性分配（%5 ∈ {0,1} = 精确 40%），
    不受候选跳过影响；gold 在标记后按子集自含约束过滤。
    """
    for i, c in enumerate(chunks):
        c["_holdout"] = (i % 10) in (7, 8, 9)

    # 术语 → 命中 chunk（标题子串匹配；拉丁已统一小写）
    term_hits = {}
    for c in chunks:
        seen = set()
        for t in title_terms(c["title"]):
            if t in seen:
                continue
            seen.add(t)
            term_hits.setdefault(t, []).append(c["id"])

    candidates = [t for t, ids in term_hits.items() if len(ids) <= MAX_GOLD]
    # 确定性排序：df 降序（gold 多者信息量大）、同 df 按术语字典序
    candidates.sort(key=lambda t: (-len(term_hits[t]), t))

    queries = []
    used_gold = set()
    for term in candidates:
        if len(queries) >= TARGET_QUERIES:
            break
        holdout = (len(queries) % 5) in (0, 1)  # 按最终位置分配 → 精确 40%
        # 子集自含：gold 只取与查询同子集的 chunk
        gold = [cid for cid in term_hits[term]
                if chunks[int(cid.split("-")[-1]) - 1]["_holdout"] == holdout]
        if not (1 <= len(gold) <= MAX_GOLD):
            continue
        key = (holdout, tuple(gold))
        if key in used_gold:
            continue
        used_gold.add(key)
        rank = len(queries)
        is_cjk = bool(CJK_RE.fullmatch(term))
        if is_cjk:
            text = CJK_TEMPLATE[rank % len(CJK_TEMPLATE)].format(t=term)
        else:
            text = LATIN_TEMPLATE[rank % len(LATIN_TEMPLATE)].format(t=term)
        queries.append({"term": term, "text": text, "gold": gold, "_holdout": holdout})
    return queries


def build_items():
    chunks = build_chunks()
    # 先分配 id（build_queries 的 gold 引用与子集定位都依赖 id）
    for i, c in enumerate(chunks, 1):
        c["id"] = "ec-ch-%04d" % i
    queries = build_queries(chunks)
    items = []
    for c in chunks:
        items.append({
            "id": c["id"],
            "type": "chunk",
            "source": c["source"],
            "title": c["title"],
            "text": c["text"],
            "holdout": c["_holdout"],
        })
    for i, q in enumerate(queries, 1):
        items.append({
            "id": "ec-q-%04d" % i,
            "type": "query",
            "term": q["term"],
            "text": q["text"],
            "gold": q["gold"],
            "holdout": q["_holdout"],
        })
    return chunks, queries, items


def dump(items):
    # 确定性序列化：sort_keys + 固定缩进 + 尾换行（无时间戳）
    return json.dumps(items, ensure_ascii=False, sort_keys=True, indent=1) + "\n"


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--verify", action="store_true", help="重跑对账（确定性自检）")
    args = ap.parse_args()

    chunks, queries, items = build_items()
    payload = dump(items)

    # 自检：规模与 R4 留出率
    n_hold = sum(1 for it in items if it["holdout"])
    rate = n_hold / len(items) if items else 0.0
    errors = []
    if not (180 <= len(chunks) <= 220):
        errors.append("chunk 数 %d 不在 180-220" % len(chunks))
    if not (60 <= len(queries) <= 80):
        errors.append("查询数 %d 不在 60-80" % len(queries))
    if rate < 0.30:
        errors.append("留出率 %.4f < 0.30（R4）" % rate)
    gold_ids = {c["id"] for c in items if c["type"] == "chunk"}
    for q in (it for it in items if it["type"] == "query"):
        bad = [g for g in q["gold"] if g not in gold_ids]
        if bad:
            errors.append("查询 %s 引用不存在的 gold: %s" % (q["id"], bad))
        qidx = int(q["id"].split("-")[-1]) - 1
        qhold = q["holdout"]
        for g in q["gold"]:
            chold = items[int(g.split("-")[-1]) - 1]["holdout"]
            if chold != qhold:
                errors.append("子集泄漏：查询 %s(holdout=%s) 引用 chunk %s(holdout=%s)"
                              % (q["id"], qhold, g, chold))
    if errors:
        for e in errors:
            print("❌ " + e, file=sys.stderr)
        sys.exit(1)

    if args.verify:
        if not os.path.exists(OUT_PATH):
            print("❌ 输出文件不存在", file=sys.stderr)
            sys.exit(1)
        with open(OUT_PATH, "rb") as f:
            old = f.read()
        if hashlib.sha256(old).hexdigest() != hashlib.sha256(payload.encode("utf-8")).hexdigest():
            print("❌ 确定性对账失败：重跑输出与已存文件不一致", file=sys.stderr)
            sys.exit(1)
        print("✅ 确定性对账通过（逐字节一致）")
        return

    os.makedirs(os.path.dirname(OUT_PATH), exist_ok=True)
    with open(OUT_PATH, "w", encoding="utf-8") as f:
        f.write(payload)
    print("✅ 生成 %s" % os.path.relpath(OUT_PATH, ROOT))
    print("   chunks=%d queries=%d 总样本=%d 留出=%d (%.1f%%)"
          % (len(chunks), len(queries), len(items), n_hold, rate * 100))
    print("   sha256=%s" % hashlib.sha256(payload.encode("utf-8")).hexdigest()[:16] + "…")


if __name__ == "__main__":
    main()
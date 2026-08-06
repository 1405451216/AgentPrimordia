# -*- coding: utf-8 -*-
"""生成 AgentPrimordia v4.0 架构图（SVG + PNG）。

布局：7 层纵向分层（入口 → 公共API → 核心引擎 → 能力四件套 → 编排 → 横向支撑 → 基础设施）
风格：现代化深色技术风（深空背景 + 霓虹渐变 + 清晰分区）
转换：优先 cairosvg，失败回退 Edge 无头截图
"""
import os
import subprocess
import sys

W, H = 2400, 1460
# 内容缩放因子：<1 时整体放大（字体更大、布局等比），viewBox 相应缩小
SCALE = 0.68
RW, RH = int(W * SCALE), int(H * SCALE)
BG_TOP, BG_MID, BG_BOT = "#ffffff", "#f8fafc", "#f1f5f9"

# 分区配色（霓虹色系）
C = {
    "core":  "#00d4ff",  # 核心引擎
    "llm":   "#d946d9",  # LLM
    "tools": "#2dd4a0",  # 工具
    "mem":   "#f0a030",  # 记忆
    "pool":  "#3b82f6",  # Pool
    "orch":  "#a78bfa",  # 编排
    "a2a":   "#38bdf8",  # A2A
    "sec":   "#f43f5e",  # 安全
    "guard": "#ec4899",  # 护栏
    "obs":   "#eab308",  # 可观测
    "cfg":   "#84cc16",  # 配置
    "infra": "#94a3b8",  # 基础设施
    "dbg":   "#34d399",  # 调试
    "adm":   "#fb923c",  # 管理
    "evt":   "#14b8a6",  # 事件
    "slate": "#64748b",  # 次要文字
}

def _g(id_, *stops):
    # stops 为扁平参数：offset, color, alpha, offset, color, alpha, ...
    it = iter(stops)
    pairs = zip(it, it, it)
    return (f'<linearGradient id="{id_}" x1="0" y1="0" x2="0" y2="1">' +
            "".join(f'<stop offset="{o}" stop-color="{c}" stop-opacity="{a}"/>' for o, c, a in pairs) +
            "</linearGradient>")

def box(x, y, w, h, fill, stroke, rx=10, sw=1.2, fill_op=None, dash=None, glow=False):
    extra = f' stroke-dasharray="{dash}"' if dash else ""
    filt = ' filter="url(#gl)"' if glow else ''
    return (f'<rect x="{x}" y="{y}" width="{w}" height="{h}" rx="{rx}" fill="{fill}"'
            + (f' fill-opacity="{fill_op}"' if fill_op is not None else "")
            + f' stroke="{stroke}" stroke-opacity="0.5" stroke-width="{sw}"{extra}{filt}/>')

def chip(x, y, w, h, color, text, fs=12, sw=0.8, opacity=0.12, txt_op=0.8):
    """小标签块"""
    tx = x + w / 2
    ty = y + h / 2
    return (f'<rect x="{x}" y="{y}" width="{w}" height="{h}" rx="{h/2}" fill="{color}" fill-opacity="{opacity}" '
            f'stroke="{color}" stroke-opacity="0.35" stroke-width="{sw}"/>'
            f'<text x="{tx}" y="{ty}" text-anchor="middle" dominant-baseline="central" fill="{color}" '
            f'font-family="system-ui,sans-serif" font-size="{fs}" opacity="{txt_op}">{text}</text>')

def title(x, y, text, fs=15, weight=600, color="#0f172a", anchor="middle"):
    return (f'<text x="{x}" y="{y}" text-anchor="{anchor}" fill="{color}" '
            f'font-family="system-ui,sans-serif" font-size="{fs}" font-weight="{weight}">{text}</text>')

def sub(x, y, text, fs=10, color="#64748b", anchor="middle", letter=0):
    l = f' letter-spacing="{letter}"' if letter else ""
    return (f'<text x="{x}" y="{y}" text-anchor="{anchor}" fill="{color}" '
            f'font-family="system-ui,sans-serif" font-size="{fs}"{l} opacity="0.7">{text}</text>')

def panel(x, y, w, h, label, color, label_x=None):
    """分区大框：左上角层标签"""
    lx = label_x if label_x is not None else x + 24
    return (box(x, y, w, h, color, color, rx=14, sw=0.7, fill_op=0.02, dash="7 5") +
            f'<text x="{lx}" y="{y + 22}" fill="{color}" font-family="system-ui,sans-serif" '
            f'font-size="10" font-weight="700" letter-spacing="3" opacity="0.5">{label}</text>')

def arrow(x1, y1, x2, y2, color="#00d4ff", dash=None, marker="a_c", opacity=0.4, sw=1.2):
    d = f' stroke-dasharray="{dash}"' if dash else ""
    return (f'<line x1="{x1}" y1="{y1}" x2="{x2}" y2="{y2}" stroke="{color}" stroke-width="{sw}" '
            f'opacity="{opacity}"{d} marker-end="url(#{marker})"/>')

def card(x, y, w, h, color, title_text, sub_text, fs_t=15, fs_s=10):
    """能力卡片：带光晕的外框 + 标题 + 副标题"""
    cx, cy = x + w / 2, y + h / 2
    return (f'<rect x="{x-6}" y="{y-6}" width="{w+12}" height="{h+12}" rx="16" fill="{color}" opacity="0.05" filter="url(#gs)"/>'
            + box(x, y, w, h, color, color, rx=12, sw=1.2, fill_op=0.06)
            + title(cx, y + 40, title_text, fs=fs_t, color="#0f172a")
            + sub(cx, y + 62, sub_text, fs=fs_s, color=color))

# ============ 生成 SVG ============
svg = []
svg.append(f'<svg xmlns="http://www.w3.org/2000/svg" width="{W}" height="{H}" viewBox="0 0 {RW} {RH}">')
svg.append("<defs>")
svg.append(f'<radialGradient id="bgR" cx="50%" cy="25%" r="75%"><stop offset="0%" stop-color="{BG_TOP}"/>'
           f'<stop offset="55%" stop-color="{BG_MID}"/><stop offset="100%" stop-color="{BG_BOT}"/></radialGradient>')
svg.append(_g("g_core", "0%", C["core"], "0.22", "100%", C["core"], "0.05"))
svg.append(_g("g_llm",  "0%", C["llm"],  "0.22", "100%", C["llm"],  "0.05"))
svg.append(_g("g_mem",  "0%", C["mem"],  "0.22", "100%", C["mem"],  "0.05"))
svg.append(_g("g_tool", "0%", C["tools"],"0.22", "100%", C["tools"],"0.05"))
svg.append(_g("g_pool", "0%", C["pool"], "0.22", "100%", C["pool"], "0.05"))
svg.append(_g("g_orch", "0%", C["orch"], "0.22", "100%", C["orch"], "0.05"))
svg.append('<filter id="gs" x="-40%" y="-40%" width="180%" height="180%">'
           '<feGaussianBlur stdDeviation="5" result="b"/><feMerge><feMergeNode in="b"/>'
           '<feMergeNode in="SourceGraphic"/></feMerge></filter>')
svg.append('<filter id="gl" x="-60%" y="-60%" width="220%" height="220%">'
           '<feGaussianBlur stdDeviation="10" result="b"/>'
           '<feComponentTransfer in="b" result="g"><feFuncA type="linear" slope="0.22"/></feComponentTransfer>'
           '<feMerge><feMergeNode in="g"/><feMergeNode in="SourceGraphic"/></feMerge></filter>')
for name, col in [("c", C["core"]), ("m", C["llm"]), ("a", C["mem"]), ("t", C["tools"]),
                  ("p", C["pool"]), ("o", C["orch"]), ("s", C["infra"]), ("e", C["evt"]),
                  ("l", C["cfg"]), ("r", C["sec"]), ("k", C["a2a"]), ("g", C["guard"]),
                  ("b", C["obs"]), ("d", C["dbg"]), ("n", C["adm"])]:
    svg.append(f'<marker id="a_{name}" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6" markerHeight="6" orient="auto">'
               f'<path d="M0 1 L9 5 L0 9z" fill="{col}" opacity="0.65"/></marker>')
svg.append('<pattern id="gp" width="42" height="42" patternUnits="userSpaceOnUse">'
           '<path d="M42 0L0 0 0 42" fill="none" stroke="#cbd5e1" stroke-width="0.4" opacity="0.35"/></pattern>')
svg.append("</defs>")

svg.append(f'<g transform="scale({SCALE})">')

svg.append(f'<rect width="{W}" height="{H}" fill="url(#bgR)"/>')
svg.append(f'<rect width="{W}" height="{H}" fill="url(#gp)"/>')

# ---------- 标题区 ----------
svg.append(title(1200, 52, "AGENTPRIMORDIA", fs=38, weight=200, color="#0f172a"))
svg.append('<text x="1200" y="84" text-anchor="middle" fill="#64748b" font-family="system-ui,sans-serif" '
           'font-size="13" letter-spacing="5" opacity="0.8">UNIVERSAL AI AGENT FRAMEWORK · GO + TYPESCRIPT DUAL SDK</text>')
svg.append('<line x1="860" y1="100" x2="1540" y2="100" stroke="#00d4ff" stroke-width="0.5" opacity="0.3"/>')
svg.append(sub(1200, 122, "v4.0.0  ·  34 Modules Parity  ·  Apache-2.0  ·  Zero CGO", fs=11, color="#00d4ff"))

# ============ ① 入口层 ============
p1 = panel(60, 150, 2280, 100, "①  APPLICATION / ENTRYPOINTS", C["infra"], label_x=80)
svg.append(p1)
entries = [("Your App", "Go / TS 集成"), ("CLI · ap", "init / run / debug"), ("Studio UI", "可视化编排"),
           ("VSCode", "Inspector 扩展"), ("Edge / Browser", "WASM 沙箱")]
ex = 90
for name, desc in entries:
    svg.append(box(ex, 168, 420, 64, "#94a3b8", "#94a3b8", rx=12, sw=0.9, fill_op=0.05))
    svg.append(title(ex + 210, 193, name, fs=14, color="#0f172a"))
    svg.append(sub(ex + 210, 215, desc, fs=9.5, color="#64748b"))
    ex += 455
svg.append(arrow(1200, 250, 1200, 292, C["infra"], dash="4 4", marker="a_s", opacity=0.5))

# ============ ② 公共 API 层 ============
p2 = panel(60, 300, 2280, 100, "②  PUBLIC API", C["cfg"], label_x=80)
svg.append(p2)
# Go pkg
svg.append(box(90, 318, 1080, 64, "#84cc16", "#84cc16", rx=12, sw=0.9, fill_op=0.05))
svg.append(title(340, 343, "Go  ·  pkg/", fs=14, color="#0f172a"))
svg.append(sub(340, 367, "ap.NewAgent() · Chain API · Type Aliases", fs=9.5, color="#84cc16"))
svg.append(chip(90, 334, 150, 32, "#84cc16", "public API", fs=11))
# TS SDK
svg.append(box(1230, 318, 1080, 64, "#84cc16", "#84cc16", rx=12, sw=0.9, fill_op=0.05))
svg.append(title(1480, 343, "TypeScript  ·  @agentprimordia/sdk", fs=14, color="#0f172a"))
svg.append(sub(1480, 367, "34 模块 Go Parity · npm install @agentprimordia/sdk", fs=9.5, color="#84cc16"))
svg.append(chip(1230, 334, 150, 32, "#84cc16", "npm SDK", fs=11))
svg.append(arrow(1200, 400, 1200, 442, C["cfg"], dash="4 4", marker="a_l", opacity=0.5))

# ============ ③ 核心引擎 ============
p3 = panel(60, 450, 2280, 210, "③  CORE ENGINE", C["core"], label_x=80)
svg.append(p3)
# 引擎大框
svg.append(f'<rect x="170" y="470" width="2060" height="172" rx="20" fill="url(#g_core)" stroke="{C["core"]}" stroke-opacity="0.55" stroke-width="1.6"/>')
svg.append(title(1200, 505, "ReAct Loop Engine  ·  Go + TS 双实现", fs=20, weight=600, color="#0f172a"))
svg.append(sub(1200, 528, "Reason → Act → Observe · 20+ Lifecycle Hooks · Planning · Reflection · Tool Learning", fs=11, color="#00d4ff"))
# 引擎内部流程（靠左三个紧凑框）
flow = [("Reason", "推理", C["llm"]), ("Act", "工具决策", C["tools"]), ("Observe", "结果回读", C["mem"])]
fx = 300
for i, (name, desc, col) in enumerate(flow):
    svg.append(box(fx, 550, 170, 60, col, col, rx=10, sw=1, fill_op=0.1))
    svg.append(title(fx + 85, 575, name, fs=16, weight=600, color="#0f172a"))
    svg.append(sub(fx + 85, 597, desc, fs=9.5, color=col))
    if i < 2:
        svg.append(arrow(fx + 170, 580, fx + 215, 580, C["core"], marker="a_c", opacity=0.6))
    fx += 245
# 引擎子能力 chips（靠右，避免重叠）
svg.append(chip(1020, 550, 210, 34, "#00d4ff", "Planning", fs=12))
svg.append(chip(1250, 550, 220, 34, "#00d4ff", "Reflection", fs=12))
svg.append(chip(1490, 550, 250, 34, "#00d4ff", "Tool Learning", fs=12))
svg.append(chip(1760, 550, 220, 34, "#00d4ff", "20+ Hooks", fs=12))
# 引擎 → 能力四件套 连线（对准各卡片中心）
svg.append(arrow(354, 660, 354, 712, C["core"], marker="a_c", opacity=0.55))
svg.append(arrow(894, 660, 894, 712, C["core"], marker="a_c", opacity=0.55))
svg.append(arrow(1434, 660, 1434, 712, C["core"], marker="a_c", opacity=0.55))
svg.append(arrow(1974, 660, 1974, 712, C["core"], marker="a_c", opacity=0.55))

# ============ ④ 能力四件套 ============
p4 = panel(60, 720, 2280, 240, "④  CORE CAPABILITIES", C["core"], label_x=80)
svg.append(p4)
cards = [
    (90,  "LLM Abstraction", "12+ Providers · Resilient", C["llm"], "g_llm",
     ["OpenAI", "Anthropic", "Gemini", "DeepSeek", "Ollama", "Qwen"], "Retry · Fallback · CircuitBreak"),
    (630, "Tool System", "Builtin + MCP + Plugin", C["tools"], "g_tool",
     ["FileSystem", "Shell", "Web", "MCP", "WASM", "Plugin"], "Registry · Executor · Scope"),
    (1170,"Memory Store", "三层混合检索", C["mem"], "g_mem",
     ["SQLite FTS5", "Vector HNSW", "RAG", "RRF 融合", "Summarize", "Episodic"], "Importance · Archive · Compress"),
    (1710,"Agent Pool", "并发调度 · 会话隔离", C["pool"], "g_pool",
     ["Semaphore", "Session", "Retry", "Autoscale", "Events", "Tenant"], "Graceful Shutdown · Dispatcher"),
]
for x, t, s, col, gid, chips_, footer in cards:
    svg.append(f'<rect x="{x-6}" y="{734-6}" width="{528+12}" height="{212+12}" rx="18" fill="{col}" opacity="0.05" filter="url(#gs)"/>')
    svg.append(f'<rect x="{x}" y="{734}" width="528" height="212" rx="14" fill="url(#{gid})" stroke="{col}" stroke-opacity="0.5" stroke-width="1.3"/>')
    svg.append(title(x + 264, 770, t, fs=17, weight=600, color="#0f172a"))
    svg.append(sub(x + 264, 794, s, fs=10.5, color=col))
    # 两行 3+3 chips，卡片可用宽 528-44=484，3*140+2*12=444 < 484
    for row in range(2):
        cxx = x + 22
        for cc in chips_[row * 3:(row + 1) * 3]:
            svg.append(chip(cxx, 818 + row * 46, 140, 30, col, cc, fs=10.5))
            cxx += 152
    svg.append(sub(x + 264, 930, footer, fs=9, color=col, )) 

# ============ ⑤ 编排层 ============
p5 = panel(60, 980, 2280, 120, "⑤  ORCHESTRATION", C["orch"], label_x=80)
svg.append(p5)
svg.append(box(90, 1000, 2220, 82, "#a78bfa", "#a78bfa", rx=14, sw=1, fill_op=0.05))
modes = ["Pipeline", "Handoff", "Parallel", "DAG", "GroupChat", "A2A", "Swarm", "MapReduce"]
mx = 160
for m in modes:
    svg.append(chip(mx, 1025, 210, 34, "#a78bfa", m, fs=12))
    mx += 245
svg.append(sub(1200, 1074, "Collaboration · Debate · Worker Pool · Planner · Multi-Agent 分工", fs=9.5, color="#a78bfa"))

# ============ ⑥ 横向支撑 ============
p6 = panel(60, 1120, 2280, 200, "⑥  CROSS-CUTTING SUPPORT", C["obs"], label_x=80)
svg.append(p6)
support = [
    ("Security / Guardrail", C["sec"], "ACL · Sandbox · PII · Guardrails"),
    ("Governance", C["guard"], "Tenant · Quota · Policy"),
    ("Audit", C["adm"], "合规审计 · 报告生成"),
    ("Observability", C["obs"], "Metrics · OTel · SLO/SLI"),
    ("Resilience", C["cfg"], "重试 · 降级 · 熔断"),
    ("Debugger", C["dbg"], "Inspector · 时间旅行"),
    ("Admin API", C["adm"], "管理 HTTP API"),
    ("Health", C["evt"], "healthz / readyz / livez"),
    ("Config", C["cfg"], "热加载 · Feature"),
    ("Event Bus", C["evt"], "Pub / Sub · 事件流"),
]
sx, sy = 90, 1142
for i, (t, col, s) in enumerate(support):
    col_i = i % 2
    x = sx + (i % 5) * 455
    y = sy + (i // 5) * 82
    svg.append(box(x, y, 430, 66, col, col, rx=10, sw=0.9, fill_op=0.06))
    svg.append(title(x + 215, y + 26, t, fs=13, weight=600, color="#0f172a"))
    svg.append(sub(x + 215, y + 47, s, fs=9, color=col))

# ============ ⑦ 基础设施 ============
p7 = panel(60, 1340, 2280, 90, "⑦  INFRASTRUCTURE", C["infra"], label_x=80)
svg.append(p7)
infra = ["Checkpoint (etcd/Redis)", "K8s Operator", "WASM (wazero)", "Chaos Engineering", "Eval / Bench", "Registry · Marketplace"]
ix = 90
for t in infra:
    svg.append(chip(ix, 1358, 350, 38, "#94a3b8", t, fs=11.5, opacity=0.08))
    ix += 380

# ============ 图例 ============
legend_y = 1450
svg.append(sub(1200, legend_y, "Go: agentprimordia/ · TS: sdk/typescript/ · 双语言功能对等，34 模块全覆盖", fs=10, color="#64748b"))

svg.append("</g>")
svg.append("</svg>")

svg_text = "\n".join(svg)
repo = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
svg_path = os.path.join(repo, "agentprimordia", "docs", "ap-architecture.svg")
png_path = os.path.join(repo, "agentprimordia", "docs", "ap-architecture.png")
os.makedirs(os.path.dirname(svg_path), exist_ok=True)

with open(svg_path, "w", encoding="utf-8") as f:
    f.write(svg_text)
print(f"SVG 写入: {svg_path}")

# ---------- 转 PNG ----------
def try_edge():
    html_path = os.path.join(repo, "agentprimordia", "docs", "_arch_tmp.html")
    with open(html_path, "w", encoding="utf-8") as f:
        f.write('<!DOCTYPE html><html><head><meta charset="utf-8"></head>'
                '<body style="margin:0;padding:0;background:#ffffff">'
                + svg_text + '</body></html>')
    edge_paths = [r"C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe",
                  r"C:\Program Files\Microsoft\Edge\Application\msedge.exe"]
    edge = next((p for p in edge_paths if os.path.exists(p)), None)
    if not edge:
        return False
    cmd = [edge, "--headless=new", "--disable-gpu", "--no-sandbox",
           f"--screenshot={png_path}", f"--window-size={W},{H}", "--hide-scrollbars",
           "file:///" + html_path.replace(os.sep, "/")]
    r = subprocess.run(cmd, capture_output=True, text=True, timeout=60)
    if os.path.exists(png_path) and os.path.getsize(png_path) > 0:
        os.remove(html_path)
        return True
    return False

def try_cairosvg():
    try:
        import cairosvg
        cairosvg.svg2png(bytestring=svg_text.encode("utf-8"), write_to=png_path,
                         output_width=W, output_height=H)
        return True
    except Exception:
        return False

if try_cairosvg():
    print(f"PNG 生成成功 (cairosvg): {png_path} ({os.path.getsize(png_path)} bytes)")
    sys.exit(0)
if try_edge():
    print(f"PNG 生成成功 (Edge): {png_path} ({os.path.getsize(png_path)} bytes)")
    sys.exit(0)
print("PNG 生成失败")
sys.exit(1)

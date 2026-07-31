"""SVG 转 PNG 工具脚本"""
import subprocess
import os
import sys

SVG_PATH = r'e:\ap\AgentPrimordia\agentprimordia\docs\ap-architecture.svg'
PNG_PATH = r'e:\ap\AgentPrimordia\docs\images\ppt\architecture_hd.png'
HTML_PATH = r'e:\ap\AgentPrimordia\docs\images\ppt\_svg_convert.html'

def try_cairosvg():
    """尝试用 cairosvg 转换"""
    try:
        import cairosvg
        cairosvg.svg2png(url=SVG_PATH, write_to=PNG_PATH, output_width=2800, output_height=3400)
        print("cairosvg 转换成功")
        return True
    except Exception as e:
        print(f"cairosvg 失败: {e}")
        return False

def try_svglib():
    """尝试用 svglib + reportlab 转换"""
    try:
        from svglib.svglib import svg2rlg
        from reportlab.graphics import renderPM
        drawing = svg2rlg(SVG_PATH)
        scale_x = 2800 / drawing.width
        scale_y = 3400 / drawing.height
        drawing.width = 2800
        drawing.height = 3400
        drawing.scale(scale_x, scale_y)
        renderPM.drawToFile(drawing, PNG_PATH, fmt='PNG')
        print("svglib 转换成功")
        return True
    except Exception as e:
        print(f"svglib 失败: {e}")
        return False

def try_edge_headless():
    """用 Edge 无头模式截图"""
    # 创建包装 HTML
    with open(SVG_PATH, 'r', encoding='utf-8') as f:
        svg_content = f.read()
    html = '<!DOCTYPE html><html><head><meta charset="utf-8"></head>'
    html += '<body style="margin:0;padding:0;background:transparent">'
    html += svg_content
    html += '</body></html>'
    with open(HTML_PATH, 'w', encoding='utf-8') as f:
        f.write(html)
    
    edge_paths = [
        r'C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe',
        r'C:\Program Files\Microsoft\Edge\Application\msedge.exe',
    ]
    edge_exe = None
    for p in edge_paths:
        if os.path.exists(p):
            edge_exe = p
            break
    if not edge_exe:
        print("Edge 未找到")
        return False
    
    # 使用 --screenshot 参数
    cmd = [
        edge_exe,
        '--headless=new',
        '--disable-gpu',
        '--no-sandbox',
        f'--screenshot={PNG_PATH}',
        '--window-size=2800,3400',
        '--hide-scrollbars',
        f'file:///{HTML_PATH.replace(os.sep, "/")}'
    ]
    result = subprocess.run(cmd, capture_output=True, text=True, timeout=30)
    if os.path.exists(PNG_PATH) and os.path.getsize(PNG_PATH) > 0:
        print(f"Edge 截图成功: {os.path.getsize(PNG_PATH)} bytes")
        # 清理临时 HTML
        try:
            os.remove(HTML_PATH)
        except:
            pass
        return True
    else:
        print(f"Edge 截图失败: {result.stderr[:200]}")
        return False

def try_pillow_svg():
    """尝试用 Pillow 直接读取 SVG (需要 Pillow >= 10.x 的 SVG 支持)"""
    try:
        from PIL import Image
        img = Image.open(SVG_PATH)
        img = img.resize((2800, 3400), Image.LANCZOS)
        img.save(PNG_PATH, 'PNG')
        print("Pillow SVG 转换成功")
        return True
    except Exception as e:
        print(f"Pillow SVG 失败: {e}")
        return False

if __name__ == '__main__':
    os.makedirs(os.path.dirname(PNG_PATH), exist_ok=True)
    
    # 按优先级尝试各方案
    for fn in [try_cairosvg, try_svglib, try_edge_headless, try_pillow_svg]:
        if fn():
            print(f"输出: {PNG_PATH}")
            sys.exit(0)
    
    print("所有方案均失败")
    sys.exit(1)

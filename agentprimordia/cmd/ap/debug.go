package main

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
)

func runDebug(args []string) {
	var port string

	i := 0
	for i < len(args) {
		switch args[i] {
		case "--port", "-p":
			i++
			if i >= len(args) {
				fmt.Fprintln(os.Stderr, "错误: --port 需要指定端口号")
				os.Exit(1)
			}
			port = args[i]
		case "--help", "-h":
			fmt.Print(`ap debug — 启动调试服务器

用法:
  ap debug [--port 8080]

选项:
  --port, -p   调试服务器端口 (默认: 6060)

说明:
  启动 HTTP 调试服务器，在浏览器中查看 Agent 实时推理链、
  工具调用、记忆搜索和性能指标。

示例:
  ap debug
  ap debug --port 3000
`)
			return
		}
		i++
	}
	if port == "" {
		port = "6060"
	}

	dir, err := findProjectDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}

	// 启动调试服务器
	mux := http.NewServeMux()

	// 首页：调试面板
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, debugHTML)
	})

	// API: 项目信息
	mux.HandleFunc("/api/project", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"name": %q, "dir": %q}`, filepath.Base(dir), dir)
	})

	// API: Go 环境信息
	mux.HandleFunc("/api/env", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		goVersion, _ := exec.Command("go", "version").CombinedOutput()
		fmt.Fprintf(w, `{"go_version": %q}`, string(goVersion))
	})

	// API: 文件列表
	mux.HandleFunc("/api/files", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"files": []}`)
	})

	addr := ":" + port
	fmt.Printf("调试服务器启动: http://localhost%s\n", addr)
	fmt.Println("按 Ctrl+C 停止")
	if err := http.ListenAndServe(addr, mux); err != nil {
		fmt.Fprintf(os.Stderr, "服务器启动失败: %v\n", err)
		os.Exit(1)
	}
}

const debugHTML = `<!DOCTYPE html>
<html lang="zh">
<head>
<meta charset="UTF-8">
<title>AgentPrimordia Debug</title>
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body { font-family: 'Menlo', 'Consolas', monospace; background: #1a1a2e; color: #e0e0e0; }
  .header { background: #16213e; padding: 12px 20px; border-bottom: 1px solid #0f3460; }
  .header h1 { font-size: 16px; color: #00d2ff; }
  .container { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; padding: 12px; height: calc(100vh - 52px); }
  .panel { background: #16213e; border-radius: 8px; padding: 12px; overflow: auto; }
  .panel h2 { font-size: 13px; color: #00d2ff; margin-bottom: 8px; text-transform: uppercase; }
  .trace { border-left: 2px solid #0f3460; padding-left: 12px; }
  .trace .turn { margin-bottom: 8px; }
  .trace .label { font-size: 11px; color: #888; }
  .trace .content { font-size: 12px; padding: 4px 0; }
  .trace .tool-call { background: #0f3460; padding: 4px 8px; border-radius: 4px; margin: 4px 0; }
  .status-bar { position: fixed; bottom: 0; left: 0; right: 0; background: #0f3460; padding: 6px 20px; font-size: 11px; }
</style>
</head>
<body>
<div class="header">
  <h1>AP Debug Console</h1>
</div>
<div class="container">
  <div class="panel">
    <h2>Agent 推理链</h2>
    <div class="trace" id="trace">
      <div class="turn">
        <div class="label">等待 Agent 运行...</div>
        <div class="content" style="color:#666">使用 ap run 启动 Agent 后，推理过程将在此实时显示</div>
      </div>
    </div>
  </div>
  <div class="panel">
    <h2>工具调用</h2>
    <div id="tools" style="color:#666; font-size:12px">暂无工具调用记录</div>
  </div>
  <div class="panel">
    <h2>记忆搜索</h2>
    <div style="margin-bottom:8px">
      <input id="search-input" placeholder="搜索记忆..." style="width:100%;padding:6px;background:#1a1a2e;border:1px solid #0f3460;color:#e0e0e0;border-radius:4px;">
    </div>
    <div id="memory" style="color:#666; font-size:12px">暂无记忆数据</div>
  </div>
  <div class="panel">
    <h2>性能指标</h2>
    <div id="metrics" style="color:#666; font-size:12px">暂无指标数据</div>
  </div>
</div>
<div class="status-bar">
  <span id="status">就绪</span>
</div>
<script>
  // 连接 SSE 事件流
  const evtSource = new EventSource('/api/events');
  evtSource.onmessage = function(e) {
    const data = JSON.parse(e.data);
    document.getElementById('status').textContent = data.status || '运行中';
  };
  evtSource.onerror = function() {
    document.getElementById('status').textContent = '未连接';
  };
</script>
</body>
</html>
`

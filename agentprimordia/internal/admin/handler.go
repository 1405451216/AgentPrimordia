package admin

import (
	"encoding/json"
	"html"
	"log/slog"
	"net/http"
	"runtime"
	"time"

	"agentprimordia/internal/pool"
)

type AdminHandler struct {
	pool   *pool.Pool
	mux    *http.ServeMux
	logger *slog.Logger
}

func NewAdminHandler(p *pool.Pool) *AdminHandler {
	h := &AdminHandler{
		pool:   p,
		mux:    http.NewServeMux(),
		logger: slog.Default(),
	}

	h.mux.HandleFunc("GET /api/agents", h.listAgents)
	h.mux.HandleFunc("GET /api/agents/{id}", h.getAgent)
	h.mux.HandleFunc("GET /api/stats", h.stats)
	h.mux.HandleFunc("GET /api/tasks", h.tasks)
	h.mux.HandleFunc("GET /api/health", h.health)
	h.mux.HandleFunc("GET /api/system", h.systemInfo)
	h.mux.HandleFunc("GET /", h.index)

	return h
}

func (h *AdminHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

func (h *AdminHandler) listAgents(w http.ResponseWriter, r *http.Request) {
	agents := h.pool.ListAgents()
	writeJSON(w, http.StatusOK, agents)
}

func (h *AdminHandler) getAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "缺少 agent ID"})
		return
	}

	agents := h.pool.ListAgents()
	if status, exists := agents[id]; exists {
		writeJSON(w, http.StatusOK, map[string]any{
			"id":     id,
			"status": status,
		})
		return
	}

	writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent 未找到"})
}

func (h *AdminHandler) stats(w http.ResponseWriter, r *http.Request) {
	stats := h.pool.Stats()
	writeJSON(w, http.StatusOK, stats)
}

func (h *AdminHandler) tasks(w http.ResponseWriter, r *http.Request) {
	tasks := h.pool.ListTasks()

	items := make([]map[string]any, 0, len(tasks))
	for _, t := range tasks {
		items = append(items, taskResultToJSON(t))
	}

	writeJSON(w, http.StatusOK, items)
}

func (h *AdminHandler) health(w http.ResponseWriter, r *http.Request) {
	stats := h.pool.Stats()
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "healthy",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"tasks":     stats.TotalTasks,
		"running":   stats.RunningTasks,
	})
}

func (h *AdminHandler) systemInfo(w http.ResponseWriter, r *http.Request) {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	writeJSON(w, http.StatusOK, map[string]any{
		"go_version":   runtime.Version(),
		"goroutines":   runtime.NumGoroutine(),
		"cpu_count":    runtime.NumCPU(),
		"mem_alloc_mb": float64(memStats.Alloc) / 1024 / 1024,
		"mem_sys_mb":   float64(memStats.Sys) / 1024 / 1024,
		"gc_count":     memStats.NumGC,
		"heap_objects": memStats.HeapObjects,
		"stack_use_mb": float64(memStats.StackInuse) / 1024 / 1024,
	})
}

func (h *AdminHandler) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "未找到"})
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(indexHTML))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// taskResultToJSON 将 TaskResult 转为可 JSON 序列化的 map
// 处理 error 和 time.Duration 的序列化问题
func taskResultToJSON(t pool.TaskResult) map[string]any {
	m := map[string]any{
		"task_id": t.TaskID,
		"task":    t.Task,
		"status":  t.Status,
	}
	if t.Error != nil {
		m["error"] = t.Error.Error()
	}
	if t.Duration > 0 {
		m["duration_ms"] = t.Duration.Milliseconds()
	}
	if t.Response != nil {
		m["response"] = t.Response
	}
	return m
}

// sanitizeHTML 对字符串进行 HTML 转义，防止 XSS
func sanitizeHTML(s string) string {
	return html.EscapeString(s)
}

const indexHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>AgentPrimordia 管理面板</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:system-ui,-apple-system,sans-serif;background:#0f172a;color:#e2e8f0;padding:24px}
h1{font-size:1.5rem;margin-bottom:16px;color:#38bdf8}
.stats{display:grid;grid-template-columns:repeat(auto-fit,minmax(140px,1fr));gap:12px;margin-bottom:24px}
.stat{background:#1e293b;border-radius:8px;padding:16px;text-align:center}
.stat .num{font-size:1.8rem;font-weight:700;color:#38bdf8}
.stat .lbl{font-size:.8rem;color:#94a3b8;margin-top:4px}
table{width:100%;border-collapse:collapse;background:#1e293b;border-radius:8px;overflow:hidden}
th,td{padding:10px 14px;text-align:left;border-bottom:1px solid #334155}
th{background:#0f172a;color:#94a3b8;font-size:.8rem;text-transform:uppercase}
td{font-size:.9rem}
.tag{display:inline-block;padding:2px 8px;border-radius:4px;font-size:.75rem;font-weight:600}
.tag-queued{background:#fbbf24;color:#1e293b}
.tag-running{background:#38bdf8;color:#1e293b}
.tag-completed{background:#4ade80;color:#1e293b}
.tag-failed{background:#f87171;color:#1e293b}
.tag-cancelled{background:#94a3b8;color:#1e293b}
.refresh{font-size:.75rem;color:#64748b;margin-bottom:12px}
</style>
</head>
<body>
<h1>AgentPrimordia 管理面板</h1>
<div class="refresh">每 5 秒自动刷新</div>
<div id="stats" class="stats"></div>
<table>
<thead><tr><th>ID</th><th>标题</th><th>状态</th><th>耗时(ms)</th><th>错误</th></tr></thead>
<tbody id="tasks"></tbody>
</table>
<script>
function tagClass(s){return 'tag tag-'+s}
function esc(s){if(s==null)return '';var d=document.createElement('div');d.textContent=s;return d.innerHTML}
function fetchAndUpdate(){
  Promise.all([
    fetch('/api/stats').then(r=>r.json()),
    fetch('/api/tasks').then(r=>r.json())
  ]).then(([stats,tasks])=>{
    document.getElementById('stats').innerHTML=
      '<div class="stat"><div class="num">'+stats.total_tasks+'</div><div class="lbl">总任务</div></div>'+
      '<div class="stat"><div class="num">'+stats.running_tasks+'</div><div class="lbl">运行中</div></div>'+
      '<div class="stat"><div class="num">'+stats.completed_tasks+'</div><div class="lbl">已完成</div></div>'+
      '<div class="stat"><div class="num">'+stats.failed_tasks+'</div><div class="lbl">失败</div></div>';
    var rows='';
    for(var i=0;i<tasks.length;i++){
      var t=tasks[i];
      var dur=t.duration_ms!=null?t.duration_ms:'-';
      var err=t.error||'';
      rows+='<tr><td>'+esc(t.task_id)+'</td><td>'+esc(t.task&&t.task.title||'-')+'</td>'+
        '<td><span class="'+tagClass(t.status)+'">'+esc(t.status)+'</span></td>'+
        '<td>'+dur+'</td><td>'+esc(err)+'</td></tr>';
    }
    document.getElementById('tasks').innerHTML=rows;
  }).catch(()=>{});
}
fetchAndUpdate();
setInterval(fetchAndUpdate,5000);
</script>
</body>
</html>`

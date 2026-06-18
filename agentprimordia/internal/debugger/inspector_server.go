package debugger

import (
	"encoding/json"
	"net/http"
)

// InspectorServer 提供Inspector的HTTP API和Web UI
type InspectorServer struct {
	inspector *Inspector
	mux       *http.ServeMux
}

// NewInspectorServer 创建Inspector HTTP服务器
func NewInspectorServer(inspector *Inspector) *InspectorServer {
	s := &InspectorServer{
		inspector: inspector,
		mux:       http.NewServeMux(),
	}

	s.mux.HandleFunc("/inspector", s.handleInspectorUI)
	s.mux.HandleFunc("/api/inspector/traces", s.handleGetTraces)
	s.mux.HandleFunc("/api/inspector/sessions", s.handleGetSessions)
	s.mux.HandleFunc("/api/inspector/session/", s.handleGetSessionTrace)
	s.mux.HandleFunc("/api/inspector/stats", s.handleGetStats)

	return s
}

// Handler 返回HTTP处理器
func (s *InspectorServer) Handler() http.Handler {
	return s.mux
}

// handleInspectorUI 提供Web UI界面
func (s *InspectorServer) handleInspectorUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(inspectorHTML))
}

// handleGetTraces 获取所有追踪数据
func (s *InspectorServer) handleGetTraces(w http.ResponseWriter, r *http.Request) {
	traces := s.inspector.GetTraces()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(traces)
}

// handleGetSessions 获取所有会话
func (s *InspectorServer) handleGetSessions(w http.ResponseWriter, r *http.Request) {
	sessions := s.inspector.GetAllSessions()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(sessions)
}

// handleGetSessionTrace 获取指定会话的追踪
func (s *InspectorServer) handleGetSessionTrace(w http.ResponseWriter, r *http.Request) {
	// 从URL路径中提取session ID
	sessionID := r.URL.Path[len("/api/inspector/session/"):]
	if sessionID == "" {
		http.Error(w, "session ID required", http.StatusBadRequest)
		return
	}

	session := s.inspector.GetSessionTrace(sessionID)
	if session == nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(session)
}

// handleGetStats 获取统计信息
func (s *InspectorServer) handleGetStats(w http.ResponseWriter, r *http.Request) {
	stats := s.inspector.GetStats()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(stats)
}

const inspectorHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>AP Inspector - Agent Trace Viewer</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
            background: #f5f7fa;
            color: #2c3e50;
        }
        
        .header {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            padding: 20px 40px;
            box-shadow: 0 2px 10px rgba(0,0,0,0.1);
        }
        
        .header h1 {
            font-size: 28px;
            font-weight: 600;
        }
        
        .header p {
            margin-top: 5px;
            opacity: 0.9;
            font-size: 14px;
        }
        
        .container {
            max-width: 1400px;
            margin: 0 auto;
            padding: 30px 40px;
        }
        
        .stats-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 20px;
            margin-bottom: 30px;
        }
        
        .stat-card {
            background: white;
            padding: 20px;
            border-radius: 12px;
            box-shadow: 0 2px 8px rgba(0,0,0,0.08);
            transition: transform 0.2s;
        }
        
        .stat-card:hover {
            transform: translateY(-2px);
            box-shadow: 0 4px 12px rgba(0,0,0,0.12);
        }
        
        .stat-label {
            font-size: 13px;
            color: #7f8c8d;
            text-transform: uppercase;
            letter-spacing: 0.5px;
            margin-bottom: 8px;
        }
        
        .stat-value {
            font-size: 32px;
            font-weight: 700;
            color: #2c3e50;
        }
        
        .stat-value.primary { color: #667eea; }
        .stat-value.success { color: #27ae60; }
        .stat-value.warning { color: #f39c12; }
        .stat-value.danger { color: #e74c3c; }
        
        .tabs {
            display: flex;
            gap: 10px;
            margin-bottom: 20px;
            border-bottom: 2px solid #e0e0e0;
        }
        
        .tab {
            padding: 12px 24px;
            background: none;
            border: none;
            color: #7f8c8d;
            font-size: 15px;
            font-weight: 500;
            cursor: pointer;
            border-bottom: 3px solid transparent;
            transition: all 0.2s;
        }
        
        .tab:hover {
            color: #667eea;
        }
        
        .tab.active {
            color: #667eea;
            border-bottom-color: #667eea;
        }
        
        .tab-content {
            display: none;
        }
        
        .tab-content.active {
            display: block;
        }
        
        .trace-list {
            background: white;
            border-radius: 12px;
            box-shadow: 0 2px 8px rgba(0,0,0,0.08);
            overflow: hidden;
        }
        
        .trace-item {
            padding: 16px 20px;
            border-bottom: 1px solid #ecf0f1;
            cursor: pointer;
            transition: background 0.2s;
        }
        
        .trace-item:hover {
            background: #f8f9fa;
        }
        
        .trace-item:last-child {
            border-bottom: none;
        }
        
        .trace-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 8px;
        }
        
        .trace-name {
            font-weight: 600;
            font-size: 15px;
            color: #2c3e50;
        }
        
        .trace-kind {
            display: inline-block;
            padding: 4px 12px;
            border-radius: 12px;
            font-size: 12px;
            font-weight: 500;
            text-transform: uppercase;
        }
        
        .trace-kind.agent { background: #e3f2fd; color: #1976d2; }
        .trace-kind.llm { background: #f3e5f5; color: #7b1fa2; }
        .trace-kind.tool { background: #e8f5e9; color: #388e3c; }
        .trace-kind.memory { background: #fff3e0; color: #f57c00; }
        
        .trace-meta {
            display: flex;
            gap: 20px;
            font-size: 13px;
            color: #7f8c8d;
        }
        
        .trace-meta-item {
            display: flex;
            align-items: center;
            gap: 5px;
        }
        
        .status-badge {
            display: inline-block;
            padding: 3px 10px;
            border-radius: 10px;
            font-size: 11px;
            font-weight: 600;
            text-transform: uppercase;
        }
        
        .status-badge.completed { background: #d4edda; color: #155724; }
        .status-badge.failed { background: #f8d7da; color: #721c24; }
        .status-badge.started { background: #fff3cd; color: #856404; }
        
        .session-list {
            background: white;
            border-radius: 12px;
            box-shadow: 0 2px 8px rgba(0,0,0,0.08);
            overflow: hidden;
        }
        
        .session-item {
            padding: 16px 20px;
            border-bottom: 1px solid #ecf0f1;
            cursor: pointer;
            transition: background 0.2s;
        }
        
        .session-item:hover {
            background: #f8f9fa;
        }
        
        .session-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 8px;
        }
        
        .session-id {
            font-weight: 600;
            font-size: 15px;
            color: #2c3e50;
            font-family: 'Courier New', monospace;
        }
        
        .session-meta {
            display: flex;
            gap: 20px;
            font-size: 13px;
            color: #7f8c8d;
        }
        
        .empty-state {
            text-align: center;
            padding: 60px 20px;
            color: #95a5a6;
        }
        
        .empty-state-icon {
            font-size: 64px;
            margin-bottom: 20px;
            opacity: 0.3;
        }
        
        .loading {
            text-align: center;
            padding: 40px;
            color: #95a5a6;
        }
        
        .spinner {
            border: 3px solid #f3f3f3;
            border-top: 3px solid #667eea;
            border-radius: 50%;
            width: 40px;
            height: 40px;
            animation: spin 1s linear infinite;
            margin: 0 auto 20px;
        }
        
        @keyframes spin {
            0% { transform: rotate(0deg); }
            100% { transform: rotate(360deg); }
        }
        
        .refresh-btn {
            position: fixed;
            bottom: 30px;
            right: 30px;
            background: #667eea;
            color: white;
            border: none;
            padding: 15px 25px;
            border-radius: 50px;
            font-size: 14px;
            font-weight: 600;
            cursor: pointer;
            box-shadow: 0 4px 12px rgba(102, 126, 234, 0.4);
            transition: all 0.2s;
        }
        
        .refresh-btn:hover {
            background: #5568d3;
            transform: translateY(-2px);
            box-shadow: 0 6px 16px rgba(102, 126, 234, 0.5);
        }
    </style>
</head>
<body>
    <div class="header">
        <h1>🔍 AP Inspector</h1>
        <p>Agent Trace Viewer - 实时监控和调试Agent执行</p>
    </div>
    
    <div class="container">
        <div class="stats-grid" id="stats-grid">
            <div class="loading">
                <div class="spinner"></div>
                <p>加载统计数据...</p>
            </div>
        </div>
        
        <div class="tabs">
            <button class="tab active" onclick="switchTab('traces')">追踪列表</button>
            <button class="tab" onclick="switchTab('sessions')">会话列表</button>
        </div>
        
        <div id="traces-tab" class="tab-content active">
            <div id="traces-container" class="trace-list">
                <div class="loading">
                    <div class="spinner"></div>
                    <p>加载追踪数据...</p>
                </div>
            </div>
        </div>
        
        <div id="sessions-tab" class="tab-content">
            <div id="sessions-container" class="session-list">
                <div class="loading">
                    <div class="spinner"></div>
                    <p>加载会话数据...</p>
                </div>
            </div>
        </div>
    </div>
    
    <button class="refresh-btn" onclick="refreshData()">🔄 刷新数据</button>
    
    <script>
        function switchTab(tabName) {
            // 更新tab按钮状态
            document.querySelectorAll('.tab').forEach(tab => {
                tab.classList.remove('active');
            });
            event.target.classList.add('active');
            
            // 更新tab内容
            document.querySelectorAll('.tab-content').forEach(content => {
                content.classList.remove('active');
            });
            document.getElementById(tabName + '-tab').classList.add('active');
        }
        
        async function loadStats() {
            try {
                const response = await fetch('/api/inspector/stats');
                const stats = await response.json();
                
                const statsGrid = document.getElementById('stats-grid');
                statsGrid.innerHTML = '<div class="stat-card">' +
                    '<div class="stat-label">总追踪数</div>' +
                    '<div class="stat-value primary">' + (stats.total_spans || 0) + '</div>' +
                    '</div>' +
                    '<div class="stat-card">' +
                    '<div class="stat-label">会话数</div>' +
                    '<div class="stat-value">' + (stats.total_sessions || 0) + '</div>' +
                    '</div>' +
                    '<div class="stat-card">' +
                    '<div class="stat-label">总Token数</div>' +
                    '<div class="stat-value warning">' + (stats.total_tokens || 0) + '</div>' +
                    '</div>' +
                    '<div class="stat-card">' +
                    '<div class="stat-label">成功</div>' +
                    '<div class="stat-value success">' + ((stats.span_by_status && stats.span_by_status.completed) || 0) + '</div>' +
                    '</div>' +
                    '<div class="stat-card">' +
                    '<div class="stat-label">失败</div>' +
                    '<div class="stat-value danger">' + ((stats.span_by_status && stats.span_by_status.failed) || 0) + '</div>' +
                    '</div>';
            } catch (error) {
                console.error('加载统计数据失败:', error);
            }
        }
        
        async function loadTraces() {
            try {
                const response = await fetch('/api/inspector/traces');
                const traces = await response.json();
                
                const container = document.getElementById('traces-container');
                
                if (!traces || traces.length === 0) {
                    container.innerHTML = '<div class="empty-state">' +
                        '<div class="empty-state-icon">📭</div>' +
                        '<p>暂无追踪数据</p>' +
                        '<p style="font-size: 13px; margin-top: 10px;">运行Agent后，追踪数据将显示在这里</p>' +
                        '</div>';
                    return;
                }
                
                container.innerHTML = traces.map(trace => {
                    const duration = trace.duration ? (trace.duration / 1000000).toFixed(2) + 'ms' : '-';
                    const tokens = trace.total_tokens || 0;
                    
                    return '<div class="trace-item">' +
                        '<div class="trace-header">' +
                        '<span class="trace-name">' + trace.name + '</span>' +
                        '<span class="trace-kind ' + trace.kind + '">' + trace.kind + '</span>' +
                        '</div>' +
                        '<div class="trace-meta">' +
                        '<div class="trace-meta-item">' +
                        '<span class="status-badge ' + trace.status + '">' + trace.status + '</span>' +
                        '</div>' +
                        '<div class="trace-meta-item">⏱️ ' + duration + '</div>' +
                        '<div class="trace-meta-item">🎯 ' + tokens + ' tokens</div>' +
                        '<div class="trace-meta-item">📅 ' + new Date(trace.start_time).toLocaleString('zh-CN') + '</div>' +
                        '</div>' +
                        '</div>';
                }).join('');
            } catch (error) {
                console.error('加载追踪数据失败:', error);
            }
        }
        
        async function loadSessions() {
            try {
                const response = await fetch('/api/inspector/sessions');
                const sessions = await response.json();
                
                const container = document.getElementById('sessions-container');
                
                if (!sessions || sessions.length === 0) {
                    container.innerHTML = '<div class="empty-state">' +
                        '<div class="empty-state-icon">📭</div>' +
                        '<p>暂无会话数据</p>' +
                        '<p style="font-size: 13px; margin-top: 10px;">运行Agent后，会话数据将显示在这里</p>' +
                        '</div>';
                    return;
                }
                
                container.innerHTML = sessions.map(session => {
                    const spanCount = session.spans ? session.spans.length : 0;
                    const startTime = new Date(session.start_time).toLocaleString('zh-CN');
                    
                    return '<div class="session-item" onclick="viewSession(\'' + session.session_id + '\')">' +
                        '<div class="session-header">' +
                        '<span class="session-id">' + session.session_id + '</span>' +
                        '<span class="status-badge completed">' + spanCount + ' spans</span>' +
                        '</div>' +
                        '<div class="session-meta">' +
                        '<div>📅 ' + startTime + '</div>' +
                        '<div>🎯 ' + (session.total_turns || 0) + ' turns</div>' +
                        '<div>💰 $' + (session.total_cost || 0).toFixed(4) + '</div>' +
                        '</div>' +
                        '</div>';
                }).join('');
            } catch (error) {
                console.error('加载会话数据失败:', error);
            }
        }
        
        function viewSession(sessionId) {
            alert('会话详情功能开发中: ' + sessionId);
        }
        
        function refreshData() {
            loadStats();
            loadTraces();
            loadSessions();
        }
        
        // 初始加载
        refreshData();
        
        // 自动刷新（每5秒）
        setInterval(refreshData, 5000);
    </script>
</body>
</html>`

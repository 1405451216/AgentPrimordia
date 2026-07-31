package debugger

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"agentprimordia/internal/agent"
	"agentprimordia/internal/orchestration"
)

// 默认执行超时时间
const defaultExecutionTimeout = 5 * time.Minute

// VisualEditor 可视化编排编辑器
type VisualEditor struct {
	mu            sync.RWMutex
	orchestrators map[string]*orchestration.Orchestrator
	configs       map[string]*EditorConfig
	executions    map[string]*ExecutionRecord
	agents        map[string]agent.Agent // 已注册的 Agent 实例（按节点 ID）
}

// EditorConfig 编辑器配置（可序列化的编排配置）
type EditorConfig struct {
	ID          string                         `json:"id"`
	Name        string                         `json:"name"`
	Description string                         `json:"description"`
	Mode        orchestration.OrchestratorMode `json:"mode"`
	Nodes       []WorkflowNode                 `json:"nodes"`
	Edges       []WorkflowEdge                 `json:"edges"`
	CreatedAt   time.Time                      `json:"created_at"`
	UpdatedAt   time.Time                      `json:"updated_at"`
}

// WorkflowNode 工作流节点
type WorkflowNode struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"` // "agent", "start", "end", "condition"
	Name     string                 `json:"name"`
	Position NodePosition           `json:"position"`
	Config   map[string]interface{} `json:"config"`
}

// NodePosition 节点位置
type NodePosition struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// WorkflowEdge 工作流边
type WorkflowEdge struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
	Label  string `json:"label,omitempty"`
}

// ExecutionRecord 执行记录
type ExecutionRecord struct {
	ID          string                               `json:"id"`
	ConfigID    string                               `json:"config_id"`
	Status      orchestration.ExecutionStatus        `json:"status"`
	StartTime   time.Time                            `json:"start_time"`
	EndTime     time.Time                            `json:"end_time,omitempty"`
	Duration    time.Duration                        `json:"duration,omitempty"`
	StepResults map[string]*orchestration.StepResult `json:"step_results"`
	FinalOutput map[string]interface{}               `json:"final_output"`
	Error       string                               `json:"error,omitempty"`
}

// NewVisualEditor 创建可视化编辑器
func NewVisualEditor() *VisualEditor {
	return &VisualEditor{
		orchestrators: make(map[string]*orchestration.Orchestrator),
		configs:       make(map[string]*EditorConfig),
		executions:    make(map[string]*ExecutionRecord),
		agents:        make(map[string]agent.Agent),
	}
}

// RegisterAgent 注册 Agent 实例，供编排执行时使用
// nodeID 对应 EditorConfig.Nodes 中 "agent" 类型节点的 ID
func (ve *VisualEditor) RegisterAgent(nodeID string, a agent.Agent) {
	ve.mu.Lock()
	defer ve.mu.Unlock()
	ve.agents[nodeID] = a
}

// buildOrchestrator 从 EditorConfig 构建编排器
func (ve *VisualEditor) buildOrchestrator(cfg *EditorConfig) (*orchestration.Orchestrator, error) {
	orch := orchestration.NewOrchestrator(orchestration.OrchestratorConfig{
		Name:        cfg.Name,
		Description: cfg.Description,
		Mode:        cfg.Mode,
		Timeout:     defaultExecutionTimeout,
	})

	// 将工作流节点转换为编排步骤
	agentCount := 0
	for _, node := range cfg.Nodes {
		if node.Type != "agent" {
			continue // 跳过 start/end/condition 等非 Agent 节点
		}

		step := &orchestration.AgentStep{
			ID:   node.ID,
			Name: node.Name,
		}

		// 从节点配置中提取 prompt
		if prompt, ok := node.Config["prompt"].(string); ok {
			step.Prompt = prompt
		}

		// 解析已注册的 Agent 实例，未注册时使用 echoAgent
		ve.mu.RLock()
		if a, exists := ve.agents[node.ID]; exists {
			step.Agent = a
		} else {
			step.Agent = &echoAgent{name: node.Name}
		}
		ve.mu.RUnlock()

		if err := orch.AddStep(step); err != nil {
			return nil, fmt.Errorf("添加步骤 %s 失败: %w", node.ID, err)
		}
		agentCount++
	}

	// 无 agent 节点时拒绝执行
	if agentCount == 0 {
		return nil, fmt.Errorf("配置 %q 中无 agent 类型节点，无法执行编排", cfg.Name)
	}

	// DAG 模式下添加边
	if cfg.Mode == orchestration.DAGMode {
		for _, edge := range cfg.Edges {
			if err := orch.AddEdge(edge.Source, edge.Target); err != nil {
				return nil, fmt.Errorf("添加边 %s->%s 失败: %w", edge.Source, edge.Target, err)
			}
		}
	}

	return orch, nil
}

// executeAsync 异步执行编排并更新执行记录
func (ve *VisualEditor) executeAsync(execRecord *ExecutionRecord, cfg *EditorConfig) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultExecutionTimeout)
	defer cancel()

	orch, err := ve.buildOrchestrator(cfg)
	if err != nil {
		ve.mu.Lock()
		execRecord.Status = orchestration.StatusFailed
		execRecord.Error = fmt.Sprintf("构建编排器失败: %v", err)
		execRecord.EndTime = time.Now()
		execRecord.Duration = execRecord.EndTime.Sub(execRecord.StartTime)
		ve.mu.Unlock()
		return
	}

	result, execErr := orch.Execute(ctx, map[string]any{
		"config_id":   cfg.ID,
		"config_name": cfg.Name,
	})

	ve.mu.Lock()
	defer ve.mu.Unlock()

	execRecord.EndTime = time.Now()
	execRecord.Duration = execRecord.EndTime.Sub(execRecord.StartTime)

	if result != nil {
		execRecord.Status = result.Status
		execRecord.StepResults = result.Steps
		execRecord.FinalOutput = result.FinalOutput
		if result.Error != nil {
			execRecord.Error = result.Error.Error()
		}
	} else {
		execRecord.Status = orchestration.StatusFailed
	}

	if execErr != nil && execRecord.Error == "" {
		execRecord.Error = execErr.Error()
	}

	// 如果状态为失败但错误信息仍为空，从步骤结果中提取
	if execRecord.Status == orchestration.StatusFailed && execRecord.Error == "" {
		for _, step := range execRecord.StepResults {
			if step != nil && step.Error != nil {
				execRecord.Error = fmt.Sprintf("步骤 %s 失败: %v", step.StepID, step.Error)
				break
			}
		}
		if execRecord.Error == "" {
			execRecord.Error = "执行失败（未知原因）"
		}
	}
}

// echoAgent 默认回显 Agent，用于未注册真实 Agent 时的占位执行
type echoAgent struct {
	name string
}

func (a *echoAgent) Run(_ context.Context, input agent.Message) (*agent.Response, error) {
	return &agent.Response{
		Content: fmt.Sprintf("[%s] echo: %s", a.name, input.Content),
	}, nil
}

func (a *echoAgent) StreamRun(_ context.Context, input agent.Message) (<-chan agent.StreamEvent, error) {
	ch := make(chan agent.StreamEvent, 1)
	ch <- agent.StreamEvent{Type: agent.StreamEventToken, Content: fmt.Sprintf("[%s] echo: %s", a.name, input.Content)}
	close(ch)
	return ch, nil
}

func (a *echoAgent) Stop()                   {}
func (a *echoAgent) Stats() agent.AgentStats { return agent.AgentStats{} }
func (a *echoAgent) Name() string            { return a.name }

// VisualEditorServer 可视化编辑器HTTP服务器
type VisualEditorServer struct {
	editor *VisualEditor
	mux    *http.ServeMux
}

// NewVisualEditorServer 创建可视化编辑器服务器
func NewVisualEditorServer(editor *VisualEditor) *VisualEditorServer {
	s := &VisualEditorServer{
		editor: editor,
		mux:    http.NewServeMux(),
	}

	// API路由
	s.mux.HandleFunc("/api/editor/configs", s.handleConfigs)
	s.mux.HandleFunc("/api/editor/config/", s.handleConfig)
	s.mux.HandleFunc("/api/editor/execute/", s.handleExecute)
	s.mux.HandleFunc("/api/editor/executions", s.handleExecutions)
	s.mux.HandleFunc("/api/editor/execution/", s.handleExecution)

	// Web UI
	s.mux.HandleFunc("/editor", s.handleEditorUI)

	return s
}

// Handler 返回HTTP处理器
func (s *VisualEditorServer) Handler() http.Handler {
	return s.mux
}

// handleConfigs 处理配置列表
func (s *VisualEditorServer) handleConfigs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listConfigs(w, r)
	case http.MethodPost:
		s.createConfig(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// listConfigs 列出所有配置
func (s *VisualEditorServer) listConfigs(w http.ResponseWriter, r *http.Request) {
	s.editor.mu.RLock()
	defer s.editor.mu.RUnlock()

	configs := make([]*EditorConfig, 0, len(s.editor.configs))
	for _, cfg := range s.editor.configs {
		configs = append(configs, cfg)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(configs)
}

// createConfig 创建新配置
func (s *VisualEditorServer) createConfig(w http.ResponseWriter, r *http.Request) {
	var cfg EditorConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if cfg.ID == "" {
		cfg.ID = generateID()
	}
	cfg.CreatedAt = time.Now()
	cfg.UpdatedAt = time.Now()

	s.editor.mu.Lock()
	s.editor.configs[cfg.ID] = &cfg
	s.editor.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(cfg)
}

// handleConfig 处理单个配置
func (s *VisualEditorServer) handleConfig(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/api/editor/config/"):]
	if id == "" {
		http.Error(w, "Config ID required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.getConfig(w, r, id)
	case http.MethodPut:
		s.updateConfig(w, r, id)
	case http.MethodDelete:
		s.deleteConfig(w, r, id)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// getConfig 获取配置
func (s *VisualEditorServer) getConfig(w http.ResponseWriter, r *http.Request, id string) {
	s.editor.mu.RLock()
	cfg, exists := s.editor.configs[id]
	s.editor.mu.RUnlock()

	if !exists {
		http.Error(w, "Config not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cfg)
}

// updateConfig 更新配置
func (s *VisualEditorServer) updateConfig(w http.ResponseWriter, r *http.Request, id string) {
	s.editor.mu.Lock()
	defer s.editor.mu.Unlock()

	cfg, exists := s.editor.configs[id]
	if !exists {
		http.Error(w, "Config not found", http.StatusNotFound)
		return
	}

	var updated EditorConfig
	if err := json.NewDecoder(r.Body).Decode(&updated); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	updated.ID = id
	updated.CreatedAt = cfg.CreatedAt
	updated.UpdatedAt = time.Now()

	s.editor.configs[id] = &updated

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updated)
}

// deleteConfig 删除配置
func (s *VisualEditorServer) deleteConfig(w http.ResponseWriter, r *http.Request, id string) {
	s.editor.mu.Lock()
	defer s.editor.mu.Unlock()

	if _, exists := s.editor.configs[id]; !exists {
		http.Error(w, "Config not found", http.StatusNotFound)
		return
	}

	delete(s.editor.configs, id)
	delete(s.editor.orchestrators, id)

	w.WriteHeader(http.StatusNoContent)
}

// handleExecute 执行编排
func (s *VisualEditorServer) handleExecute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.URL.Path[len("/api/editor/execute/"):]
	if id == "" {
		http.Error(w, "Config ID required", http.StatusBadRequest)
		return
	}

	s.editor.mu.RLock()
	_, exists := s.editor.configs[id]
	s.editor.mu.RUnlock()

	if !exists {
		http.Error(w, "Config not found", http.StatusNotFound)
		return
	}

	// 创建执行记录
	execID := generateID()
	execRecord := &ExecutionRecord{
		ID:          execID,
		ConfigID:    id,
		Status:      orchestration.StatusRunning,
		StartTime:   time.Now(),
		StepResults: make(map[string]*orchestration.StepResult),
		FinalOutput: make(map[string]interface{}),
	}

	s.editor.mu.Lock()
	s.editor.executions[execID] = execRecord
	cfg := s.editor.configs[id]
	s.editor.mu.Unlock()

	// 异步执行编排
	go s.editor.executeAsync(execRecord, cfg)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{
		"execution_id": execID,
		"status":       "started",
	})
}

// handleExecutions 处理执行列表
func (s *VisualEditorServer) handleExecutions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.editor.mu.RLock()
	defer s.editor.mu.RUnlock()

	executions := make([]*ExecutionRecord, 0, len(s.editor.executions))
	for _, exec := range s.editor.executions {
		executions = append(executions, exec)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(executions)
}

// handleExecution 处理单个执行
func (s *VisualEditorServer) handleExecution(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.URL.Path[len("/api/editor/execution/"):]
	if id == "" {
		http.Error(w, "Execution ID required", http.StatusBadRequest)
		return
	}

	s.editor.mu.RLock()
	exec, exists := s.editor.executions[id]
	s.editor.mu.RUnlock()

	if !exists {
		http.Error(w, "Execution not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(exec)
}

// handleEditorUI 提供编辑器Web UI
func (s *VisualEditorServer) handleEditorUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(editorHTML))
}

// editorHTML 编辑器HTML（使用React Flow）
const editorHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>AP Visual Editor - 可视化编排工具</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: #f5f7fa;
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
            display: flex;
            height: calc(100vh - 100px);
        }
        
        .sidebar {
            width: 300px;
            background: white;
            border-right: 1px solid #e0e0e0;
            padding: 20px;
            overflow-y: auto;
        }
        
        .sidebar h2 {
            font-size: 18px;
            margin-bottom: 15px;
            color: #2c3e50;
        }
        
        .node-palette {
            display: grid;
            grid-template-columns: 1fr 1fr;
            gap: 10px;
            margin-bottom: 30px;
        }
        
        .node-item {
            padding: 15px;
            border: 2px solid #e0e0e0;
            border-radius: 8px;
            text-align: center;
            cursor: pointer;
            transition: all 0.2s;
        }
        
        .node-item:hover {
            border-color: #667eea;
            background: #f8f9ff;
        }
        
        .node-item.agent {
            background: #e3f2fd;
            border-color: #2196f3;
        }
        
        .node-item.start {
            background: #e8f5e9;
            border-color: #4caf50;
        }
        
        .node-item.end {
            background: #ffebee;
            border-color: #f44336;
        }
        
        .node-item.condition {
            background: #fff3e0;
            border-color: #ff9800;
        }
        
        .canvas {
            flex: 1;
            background: #fafafa;
            position: relative;
        }
        
        .toolbar {
            position: absolute;
            top: 20px;
            right: 20px;
            display: flex;
            gap: 10px;
            z-index: 10;
        }
        
        .btn {
            padding: 10px 20px;
            border: none;
            border-radius: 6px;
            font-size: 14px;
            font-weight: 500;
            cursor: pointer;
            transition: all 0.2s;
        }
        
        .btn-primary {
            background: #667eea;
            color: white;
        }
        
        .btn-primary:hover {
            background: #5568d3;
        }
        
        .btn-secondary {
            background: white;
            color: #667eea;
            border: 2px solid #667eea;
        }
        
        .btn-secondary:hover {
            background: #f8f9ff;
        }
        
        .config-list {
            margin-top: 20px;
        }
        
        .config-item {
            padding: 12px;
            border: 1px solid #e0e0e0;
            border-radius: 6px;
            margin-bottom: 10px;
            cursor: pointer;
            transition: all 0.2s;
        }
        
        .config-item:hover {
            border-color: #667eea;
            background: #f8f9ff;
        }
        
        .config-item.active {
            border-color: #667eea;
            background: #e8eaff;
        }
        
        .config-name {
            font-weight: 600;
            color: #2c3e50;
            margin-bottom: 5px;
        }
        
        .config-meta {
            font-size: 12px;
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
        
        /* React Flow 样式 */
        .react-flow {
            width: 100%;
            height: 100%;
        }
        
        .react-flow__node {
            border-radius: 8px;
            padding: 10px;
            box-shadow: 0 2px 8px rgba(0,0,0,0.1);
        }
        
        .react-flow__node-agent {
            background: #e3f2fd;
            border: 2px solid #2196f3;
        }
        
        .react-flow__node-start {
            background: #e8f5e9;
            border: 2px solid #4caf50;
        }
        
        .react-flow__node-end {
            background: #ffebee;
            border: 2px solid #f44336;
        }
        
        .react-flow__node-condition {
            background: #fff3e0;
            border: 2px solid #ff9800;
        }
        
        .react-flow__edge-path {
            stroke: #667eea;
            stroke-width: 2;
        }
    </style>
</head>
<body>
    <div class="header">
        <h1>🎨 AP Visual Editor</h1>
        <p>可视化编排工具 - 拖拽式创建Agent工作流</p>
    </div>
    
    <div class="container">
        <div class="sidebar">
            <h2>节点类型</h2>
            <div class="node-palette">
                <div class="node-item start" draggable="true" data-type="start">
                    <div>🟢</div>
                    <div>开始</div>
                </div>
                <div class="node-item agent" draggable="true" data-type="agent">
                    <div>🤖</div>
                    <div>Agent</div>
                </div>
                <div class="node-item condition" draggable="true" data-type="condition">
                    <div>🔀</div>
                    <div>条件</div>
                </div>
                <div class="node-item end" draggable="true" data-type="end">
                    <div>🔴</div>
                    <div>结束</div>
                </div>
            </div>
            
            <h2>工作流配置</h2>
            <div class="config-list" id="config-list">
                <div class="empty-state">
                    <div class="empty-state-icon">📋</div>
                    <p>暂无配置</p>
                    <p style="font-size: 13px; margin-top: 10px;">点击"新建工作流"开始创建</p>
                </div>
            </div>
        </div>
        
        <div class="canvas">
            <div class="toolbar">
                <button class="btn btn-secondary" onclick="newWorkflow()">新建工作流</button>
                <button class="btn btn-primary" onclick="saveWorkflow()">保存</button>
                <button class="btn btn-primary" onclick="executeWorkflow()">执行</button>
            </div>
            
            <div id="react-flow-container" class="react-flow">
                <div class="empty-state">
                    <div class="empty-state-icon">🎨</div>
                    <p>从左侧拖拽节点到画布</p>
                    <p style="font-size: 13px; margin-top: 10px;">连接节点创建数据流</p>
                </div>
            </div>
        </div>
    </div>
    
    <script>
        // 这里应该引入React Flow库
        // 由于是示例，我们使用简化的实现
        
        let currentConfig = null;
        let nodes = [];
        let edges = [];
        
        // 拖拽开始
        document.querySelectorAll('.node-item').forEach(item => {
            item.addEventListener('dragstart', (e) => {
                e.dataTransfer.setData('type', e.target.dataset.type);
            });
        });
        
        // 拖拽放置
        const canvas = document.querySelector('.canvas');
        canvas.addEventListener('dragover', (e) => {
            e.preventDefault();
        });
        
        canvas.addEventListener('drop', (e) => {
            e.preventDefault();
            const type = e.dataTransfer.getData('type');
            const rect = canvas.getBoundingClientRect();
            const x = e.clientX - rect.left;
            const y = e.clientY - rect.top;
            
            addNode(type, x, y);
        });
        
        function addNode(type, x, y) {
            const node = {
                id: 'node_' + Date.now(),
                type: type,
                position: { x, y },
                data: { label: type.charAt(0).toUpperCase() + type.slice(1) }
            };
            nodes.push(node);
            renderNodes();
        }
        
        function renderNodes() {
            const container = document.getElementById('react-flow-container');
            container.innerHTML = nodes.map(node => 
                '<div class="react-flow__node react-flow__node-' + node.type + '" ' +
                'style="position: absolute; left: ' + node.position.x + 'px; top: ' + node.position.y + 'px;">' +
                node.data.label +
                '</div>'
            ).join('');
        }
        
        function newWorkflow() {
            currentConfig = {
                id: 'wf_' + Date.now(),
                name: '新工作流',
                description: '',
                mode: 'dag',
                nodes: [],
                edges: [],
                created_at: new Date().toISOString(),
                updated_at: new Date().toISOString()
            };
            nodes = [];
            edges = [];
            renderNodes();
        }
        
        async function saveWorkflow() {
            if (!currentConfig) {
                alert('请先创建新工作流');
                return;
            }
            
            currentConfig.nodes = nodes;
            currentConfig.edges = edges;
            
            try {
                const response = await fetch('/api/editor/configs', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(currentConfig)
                });
                
                if (response.ok) {
                    alert('保存成功');
                    loadConfigs();
                } else {
                    alert('保存失败');
                }
            } catch (error) {
                console.error('保存失败:', error);
                alert('保存失败: ' + error.message);
            }
        }
        
        async function executeWorkflow() {
            if (!currentConfig) {
                alert('请先保存工作流');
                return;
            }
            
            try {
                const response = await fetch('/api/editor/execute/' + currentConfig.id, {
                    method: 'POST'
                });
                
                if (response.ok) {
                    const result = await response.json();
                    alert('执行已启动，ID: ' + result.execution_id);
                } else {
                    alert('执行失败');
                }
            } catch (error) {
                console.error('执行失败:', error);
                alert('执行失败: ' + error.message);
            }
        }
        
        async function loadConfigs() {
            try {
                const response = await fetch('/api/editor/configs');
                const configs = await response.json();
                
                const list = document.getElementById('config-list');
                if (configs.length === 0) {
                    list.innerHTML = '<div class="empty-state">' +
                        '<div class="empty-state-icon">📋</div>' +
                        '<p>暂无配置</p>' +
                        '</div>';
                    return;
                }
                
                list.innerHTML = configs.map(cfg => 
                    '<div class="config-item" onclick="loadConfig(\'' + cfg.id + '\')">' +
                    '<div class="config-name">' + cfg.name + '</div>' +
                    '<div class="config-meta">' + cfg.nodes.length + ' 节点 | ' + 
                    new Date(cfg.updated_at).toLocaleString('zh-CN') + '</div>' +
                    '</div>'
                ).join('');
            } catch (error) {
                console.error('加载配置失败:', error);
            }
        }
        
        async function loadConfig(id) {
            try {
                const response = await fetch('/api/editor/config/' + id);
                if (response.ok) {
                    currentConfig = await response.json();
                    nodes = currentConfig.nodes || [];
                    edges = currentConfig.edges || [];
                    renderNodes();
                }
            } catch (error) {
                console.error('加载配置失败:', error);
            }
        }
        
        // 初始加载
        loadConfigs();
    </script>
</body>
</html>`

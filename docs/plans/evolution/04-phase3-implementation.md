# Phase 3 实施文档：生态引领期（12-18 个月）

> 工作域：G3（Go 生产级治理 + WASM）+ T3（TS Edge-Native + 浏览器 Agent）  
> 前置条件：Phase 2 全部完成

---

## G3：Go 端生态引领

### G3-1：MCP Server 实现

#### 目标

Go 端不仅消费 MCP 工具（Client），还提供 MCP 工具给其他 Agent 框架调用（Server），实现双向 MCP 能力。

#### 背景

当前 `agentprimordia/internal/mcp/` 只有 Client 实现。2026 年 MCP 已成为 Agent 工具集成标准，提供 Server 能力意味着 AgentPrimordia 的 Agent 能被 Claude、ChatGPT、Cursor 等任何支持 MCP 的客户端直接调用。

#### 新增文件

```
agentprimordia/internal/mcp/
    server.go                 ← MCP Server 核心实现
    server_registry.go        ← 工具注册表
    server_transport.go       ← stdio / SSE / WebSocket 传输层
    server_test.go
```

#### 核心设计

```go
package mcp

import (
    "context"
    "encoding/json"
    "fmt"
    "io"
    "os"
    "sync"
)

// MCPServer MCP 服务端
type MCPServer struct {
    mu         sync.RWMutex
    tools      map[string]ToolHandler
    resources  map[string]ResourceHandler
    prompts    map[string]PromptHandler
    transport  ServerTransport
}

// ToolHandler 工具处理函数
type ToolHandler func(ctx context.Context, args json.RawMessage) (interface{}, error)

// ServerTransport 服务端传输层接口
type ServerTransport interface {
    Start(ctx context.Context) error
    Stop() error
    Send(message json.RawMessage) error
    Receive() <-chan json.RawMessage
}

// NewMCPServer 创建 MCP 服务端
func NewMCPServer(transport ServerTransport) *MCPServer {
    return &MCPServer{
        tools:     make(map[string]ToolHandler),
        resources: make(map[string]ResourceHandler),
        prompts:   make(map[string]PromptHandler),
        transport: transport,
    }
}

// RegisterTool 注册 MCP 工具
func (s *MCPServer) RegisterTool(name, description string, schema json.RawMessage, handler ToolHandler) {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.tools[name] = handler
}

// Serve 启动服务（阻塞）
func (s *MCPServer) Serve(ctx context.Context) error {
    if err := s.transport.Start(ctx); err != nil {
        return err
    }
    defer s.transport.Stop()

    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case msg, ok := <-s.transport.Receive():
            if !ok {
                return nil
            }
            go s.handleMessage(ctx, msg)
        }
    }
}

// handleMessage 处理 JSON-RPC 请求
func (s *MCPServer) handleMessage(ctx context.Context, msg json.RawMessage) {
    var req JSONRPCRequest
    if err := json.Unmarshal(msg, &req); err != nil {
        s.sendError(req.ID, -32700, "Parse error")
        return
    }

    switch req.Method {
    case "initialize":
        s.handleInitialize(ctx, req)
    case "tools/list":
        s.handleToolsList(ctx, req)
    case "tools/call":
        s.handleToolCall(ctx, req)
    case "resources/list":
        s.handleResourcesList(ctx, req)
    case "prompts/list":
        s.handlePromptsList(ctx, req)
    default:
        s.sendError(req.ID, -32601, "Method not found")
    }
}

// ===== 传输层实现 =====

// StdioTransport stdio 传输（标准 MCP 传输方式）
type StdioTransport struct {
    reader  io.Reader
    writer  io.Writer
    recvCh  chan json.RawMessage
}

func NewStdioTransport() *StdioTransport {
    return &StdioTransport{
        reader: os.Stdin,
        writer: os.Stdout,
        recvCh: make(chan json.RawMessage, 100),
    }
}

// 注意：StdioTransport 需实现 ServerTransport 接口的全部方法：
// Start(ctx) / Stop() / Send(message) / Receive() <-chan json.RawMessage
// 此处省略实现细节，完整代码需补充 Start（启动读取 goroutine）、
// Stop（关闭通道）、Send（写入 stdout）、Receive（返回 recvCh）

// SSETransport Server-Sent Events 传输（用于远程 MCP 客户端）
type SSETransport struct {
    port     int
    clients  map[string]chan json.RawMessage
    mu       sync.RWMutex
}
```

#### 集成方式

```go
// 将 Agent 暴露为 MCP Server
agent := NewReActAgent(config).WithToolkit(toolkit)

mcpServer := mcp.NewMCPServer(mcp.NewStdioTransport())
mcpServer.RegisterTool("agent.run", "执行 Agent 任务", agentRunSchema, func(ctx context.Context, args json.RawMessage) (interface{}, error) {
    var input struct{ Query string `json:"query"` }
    json.Unmarshal(args, &input)
    resp, err := agent.Run(ctx, UserMessage(input.Query))
    return resp, err
})
mcpServer.RegisterTool("agent.status", "查询 Agent 状态", agentStatusSchema, func(ctx context.Context, args json.RawMessage) (interface{}, error) {
    return agent.Stats(), nil
})

mcpServer.Serve(ctx)
```

#### 验收标准

```bash
go test -run TestMCPServer ./internal/mcp/
# 使用 MCP Inspector 工具验证
npx @anthropic/mcp-inspector go run ./cmd/mcp-server/
```

---

### G3-2：Agent 治理引擎

#### 目标

策略即代码：用 YAML 定义 Agent 行为策略，运行时强制执行。

#### 新增文件

```
agentprimordia/internal/governance/
    policy.go              ← 策略引擎核心
    policy_loader.go       ← YAML 策略加载器
    policy_enforcer.go     ← 运行时策略执行器
    policy_test.go
```

#### 策略定义格式

```yaml
# agent-policy.yaml
apiVersion: agent.primordia.dev/v1
kind: AgentPolicy
metadata:
  name: production-safety
spec:
  # 工具限制
  toolRestrictions:
    - tool: "shell"
      requireApproval: true
      maxCallsPerRun: 5
      blockedArgs:
        - "rm -rf"
        - "sudo"
    - tool: "http_request"
      allowedDomains:
        - "api.openai.com"
        - "api.anthropic.com"
      maxCallsPerRun: 20

  # 成本限制
  costLimits:
    perRequest: 0.50        # 单次请求最大成本（美元）
    perDay: 50.00           # 每日最大成本
    perSession: 5.00        # 每会话最大成本

  # 输出安全
  outputGuardrail:
    piiFilter: strict       # strict | moderate | off
    injectionBlock: true
    maxLength: 10000

  # 行为约束
  behaviorConstraints:
    maxTurns: 20
    maxToolCalls: 50
    requireReflection: true # 强制要求 Reflection 检查
```

#### 核心设计

```go
package governance

import (
    "context"
    "fmt"
    "os"
    "strings"
    "sync"
    "gopkg.in/yaml.v3"
)

// 注意：Policy 结构体仅映射了 metadata 和 spec，
// YAML 中的 apiVersion 和 kind 字段需额外定义或忽略
type Policy struct {
    APIVersion string         `yaml:"apiVersion"`  // 新增：映射 YAML apiVersion
    Kind       string         `yaml:"kind"`        // 新增：映射 YAML kind
    Metadata   PolicyMetadata `yaml:"metadata"`
    Spec       PolicySpec     `yaml:"spec"`
}

type PolicySpec struct {
    ToolRestrictions   []ToolRestriction   `yaml:"toolRestrictions"`
    CostLimits         CostLimits          `yaml:"costLimits"`
    OutputGuardrail    OutputGuardrail     `yaml:"outputGuardrail"`
    BehaviorConstraints BehaviorConstraints `yaml:"behaviorConstraints"`
}

type ToolRestriction struct {
    Tool            string   `yaml:"tool"`
    RequireApproval bool     `yaml:"requireApproval"`
    MaxCallsPerRun  int      `yaml:"maxCallsPerRun"`
    BlockedArgs     []string `yaml:"blockedArgs"`
    AllowedDomains  []string `yaml:"allowedDomains"`
}

// PolicyEnforcer 策略执行器
type PolicyEnforcer struct {
    policy *Policy
    // 运行时状态
    toolCallCount map[string]int
    totalCost     float64
    mu            sync.Mutex
}

// 注意：以下错误变量需定义：
// var ErrToolCallLimitExceeded = errors.New("tool call limit exceeded")
// var ErrCostLimitExceeded = errors.New("cost limit exceeded")

// CheckToolCall 在工具执行前检查策略
func (e *PolicyEnforcer) CheckToolCall(toolName, args string) error {
    e.mu.Lock()
    defer e.mu.Unlock()
    
    restriction := e.findToolRestriction(toolName)
    if restriction == nil {
        return nil // 无限制
    }
    
    // 检查调用次数
    if restriction.MaxCallsPerRun > 0 {
        if e.toolCallCount[toolName] >= restriction.MaxCallsPerRun {
            return ErrToolCallLimitExceeded
        }
    }
    
    // 检查禁止参数
    for _, blocked := range restriction.BlockedArgs {
        if strings.Contains(args, blocked) {
            return fmt.Errorf("tool %s: blocked argument pattern '%s'", toolName, blocked)
        }
    }
    
    e.toolCallCount[toolName]++
    return nil
}

// CheckCost 在 LLM 调用后检查成本
func (e *PolicyEnforcer) CheckCost(cost float64) error {
    e.mu.Lock()
    defer e.mu.Unlock()
    
    e.totalCost += cost
    if e.totalCost > e.policy.Spec.CostLimits.PerRequest {
        return ErrCostLimitExceeded
    }
    return nil
}
```

#### 集成到 ReAct Loop

```go
// react_loop_core.go — 在 runLoop 中插入策略检查
if enforcer := a.getPolicyEnforcer(); enforcer != nil {
    if err := enforcer.CheckCost(estimatedCost); err != nil {
        return &Response{Error: err}, err
    }
}

// react_loop_tools.go — 在 executeToolCalls 中插入工具策略检查
if enforcer := a.getPolicyEnforcer(); enforcer != nil {
    if err := enforcer.CheckToolCall(tc.Name, tc.Args); err != nil {
        // 策略违规，记录审计 + 拒绝执行
        a.writeAudit(ctx, AuditEvent{
            Action: "policy.violation",
            Result: "blocked",
            Details: map[string]any{"tool": tc.Name, "reason": err.Error()},
        })
        continue
    }
}
```

#### 验收标准

```bash
go test -run TestPolicyEnforcer ./internal/governance/
go test -run TestPolicyIntegration ./internal/agent/
```

---

### G3-3：深度 WASM 工具沙箱

#### 目标

用户自定义工具编译为 WASM，在 wazero 沙箱中安全执行，支持 Fuel 计量和内存隔离。

#### 改动点

**文件**：`wasm/runtime.go` 扩展

```go
// 新增：工具执行接口
// 注意：需补充以下 import：
// import (
//     "bufio"
//     "context"
//     "encoding/json"
//     "fmt"
//     "os"
//     "github.com/tetratelabs/wazero"
//     "agentprimordia/internal/tools"
// )
func (r *Runtime) ExecuteTool(ctx context.Context, moduleName, functionName string, input []byte) ([]byte, error) {
    r.mu.RLock()
    compiled, exists := r.modules[moduleName]
    r.mu.RUnlock()
    if !exists {
        return nil, fmt.Errorf("wasm: module %q not found", moduleName)
    }

    // 创建带 Fuel 限制的实例
    moduleCfg := wazero.NewModuleConfig().
        WithStdout(bufio.NewWriter(os.Stdout)).
        WithStderr(bufio.NewWriter(os.Stderr))
    
    // Fuel 计量：限制 CPU 使用
    if r.config.MaxFuel > 0 {
        moduleCfg = moduleCfg.WithFuel(r.config.MaxFuel)
    }

    mod, err := r.ctx.InstantiateModule(ctx, compiled, moduleCfg)
    if err != nil {
        return nil, fmt.Errorf("wasm: instantiate %q: %w", moduleName, err)
    }
    defer mod.Close(ctx)

    // 调用工具函数
    fn := mod.ExportedFunction(functionName)
    if fn == nil {
        return nil, fmt.Errorf("wasm: function %q not found in module %q", functionName, moduleName)
    }

    // 写入输入参数到内存
    inputPtr, err := r.writeToMemory(ctx, mod, input)
    if err != nil {
        return nil, err
    }

    // 执行
    results, err := fn.Call(ctx, uint64(inputPtr), uint64(len(input)))
    if err != nil {
        return nil, fmt.Errorf("wasm: execute %q: %w", functionName, err)
    }

    // 读取输出
    outputPtr := results[0]
    outputLen := results[1]
    return r.readFromMemory(ctx, mod, uint32(outputPtr), uint32(outputLen))
}

// 注册为 Agent 工具
type WASMTool struct {
    name        string
    description string
    runtime     *wasm.Runtime
    moduleName  string
    funcName    string
}

func (t *WASMTool) Execute(ctx context.Context, args json.RawMessage) (*tools.Result, error) {
    output, err := t.runtime.ExecuteTool(ctx, t.moduleName, t.funcName, args)
    if err != nil {
        return tools.NewErrorResult(err.Error()), err
    }
    return tools.NewResult(string(output)), nil
}
```

#### 验收标准

```bash
go test -run TestWASMToolExecute ./wasm/
# 安全性测试：确认 WASM 无法访问文件系统
go test -run TestWASMSandbox_Isolation ./wasm/
# Fuel 计量测试
go test -run TestWASMFuel ./wasm/
```

---

### G3-4：分层记忆架构

#### 目标

三层记忆：Working Memory（内存）→ Episodic Memory（SQLite+向量）→ Semantic Memory（结构化知识）。

#### 新增文件

```
agentprimordia/internal/memory/
    working_memory.go        ← 工作记忆（当前对话上下文）
    semantic_memory.go       ← 语义记忆（结构化知识）
    memory_distiller.go      ← 记忆蒸馏器（Episodic → Semantic）
```

#### 核心设计

```go
// working_memory.go — 短期工作记忆
type WorkingMemory struct {
    messages    []Message
    maxTokens   int
    compressor  ContextCompressor
}

// 在每轮 ReAct 结束后压缩工作记忆
func (w *WorkingMemory) Compress() {
    if w.estimateTokens() > w.maxTokens {
        // 保留最近 N 条 + 压缩早期消息为摘要
        w.messages = w.compressor.Compress(w.messages, w.maxTokens)
    }
}

// semantic_memory.go — 长期语义记忆
type SemanticMemory struct {
    store      MemoryStore
    patterns   map[string][]Pattern  // 工具使用模式
    facts      map[string]Fact       // 结构化事实
}

type Pattern struct {
    Pattern     string
    Description string
    SuccessRate float64
    Examples    []string
    LastUpdated time.Time
}

type Fact struct {
    Key         string
    Value       string
    Confidence  float64
    Source      string  // "distilled" | "user_provided"
    CreatedAt   time.Time
}

// memory_distiller.go — 从 Episodic 蒸馏到 Semantic
type MemoryDistiller struct {
    episodicStore  MemoryStore
    semanticMemory *SemanticMemory
    llm            llm.Provider  // 用于蒸馏推理
}

// Distill 从历史对话中提取结构化知识
func (d *MemoryDistiller) Distill(ctx context.Context, sessionID string) error {
    // 1. 查询该 session 的所有 episode
    episodes, err := d.episodicStore.Query(ctx, sessionID, nil)
    if err != nil {
        return err
    }

    // 2. 使用 LLM 从对话中提取模式
    prompt := d.buildDistillationPrompt(episodes)
    resp, err := d.llm.Complete(ctx, &llm.CompletionRequest{
        Messages: []llm.ChatMessage{{Role: "user", Content: prompt}},
    })
    if err != nil {
        return err
    }

    // 3. 解析 LLM 输出为结构化知识
    patterns, facts := d.parseDistillationResult(resp.Content)
    
    // 4. 存储到 SemanticMemory
    for _, p := range patterns {
        d.semanticMemory.AddPattern(ctx, p)
    }
    for _, f := range facts {
        d.semanticMemory.AddFact(ctx, f)
    }

    return nil
}

// 在 ReAct Loop 中使用 SemanticMemory
// 每轮开始时，从 SemanticMemory 检索相关模式和事实，注入 system prompt
```

#### 验收标准

```bash
go test -run TestWorkingMemory ./internal/memory/
go test -run TestSemanticMemory ./internal/memory/
go test -run TestMemoryDistiller ./internal/memory/
go test -run TestLayeredMemoryIntegration ./internal/agent/
```

---

## T3：TS 端 Edge-Native Agent

### T3-1：Edge Agent 运行时

#### 目标

Agent 在 Cloudflare Workers / Deno Deploy / Bun 上运行，利用平台原生存储。

#### 新增文件

```
sdk/typescript/src/edge/
    cloudflare-agent.ts      ← Cloudflare Workers 适配
    deno-agent.ts            ← Deno Deploy 适配
    bun-agent.ts             ← Bun 适配
    edge-storage.ts          ← 统一存储抽象
```

#### 核心设计

```typescript
// cloudflare-agent.ts — Cloudflare Workers 上的轻量 Agent
import { DurableObject } from 'cloudflare:workers';

// 使用 Durable Objects 做 Agent 状态存储
export class AgentDurableObject extends DurableObject {
  private agent: ReActAgent;

  constructor(state: DurableObjectState) {
    super(state);
    // 使用 Durable Object storage 作为 MemoryStore
    const memory = new DurableObjectMemoryStore(state.storage);
    this.agent = createAgent('edge-agent')
      .withProvider(getEdgeProvider())
      .withMemory(memory)
      .build();
  }

  async fetch(request: Request): Promise<Response> {
    const input = await request.text();
    const response = await this.agent.run(input);
    return new Response(response.content);
  }
}

// edge-storage.ts — 统一存储抽象
export interface EdgeStorage {
  get(key: string): Promise<unknown>;
  set(key: string, value: unknown): Promise<void>;
  delete(key: string): Promise<void>;
  list(prefix: string): Promise<[string, unknown][]>;
}

// 各平台适配器
export class CloudflareKVStorage implements EdgeStorage { /* ... */ }
export class DenoKVStorage implements EdgeStorage { /* ... */ }
export class BunSQLiteStorage implements EdgeStorage { /* ... */ }
```

#### 验收标准

```bash
npm test -- --grep "CloudflareAgent"
npm test -- --grep "DenoAgent"
npm test -- --grep "EdgeStorage"
```

---

### T3-2：浏览器端 Agent（WASM）

#### 目标

将 TS Agent 编译为 WASM 在浏览器中运行，用户数据不离开浏览器。

#### 新增文件

```
sdk/typescript/src/browser/
    wasm-agent.ts            ← WASM Agent 入口
    browser-provider.ts      ← WebGPU 本地推理 Provider
    indexeddb-checkpoint.ts  ← IndexedDB 检查点存储
```

#### 核心设计

```typescript
// browser-provider.ts — 使用 WebGPU 做本地推理
// 注意：Provider 接口要求实现 complete, callTools, info 方法
// stream 和 embeddings 为可选方法
import type { Provider, CompletionRequest, CompletionResponse, ToolCallRequest, ToolCallResponse, ModelInfo, Chunk } from '../types.js';

export class WebGPUProvider implements Provider {
  private device: GPUDevice;
  private model: GPUBuffer;

  async init(modelUrl: string): Promise<void> {
    const adapter = await navigator.gpu.requestAdapter();
    this.device = await adapter.requestDevice();
    
    // 加载模型（GGUF/WASM 格式）
    const modelData = await fetch(modelUrl).then(r => r.arrayBuffer());
    this.model = this.device.createBuffer({
      size: modelData.byteLength,
      usage: GPUBufferUsage.STORAGE | GPUBufferUsage.COPY_DST,
    });
    this.device.queue.writeBuffer(this.model, 0, modelData);
  }

  async complete(req: CompletionRequest): Promise<CompletionResponse> {
    // 使用 WebGPU compute shader 做推理
    // ...
  }

  async *stream(req: CompletionRequest): AsyncGenerator<Chunk> {
    // 流式推理
    // ...
  }
}

// wasm-agent.ts — 浏览器端 Agent 入口
export class BrowserAgent {
  static async create(config: {
    modelUrl: string;
    systemPrompt?: string;
  }): Promise<BrowserAgent> {
    const provider = new WebGPUProvider();
    await provider.init(config.modelUrl);
    
    const memory = new IndexedDBVectorStore();
    await memory.init();
    
    const agent = createAgent('browser-agent')
      .withProvider(provider)
      .withMemory(memory)
      .withMaxTurns(10)
      .build();
    
    return new BrowserAgent(agent);
  }
}
```

#### 验收标准

```bash
# 需要支持 WebGPU 的浏览器
npm run test:e2e -- --browser chrome --grep "BrowserAgent"
```

---

### T3-3：投机执行 v2

#### 目标

基于历史数据训练预测模型，替代简单的"用上一次结果"策略。

#### 改动点

**文件**：`sdk/typescript/src/agent/speculative-exec.ts` 扩展

```typescript
import * as tf from '@tensorflow/tfjs';

export class NeuralToolPredictor extends ToolResultPredictor {
  private model: tf.LayersModel | null = null;
  private toolEncoders: Map<string, ToolEncoder> = new Map();

  async train(records: ToolUsageRecord[]): Promise<void> {
    // 构建训练数据
    const features = records.map(r => this.encode(r));
    const labels = records.map(r => this.encodeResult(r));
    
    const xs = tf.tensor2d(features);
    const ys = tf.tensor2d(labels);
    
    // 构建简单模型
    this.model = tf.sequential({
      layers: [
        tf.layers.dense({ inputShape: [features[0].length], units: 64, activation: 'relu' }),
        tf.layers.dense({ units: 32, activation: 'relu' }),
        tf.layers.dense({ units: labels[0].length, activation: 'sigmoid' }),
      ],
    });
    
    this.model.compile({ optimizer: 'adam', loss: 'binaryCrossentropy' });
    await this.model.fit(xs, ys, { epochs: 50 });
  }

  predict(toolName: string, args: string): ToolResult | null {
    if (!this.model) return super.predict(toolName, args);
    
    const encoded = this.encode({ toolName, args, result: '', success: false });
    const prediction = this.model.predict(tf.tensor2d([encoded])) as tf.Tensor;
    const result = prediction.dataSync();
    
    // 解码预测结果
    return this.decodeResult(result);
  }
}
```

#### 验收标准

```bash
npm test -- --grep "NeuralToolPredictor"
# 预测准确率应 > 60%（比 v1 的"用上一次结果"策略更高）
npm run bench -- speculative-exec
```

---

### T3-4：实时多 Agent 协作 UI

#### 目标

可视化多 Agent 协作过程，支持实时消息流和人类介入。

#### 新增文件

```
sdk/typescript/src/react/
    collaboration/
      CollaborationView.tsx       ← 协作主视图
      AgentNode.tsx               ← Agent 节点
      MessageFlow.tsx             ← 消息流动画
      HITLPanel.tsx               ← 人类介入面板
      CollaborationReplay.tsx     ← 协作回放
```

#### 核心设计

```tsx
// CollaborationView.tsx
export function CollaborationView({ session }: { session: CollaborationSession }) {
  const [agents, setAgents] = useState<AgentNode[]>([]);
  const [messages, setMessages] = useState<CollaborationMessage[]>([]);

  useEffect(() => {
    // 通过 WebSocket 订阅实时消息流
    const ws = new WebSocketTransport({ url: session.websocketUrl });
    ws.onMessage((msg) => {
      setMessages(prev => [...prev, msg]);
    });
    return () => ws.close();
  }, [session]);

  return (
    <div className="flex flex-col h-screen">
      {/* 顶部：Agent 状态栏 */}
      <div className="flex gap-4 p-4">
        {agents.map(agent => (
          <AgentStatusCard key={agent.id} agent={agent} />
        ))}
      </div>
      
      {/* 中部：消息流可视化 */}
      <div className="flex-1 relative">
        <MessageFlow messages={messages} />
      </div>
      
      {/* 底部：HITL 面板 */}
      <HITLPanel
        pendingApprovals={session.pendingApprovals}
        onApprove={session.approve}
        onReject={session.reject}
      />
    </div>
  );
}

// MessageFlow.tsx — 消息流动画
export function MessageFlow({ messages }: { messages: CollaborationMessage[] }) {
  return (
    <ReactFlow
      nodes={messages.map(msg => ({
        id: msg.id,
        position: calculatePosition(msg),
        data: { label: <MessageBubble message={msg} /> },
      }))}
      edges={messages.map((msg, i) => ({
        id: `edge-${i}`,
        source: msg.from,
        target: msg.to,
        animated: true,
      }))}
    />
  );
}
```

#### 验收标准

```bash
npm run storybook -- --stories-file src/react/collaboration/CollaborationView.stories.tsx
npm test -- --grep "CollaborationView"
```

---

## Phase 3 里程碑

| 里程碑 | 时间 | 交付物 |
|--------|------|--------|
| M3.1 | 第 40 周 | G3-1 MCP Server + G3-2 治理引擎 |
| M3.2 | 第 44 周 | G3-3 WASM 工具沙箱 + G3-4 分层记忆 |
| M3.3 | 第 48 周 | T3-1 Edge Agent 运行时 |
| M3.4 | 第 52 周 | T3-2 浏览器 WASM Agent |
| M3.5 | 第 56 周 | T3-3 投机执行 v2 + T3-4 协作 UI |
| M3.6 | 第 60 周 | 端到端测试 + 文档 + 发布 |

## Phase 3 验收标准

### Go 端

```bash
# MCP Server 兼容性（使用官方 MCP Inspector）
npx @anthropic/mcp-inspector go run ./cmd/mcp-server/

# 治理引擎策略执行
go test -run TestPolicyEnforcement ./internal/governance/

# WASM 沙箱安全性
go test -run TestWASMSandbox_Isolation ./wasm/

# 分层记忆蒸馏
go test -run TestMemoryDistillation ./internal/memory/

# 全量回归
go test -race ./...
```

### TS 端

```bash
cd sdk/typescript

# Edge Agent 部署测试
wrangler deploy --test  # Cloudflare Workers
deno deploy --test       # Deno Deploy

# 浏览器 Agent 测试
npm run test:e2e -- --browser chrome --grep "BrowserAgent"

# 投机执行准确率
npm run bench -- speculative-exec --min-accuracy 0.6

# 协作 UI 渲染
npm run storybook -- --stories-file src/react/collaboration/

# 全量测试
npm test
```

---

## 全局风险与缓解

| 风险 | 影响 | 缓解措施 |
|------|------|---------|
| Go WASM 沙箱逃逸 | 安全 | wazero 编译期验证 + Fuel 计量 + 运行时监控 |
| 浏览器 Agent 模型体积过大 | UX | 量化模型 + 按需加载 + Service Worker 缓存 |
| MCP 协议版本变更 | 兼容 | 抽象传输层 + 版本协商 |
| Edge 平台 API 差异 | 维护 | 统一抽象层 + 平台检测 + 渐进降级 |
| 分层记忆蒸馏准确率 | 效果 | 人审机制 + 置信度阈值 + 可回滚 |

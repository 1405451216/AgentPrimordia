# AgentPrimordia Architecture Diagrams - Mermaid Format

## 1. System Architecture

```mermaid
graph TB
    subgraph CLI["CLI - cmd/ap"]
        Init["ap init"]
        Run["ap run"]
        Debug["ap debug"]
        Test["ap test"]
        MCP["ap mcp"]
    end

    subgraph Core["Go Engine - internal/"]
        subgraph AgentLayer["Agent Layer - internal/agent"]
            RA["ReActAgent"]
            RL["ReAct Loop"]
            Hooks["Hook System"]
            RAG["RAG Provider"]
        end

        subgraph LLMLayer["LLM Layer - internal/llm"]
            P["Provider Interface"]
            OAI["OpenAI"]
            ANT["Anthropic"]
            AZ["Azure"]
            GEM["Gemini"]
            OLL["Ollama"]
            RES["ResilientProvider"]
        end

        subgraph MemoryLayer["Memory Layer - internal/memory"]
            MI["Memory Interface"]
            SQL["SQLite"]
            MIL["Milvus"]
            QDR["Qdrant"]
        end

        subgraph ToolLayer["Tool Layer - internal/tools"]
            TI["Tool Interface"]
            FS["Filesystem"]
            API["API Call"]
            CMD["Shell"]
            MCP_T["MCP Tools"]
        end

        subgraph OrchLayer["Orchestration - internal/orchestration"]
            ORC["Orchestrator"]
            SEQ["Sequential"]
            PAR["Parallel"]
            DAG["DAG"]
        end
    end

    subgraph Operator["K8s Operator"]
        CRD["AgentDeployment CRD"]
        CTRL["Controller"]
        CM["ConfigMap"]
        DEP["Deployment"]
    end

    subgraph SDK["TypeScript SDK"]
        TSRA["ReActAgent"]
        TSP["OpenAI / Resilient"]
        TSM["InMemory / Vector / SQLite"]
        TST["ToolRegistry"]
        TSPool["AgentPool"]
    end

    CLI --> AgentLayer
    AgentLayer --> LLMLayer
    AgentLayer --> MemoryLayer
    AgentLayer --> ToolLayer
    OrchLayer --> AgentLayer
    Operator --> Core
    SDK --> TSP
    SDK --> TSM
```

---

## 2. ReAct Loop Execution Flow

```mermaid
flowchart TD
    Start([Initialize ReActAgent]) --> Think[Call LLM with tools + memory context]
    Think --> Parse[Parse LLM Response]

    Parse --> CheckTool{Has tool_calls?}

    CheckTool -- Yes --> SelectTool[Select Tool from Registry]
    SelectTool --> CheckPerm{Permission granted?}
    CheckPerm -- Yes --> ExecTool[Execute Tool in Sandbox]
    CheckPerm -- No --> Deny[Return permission denied]
    ExecTool --> Observe[Observe Result]
    Deny --> Observe
    Observe --> Record[Record Episode to Memory]
    Record --> Hook[Fire afterTool Hook]
    Hook --> CheckFail{Consecutive failures?}
    CheckFail -- Below threshold --> Think
    CheckFail -- At threshold --> ExitFail([Exit: too many failures])

    CheckTool -- No --> CheckDone{Max turns reached?}
    CheckDone -- No --> Think
    CheckDone -- Yes --> Complete([Return final response])
```

---

## 3. Core Data Structures

```mermaid
classDiagram
    class ReActAgent {
        +Provider provider
        +MemoryStore memory
        +ToolRegistry tools
        +HookManager hooks
        +int maxTurns
        +int consecutiveFailures
        +run(input) Response
        +stream(input) Channel~StreamEvent~
    }

    class Provider {
        <<interface>>
        +Complete(req) CompletionResponse
        +Stream(req) Channel~Chunk~
        +CallTools(req) ToolCallResponse
        +Embeddings(texts) Embeddings
    }

    class MemoryStore {
        <<interface>>
        +Add(episode) void
        +Get(id) Episode
        +Search(query, opts) Episode list
        +Delete(id) void
    }

    class Tool {
        <<interface>>
        +Name() string
        +Description() string
        +Parameters() Schema
        +Execute(input) Result
    }

    class AgentDeployment {
        +string Name
        +AgentDeploymentSpec Spec
        +AgentDeploymentStatus Status
    }

    class AgentDeploymentSpec {
        +int32 Replicas
        +AgentTemplateSpec Template
    }

    class AgentTemplateSpec {
        +string Provider
        +string Model
        +string Image
        +int32 MaxTurns
        +string SystemPrompt
        +ResourceSpec Resources
    }

    ReActAgent --> Provider : uses
    ReActAgent --> MemoryStore : uses
    ReActAgent --> Tool : registers
    AgentDeployment --> AgentDeploymentSpec : has
    AgentDeploymentSpec --> AgentTemplateSpec : contains
```

---

## 4. Kubernetes Operator Reconciliation

```mermaid
sequenceDiagram
    participant K8s as Kubernetes API
    participant CTRL as Controller
    participant CM as ConfigMap
    participant DEP as Deployment
    participant CRD as AgentDeployment CRD

    K8s->>CTRL: Watch AgentDeployment changes
    CTRL->>CRD: Get current state

    alt Deletion timestamp set
        CTRL->>CRD: Remove finalizer
        CTRL->>K8s: Update CRD
    else No finalizer
        CTRL->>CRD: Add finalizer
        CTRL->>K8s: Update CRD
    else Normal reconciliation
        CTRL->>CM: Ensure ConfigMap with ap.yaml
        CTRL->>DEP: Ensure Deployment with agent + metrics containers
        CTRL->>DEP: Get deployment status
        CTRL->>CRD: Update status (ActiveReplicas, CompletedTasks, ErrorRate)
    end
```

---

## 5. Resilient Provider Pattern

```mermaid
stateDiagram-v2
    [*] --> Closed
    Closed --> Open : failures >= threshold
    Open --> HalfOpen : recoverAfter elapsed
    HalfOpen --> Closed : probe succeeds
    HalfOpen --> Open : probe fails

    state Closed {
        [*] --> Execute
        Execute --> Retry : error + retries left
        Retry --> Execute : backoff elapsed
        Execute --> Fallback : error + no retries
        Execute --> Success : ok
    }

    state HalfOpen {
        [*] --> Probe
        Probe --> AllowFirst : halfOpenProbe=false
        Probe --> Reject : halfOpenProbe=true
    }
```

---

## 6. Multi-Agent Orchestration Modes

```mermaid
graph LR
    subgraph Sequential["Sequential Mode"]
        S1["Step 1"] --> S2["Step 2"] --> S3["Step 3"]
    end

    subgraph Parallel["Parallel Mode"]
        P1["Step 1"]
        P2["Step 2"]
        P3["Step 3"]
    end

    subgraph DAGMode["DAG Mode"]
        D1["Step 1"] --> D3["Step 3"]
        D2["Step 2"] --> D3
        D1 --> D4["Step 4"]
        D2 --> D4
    end
```

---

## 7. TypeScript SDK — Go Parity Architecture

```mermaid
graph TB
    subgraph TSApp["TypeScript Application"]
        TSI["import { ReActAgent, ... } from '@agentprimordia/sdk'"]
    end

    subgraph TSSDK["sdk/typescript/src/ — 24 Modules (100% Go Parity)"]
        subgraph TSAgent["agent/ — 12 files"]
            TSRA["ReActAgent"]
            TSCA["CapabilityAgent"]
            TSSE["Session"]
            TSHITL["HITLManager"]
            TSCOST["CostTracker"]
        end

        subgraph TSLLM["llm/ — 9 files"]
            TSOAI["OpenAIProvider"]
            TSANT["AnthropicProvider"]
            TSGEM["GeminiProvider"]
            TSOLL["OllamaProvider"]
            TSDS["DeepSeek / Qwen / GLM / Mistral / Cohere / Azure"]
            TSRES["ResilientProvider"]
            TSMM["MultimodalAdapter"]
            TSCACHE["InMemoryCache / CachedProvider"]
        end

        subgraph TSTools["tools/ — 7 files"]
            TSREG["ToolRegistry"]
            TSFS["FileSystemTool"]
            TSSH["ShellTool"]
            TSPLUGIN["PluginLoader"]
            TSDOC["PDF / DOCX / JSON / CSV Loaders"]
        end

        subgraph TSMem["memory/ — 5 files"]
            TSIN["InMemoryStore"]
            TSSQL["SqliteStore (FTS5)"]
            TSVEC["VectorStore / HNSW"]
            TSRAG["RAGStore / RAGPipeline"]
            TSMIL["MilvusProvider / QdrantProvider"]
        end

        subgraph TSInfra["Infrastructure — Phase 24"]
            TSAUDIT["AuditLogger"]
            TSADMIN["AdminHandler (Bearer Token + Web UI)"]
            TSDBG["Inspector / DebugServer"]
            TSPERSIST["SQLiteCheckpointStore"]
            TSHEALTH["HealthServer (/healthz /readyz /livez)"]
        end

        subgraph TSComm["Communication"]
            TSA2A["A2ABus / HTTPTransport / TCPTransport"]
            TSMCP["MCPClient / MCPRegistry / MCPAdapter"]
        end

        subgraph TSSec["Security"]
            TSACL["ACL / Sandbox"]
            TSGUARD["PIIDetector / InjectionDetector / GuardrailEngine"]
        end

        subgraph TSObs["Observability"]
            TSMET["MetricsCollector / PrometheusExporter"]
            TSOTEL["OTelTracer / OTLPExporter"]
        end
    end

    subgraph GoFW["Go Framework — agentprimordia/internal/"]
        GOAGENT["agent/"]
        GOLLM["llm/"]
        GOTOOLS["tools/"]
        GOMEM["memory/"]
        GOINFRA["audit/ admin/ debugger/ persist/ health/"]
        GONOTE["✅ Every Go module has a 1:1 TypeScript counterpart"]
    end

    TSI --> TSAgent
    TSAgent --> TSLLM
    TSAgent --> TSTools
    TSAgent --> TSMem
    TSAgent --> TSComm
    TSSec -.-> TSAgent
    TSObs -.-> TSAgent
    TSInfra -.-> TSAgent

    TSAgent -.->|"1:1 parity"| GOAGENT
    TSLLM -.->|"1:1 parity"| GOLLM
    TSTools -.->|"1:1 parity"| GOTOOLS
    TSMem -.->|"1:1 parity"| GOMEM
    TSInfra -.->|"1:1 parity"| GOINFRA
```

### Go ↔ TypeScript Module Mapping

```mermaid
graph LR
    subgraph GoModules["Go internal/"]
        G1["agent/"]
        G2["llm/"]
        G3["tools/"]
        G4["memory/"]
        G5["orchestration/"]
        G6["pool/"]
        G7["agent/a2a/"]
        G8["security/ + guardrail/"]
        G9["metrics/ + otel/"]
        G10["resilience/"]
        G11["prompt/"]
        G12["audit/"]
        G13["admin/"]
        G14["debugger/"]
        G15["persist/"]
        G16["health/"]
    end

    subgraph TSModules["TS src/"]
        T1["agent/"]
        T2["llm/"]
        T3["tools/"]
        T4["memory/"]
        T5["orchestration/"]
        T6["pool/"]
        T7["a2a/"]
        T8["security/"]
        T9["metrics/"]
        T10["resilience/"]
        T11["prompt/"]
        T12["audit/"]
        T13["admin/"]
        T14["debugger/"]
        T15["persist/"]
        T16["health/"]
    end

    G1 -.->|"✅"| T1
    G2 -.->|"✅"| T2
    G3 -.->|"✅"| T3
    G4 -.->|"✅"| T4
    G5 -.->|"✅"| T5
    G6 -.->|"✅"| T6
    G7 -.->|"✅"| T7
    G8 -.->|"✅"| T8
    G9 -.->|"✅"| T9
    G10 -.->|"✅"| T10
    G11 -.->|"✅"| T11
    G12 -.->|"✅"| T12
    G13 -.->|"✅"| T13
    G14 -.->|"✅"| T14
    G15 -.->|"✅"| T15
    G16 -.->|"✅"| T16
```

---

## Usage Instructions

### Preview in Markdown
- **VS Code**: Install "Markdown Preview Mermaid Support" extension
- **GitHub/Gitee**: Auto-render in .md files
- **Typora**: Native Mermaid support

### Export to Image
```bash
npm install -g @mermaid-js/mermaid-cli
mmdc -i architecture-mermaid.md -o architecture.png -w 2400 -H 1800
```

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

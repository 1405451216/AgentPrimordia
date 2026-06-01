# CodeCast Architecture Diagrams - Mermaid Format

## 1. System Architecture

```mermaid
graph TB
    subgraph Frontend["Frontend - React"]
        ChatCard["Chat Inline Card"]
        SidebarPanel["Sidebar Agents Panel"]
    end

    subgraph Backend["Backend - Go"]
        subgraph AgentPool["AgentPool - Semaphore 10"]
            SA1["SubAgent 1"]
            SA2["SubAgent 2"]
            SA3["SubAgent 3"]
            SAN["... more"]
        end

        subgraph ToolLayer["Tool Executor Layer"]
            TF["read_file"]
            TW["write_file"]
            TE["edit_file"]
            TR["run_command"]
            TS["search"]
            WF["web_fetch"]
        end

        subgraph Persistence["Persistence Layer"]
            Store["agents/session/agent.json"]
        end
    end

    subgraph API["DeepSeek API"]
        LLM["LLM Service - deepseek-v4-flash"]
    end

    ChatCard <-->|"Wails Events"| AgentPool
    SidebarPanel <-->|"Wails Events"| AgentPool

    SA1 --> ToolLayer
    SA2 --> ToolLayer
    SA3 --> ToolLayer
    SAN --> ToolLayer

    AgentPool --> Persistence
    AgentPool --> LLM
```

---

## 2. Agent Loop Execution Flow

```mermaid
flowchart TD
    Start([Initialize SubAgent]) --> LLMCall[Call LLM with tools]
    LLMCall --> ParseResp[Parse Response]

    ParseResp --> CheckTool{Has tool_calls?}

    CheckTool -- Yes --> ExecTool[Execute Tool]
    ExecTool --> ObserveResult[Observe Result]
    ObserveResult --> AppendMsg[Append Message]
    AppendMsg --> LLMCall

    CheckTool -- No --> TaskComplete([Task Complete])

    ParseResp --> CheckError{MaxTurns or Error?}
    CheckError -- Yes --> MarkFailed([Mark Failed])
```

---

## 3. Data Structure Relationship

```mermaid
classDiagram
    class SubAgent {
        +string ID
        +string SessionID
        +string ParentMsgID
        +string Title
        +string Prompt
        +FilesScope: list
        +Status: AgentStatus
        +Messages: AgentMessage list
        +string Result
        +string Error
        +int TurnCount
        +int MaxTurns
        +CreatedAt: time.Time
        +UpdatedAt: time.Time
        +Mode: AgentMode
        -ctx: context.Context
        -cancel: CancelFunc
    }

    class AgentMessage {
        +string Role
        +string Content
        +ToolCalls: ToolCall list
        +ToolResult: ToolResult ptr
    }

    class ToolCall {
        +string ID
        +string Name
        +string Args
    }

    class ToolResult {
        +string ToolCallID
        +string Content
        +bool IsError
    }

    class AgentPool {
        -mu: sync.Mutex
        -agents: SubAgent map
        -semaphore: chan struct
        -queue: SubAgent list
        -ctx: context.Context
        -cancel: CancelFunc
        -app: App ptr
    }

    class AgentEvent {
        +string AgentID
        +string Type
        +Status: AgentStatus
        +int Turn
        +int MaxTurns
        +string ToolName
        +string Message
    }

    SubAgent "1" --> "*" AgentMessage : contains
    SubAgent "1" --> "*" ToolCall : generates
    SubAgent "1" --> "*" ToolResult : receives
    AgentPool "1" --> "*" SubAgent : manages
    AgentPool ..> AgentEvent : emits
```

---

## 4. Component Interaction Sequence

```mermaid
sequenceDiagram
    participant User as User
    participant Main as MainAgent
    participant Pool as AgentPool
    participant SA as SubAgent
    participant LLM as DeepSeek API
    participant Tool as ToolExecutor
    participant FE as Frontend React

    User->>Main: Send complex task
    Main->>Main: Decompose task
    Main->>Pool: dispatch_agents tasks

    loop For each subtask
        Pool->>SA: Create and start goroutine
        Pool->>FE: Wails Event status running

        loop Agent Loop maxTurns 50
            SA->>LLM: Call LLM with tools
            LLM-->>SA: Return response

            alt Has tool_calls
                SA->>Tool: Execute tool
                Tool-->>SA: Return result
                SA->>SA: Append to history
                SA->>FE: Wails Event progress
            else Pure text response
                SA->>SA: Mark complete
                SA->>FE: Wails Event result
            end
        end

        Pool->>FE: Wails Event completed
    end

    Main-->>User: Summarize all results
```

---

## 5. File System Structure

```mermaid
graph LR
    Root["~/.codecast/"] --> Agents["agents/"]
    Agents --> Session["session_id/"]
    Session --> A1["agent_1.json"]
    Session --> A2["agent_2.json"]
    Session --> AN["..."]

    subgraph JSONStructure ["JSON Structure Example"]
        direction TB
        JID["id: string"]
        JStatus["status: enum"]
        JMessages["messages: array"]
        JResult["result: string"]
        JMeta["metadata..."]
    end

    A1 -.-> JSONStructure
```

---

## Usage Instructions

### Preview in Markdown
- **VS Code**: Install "Markdown Preview Mermaid Support" extension
- **GitHub/Gitee**: Auto-render in .md files
- **Typora**: Native Mermaid support

### Export to Image
```bash
# Using mmdc (Mermaid CLI)
npm install -g @mermaid-js/mermaid-cli
mmdc -i architecture-mermaid.md -o architecture.png -w 2400 -H 1800
```

### Online Editor
- [Mermaid Live Editor](https://mermaid.live/)

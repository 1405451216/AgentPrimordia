# Config API

> 配置热加载：.agent.yaml 文件监听 + 动态生效。

## 接口概览

```go
type Config struct {
    Name     string         `yaml:"name"`
    LLM      LLMConfig      `yaml:"llm"`
    Memory   MemoryConfig   `yaml:"memory"`
    Agent    AgentConfig    `yaml:"agent"`
    Tools    []string       `yaml:"tools"`
    Pool     PoolConfig     `yaml:"pool,omitempty"`
    Guardrail GuardrailConfig `yaml:"guardrail,omitempty"`
}

type Loader struct {
    path string
    mu   sync.RWMutex
    cfg  Config
}

func NewLoader(path string) (*Loader, error)
func (l *Loader) Watch(ctx context.Context) error    // 监听文件变更
func (l *Loader) Reload() error                       // 手动重载
func (l *Loader) Get() Config                        // 获取当前配置（线程安全）
func (l *Loader) OnChange(fn func(old, new Config))  // 注册变更回调
```

## 示例

```go
loader, _ := config.NewLoader(".ap.yaml")
go loader.Watch(ctx)  // 后台监听

loader.OnChange(func(old, new config.Config) {
    log.Printf("配置变更: %s -> %s", old.LLM.Model, new.LLM.Model)
    // 重建 Agent / 更新 Provider
})

// 当前配置（线程安全）
cfg := loader.Get()
```

## 配置项

| 字段 | 类型 | 说明 |
|------|------|------|
| `name` | string | 项目名称 |
| `llm.provider` | string | openai / anthropic / gemini / ollama 等 |
| `llm.model` | string | 模型名称 |
| `llm.api_key` | string | API Key（建议用 `${ENV_VAR}`） |
| `memory.backend` | string | sqlite / memory / vector |
| `memory.path` | string | SQLite 路径 |
| `agent.max_turns` | int | ReAct 最大轮次 |
| `agent.system_prompt` | string | 系统提示 |
| `tools[]` | []string | 启用的内置工具列表 |

## 环境变量

配置值支持 `${ENV_VAR}` 语法：

```yaml
llm:
  api_key: ${AP_LLM_API_KEY}
```

优先级：shell 环境变量 > .env 文件 > 默认值。

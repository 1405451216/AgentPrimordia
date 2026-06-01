# 新建对话模式选择功能 — 设计文档

> 日期：2026-05-24
> 状态：已批准，待实现

## 1. 需求概述

用户在点击「新建对话」按钮后，能够通过弹出面板自主选择**编程模式**或**日常对话模式**。每个会话独立记忆所选模式，使用对应不同的 System Prompt（`PromptCoding` / `PromptDaily`）。

## 2. 当前状态分析

| 组件 | 现状 |
|------|------|
| 新建入口 | 侧边栏「新建对话」按钮 → 重置状态 → Welcome 页面 |
| Session 创建 | 用户首次发消息时才真正调用 `api.createSession('新对话', '')` |
| 模式系统 | 后端已有全局 `Settings.WorkMode`（`coding` / `daily`），在设置页面修改 |
| Session 结构 | 前后端均**无 Mode 字段**，模式全局统一 |

## 3. 技术方案

### 3.1 数据模型变更

**后端 `Session` 结构体**（`session.go`）：

```go
type Session struct {
    ID        string
    Name      string
    CreatedAt time.Time
    SkillID   string
    Mode      string  // 新增: "" | "coding" | "daily"
    Messages  []Message
}
```

**前端 `Session` 接口**（`store/types.ts`）：

```typescript
export interface Session {
  ID: string;
  Name: string;
  CreatedAt: string;
  SkillID: string;
  Mode: 'coding' | 'daily' | '';   // 新增
  Messages: Message[];
}

export type SessionMode = 'coding' | 'daily';
export const DEFAULT_SESSION_MODE: SessionMode = 'daily';
```

**向后兼容**：已有会话的 `Mode` 为空字符串时，`buildSystemPrompt` fallback 到全局 `settings.WorkMode`。

### 3.2 后端改动

#### 3.2.1 `CreateSession` 签名扩展

```go
func (a *App) CreateSession(name, skillID, mode string) *Session {
    session := NewSession(name, skillID)
    session.Mode = mode
    a.sessions = append(a.sessions, session)
    return deepCopySession(session)
}
```

#### 3.2.2 `NewSession` 初始化

```go
func NewSession(name, skillID string) *Session {
    return &Session{
        ID:       fmt.Sprintf("session_%d", time.Now().UnixNano()),
        Name:     name,
        CreatedAt: time.Now(),
        SkillID:  skillID,
        Mode:     "",
        Messages: []Message{},
    }
}
```

#### 3.2.3 `buildSystemPrompt` 模式优先级

```go
func (a *App) buildSystemPrompt(session *Session) string {
    var systemPrompt string

    effectiveMode := session.Mode
    if effectiveMode == "" {
        effectiveMode = a.settings.WorkMode  // fallback 到全局设置
    }

    if effectiveMode == "coding" {
        systemPrompt = PromptCoding
    } else {
        systemPrompt = PromptDaily
    }
    // ... 后续人格、自定义指令等拼接逻辑不变
}
```

优先级链路：`session.Mode` → `settings.WorkMode` → 默认 `"daily"`

### 3.3 前端改动

#### 3.3.1 API 层（`api.ts`）

```typescript
// GoAppMethods 接口
CreateSession(name: string, skillId: string, mode?: string): Promise<GoSession>;

// 导出函数
export const createSession = (name: string, skillId: string, mode?: string) =>
  callGo('CreateSession', name, skillId, mode ?? '');
```

#### 3.3.2 新建组件 `ModeSelector.tsx`

**交互流程**：
1. 点击侧边栏「新建对话」按钮
2. 弹出居中模态面板，展示两张模式卡片
3. 用户选择一张卡片（click 或 Enter）
4. 面板关闭，以选中模式创建会话并进入 Welcome/Chat 视图
5. 点击遮罩层或按 Esc 可取消关闭

**UI 布局**：

```
┌──────────────────────────────────────┐
│  （遮罩层 backdrop-blur）              │
│  ┌────────────────────────────────┐  │
│  │  选择对话模式           ✕      │  │
│  ├───────────────────┬────────────┤  │
│  │                   │            │  │
│  │   💻 编程模式      │  💬 日常对话 │  │
│  │                   │            │  │
│  │  代码生成、调试、   │  写作、问答、 │  │
│  │  文件操作、Git     │  翻译、分析  │  │
│  │                   │            │  │
│  └───────────────────┴────────────┘  │
└──────────────────────────────────────┘
```

- 卡片 hover 状态：边框高亮 + 轻微上浮
- 选中状态：主题色填充背景
- 支持键盘导航（Tab 切换、Enter 确认、Esc 关闭）

#### 3.3.3 `useChatSender` 改动

```typescript
// 创建 session 时传入 pendingMode（来自 ModeSelector 的选择结果）
const session = await api.createSession('新对话', '', pendingMode);
```

#### 3.3.4 `useSessionActions` + `Sidebar` 改动

- `handleNewSession` 不再直接重置视图，而是打开 ModeSelector 弹窗
- 新增 state：`showModeSelector: boolean` 和 `pendingMode: SessionMode | null`
- ModeSelector 关闭回调触发实际的 session 创建逻辑

#### 3.3.5 侧边栏会话列表（可选增强）

在会话名称旁显示小图标区分模式：
- `Mode === 'coding'` → 💻 图标
- `Mode === 'daily'` → 💬 图标
- `Mode === ''` → 不显示图标（历史兼容）

### 3.4 文件变更清单

| 文件 | 变更类型 | 说明 |
|------|----------|------|
| `CodeCast-desktop/session.go` | 修改 | Session 结构体新增 Mode 字段；CreateSession 签名扩展；buildSystemPrompt 优先级调整 |
| `CodeCast-desktop/frontend/src/store/types.ts` | 修改 | Session 接口新增 Mode 字段；新增 SessionMode 类型 |
| `CodeCast-desktop/frontend/src/api/types.ts` | 修改 | GoSession 类型映射新增 Mode |
| `CodeCast-desktop/frontend/src/api.ts` | 修改 | CreateSession 调用签名扩展 |
| `CodeCast-desktop/frontend/src/components/ModeSelector.tsx` | **新建** | 模式选择弹窗组件 |
| `CodeCast-desktop/frontend/src/components/Sidebar.tsx` | 修改 | 新建按钮绑定 ModeSelector 弹窗 |
| `CodeCast-desktop/frontend/src/hooks/useSessionActions.ts` | 修改 | handleNewSession 逻辑调整 |
| `CodeCast-desktop/frontend/src/hooks/useChatSender.ts` | 修改 | createSession 调用传入 mode |
| `CodeCast-desktop/frontend/src/styles/mode-selector.css` | **新建** | ModeSelector 样式 |

## 4. 边界情况处理

| 场景 | 处理方式 |
|------|----------|
| 已有历史会话（Mode 为空） | fallback 到全局 `settings.WorkMode` |
| 用户点击遮罩层关闭弹窗 | 不创建会话，保持在当前状态 |
| 快速连续点击新建按钮 | 防抖/锁，避免重复弹窗 |
| Wails 类型声明更新 | 运行 `wails generate module` 或手动更新 `wailsjs/go/main/App.d.ts` |

## 5. 验收标准

- [ ] 点击「新建对话」弹出模式选择面板
- [ ] 面板展示编程模式和日常对话两个选项卡片
- [ ] 选择编程模式后，新会话使用 PromptCoding
- [ ] 选择日常对话后，新会话使用 PromptDaily
- [ ] 已有会话不受影响，切换回来仍保持原有行为
- [ ] 支持 Esc 关闭、点击遮罩关闭
- [ ] 支持键盘导航（Tab/Enter）

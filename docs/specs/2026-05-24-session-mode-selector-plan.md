# 新建对话模式选择功能 — 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在点击「新建对话」按钮后弹出模式选择面板，让用户自主选择编程模式或日常对话模式，每个会话独立记忆所选模式。

**Architecture:** 后端 Session 结构体新增 Mode 字段，CreateSession 签名扩展接收 mode 参数，buildSystemPrompt 优先使用 session.Mode；前端新建 ModeSelector 弹窗组件，通过 Zustand store 管理待选模式状态，创建会话时传入 mode。

**Tech Stack:** Go 1.25 (Wails v2), React 18, TypeScript, Vite 5, Zustand 4

---

### Task 1: 后端 Session 结构体新增 Mode 字段

**Files:**
- Modify: `CodeCast-desktop/session.go:49-55` (Session struct)
- Modify: `CodeCast-desktop/session.go:57-65` (NewSession function)

- [ ] **Step 1: 修改 Session 结构体，新增 Mode 字段**

在 `session.go` 的 `Session` 结构体中，在 `SkillID` 之后添加 `Mode` 字段：

```go
type Session struct {
	ID        string
	Name      string
	CreatedAt time.Time
	SkillID   string
	Mode      string // "" | "coding" | "daily"
	Messages  []Message
}
```

- [ ] **Step 2: 修改 NewSession 函数，初始化 Mode 为空字符串**

将 `NewSession` 函数更新为：

```go
func NewSession(name, skillID string) *Session {
	return &Session{
		ID:        fmt.Sprintf("session_%d", time.Now().UnixNano()),
		Name:      name,
		CreatedAt: time.Now(),
		SkillID:   skillID,
		Mode:      "",
		Messages:  []Message{},
	}
}
```

- [ ] **Step 3: 验证编译通过**

Run: `cd CodeCast-desktop && go build ./...`
Expected: 编译成功，无错误

---

### Task 2: 后端 CreateSession 签名扩展 + buildSystemPrompt 模式优先级

**Files:**
- Modify: `CodeCast-desktop/session.go:182-189` (CreateSession)
- Modify: `CodeCast-desktop/session.go:303-310` (buildSystemPrompt)

- [ ] **Step 1: 扩展 CreateSession 接收 mode 参数**

```go
func (a *App) CreateSession(name, skillID, mode string) *Session {
	a.mu.Lock()
	defer a.mu.Unlock()

	session := NewSession(name, skillID)
	session.Mode = mode
	a.sessions = append(a.sessions, session)
	return deepCopySession(session)
}
```

- [ ] **Step 2: 修改 buildSystemPrompt 使用 session.Mode 优先**

将原有的：
```go
if a.settings.WorkMode == "coding" {
```
替换为：
```go
effectiveMode := session.Mode
if effectiveMode == "" {
	effectiveMode = a.settings.WorkMode
}

if effectiveMode == "coding" {
```

完整修改后的 `buildSystemPrompt` 开头部分：

```go
func (a *App) buildSystemPrompt(session *Session) string {
	var systemPrompt string

	effectiveMode := session.Mode
	if effectiveMode == "" {
		effectiveMode = a.settings.WorkMode
	}

	if effectiveMode == "coding" {
		systemPrompt = PromptCoding
	} else {
		systemPrompt = PromptDaily
	}
	// ... 后续代码不变
```

- [ ] **Step 3: 验证编译通过**

Run: `cd CodeCast-desktop && go build ./...`
Expected: 编译成功

---

### Task 3: 前端类型定义更新

**Files:**
- Modify: `CodeCast-desktop/frontend/src/store/types.ts` (Session interface)
- Modify: `CodeCast-desktop/frontend/src/api/types.ts` (GoSession interface + toSession)

- [ ] **Step 1: 在 types.ts 的 Session 接口中新增 Mode 字段和类型导出**

在 `types.ts` 的 `Session` 接口中添加 `Mode` 属性，并在文件末尾添加类型导出：

```typescript
export interface Session {
  ID: string;
  Name: string;
  CreatedAt: string;
  SkillID: string;
  Mode: 'coding' | 'daily' | '';
  Messages: Message[];
}

export type SessionMode = 'coding' | 'daily';
export const DEFAULT_SESSION_MODE: SessionMode = 'daily';
```

- [ ] **Step 2: 在 api/types.ts 中更新 GoSession 和 toSession**

更新 `GoSession` 接口：
```typescript
export interface GoSession {
  ID: string;
  Name: string;
  CreatedAt: string;
  SkillID: string;
  Mode: string;
  Messages: GoMessage[];
}
```

更新 `toSession` 函数：
```typescript
export function toSession(gs: GoSession): import('../store/types').Session {
  return {
    ID: gs.ID,
    Name: gs.Name,
    CreatedAt: gs.CreatedAt,
    SkillID: gs.SkillID,
    Mode: (gs.Mode as 'coding' | 'daily' | '') || '',
    Messages: (gs.Messages || []).map(toMessage),
  };
}
```

- [ ] **Step 3: 验证 TypeScript 编译**

Run: `cd CodeCast-desktop/frontend && npx tsc --noEmit`
Expected: 无类型错误

---

### Task 4: 前端 API 层 + Wails 类型声明更新

**Files:**
- Modify: `CodeCast-desktop/frontend/src/api.ts` (CreateSession 签名)
- Modify: `CodeCast-desktop/frontend/wailsjs/go/main/App.d.ts` (CreateSession 签名)

- [ ] **Step 1: 更新 api.ts 中 CreateSession 的接口定义和导出函数**

在 `GoAppMethods` 接口中更新：
```typescript
CreateSession(name: string, skillId: string, mode?: string): Promise<GoSession>;
```

更新导出函数：
```typescript
export const createSession = (name: string, skillId: string, mode?: string) =>
  callGo('CreateSession', name, skillId, mode ?? '');
```

- [ ] **Step 2: 更新 Wails 自动生成的 App.d.ts 类型声明**

将：
```typescript
export function CreateSession(arg1:string,arg2:string):Promise<main.Session>;
```
改为：
```typescript
export function CreateSession(arg1:string,arg2:string,arg3?:string):Promise<main.Session>;
```

- [ ] **Step 3: 验证 TypeScript 编译**

Run: `cd CodeCast-desktop/frontend && npx tsc --noEmit`
Expected: 无类型错误

---

### Task 5: UI Store 新增模式选择器状态

**Files:**
- Modify: `CodeCast-desktop/frontend/src/store/useUIStore.ts`

- [ ] **Step 1: 在 UISlice 中新增 showModeSelector 和 pendingMode 状态**

在 `UISlice` 接口中添加：
```typescript
interface UISlice {
  // ... 已有字段 ...
  showModeSelector: boolean;
  setShowModeSelector: (show: boolean) => void;
  pendingMode: SessionMode | null;
  setPendingMode: (mode: SessionMode | null) => void;
}
```

注意：需要在文件顶部导入 `SessionMode` 类型（从 `'./types'` 导入，但需避免循环依赖，实际导入路径为 `'../types'`）。

在 `createUISlice` 返回值中添加：
```typescript
showModeSelector: false,
setShowModeSelector: (show) => set({ showModeSelector: show }),
pendingMode: null,
setPendingMode: (mode) => set({ pendingMode: mode }),
```

- [ ] **Step 2: 验证 TypeScript 编译**

Run: `cd CodeCast-desktop/frontend && npx tsc --noEmit`
Expected: 无类型错误

---

### Task 6: 创建 ModeSelector 弹窗组件

**Files:**
- Create: `CodeCast-desktop/frontend/src/components/ModeSelector.tsx`
- Create: `CodeCast-desktop/frontend/src/styles/mode-selector.css`

- [ ] **Step 1: 创建 ModeSelector.tsx 组件**

```tsx
import React, { useEffect, useRef, useCallback } from 'react';
import { useAppStore } from '../store';
import type { SessionMode } from '../store/types';
import '../styles/mode-selector.css';

interface ModeSelectorProps {
  onSelect: (mode: SessionMode) => void;
  onClose: () => void;
}

const MODES: { key: SessionMode; icon: string; title: string; desc: string }[] = [
  {
    key: 'coding',
    icon: '💻',
    title: '编程模式',
    desc: '代码生成、调试、文件操作、Git',
  },
  {
    key: 'daily',
    icon: '💬',
    title: '日常对话',
    desc: '写作、问答、翻译、分析',
  },
];

const ModeSelector: React.FC<ModeSelectorProps> = ({ onSelect, onClose }) => {
  const overlayRef = useRef<HTMLDivElement>(null);
  const [activeIndex, setActiveIndex] = React.useState(0);

  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault();
        onClose();
      }
      if (e.key === 'Enter') {
        e.preventDefault();
        onSelect(MODES[activeIndex].key);
      }
      if (e.key === 'Tab') {
        e.preventDefault();
        setActiveIndex((prev) => (prev + 1) % MODES.length);
      }
    },
    [activeIndex, onSelect, onClose]
  );

  useEffect(() => {
    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [handleKeyDown]);

  return (
    <div className="mode-selector-overlay" ref={overlayRef} onClick={(e) => { if (e.target === overlayRef.current) onClose(); }}>
      <div className="mode-selector-box">
        <div className="mode-selector-header">
          <span className="mode-selector-title">选择对话模式</span>
          <button className="mode-selector-close" onClick={onClose}>✕</button>
        </div>
        <div className="mode-selector-grid">
          {MODES.map((mode, i) => (
            <div
              key={mode.key}
              className={`mode-card ${i === activeIndex ? 'active' : ''}`}
              onClick={() => onSelect(mode.key)}
              onMouseEnter={() => setActiveIndex(i)}
            >
              <span className="mode-card-icon">{mode.icon}</span>
              <span className="mode-card-title">{mode.title}</span>
              <span className="mode-card-desc">{mode.desc}</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
};

export default React.memo(ModeSelector);
```

- [ ] **Step 2: 创建 mode-selector.css 样式文件**

```css
.mode-selector-overlay {
  position: fixed;
  inset: 0;
  z-index: 10000;
  background: rgba(0,0,0,0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  animation: fade-in 0.15s ease-out;
}

.mode-selector-box {
  background: var(--bg2);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  padding: 28px;
  min-width: 420px;
  max-width: 520px;
  box-shadow: var(--shadow-lg);
  animation: dialog-in 0.2s ease-out;
}

.mode-selector-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 20px;
}

.mode-selector-title {
  font-size: 16px;
  font-weight: 600;
  color: var(--text);
}

.mode-selector-close {
  background: none;
  border: none;
  color: var(--text-muted);
  font-size: 18px;
  cursor: pointer;
  padding: 4px 8px;
  border-radius: var(--radius-sm);
  transition: all 0.15s;
}

.mode-selector-close:hover {
  color: var(--text);
  background: var(--glass-hover);
}

.mode-selector-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 14px;
}

.mode-card {
  background: var(--bg);
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  padding: 24px 18px;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  transition: all 0.18s ease;
}

.mode-card:hover {
  border-color: var(--accent);
  transform: translateY(-2px);
  box-shadow: var(--shadow-glow);
}

.mode-card.active {
  border-color: var(--accent);
  background: var(--gradient-accent-subtle);
}

.mode-card-icon {
  font-size: 32px;
  line-height: 1;
}

.mode-card-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--text);
}

.mode-card-desc {
  font-size: 12px;
  color: var(--text-dim);
  text-align: center;
  line-height: 1.4;
}
```

- [ ] **Step 3: 在 index.css 或 main.css 中引入新样式**

在 `styles/index.css` 中添加：
```css
@import './mode-selector.css';
```

- [ ] **Step 4: 验证 TypeScript 编译**

Run: `cd CodeCast-desktop/frontend && npx tsc --noEmit`
Expected: 无类型错误

---

### Task 7: 集成 ModeSelector 到 Sidebar 和 App

**Files:**
- Modify: `CodeCast-desktop/frontend/src/components/Sidebar.tsx`
- Modify: `CodeCast-desktop/frontend/src/hooks/useSessionActions.ts`
- Modify: `CodeCast-desktop/frontend/src/App.tsx`
- Modify: `CodeCast-desktop/frontend/src/hooks/useChatSender.ts`

- [ ] **Step 1: 修改 useSessionActions — handleNewSession 打开模式选择器**

在 `useSessionActions.ts` 中：

```typescript
import { useCallback } from 'react';
import { useAppStore, Session, AppState } from '../store';
import * as api from '../api';
import { toMessage } from '../api/types';

interface UseSessionActionsReturn {
  handleNewSession: () => void;
  handleSelectSession: (id: string) => Promise<void>;
  handleDeleteSession: (id: string) => Promise<void>;
  handleClearSession: () => void;
  handleModeSelect: (mode: 'coding' | 'daily') => void;
}

export function useSessionActions(): UseSessionActionsReturn {
  const sessions = useAppStore((s: AppState) => s.sessions);
  const currentSessionId = useAppStore((s: AppState) => s.currentSessionId);
  const setCurrentSessionId = useAppStore((s: AppState) => s.setCurrentSessionId);
  const addSession = useAppStore((s: AppState) => s.addSession);
  const removeSession = useAppStore((s: AppState) => s.removeSession);
  const setAttachments = useAppStore((s: AppState) => s.setAttachments);
  const setActivePanel = useAppStore((s: AppState) => s.setActivePanel);
  const setMessages = useAppStore((s: AppState) => s.setMessages);
  const clearMessages = useAppStore((s: AppState) => s.clearMessages);
  const setTitle = useAppStore((s: AppState) => s.setTitle);
  const setView = useAppStore((s: AppState) => s.setView);
  const setShowModeSelector = useAppStore((s: AppState) => s.setShowModeSelector);
  const setPendingMode = useAppStore((s: AppState) => s.setPendingMode);

  const handleNewSession = useCallback(() => {
    setShowModeSelector(true);
  }, [setShowModeSelector]);

  const handleModeSelect = useCallback((mode: 'coding' | 'daily') => {
    setPendingMode(mode);
    setShowModeSelector(false);
    setCurrentSessionId(null);
    setAttachments([]);
    clearMessages();
    setTitle('CodeCast');
    setView('welcome');
    setActivePanel(null);
  }, [setCurrentSessionId, setAttachments, setActivePanel, clearMessages, setTitle, setView, setShowModeSelector, setPendingMode]);

  // ... handleSelectSession, handleDeleteSession, handleClearSession 保持不变 ...

  return { handleNewSession, handleSelectSession, handleDeleteSession, handleClearSession, handleModeSelect };
}
```

- [ ] **Step 2: 修改 App.tsx — 渲染 ModeSelector 并传递回调**

在 `App.tsx` 中：
1. 从 `useSessionActions` 解构出 `handleModeSelect`
2. 从 store 获取 `showModeSelector` 和 `setShowModeSelector`
3. 在 JSX 中渲染 `<ModeSelector>`

```tsx
const showModeSelector = useAppStore((s: AppState) => s.showModeSelector);
const setShowModeSelector = useAppStore((s: AppState) => s.setShowModeSelector);

// 在 return 的 JSX 中，<SettingsPage /> 之前添加：
{showModeSelector && (
  <ModeSelector
    onSelect={handleModeSelect}
    onClose={() => setShowModeSelector(false)}
  />
)}
```

同时在文件顶部添加 import：
```tsx
import ModeSelector from './components/ModeSelector';
```

- [ ] **Step 3: 修改 useChatSender — 创建 session 时传入 pendingMode**

在 `useChatSender.ts` 中：

```typescript
const pendingMode = useAppStore((s: AppState) => s.pendingMode);
```

然后修改 createSession 调用：
```typescript
const session = await api.createSession('新对话', '', pendingMode ?? '');
```

需要在 `useCallback` 的依赖数组中添加 `pendingMode`。

完整修改后的关键片段：
```typescript
const pendingMode = useAppStore((s: AppState) => s.pendingMode);

const handleSendMessage = useCallback(async (text: string) => {
  if (isLoading) return;

  let sessionId: string | null = currentSessionId;

  if (!sessionId) {
    try {
      const session = await api.createSession('新对话', '', pendingMode ?? '');
      sessionId = session.ID;
      setCurrentSessionId(sessionId);
      addSession(toSession(session));
    } catch (e) {
      console.error('Create session failed:', e);
      return;
    }
  }
  // ... 后续不变
}, [isLoading, currentSessionId, selectedModel, thinkingMode, pendingMode, setCurrentSessionId, addSession, addMessage, updateLastMessage, setView, setTitle, setIsLoading]);
```

- [ ] **Step 4: 验证 TypeScript 编译和构建**

Run: `cd CodeCast-desktop/frontend && npx tsc --noEmit && npm run build`
Expected: 无错误，构建成功

---

### Task 8: 可选增强 — 侧边栏会话列表显示模式图标

**Files:**
- Modify: `CodeCast-desktop/frontend/src/components/Sidebar.tsx`

- [ ] **Step 1: 在侧边栏会话项中根据 Mode 显示图标**

在 Sidebar 的 session 列表渲染中，每个 item 的 label 前根据 `s.Mode` 添加小图标：

```tsx
<span className="item-label">
  {s.Mode === 'coding' ? '💻 ' : s.Mode === 'daily' ? '💬 ' : ''}
  {s.Name}
</span>
```

- [ ] **Step 2: 验证构建**

Run: `cd CodeCast-desktop/frontend && npm run build`
Expected: 构建成功

---

### Task 9: 端到端验证

- [ ] **Step 1: 启动开发服务器验证功能**

Run: `cd CodeCast-desktop && wails dev`
手动验证清单：
- [ ] 点击「新建对话」弹出模式选择面板
- [ ] 面板展示两张卡片：编程模式 / 日常对话
- [ ] hover 卡片有高亮效果
- [ ] 点击编程模式卡片 → 关闭面板 → 进入 Welcome 页面 → 发送消息 → AI 使用编程风格回复
- [ ] 点击日常对话卡片 → 关闭面板 → 进入 Welcome 页面 → 发送消息 → AI 使用日常对话风格回复
- [ ] 按 Esc 可关闭面板
- [ ] 点击遮罩层可关闭面板
- [ ] Tab 键可在两个卡片间切换焦点样式
- [ ] Enter 键确认选中的模式
- [ ] 已有历史会话切换回来仍正常工作（fallback 到全局 WorkMode）

/**
 * AgentPrimordia Chat Panel — 侧边栏 WebviewViewProvider。
 *
 * 职责：
 * 1. 在 VS Code 侧边栏提供持久化聊天界面
 * 2. 通过 StudioApi 与 Agent 通信，渲染 Markdown 响应
 * 3. 支持 SSE 流式 token 实时展示
 *
 * 设计要点：
 * - WebviewViewProvider 模式（侧边栏面板，非独立 tab）
 * - 消息历史保存在视图状态中，跨 session 持久化
 * - 与 vscode API 解耦，便于 stub 测试
 */

import type { StudioApi, StreamEvent } from './studioApi.js';

/** 聊天消息角色 */
export type ChatRole = 'user' | 'assistant' | 'system';

/** 单条聊天消息 */
export interface ChatMessage {
  role: ChatRole;
  content: string;
  /** 消息时间戳 */
  timestamp: number;
}

/** 视图对外暴露的 API（供 extension.ts 注入） */
export interface ChatPanelDeps {
  api: StudioApi;
  /** 当前默认模板名 */
  defaultTemplate: string;
  /** 持久化消息历史 */
  loadHistory(): ChatMessage[];
  saveHistory(msgs: ChatMessage[]): void;
}

/** 渲染聊天 HTML（前端逻辑最小化，仅展示 + 输入框） */
function renderChatHtml(messages: ChatMessage[]): string {
  const msgsJson = JSON.stringify(messages).replace(/</g, '\\u003c');
  return `<!doctype html>
<html><head><meta charset="utf-8"><style>
body{font-family:var(--vscode-font-family);font-size:var(--vscode-font-size);padding:8px}
#messages{display:flex;flex-direction:column;gap:6px;margin-bottom:8px}
.msg{padding:6px 8px;border-radius:4px;white-space:pre-wrap;word-break:break-word;word-wrap:break-word;overflow-wrap:break-word}
.msg.user{background:var(--vscode-textCodeBlock-background)}
.msg.assistant{background:var(--vscode-editor-background);border:1px solid var(--vscode-panel-border)}
.msg .role{font-weight:bold;font-size:.85em;color:var(--vscode-descriptionForeground)}
#input-row{display:flex;gap:4px}
#input{flex:1;background:var(--vscode-input-background);color:var(--vscode-input-foreground);border:1px solid var(--vscode-input-border);padding:4px 6px;border-radius:2px}
#send{background:var(--vscode-button-background);color:var(--vscode-button-foreground);border:none;padding:4px 12px;border-radius:2px;cursor:pointer}
#streaming{color:var(--vscode-descriptionForeground);font-style:italic;display:none}
</style></head><body>
<div id="messages"></div><div id="streaming">...</div>
<div id="input-row"><input id="input" placeholder="输入消息，Enter 发送 / Shift+Enter 换行"/><button id="send">发送</button></div>
<script>
const vscode=acquireVsCodeApi();
const initialMsgs=${msgsJson};
const messagesEl=document.getElementById('messages');
const inputEl=document.getElementById('input');
const sendEl=document.getElementById('send');
const streamingEl=document.getElementById('streaming');
function renderMsgs(msgs){messagesEl.innerHTML='';msgs.forEach(m=>{const d=document.createElement('div');d.className='msg '+m.role;d.innerHTML='<div class="role">'+m.role+'</div>'+m.content;messagesEl.appendChild(d)});messagesEl.scrollTop=messagesEl.scrollHeight}
renderMsgs(initialMsgs);
sendEl.addEventListener('click',()=>send());
inputEl.addEventListener('keydown',e=>{if(e.key==='Enter'&&!e.shiftKey){e.preventDefault();send()}});
function send(){const t=inputEl.value.trim();if(!t)return;inputEl.value='';streamingEl.style.display='block';vscode.postMessage({type:'send',text:t})}
window.addEventListener('message',ev=>{const m=ev.data;if(m.type==='messages'){streamingEl.style.display='none';renderMsgs(m.messages)}else if(m.type==='streamChunk'){streamingEl.textContent=m.text;streamingEl.style.display='block'}else if(m.type==='streamEnd'){streamingEl.style.display='none'}});
</script></body></html>`;
}

/** 聊天面板提供者 — 实现 WebviewViewProvider 接口 */
export class ChatPanelProvider {
  static readonly viewId = 'agentprimordia.chat';

  private view: any = null;
  private history: ChatMessage[] = [];
  private streamingAbort: AbortController | null = null;

  constructor(private readonly deps: ChatPanelDeps) {
    this.history = deps.loadHistory();
  }

  /** VS Code 解析视图时调用 */
  resolveWebviewView(webviewView: any): void {
    this.view = webviewView;
    const wv = webviewView.webview;
    wv.options = { enableScripts: true, localResourceRoots: [] };
    wv.html = renderChatHtml(this.history);

    wv.onDidReceiveMessage((msg: unknown) => {
      const m = msg as { type: string; text?: string };
      if (m.type === 'send' && m.text) {
        void this.handleSend(m.text);
      }
    });
  }

  /** 处理用户发送消息 */
  private async handleSend(text: string): Promise<void> {
    const userMsg: ChatMessage = { role: 'user', content: text, timestamp: Date.now() };
    this.history.push(userMsg);
    this.persist();
    this.postMessages();

    const assistantText = await this.runAgent(text);
    const assistantMsg: ChatMessage = {
      role: 'assistant',
      content: assistantText || '(无响应)',
      timestamp: Date.now(),
    };
    this.history.push(assistantMsg);
    this.persist();
    this.postMessages();
  }

  /** 通过 StudioApi 发起运行并流式接收响应 */
  private async runAgent(message: string): Promise<string> {
    const { api, defaultTemplate } = this.deps;
    try {
      const run = await api.startRun(defaultTemplate, message);
      return await this.consumeStream(run.id);
    } catch (err) {
      return `错误: ${err instanceof Error ? err.message : String(err)}`;
    }
  }

  /** 消费 SSE 流，拼接文本并实时推送 chunk 到视图 */
  private async consumeStream(runId: string): Promise<string> {
    const { api } = this.deps;
    let fullText = '';
    this.streamingAbort = new AbortController();

    try {
      const res = await api.streamRun(runId);
      const reader = res.body?.getReader();
      if (!reader) return fullText;

      const decoder = new TextDecoder();
      let buf = '';
      while (true) {
        const { value, done } = await reader.read();
        if (done) break;
        buf += decoder.decode(value, { stream: true });
        const events = this.parseSse(buf);
        buf = events.rest;
        for (const evt of events.list) {
          if (evt.type === 'token' && evt.text) {
            fullText += evt.text;
            this.postStreamChunk(fullText);
          } else if (evt.type === 'done') {
            break;
          } else if (evt.type === 'error') {
            fullText += `\n\n[错误: ${evt.text ?? 'unknown'}]`;
            break;
          }
        }
      }
    } catch (err) {
      fullText += `\n\n[流中断: ${err instanceof Error ? err.message : String(err)}]`;
    } finally {
      this.streamingAbort = null;
      this.postStreamEnd();
    }
    return fullText;
  }

  /** 解析 SSE 缓冲区，返回已解析事件与剩余缓冲 */
  private parseSse(buf: string): { list: StreamEvent[]; rest: string } {
    const list: StreamEvent[] = [];
    const parts = buf.split('\n\n');
    const rest = parts.pop() ?? '';
    for (const block of parts) {
      const lines = block.split('\n');
      for (const line of lines) {
        if (line.startsWith('data: ')) {
          try {
            list.push(JSON.parse(line.slice(6)) as StreamEvent);
          } catch {
            // 忽略解析失败行
          }
        }
      }
    }
    return { list, rest };
  }

  /** 推送消息快照到视图 */
  private postMessages(): void {
    this.view?.webview.postMessage({ type: 'messages', messages: this.history });
  }

  /** 推送流式 chunk（当前累积文本） */
  private postStreamChunk(text: string): void {
    this.view?.webview.postMessage({ type: 'streamChunk', text });
  }

  /** 推送流结束信号 */
  private postStreamEnd(): void {
    this.view?.webview.postMessage({ type: 'streamEnd' });
  }

  /** 持久化消息历史 */
  private persist(): void {
    this.deps.saveHistory(this.history);
  }

  /** 清理流式资源 */
  dispose(): void {
    this.streamingAbort?.abort();
    this.streamingAbort = null;
  }
}

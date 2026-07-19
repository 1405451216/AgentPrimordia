/**
 * VS Code 扩展入口。
 *
 * 职责：
 * 1. 注册命令：inspect / run / debug / stop
 * 2. 注册调试配置提供者
 * 3. 把 .ap.yaml 解析 + Inspector 状态机接入 VS Code Webview
 *
 * 设计要点：
 * - 业务逻辑（inspector.ts / debugger.ts / format.ts）与 vscode API
 *   完全解耦，便于 Node 环境单测。
 * - 通过动态 import `vscode` 包并在缺失时安全降级，让本文件也能在
 *   普通 Node 进程中被静态分析或测试桩注入。
 */

import { createInspectorState, applyCommand, applyStep, serializeState } from './inspector.js';
import {
  parseYamlLite,
  normalizeApConfig,
  toDebugConfig,
  validateDebugConfig,
  buildLaunchJson,
  pickApYamlFilename,
  generateLaunchJsonTemplate,
} from './debugger.js';
import { formatStepHistory, formatStateSummary, toWebviewPayload } from './format.js';
import type { AgentDebugConfig, InspectorState } from './types.js';

// vscode 模块是动态导入的：在 VS Code 运行时由宿主注入；在 Node 测试环境
// 可能不存在，使用 require 兼容方案。
let vscode: any = null;
try {
  // eslint-disable-next-line @typescript-eslint/no-var-requires
  vscode = require('vscode');
} catch {
  vscode = null;
}

/** 扩展上下文存储 */
interface ExtensionState {
  state: InspectorState;
  output: { append: (text: string) => void; show: () => void } | null;
}

const STATE: ExtensionState = {
  state: createInspectorState(),
  output: null,
};

/** 激活扩展（在 activationEvents 触发时由 VS Code 调用） */
export function activate(context: { subscriptions: unknown[] }): void {
  if (!vscode) {
    // 非 VS Code 环境（测试或桩），仅记录激活信息
    return;
  }

  // 注册命令：inspect
  const inspectCmd = vscode.commands.registerCommand('agentprimordia.inspect', () => {
    showInspectorPanel(context);
  });
  context.subscriptions.push(inspectCmd);

  // 注册命令：run
  const runCmd = vscode.commands.registerCommand('agentprimordia.run', async () => {
    const prompt = await vscode.window.showInputBox({
      prompt: 'AgentPrimordia: 输入 prompt',
      placeHolder: '请输入',
    });
    if (!prompt) return;
    const maxTurns = vscode.workspace
      .getConfiguration('agentprimordia')
      .get('maxTurns', 10) as number;
    const result = applyCommand(STATE.state, {
      type: 'start',
      prompt,
      maxTurns,
    });
    STATE.state = result.state;
    log(`Inspector 启动: "${prompt}" (maxTurns=${maxTurns})`);
  });
  context.subscriptions.push(runCmd);

  // 注册命令：debug
  const debugCmd = vscode.commands.registerCommand('agentprimordia.debug', async () => {
    const cfg = await resolveDebugConfig();
    if (!cfg) return;
    vscode.debug.startDebugging(undefined, buildLaunchJson(cfg));
  });
  context.subscriptions.push(debugCmd);

  // 注册命令：stop
  const stopCmd = vscode.commands.registerCommand('agentprimordia.stop', () => {
    const result = applyCommand(STATE.state, { type: 'stop' });
    STATE.state = result.state;
    log('Inspector 已停止');
  });
  context.subscriptions.push(stopCmd);

  // 注册调试配置提供者
  const provider = new DebugConfigProvider();
  context.subscriptions.push(
    vscode.debug.registerDebugConfigurationProvider('agentprimordia', provider),
  );
}

/** 停用扩展 */
export function deactivate(): void {
  STATE.state = createInspectorState();
}

/** 显示 Inspector Webview 面板 */
function showInspectorPanel(context: { subscriptions: unknown[] }): void {
  if (!vscode) return;
  const panel = vscode.window.createWebviewPanel(
    'agentprimordia.inspector',
    'Agent Inspector',
    vscode.ViewColumn.Beside,
    { enableScripts: true, retainContextWhenHidden: true },
  );

  // 渲染初始 HTML
  panel.webview.html = renderInspectorHtml(toWebviewPayload(STATE.state));

  // 处理 Webview 消息
  panel.webview.onDidReceiveMessage((msg: unknown) => {
    const cmd = msg as { type: string };
    if (cmd.type === 'command') {
      const result = applyCommand(STATE.state, (cmd as { type: 'command'; command: unknown }).command as never);
      STATE.state = result.state;
      panel.webview.postMessage({ type: 'state', payload: toWebviewPayload(STATE.state) });
    } else if (cmd.type === 'step') {
      const step = cmd as unknown as { kind: string; text?: string; tool?: string; args?: unknown };
      const result = applyStep(STATE.state, step.kind as never, {
        text: step.text,
        tool: step.tool,
        args: step.args,
      });
      STATE.state = result.state;
    }
  });

  context.subscriptions.push(panel);
}

/** 解析当前 workspace 的 .ap.yaml → AgentDebugConfig */
async function resolveDebugConfig(): Promise<AgentDebugConfig | null> {
  if (!vscode) return null;
  const folders = vscode.workspace.workspaceFolders ?? [];
  if (folders.length === 0) {
    vscode.window.showErrorMessage('AgentPrimordia: 请先打开一个工作目录');
    return null;
  }

  const root = folders[0].uri.fsPath;

  // 在 root 下查找 .ap.yaml / ap.yaml / agent.yaml
  const candidates = ['.ap.yaml', 'ap.yaml', 'agent.yaml'];
  let found: string | null = null;
  for (const name of candidates) {
    const uri = vscode.Uri.file(`${root}/${name}`);
    try {
      await vscode.workspace.fs.stat(uri);
      found = name;
      break;
    } catch {
      continue;
    }
  }

  if (!found) {
    const pick = await vscode.window.showQuickPick(candidates, {
      placeHolder: '未找到 .ap.yaml，是否生成模板？',
    });
    if (pick) {
      const uri = vscode.Uri.file(`${root}/${pick}`);
      await vscode.workspace.fs.writeFile(uri, Buffer.from(generateLaunchJsonTemplate(), 'utf8'));
    }
    return null;
  }

  const data = await vscode.workspace.fs.readFile(vscode.Uri.file(`${root}/${found}`));
  const text = Buffer.from(data).toString('utf8');
  const raw = parseYamlLite(text);
  if (!raw) {
    vscode.window.showErrorMessage(`AgentPrimordia: 解析 ${found} 失败`);
    return null;
  }
  const cfg = toDebugConfig(normalizeApConfig(raw), root);
  const errs = validateDebugConfig(cfg);
  if (errs.length > 0) {
    vscode.window.showErrorMessage(
      `AgentPrimordia: 配置无效: ${errs.map((e) => e.message).join('; ')}`,
    );
    return null;
  }
  return cfg;
}

/** 调试配置提供者 */
class DebugConfigProvider {
  provideDebugConfigurations(): Record<string, unknown>[] {
    return [buildLaunchJson(toDebugConfig({ name: 'agentprimordia-agent' }, '${workspaceFolder}'))];
  }

  resolveDebugConfiguration?(folder: unknown, config: Record<string, unknown>): Record<string, unknown> | null {
    if (!vscode) return config ?? null;
    const cwd =
      typeof config['cwd'] === 'string' && (config['cwd'] as string).includes('${workspaceFolder}')
        ? (folder as { uri: { fsPath: string } } | undefined)?.uri.fsPath ?? process.cwd()
        : (config['cwd'] as string) ?? process.cwd();
    const cfg = toDebugConfig(
      {
        name: (config['agentName'] as string) ?? 'agentprimordia-agent',
        systemPrompt: (config['systemPrompt'] as string) ?? '',
        initialPrompt: (config['initialPrompt'] as string) ?? '',
        maxTurns: (config['maxTurns'] as number) ?? 10,
      },
      cwd,
      (config['trace'] as boolean) ?? true,
    );
    const errs = validateDebugConfig(cfg);
    if (errs.length > 0) {
      vscode.window.showErrorMessage(
        `AgentPrimordia: 调试配置无效: ${errs.map((e) => e.message).join('; ')}`,
      );
      return null;
    }
    return buildLaunchJson(cfg);
  }
}

/** 输出通道日志 */
function log(text: string): void {
  if (!vscode) return;
  if (!STATE.output) {
    STATE.output = vscode.window.createOutputChannel('AgentPrimordia');
  }
  const out = STATE.output;
  if (out) {
    out.append(`[${new Date().toISOString()}] ${text}\n`);
    out.show();
  }
}

/** 渲染 Webview HTML（极简内联实现，避免引入打包器） */
function renderInspectorHtml(payload: Record<string, unknown>): string {
  const json = JSON.stringify(payload).replace(/</g, '\\u003c');
  return `<!doctype html>
<html>
<head>
<meta charset="utf-8" />
<title>Agent Inspector</title>
<style>
body { font-family: var(--vscode-font-family, monospace); padding: 12px; }
.state { background: var(--vscode-editor-background, #1e1e1e); padding: 8px; border-radius: 4px; }
.status { font-weight: bold; }
pre { white-space: pre-wrap; }
</style>
</head>
<body>
<div class="state">
<div class="status" id="status">${(payload as { statusLabel?: string }).statusLabel ?? '空闲'}</div>
<pre id="history"></pre>
</div>
<script>
const vscode = acquireVsCodeApi();
const initialState = ${json};
document.getElementById('history').textContent = JSON.stringify(initialState, null, 2);
window.addEventListener('message', (ev) => {
  const msg = ev.data;
  if (msg.type === 'state') {
    document.getElementById('status').textContent = msg.payload.statusLabel || '空闲';
    document.getElementById('history').textContent = JSON.stringify(msg.payload, null, 2);
  }
});
</script>
</body>
</html>`;
}

// 暴露内部 helper 供测试桩注入（不导出为公共 API）
export const __test__ = {
  resetState: () => {
    STATE.state = createInspectorState();
  },
  getState: () => serializeState(STATE.state),
  formatStateSummary,
  formatStepHistory,
};
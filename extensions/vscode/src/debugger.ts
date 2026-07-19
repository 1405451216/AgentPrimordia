/**
 * Agent 调试配置提供者（与 VS Code API 解耦）。
 *
 * 职责：
 * 1. 解析 .ap.yaml / ap.yaml → AgentDebugConfig
 * 2. 验证调试配置必填字段
 * 3. 生成 VS Code DebugConfiguration（仅在 extension.ts 中组合 vscode API）
 */

import type { AgentDebugConfig, ApYamlConfig } from './types.js';

/** .ap.yaml 候选文件名（按优先级） */
const AP_YAML_CANDIDATES = ['.ap.yaml', 'ap.yaml', 'agent.yaml'] as const;

/** 解析 YAML 字符串为对象。
 *
 * 为避免引入 yaml 解析依赖（VS Code 扩展应尽量轻量），实现一个最小
 * 子集解析：仅支持顶层 key: value、嵌套 map（缩进 2 空格）、数组
 *（- item）、注释（#）。失败时返回 null。
 *
 * 如需完整 YAML 支持，可在 host 端使用 yaml npm 包替代。
 */
export function parseYamlLite(yaml: string): Record<string, unknown> | null {
  const lines = yaml.split(/\r?\n/);
  const root: Record<string, unknown> = {};
  // 用栈跟踪当前路径
  const stack: Array<{ indent: number; container: Record<string, unknown> | unknown[] }> = [
    { indent: -1, container: root },
  ];

  for (let i = 0; i < lines.length; i++) {
    const raw = lines[i];
    const trimmed = raw.trim();
    if (trimmed === '' || trimmed.startsWith('#')) continue;

    const indent = raw.length - raw.trimStart().length;

    // 弹栈：找到 indent 小于当前 indent 的最近一帧
    while (stack.length > 1 && stack[stack.length - 1].indent >= indent) {
      stack.pop();
    }

    const top = stack[stack.length - 1];
    if (top.container instanceof Array) {
      // 数组元素
      if (trimmed.startsWith('- ')) {
        const value = parseValue(trimmed.slice(2).trim());
        top.container.push(value);
      }
    } else {
      // map：key: value
      const colonIdx = trimmed.indexOf(':');
      if (colonIdx === -1) return null;
      const key = trimmed.slice(0, colonIdx).trim();
      const rest = trimmed.slice(colonIdx + 1).trim();

      if (rest === '') {
        // 后续行是嵌套结构：peek 下一行决定是数组（以 `- ` 开头）
        // 还是对象（key: value）。简单 YAML 子集足够覆盖。
        const next = lines[i + 1];
        const nextTrimmed = next?.trim() ?? '';
        if (nextTrimmed.startsWith('- ')) {
          const arr: unknown[] = [];
          top.container[key] = arr;
          stack.push({ indent, container: arr });
        } else {
          const child: Record<string, unknown> = {};
          top.container[key] = child;
          stack.push({ indent, container: child });
        }
      } else if (rest.startsWith('[') && rest.endsWith(']')) {
        // 行内数组 [a, b, c]
        const arr = rest
          .slice(1, -1)
          .split(',')
          .map((s) => parseValue(s.trim()))
          .filter((v) => v !== '');
        top.container[key] = arr;
      } else if (rest.startsWith('- ')) {
        // 嵌套数组
        const arr: unknown[] = [parseValue(rest.slice(2).trim())];
        top.container[key] = arr;
        stack.push({ indent, container: arr });
      } else {
        top.container[key] = parseValue(rest);
      }
    }
  }
  return root;
}

function parseValue(v: string): unknown {
  if (v === '') return '';
  // 引号
  if ((v.startsWith('"') && v.endsWith('"')) || (v.startsWith("'") && v.endsWith("'"))) {
    return v.slice(1, -1);
  }
  // 数字
  if (/^-?\d+$/.test(v)) return parseInt(v, 10);
  if (/^-?\d*\.\d+$/.test(v)) return parseFloat(v);
  // 布尔
  if (v === 'true') return true;
  if (v === 'false') return false;
  if (v === 'null') return null;
  return v;
}

/** 已知字段集合（用于透传过滤，避免未知类型的已知字段被错误透传） */
const KNOWN_FIELDS = new Set([
  'name',
  'systemPrompt',
  'initialPrompt',
  'maxTurns',
  'tools',
]);

/** 规范化 .ap.yaml 配置字段 */
export function normalizeApConfig(raw: Record<string, unknown>): ApYamlConfig {
  const cfg: ApYamlConfig = {};
  if (typeof raw['name'] === 'string') cfg.name = raw['name'];
  if (typeof raw['systemPrompt'] === 'string') cfg.systemPrompt = raw['systemPrompt'];
  if (typeof raw['initialPrompt'] === 'string') cfg.initialPrompt = raw['initialPrompt'];
  if (typeof raw['maxTurns'] === 'number') cfg.maxTurns = raw['maxTurns'];
  if (Array.isArray(raw['tools'])) {
    cfg.tools = raw['tools'].filter((t): t is string => typeof t === 'string');
  }
  // 其它字段：仅透传基础类型（string/number/boolean/array/object）。
  // 已知字段（即便类型不匹配）也跳过透传，保持 ApYamlConfig 契约纯净。
  for (const [k, v] of Object.entries(raw)) {
    if (KNOWN_FIELDS.has(k)) continue;
    if (
      typeof v === 'string' ||
      typeof v === 'number' ||
      typeof v === 'boolean' ||
      Array.isArray(v) ||
      (v !== null && typeof v === 'object')
    ) {
      cfg[k] = v;
    }
  }
  return cfg;
}

/** 把 ApYamlConfig 转成 AgentDebugConfig，补全默认值。 */
export function toDebugConfig(
  cfg: ApYamlConfig,
  cwd: string,
  trace: boolean = true,
): AgentDebugConfig {
  return {
    name: cfg.name ?? 'agentprimordia-agent',
    agentName: cfg.name ?? 'agentprimordia-agent',
    systemPrompt: cfg.systemPrompt ?? '',
    initialPrompt: cfg.initialPrompt ?? '',
    maxTurns: cfg.maxTurns ?? 10,
    trace,
    cwd,
  };
}

/** 校验调试配置必填字段 */
export interface DebugConfigError {
  field: keyof AgentDebugConfig;
  message: string;
}

export function validateDebugConfig(cfg: AgentDebugConfig): DebugConfigError[] {
  const errs: DebugConfigError[] = [];
  if (!cfg.agentName) errs.push({ field: 'agentName', message: 'agent 名称不能为空' });
  if (!cfg.cwd) errs.push({ field: 'cwd', message: '工作目录不能为空' });
  if (cfg.maxTurns < 1) errs.push({ field: 'maxTurns', message: 'maxTurns 必须 >= 1' });
  if (cfg.maxTurns > 100) errs.push({ field: 'maxTurns', message: 'maxTurns 不能超过 100' });
  return errs;
}

/** 生成 launch.json 风格 debug configuration 段 */
export function buildLaunchJson(cfg: AgentDebugConfig): Record<string, unknown> {
  return {
    type: 'agentprimordia',
    request: 'launch',
    name: cfg.name,
    agentName: cfg.agentName,
    systemPrompt: cfg.systemPrompt,
    initialPrompt: cfg.initialPrompt,
    maxTurns: cfg.maxTurns,
    trace: cfg.trace,
    cwd: cfg.cwd,
  };
}

/** 选择已找到的 .ap.yaml 文件名（按优先级 .ap.yaml > ap.yaml > agent.yaml） */
export function pickApYamlFilename(available: readonly string[]): string | null {
  for (const cand of AP_YAML_CANDIDATES) {
    if (available.includes(cand)) return cand;
  }
  return null;
}

/** 生成 VS Code launch.json 模板字符串 */
export function generateLaunchJsonTemplate(): string {
  return `{
  "version": "0.2.0",
  "configurations": [
    {
      "type": "agentprimordia",
      "request": "launch",
      "name": "AgentPrimordia: Debug",
      "agentName": "my-agent",
      "systemPrompt": "You are a helpful assistant.",
      "initialPrompt": "Hello!",
      "maxTurns": 10,
      "trace": true,
      "cwd": "\${workspaceFolder}"
    }
  ]
}
`;
}

/** 提示信息：在用户尚未配置 maxTurns 时给出默认建议 */
export const DEFAULT_MAX_TURNS = 10;
export const MIN_MAX_TURNS = 1;
export const MAX_MAX_TURNS = 100;
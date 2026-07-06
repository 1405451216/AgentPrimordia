/**
 * Debugger Provider 测试（Phase 5 Task 9）。
 */

import { describe, it, expect } from 'vitest';

import {
  parseYamlLite,
  normalizeApConfig,
  toDebugConfig,
  validateDebugConfig,
  buildLaunchJson,
  pickApYamlFilename,
  generateLaunchJsonTemplate,
  DEFAULT_MAX_TURNS,
  MIN_MAX_TURNS,
  MAX_MAX_TURNS,
} from '../src/debugger.js';

describe('parseYamlLite', () => {
  it('解析顶层 key: value', () => {
    const out = parseYamlLite(`
name: my-agent
maxTurns: 5
trace: true
`);
    expect(out).toEqual({ name: 'my-agent', maxTurns: 5, trace: true });
  });

  it('解析嵌套 map', () => {
    const out = parseYamlLite(`
agent:
  name: foo
  tools:
    - http
    - shell
`);
    expect(out).toEqual({
      agent: {
        name: 'foo',
        tools: ['http', 'shell'],
      },
    });
  });

  it('解析顶层数组', () => {
    const out = parseYamlLite(`
tools:
  - http
  - shell
`);
    expect(out).toEqual({ tools: ['http', 'shell'] });
  });

  it('支持引号字符串', () => {
    const out = parseYamlLite(`
name: "my agent"
greeting: 'hello'
`);
    expect(out).toEqual({ name: 'my agent', greeting: 'hello' });
  });

  it('支持行内数组', () => {
    const out = parseYamlLite(`
tools: [a, b, c]
`);
    expect(out).toEqual({ tools: ['a', 'b', 'c'] });
  });

  it('注释行被忽略', () => {
    const out = parseYamlLite(`
# 这是注释
name: agent
# another
maxTurns: 3
`);
    expect(out).toEqual({ name: 'agent', maxTurns: 3 });
  });

  it('空字符串返回空对象', () => {
    expect(parseYamlLite('')).toEqual({});
  });

  it('无冒号返回 null', () => {
    expect(parseYamlLite('foo')).toBeNull();
  });
});

describe('normalizeApConfig', () => {
  it('提取已知字段并保留透传字段', () => {
    const raw = {
      name: 'agent',
      systemPrompt: 'sys',
      maxTurns: 5,
      tools: ['http'],
      customField: 'x',
    };
    const out = normalizeApConfig(raw);
    expect(out.name).toBe('agent');
    expect(out.systemPrompt).toBe('sys');
    expect(out.maxTurns).toBe(5);
    expect(out.tools).toEqual(['http']);
    expect(out.customField).toBe('x');
  });

  it('类型错误字段被丢弃', () => {
    const out = normalizeApConfig({ name: 123, maxTurns: 'x' });
    expect(out.name).toBeUndefined();
    expect(out.maxTurns).toBeUndefined();
  });

  it('tools 非数组被丢弃', () => {
    const out = normalizeApConfig({ tools: 'http' });
    expect(out.tools).toBeUndefined();
  });
});

describe('toDebugConfig', () => {
  it('使用默认值补全字段', () => {
    const cfg = toDebugConfig({}, '/work', false);
    expect(cfg.agentName).toBe('agentprimordia-agent');
    expect(cfg.maxTurns).toBe(DEFAULT_MAX_TURNS);
    expect(cfg.trace).toBe(false);
    expect(cfg.cwd).toBe('/work');
  });

  it('保留 cfg 中的所有已知字段', () => {
    const cfg = toDebugConfig(
      { name: 'foo', systemPrompt: 's', initialPrompt: 'p', maxTurns: 3 },
      '/w',
    );
    expect(cfg.agentName).toBe('foo');
    expect(cfg.systemPrompt).toBe('s');
    expect(cfg.initialPrompt).toBe('p');
    expect(cfg.maxTurns).toBe(3);
  });
});

describe('validateDebugConfig', () => {
  it('合法配置无错误', () => {
    const cfg = toDebugConfig({ name: 'a' }, '/w');
    expect(validateDebugConfig(cfg)).toEqual([]);
  });

  it('maxTurns < 1 报错', () => {
    const cfg = toDebugConfig({ maxTurns: 0 }, '/w');
    const errs = validateDebugConfig(cfg);
    expect(errs.some((e) => e.field === 'maxTurns')).toBe(true);
  });

  it('maxTurns > MAX_MAX_TURNS 报错', () => {
    const cfg = toDebugConfig({ maxTurns: MAX_MAX_TURNS + 1 }, '/w');
    const errs = validateDebugConfig(cfg);
    expect(errs.some((e) => e.field === 'maxTurns')).toBe(true);
  });

  it('空 cwd 报错', () => {
    const cfg = toDebugConfig({ name: 'a' }, '');
    const errs = validateDebugConfig(cfg);
    expect(errs.some((e) => e.field === 'cwd')).toBe(true);
  });
});

describe('buildLaunchJson', () => {
  it('生成 VS Code 兼容配置', () => {
    const cfg = toDebugConfig({ name: 'a' }, '/w');
    const json = buildLaunchJson(cfg);
    expect(json['type']).toBe('agentprimordia');
    expect(json['request']).toBe('launch');
    expect(json['cwd']).toBe('/w');
  });
});

describe('pickApYamlFilename', () => {
  it('按优先级选择 .ap.yaml > ap.yaml > agent.yaml', () => {
    expect(pickApYamlFilename(['ap.yaml', 'agent.yaml', '.ap.yaml'])).toBe('.ap.yaml');
    expect(pickApYamlFilename(['agent.yaml', 'ap.yaml'])).toBe('ap.yaml');
    expect(pickApYamlFilename(['agent.yaml'])).toBe('agent.yaml');
  });

  it('未匹配返回 null', () => {
    expect(pickApYamlFilename(['other.yaml'])).toBeNull();
  });
});

describe('generateLaunchJsonTemplate', () => {
  it('返回有效 JSON', () => {
    const tpl = generateLaunchJsonTemplate();
    const parsed = JSON.parse(tpl);
    expect(parsed.version).toBe('0.2.0');
    expect(Array.isArray(parsed.configurations)).toBe(true);
    expect(parsed.configurations[0].type).toBe('agentprimordia');
  });
});

describe('constants', () => {
  it('默认值合理', () => {
    expect(DEFAULT_MAX_TURNS).toBe(10);
    expect(MIN_MAX_TURNS).toBe(1);
    expect(MAX_MAX_TURNS).toBe(100);
  });
});
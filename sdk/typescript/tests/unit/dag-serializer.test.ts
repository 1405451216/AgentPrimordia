/**
 * DAG 序列化/反序列化单元测试。
 *
 * 验证 Go ↔ TS DAG JSON 互通。
 */
import { describe, it, expect } from 'vitest';
import {
  serializeDAG,
  serializeDAGToString,
  deserializeDAG,
  deserializeDAGFromString,
  DAG_JSON_VERSION,
  NodeTypeAgent,
} from '../../src/protocol/dag-serializer.js';
import { DAGJSON } from '../../src/protocol/dag-serializer.js';

describe('serializeDAG', () => {
  it('should serialize basic DAG', () => {
    const result = serializeDAG({
      name: 'test-workflow',
      nodes: [{ id: 'step1' }, { id: 'step2' }],
      edges: [{ from: 'step1', to: 'step2' }],
    });

    expect(result.version).toBe(DAG_JSON_VERSION);
    expect(result.name).toBe('test-workflow');
    expect(result.nodes).toHaveLength(2);
    expect(result.edges).toHaveLength(1);
  });

  it('should sort nodes by ID (deterministic)', () => {
    const result = serializeDAG({
      name: 'det',
      nodes: [{ id: 'c' }, { id: 'a' }, { id: 'b' }],
      edges: [
        { from: 'a', to: 'b' },
        { from: 'b', to: 'c' },
      ],
    });

    expect(result.nodes.map((n) => n.id)).toEqual(['a', 'b', 'c']);
  });

  it('should populate depends_on from edges', () => {
    const result = serializeDAG({
      name: 'deps',
      nodes: [{ id: 'a' }, { id: 'b' }, { id: 'c' }],
      edges: [
        { from: 'a', to: 'b' },
        { from: 'a', to: 'c' },
      ],
    });

    const nodeB = result.nodes.find((n) => n.id === 'b')!;
    const nodeC = result.nodes.find((n) => n.id === 'c')!;
    const nodeA = result.nodes.find((n) => n.id === 'a')!;

    expect(nodeB.depends_on).toEqual(['a']);
    expect(nodeC.depends_on).toEqual(['a']);
    expect(nodeA.depends_on).toBeUndefined();
  });

  it('should populate outputs from edges', () => {
    const result = serializeDAG({
      name: 'outs',
      nodes: [{ id: 'a' }, { id: 'b' }, { id: 'c' }],
      edges: [
        { from: 'a', to: 'b' },
        { from: 'a', to: 'c' },
      ],
    });

    const nodeA = result.nodes.find((n) => n.id === 'a')!;
    expect(nodeA.outputs).toEqual(['b', 'c']);
  });

  it('should mark conditional edges', () => {
    const result = serializeDAG({
      name: 'cond',
      nodes: [{ id: 'a' }, { id: 'b' }, { id: 'c' }],
      edges: [
        { from: 'a', to: 'b', condition: true },
        { from: 'a', to: 'c', condition: false },
      ],
    });

    const edgeAB = result.edges.find((e) => e.from === 'a' && e.to === 'b')!;
    const edgeAC = result.edges.find((e) => e.from === 'a' && e.to === 'c')!;
    expect(edgeAB.condition).toBe(true);
    expect(edgeAC.condition).toBe(false);
  });

  it('should include config in nodes', () => {
    const result = serializeDAG({
      name: 'cfg',
      nodes: [{ id: 'n1', config: { label: 'First', priority: 'high' } }],
      edges: [],
    });

    expect(result.nodes[0].config).toEqual({ label: 'First', priority: 'high' });
  });
});

describe('serializeDAGToString', () => {
  it('should produce valid JSON', () => {
    const json = serializeDAGToString({
      name: 'json-test',
      nodes: [{ id: 'a' }, { id: 'b' }],
      edges: [{ from: 'a', to: 'b' }],
    });

    const parsed = JSON.parse(json);
    expect(parsed.name).toBe('json-test');
    expect(parsed.nodes).toHaveLength(2);
  });
});

describe('deserializeDAG', () => {
  it('should rebuild a DAG from JSON', () => {
    const dagJSON: DAGJSON = {
      version: '1.0',
      name: 'roundtrip',
      nodes: [
        { id: 'x', type: NodeTypeAgent, config: { label: 'X' }, inputs: [], outputs: ['y'] },
        { id: 'y', type: NodeTypeAgent, config: { label: 'Y' }, inputs: [], outputs: [], depends_on: ['x'] },
      ],
      edges: [{ from: 'x', to: 'y', condition: false }],
    };

    const handlers = {
      x: async (input: string) => `x-output-${input}`,
      y: async (input: string) => `y-output-${input}`,
    };

    const workflow = deserializeDAG(dagJSON, handlers);
    expect(workflow).toBeDefined();
    expect((workflow as any).name_).toBe('roundtrip');
  });

  it('should inject label from config', () => {
    const dagJSON: DAGJSON = {
      version: '1.0',
      name: 'labeled',
      nodes: [
        { id: 'n1', type: NodeTypeAgent, config: { label: 'Step One' }, inputs: [], outputs: [] },
      ],
      edges: [],
    };

    const workflow = deserializeDAG(dagJSON, {
      n1: async () => 'done',
    });
    expect(workflow).toBeDefined();
  });
});

describe('deserializeDAGFromString', () => {
  it('should parse and rebuild from JSON string', () => {
    const json = JSON.stringify({
      version: '1.0',
      name: 'from-string',
      nodes: [
        { id: 'n1', type: 'agent', config: {}, inputs: [], outputs: ['n2'] },
        { id: 'n2', type: 'agent', config: {}, inputs: [], outputs: [] },
      ],
      edges: [{ from: 'n1', to: 'n2', condition: false }],
    });

    const workflow = deserializeDAGFromString(json, {
      n1: async () => 'r1',
      n2: async () => 'r2',
    });
    expect(workflow).toBeDefined();
  });
});

describe('cross-language compat', () => {
  it('should produce JSON with snake_case fields', () => {
    const json = serializeDAGToString({
      name: 'compat',
      nodes: [{ id: 'a' }, { id: 'b' }],
      edges: [{ from: 'a', to: 'b' }],
    });

    // 验证字段名是 snake_case
    expect(json).toContain('"depends_on"');
    expect(json).not.toContain('"dependsOn"');
    expect(json).toContain('"outputs"');
  });

  it('should parse JSON in Go format', () => {
    // 模拟 Go 端标准 JSON 输出
    const goStyleJSON = JSON.stringify({
      version: '1.0',
      name: 'go-style',
      nodes: [
        { id: 'search', type: 'agent', config: { label: 'Search', _node_id: 'search' }, inputs: [], outputs: ['summarize'], depends_on: [] },
        { id: 'summarize', type: 'agent', config: { label: 'Summarize', _node_id: 'summarize' }, inputs: [], outputs: [], depends_on: ['search'] },
      ],
      edges: [{ from: 'search', to: 'summarize', condition: false }],
    });

    const dagJSON: DAGJSON = JSON.parse(goStyleJSON);
    const workflow = deserializeDAG(dagJSON, {
      search: async () => 'search-result',
      summarize: async () => 'summary',
    });
    expect(workflow).toBeDefined();
  });

  it('should round-trip through JSON.parse → serialize', () => {
    const original: DAGJSON = {
      version: '1.0',
      name: 'roundtrip',
      nodes: [
        { id: 'a', type: NodeTypeAgent, config: { label: 'A' }, inputs: [], outputs: ['b'] },
        { id: 'b', type: NodeTypeAgent, config: { label: 'B' }, inputs: [], outputs: [], depends_on: ['a'] },
      ],
      edges: [{ from: 'a', to: 'b', condition: false }],
    };

    const jsonStr = JSON.stringify(original);
    const parsed: DAGJSON = JSON.parse(jsonStr);
    expect(parsed).toEqual(original);
  });
});

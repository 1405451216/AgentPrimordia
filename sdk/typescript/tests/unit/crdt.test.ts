/**
 * CRDT 协作编辑单元测试
 *
 * 验证：
 * - Lamport Clock 正确递增
 * - Last-Write-Wins 冲突解决
 * - 数组合量合并
 * - 离线编辑后合并
 * - 操作顺序保证
 */

import { describe, it, expect } from 'vitest';
import {
  LamportClock,
  LWWRegister,
  LWWElementSet,
  CRDTDocumentImpl,
  LCROperations,
  compareOperations,
  type Operation,
  type CRDTDocument,
} from '../../src/collaboration/crdt.js';

describe('LamportClock', () => {
  it('should start at 0 by default', () => {
    const clock = new LamportClock('client-a');
    expect(clock.value).toBe(0);
  });

  it('should tick monotonically', () => {
    const clock = new LamportClock('client-a');
    expect(clock.tick()).toBe(1);
    expect(clock.tick()).toBe(2);
    expect(clock.tick()).toBe(3);
    expect(clock.value).toBe(3);
  });

  it('should update with remote clock', () => {
    const clock = new LamportClock('client-a');
    clock.tick(); // 1
    clock.update(5);
    expect(clock.value).toBe(6);
  });

  it('should handle concurrent clocks', () => {
    const clock = new LamportClock('client-a');
    clock.tick(); // 1
    clock.tick(); // 2
    clock.update(2); // max(2, 2) + 1 = 3
    expect(clock.value).toBe(3);
  });
});

describe('LWWRegister', () => {
  it('should hold initial value', () => {
    const reg = new LWWRegister<string>('hello');
    expect(reg.get()).toBe('hello');
  });

  it('should update with higher clock', () => {
    const reg = new LWWRegister<string>('old');
    expect(reg.set('new', 'client-a', 1)).toBe(true);
    expect(reg.get()).toBe('new');
  });

  it('should reject lower clock', () => {
    const reg = new LWWRegister<string>('new');
    reg.set('new', 'client-a', 5);
    expect(reg.set('old', 'client-b', 3)).toBe(false);
    expect(reg.get()).toBe('new');
  });

  it('should use clientID for tie-breaking', () => {
    const reg = new LWWRegister<string>('initial');
    reg.set('a', 'client-a', 10);
    // 相同 clock 但 clientID 更大时胜出
    expect(reg.set('b', 'client-b', 10)).toBe(true);
    expect(reg.get()).toBe('b');
  });

  it('should allow same client to update', () => {
    const reg = new LWWRegister<string>('v1');
    reg.set('v1', 'client-a', 1);
    expect(reg.set('v2', 'client-a', 2)).toBe(true);
    expect(reg.get()).toBe('v2');
  });
});

describe('LWWElementSet', () => {
  it('should add and retrieve elements', () => {
    const set = new LWWElementSet<string>();
    set.add('a', 'Alice', 1, 'client-1');
    set.add('b', 'Bob', 2, 'client-1');

    const elements = set.getElements();
    expect(elements.length).toBe(2);
    expect(elements[0].value).toBe('Alice');
    expect(elements[1].value).toBe('Bob');
  });

  it('should support logical delete', () => {
    const set = new LWWElementSet<string>();
    set.add('a', 'Alice', 1, 'client-1');
    set.remove('a', 2, 'client-1');

    expect(set.getElements().length).toBe(0);
  });

  it('should not remove with older clock', () => {
    const set = new LWWElementSet<string>();
    set.add('a', 'Alice', 5, 'client-1');
    set.remove('a', 3, 'client-1');

    // Remove clock 更早，不应生效
    expect(set.getElements().length).toBe(1);
  });

  it('should merge two sets correctly', () => {
    const set1 = new LWWElementSet<string>();
    set1.add('a', 'Alice', 1, 'client-1');
    set1.add('b', 'Bob', 2, 'client-1');

    const set2 = new LWWElementSet<string>();
    set2.add('c', 'Charlie', 3, 'client-2');
    set2.remove('a', 4, 'client-2');

    set1.merge(set2);

    const elements = set1.getElements();
    expect(elements.length).toBe(2); // Bob, Charlie
    expect(elements.map((e: { key: string; value: unknown; clock: number; clientID: string }) => e.value)).toContain('Bob');
    expect(elements.map((e: { key: string; value: unknown; clock: number; clientID: string }) => e.value)).toContain('Charlie');
    expect(elements.map((e: { key: string; value: unknown; clock: number; clientID: string }) => e.value)).not.toContain('Alice');
  });

  it('should sort elements by clock', () => {
    const set = new LWWElementSet<number>();
    set.add('c', 3, 3, 'client-1');
    set.add('a', 1, 1, 'client-1');
    set.add('b', 2, 2, 'client-1');

    const elements = set.getElements();
    expect(elements.map((e: { key: string; value: unknown; clock: number; clientID: string }) => e.value)).toEqual([1, 2, 3]);
  });
});

describe('CRDTDocumentImpl', () => {
  interface TestDoc {
    name: string;
    count: number;
    tags: string[];
  }

  it('should get initial state', () => {
    const doc = new CRDTDocumentImpl<TestDoc>('client-1', {
      name: 'test',
      count: 0,
      tags: [],
    });

    const state = doc.getState();
    expect(state.name).toBe('test');
    expect(state.count).toBe(0);
  });

  it('should apply set operations', () => {
    const doc = new CRDTDocumentImpl<TestDoc>('client-1');

    doc.set('name', 'Alice');
    expect(doc.get<string>('name')).toBe('Alice');

    doc.set('count', 42);
    expect(doc.get<number>('count')).toBe(42);
  });

  it('should apply delete operations', () => {
    const doc = new CRDTDocumentImpl<TestDoc>('client-1');

    doc.set('name', 'Alice');
    expect(doc.get<string>('name')).toBe('Alice');

    doc.delete('name');
    expect(doc.get<string>('name')).toBeUndefined();
  });

  it('should track all operations', () => {
    const doc = new CRDTDocumentImpl<TestDoc>('client-1');

    doc.set('name', 'Alice');
    doc.set('count', 10);
    doc.delete('tags');

    const ops = doc.getOperations();
    expect(ops.length).toBe(3);
    expect(ops[0].type).toBe('update');
    expect(ops[2].type).toBe('delete');
  });

  it('should merge documents with LWW', () => {
    const doc1 = new CRDTDocumentImpl<TestDoc>('client-1');
    const doc2 = new CRDTDocumentImpl<TestDoc>('client-2');

    // doc1 先写
    doc1.set('name', 'Alice');
    // doc2 后写（更高 clock）
    doc2.set('name', 'Bob');

    // 合并后应该是 Bob（更高 clock）
    doc1.merge(doc2);
    expect(doc1.get<string>('name')).toBe('Bob');
  });

  it('should preserve local writes with higher clock', () => {
    const doc1 = new CRDTDocumentImpl<TestDoc>('client-1');
    const doc2 = new CRDTDocumentImpl<TestDoc>('client-2');

    // doc1 后写（更高 clock）
    doc1.set('name', 'Alice');
    doc1.set('name', 'Alicia');

    // doc2 先写
    doc2.set('name', 'Bob');

    doc1.merge(doc2);
    // Alicia 的 clock 更高，保留
    expect(doc1.get<string>('name')).toBe('Alicia');
  });
});

describe('Offline editing and merge', () => {
  interface Doc {
    title: string;
    body: string;
    version: number;
  }

  it('should support offline edits then merge', () => {
    // 初始状态
    const initialState: Doc = { title: 'Doc', body: 'Initial', version: 1 };

    // 两个客户端各自离线编辑
    const client1 = new CRDTDocumentImpl<Doc>('client-1', initialState);
    const client2 = new CRDTDocumentImpl<Doc>('client-2', initialState);

    // 各自编辑不同字段
    client1.set('title', 'Doc by Client 1');
    client1.set('version', 2);

    client2.set('body', 'Updated by Client 2');
    client2.set('version', 3);

    // 合并
    client1.merge(client2);

    const state = client1.getState();
    expect(state.title).toBe('Doc by Client 1');
    expect(state.body).toBe('Updated by Client 2');
    // version: 谁的 clock 高谁赢
    expect(state.version).toBeGreaterThanOrEqual(2);
  });

  it('should converge after mutual merge', () => {
    const client1 = new CRDTDocumentImpl<{ x: number; y: number }>('client-1', { x: 0, y: 0 });
    const client2 = new CRDTDocumentImpl<{ x: number; y: number }>('client-2', { x: 0, y: 0 });

    client1.set('x', 10);
    client2.set('y', 20);

    // 双向合并
    client1.merge(client2);
    client2.merge(client1);

    // 最终状态应一致
    expect(client1.getState().x).toBe(client2.getState().x);
    expect(client1.getState().y).toBe(client2.getState().y);
  });

  it('should handle triple-client merge', () => {
    interface Shared {
      text: string;
      count: number;
    }

    const a = new CRDTDocumentImpl<Shared>('client-a', { text: '', count: 0 });
    const b = new CRDTDocumentImpl<Shared>('client-b', { text: '', count: 0 });
    const c = new CRDTDocumentImpl<Shared>('client-c', { text: '', count: 0 });

    a.set('text', 'Hello');
    b.set('count', 5);
    c.set('text', 'World'); // 更高 clock

    // 全部合并到 a
    a.merge(b);
    a.merge(c);

    const state = a.getState();
    expect(state.count).toBe(5);
    // World 的 clock 更高
    expect(state.text).toBe('World');
  });
});

describe('LCROperations', () => {
  interface Config {
    theme: string;
    fontSize: number;
  }

  it('should set and get values', () => {
    const lcr = new LCROperations<Config>('client-1');

    lcr.set('theme', 'dark', 'client-1', 1);
    lcr.set('fontSize', 14, 'client-1', 2);

    expect(lcr.get<string>('theme')).toBe('dark');
    expect(lcr.get<number>('fontSize')).toBe(14);
  });

  it('should resolve conflicts with LWW', () => {
    const lcr = new LCROperations<Config>('client-1');

    lcr.set('theme', 'light', 'client-1', 1);
    lcr.set('theme', 'dark', 'client-2', 5); // Higher clock wins

    expect(lcr.get<string>('theme')).toBe('dark');
  });

  it('should get full state', () => {
    const lcr = new LCROperations<Config>('client-1', { theme: 'light', fontSize: 12 });

    lcr.set('theme', 'dark', 'client-1', 1);

    const state = lcr.getState();
    expect(state.theme).toBe('dark');
    expect(state.fontSize).toBe(12);
  });
});

describe('compareOperations', () => {
  it('should order by clock', () => {
    const op1: Operation = { type: 'update', path: 'x', clock: 1, clientID: 'a' };
    const op2: Operation = { type: 'update', path: 'x', clock: 2, clientID: 'a' };

    expect(compareOperations(op1, op2)).toBeLessThan(0);
    expect(compareOperations(op2, op1)).toBeGreaterThan(0);
  });

  it('should break ties by clientID', () => {
    const op1: Operation = { type: 'update', path: 'x', clock: 5, clientID: 'a' };
    const op2: Operation = { type: 'update', path: 'x', clock: 5, clientID: 'b' };

    expect(compareOperations(op1, op2)).toBeLessThan(0);
  });

  it('should return 0 for same clock and clientID', () => {
    const op1: Operation = { type: 'update', path: 'x', clock: 5, clientID: 'a' };
    const op2: Operation = { type: 'update', path: 'x', clock: 5, clientID: 'a' };

    expect(compareOperations(op1, op2)).toBe(0);
  });
});
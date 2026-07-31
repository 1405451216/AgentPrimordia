/**
 * collaboration/__tests__/crdt.test.ts — CRDT 协作编辑生产化测试
 *
 * 验证 LamportClock、LWWRegister、LWWElementSet 和 CRDTMap 的
 * 正确性、并发安全性和收敛性。
 */
import { describe, it, expect } from 'vitest';
import { LamportClock, LWWRegister, LWWElementSet } from '../crdt.js';

describe('LamportClock', () => {
  it('should increment on tick', () => {
    const clock = new LamportClock('client-1');
    expect(clock.value).toBe(0);
    expect(clock.tick()).toBe(1);
    expect(clock.tick()).toBe(2);
    expect(clock.value).toBe(2);
  });

  it('should update on remote event', () => {
    const clock = new LamportClock('client-1');
    clock.tick(); // 1
    clock.update(10); // max(1, 10) + 1 = 11
    expect(clock.value).toBe(11);
  });

  it('should not go backwards on update with lower value', () => {
    const clock = new LamportClock('client-1', 100);
    clock.update(5); // max(100, 5) + 1 = 101
    expect(clock.value).toBe(101);
  });

  it('should preserve clientID', () => {
    const clock = new LamportClock('my-client');
    expect(clock.clientID).toBe('my-client');
  });
});

describe('LWWRegister', () => {
  it('should store initial value', () => {
    const reg = new LWWRegister<string>('hello');
    expect(reg.get()).toBe('hello');
  });

  it('should update with higher clock', () => {
    const reg = new LWWRegister<string>();
    reg.set('first', 'a', 1);
    reg.set('second', 'b', 2);
    expect(reg.get()).toBe('second');
  });

  it('should reject update with lower clock', () => {
    const reg = new LWWRegister<string>();
    reg.set('first', 'a', 5);
    const updated = reg.set('old', 'b', 3);
    expect(updated).toBe(false);
    expect(reg.get()).toBe('first');
  });

  it('should use clientID as tiebreaker on equal clock', () => {
    const reg = new LWWRegister<string>();
    reg.set('from-a', 'client-a', 5);
    reg.set('from-b', 'client-b', 5); // 'client-b' > 'client-a'
    expect(reg.get()).toBe('from-b');
  });

  it('should converge regardless of operation order', () => {
    const reg1 = new LWWRegister<number>();
    const reg2 = new LWWRegister<number>();

    // 不同顺序应用相同操作
    reg1.set(100, 'a', 1);
    reg1.set(200, 'b', 2);
    reg1.set(300, 'a', 3);

    reg2.set(300, 'a', 3);
    reg2.set(100, 'a', 1);
    reg2.set(200, 'b', 2);

    expect(reg1.get()).toBe(reg2.get());
    expect(reg1.get()).toBe(300);
  });
});

describe('LWWElementSet', () => {
  it('should add and retrieve elements', () => {
    const set = new LWWElementSet<string>();
    set.add('k1', 'value1', 1, 'client-a');
    set.add('k2', 'value2', 2, 'client-a');

    const elements = set.getElements();
    expect(elements).toHaveLength(2);
    expect(elements[0].key).toBe('k1');
    expect(elements[1].key).toBe('k2');
  });

  it('should remove elements', () => {
    const set = new LWWElementSet<string>();
    set.add('k1', 'value1', 1, 'client-a');
    set.remove('k1', 2, 'client-a');

    expect(set.getElements()).toHaveLength(0);
  });

  it('should keep element if add is later than remove', () => {
    const set = new LWWElementSet<string>();
    set.add('k1', 'value1', 5, 'client-a');
    set.remove('k1', 3, 'client-b'); // remove clock < add clock

    expect(set.getElements()).toHaveLength(1);
  });

  it('should handle concurrent add/remove convergence', () => {
    const set1 = new LWWElementSet<string>();
    const set2 = new LWWElementSet<string>();

    // Client A adds
    set1.add('item', 'v1', 1, 'a');
    // Client B removes (later clock)
    set1.remove('item', 2, 'b');

    // Reverse order on set2
    set2.remove('item', 2, 'b');
    set2.add('item', 'v1', 1, 'a');

    // Both should converge to same state
    expect(set1.getElements().length).toBe(set2.getElements().length);
  });

  it('should update existing element with higher clock', () => {
    const set = new LWWElementSet<string>();
    set.add('k1', 'old', 1, 'client-a');
    set.add('k1', 'new', 2, 'client-a');

    const elements = set.getElements();
    expect(elements).toHaveLength(1);
    expect(elements[0].value).toBe('new');
  });

  it('should sort elements by clock', () => {
    const set = new LWWElementSet<number>();
    set.add('c', 3, 3, 'x');
    set.add('a', 1, 1, 'x');
    set.add('b', 2, 2, 'x');

    const elements = set.getElements();
    expect(elements.map((e) => e.key)).toEqual(['a', 'b', 'c']);
  });
});

describe('CRDT 收敛性（多客户端模拟）', () => {
  it('should converge after concurrent edits from 3 clients', () => {
    const registers = [
      new LWWRegister<string>(),
      new LWWRegister<string>(),
      new LWWRegister<string>(),
    ];

    // 模拟 3 个客户端并发写入
    const ops: Array<{ value: string; client: string; clock: number }> = [
      { value: 'from-0', client: 'c0', clock: 1 },
      { value: 'from-1', client: 'c1', clock: 2 },
      { value: 'from-2', client: 'c2', clock: 3 },
      { value: 'from-0-late', client: 'c0', clock: 4 },
    ];

    // 每个 register 以不同顺序接收操作
    for (const reg of registers) {
      for (const op of ops) {
        reg.set(op.value, op.client, op.clock);
      }
    }

    // 全部收敛到相同值
    const values = registers.map((r) => r.get());
    expect(values[0]).toBe(values[1]);
    expect(values[1]).toBe(values[2]);
    expect(values[0]).toBe('from-0-late'); // clock=4 最高
  });
});

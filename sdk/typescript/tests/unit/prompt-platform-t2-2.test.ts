// prompt-platform-t2-2.test.ts — Phase 2 T2-2 Prompt 平台化测试
import { describe, it, expect, beforeEach } from 'vitest';
import { VersionedPromptRegistry } from '../../src/prompt/versioned-registry.js';
import { HotUpdateManager } from '../../src/prompt/hot-update.js';

describe('VersionedPromptRegistry', () => {
  let fixedNow: () => Date;
  let reg: VersionedPromptRegistry;

  beforeEach(() => {
    fixedNow = () => new Date('2026-07-09T12:00:00Z');
    reg = new VersionedPromptRegistry({ now: fixedNow });
  });

  it('register creates v1', () => {
    const v = reg.register('greeting', 'hello', { author: 'alice' });
    expect(v.version).toBe(1);
    expect(v.content).toBe('hello');
    expect(v.author).toBe('alice');
    expect(v.createdAt).toBe('2026-07-09T12:00:00.000Z');
  });

  it('register rejects duplicate name', () => {
    reg.register('greeting', 'v1');
    expect(() => reg.register('greeting', 'v2')).toThrow(/already exists/);
  });

  it('addVersion increments and auto-activates', () => {
    reg.register('greeting', 'v1');
    const v2 = reg.addVersion('greeting', 'v2', { author: 'bob' });
    expect(v2.version).toBe(2);
    expect(reg.getActive('greeting')?.content).toBe('v2');
  });

  it('addVersion throws when not registered', () => {
    expect(() => reg.addVersion('missing', 'x')).toThrow(/not registered/);
  });

  it('addVersion with deprecated flag does not auto-activate', () => {
    reg.register('greeting', 'v1');
    reg.addVersion('greeting', 'v2-bad', { deprecated: true });
    expect(reg.getActive('greeting')?.content).toBe('v1');
  });

  it('listVersions returns sorted', () => {
    reg.register('p', 'v1');
    reg.addVersion('p', 'v2');
    reg.addVersion('p', 'v3');
    const versions = reg.listVersions('p');
    expect(versions.map((v) => v.version)).toEqual([1, 2, 3]);
  });

  it('activate switches active version', () => {
    reg.register('p', 'v1');
    reg.addVersion('p', 'v2');
    reg.activate('p', 1);
    expect(reg.getActive('p')?.content).toBe('v1');
  });

  it('activate refuses deprecated version', () => {
    reg.register('p', 'v1');
    reg.addVersion('p', 'v2-bad', { deprecated: true });
    expect(() => reg.activate('p', 2)).toThrow(/deprecated/);
  });

  it('rollback decrements active version', () => {
    reg.register('p', 'v1');
    reg.addVersion('p', 'v2');
    reg.rollback('p');
    expect(reg.getActive('p')?.content).toBe('v1');
  });

  it('rollback refuses at v1', () => {
    reg.register('p', 'v1');
    expect(() => reg.rollback('p')).toThrow(/already at v1/);
  });

  it('findByTag locates tagged version', () => {
    reg.register('p', 'v1', { tags: ['baseline'] });
    reg.addVersion('p', 'v2', { tags: ['experiment-A'] });
    const found = reg.findByTag('p', 'experiment-A');
    expect(found?.content).toBe('v2');
    expect(reg.findByTag('p', 'nonexistent')).toBeUndefined();
  });

  it('delete removes entry', () => {
    reg.register('p', 'v1');
    expect(reg.delete('p')).toBe(true);
    expect(reg.delete('p')).toBe(false);
    expect(reg.getActive('p')).toBeUndefined();
  });

  it('listNames returns sorted names', () => {
    reg.register('z', 'z');
    reg.register('a', 'a');
    reg.register('m', 'm');
    expect(reg.listNames()).toEqual(['a', 'm', 'z']);
  });

  it('maxVersions evicts oldest non-deprecated', () => {
    const r = new VersionedPromptRegistry({ now: fixedNow, maxVersions: 3 });
    r.register('p', 'v1');
    r.addVersion('p', 'v2');
    r.addVersion('p', 'v3');
    r.addVersion('p', 'v4');
    const versions = r.listVersions('p');
    expect(versions.length).toBe(3);
    expect(versions.map((v) => v.version)).toEqual([2, 3, 4]);
  });

  it('toJSON/fromJSON round-trips', () => {
    reg.register('p', 'v1', { author: 'alice', tags: ['x'] });
    reg.addVersion('p', 'v2');
    const json = reg.toJSON();
    const restored = VersionedPromptRegistry.fromJSON(json, { now: fixedNow });
    expect(restored.listNames()).toEqual(['p']);
    expect(restored.getActive('p')?.content).toBe('v2');
    // v1 仍带 author 字段（即使不是 active version）
    const v1 = restored.getVersion('p', 1);
    expect(v1?.author).toBe('alice');
    expect(v1?.tags).toEqual(['x']);
  });

  it('describe returns defensive copy', () => {
    reg.register('p', 'v1');
    const desc = reg.describe('p');
    desc!.versions[0]!.content = 'mutated';
    expect(reg.getActive('p')?.content).toBe('v1'); // 不影响 registry
  });
});

describe('HotUpdateManager', () => {
  let reg: VersionedPromptRegistry;
  let manager: HotUpdateManager;
  let events: Array<unknown>;

  beforeEach(() => {
    reg = new VersionedPromptRegistry();
    manager = new HotUpdateManager(reg);
    events = [];
    manager.subscribe((e) => events.push(e));
  });

  it('update creates new prompt and emits event', () => {
    const v = manager.update('p', 'content', { author: 'alice' });
    expect(v.version).toBe(1);
    expect(events).toContainEqual({
      type: 'version_added',
      name: 'p',
      version: v,
    });
  });

  it('update on existing emits both added + activated', () => {
    manager.update('p', 'v1');
    events.length = 0;
    const v2 = manager.update('p', 'v2');
    expect(events).toContainEqual({ type: 'version_added', name: 'p', version: v2 });
    expect(events).toContainEqual({ type: 'version_activated', name: 'p', version: v2 });
  });

  it('activate emits activated event', () => {
    manager.update('p', 'v1');
    manager.update('p', 'v2');
    events.length = 0;
    const v = manager.activate('p', 1);
    expect(events).toContainEqual({ type: 'version_activated', name: 'p', version: v });
  });

  it('subscribe returns unsubscribe function', () => {
    const called: unknown[] = [];
    const unsub = manager.subscribe(() => called.push('hit'));
    unsub();
    manager.update('p', 'v1');
    expect(called).toEqual([]);
    // 默认的 events listener 仍应收到
    expect(events.length).toBeGreaterThan(0);
  });

  it('listener error does not break other listeners', () => {
    const events2: unknown[] = [];
    manager.subscribe(() => { throw new Error('boom'); });
    manager.subscribe((e) => events2.push(e));
    // 不应抛错，第二个 listener 仍能收到事件
    manager.update('p', 'v1');
    expect(events2.length).toBeGreaterThan(0);
  });

  it('getRegistry returns underlying registry', () => {
    expect(manager.getRegistry()).toBe(reg);
  });

  it('attachSource rejects duplicate id (and detach allows re-attach)', async () => {
    // 使用 mock source 而非真实 FileWatcherSource（避免 fs.watch 在测试环境阻塞）
    const mockSource = {
      _running: false,
      async start() { this._running = true; },
      async stop() { this._running = false; },
      isRunning() { return this._running; },
      onUpdate: null as null | ((...args: unknown[]) => void),
      onError: null as null | ((err: Error) => void),
    };
    await manager.attachSource('mock', mockSource as never);
    await expect(manager.attachSource('mock', mockSource as never)).rejects.toThrow(/already attached/);
    await manager.detachSource('mock');
    // detach 后允许再次 attach
    await manager.attachSource('mock', mockSource as never);
    await manager.detachSource('mock');
  });

  it('stopAll stops all attached sources', async () => {
    const mockSource = {
      _running: false,
      async start() { this._running = true; },
      async stop() { this._running = false; },
      isRunning() { return this._running; },
      onUpdate: null as null | ((...args: unknown[]) => void),
      onError: null as null | ((err: Error) => void),
    };
    await manager.attachSource('a', mockSource as never);
    await manager.attachSource('b', mockSource as never);
    expect(events.filter((e: any) => e.type === 'source_stopped')).toHaveLength(0);
    await manager.stopAll();
    expect(events.filter((e: any) => e.type === 'source_stopped')).toHaveLength(2);
  });
});
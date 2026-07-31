/**
 * cluster.test.ts — 集群协调模块单元测试
 *
 * 覆盖：
 * - ConsistentHash 一致性哈希环
 * - ClusterManager 集群管理器
 * - MemKVStore 内存 KV 存储
 * - DistributedDiscovery 分布式发现
 * - ClusterCoordinator 消息协调
 * - NodeInfo / NodeRole / NodeStatus 类型
 */

import { describe, it, expect, vi, beforeEach } from 'vitest';
import {
  ConsistentHash,
  ClusterManager,
  clusterConfigWithDefaults,
  MemKVStore,
  DistributedDiscovery,
  ClusterCoordinator,
} from '../../src/cluster/index.js';
import type {
  NodeInfo,
  NodeRole,
  NodeStatus,
  ClusterConfig,
  ClusterMessage,
  ClusterReply,
  RemoteNode,
} from '../../src/cluster/index.js';

// ===== ConsistentHash =====

describe('ConsistentHash', () => {
  it('应创建空哈希环', () => {
    const ring = new ConsistentHash(32);
    expect(ring.ringSize).toBe(0);
    expect(ring.getNodesList()).toEqual([]);
    expect(ring.getNode('key')).toBeNull();
  });

  it('应添加节点并路由 key', () => {
    const ring = new ConsistentHash(16);
    ring.addNode('node-a');
    ring.addNode('node-b');
    ring.addNode('node-c');

    expect(ring.ringSize).toBe(48); // 3 nodes * 16 replicas
    expect(ring.getNodesList().sort()).toEqual(['node-a', 'node-b', 'node-c']);

    // 同一个 key 应该总是路由到同一个节点
    const target = ring.getNode('test-key');
    expect(target).not.toBeNull();
    expect(ring.getNode('test-key')).toBe(target);
  });

  it('应在移除节点后重新路由', () => {
    const ring = new ConsistentHash(8);
    ring.addNode('a');
    ring.addNode('b');
    expect(ring.ringSize).toBe(16);

    ring.removeNode('a');
    expect(ring.ringSize).toBe(8);
    expect(ring.getNodesList()).toEqual(['b']);
    // 所有 key 都应路由到 b
    expect(ring.getNode('any-key')).toBe('b');
  });

  it('应支持 getNodes 获取多个不同节点', () => {
    const ring = new ConsistentHash(16);
    ring.addNode('x');
    ring.addNode('y');
    ring.addNode('z');

    const nodes = ring.getNodes('some-key', 2);
    expect(nodes.length).toBe(2);
    // 两个节点应不同
    expect(new Set(nodes).size).toBe(2);
  });

  it('getNodes 请求数超过实际节点时应返回全部', () => {
    const ring = new ConsistentHash(8);
    ring.addNode('only');
    const nodes = ring.getNodes('key', 5);
    expect(nodes).toEqual(['only']);
  });

  it('重复添加同一节点应无效果', () => {
    const ring = new ConsistentHash(4);
    ring.addNode('dup');
    ring.addNode('dup');
    expect(ring.ringSize).toBe(4);
  });

  it('移除不存在的节点应无效果', () => {
    const ring = new ConsistentHash(4);
    ring.addNode('real');
    ring.removeNode('ghost');
    expect(ring.ringSize).toBe(4);
  });

  it('空环 getNode 返回 null，getNodes 返回空数组', () => {
    const ring = new ConsistentHash();
    expect(ring.getNode('k')).toBeNull();
    expect(ring.getNodes('k', 3)).toEqual([]);
  });
});

// ===== clusterConfigWithDefaults =====

describe('clusterConfigWithDefaults', () => {
  it('应填充默认值', () => {
    const cfg = clusterConfigWithDefaults({ nodeId: 'n1', listenAddr: ':8080' });
    expect(cfg.heartbeatIntervalMs).toBe(5000);
    expect(cfg.heartbeatTimeoutMs).toBe(15000);
    expect(cfg.electionTimeoutMs).toBe(10000);
  });

  it('应保留用户指定值', () => {
    const cfg = clusterConfigWithDefaults({
      nodeId: 'n2', listenAddr: ':9090',
      heartbeatIntervalMs: 1000, heartbeatTimeoutMs: 3000, electionTimeoutMs: 2000,
    });
    expect(cfg.heartbeatIntervalMs).toBe(1000);
    expect(cfg.heartbeatTimeoutMs).toBe(3000);
    expect(cfg.electionTimeoutMs).toBe(2000);
  });
});

// ===== ClusterManager =====

describe('ClusterManager', () => {
  it('应创建并获取本地节点', () => {
    const mgr = new ClusterManager({ nodeId: 'node-1', listenAddr: ':8080' });
    const local = mgr.getLocalNode();
    expect(local.id).toBe('node-1');
    expect(local.address).toBe(':8080');
    expect(local.role).toBe('follower');
    expect(local.status).toBe('online');
  });

  it('启动后 running 为 true，停止后为 false', async () => {
    const mgr = new ClusterManager({ nodeId: 'n1', listenAddr: ':0' });
    expect(mgr.running).toBe(false);
    await mgr.start();
    expect(mgr.running).toBe(true);
    await mgr.stop();
    expect(mgr.running).toBe(false);
  });

  it('listNodes 至少包含本地节点', () => {
    const mgr = new ClusterManager({ nodeId: 'self', listenAddr: ':0' });
    const nodes = mgr.listNodes();
    expect(nodes.length).toBeGreaterThanOrEqual(1);
    expect(nodes[0].id).toBe('self');
  });

  it('getNode 查询本地节点应返回副本', () => {
    const mgr = new ClusterManager({ nodeId: 'me', listenAddr: ':0' });
    const node = mgr.getNode('me');
    expect(node).not.toBeNull();
    expect(node!.id).toBe('me');
    // 返回的是副本，非同一引用
    expect(node).not.toBe(mgr.getLocalNode());
  });

  it('getNode 查询未知节点返回 null', () => {
    const mgr = new ClusterManager({ nodeId: 'me', listenAddr: ':0' });
    expect(mgr.getNode('unknown')).toBeNull();
  });

  it('getLeader / isLeader / getRole 初始状态', () => {
    const mgr = new ClusterManager({ nodeId: 'n1', listenAddr: ':0' });
    expect(mgr.getLeader()).toBe('');
    expect(mgr.isLeader()).toBe(false);
    expect(mgr.getRole()).toBe('follower');
  });

  it('getHashRing 应包含本地节点', () => {
    const mgr = new ClusterManager({ nodeId: 'ring-node', listenAddr: ':0' });
    const ring = mgr.getHashRing();
    expect(ring.getNodesList()).toContain('ring-node');
  });
});

// ===== MemKVStore =====

describe('MemKVStore', () => {
  let kv: MemKVStore;

  beforeEach(() => {
    kv = new MemKVStore();
  });

  it('应 put 和 get', async () => {
    await kv.put('k1', 'v1');
    expect(await kv.get('k1')).toBe('v1');
  });

  it('get 不存在的 key 应抛异常', async () => {
    await expect(kv.get('missing')).rejects.toThrow('key not found');
  });

  it('应支持 TTL 过期', async () => {
    vi.useFakeTimers();
    await kv.put('ttl-key', 'val', 100);
    expect(await kv.get('ttl-key')).toBe('val');
    vi.advanceTimersByTime(150);
    await expect(kv.get('ttl-key')).rejects.toThrow('key expired');
    vi.useRealTimers();
  });

  it('应 delete', async () => {
    await kv.put('d', 'v');
    await kv.delete('d');
    await expect(kv.get('d')).rejects.toThrow('key not found');
  });

  it('应按前缀列出条目', async () => {
    await kv.put('prefix/a', '1');
    await kv.put('prefix/b', '2');
    await kv.put('other/c', '3');
    const result = await kv.listByPrefix('prefix/');
    expect(result.size).toBe(2);
    expect(result.get('prefix/a')).toBe('1');
    expect(result.get('prefix/b')).toBe('2');
  });

  it('close 应清理数据', async () => {
    await kv.put('x', 'y');
    await kv.close();
    await expect(kv.get('x')).rejects.toThrow('key not found');
  });
});

// ===== DistributedDiscovery =====

describe('DistributedDiscovery', () => {
  let kv: MemKVStore;
  let dd: DistributedDiscovery;

  beforeEach(() => {
    kv = new MemKVStore();
    dd = new DistributedDiscovery({ nodeId: 'node-1', kvStore: kv });
  });

  it('应注册和发现节点', async () => {
    await dd.register({
      id: 'node-1', name: 'node-1', address: ':8080', lastSeen: new Date(),
    });
    const info = await dd.discover('node-1');
    expect(info).not.toBeNull();
    expect(info!.id).toBe('node-1');
  });

  it('listAgents 应返回已注册节点', async () => {
    await dd.register({ id: 'a', name: 'a', address: ':1', lastSeen: new Date() });
    await dd.register({ id: 'b', name: 'b', address: ':2', lastSeen: new Date() });
    const agents = await dd.listAgents();
    expect(agents.length).toBe(2);
  });

  it('unregister 后 discover 返回 null', async () => {
    await dd.register({ id: 'x', name: 'x', address: ':0', lastSeen: new Date() });
    await dd.unregister('x');
    const info = await dd.discover('x');
    expect(info).toBeNull();
  });

  it('heartbeat 应更新 lastSeen', async () => {
    await dd.register({ id: 'hb', name: 'hb', address: ':0', lastSeen: new Date() });
    const before = await dd.discover('hb');
    await new Promise(r => setTimeout(r, 5));
    await dd.heartbeat('hb');
    const after = await dd.discover('hb');
    expect(after!.lastSeen!.getTime()).toBeGreaterThanOrEqual(before!.lastSeen!.getTime());
  });

  it('close 应清理缓存并停止心跳', async () => {
    await dd.register({ id: 'c', name: 'c', address: ':0', lastSeen: new Date() });
    await dd.close();
    // close 后缓存已清空，但 KV 中数据仍在
    // discover 会回退到 KV 读取，因此仍能发现节点
    const info = await dd.discover('c');
    expect(info).not.toBeNull();
    expect(info!.id).toBe('c');
  });
});

// ===== ClusterCoordinator =====

describe('ClusterCoordinator', () => {
  function makeMockNode(id: string, replyContent = 'ok'): RemoteNode {
    return {
      id,
      address: `:${id}`,
      sendMessage: vi.fn().mockResolvedValue({
        id: `reply-${id}`,
        inReplyTo: '',
        from: id,
        content: replyContent,
        isError: false,
        timestamp: Date.now(),
      } satisfies ClusterReply),
      healthCheck: vi.fn().mockResolvedValue(true),
      close: vi.fn().mockResolvedValue(undefined),
    };
  }

  it('应注册并处理消息', async () => {
    const coord = new ClusterCoordinator({
      nodeId: 'self',
      discovery: new DistributedDiscovery({ nodeId: 'self', kvStore: new MemKVStore() }),
    });
    coord.onMessage('ping', async (msg) => ({
      id: 'pong',
      inReplyTo: msg.id,
      from: 'self',
      content: 'pong',
      isError: false,
      timestamp: Date.now(),
    }));

    const reply = await coord.handleIncoming({
      id: 'msg-1', from: 'other', to: 'self',
      type: 'ping', content: 'hello', timestamp: Date.now(),
    });
    expect(reply.content).toBe('pong');
    expect(reply.isError).toBe(false);
  });

  it('未注册类型的消息应返回错误', async () => {
    const coord = new ClusterCoordinator({
      nodeId: 'self',
      discovery: new DistributedDiscovery({ nodeId: 'self', kvStore: new MemKVStore() }),
    });
    const reply = await coord.handleIncoming({
      id: 'msg-2', from: 'x', to: 'self',
      type: 'unknown', content: '', timestamp: Date.now(),
    });
    expect(reply.isError).toBe(true);
    expect(reply.content).toContain('no handler');
  });

  it('sendToNode 应发送到已注册节点', async () => {
    const coord = new ClusterCoordinator({
      nodeId: 'self',
      discovery: new DistributedDiscovery({ nodeId: 'self', kvStore: new MemKVStore() }),
    });
    const node = makeMockNode('remote-1', 'hello-back');
    coord.addRemoteNode(node);

    const msg: ClusterMessage = {
      id: 'm1', from: 'self', to: 'remote-1',
      type: 'data', content: 'test', timestamp: Date.now(),
    };
    const reply = await coord.sendToNode('remote-1', msg);
    expect(reply.content).toBe('hello-back');
    expect(node.sendMessage).toHaveBeenCalledOnce();
  });

  it('sendToNode 到未知节点应返回错误', async () => {
    const coord = new ClusterCoordinator({
      nodeId: 'self',
      discovery: new DistributedDiscovery({ nodeId: 'self', kvStore: new MemKVStore() }),
    });
    const msg: ClusterMessage = {
      id: 'm2', from: 'self', to: 'ghost',
      type: 'x', content: '', timestamp: Date.now(),
    };
    const reply = await coord.sendToNode('ghost', msg);
    expect(reply.isError).toBe(true);
    expect(reply.content).toContain('node not found');
  });

  it('broadcast 应向所有节点发送', async () => {
    const coord = new ClusterCoordinator({
      nodeId: 'self',
      discovery: new DistributedDiscovery({ nodeId: 'self', kvStore: new MemKVStore() }),
    });
    const n1 = makeMockNode('n1');
    const n2 = makeMockNode('n2');
    coord.addRemoteNode(n1);
    coord.addRemoteNode(n2);

    const msg: ClusterMessage = {
      id: 'b1', from: 'self', to: '*',
      type: 'broadcast', content: 'hi', timestamp: Date.now(),
    };
    const replies = await coord.broadcast(msg);
    expect(replies.length).toBe(2);
    expect(n1.sendMessage).toHaveBeenCalledOnce();
    expect(n2.sendMessage).toHaveBeenCalledOnce();
  });

  it('healthCheckAll 应检查所有节点', async () => {
    const coord = new ClusterCoordinator({
      nodeId: 'self',
      discovery: new DistributedDiscovery({ nodeId: 'self', kvStore: new MemKVStore() }),
    });
    coord.addRemoteNode(makeMockNode('h1'));
    coord.addRemoteNode(makeMockNode('h2'));

    const results = await coord.healthCheckAll();
    expect(results.size).toBe(2);
    expect(results.get('h1')).toBe(true);
    expect(results.get('h2')).toBe(true);
  });

  it('localNodeId 和 isRunning', () => {
    const coord = new ClusterCoordinator({
      nodeId: 'me',
      discovery: new DistributedDiscovery({ nodeId: 'me', kvStore: new MemKVStore() }),
    });
    expect(coord.localNodeId).toBe('me');
    expect(coord.isRunning).toBe(false);
  });
});

// ===== NodeInfo / NodeRole / NodeStatus 类型验证 =====

describe('NodeInfo types', () => {
  it('应构造合法 NodeInfo', () => {
    const node: NodeInfo = {
      id: 'n1',
      address: ':8080',
      role: 'follower' as NodeRole,
      status: 'online' as NodeStatus,
      joinTime: new Date(),
      lastSeen: new Date(),
    };
    expect(node.id).toBe('n1');
    expect(node.role).toBe('follower');
    expect(node.status).toBe('online');
  });

  it('应支持所有 NodeRole 值', () => {
    const roles: NodeRole[] = ['follower', 'leader', 'candidate'];
    expect(roles).toHaveLength(3);
  });

  it('应支持所有 NodeStatus 值', () => {
    const statuses: NodeStatus[] = ['online', 'offline', 'leaving'];
    expect(statuses).toHaveLength(3);
  });
});

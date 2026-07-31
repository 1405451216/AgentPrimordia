/**
 * governance/__tests__/governance.test.ts — 多租户治理模块测试
 */
import { describe, it, expect } from 'vitest';
import { TokenBucket, QuotaManager, ResourceManager } from '../quota.js';
import { PolicyEnforcer, PolicyViolationError, InMemoryAuditLogger } from '../policy.js';
import { TenantManager } from '../tenant.js';
import { defaultQuota } from '../types.js';
import type { Policy } from '../types.js';

describe('governance/TokenBucket', () => {
  it('should allow requests within rate', () => {
    const bucket = new TokenBucket(10, 10);
    for (let i = 0; i < 10; i++) {
      expect(bucket.take(1)).toBe(true);
    }
  });

  it('should reject when exhausted', () => {
    const bucket = new TokenBucket(5, 5);
    for (let i = 0; i < 5; i++) bucket.take(1);
    expect(bucket.take(1)).toBe(false);
  });

  it('should report available tokens', () => {
    const bucket = new TokenBucket(10, 10);
    bucket.take(3);
    expect(bucket.available()).toBe(7);
  });
});

describe('governance/QuotaManager', () => {
  it('should enforce agent quota', () => {
    const qm = new QuotaManager('t1', defaultQuota('free'));
    expect(qm.addAgent()).toBe(true);
    expect(qm.addAgent()).toBe(true);
    expect(qm.addAgent()).toBe(true);
    expect(qm.addAgent()).toBe(false); // max 3
  });

  it('should enforce daily token limit', () => {
    const qm = new QuotaManager('t1', { ...defaultQuota('free'), maxTokensPerDay: 100 });
    expect(qm.recordTokens(60)).toBe(true);
    expect(qm.recordTokens(60)).toBe(false); // 60+60 > 100
  });

  it('should provide snapshot', () => {
    const qm = new QuotaManager('t1', defaultQuota('pro'));
    qm.addAgent();
    qm.addSession();
    const snap = qm.snapshot();
    expect(snap.agentCount).toBe(1);
    expect(snap.sessionCount).toBe(1);
  });
});

describe('governance/ResourceManager', () => {
  it('should register and retrieve managers', () => {
    const rm = new ResourceManager();
    const qm = rm.register('t1', defaultQuota('free'));
    expect(rm.get('t1')).toBe(qm);
    expect(rm.listTenants()).toContain('t1');
  });

  it('should return existing manager on duplicate register', () => {
    const rm = new ResourceManager();
    const qm1 = rm.register('t1', defaultQuota('free'));
    const qm2 = rm.register('t1', defaultQuota('pro'));
    expect(qm1).toBe(qm2);
  });
});

describe('governance/PolicyEnforcer', () => {
  const testPolicy: Policy = {
    apiVersion: 'v1',
    kind: 'AgentPolicy',
    metadata: { name: 'test-policy' },
    spec: {
      toolRestrictions: [
        { tool: 'shell', maxCallsPerRun: 2, blockedArgs: ['rm -rf'] },
      ],
      costLimits: { perRequest: 1.0, perDay: 10.0 },
      outputGuardrail: { maxLength: 100, piiFilter: 'strict' },
      behaviorConstraints: { maxToolCalls: 5 },
    },
  };

  it('should allow valid tool calls', () => {
    const enforcer = new PolicyEnforcer(testPolicy);
    expect(() => enforcer.checkToolCall('shell', 'ls -la')).not.toThrow();
  });

  it('should block exceeded tool calls', () => {
    const enforcer = new PolicyEnforcer(testPolicy);
    enforcer.recordToolCall('shell');
    enforcer.recordToolCall('shell');
    expect(() => enforcer.checkToolCall('shell', 'ls')).toThrow(PolicyViolationError);
  });

  it('should block dangerous arguments', () => {
    const enforcer = new PolicyEnforcer(testPolicy);
    expect(() => enforcer.checkToolCall('shell', 'rm -rf /')).toThrow(PolicyViolationError);
  });

  it('should enforce cost limits', () => {
    const enforcer = new PolicyEnforcer(testPolicy);
    expect(() => enforcer.checkCost(2.0)).toThrow(PolicyViolationError);
  });

  it('should enforce output length', () => {
    const enforcer = new PolicyEnforcer(testPolicy);
    expect(() => enforcer.checkOutput('x'.repeat(200))).toThrow(PolicyViolationError);
  });

  it('should detect PII in strict mode', () => {
    const enforcer = new PolicyEnforcer(testPolicy);
    expect(() => enforcer.checkOutput('电话: 13812345678')).toThrow(PolicyViolationError);
  });

  it('should emit audit events', () => {
    const logger = new InMemoryAuditLogger();
    const enforcer = new PolicyEnforcer(testPolicy, { auditLog: logger, agentId: 'a1' });
    try { enforcer.checkCost(99); } catch { /* expected */ }
    expect(logger.events.length).toBe(1);
    expect(logger.events[0].type).toBe('cost_exceeded');
  });

  it('should provide snapshot', () => {
    const enforcer = new PolicyEnforcer(testPolicy);
    enforcer.recordToolCall('shell');
    enforcer.recordCost(0.5);
    const snap = enforcer.snapshot();
    expect(snap.totalToolCalls).toBe(1);
    expect(snap.totalCost).toBe(0.5);
  });
});

describe('governance/TenantManager', () => {
  it('should create tenant with default quota', () => {
    const tm = new TenantManager();
    const { tenant, apiKey } = tm.createTenant('Acme Corp');
    expect(tenant.name).toBe('Acme Corp');
    expect(tenant.plan).toBe('free');
    expect(tenant.status).toBe('active');
    expect(apiKey).toBeTruthy();
  });

  it('should authenticate with API key', () => {
    const tm = new TenantManager();
    const { tenant, apiKey } = tm.createTenant('Test');
    const authed = tm.authenticate(apiKey);
    expect(authed?.id).toBe(tenant.id);
  });

  it('should reject invalid API key', () => {
    const tm = new TenantManager();
    expect(tm.authenticate('invalid-key')).toBeUndefined();
  });

  it('should update tenant status', () => {
    const tm = new TenantManager();
    const { tenant } = tm.createTenant('Test');
    tm.updateStatus(tenant.id, 'disabled');
    expect(tm.getTenant(tenant.id)?.status).toBe('disabled');
  });

  it('should not authenticate disabled tenant', () => {
    const tm = new TenantManager();
    const { tenant, apiKey } = tm.createTenant('Test');
    tm.updateStatus(tenant.id, 'disabled');
    expect(tm.authenticate(apiKey)).toBeUndefined();
  });
});

describe('governance/defaultQuota', () => {
  it('should return increasing quotas per plan', () => {
    const free = defaultQuota('free');
    const pro = defaultQuota('pro');
    const ent = defaultQuota('enterprise');
    expect(pro.maxAgents).toBeGreaterThan(free.maxAgents);
    expect(ent.maxAgents).toBeGreaterThan(pro.maxAgents);
  });
});

/**
 * 多租户治理示例 — 配额限流 + 策略执行
 *
 * 运行: npx tsx examples/governance-quota.ts
 */
import { TenantManager, QuotaManager, TokenBucket, PolicyEnforcer } from '../src/governance/index.js';

async function main() {
  console.log('=== AgentPrimordia TS SDK: Governance & Quota ===\n');

  // 1. 租户管理
  console.log('--- Tenant Management ---');
  const tenants = new TenantManager();
  tenants.create({ id: 'tenant-acme', name: 'Acme Corp', plan: 'enterprise', status: 'active' });
  tenants.create({ id: 'tenant-startup', name: 'Startup Inc', plan: 'free', status: 'active' });

  const acme = tenants.get('tenant-acme');
  console.log(`Tenant: ${acme?.name} (${acme?.plan})`);
  console.log(`All tenants: ${tenants.list().map(t => t.id).join(', ')}`);

  // 2. 令牌桶限流
  console.log('\n--- Token Bucket Rate Limiting ---');
  const bucket = new TokenBucket({ capacity: 10, refillRate: 2 });

  console.log(`Initial tokens: ${bucket.tokens()}`);
  for (let i = 0; i < 12; i++) {
    const allowed = bucket.consume(1);
    if (!allowed) {
      console.log(`Request #${i + 1}: REJECTED (bucket empty)`);
      break;
    }
  }
  console.log(`Remaining: ${bucket.tokens()}`);

  // 3. 配额管理
  console.log('\n--- Quota Manager ---');
  const quota = new QuotaManager({
    'tenant-acme': { maxRequestsPerMinute: 100, maxTokensPerDay: 1_000_000, maxConcurrentAgents: 10 },
    'tenant-startup': { maxRequestsPerMinute: 10, maxTokensPerDay: 10_000, maxConcurrentAgents: 1 },
  });

  console.log(`Acme quota: ${JSON.stringify(quota.getQuota('tenant-acme'))}`);
  console.log(`Startup quota: ${JSON.stringify(quota.getQuota('tenant-startup'))}`);

  // 4. 策略执行
  console.log('\n--- Policy Enforcer ---');
  const enforcer = new PolicyEnforcer();
  enforcer.addPolicy({
    id: 'no-rm-command',
    description: 'Block rm commands',
    rules: [{ type: 'tool_restriction', tool: 'shell', pattern: 'rm -rf' }],
  });

  const allowed = enforcer.check('tenant-acme', { tool: 'shell', args: { cmd: 'ls -la' } });
  const blocked = enforcer.check('tenant-acme', { tool: 'shell', args: { cmd: 'rm -rf /' } });
  console.log(`ls -la: ${allowed ? 'ALLOWED' : 'BLOCKED'}`);
  console.log(`rm -rf /: ${blocked ? 'ALLOWED' : 'BLOCKED'}`);

  console.log('\n--- Done ---');
}

main().catch(console.error);

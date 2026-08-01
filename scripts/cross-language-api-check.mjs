#!/usr/bin/env node
/**
 * cross-language-api-check.mjs — 跨语言 API 兼容性检查
 *
 * 验证 cross-language-spec.json 中声明的 Go/TS 等价关系是否仍然成立：
 *   1. 运行 Go api-extract 生成最新的 api-contract.json
 *   2. 从 api-contract.json 提取 Go 公共 API 符号列表
 *   3. 从 cross-language-spec.json 读取声明的测试套件
 *   4. 验证 spec 中声明的每个 Go 类型/函数在 api-contract.json 中存在
 *   5. 验证 spec 中声明的每个 TS 对应实现在 TS 源码中存在
 *   6. 发现漂移时 CI 失败
 *
 * 用法：node scripts/cross-language-api-check.mjs
 */

import { readFileSync, existsSync } from 'node:fs';
import { resolve, dirname } from 'node:path';
import { execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { tmpdir } from 'node:os';

const __dirname = dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = resolve(__dirname, '..');

// ===== 测试套件 → Go/TS 符号映射 =====
// 每个测试套件声明了它所依赖的 Go 公共符号和 TS 导出。
// 当这些符号在任一侧消失时，说明发生了跨语言 API 漂移。

const SUITE_SYMBOLS = {
  agent_config: {
    go: ['Agent', 'NewAgent'],
    ts: ['Agent', 'AgentConfig'],
    tsSearchPaths: ['sdk/typescript/src'],
  },
  tool_execution: {
    go: ['Tool', 'ToolRegistry', 'NewToolRegistry'],
    ts: ['ToolRegistry', 'Tool'],
    tsSearchPaths: ['sdk/typescript/src/tools'],
  },
  vector_operations: {
    go: ['VectorRecord'],
    ts: ['cosineSimilarity', 'VectorRecord'],
    tsSearchPaths: ['sdk/typescript/src'],
  },
  error_handling: {
    go: ['CodeError', 'GetErrorCode'],
    ts: ['CodeError', 'getErrorCode'],
    tsSearchPaths: ['sdk/typescript/src'],
  },
  json_serialization: {
    go: ['VectorRecord'],
    ts: ['VectorRecord'],
    tsSearchPaths: ['sdk/typescript/src'],
  },
  error_code_mapping: {
    // 错误码前缀以 sentinel 错误常量形式暴露（如 ErrAgentStopped -> AGENT_001），
    // 检查两侧均存在的 sentinel 错误符号 + 提取函数
    go: ['ErrAgentStopped', 'ErrToolNotFound', 'ErrLLMCallFailed', 'ErrPoolFull', 'ErrEpisodeNotFound', 'ErrCommandBlocked', 'GetErrorCode'],
    ts: ['ErrAgentStopped', 'ErrToolNotFound', 'ErrLLMCallFailed', 'ErrPoolFull', 'ErrEpisodeNotFound', 'ErrCommandBlocked', 'getErrorCode'],
    tsSearchPaths: ['sdk/typescript/src'],
  },
  memory_store: {
    go: ['Memory', 'MemoryStore', 'NewMemory'],
    ts: ['Memory', 'MemoryStore'],
    tsSearchPaths: ['sdk/typescript/src'],
  },
  llm_provider: {
    go: ['Provider', 'CompletionRequest', 'CompletionResponse'],
    ts: ['Provider', 'LLMProvider'],
    tsSearchPaths: ['sdk/typescript/src/llm'],
  },
  health_check: {
    go: ['HealthChecker', 'NewHealthChecker'],
    ts: ['HealthChecker', 'healthCheck'],
    tsSearchPaths: ['sdk/typescript/src'],
  },
  chaos_config: {
    go: ['ChaosEngine', 'ChaosExperiment'],
    ts: ['ChaosEngine', 'Experiment'],
    tsSearchPaths: ['sdk/typescript/src/chaos'],
  },
  orchestration: {
    go: ['Pipeline', 'DAGWorkflow', 'NewPipeline'],
    ts: ['Pipeline', 'DAGWorkflow', 'DAGBuilder'],
    tsSearchPaths: ['sdk/typescript/src/orchestration'],
  },
  governance_quota: {
    go: ['QuotaManager', 'NewQuotaManager', 'TenantManager'],
    ts: ['QuotaManager', 'TokenBucket', 'TenantManager'],
    tsSearchPaths: ['sdk/typescript/src/governance'],
  },
  security_acl: {
    go: ['ACL', 'ACLRule', 'NewACL'],
    ts: ['ACL', 'Sandbox'],
    tsSearchPaths: ['sdk/typescript/src/security'],
  },
  guardrail_rules: {
    go: ['GuardrailEngine', 'NewGuardrailEngine'],
    ts: ['GuardrailEngine', 'PromptInjectionRule', 'PIIDetector'],
    tsSearchPaths: ['sdk/typescript/src/security'],
  },
  persist_checkpoint: {
    go: ['CheckpointStore', 'SQLiteCheckpointStore', 'InMemoryCheckpointStore'],
    ts: ['SQLiteCheckpointStore', 'AgentState'],
    tsSearchPaths: ['sdk/typescript/src/persist'],
  },
};

// ===== 主流程 =====

let hasError = false;

console.log('========================================');
console.log('  跨语言 API 兼容性检查');
console.log('========================================\n');

// 步骤 1: 运行 Go api-extract 生成最新的 API 契约
console.log('[1/5] 运行 Go API 提取器...');
const tmpContract = resolve(tmpdir(), 'go-api-contract-check.json');
try {
  execFileSync('go', ['run', './scripts/api-extract/', '-output', tmpContract],
    { cwd: resolve(REPO_ROOT, 'agentprimordia'), stdio: 'pipe', timeout: 120000 }
  );
  console.log('  OK: Go API 提取完成\n');
} catch (e) {
  console.error(`::error::Go API 提取失败: ${e.stderr?.toString() || e.message}`);
  process.exit(1);
}

// 步骤 2: 加载 API 契约和 cross-language-spec
console.log('[2/5] 加载 API 契约和跨语言规范...');
const contract = JSON.parse(readFileSync(tmpContract, 'utf-8'));
const specPath = resolve(REPO_ROOT, 'sdk/typescript/tests/shared/cross-language-spec.json');
const spec = JSON.parse(readFileSync(specPath, 'utf-8'));

// 步骤 3: 从 api-contract.json 收集所有 Go 公共符号
console.log('[3/5] 收集 Go 公共 API 符号...');
const goSymbols = new Set();
const goConstants = new Set();

for (const mod of contract.modules || []) {
  for (const t of mod.types || []) {
    goSymbols.add(t.name);
    // 收集方法名
    for (const m of t.methods || []) {
      goSymbols.add(`${t.name}.${m}`);
    }
  }
  for (const f of mod.functions || []) {
    goSymbols.add(f.name);
  }
  for (const c of mod.constants || []) {
    goSymbols.add(c.name);
    goConstants.add(c.name);
    // 也收集常量值（用于错误码前缀匹配）
    if (c.value) {
      goConstants.add(`${c.name}=${c.value}`);
    }
  }
  for (const v of mod.variables || []) {
    goSymbols.add(v.name);
  }
}
console.log(`  找到 ${goSymbols.size} 个 Go 符号, ${goConstants.size} 个常量\n`);

// 步骤 4: 验证每个测试套件声明的 Go 符号存在
console.log('[4/5] 验证 spec 中声明的 Go 符号存在于 api-contract.json...');
const suiteNames = spec.testSuites.map(s => s.name);
let goDriftCount = 0;

for (const suiteName of suiteNames) {
  const mapping = SUITE_SYMBOLS[suiteName];
  if (!mapping) {
    console.log(`  SKIP: ${suiteName} (无符号映射)`);
    continue;
  }

  const missing = [];
  for (const sym of mapping.go) {
    // 对于错误码前缀（如 AGENT_），检查常量名是否包含该前缀
    if (sym.endsWith('_')) {
      const found = [...goConstants].some(c => c.includes(sym));
      if (!found) {
        missing.push(sym);
      }
    } else if (!goSymbols.has(sym)) {
      missing.push(sym);
    }
  }

  if (missing.length > 0) {
    console.log(`  FAIL: ${suiteName} — Go 符号缺失: ${missing.join(', ')}`);
    console.log(`::error::[${suiteName}] Go API 漂移: 缺失符号 ${missing.join(', ')}`);
    goDriftCount += missing.length;
    hasError = true;
  } else {
    console.log(`  OK: ${suiteName} — Go 符号完整`);
  }
}

if (goDriftCount > 0) {
  console.log(`\n::error::共 ${goDriftCount} 个 Go 符号缺失，请运行 'make api-extract' 更新契约\n`);
} else {
  console.log('  OK: 所有 Go 符号验证通过\n');
}

// 步骤 5: 验证 TS 对应实现存在
console.log('[5/5] 验证 spec 中声明的 TS 符号存在于源码...');
let tsDriftCount = 0;

for (const suiteName of suiteNames) {
  const mapping = SUITE_SYMBOLS[suiteName];
  if (!mapping) continue;

  const searchPaths = mapping.tsSearchPaths || ['sdk/typescript/src'];
  const missing = [];

  for (const sym of mapping.ts) {
    let found = false;
    for (const searchPath of searchPaths) {
      const fullDir = resolve(REPO_ROOT, searchPath);
      if (!existsSync(fullDir)) continue;

      // 递归搜索 TS 源码文件中的符号声明
      try {
        const result = execFileSync('grep', ['-rl', sym, '--include=*.ts', '--include=*.tsx', fullDir],
          { encoding: 'utf-8', timeout: 30000 }
        );
        if (result.trim().length > 0) {
          found = true;
          break;
        }
      } catch {
        // grep 无结果不报错
      }
    }
    if (!found) {
      missing.push(sym);
    }
  }

  if (missing.length > 0) {
    console.log(`  FAIL: ${suiteName} — TS 符号缺失: ${missing.join(', ')}`);
    console.log(`::error::[${suiteName}] TS 实现漂移: 缺失符号 ${missing.join(', ')}`);
    tsDriftCount += missing.length;
    hasError = true;
  } else {
    console.log(`  OK: ${suiteName} — TS 符号完整`);
  }
}

console.log('');

// ===== 结果汇总 =====
if (hasError) {
  console.log(`::error::跨语言 API 兼容性检查失败！`);
  console.log(`  Go 符号缺失: ${goDriftCount}`);
  console.log(`  TS 符号缺失: ${tsDriftCount}`);
  console.log('');
  console.log('  请确保 cross-language-spec.json 中声明的符号在两侧 SDK 中均存在。');
  console.log('  如果是有意的 API 变更，请同步更新 spec 和两侧实现。');
  process.exit(1);
}

console.log('========================================');
console.log('  跨语言 API 兼容性检查通过');
console.log('========================================');

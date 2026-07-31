/**
 * 工具系统示例 — 工具注册、调用、权限控制
 *
 * 运行: npx tsx examples/with-tools.ts
 */
import { ToolRegistry } from '../src/tools/registry.js';
import { Executor } from '../src/tools/executor.js';

async function main() {
  console.log('=== AgentPrimordia TS SDK: Tools Example ===\n');

  const registry = new ToolRegistry();

  // 注册计算器工具
  registry.register({
    name: 'calculator',
    description: '执行基本数学运算',
    parameters: {
      type: 'object',
      properties: {
        expression: { type: 'string', description: '数学表达式，如 "2+3*4"' },
      },
      required: ['expression'],
    },
    execute: async (args: Record<string, unknown>) => {
      const expr = String(args.expression ?? '');
      // 安全实现：仅支持数字和基本运算符
      if (!/^[\d+\-*/.() ]+$/.test(expr)) {
        return { content: '错误：表达式包含非法字符' };
      }
      try {
        // 使用安全的整数运算（示例用，生产环境建议用 mathjs 等库）
        const result = safeEval(expr);
        return { content: `计算结果: ${result}` };
      } catch {
        return { content: '错误：无法解析表达式' };
      }
    },
  });

  // 注册天气查询工具（模拟）
  registry.register({
    name: 'weather',
    description: '查询城市天气',
    parameters: {
      type: 'object',
      properties: {
        city: { type: 'string', description: '城市名称' },
      },
      required: ['city'],
    },
    execute: async (args: Record<string, unknown>) => {
      return { content: `${args.city}: 晴天 25°C，湿度 60%` };
    },
  });

  console.log(`已注册工具: ${registry.list().join(', ')}`);
  console.log(`工具数量: ${registry.count()}\n`);

  // 执行工具调用
  const executor = new Executor(registry);
  const calcResult = await executor.execute({ name: 'calculator', arguments: '{"expression": "2+3*4"}' });
  console.log(`Calculator: ${calcResult?.content}`);

  const weatherResult = await executor.execute({ name: 'weather', arguments: '{"city": "北京"}' });
  console.log(`Weather: ${weatherResult?.content}`);

  // 工具定义（供 LLM function calling）
  const definitions = registry.definitions();
  console.log(`\n工具定义数量: ${definitions.length}`);
  console.log(`第一个工具: ${definitions[0]?.function?.name}`);

  console.log('\n--- Done ---');
}

main().catch(console.error);

/** 安全的简单四则运算（仅支持 + - * / 和括号） */
function safeEval(expr: string): number {
  // 移除空格
  const s = expr.replace(/\s/g, '');
  // 递归下降解析器
  let pos = 0;

  function parseExpr(): number {
    let result = parseTerm();
    while (pos < s.length && (s[pos] === '+' || s[pos] === '-')) {
      const op = s[pos++];
      const right = parseTerm();
      result = op === '+' ? result + right : result - right;
    }
    return result;
  }

  function parseTerm(): number {
    let result = parseFactor();
    while (pos < s.length && (s[pos] === '*' || s[pos] === '/')) {
      const op = s[pos++];
      const right = parseFactor();
      result = op === '*' ? result * right : result / right;
    }
    return result;
  }

  function parseFactor(): number {
    if (s[pos] === '(') {
      pos++; // skip '('
      const result = parseExpr();
      pos++; // skip ')'
      return result;
    }
    const start = pos;
    while (pos < s.length && /[\d.]/.test(s[pos])) pos++;
    return parseFloat(s.slice(start, pos));
  }

  return parseExpr();
}

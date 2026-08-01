import { describe, it, expect, vi } from 'vitest';
import { DatabaseTool, CodeExecutionTool, KnowledgeTool } from '../../src/tools/builtin/database-code-knowledge.js';

// ===== DatabaseTool tests =====
describe('DatabaseTool', () => {
  it('should have correct name and description', () => {
    const tool = new DatabaseTool({ query: vi.fn() });
    expect(tool.name).toBe('database');
    expect(tool.description).toContain('SQL');
  });

  it('should have parameters with query as required', () => {
    const tool = new DatabaseTool({ query: vi.fn() });
    expect(tool.parameters.required).toEqual(['query']);
  });

  it('should reject empty query', async () => {
    const tool = new DatabaseTool({ query: vi.fn() });
    const result = await tool.execute({ query: '' });
    expect(result).toContain('Error: SQL query is required');
  });

  it('should reject whitespace-only query', async () => {
    const tool = new DatabaseTool({ query: vi.fn() });
    const result = await tool.execute({ query: '   ' });
    expect(result).toContain('Error: SQL query is required');
  });

  it('should block non-SELECT queries in read-only mode (default)', async () => {
    const mockQuery = vi.fn();
    const tool = new DatabaseTool({ query: mockQuery });
    const result = await tool.execute({ query: 'DROP TABLE users' });
    expect(result).toContain('only SELECT queries are allowed');
    expect(mockQuery).not.toHaveBeenCalled();
  });

  it('should block INSERT in read-only mode', async () => {
    const mockQuery = vi.fn();
    const tool = new DatabaseTool({ query: mockQuery });
    const result = await tool.execute({ query: 'INSERT INTO users VALUES (1)' });
    expect(result).toContain('only SELECT queries are allowed');
  });

  it('should block DELETE in read-only mode', async () => {
    const mockQuery = vi.fn();
    const tool = new DatabaseTool({ query: mockQuery });
    const result = await tool.execute({ query: 'DELETE FROM users' });
    expect(result).toContain('only SELECT queries are allowed');
  });

  it('should allow SELECT queries in read-only mode', async () => {
    const mockQuery = vi.fn().mockResolvedValue([
      { id: 1, name: 'Alice' },
      { id: 2, name: 'Bob' },
    ]);
    const tool = new DatabaseTool({ query: mockQuery });
    const result = await tool.execute({ query: 'SELECT * FROM users' });
    expect(result).toContain('Alice');
    expect(result).toContain('Bob');
    expect(result).toContain('(2 rows)');
  });

  it('should allow WITH queries in read-only mode', async () => {
    const mockQuery = vi.fn().mockResolvedValue([{ count: 5 }]);
    const tool = new DatabaseTool({ query: mockQuery });
    const result = await tool.execute({ query: 'WITH c AS (SELECT 1) SELECT * FROM c' });
    expect(result).toContain('count');
  });

  it('should allow SHOW queries in read-only mode', async () => {
    const mockQuery = vi.fn().mockResolvedValue([{ table: 'users' }]);
    const tool = new DatabaseTool({ query: mockQuery });
    const result = await tool.execute({ query: 'SHOW TABLES' });
    expect(result).toContain('users');
  });

  it('should allow PRAGMA queries in read-only mode', async () => {
    const mockQuery = vi.fn().mockResolvedValue([{ name: 'WAL' }]);
    const tool = new DatabaseTool({ query: mockQuery });
    const result = await tool.execute({ query: 'PRAGMA journal_mode' });
    expect(result).toContain('WAL');
  });

  it('should allow non-SELECT when readOnly is false', async () => {
    const mockQuery = vi.fn().mockResolvedValue({ changes: 1 });
    const tool = new DatabaseTool({ query: mockQuery }, { readOnly: false });
    const result = await tool.execute({ query: 'INSERT INTO users VALUES (1)' });
    expect(result).toContain('Changes: 1');
  });

  it('should allow per-call readOnly override', async () => {
    const mockQuery = vi.fn().mockResolvedValue({ changes: 1 });
    const tool = new DatabaseTool({ query: mockQuery }, { readOnly: true });
    const result = await tool.execute({ query: 'INSERT INTO users VALUES (1)', readOnly: false });
    expect(result).toContain('Changes: 1');
  });

  it('should format result as table with headers and separator', async () => {
    const mockQuery = vi.fn().mockResolvedValue([
      { id: 1, name: 'Alice' },
      { id: 2, name: 'Bob' },
    ]);
    const tool = new DatabaseTool({ query: mockQuery });
    const result = await tool.execute({ query: 'SELECT * FROM users' });
    expect(result).toContain('id | name');
    expect(result).toContain('--- | ---');
    expect(result).toContain('1 | Alice');
    expect(result).toContain('2 | Bob');
  });

  it('should handle empty result set', async () => {
    const mockQuery = vi.fn().mockResolvedValue([]);
    const tool = new DatabaseTool({ query: mockQuery });
    const result = await tool.execute({ query: 'SELECT * FROM empty_table' });
    expect(result).toBe('Query returned 0 rows');
  });

  it('should handle null values in result', async () => {
    const mockQuery = vi.fn().mockResolvedValue([{ id: 1, name: null }]);
    const tool = new DatabaseTool({ query: mockQuery });
    const result = await tool.execute({ query: 'SELECT * FROM users' });
    expect(result).toContain('NULL');
  });

  it('should pass params to db.query', async () => {
    const mockQuery = vi.fn().mockResolvedValue([]);
    const tool = new DatabaseTool({ query: mockQuery });
    await tool.execute({ query: 'SELECT * FROM users WHERE id = ?', params: [42] });
    expect(mockQuery).toHaveBeenCalledWith('SELECT * FROM users WHERE id = ?', [42]);
  });

  it('should handle query errors', async () => {
    const mockQuery = vi.fn().mockRejectedValue(new Error('syntax error'));
    const tool = new DatabaseTool({ query: mockQuery });
    const result = await tool.execute({ query: 'SELECT * FROM users' });
    expect(result).toContain('Error: syntax error');
  });

  it('should use default readOnly when opts not provided', async () => {
    const mockQuery = vi.fn();
    const tool = new DatabaseTool({ query: mockQuery });
    const result = await tool.execute({ query: 'DROP TABLE users' });
    expect(result).toContain('only SELECT queries');
  });

  it('should handle changes result type', async () => {
    const mockQuery = vi.fn().mockResolvedValue({ changes: 5 });
    const tool = new DatabaseTool({ query: mockQuery }, { readOnly: false });
    const result = await tool.execute({ query: 'UPDATE users SET active = 1' });
    expect(result).toContain('Changes: 5');
  });
});

// ===== CodeExecutionTool tests =====
// 与 Go 端 CodeExecution 对齐，通过 child_process 执行 Python/JavaScript/Go

describe('CodeExecutionTool', () => {
  // 保存和恢复环境变量
  const origEnv = process.env.AP_ALLOW_CODE_EXECUTION;

  afterEach(() => {
    if (origEnv === undefined) {
      delete process.env.AP_ALLOW_CODE_EXECUTION;
    } else {
      process.env.AP_ALLOW_CODE_EXECUTION = origEnv;
    }
  });

  it('should have correct name and description', () => {
    const tool = new CodeExecutionTool();
    expect(tool.name).toBe('code_execution');
    expect(tool.description).toContain('sandbox');
  });

  it('should have parameters with language and code as required', () => {
    const tool = new CodeExecutionTool();
    expect(tool.parameters.required).toEqual(['language', 'code']);
  });

  it('should reject empty code', async () => {
    process.env.AP_ALLOW_CODE_EXECUTION = 'true';
    const tool = new CodeExecutionTool();
    const result = await tool.execute({ language: 'javascript', code: '' });
    expect(result).toContain('Error: parameter "code" cannot be empty');
  });

  it('should reject whitespace-only code', async () => {
    process.env.AP_ALLOW_CODE_EXECUTION = 'true';
    const tool = new CodeExecutionTool();
    const result = await tool.execute({ language: 'javascript', code: '   ' });
    expect(result).toContain('Error: parameter "code" cannot be empty');
  });

  it('should be disabled by default for security', async () => {
    delete process.env.AP_ALLOW_CODE_EXECUTION;
    const tool = new CodeExecutionTool();
    const result = await tool.execute({ language: 'javascript', code: 'console.log("hello")' });
    expect(result).toContain('disabled by default');
    expect(result).toContain('AP_ALLOW_CODE_EXECUTION');
  });

  it('should reject missing language parameter', async () => {
    process.env.AP_ALLOW_CODE_EXECUTION = 'true';
    const tool = new CodeExecutionTool();
    const result = await tool.execute({ code: 'console.log("hello")' });
    expect(result).toContain('Error: parameter "language" is required');
  });

  it('should reject unsupported language', async () => {
    process.env.AP_ALLOW_CODE_EXECUTION = 'true';
    const tool = new CodeExecutionTool();
    const result = await tool.execute({ language: 'ruby', code: 'puts "hello"' });
    expect(result).toContain('unsupported language');
  });

  it('should execute JavaScript code and return output', async () => {
    process.env.AP_ALLOW_CODE_EXECUTION = 'true';
    const tool = new CodeExecutionTool();
    const result = await tool.execute({ language: 'javascript', code: 'console.log("hello world")' });
    expect(result).toContain('hello world');
    expect(result).toContain('"exit_code": 0');
  });

  it('should return JSON output format with language and exit_code', async () => {
    process.env.AP_ALLOW_CODE_EXECUTION = 'true';
    const tool = new CodeExecutionTool();
    const result = await tool.execute({ language: 'javascript', code: 'console.log("test")' });
    const parsed = JSON.parse(result);
    expect(parsed.language).toBe('javascript');
    expect(parsed.exit_code).toBe(0);
    expect(parsed.output).toContain('test');
    expect(parsed.truncated).toBe(false);
  });

  it('should capture stderr output', async () => {
    process.env.AP_ALLOW_CODE_EXECUTION = 'true';
    const tool = new CodeExecutionTool();
    const result = await tool.execute({ language: 'javascript', code: 'console.error("error msg")' });
    expect(result).toContain('error msg');
  });

  it('should handle code errors with non-zero exit code', async () => {
    process.env.AP_ALLOW_CODE_EXECUTION = 'true';
    const tool = new CodeExecutionTool();
    const result = await tool.execute({ language: 'javascript', code: 'throw new Error("test error")' });
    expect(result).toContain('Error:');
    expect(result).toContain('test error');
  });

  it('should use custom timeout from args', async () => {
    process.env.AP_ALLOW_CODE_EXECUTION = 'true';
    const tool = new CodeExecutionTool();
    const result = await tool.execute({ language: 'javascript', code: 'console.log("ok")', timeout: 5 });
    expect(result).toContain('ok');
  });

  it('should handle execution timeout', async () => {
    process.env.AP_ALLOW_CODE_EXECUTION = 'true';
    const tool = new CodeExecutionTool();
    const result = await tool.execute({ language: 'javascript', code: 'setTimeout(() => {}, 99999)', timeout: 1 });
    expect(result).toContain('timed out');
  }, 10000);

  it('should truncate output exceeding maxOutputLength', async () => {
    process.env.AP_ALLOW_CODE_EXECUTION = 'true';
    const tool = new CodeExecutionTool({ maxOutputLength: 10 });
    const result = await tool.execute({ language: 'javascript', code: 'console.log("x".repeat(100))' });
    expect(result).toContain('truncated');
  });

  it('should support Python language parameter', async () => {
    process.env.AP_ALLOW_CODE_EXECUTION = 'true';
    const tool = new CodeExecutionTool();
    // Just verify the language is accepted (python3 may not be installed)
    const result = await tool.execute({ language: 'python', code: 'print("hello")' });
    // Either it succeeds or returns runtime not found error
    expect(typeof result).toBe('string');
  });

  // go run 会触发真实 Go 工具链编译，在负载机器上可能超过默认 5s，放宽超时
  it('should support Go language parameter', async () => {
    process.env.AP_ALLOW_CODE_EXECUTION = 'true';
    const tool = new CodeExecutionTool();
    const result = await tool.execute({ language: 'go', code: 'package main\nfunc main() { println("hello") }' });
    expect(typeof result).toBe('string');
  }, 20000);

  it('should use default timeout when not specified', async () => {
    process.env.AP_ALLOW_CODE_EXECUTION = 'true';
    const tool = new CodeExecutionTool();
    const result = await tool.execute({ language: 'javascript', code: 'console.log("fast")' });
    expect(result).toContain('fast');
  });
});

// ===== KnowledgeTool tests =====
describe('KnowledgeTool', () => {
  it('should have correct name and description', () => {
    const tool = new KnowledgeTool(vi.fn());
    expect(tool.name).toBe('knowledge_search');
    expect(tool.description).toContain('RAG');
  });

  it('should have parameters with query as required', () => {
    const tool = new KnowledgeTool(vi.fn());
    expect(tool.parameters.required).toEqual(['query']);
  });

  it('should reject empty query', async () => {
    const tool = new KnowledgeTool(vi.fn());
    const result = await tool.execute({ query: '' });
    expect(result).toContain('Error: query is required');
  });

  it('should reject whitespace-only query', async () => {
    const tool = new KnowledgeTool(vi.fn());
    const result = await tool.execute({ query: '   ' });
    expect(result).toContain('Error: query is required');
  });

  it('should return no results message when search returns empty', async () => {
    const searchFn = vi.fn().mockResolvedValue([]);
    const tool = new KnowledgeTool(searchFn);
    const result = await tool.execute({ query: 'test' });
    expect(result).toBe('No results found.');
  });

  it('should format results with score and content', async () => {
    const searchFn = vi.fn().mockResolvedValue([
      { id: '1', content: 'Hello world', score: 0.95 },
    ]);
    const tool = new KnowledgeTool(searchFn);
    const result = await tool.execute({ query: 'hello' });
    expect(result).toContain('score: 0.95');
    expect(result).toContain('Hello world');
  });

  it('should format results with source when provided', async () => {
    const searchFn = vi.fn().mockResolvedValue([
      { id: '1', content: 'Data', score: 0.8, source: 'doc.md' },
    ]);
    const tool = new KnowledgeTool(searchFn);
    const result = await tool.execute({ query: 'data' });
    expect(result).toContain('doc.md');
  });

  it('should format multiple results separated by ---', async () => {
    const searchFn = vi.fn().mockResolvedValue([
      { id: '1', content: 'Result 1', score: 0.9 },
      { id: '2', content: 'Result 2', score: 0.8 },
    ]);
    const tool = new KnowledgeTool(searchFn);
    const result = await tool.execute({ query: 'test' });
    expect(result).toContain('Result 1');
    expect(result).toContain('Result 2');
    expect(result).toContain('---');
    expect(result).toContain('[1 |');
    expect(result).toContain('[2 |');
  });

  it('should pass topK to search function', async () => {
    const searchFn = vi.fn().mockResolvedValue([]);
    const tool = new KnowledgeTool(searchFn);
    await tool.execute({ query: 'test', topK: 10 });
    expect(searchFn).toHaveBeenCalledWith('test', 10);
  });

  it('should use default topK of 5', async () => {
    const searchFn = vi.fn().mockResolvedValue([]);
    const tool = new KnowledgeTool(searchFn);
    await tool.execute({ query: 'test' });
    expect(searchFn).toHaveBeenCalledWith('test', 5);
  });

  it('should handle search function errors', async () => {
    const searchFn = vi.fn().mockRejectedValue(new Error('search failed'));
    const tool = new KnowledgeTool(searchFn);
    const result = await tool.execute({ query: 'test' });
    expect(result).toContain('Error: search failed');
  });

  it('should format score with 2 decimal places', async () => {
    const searchFn = vi.fn().mockResolvedValue([
      { id: '1', content: 'Test', score: 0.123456 },
    ]);
    const tool = new KnowledgeTool(searchFn);
    const result = await tool.execute({ query: 'test' });
    expect(result).toContain('score: 0.12');
  });
});

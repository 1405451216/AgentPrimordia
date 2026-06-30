import * as os from 'node:os';
import * as path from 'node:path';
import * as fs from 'node:fs';
import { execFile } from 'node:child_process';
import type { Tool } from '../../types.js';

/**
 * Database tool — SQL database queries.
 * Supports any database with a query interface.
 */
export class DatabaseTool implements Tool {
  name = 'database';
  description = 'Execute SQL queries on a database';
  parameters = {
    type: 'object',
    properties: {
      query: { type: 'string', description: 'SQL query to execute' },
      params: { type: 'array', items: {}, description: 'Query parameters (optional)' },
      readOnly: { type: 'boolean', description: 'If true, only SELECT queries are allowed (default: true)' },
    },
    required: ['query'],
  };

  private db: { query: (sql: string, params?: unknown[]) => Promise<unknown[]> | Promise<{ changes: number }> };
  private defaultReadOnly: boolean;

  constructor(
    db: { query: (sql: string, params?: unknown[]) => Promise<unknown[]> | Promise<{ changes: number }> },
    opts?: { readOnly?: boolean }
  ) {
    this.db = db;
    this.defaultReadOnly = opts?.readOnly ?? true;
  }

  async execute(args: Record<string, unknown>): Promise<string> {
    const query = args.query as string;
    if (!query?.trim()) return 'Error: SQL query is required';

    const params = (args.params as unknown[]) ?? [];
    const readOnly = (args.readOnly as boolean) ?? this.defaultReadOnly;

    // Check read-only constraint
    if (readOnly) {
      const upperQuery = query.trim().toUpperCase();
      if (!upperQuery.startsWith('SELECT') && !upperQuery.startsWith('WITH') && !upperQuery.startsWith('SHOW') && !upperQuery.startsWith('PRAGMA')) {
        return 'Error: only SELECT queries are allowed in read-only mode';
      }
    }

    try {
      const result = await this.db.query(query, params);
      if (Array.isArray(result)) {
        if (result.length === 0) return 'Query returned 0 rows';
        // Format as table
        const cols = Object.keys(result[0] as Record<string, unknown>);
        const header = cols.join(' | ');
        const separator = cols.map(() => '---').join(' | ');
        const rows = (result as Record<string, unknown>[]).map((row) =>
          cols.map((c) => String(row[c] ?? 'NULL')).join(' | ')
        );
        return `${header}\n${separator}\n${rows.join('\n')}\n\n(${result.length} rows)`;
      }
      return `Query executed successfully. Changes: ${(result as { changes: number }).changes}`;
    } catch (err) {
      return `Error: ${(err as Error).message}`;
    }
  }
}

// ===== P3-3: Code Execution Tool（与 Go 端 code_execution.go 对齐） =====

const CODE_EXEC_DEFAULT_TIMEOUT = 10; // seconds
const CODE_EXEC_MAX_OUTPUT_SIZE = 10 * 1024; // 10KB

/** 根据语言返回运行时命令和临时文件扩展名（与 Go 端 runtimeCommand 对齐） */
function runtimeCommand(language: string): { cmd: string; ext: string } {
  switch (language) {
    case 'python':
      return { cmd: process.platform === 'win32' ? 'python' : 'python3', ext: '.py' };
    case 'javascript':
      return { cmd: 'node', ext: '.js' };
    case 'go':
      return { cmd: 'go', ext: '.go' };
    default:
      return { cmd: '', ext: '' };
  }
}

/** 返回语言的显示名称 */
function languageDisplayName(lang: string): string {
  switch (lang) {
    case 'python': return 'Python';
    case 'javascript': return 'Node.js';
    case 'go': return 'Go';
    default: return lang;
  }
}

/** 构建代码执行的环境变量（隔离，与 Go 端 buildCodeExecEnv 对齐） */
function buildCodeExecEnv(language: string): Record<string, string> {
  const env: Record<string, string> = { PATH: process.env.PATH ?? '' };
  for (const name of ['HOME', 'TEMP', 'TMP', 'USERPROFILE']) {
    const v = process.env[name];
    if (v) env[name] = v;
  }
  // Go 运行时额外需要 GOPATH/GOROOT/GOCACHE 等
  if (language === 'go') {
    for (const name of ['GOPATH', 'GOROOT', 'GOCACHE', 'GOMODCACHE', 'LOCALAPPDATA', 'APPDATA', 'SYSTEMROOT', 'USERPROFILE']) {
      const v = process.env[name];
      if (v) env[name] = v;
    }
  }
  return env;
}

/**
 * Code execution tool — run code in a sandboxed environment.
 *
 * 与 Go 端 CodeExecution 对齐，支持 Python、JavaScript、Go 三种语言。
 * 通过 child_process 执行，具有超时控制、输出截断、环境变量隔离。
 *
 * 安全警告：此工具 NOT a sandbox，代码直接在主机上运行。
 * 仅在可信环境中启用（设置 AP_ALLOW_CODE_EXECUTION=true）。
 */
export class CodeExecutionTool implements Tool {
  name = 'code_execution';
  description = 'Execute code with timeout and output limits. Supports Python, JavaScript, and Go. ' +
    'WARNING: This is NOT a security sandbox. Code runs directly on the host. ' +
    'Enable only in trusted environments by setting AP_ALLOW_CODE_EXECUTION=true.';
  parameters = {
    type: 'object',
    properties: {
      language: { type: 'string', enum: ['python', 'javascript', 'go'], description: 'Programming language: python, javascript, or go' },
      code: { type: 'string', description: 'Source code to execute' },
      timeout: { type: 'number', description: 'Execution timeout in seconds (default: 10)' },
    },
    required: ['language', 'code'],
  };

  private defaultTimeout: number;
  private maxOutputSize: number;

  constructor(opts?: { timeoutMs?: number; maxOutputLength?: number; defaultTimeoutSec?: number }) {
    this.defaultTimeout = opts?.defaultTimeoutSec ?? CODE_EXEC_DEFAULT_TIMEOUT;
    this.maxOutputSize = opts?.maxOutputLength ?? CODE_EXEC_MAX_OUTPUT_SIZE;
  }

  async execute(args: Record<string, unknown>): Promise<string> {
    // 安全门控
    if (process.env.AP_ALLOW_CODE_EXECUTION !== 'true') {
      return 'code_execution is disabled by default for security reasons. ' +
        'It is NOT a sandbox and runs arbitrary code on the host. ' +
        'Set AP_ALLOW_CODE_EXECUTION=true to enable it only in trusted environments.';
    }

    const language = (args.language as string)?.toLowerCase().trim();
    if (!language) return 'Error: parameter "language" is required';

    const code = args.code as string;
    if (!code?.trim()) return 'Error: parameter "code" cannot be empty';

    const timeoutSec = (args.timeout as number) ?? this.defaultTimeout;

    // 确定运行时命令和文件扩展名
    const { cmd: cmdName, ext } = runtimeCommand(language);
    if (!cmdName) {
      return `Error: unsupported language: ${language} (supported: python, javascript, go)`;
    }

    // 创建临时文件
    const tmpDir = os.tmpdir();
    const tmpFile = path.join(tmpDir, `code_exec_${Date.now()}_${Math.random().toString(36).slice(2, 8)}${ext}`);

    try {
      fs.writeFileSync(tmpFile, code, 'utf-8');

      // 构建执行命令参数
      const cmdArgs = language === 'go' ? ['run', tmpFile] : [tmpFile];

      // 执行
      const result = await new Promise<{ stdout: string; exitCode: number; timedOut: boolean }>((resolve) => {
        const proc = execFile(cmdName, cmdArgs, {
          env: buildCodeExecEnv(language),
          cwd: os.tmpdir(),
          timeout: timeoutSec * 1000,
          maxBuffer: this.maxOutputSize * 2,
        }, (err, stdout, stderr) => {
          const output = stdout + (stderr ? '\n' + stderr : '');
          if (err) {
            const exitCode = (err as NodeJS.ErrnoException).code ?? 1;
            // Node.js execFile timeout: err.killed=true, err.signal='SIGTERM'
            const execErr = err as NodeJS.ErrnoException & { killed?: boolean; signal?: string };
            const timedOut = execErr.killed === true && execErr.signal === 'SIGTERM';
            resolve({ stdout: output, exitCode: typeof exitCode === 'number' ? exitCode : 1, timedOut });
          } else {
            resolve({ stdout: output, exitCode: 0, timedOut: false });
          }
        });
        // Handle timeout kill
        proc.on('error', () => {
          resolve({ stdout: `runtime '${cmdName}' not found. Please install ${languageDisplayName(language)} and ensure it is in your PATH.`, exitCode: 1, timedOut: false });
        });
      });

      // 超时处理
      if (result.timedOut) {
        const outputStr = result.stdout || '(no output before timeout)';
        return JSON.stringify({
          language,
          exit_code: result.exitCode,
          output: `execution timed out after ${timeoutSec} seconds\n${outputStr}`,
          truncated: false,
        }, null, 2);
      }

      // 输出截断
      let outputStr = result.stdout;
      let truncated = false;
      if (this.maxOutputSize > 0 && outputStr.length > this.maxOutputSize) {
        outputStr = outputStr.slice(0, this.maxOutputSize) + '\n... [output truncated, exceeded 10KB limit]';
        truncated = true;
      }

      const resultObj = {
        language,
        exit_code: result.exitCode,
        output: outputStr,
        truncated,
      };

      const resultJSON = JSON.stringify(resultObj, null, 2);

      if (result.exitCode !== 0) {
        return `Error: ${resultJSON}`;
      }
      return resultJSON;
    } catch (err) {
      return `Error: ${(err as Error).message}`;
    } finally {
      // 清理临时文件
      try { fs.unlinkSync(tmpFile); } catch { /* ignore */ }
    }
  }
}

/**
 * Knowledge tool — RAG on_demand knowledge retrieval.
 */
export class KnowledgeTool implements Tool {
  name = 'knowledge_search';
  description = 'Search the knowledge base for relevant information using RAG';
  parameters = {
    type: 'object',
    properties: {
      query: { type: 'string', description: 'Search query' },
      topK: { type: 'number', description: 'Number of results to return (default: 5)' },
    },
    required: ['query'],
  };

  private searchFn: (query: string, topK: number) => Promise<{ id: string; content: string; score: number; source?: string }[]>;

  constructor(searchFn: (query: string, topK: number) => Promise<{ id: string; content: string; score: number; source?: string }[]>) {
    this.searchFn = searchFn;
  }

  async execute(args: Record<string, unknown>): Promise<string> {
    const query = args.query as string;
    if (!query?.trim()) return 'Error: query is required';

    const topK = (args.topK as number) ?? 5;

    try {
      const results = await this.searchFn(query, topK);
      if (results.length === 0) return 'No results found.';

      return results.map((r, i) =>
        `[${i + 1} | score: ${r.score.toFixed(2)}${r.source ? ` | ${r.source}` : ''}]\n${r.content}`
      ).join('\n\n---\n\n');
    } catch (err) {
      return `Error: ${(err as Error).message}`;
    }
  }
}

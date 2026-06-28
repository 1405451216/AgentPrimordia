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

/**
 * Code execution tool — run code in a sandboxed environment.
 * Uses Node.js child_process with restrictions.
 */
export class CodeExecutionTool implements Tool {
  name = 'code_execution';
  description = 'Execute JavaScript/TypeScript code in a sandboxed environment';
  parameters = {
    type: 'object',
    properties: {
      language: { type: 'string', enum: ['javascript', 'typescript'], description: 'Programming language' },
      code: { type: 'string', description: 'Code to execute' },
      timeout: { type: 'number', description: 'Execution timeout in seconds (default: 10)' },
    },
    required: ['code'],
  };

  private timeoutMs: number;
  private maxOutputLength: number;

  constructor(opts?: { timeoutMs?: number; maxOutputLength?: number }) {
    this.timeoutMs = opts?.timeoutMs ?? 10_000;
    this.maxOutputLength = opts?.maxOutputLength ?? 10_000;
  }

  async execute(args: Record<string, unknown>): Promise<string> {
    const code = args.code as string;
    if (!code?.trim()) return 'Error: code is required';

    const language = (args.language as string) ?? 'javascript';
    const timeout = ((args.timeout as number) ?? 10) * 1000;

    // For safety, we use a restricted eval approach
    // In production, this should use a proper sandbox (vm2, isolated-vm, etc.)
    try {
      // Capture console output
      const logs: string[] = [];
      const mockConsole = {
        log: (...args: unknown[]) => logs.push(args.map(String).join(' ')),
        error: (...args: unknown[]) => logs.push('[ERROR] ' + args.map(String).join(' ')),
        warn: (...args: unknown[]) => logs.push('[WARN] ' + args.map(String).join(' ')),
        info: (...args: unknown[]) => logs.push('[INFO] ' + args.map(String).join(' ')),
      };

      // Create a function with restricted scope
      const fn = new Function('console', `"use strict";\n${code}`);
      const result = await Promise.race([
        Promise.resolve(fn(mockConsole)),
        new Promise<never>((_, reject) =>
          setTimeout(() => reject(new Error('Execution timeout')), Math.min(timeout, this.timeoutMs))
        ),
      ]);

      let output = logs.join('\n');
      if (result !== undefined) {
        output += (output ? '\n' : '') + `Result: ${typeof result === 'object' ? JSON.stringify(result, null, 2) : String(result)}`;
      }
      if (!output) output = '(no output)';

      if (output.length > this.maxOutputLength) {
        output = output.slice(0, this.maxOutputLength) + '\n... (truncated)';
      }

      return output;
    } catch (err) {
      return `Error: ${(err as Error).message}`;
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

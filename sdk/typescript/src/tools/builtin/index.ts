import * as fs from 'node:fs';
import type { Tool } from '../../types.js';
import { FileSystemTool, type FileSystemConfig } from './filesystem.js';
import { ShellTool, type ShellConfig } from './shell.js';
import { WebTool, APITool, type WebConfig } from './web-api.js';
import { DatabaseTool, CodeExecutionTool, KnowledgeTool } from './database-code-knowledge.js';
import { ToolRegistry } from '../registry.js';

export { FileSystemTool, ShellTool, WebTool, APITool, DatabaseTool, CodeExecutionTool, KnowledgeTool };
export type { FileSystemConfig, ShellConfig, WebConfig };

// ===== Document Loaders =====

export interface LoadedDocument {
  content: string;
  metadata: {
    source: string;
    format: string;
    size: number;
    [key: string]: unknown;
  };
}

/** JSON document loader. */
export class JSONLoader {
  async load(filePath: string): Promise<LoadedDocument> {
    const content = fs.readFileSync(filePath, 'utf-8');
    const parsed = JSON.parse(content);
    return {
      content: JSON.stringify(parsed, null, 2),
      metadata: { source: filePath, format: 'json', size: content.length },
    };
  }

  async loadFromString(content: string): Promise<LoadedDocument> {
    const parsed = JSON.parse(content);
    return {
      content: JSON.stringify(parsed, null, 2),
      metadata: { source: 'inline', format: 'json', size: content.length },
    };
  }
}

/** CSV document loader. */
export class CSVLoader {
  async load(filePath: string): Promise<LoadedDocument> {
    const content = fs.readFileSync(filePath, 'utf-8');
    return this.loadFromString(content, filePath);
  }

  async loadFromString(content: string, source = 'inline'): Promise<LoadedDocument> {
    const lines = content.split('\n').filter((l) => l.trim());
    if (lines.length === 0) {
      return { content: '', metadata: { source, format: 'csv', size: 0 } };
    }

    // Parse CSV (simple parser, handles quoted fields)
    const rows: string[][] = [];
    for (const line of lines) {
      const row: string[] = [];
      let current = '';
      let inQuotes = false;
      for (let i = 0; i < line.length; i++) {
        const ch = line[i];
        if (ch === '"') {
          if (inQuotes && line[i + 1] === '"') { current += '"'; i++; }
          else { inQuotes = !inQuotes; }
        } else if (ch === ',' && !inQuotes) {
          row.push(current);
          current = '';
        } else {
          current += ch;
        }
      }
      row.push(current);
      rows.push(row);
    }

    const headers = rows[0];
    const dataRows = rows.slice(1);
    const formatted = dataRows.map((row) => {
      const obj: Record<string, string> = {};
      headers.forEach((h, i) => { obj[h] = row[i] ?? ''; });
      return JSON.stringify(obj);
    }).join('\n');

    return {
      content: formatted,
      metadata: { source, format: 'csv', size: content.length, rows: dataRows.length, columns: headers.length },
    };
  }
}

/** HTML document loader (extracts text). */
export class HTMLLoader {
  async load(filePath: string): Promise<LoadedDocument> {
    const content = fs.readFileSync(filePath, 'utf-8');
    return this.loadFromString(content, filePath);
  }

  async loadFromString(content: string, source = 'inline'): Promise<LoadedDocument> {
    // Simple HTML text extraction (remove tags)
    const text = content
      .replace(/<script[\s\S]*?<\/script>/gi, '')
      .replace(/<style[\s\S]*?<\/style>/gi, '')
      .replace(/<[^>]+>/g, ' ')
      .replace(/&nbsp;/g, ' ')
      .replace(/&amp;/g, '&')
      .replace(/&lt;/g, '<')
      .replace(/&gt;/g, '>')
      .replace(/&quot;/g, '"')
      .replace(/&#39;/g, "'")
      .replace(/\s+/g, ' ')
      .trim();

    return {
      content: text,
      metadata: { source, format: 'html', size: content.length },
    };
  }
}

/** Markdown document loader. */
export class MarkdownLoader {
  async load(filePath: string): Promise<LoadedDocument> {
    const content = fs.readFileSync(filePath, 'utf-8');
    return {
      content,
      metadata: { source: filePath, format: 'markdown', size: content.length },
    };
  }
}

/** Text splitter — split text into chunks. */
export class TextSplitter {
  private chunkSize: number;
  private chunkOverlap: number;
  private separator: string;

  constructor(opts?: { chunkSize?: number; chunkOverlap?: number; separator?: string }) {
    this.chunkSize = opts?.chunkSize ?? 1000;
    this.chunkOverlap = opts?.chunkOverlap ?? 200;
    this.separator = opts?.separator ?? '\n\n';
  }

  split(text: string): string[] {
    if (text.length <= this.chunkSize) return [text];

    const chunks: string[] = [];
    const sections = text.split(this.separator);
    let current = '';

    for (const section of sections) {
      if (current.length + section.length + this.separator.length > this.chunkSize && current) {
        chunks.push(current);
        // Keep overlap
        const overlap = current.slice(-this.chunkOverlap);
        current = overlap + this.separator + section;
      } else {
        current = current ? current + this.separator + section : section;
      }
    }

    if (current) chunks.push(current);

    // Split any remaining oversized chunks
    const finalChunks: string[] = [];
    for (const chunk of chunks) {
      if (chunk.length <= this.chunkSize * 1.5) {
        finalChunks.push(chunk);
      } else {
        // Split by character count
        for (let i = 0; i < chunk.length; i += this.chunkSize - this.chunkOverlap) {
          finalChunks.push(chunk.slice(i, i + this.chunkSize));
        }
      }
    }

    return finalChunks;
  }
}

// ===== Plugin System =====

export interface ToolPlugin {
  name: string;
  version: string;
  tools: Tool[];
  init?: (context: PluginContext) => Promise<void>;
}

export interface PluginContext {
  config: Record<string, unknown>;
  logger?: { info: (msg: string) => void; error: (msg: string) => void; warn: (msg: string) => void };
}

export class PluginLoader {
  private plugins: Map<string, ToolPlugin> = new Map();

  async load(plugin: ToolPlugin, context?: PluginContext): Promise<void> {
    if (plugin.init) {
      await plugin.init(context ?? { config: {} });
    }
    this.plugins.set(plugin.name, plugin);
  }

  getTools(): Tool[] {
    const tools: Tool[] = [];
    for (const plugin of this.plugins.values()) {
      tools.push(...plugin.tools);
    }
    return tools;
  }

  getPlugin(name: string): ToolPlugin | undefined {
    return this.plugins.get(name);
  }

  list(): string[] {
    return Array.from(this.plugins.keys());
  }

  unload(name: string): boolean {
    return this.plugins.delete(name);
  }
}

// ===== Default Toolkit =====

export interface ToolkitConfig {
  rootDir?: string;
  enableFS?: boolean;
  enableShell?: boolean;
  enableWeb?: boolean;
  enableAPI?: boolean;
  enableDatabase?: boolean;
  enableCodeExecution?: boolean;
  enableKnowledge?: boolean;
  shellConfig?: ShellConfig;
  fsConfig?: FileSystemConfig;
  webConfig?: WebConfig;
  knowledgeSearchFn?: (query: string, topK: number) => Promise<{ id: string; content: string; score: number; source?: string }[]>;
  dbConnection?: { query: (sql: string, params?: unknown[]) => Promise<unknown[]> | Promise<{ changes: number }> };
}

/** Create a toolkit with selected built-in tools. */
export function defaultToolkit(config: ToolkitConfig): ToolRegistry {
  const registry = new ToolRegistry();

  if (config.enableFS) {
    registry.register(new FileSystemTool({
      rootDir: config.rootDir ?? '.',
      ...config.fsConfig,
    }));
  }

  if (config.enableShell) {
    registry.register(new ShellTool(config.shellConfig));
  }

  if (config.enableWeb) {
    registry.register(new WebTool(config.webConfig));
  }

  if (config.enableAPI) {
    // API tool requires baseURL, skip if not provided
  }

  if (config.enableDatabase && config.dbConnection) {
    registry.register(new DatabaseTool(config.dbConnection));
  }

  if (config.enableCodeExecution) {
    registry.register(new CodeExecutionTool());
  }

  if (config.enableKnowledge && config.knowledgeSearchFn) {
    registry.register(new KnowledgeTool(config.knowledgeSearchFn));
  }

  return registry;
}

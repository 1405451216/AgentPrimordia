import * as fs from 'node:fs';
import * as path from 'node:path';
import type { Tool } from '../../types.js';

export interface FileSystemConfig {
  rootDir: string;
  allowedExtensions?: string[];
  maxFileSize?: number; // bytes
}

/**
 * FileSystem tool — file/directory read, write, search, list.
 */
export class FileSystemTool implements Tool {
  name = 'filesystem';
  description = 'Read, write, search, and list files and directories';
  parameters = {
    type: 'object',
    properties: {
      action: { type: 'string', enum: ['read', 'write', 'append', 'list', 'search', 'delete', 'mkdir', 'exists'], description: 'The action to perform' },
      path: { type: 'string', description: 'File or directory path' },
      content: { type: 'string', description: 'Content to write (for write/append)' },
      pattern: { type: 'string', description: 'Search pattern (for search)' },
    },
    required: ['action', 'path'],
  };

  private config: FileSystemConfig;

  constructor(config: FileSystemConfig) {
    this.config = {
      maxFileSize: 10 * 1024 * 1024, // 10MB default
      ...config,
    };
  }

  async execute(args: Record<string, unknown>): Promise<string> {
    const action = args.action as string;
    const filePath = args.path as string;
    const rootDir = path.resolve(this.config.rootDir);

    // Resolve and validate path (prevent traversal)
    const resolved = path.resolve(rootDir, filePath);
    if (!resolved.startsWith(rootDir)) {
      return `Error: path traversal detected: ${filePath}`;
    }

    switch (action) {
      case 'read': return this.readFile(resolved);
      case 'write': return this.writeFile(resolved, args.content as string);
      case 'append': return this.appendFile(resolved, args.content as string);
      case 'list': return this.listDir(resolved);
      case 'search': return this.searchFiles(resolved, args.pattern as string);
      case 'delete': return this.deleteFile(resolved);
      case 'mkdir': return this.makeDir(resolved);
      case 'exists': return this.exists(resolved);
      default: return `Error: unknown action: ${action}`;
    }
  }

  private async readFile(filePath: string): Promise<string> {
    try {
      const stat = fs.statSync(filePath);
      if (stat.size > (this.config.maxFileSize ?? 10 * 1024 * 1024)) {
        return `Error: file too large (${stat.size} bytes, max ${this.config.maxFileSize})`;
      }
      return fs.readFileSync(filePath, 'utf-8');
    } catch (err) {
      return `Error reading file: ${(err as Error).message}`;
    }
  }

  private async writeFile(filePath: string, content: string): Promise<string> {
    try {
      fs.mkdirSync(path.dirname(filePath), { recursive: true });
      fs.writeFileSync(filePath, content, 'utf-8');
      return `File written: ${filePath}`;
    } catch (err) {
      return `Error writing file: ${(err as Error).message}`;
    }
  }

  private async appendFile(filePath: string, content: string): Promise<string> {
    try {
      fs.appendFileSync(filePath, content, 'utf-8');
      return `Content appended to: ${filePath}`;
    } catch (err) {
      return `Error appending to file: ${(err as Error).message}`;
    }
  }

  private async listDir(dirPath: string): Promise<string> {
    try {
      const entries = fs.readdirSync(dirPath, { withFileTypes: true });
      const lines = entries.map((e) => {
        const type = e.isDirectory() ? '[DIR]' : '[FILE]';
        return `${type} ${e.name}`;
      });
      return lines.join('\n') || '(empty directory)';
    } catch (err) {
      return `Error listing directory: ${(err as Error).message}`;
    }
  }

  private async searchFiles(dirPath: string, pattern: string): Promise<string> {
    if (!pattern) return 'Error: search pattern is required';
    try {
      const results: string[] = [];
      const regex = new RegExp(pattern, 'i');
      const walk = (dir: string) => {
        const entries = fs.readdirSync(dir, { withFileTypes: true });
        for (const entry of entries) {
          const fullPath = path.join(dir, entry.name);
          if (entry.isDirectory()) {
            walk(fullPath);
          } else if (regex.test(entry.name) || regex.test(fullPath)) {
            results.push(fullPath);
          }
        }
      };
      walk(dirPath);
      return results.length > 0 ? results.join('\n') : 'No files found matching pattern';
    } catch (err) {
      return `Error searching files: ${(err as Error).message}`;
    }
  }

  private async deleteFile(filePath: string): Promise<string> {
    try {
      const stat = fs.statSync(filePath);
      if (stat.isDirectory()) {
        fs.rmSync(filePath, { recursive: true });
      } else {
        fs.unlinkSync(filePath);
      }
      return `Deleted: ${filePath}`;
    } catch (err) {
      return `Error deleting: ${(err as Error).message}`;
    }
  }

  private async makeDir(dirPath: string): Promise<string> {
    try {
      fs.mkdirSync(dirPath, { recursive: true });
      return `Directory created: ${dirPath}`;
    } catch (err) {
      return `Error creating directory: ${(err as Error).message}`;
    }
  }

  private async exists(filePath: string): Promise<string> {
    return fs.existsSync(filePath) ? 'true' : 'false';
  }
}

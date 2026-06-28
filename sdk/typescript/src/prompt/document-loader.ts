// document-loader.ts 实现文档加载器
// 支持多种文档格式的加载（文本、Markdown、JSON、代码等）
// 与 Go 端 memory 包中文档加载能力对齐

// ===== 文档类型 =====

/** 加载的文档结果 */
export interface ExtractedDocument {
  /** 文档 ID */
  id: string;
  /** 文档内容 */
  content: string;
  /** 来源路径 */
  source: string;
  /** 文档类型 */
  type: string;
  /** 元数据 */
  metadata: Record<string, string>;
}

// ===== 文档加载器接口 =====

/** 文档加载器接口 */
export interface FileDocLoader {
  /** 加载文档，返回文档列表 */
  load(source: string): Promise<ExtractedDocument[]>;
}

// ===== 文本文件加载器配置 =====

/** 文本文件加载器配置 */
export interface TextLoaderConfig {
  /** 文件编码（默认 utf-8） */
  encoding?: string;
  /** 是否自动分割（默认 false） */
  autoSplit?: boolean;
}

// ===== 文本文件加载器 =====

/** 文本文件加载器 */
export class TextLoader implements FileDocLoader {
  private encoding: string;

  constructor(config: TextLoaderConfig = {}) {
    this.encoding = config.encoding ?? 'utf-8';
  }

  async load(source: string): Promise<ExtractedDocument[]> {
    if (typeof process !== 'undefined' && process.env) {
      const fs = await import('fs/promises');
      const path = await import('path');
      const content = await fs.readFile(source, { encoding: this.encoding as BufferEncoding });
      const name = path.basename(source);
      return [
        {
          id: `doc_${source.replace(/[^a-zA-Z0-9]/g, '_')}`,
          content,
          source,
          type: 'text',
          metadata: { filename: name, encoding: this.encoding },
        },
      ];
    }
    throw new Error('TextLoader 仅支持 Node.js 环境');
  }
}

// ===== Markdown 加载器 =====

/** Markdown 文件加载器 */
export class MDLoader implements FileDocLoader {
  async load(source: string): Promise<ExtractedDocument[]> {
    if (typeof process !== 'undefined' && process.env) {
      const fs = await import('fs/promises');
      const path = await import('path');
      const content = await fs.readFile(source, { encoding: 'utf-8' as BufferEncoding });
      const name = path.basename(source);
      return [
        {
          id: `md_${source.replace(/[^a-zA-Z0-9]/g, '_')}`,
          content,
          source,
          type: 'markdown',
          metadata: { filename: name },
        },
      ];
    }
    throw new Error('MDLoader 仅支持 Node.js 环境');
  }
}

// ===== JSON 加载器 =====

/** JSON 文件加载器 */
export class JSONDocLoader implements FileDocLoader {
  async load(source: string): Promise<ExtractedDocument[]> {
    if (typeof process !== 'undefined' && process.env) {
      const fs = await import('fs/promises');
      const path = await import('path');
      const raw = await fs.readFile(source, { encoding: 'utf-8' as BufferEncoding });
      const content = JSON.stringify(JSON.parse(raw), null, 2);
      const name = path.basename(source);
      return [
        {
          id: `json_${source.replace(/[^a-zA-Z0-9]/g, '_')}`,
          content,
          source,
          type: 'json',
          metadata: { filename: name },
        },
      ];
    }
    throw new Error('JSONDocLoader 仅支持 Node.js 环境');
  }
}

// ===== 代码文件加载器 =====

/** 代码文件加载器，自动检测语言 */
export class CodeLoader implements FileDocLoader {
  private languageMap: Record<string, string> = {
    '.ts': 'typescript',
    '.tsx': 'typescript',
    '.js': 'javascript',
    '.jsx': 'javascript',
    '.go': 'go',
    '.py': 'python',
    '.rs': 'rust',
    '.java': 'java',
    '.cpp': 'cpp',
    '.c': 'c',
    '.h': 'c',
    '.hpp': 'cpp',
    '.cs': 'csharp',
    '.rb': 'ruby',
    '.php': 'php',
    '.swift': 'swift',
    '.kt': 'kotlin',
    '.scala': 'scala',
  };

  async load(source: string): Promise<ExtractedDocument[]> {
    if (typeof process !== 'undefined' && process.env) {
      const fs = await import('fs/promises');
      const path = await import('path');
      const content = await fs.readFile(source, { encoding: 'utf-8' as BufferEncoding });
      const name = path.basename(source);
      const ext = path.extname(source).toLowerCase();
      const language = this.languageMap[ext] ?? 'text';
      return [
        {
          id: `code_${source.replace(/[^a-zA-Z0-9]/g, '_')}`,
          content,
          source,
          type: 'code',
          metadata: { filename: name, language, extension: ext },
        },
      ];
    }
    throw new Error('CodeLoader 仅支持 Node.js 环境');
  }
}

// ===== 目录加载器 =====

/** 目录加载器，递归加载目录中的所有文件 */
export class DirectoryLoader implements FileDocLoader {
  private fileLoader: FileDocLoader;
  private extensions: string[];

  constructor(fileLoader: FileDocLoader, extensions: string[] = []) {
    this.fileLoader = fileLoader;
    this.extensions = extensions;
  }

  async load(source: string): Promise<ExtractedDocument[]> {
    if (typeof process !== 'undefined' && process.env) {
      const fs = await import('fs/promises');
      const path = await import('path');
      const results: ExtractedDocument[] = [];

      const walk = async (dir: string): Promise<void> => {
        const entries = await fs.readdir(dir, { withFileTypes: true });
        for (const entry of entries) {
          const fullPath = path.join(dir, entry.name);
          if (entry.isDirectory()) {
            await walk(fullPath);
          } else if (entry.isFile()) {
            const ext = path.extname(entry.name).toLowerCase();
            if (this.extensions.length === 0 || this.extensions.includes(ext)) {
              try {
                const docs = await this.fileLoader.load(fullPath);
                results.push(...docs);
              } catch {
                // 跳过无法加载的文件
              }
            }
          }
        }
      };

      await walk(source);
      return results;
    }
    throw new Error('DirectoryLoader 仅支持 Node.js 环境');
  }
}
import type { Tool } from '../types.js';

// ===== Document Loaders =====

export interface LoadedDocument {
  content: string;
  metadata: {
    source: string;
    format: string;
    size: number;
    pages?: number;
    createdAt?: string;
  };
}

// PDF Loader (simplified - parses basic structure)
export class PDFLoader {
  async load(buffer: Buffer | string, source: string = 'unknown'): Promise<LoadedDocument> {
    // In a real implementation, this would use a PDF parsing library
    // For now, we extract text between stream/endstream markers
    const content = typeof buffer === 'string' ? buffer : buffer.toString('utf-8');

    // Basic text extraction from PDF streams
    const textMatches: string[] = [];
    const streamRegex = /stream\r?\n([\s\S]*?)\r?\nendstream/g;
    let match;
    while ((match = streamRegex.exec(content)) !== null) {
      const text = match[1]!;
      // Filter for readable text (BT/ET text blocks)
      if (text.includes('BT') && text.includes('Tj')) {
        const tjMatches = text.matchAll(/\(([^)]*)\)\s*Tj/g);
        for (const tjMatch of tjMatches) {
          textMatches.push(tjMatch[1]!);
        }
      }
    }

    const extractedText = textMatches.join(' ') || content;

    return {
      content: extractedText,
      metadata: {
        source,
        format: 'pdf',
        size: typeof buffer === 'string' ? buffer.length : buffer.length,
      },
    };
  }
}

// DOCX Loader (simplified)
export class DOCXLoader {
  async load(buffer: Buffer | string, source: string = 'unknown'): Promise<LoadedDocument> {
    // In a real implementation, this would parse the DOCX ZIP structure
    // For now, we extract text from XML content
    const content = typeof buffer === 'string' ? buffer : buffer.toString('utf-8');

    // Extract text from <w:t> tags (Word document text)
    const textMatches: string[] = [];
    const textRegex = /<w:t[^>]*>([^<]*)<\/w:t>/g;
    let match;
    while ((match = textRegex.exec(content)) !== null) {
      textMatches.push(match[1]!);
    }

    return {
      content: textMatches.join(' ') || content,
      metadata: {
        source,
        format: 'docx',
        size: typeof buffer === 'string' ? buffer.length : buffer.length,
      },
    };
  }
}

// CSV Loader
export class CSVLoader {
  async load(content: string, source: string = 'unknown'): Promise<LoadedDocument> {
    const lines = content.split('\n').filter(l => l.trim());

    // Parse header
    const headers = this.parseCSVLine(lines[0] ?? '');
    const rows = lines.slice(1).map(line => {
      const values = this.parseCSVLine(line);
      const row: Record<string, string> = {};
      headers.forEach((h, i) => { row[h] = values[i] ?? ''; });
      return row;
    });

    // Convert to text representation
    const textContent = `Headers: ${headers.join(', ')}\n\n` +
      rows.map(r => Object.entries(r).map(([k, v]) => `${k}: ${v}`).join(', ')).join('\n');

    return {
      content: textContent,
      metadata: {
        source,
        format: 'csv',
        size: content.length,
      },
    };
  }

  private parseCSVLine(line: string): string[] {
    const result: string[] = [];
    let current = '';
    let inQuotes = false;

    for (let i = 0; i < line.length; i++) {
      const char = line[i]!;
      if (char === '"') {
        if (inQuotes && line[i + 1] === '"') {
          current += '"';
          i++;
        } else {
          inQuotes = !inQuotes;
        }
      } else if (char === ',' && !inQuotes) {
        result.push(current.trim());
        current = '';
      } else {
        current += char;
      }
    }
    result.push(current.trim());
    return result;
  }
}

// HTML Loader
export class HTMLLoader {
  async load(html: string, source: string = 'unknown'): Promise<LoadedDocument> {
    // Remove scripts and styles
    let clean = html.replace(/<script[\s\S]*?<\/script>/gi, '');
    clean = clean.replace(/<style[\s\S]*?<\/style>/gi, '');

    // Extract title
    const titleMatch = clean.match(/<title[^>]*>([^<]*)<\/title>/i);
    const title = titleMatch?.[1]?.trim() ?? '';

    // Remove tags and decode entities
    let text = clean.replace(/<[^>]+>/g, ' ');
    text = text
      .replace(/&nbsp;/g, ' ')
      .replace(/&amp;/g, '&')
      .replace(/&lt;/g, '<')
      .replace(/&gt;/g, '>')
      .replace(/&quot;/g, '"')
      .replace(/&#39;/g, "'")
      .replace(/\s+/g, ' ')
      .trim();

    if (title) text = `${title}\n\n${text}`;

    return {
      content: text,
      metadata: { source, format: 'html', size: html.length },
    };
  }
}

// Markdown Loader
export class MarkdownLoader {
  async load(md: string, source: string = 'unknown'): Promise<LoadedDocument> {
    // Remove markdown formatting for plain text
    let text = md;
    // Remove code blocks
    text = text.replace(/```[\s\S]*?```/g, '[code block]');
    // Remove inline code
    text = text.replace(/`([^`]+)`/g, '$1');
    // Remove images
    text = text.replace(/!\[([^\]]*)\]\([^)]*\)/g, '[image: $1]');
    // Remove links, keep text
    text = text.replace(/\[([^\]]*)\]\([^)]*\)/g, '$1');
    // Remove headers markers
    text = text.replace(/^#+\s+/gm, '');
    // Remove bold/italic
    text = text.replace(/\*{1,3}([^*]+)\*{1,3}/g, '$1');
    text = text.replace(/_{1,3}([^_]+)_{1,3}/g, '$1');
    // Remove blockquotes
    text = text.replace(/^>\s+/gm, '');
    // Remove list markers
    text = text.replace(/^[\s]*[-*+]\s+/gm, '');

    return {
      content: text.trim(),
      metadata: { source, format: 'markdown', size: md.length },
    };
  }
}

// JSON Loader
export class JSONLoader {
  async load(json: string, source: string = 'unknown'): Promise<LoadedDocument> {
    const data = JSON.parse(json);
    const text = this.flattenToString(data);

    return {
      content: text,
      metadata: { source, format: 'json', size: json.length },
    };
  }

  private flattenToString(data: unknown, prefix: string = ''): string {
    if (data === null) return `${prefix}: null`;
    if (typeof data === 'string' || typeof data === 'number' || typeof data === 'boolean') {
      return prefix ? `${prefix}: ${data}` : String(data);
    }
    if (Array.isArray(data)) {
      return data.map((item, i) => this.flattenToString(item, `${prefix}[${i}]`)).join('\n');
    }
    if (typeof data === 'object') {
      return Object.entries(data as Record<string, unknown>)
        .map(([key, val]) => this.flattenToString(val, prefix ? `${prefix}.${key}` : key))
        .join('\n');
    }
    return '';
  }
}

// Text Splitter
export interface TextSplitterConfig {
  chunkSize: number;
  chunkOverlap: number;
  separator?: string;
}

export class TextSplitter {
  private config: TextSplitterConfig;

  constructor(config?: Partial<TextSplitterConfig>) {
    this.config = {
      chunkSize: config?.chunkSize ?? 1000,
      chunkOverlap: config?.chunkOverlap ?? 200,
      separator: config?.separator ?? '\n\n',
    };
  }

  split(text: string): string[] {
    const chunks: string[] = [];
    const separator = this.config.separator!;

    // Split by separator first
    const sections = text.split(separator);
    let currentChunk = '';

    for (const section of sections) {
      if (currentChunk.length + section.length + separator.length > this.config.chunkSize) {
        if (currentChunk) {
          chunks.push(currentChunk);
          // Keep overlap
          const overlap = currentChunk.slice(-this.config.chunkOverlap);
          currentChunk = overlap + separator + section;
        } else {
          // Section itself is too large, split by sentences
          if (section.length > this.config.chunkSize) {
            const sentences = section.split(/(?<=[.!?])\s+/);
            for (const sentence of sentences) {
              if (currentChunk.length + sentence.length > this.config.chunkSize) {
                if (currentChunk) chunks.push(currentChunk);
                currentChunk = sentence;
              } else {
                currentChunk = currentChunk ? `${currentChunk} ${sentence}` : sentence;
              }
            }
          } else {
            currentChunk = section;
          }
        }
      } else {
        currentChunk = currentChunk ? `${currentChunk}${separator}${section}` : section;
      }
    }

    if (currentChunk) chunks.push(currentChunk);

    return chunks;
  }
}

// ===== Data Tools =====

export class DataTools {
  /** CSV analysis tool. */
  csvAnalysis: Tool = {
    name: 'csv_analyze',
    description: 'Analyze CSV data and return statistics',
    parameters: {
      type: 'object',
      properties: {
        data: { type: 'string', description: 'CSV data' },
        operation: { type: 'string', enum: ['summary', 'count', 'columns', 'head'] },
      },
      required: ['data', 'operation'],
    },
    execute: async (args: Record<string, unknown>): Promise<string> => {
      const data = args.data as string;
      const operation = args.operation as string;
      const loader = new CSVLoader();
      const doc = await loader.load(data);

      const lines = data.split('\n').filter(l => l.trim());
      const headers = lines[0]?.split(',').map(h => h.trim()) ?? [];

      switch (operation) {
        case 'summary':
          return `CSV Summary: ${lines.length - 1} rows, ${headers.length} columns: ${headers.join(', ')}`;
        case 'count':
          return `Row count: ${lines.length - 1}`;
        case 'columns':
          return `Columns: ${headers.join(', ')}`;
        case 'head':
          return lines.slice(0, 6).join('\n');
        default:
          return doc.content;
      }
    },
  };

  /** JSON query tool. */
  jsonQuery: Tool = {
    name: 'json_query',
    description: 'Query JSON data using dot notation path',
    parameters: {
      type: 'object',
      properties: {
        json: { type: 'string', description: 'JSON string' },
        path: { type: 'string', description: 'Dot notation path (e.g. "user.address.city")' },
      },
      required: ['json', 'path'],
    },
    execute: async (args: Record<string, unknown>): Promise<string> => {
      const data = JSON.parse(args.json as string);
      const path = (args.path as string).split('.');
      let current: unknown = data;

      for (const key of path) {
        if (current === null || current === undefined) return 'null';
        if (typeof current === 'object' && key in (current as Record<string, unknown>)) {
          current = (current as Record<string, unknown>)[key];
        } else if (Array.isArray(current)) {
          const idx = parseInt(key);
          current = current[idx];
        } else {
          return `Path "${args.path}" not found`;
        }
      }

      return typeof current === 'object' ? JSON.stringify(current, null, 2) : String(current);
    },
  };

  /** Text statistics tool. */
  textStats: Tool = {
    name: 'text_stats',
    description: 'Calculate text statistics (word count, char count, etc.)',
    parameters: {
      type: 'object',
      properties: {
        text: { type: 'string', description: 'Text to analyze' },
      },
      required: ['text'],
    },
    execute: async (args: Record<string, unknown>): Promise<string> => {
      const text = args.text as string;
      const words = text.split(/\s+/).filter(Boolean);
      const sentences = text.split(/[.!?]+/).filter(s => s.trim());
      const paragraphs = text.split(/\n\n+/).filter(p => p.trim());

      return JSON.stringify({
        characters: text.length,
        charactersNoSpaces: text.replace(/\s/g, '').length,
        words: words.length,
        sentences: sentences.length,
        paragraphs: paragraphs.length,
        avgWordsPerSentence: sentences.length > 0 ? words.length / sentences.length : 0,
        avgCharsPerWord: words.length > 0 ? text.replace(/\s/g, '').length / words.length : 0,
      }, null, 2);
    },
  };

  /** List all data tools. */
  list(): Tool[] {
    return [this.csvAnalysis, this.jsonQuery, this.textStats];
  }
}

// ===== Tool Cache =====

export interface ToolCacheEntry {
  key: string;
  result: string;
  timestamp: number;
  ttl: number;
}

export class ToolCache {
  private cache: Map<string, ToolCacheEntry> = new Map();
  private maxSize: number;
  private defaultTTL: number;

  constructor(maxSize: number = 1000, defaultTTL: number = 300000) {
    this.maxSize = maxSize;
    this.defaultTTL = defaultTTL;
  }

  get(key: string): string | undefined {
    const entry = this.cache.get(key);
    if (!entry) return undefined;

    if (Date.now() - entry.timestamp > entry.ttl) {
      this.cache.delete(key);
      return undefined;
    }

    return entry.result;
  }

  set(key: string, result: string, ttl?: number): void {
    // Evict oldest if at capacity
    if (this.cache.size >= this.maxSize) {
      const oldest = Array.from(this.cache.entries())
        .sort((a, b) => a[1].timestamp - b[1].timestamp)[0];
      if (oldest) this.cache.delete(oldest[0]);
    }

    this.cache.set(key, {
      key,
      result,
      timestamp: Date.now(),
      ttl: ttl ?? this.defaultTTL,
    });
  }

  delete(key: string): boolean {
    return this.cache.delete(key);
  }

  clear(): void {
    this.cache.clear();
  }

  size(): number {
    return this.cache.size;
  }

  /** Generate cache key from tool name and arguments. */
  static makeKey(toolName: string, args: Record<string, unknown>): string {
    return `${toolName}:${JSON.stringify(args)}`;
  }
}

// ===== Trie Rule (for guardrail pattern matching) =====

interface TrieNode {
  children: Map<string, TrieNode>;
  isEnd: boolean;
  data?: string;
}

export class TrieRule {
  private root: TrieNode = { children: new Map(), isEnd: false };

  insert(pattern: string, data?: string): void {
    let node = this.root;
    for (const char of pattern) {
      if (!node.children.has(char)) {
        node.children.set(char, { children: new Map(), isEnd: false });
      }
      node = node.children.get(char)!;
    }
    node.isEnd = true;
    node.data = data;
  }

  search(text: string): Array<{ pattern: string; position: number; data?: string }> {
    const results: Array<{ pattern: string; position: number; data?: string }> = [];

    for (let i = 0; i < text.length; i++) {
      let node = this.root;
      let j = i;
      let lastMatch: { pattern: string; position: number; data?: string } | null = null;

      while (j < text.length && node.children.has(text[j]!)) {
        node = node.children.get(text[j]!)!;
        j++;
        if (node.isEnd) {
          lastMatch = {
            pattern: text.slice(i, j),
            position: i,
            data: node.data,
          };
        }
      }

      if (lastMatch) results.push(lastMatch);
    }

    return results;
  }

  contains(text: string): boolean {
    return this.search(text).length > 0;
  }

  remove(pattern: string): boolean {
    return this.removeHelper(this.root, pattern, 0);
  }

  private removeHelper(node: TrieNode, pattern: string, depth: number): boolean {
    if (depth === pattern.length) {
      if (!node.isEnd) return false;
      node.isEnd = false;
      node.data = undefined;
      return node.children.size === 0;
    }

    const char = pattern[depth]!;
    const child = node.children.get(char);
    if (!child) return false;

    const shouldDelete = this.removeHelper(child, pattern, depth + 1);

    if (shouldDelete) {
      node.children.delete(char);
      return node.children.size === 0 && !node.isEnd;
    }

    return false;
  }

  clear(): void {
    this.root = { children: new Map(), isEnd: false };
  }
}

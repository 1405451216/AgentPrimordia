/**
 * 增强版 RAG 处理管道，与 Go 端 rag_pipeline.go 对齐。
 *
 * 流程: 文档加载 → 策略切分 → 可选语义优化 → 向量化 → 存储
 */

import { RAGStore } from './rag.js';
import type { RAGDocument } from './rag.js';

// ===== 切分策略 =====

/** 切分策略名称，与 Go 端 SplitterStrategy 对齐 */
export type SplitterStrategy =
  | 'character'
  | 'recursive'
  | 'line'
  | 'sentence'
  | 'markdown'
  | 'token'
  | 'code'
  | 'semantic';

/** 切分器配置 */
export interface SplitterConfig {
  /** 目标块大小（字符数） */
  chunkSize?: number;
  /** 块间重叠量 */
  chunkOverlap?: number;
  /** 分隔符列表（递归切分用） */
  separators?: string[];
  /** 额外元数据 */
  metadata?: Record<string, unknown>;
}

/** 切分器接口 */
export interface RAGTextSplitter {
  split(text: string): string[];
}

// ===== 文档加载器 =====

/** 文档加载结果 */
export interface LoadedDocument {
  id: string;
  content: string;
  source: string;
  metadata?: Record<string, string>;
}

/** 文档加载器接口，与 Go 端 DocumentLoader 对齐 */
export interface DocumentLoader {
  /** 加载文档 */
  load(source: string): Promise<LoadedDocument[]>;
}

/** 简单文本加载器 */
export class SimpleTextLoader implements DocumentLoader {
  async load(source: string): Promise<LoadedDocument[]> {
    return [{
      id: `doc_${Date.now()}`,
      content: source,
      source: 'inline',
    }];
  }
}

// ===== 切分器实现 =====

/** 字符切分器 */
class CharacterSplitter implements RAGTextSplitter {
  private chunkSize: number;
  private chunkOverlap: number;

  constructor(chunkSize: number = 1000, chunkOverlap: number = 200) {
    this.chunkSize = chunkSize;
    this.chunkOverlap = chunkOverlap;
  }

  split(text: string): string[] {
    if (text.length <= this.chunkSize) return [text];
    const chunks: string[] = [];
    let start = 0;
    while (start < text.length) {
      const end = start + this.chunkSize;
      chunks.push(text.slice(start, end));
      start = end - this.chunkOverlap;
    }
    return chunks;
  }
}

/** 递归字符切分器 */
class RecursiveSplitter implements RAGTextSplitter {
  private chunkSize: number;
  private chunkOverlap: number;
  private separators: string[];

  constructor(chunkSize: number = 1000, chunkOverlap: number = 200) {
    this.chunkSize = chunkSize;
    this.chunkOverlap = chunkOverlap;
    this.separators = ['\n\n', '\n', '。', '.', ' ', ''];
  }

  split(text: string): string[] {
    if (text.length <= this.chunkSize) return [text];
    return this.splitRecursive(text);
  }

  private splitRecursive(text: string, sepIdx: number = 0): string[] {
    if (sepIdx >= this.separators.length) {
      return new CharacterSplitter(this.chunkSize, this.chunkOverlap).split(text);
    }

    const sep = this.separators[sepIdx];
    const parts = text.split(sep).filter((p) => p.trim().length > 0);

    const chunks: string[] = [];
    let current = '';

    for (const part of parts) {
      const candidate = current ? current + sep + part : part;
      if (candidate.length > this.chunkSize && current.length > 0) {
        chunks.push(current);
        current = part;
      } else {
        current = candidate;
      }
    }
    if (current) chunks.push(current);

    return chunks;
  }
}

// ===== 切分器注册表 =====

/** 切分器工厂函数 */
type SplitterFactory = (cfg: SplitterConfig) => RAGTextSplitter;

/** 切分策略注册表，与 Go 端 splitterRegistry 对齐 */
const splitterRegistry = new Map<SplitterStrategy, SplitterFactory>();

/** 注册切分策略 */
export function registerSplitter(name: SplitterStrategy, factory: SplitterFactory): void {
  splitterRegistry.set(name, factory);
}

/** 创建切分器 */
export function createSplitter(strategy: SplitterStrategy, cfg: SplitterConfig = {}): RAGTextSplitter {
  const factory = splitterRegistry.get(strategy);
  if (!factory) {
    throw new Error(`未知切分策略: ${strategy}`);
  }
  return factory(cfg);
}

/** 获取所有可用策略 */
export function availableStrategies(): SplitterStrategy[] {
  return [...splitterRegistry.keys()];
}

// 注册默认切分器
registerSplitter('character', (cfg) => new CharacterSplitter(cfg.chunkSize, cfg.chunkOverlap));
registerSplitter('recursive', (cfg) => new RecursiveSplitter(cfg.chunkSize, cfg.chunkOverlap));
registerSplitter('line', (cfg) => ({
  split: (text: string) => {
    const linesPerChunk = cfg.chunkSize ?? 100;
    const lines = text.split('\n');
    const chunks: string[] = [];
    for (let i = 0; i < lines.length; i += linesPerChunk) {
      chunks.push(lines.slice(i, i + linesPerChunk).join('\n'));
    }
    return chunks;
  },
}));
registerSplitter('sentence', (cfg) => ({
  split: (text: string) => {
    const sentences = text.split(/(?<=[。！？.!?\n])/g).filter((s) => s.trim().length > 0);
    const chunkSize = cfg.chunkSize ?? 1000;
    const chunks: string[] = [];
    let current = '';
    for (const s of sentences) {
      if (current.length + s.length > chunkSize && current.length > 0) {
        chunks.push(current);
        current = s;
      } else {
        current += s;
      }
    }
    if (current) chunks.push(current);
    return chunks.length > 0 ? chunks : [text];
  },
}));

// ===== 增强版 RAG 管道 =====

/** 文档摄入结果，与 Go 端 IngestResult 对齐 */
export interface IngestResult {
  source: string;
  ingested: number;
  failed: number;
  totalChunks: number;
  durationMs: number;
}

/** 增强版 RAG 管道配置 */
export interface EnhancedRAGPipelineConfig {
  /** 文档加载器 */
  loader?: DocumentLoader;
  /** 切分策略 */
  splitStrategy?: SplitterStrategy;
  /** 切分配置 */
  splitConfig?: SplitterConfig;
  /** RAG 存储 */
  ragStore: RAGStore;
}

/** 增强版 RAG 处理管道，与 Go 端 EnhancedRAGPipeline 对齐。
 *
 * 流程: 文档加载 → 策略切分 → 向量化 → 存储
 *
 * 使用方式：
 *   const pipeline = new EnhancedRAGPipeline({
 *     ragStore: new RAGStore(384),
 *     splitStrategy: 'recursive',
 *   });
 *   const result = await pipeline.ingest('这是一段很长的文档内容...');
 */
export class EnhancedRAGPipeline {
  private loader: DocumentLoader;
  private splitter: RAGTextSplitter;
  private store: RAGStore;

  constructor(config: EnhancedRAGPipelineConfig) {
    this.loader = config.loader ?? new SimpleTextLoader();
    this.splitter = createSplitter(config.splitStrategy ?? 'recursive', config.splitConfig);
    this.store = config.ragStore;
  }

  /** 加载文档、切分并存入 RAG 存储 */
  async ingest(source: string): Promise<IngestResult> {
    const startTime = Date.now();

    const docs = await this.loader.load(source);

    const result: IngestResult = {
      source,
      ingested: 0,
      failed: 0,
      totalChunks: 0,
      durationMs: 0,
    };

    for (const doc of docs) {
      const chunks = this.splitter.split(doc.content);
      result.totalChunks += chunks.length;

      for (let i = 0; i < chunks.length; i++) {
        const chunkDoc: RAGDocument = {
          id: `${doc.id}_chunk_${i}`,
          content: chunks[i],
          source: doc.source,
          metadata: {
            ...doc.metadata,
            ...(chunks.length > 1 ? {
              chunk_index: String(i),
              total_chunks: String(chunks.length),
            } : {}),
          },
        };

        try {
          await this.store.addDocument(chunkDoc);
          result.ingested++;
        } catch {
          result.failed++;
        }
      }
    }

    result.durationMs = Date.now() - startTime;
    return result;
  }
}
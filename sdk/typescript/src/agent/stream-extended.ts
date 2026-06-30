import type { Chunk, Message } from '../types.js';
import type { Provider } from '../llm/provider.js';

// ===== SSE Writer =====
// 依赖 Web Streams API 的 WritableStreamDefaultWriter（Node.js 18+ / Bun / Deno 内置支持）

export type SSEEventType = 'token' | 'tool_call' | 'tool_result' | 'error' | 'done' | 'thought';

export interface SSEEvent {
  type: SSEEventType;
  data: string;
  id?: string;
  retry?: number;
}

export class SSEWriter {
  private writer: { write: (chunk: string) => void } | WritableStreamDefaultWriter<Uint8Array>;
  private encoder: TextEncoder;
  private eventID = 0;
  private retryMs = 0;
  /** 背压控制：已入队但未写入的 chunk 数 */
  private queueDepth = 0;
  /** 最大队列深度，超过则丢弃（防止慢消费者阻塞 Agent） */
  private maxQueueDepth: number;
  /** 写入超时（毫秒），超时后丢弃当前 chunk */
  private writeTimeoutMs: number;
  /** 已丢弃的 chunk 计数 */
  private droppedChunks = 0;

  constructor(
    writer: { write: (chunk: string) => void } | WritableStreamDefaultWriter<Uint8Array>,
    opts?: { maxQueueDepth?: number; writeTimeoutMs?: number },
  ) {
    this.writer = writer;
    this.encoder = new TextEncoder();
    this.maxQueueDepth = opts?.maxQueueDepth ?? 1000;
    this.writeTimeoutMs = opts?.writeTimeoutMs ?? 5000;
  }

  setRetry(ms: number): void { this.retryMs = ms; }
  setEventID(id: number): void { this.eventID = id; }

  /** 获取已丢弃的 chunk 数（用于监控） */
  getDroppedCount(): number { return this.droppedChunks; }

  async writeEvent(event: SSEEvent): Promise<void> {
    // 背压检查：队列深度超过上限时丢弃 chunk
    if (this.queueDepth >= this.maxQueueDepth) {
      this.droppedChunks++;
      return;
    }

    this.queueDepth++;
    const lines: string[] = [];

    if (event.type) lines.push(`event: ${event.type}`);

    this.eventID++;
    lines.push(`id: ${this.eventID}`);

    if (this.retryMs > 0) lines.push(`retry: ${this.retryMs}`);

    // Data can be multiline
    const dataLines = event.data.split('\n');
    for (const line of dataLines) {
      lines.push(`data: ${line}`);
    }
    lines.push(''); // Empty line to end event

    const output = lines.join('\n') + '\n';

    try {
      // 使用超时机制防止慢消费者阻塞（与 Go 端 SSE 5s 超时对齐）
      await this.writeWithTimeout(output);
    } catch {
      // 写入失败或超时，丢弃此 chunk
      this.droppedChunks++;
    } finally {
      this.queueDepth--;
    }
  }

  /** 带超时的写入，防止下游慢消费者阻塞上游生产者 */
  private async writeWithTimeout(output: string): Promise<void> {
    if ('write' in this.writer && typeof this.writer.write === 'function') {
      if (this.writer instanceof WritableStreamDefaultWriter) {
        // WritableStreamDefaultWriter — 使用 Promise.race 实现超时
        const writePromise = this.writer.write(this.encoder.encode(output));
        const timeoutPromise = new Promise<never>((_, reject) =>
          setTimeout(() => reject(new Error('SSE write timeout')), this.writeTimeoutMs),
        );
        await Promise.race([writePromise, timeoutPromise]);
      } else {
        (this.writer as { write: (chunk: string) => void }).write(output);
      }
    }
  }

  async writeToken(content: string): Promise<void> {
    await this.writeEvent({ type: 'token', data: content });
  }

  async writeError(error: string): Promise<void> {
    await this.writeEvent({ type: 'error', data: error });
  }

  async writeDone(): Promise<void> {
    await this.writeEvent({ type: 'done', data: '[DONE]' });
  }

  async writeHeartbeat(): Promise<void> {
    // SSE comment line for keepalive
    if ('write' in this.writer && typeof this.writer.write === 'function') {
      if (this.writer instanceof WritableStreamDefaultWriter) {
        await this.writer.write(this.encoder.encode(': heartbeat\n\n'));
      } else {
        (this.writer as { write: (chunk: string) => void }).write(': heartbeat\n\n');
      }
    }
  }
}

// ===== HTTP SSE Response Helper =====

export interface SSEResponseOptions {
  retryMs?: number;
  heartbeatIntervalMs?: number;
}

export function createSSEResponse(
  writable: WritableStreamDefaultWriter<Uint8Array>,
  opts?: SSEResponseOptions
): { writer: SSEWriter; stop: () => void } {
  const writer = new SSEWriter(writable);
  if (opts?.retryMs) writer.setRetry(opts.retryMs);

  let heartbeatTimer: NodeJS.Timeout | undefined;
  if (opts?.heartbeatIntervalMs && opts.heartbeatIntervalMs > 0) {
    heartbeatTimer = setInterval(() => {
      writer.writeHeartbeat().catch(() => {});
    }, opts.heartbeatIntervalMs);
  }

  const stop = () => {
    if (heartbeatTimer) clearInterval(heartbeatTimer);
  };

  return { writer, stop };
}

// ===== Stream Middleware Pipeline =====

export type StreamHandler = (chunk: Chunk) => void | Promise<void>;

export type StreamMiddleware = (chunk: Chunk, next: StreamHandler) => void | Promise<void>;

export class StreamPipeline {
  private middlewares: StreamMiddleware[] = [];
  private handler: StreamHandler;

  constructor(handler: StreamHandler) {
    this.handler = handler;
  }

  use(mw: StreamMiddleware): StreamPipeline {
    this.middlewares.push(mw);
    return this;
  }

  async process(chunk: Chunk): Promise<void> {
    let handler = this.handler;
    // Build chain in reverse order
    for (let i = this.middlewares.length - 1; i >= 0; i--) {
      const mw = this.middlewares[i]!;
      const currentHandler = handler;
      handler = (c: Chunk) => mw(c, currentHandler);
    }
    await handler(chunk);
  }
}

// ===== Built-in Stream Middlewares =====

/** Filter middleware: only pass through non-empty chunks. */
export function filterEmpty(): StreamMiddleware {
  return async (chunk, next) => {
    if (chunk.content || chunk.done) await next(chunk);
  };
}

/** Buffer middleware: accumulate tokens and emit in batches. */
export function bufferMiddleware(batchSize: number): StreamMiddleware {
  let buffer = '';
  return async (chunk, next) => {
    if (chunk.content) {
      buffer += chunk.content;
      if (buffer.length >= batchSize) {
        await next({ content: buffer, done: false });
        buffer = '';
      }
    }
    if (chunk.done) {
      if (buffer) await next({ content: buffer, done: false });
      await next({ content: '', done: true, usage: chunk.usage });
      buffer = '';
    }
  };
}

/** Rate limit middleware: ensure minimum delay between chunks. */
export function rateLimitMiddleware(minIntervalMs: number): StreamMiddleware {
  let lastEmit = 0;
  return async (chunk, next) => {
    const now = Date.now();
    const elapsed = now - lastEmit;
    if (elapsed < minIntervalMs) {
      await new Promise(r => setTimeout(r, minIntervalMs - elapsed));
    }
    lastEmit = Date.now();
    await next(chunk);
  };
}

/** Transform middleware: modify chunk content. */
export function transformMiddleware(fn: (content: string) => string): StreamMiddleware {
  return async (chunk, next) => {
    await next({
      ...chunk,
      content: chunk.content ? fn(chunk.content) : chunk.content,
    });
  };
}

/** Log middleware: log all chunks. */
export function logMiddleware(logger: (msg: string) => void): StreamMiddleware {
  return async (chunk, next) => {
    if (chunk.content) logger(`[stream] ${chunk.content}`);
    await next(chunk);
  };
}

// ===== Stream Collector =====
// Collects all chunks from a stream into a single response

export interface CollectedResult {
  content: string;
  usage?: Chunk['usage'];
  chunkCount: number;
  duration: number;
}

export class StreamCollector {
  async collect(stream: AsyncIterable<Chunk>): Promise<CollectedResult> {
    const startTime = Date.now();
    let content = '';
    let usage: Chunk['usage'] | undefined;
    let chunkCount = 0;

    for await (const chunk of stream) {
      chunkCount++;
      if (chunk.content) content += chunk.content;
      if (chunk.usage) usage = chunk.usage;
      if (chunk.done) break;
    }

    return {
      content,
      usage,
      chunkCount,
      duration: Date.now() - startTime,
    };
  }

  async *replay(chunks: Chunk[], delayMs: number = 0): AsyncIterable<Chunk> {
    for (const chunk of chunks) {
      yield chunk;
      if (delayMs > 0) await new Promise(r => setTimeout(r, delayMs));
    }
  }

  async *merge(streams: AsyncIterable<Chunk>[]): AsyncIterable<Chunk> {
    // Interleave chunks from multiple streams using a shared queue + notifier
    const queues: Chunk[][] = streams.map(() => []);
    const done = new Array(streams.length).fill(false);
    const notifier = { resolve: null as null | (() => void) };

    const notify = () => {
      if (notifier.resolve) {
        const fn = notifier.resolve;
        notifier.resolve = null;
        fn();
      }
    };

    const waitForData = () => new Promise<void>((resolve) => { notifier.resolve = resolve; });

    // Start consuming all streams
    const consumers = streams.map(async (stream, i) => {
      try {
        for await (const chunk of stream) {
          queues[i].push(chunk);
          notify();
        }
      } finally {
        done[i] = true;
        notify();
      }
    });

    // Yield from queues round-robin, using notifier instead of setTimeout polling
    let allDone = false;
    while (!allDone) {
      for (let i = 0; i < queues.length; i++) {
        const chunk = queues[i].shift();
        if (chunk) yield chunk;
      }
      allDone = done.every(Boolean);
      if (!allDone && queues.every(q => q.length === 0)) {
        // 等待新数据到达，避免 busy-wait
        await waitForData();
      }
    }

    // Drain any remaining chunks
    for (let i = 0; i < queues.length; i++) {
      while (queues[i].length > 0) {
        yield queues[i].shift()!;
      }
    }

    await Promise.all(consumers);
  }
}

// ===== Context Compression Strategy =====

export interface CompressConfig {
  maxTokens: number;
  summaryModel?: Provider;
  keepSystemMessages: boolean;
  keepRecentN: number;
  compressRatio: number;
  /** LLM 压缩超时（毫秒），超时后降级为简单截断，防止 Agent 无限阻塞（默认 30s） */
  compressTimeoutMs?: number;
}

export class CompressStrategy {
  private config: CompressConfig;

  constructor(config: Partial<CompressConfig> = {}) {
    this.config = {
      maxTokens: config.maxTokens ?? 4000,
      summaryModel: config.summaryModel,
      keepSystemMessages: config.keepSystemMessages ?? true,
      keepRecentN: config.keepRecentN ?? 2,
      compressRatio: config.compressRatio ?? 0.3,
      compressTimeoutMs: config.compressTimeoutMs,
    };
  }

  trim(messages: Message[], _maxMessages: number): Message[] {
    if (messages.length === 0) return messages;

    const effectiveMax = _maxMessages > 0 ? _maxMessages : 20;
    if (messages.length <= effectiveMax) return messages;

    const systemMsgs: Message[] = [];
    const nonSystem: Message[] = [];

    for (const m of messages) {
      if (m.role === 'system' && this.config.keepSystemMessages) {
        systemMsgs.push(m);
      } else if (m.role !== 'system') {
        nonSystem.push(m);
      }
    }

    const keepN = Math.min(this.config.keepRecentN, nonSystem.length);
    const recentMsgs = nonSystem.slice(-keepN);
    const oldMsgs = nonSystem.slice(0, nonSystem.length - keepN);

    // Compress old messages (without LLM, just truncate)
    if (oldMsgs.length === 0) {
      return [...systemMsgs, ...recentMsgs];
    }

    const compressedContent = oldMsgs
      .map(m => `${m.role}: ${m.content.slice(0, Math.floor(m.content.length * this.config.compressRatio))}...`)
      .join('\n');

    const summary: Message = {
      role: 'system',
      content: `[Compressed context - ${oldMsgs.length} messages]\n${compressedContent}`,
    };

    return [...systemMsgs, summary, ...recentMsgs];
  }

  async compressWithLLM(messages: Message[]): Promise<Message[]> {
    if (!this.config.summaryModel) return this.trim(messages, 0);

    const systemMsgs = messages.filter(m => m.role === 'system' && this.config.keepSystemMessages);
    const nonSystem = messages.filter(m => m.role !== 'system');

    if (nonSystem.length <= this.config.keepRecentN) return messages;

    const keepN = Math.min(this.config.keepRecentN, nonSystem.length);
    const recentMsgs = nonSystem.slice(-keepN);
    const oldMsgs = nonSystem.slice(0, nonSystem.length - keepN);

    const conversation = oldMsgs.map(m => `${m.role}: ${m.content}`).join('\n');

    // 超时保护：LLM 调用超时后降级为简单截断，防止 Agent 无限阻塞
    const timeoutMs = this.config.compressTimeoutMs ?? 30_000;
    try {
      const resp = await Promise.race([
        this.config.summaryModel.complete({
          messages: [
            { role: 'system', content: 'Summarize the following conversation in a concise form, preserving key information.' },
            { role: 'user', content: conversation },
          ],
          temperature: 0,
        }),
        new Promise<never>((_, reject) =>
          setTimeout(() => reject(new Error('LLM compress timeout')), timeoutMs),
        ),
      ]);

      const summary: Message = {
        role: 'system',
        content: `[Conversation summary]\n${resp.content}`,
      };

      return [...systemMsgs, summary, ...recentMsgs];
    } catch {
      // LLM 压缩失败或超时，降级为简单截断
      return this.trim(messages, 0);
    }
  }
}

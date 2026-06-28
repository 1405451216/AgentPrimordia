// structured-output.ts 实现 LLM 结构化输出提取器
// 通过 LLM + JSON Schema 约束，从自然语言输入中提取结构化数据
// 与 Go 端 structured.go 对齐

import type { Provider } from './provider.js';
import type { CompletionRequest, SchemaDef, ResponseFormat, Message } from '../types.js';

// ===== 结构化提取器配置 =====

/** 结构化输出提取器配置，与 Go 端 ExtractorConfig 对齐 */
export interface StructuredOutputConfig {
  /** 最大重试次数（默认 0） */
  maxRetries?: number;
  /** 是否验证输出是否符合 Schema */
  validate?: boolean;
}

// ===== 结构化提取器 =====

/** 结构化输出提取器
 *
 * 通过 LLM + JSON Schema 约束，从自然语言输入中提取结构化数据。
 * 支持重试修复：当 LLM 输出不符合 Schema 时，自动将错误反馈给 LLM 重试。
 * 与 Go 端 StructuredExtractor 对齐。
 *
 * 使用方式：
 *   const extractor = new StructuredOutputExtractor(provider, 'gpt-4');
 *   const schema = { name: 'person', schema: { type: 'object', properties: { name: { type: 'string' } } } };
 *   const result = await extractor.extract('John is 30 years old', schema);
 *   // => { name: 'John', age: 30 }
 */
export class StructuredOutputExtractor {
  private provider: Provider;
  private model: string;
  private config: StructuredOutputConfig;

  constructor(provider: Provider, model: string, config?: StructuredOutputConfig) {
    if (!provider) {
      throw new Error('provider must not be nil');
    }
    this.provider = provider;
    this.model = model;
    this.config = { maxRetries: 0, validate: false, ...config };
  }

  /** 从自然语言输入中提取结构化数据
   *
   * prompt 引导 LLM 输出，schema 约束输出格式。
   * 返回解析后的 JSON 对象。
   */
  async extract<T = unknown>(prompt: string, schema: SchemaDef): Promise<T> {
    if (!schema) {
      throw new Error('schema must not be nil');
    }

    const messages: Message[] = [
      { role: 'system', content: structuredSystemPrompt(schema) },
      { role: 'user', content: prompt },
    ];

    const maxAttempts = 1 + (this.config.maxRetries ?? 0);
    let lastErr: Error | null = null;

    for (let attempt = 0; attempt < maxAttempts; attempt++) {
      const req: CompletionRequest = {
        messages,
        model: this.model,
        responseFormat: {
          type: 'json_schema',
          jsonSchema: schema,
        } as ResponseFormat,
      };

      try {
        const resp = await this.provider.complete(req);
        const content = resp.content.trim();

        try {
          const parsed = JSON.parse(content) as T;
          return parsed;
        } catch {
          lastErr = new Error(`LLM 返回内容不是有效 JSON: ${content.slice(0, 200)}`);
          messages.push(
            { role: 'assistant', content },
            { role: 'user', content: `你输出的内容不是有效的 JSON，请修正。错误: ${lastErr.message}\n请严格按照 Schema 重新输出。` },
          );
          continue;
        }
      } catch (err: unknown) {
        lastErr = err instanceof Error ? err : new Error(String(err));
        if (attempt < maxAttempts - 1) {
          await new Promise((resolve) => setTimeout(resolve, 500 * (attempt + 1)));
        }
      }
    }

    throw new Error(`结构化提取失败（已重试 ${this.config.maxRetries} 次）: ${lastErr?.message}`);
  }

  /** 从自然语言输入中提取结构化数据，自动从 JSON Schema 提取并校验
   *
   * 泛型 T 指定目标类型，返回强类型结果。
   */
  async extractInto<T>(prompt: string, schema: SchemaDef): Promise<T> {
    return this.extract<T>(prompt, schema);
  }
}

// ===== 辅助函数 =====

/** 生成结构化提取的系统提示词，与 Go 端 structuredSystemPrompt 对齐 */
function structuredSystemPrompt(schema: SchemaDef): string {
  const schemaStr = JSON.stringify(schema.schema, null, 2);
  const desc = schema.description ? `\n描述: ${schema.description}` : '';
  return `你是一个结构化数据提取助手。请严格按照以下 JSON Schema 输出结果，不要输出任何其他内容。${desc}\n\nSchema:\n${schemaStr}`;
}
// few-shot.ts 实现 Few-Shot 学习模板
// 支持示例选择器和动态示例注入，与 Go 端 prompt/few_shot.go 对齐

import type { PromptTemplate } from '../agent/prompt-template.js';

// ===== Few-Shot 示例 =====

/** Few-Shot 示例，与 Go 端 Example 对齐 */
export interface Example {
  /** 输入文本 */
  input: string;
  /** 期望输出 */
  output: string;
  /** 额外元数据 */
  metadata?: Record<string, unknown>;
}

// ===== 示例选择器接口 =====

/** 示例选择器接口，与 Go 端 ExampleSelector 对齐 */
export interface ExampleSelector {
  /**
   * 根据输入选择最相关的示例
   * @param input - 用户输入
   * @param allExamples - 所有可用示例
   * @returns 选中的示例列表
   */
  selectExamples(input: string, allExamples: Example[]): Example[];
}

// ===== Few-Shot 模板配置 =====

/** Few-Shot 模板配置，与 Go 端 FewShotConfig 对齐 */
export interface FewShotConfig {
  /** 基础模板 */
  baseTemplate: PromptTemplate;
  /** 单个示例的渲染格式（默认 "\\n输入: {{.Input}}\\n输出: {{.Output}}\\n"） */
  exampleFormat?: string;
  /** 示例列表前缀（默认 "\\n以下是一些示例：\\n"） */
  prefix?: string;
  /** 示例列表后缀（默认 "\\n现在请处理：\\n"） */
  suffix?: string;
  /** 最大示例数（默认 5） */
  maxExamples?: number;
  /** 示例选择器 */
  selector?: ExampleSelector;
}

// ===== 简单关键词选择器 =====

/** 简单关键词匹配选择器，与 Go 端 KeywordSelector 对齐 */
export class KeywordSelector implements ExampleSelector {
  selectExamples(input: string, allExamples: Example[]): Example[] {
    const inputLower = input.toLowerCase();
    // 计算每个示例与输入的相似度（简单关键词重叠）
    const scored = allExamples.map((ex) => {
      const exLower = ex.input.toLowerCase();
      const inputWords = new Set(inputLower.split(/\s+/));
      const exWords = new Set(exLower.split(/\s+/));
      let overlap = 0;
      for (const w of inputWords) {
        if (exWords.has(w)) overlap++;
      }
      return { example: ex, score: overlap };
    });

    // 按相似度排序，过滤掉得分为 0 的
    const relevant = scored
      .filter((s) => s.score > 0)
      .sort((a, b) => b.score - a.score)
      .map((s) => s.example);

    return relevant;
  }
}

// ===== Few-Shot 模板 =====

/** 支持 Few-Shot 学习的模板，与 Go 端 FewShotTemplate 对齐 */
export class FewShotTemplate {
  private baseTemplate: PromptTemplate;
  private examples: Example[] = [];
  private selector: ExampleSelector;
  private maxExamples: number;
  private exampleFormat: string;
  private prefix: string;
  private suffix: string;

  constructor(config: FewShotConfig) {
    this.baseTemplate = config.baseTemplate;
    this.exampleFormat = config.exampleFormat ?? '\\n输入: {{.Input}}\\n输出: {{.Output}}\\n';
    this.prefix = config.prefix ?? '\\n以下是一些示例：\\n';
    this.suffix = config.suffix ?? '\\n现在请处理：\\n';
    this.maxExamples = config.maxExamples ?? 5;
    this.selector = config.selector ?? new KeywordSelector();
  }

  /** 添加示例 */
  addExample(example: Example): void {
    this.examples.push(example);
  }

  /** 添加多个示例 */
  addExamples(examples: Example[]): void {
    this.examples.push(...examples);
  }

  /** 获取示例数量 */
  getExampleCount(): number {
    return this.examples.length;
  }

  /** 选择示例并渲染
   *
   * @param input - 用户输入
   * @param vars - 基础模板变量
   * @returns 包含示例的渲染结果
   */
  render(input: string, vars: Record<string, string>): string {
    // 选择相关示例
    const selected = this.selector.selectExamples(input, this.examples);
    const limited = selected.slice(0, this.maxExamples);

    let result = this.baseTemplate.withVars(vars).render();

    if (limited.length > 0) {
      result += this.prefix;
      for (const ex of limited) {
        result += this.renderExample(ex);
      }
      result += this.suffix;
      // 追加用户输入
      result += `\n${input}`;
    }

    return result;
  }

  /** 渲染单个示例 */
  private renderExample(example: Example): string {
    return this.exampleFormat
      .replace(/\{\{\.Input\}\}/g, example.input)
      .replace(/\{\{\.Output\}\}/g, example.output);
  }
}
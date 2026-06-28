// parser.ts 实现 Prompt 输出解析器
// 解析 LLM 输出文本，支持 JSON、XML、Regex、Markdown 等格式
// 与 Go 端 prompt/parser.go 对齐

// ===== 输出解析器接口 =====

/** 输出解析器接口，与 Go 端 OutputParser 对齐 */
export interface OutputParser {
  /** 解析 LLM 输出文本，返回结构化数据 */
  parse(text: string): unknown;
  /** 返回给 LLM 的格式说明（注入到 Prompt 中） */
  formatInstructions(): string;
  /** 返回解析器类型名称 */
  getType(): string;
}

// ===== JSON 解析器 =====

/** JSON 解析器配置，与 Go 端 JSONParserConfig 对齐 */
export interface JSONParserConfig {
  /** 可选的 JSON Schema */
  schema?: Record<string, unknown>;
  /** 是否只接受顶级键 */
  keysOnly?: boolean;
  /** 是否允许额外的字段 */
  allowExtra?: boolean;
}

/** JSON 解析器，与 Go 端 JSONParser 对齐 */
export class JSONParser implements OutputParser {
  private schema?: Record<string, unknown>;
  private keysOnly: boolean;
  private allowExtra: boolean;

  constructor(config: JSONParserConfig = {}) {
    this.schema = config.schema;
    this.keysOnly = config.keysOnly ?? false;
    this.allowExtra = config.allowExtra ?? true;
  }

  parse(text: string): Record<string, unknown> {
    const jsonStr = extractJSON(text);
    if (!jsonStr) {
      throw new Error('输出中未找到有效的 JSON');
    }

    let result: unknown;
    try {
      result = JSON.parse(jsonStr);
    } catch {
      throw new Error(`JSON 解析失败: ${jsonStr.slice(0, 200)}`);
    }

    if (typeof result !== 'object' || result === null || Array.isArray(result)) {
      throw new Error('解析结果不是 JSON 对象');
    }

    const obj = result as Record<string, unknown>;

    // 键名校验
    if (this.keysOnly && this.schema && typeof this.schema === 'object') {
      const allowedKeys = new Set(Object.keys(this.schema));
      for (const key of Object.keys(obj)) {
        if (!allowedKeys.has(key)) {
          throw new Error(`不允许的键: ${key}`);
        }
      }
    }

    return obj;
  }

  formatInstructions(): string {
    if (this.schema) {
      return `\n请严格按照以下 JSON Schema 输出结果，不要输出任何其他内容：\n${JSON.stringify(this.schema, null, 2)}\n`;
    }
    return '\n请以 JSON 格式输出结果。\n';
  }

  getType(): string {
    return 'json';
  }
}

// ===== Markdown 解析器 =====

/** Markdown 解析器，提取代码块内容 */
export class MarkdownParser implements OutputParser {
  private language?: string;

  constructor(language?: string) {
    this.language = language;
  }

  parse(text: string): string {
    const lang = this.language ?? 'json';
    const pattern = new RegExp(`\`\`\`(?:${lang})?\\s*\\n?([\\s\\S]*?)\\n?\`\`\``, 'i');
    const match = text.match(pattern);
    if (match) {
      return match[1].trim();
    }
    // 回退：返回第一个代码块
    const fallback = text.match(/```[\s\S]*?\n([\s\S]*?)```/);
    if (fallback) {
      return fallback[1].trim();
    }
    return text.trim();
  }

  formatInstructions(): string {
    return `\n请以 Markdown 代码块格式输出结果，使用 \`\`\`${this.language ?? 'json'}\n\`\`\` 包裹。\n`;
  }

  getType(): string {
    return 'markdown';
  }
}

// ===== Regex 解析器 =====

/** Regex 解析器，使用正则表达式提取结构化数据 */
export class RegexParser implements OutputParser {
  private pattern: RegExp;
  private groupNames: string[];

  constructor(pattern: string | RegExp, groupNames: string[] = []) {
    this.pattern = typeof pattern === 'string' ? new RegExp(pattern, 'g') : pattern;
    this.groupNames = groupNames;
  }

  parse(text: string): Record<string, string> {
    const match = this.pattern.exec(text);
    if (!match) {
      throw new Error('输出与预期模式不匹配');
    }

    const result: Record<string, string> = {};
    if (this.groupNames.length > 0) {
      for (let i = 0; i < this.groupNames.length; i++) {
        result[this.groupNames[i]] = match[i + 1] ?? '';
      }
    } else {
      for (let i = 1; i < match.length; i++) {
        result[`group${i}`] = match[i];
      }
    }
    return result;
  }

  formatInstructions(): string {
    return `\n请按指定格式输出结果。\n`;
  }

  getType(): string {
    return 'regex';
  }
}

// ===== 辅助函数 =====

/** 从文本中提取 JSON（可能被包裹在 Markdown 代码块中） */
function extractJSON(text: string): string | null {
  // 尝试提取 ```json ... ``` 代码块
  const jsonBlock = text.match(/```(?:json)?\s*\n?([\s\S]*?)\n?```/);
  if (jsonBlock) {
    const inner = jsonBlock[1].trim();
    if (isValidJSON(inner)) return inner;
  }

  // 尝试查找 JSON 对象
  const objMatch = text.match(/\{[\s\S]*\}/);
  if (objMatch && isValidJSON(objMatch[0])) {
    return objMatch[0];
  }

  return null;
}

/** 检查字符串是否为有效 JSON */
function isValidJSON(str: string): boolean {
  try {
    JSON.parse(str);
    return true;
  } catch {
    return false;
  }
}
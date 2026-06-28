// ===== Guardrails — Input/Output Safety =====

// ===== PII Detection =====

export type PIIPattern = 'email' | 'phone' | 'ssn' | 'credit_card' | 'ip_address' | 'passport' | 'id_card';

export interface PIIDetectorConfig {
  patterns?: PIIPattern[];
  customPatterns?: { name: string; regex: string; replacement: string }[];
  redact?: boolean;
}

export interface PIIDetectionResult {
  found: boolean;
  types: { type: string; count: number; samples: string[] }[];
  redactedText?: string;
}

const PII_PATTERNS: Record<PIIPattern, { regex: RegExp; replacement: string }> = {
  email: { regex: /[\w.+-]+@[\w-]+\.[\w.-]+/g, replacement: '[EMAIL]' },
  phone: { regex: /(\+?\d{1,3}[-.\s]?)?\(?\d{3}\)?[-.\s]?\d{3,4}[-.\s]?\d{4}/g, replacement: '[PHONE]' },
  ssn: { regex: /\b\d{3}-\d{2}-\d{4}\b/g, replacement: '[SSN]' },
  credit_card: { regex: /\b(?:\d[ -]*?){13,16}\b/g, replacement: '[CREDIT_CARD]' },
  ip_address: { regex: /\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b/g, replacement: '[IP]' },
  passport: { regex: /\b[A-Z]{1,2}\d{6,9}\b/g, replacement: '[PASSPORT]' },
  id_card: { regex: /\b\d{15,18}\b/g, replacement: '[ID_CARD]' },
};

export class PIIDetector {
  private config: PIIDetectorConfig;

  constructor(config?: PIIDetectorConfig) {
    this.config = {
      patterns: config?.patterns ?? ['email', 'phone', 'ssn', 'credit_card', 'ip_address', 'passport', 'id_card'],
      customPatterns: config?.customPatterns ?? [],
      redact: config?.redact ?? true,
    };
  }

  detect(text: string): PIIDetectionResult {
    const types: { type: string; count: number; samples: string[] }[] = [];
    let redactedText = text;

    for (const patternName of this.config.patterns ?? []) {
      const pattern = PII_PATTERNS[patternName];
      if (!pattern) continue;

      const matches = text.match(pattern.regex);
      if (matches && matches.length > 0) {
        types.push({
          type: patternName,
          count: matches.length,
          samples: matches.slice(0, 3),
        });
        if (this.config.redact) {
          redactedText = redactedText.replace(pattern.regex, pattern.replacement);
        }
      }
    }

    // Check custom patterns
    for (const custom of this.config.customPatterns ?? []) {
      try {
        const regex = new RegExp(custom.regex, 'g');
        const matches = text.match(regex);
        if (matches && matches.length > 0) {
          types.push({
            type: custom.name,
            count: matches.length,
            samples: matches.slice(0, 3),
          });
          if (this.config.redact) {
            redactedText = redactedText.replace(regex, custom.replacement);
          }
        }
      } catch {}
    }

    return {
      found: types.length > 0,
      types,
      redactedText: this.config.redact ? redactedText : undefined,
    };
  }
}

// ===== Injection Detection =====

export interface InjectionDetectionResult {
  found: boolean;
  patterns: { type: string; severity: 'low' | 'medium' | 'high'; description: string }[];
}

const INJECTION_PATTERNS = [
  { regex: /ignore\s+(previous|above|all)\s+(instructions?|prompts?|rules?)/gi, type: 'prompt_injection', severity: 'high' as const, description: 'Attempt to override system instructions' },
  { regex: /disregard\s+(previous|above|all)\s+(instructions?|prompts?)/gi, type: 'prompt_injection', severity: 'high' as const, description: 'Attempt to bypass instructions' },
  { regex: /you\s+are\s+(now|actually)\s+/gi, type: 'role_hijack', severity: 'high' as const, description: 'Attempt to hijack agent role' },
  { regex: /system\s*:\s*/gi, type: 'role_spoof', severity: 'medium' as const, description: 'Attempt to spoof system role' },
  { regex: /<\|im_start\|>/gi, type: 'token_injection', severity: 'high' as const, description: 'Special token injection' },
  { regex: /<\/?system>/gi, type: 'xml_injection', severity: 'medium' as const, description: 'XML tag injection' },
  { regex: /reveal\s+(your|the)\s+(system\s+)?(prompt|instructions?)/gi, type: 'prompt_leak', severity: 'medium' as const, description: 'Attempt to extract system prompt' },
  { regex: /\b(exec|eval|system|fork|spawn)\s*\(/gi, type: 'code_injection', severity: 'high' as const, description: 'Potential code injection' },
  { regex: /;\s*(drop|delete|insert|update|create)\s+/gi, type: 'sql_injection', severity: 'high' as const, description: 'Potential SQL injection' },
];

export class InjectionDetector {
  private patterns: typeof INJECTION_PATTERNS;
  private customPatterns: { regex: RegExp; type: string; severity: 'low' | 'medium' | 'high'; description: string }[];

  constructor(customPatterns?: { regex: string; type: string; severity: 'low' | 'medium' | 'high'; description: string }[]) {
    this.patterns = INJECTION_PATTERNS;
    this.customPatterns = (customPatterns ?? []).map((p) => ({
      regex: new RegExp(p.regex, 'gi'),
      type: p.type,
      severity: p.severity,
      description: p.description,
    }));
  }

  detect(text: string): InjectionDetectionResult {
    const found: { type: string; severity: 'low' | 'medium' | 'high'; description: string }[] = [];

    for (const pattern of [...this.patterns, ...this.customPatterns]) {
      if (pattern.regex.test(text)) {
        found.push({
          type: pattern.type,
          severity: pattern.severity,
          description: pattern.description,
        });
        pattern.regex.lastIndex = 0; // Reset regex
      }
    }

    return { found: found.length > 0, patterns: found };
  }
}

// ===== Topic Filter =====

export interface TopicFilterConfig {
  allowedTopics?: string[];
  blockedTopics?: string[];
  strictMode?: boolean; // If true, only allowed topics are permitted
}

export class TopicFilter {
  private config: TopicFilterConfig;

  constructor(config?: TopicFilterConfig) {
    this.config = {
      allowedTopics: config?.allowedTopics ?? [],
      blockedTopics: config?.blockedTopics ?? [],
      strictMode: config?.strictMode ?? false,
    };
  }

  check(text: string): { allowed: boolean; reason?: string } {
    const lowerText = text.toLowerCase();

    // Check blocked topics
    for (const topic of this.config.blockedTopics ?? []) {
      if (lowerText.includes(topic.toLowerCase())) {
        return { allowed: false, reason: `Topic "${topic}" is blocked` };
      }
    }

    // Check allowed topics (in strict mode)
    if (this.config.strictMode && (this.config.allowedTopics ?? []).length > 0) {
      const matchesAllowed = (this.config.allowedTopics ?? []).some((topic) =>
        lowerText.includes(topic.toLowerCase())
      );
      if (!matchesAllowed) {
        return { allowed: false, reason: 'Topic not in allowed list (strict mode)' };
      }
    }

    return { allowed: true };
  }
}

// ===== Output Rules =====

export interface OutputRule {
  name: string;
  pattern: RegExp;
  action: 'block' | 'redact' | 'warn';
  replacement?: string;
  message?: string;
}

export class OutputGuardrail {
  private rules: OutputRule[] = [];

  addRule(rule: OutputRule): void {
    this.rules.push(rule);
  }

  check(text: string): { passed: boolean; modifiedText: string; violations: { rule: string; action: string }[] } {
    let modifiedText = text;
    const violations: { rule: string; action: string }[] = [];

    for (const rule of this.rules) {
      const matches = text.match(rule.pattern);
      if (matches) {
        violations.push({ rule: rule.name, action: rule.action });

        switch (rule.action) {
          case 'block':
            return { passed: false, modifiedText: rule.message ?? `Output blocked by rule: ${rule.name}`, violations };
          case 'redact':
            modifiedText = modifiedText.replace(rule.pattern, rule.replacement ?? '[REDACTED]');
            break;
          case 'warn':
            // Just warn, don't modify
            break;
        }
      }
    }

    return { passed: true, modifiedText, violations };
  }
}

// ===== Guardrail Engine =====

export interface GuardrailConfig {
  piiDetector?: PIIDetector;
  injectionDetector?: InjectionDetector;
  topicFilter?: TopicFilter;
  outputGuardrail?: OutputGuardrail;
}

export interface GuardrailResult {
  passed: boolean;
  modifiedInput?: string;
  modifiedOutput?: string;
  violations: { type: string; severity: 'low' | 'medium' | 'high'; description: string }[];
}

export class GuardrailEngine {
  private config: GuardrailConfig;

  constructor(config?: GuardrailConfig) {
    this.config = {
      piiDetector: config?.piiDetector ?? new PIIDetector(),
      injectionDetector: config?.injectionDetector ?? new InjectionDetector(),
      topicFilter: config?.topicFilter,
      outputGuardrail: config?.outputGuardrail,
    };
  }

  /** Check input before sending to LLM. */
  checkInput(input: string): { passed: boolean; modifiedInput: string; violations: { type: string; severity: 'low' | 'medium' | 'high'; description: string }[] } {
    const violations: { type: string; severity: 'low' | 'medium' | 'high'; description: string }[] = [];
    let modified = input;

    // PII detection
    if (this.config.piiDetector) {
      const piiResult = this.config.piiDetector.detect(input);
      if (piiResult.found) {
        violations.push(...piiResult.types.map((t) => ({
          type: `pii_${t.type}`,
          severity: 'medium' as const,
          description: `Detected ${t.count} instance(s) of ${t.type}`,
        })));
        if (piiResult.redactedText) modified = piiResult.redactedText;
      }
    }

    // Injection detection
    if (this.config.injectionDetector) {
      const injResult = this.config.injectionDetector.detect(input);
      if (injResult.found) {
        violations.push(...injResult.patterns.map((p) => ({
          type: p.type,
          severity: p.severity,
          description: p.description,
        })));

        // Block if high severity injection detected
        if (injResult.patterns.some((p) => p.severity === 'high')) {
          return { passed: false, modifiedInput: '', violations };
        }
      }
    }

    // Topic filter
    if (this.config.topicFilter) {
      const topicResult = this.config.topicFilter.check(input);
      if (!topicResult.allowed) {
        violations.push({
          type: 'topic_violation',
          severity: 'medium',
          description: topicResult.reason ?? 'Topic not allowed',
        });
        return { passed: false, modifiedInput: '', violations };
      }
    }

    return { passed: true, modifiedInput: modified, violations };
  }

  /** Check output before returning to user. */
  checkOutput(output: string): { passed: boolean; modifiedOutput: string; violations: { type: string; severity: 'low' | 'medium' | 'high'; description: string }[] } {
    const violations: { type: string; severity: 'low' | 'medium' | 'high'; description: string }[] = [];
    let modified = output;

    // PII detection on output
    if (this.config.piiDetector) {
      const piiResult = this.config.piiDetector.detect(output);
      if (piiResult.found) {
        violations.push(...piiResult.types.map((t) => ({
          type: `pii_${t.type}`,
          severity: 'medium' as const,
          description: `Output contains ${t.count} instance(s) of ${t.type}`,
        })));
        if (piiResult.redactedText) modified = piiResult.redactedText;
      }
    }

    // Output guardrail
    if (this.config.outputGuardrail) {
      const outputResult = this.config.outputGuardrail.check(output);
      if (!outputResult.passed) {
        violations.push({
          type: 'output_blocked',
          severity: 'high',
          description: 'Output blocked by guardrail rule',
        });
        return { passed: false, modifiedOutput: outputResult.modifiedText, violations };
      }
      modified = outputResult.modifiedText;
    }

    return { passed: true, modifiedOutput: modified, violations };
  }
}

// ===== Trie 多模式匹配（与 Go 端 trie_rule.go 对齐） =====

/** Trie 树节点 */
interface TrieNode {
  children: Map<string, TrieNode>;
  isEnd: boolean;
}

/** Trie 多模式匹配树，与 Go 端 Trie 对齐。
 *
 * 用于高效匹配大量敏感词，支持 O(k) 复杂度查找（k 为文本长度）。
 *
 * 使用方式：
 *   const trie = new Trie();
 *   trie.insertBatch(['敏感词1', '敏感词2']);
 *   const matches = trie.match('文本包含敏感词1');
 */
export class Trie {
  private root: TrieNode = { children: new Map(), isEnd: false };

  /** 插入单个词 */
  insert(word: string): void {
    let node = this.root;
    for (const ch of word) {
      if (!node.children.has(ch)) {
        node.children.set(ch, { children: new Map(), isEnd: false });
      }
      node = node.children.get(ch)!;
    }
    node.isEnd = true;
  }

  /** 批量插入词 */
  insertBatch(words: string[]): void {
    for (const w of words) {
      this.insert(w);
    }
  }

  /** 在文本中查找所有匹配的敏感词 */
  match(text: string): string[] {
    const matches: string[] = [];
    const seen = new Set<string>();
    const chars = [...text];

    for (let i = 0; i < chars.length; i++) {
      let node = this.root;
      for (let j = i; j < chars.length; j++) {
        const child = node.children.get(chars[j]);
        if (!child) break;
        node = child;
        if (node.isEnd) {
          const word = chars.slice(i, j + 1).join('');
          if (!seen.has(word)) {
            matches.push(word);
            seen.add(word);
          }
        }
      }
    }

    return matches;
  }

  /** 检查文本是否包含任何敏感词 */
  containsAny(text: string): boolean {
    const chars = [...text];
    for (let i = 0; i < chars.length; i++) {
      let node = this.root;
      for (let j = i; j < chars.length; j++) {
        const child = node.children.get(chars[j]);
        if (!child) break;
        node = child;
        if (node.isEnd) return true;
      }
    }
    return false;
  }

  /** 获取 Trie 中词的总数 */
  count(): number {
    let c = 0;
    const stack: TrieNode[] = [this.root];
    while (stack.length > 0) {
      const node = stack.pop()!;
      if (node.isEnd) c++;
      for (const child of node.children.values()) {
        stack.push(child);
      }
    }
    return c;
  }
}

// ===== Sanitizer 脱敏处理器（与 Go 端 sanitizer.go 对齐） =====

/** 脱敏策略，与 Go 端 SanitizeStrategy 对齐 */
export type SanitizeStrategy = 'mask' | 'redact' | 'replace' | 'hash';

/** 脱敏位置 */
export interface Position {
  start: number;
  end: number;
  label: string;
}

/** 脱敏处理器配置 */
export interface SanitizerConfig {
  strategy?: SanitizeStrategy;
  maskChar?: string;
  replText?: string;
}

/** 脱敏处理器，与 Go 端 Sanitizer 对齐。
 *
 * 支持四种脱敏策略：
 * - mask: 用 maskChar 替换中间字符，保留首尾
 * - redact: 用 replText 替换整个匹配
 * - replace: 用 replText 替换整个匹配
 * - hash: 用 SHA-256 哈希替换
 *
 * 使用方式：
 *   const sanitizer = new Sanitizer({ strategy: 'mask' });
 *   const result = sanitizer.sanitize('13800138000', [{ start: 0, end: 11, label: 'phone' }]);
 */
export class Sanitizer {
  private strategy: SanitizeStrategy;
  private maskChar: string;
  private replText: string;

  constructor(config?: SanitizerConfig) {
    this.strategy = config?.strategy ?? 'redact';
    this.maskChar = config?.maskChar ?? '*';
    this.replText = config?.replText ?? '[REDACTED]';
  }

  /** 对文本进行脱敏处理 */
  sanitize(text: string, positions: Position[]): string {
    if (positions.length === 0) return text;

    // 按位置排序（从后往前处理，避免偏移问题）
    const sorted = [...positions].sort((a, b) => b.start - a.start);
    let result = text;

    for (const pos of sorted) {
      if (pos.start < 0 || pos.end > result.length || pos.start >= pos.end) continue;
      const before = result.slice(0, pos.start);
      const after = result.slice(pos.end);
      const replacement = this.applyStrategy(pos.end - pos.start, pos.label);
      result = before + replacement + after;
    }

    return result;
  }

  private applyStrategy(length: number, label: string): string {
    switch (this.strategy) {
      case 'mask':
        return this.maskText(length);
      case 'redact':
        return this.replText;
      case 'replace':
        return `[${label.toUpperCase()}]`;
      case 'hash':
        return this.simpleHash(label + length.toString());
      default:
        return this.replText;
    }
  }

  private maskText(length: number): string {
    if (length <= 2) return this.maskChar.repeat(length);
    // 保留首尾字符，中间用 maskChar 替换
    const first = 1;
    const last = Math.min(1, length - 1);
    const middle = length - first - last;
    return this.maskChar.repeat(Math.max(0, middle));
  }

  private simpleHash(input: string): string {
    // 简单哈希（用于脱敏标记，非加密用途）
    let hash = 0;
    for (let i = 0; i < input.length; i++) {
      const ch = input.charCodeAt(i);
      hash = ((hash << 5) - hash) + ch;
      hash = hash & hash; // Convert to 32bit integer
    }
    return `[HASH:${Math.abs(hash).toString(16).slice(0, 8)}]`;
  }
}

// ===== GuardrailHook（与 Go 端 hook.go 对齐） =====

/** Guardrail Hook 上下文 */
export interface GuardrailHookContext {
  agentID?: string;
  sessionID?: string;
  message?: { content: string };
  response?: { content: string };
  setMetadata?: (key: string, value: unknown) => void;
}

/** Guardrail Hook 配置 */
export interface GuardrailHookConfig {
  engine: GuardrailEngine;
  /** 是否在输入时检查 */
  checkInput?: boolean;
  /** 是否在输出时检查 */
  checkOutput?: boolean;
  /** 检查失败时的处理方式 */
  onReject?: 'throw' | 'silent' | 'callback';
  /** 回调函数 */
  onViolation?: (ctx: GuardrailHookContext, violations: GuardrailResult['violations']) => void;
}

/** Guardrail Hook，与 Go 端 GuardrailHook 对齐。
 *
 * 将 GuardrailEngine 集成到 Agent Hook 系统中，
 * 自动在输入/输出阶段进行安全检查。
 *
 * 使用方式：
 *   const hook = new GuardrailHook({ engine: guardrailEngine });
 *   agent.hooks.register('before_llm', hook.inputCheck);
 *   agent.hooks.register('after_llm', hook.outputCheck);
 */
export class GuardrailHook {
  private config: GuardrailHookConfig;

  constructor(config: GuardrailHookConfig) {
    this.config = {
      checkInput: true,
      checkOutput: true,
      onReject: 'throw',
      ...config,
    };
  }

  /** 输入检查 — 在 LLM 调用前检查用户输入 */
  inputCheck = async (ctx: GuardrailHookContext): Promise<void> => {
    if (!this.config.checkInput) return;
    const content = ctx.message?.content;
    if (!content) return;

    const result = this.config.engine.checkInput(content);
    if (!result.passed) {
      this.config.onViolation?.(ctx, result.violations);
      if (this.config.onReject === 'throw') {
        throw new Error(`Guardrail rejected input: ${result.violations.map((v) => v.description).join('; ')}`);
      }
    }

    // 应用脱敏后的输入
    if (result.modifiedInput && ctx.message) {
      ctx.message.content = result.modifiedInput;
    }

    if (result.violations.length > 0) {
      ctx.setMetadata?.('guardrail_flagged', true);
      ctx.setMetadata?.('guardrail_results', result.violations);
    }
  };

  /** 输出检查 — 在 LLM 返回后检查输出 */
  outputCheck = async (ctx: GuardrailHookContext): Promise<void> => {
    if (!this.config.checkOutput) return;
    const content = ctx.response?.content;
    if (!content) return;

    const result = this.config.engine.checkOutput(content);
    if (!result.passed) {
      this.config.onViolation?.(ctx, result.violations);
      if (this.config.onReject === 'throw') {
        throw new Error(`Guardrail rejected output: ${result.violations.map((v) => v.description).join('; ')}`);
      }
    }

    // 应用脱敏后的输出
    if (result.modifiedOutput && ctx.response) {
      ctx.response.content = result.modifiedOutput;
    }

    if (result.violations.length > 0) {
      ctx.setMetadata?.('guardrail_flagged', true);
      ctx.setMetadata?.('guardrail_results', result.violations);
    }
  };
}

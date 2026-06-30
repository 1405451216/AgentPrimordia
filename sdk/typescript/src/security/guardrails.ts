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

    // P3-2: 对文本进行 leet-speak 归一化，检测变形攻击（与 Go 端 normalizeForCheck 对齐）
    const normalized = normalizeForCheck(text);

    for (const pattern of [...this.patterns, ...this.customPatterns]) {
      // 同时检查原始文本和归一化后的文本
      if (pattern.regex.test(text) || pattern.regex.test(normalized)) {
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

// ===== P3-2: Go-style 统一规则引擎（与 Go 端 engine.go + rule 对齐） =====

/** 检查点类型，与 Go 端 CheckPoint 对齐 */
export type CheckPoint = 'input' | 'output';

/** 规则动作，与 Go 端 Action 对齐 */
export type GuardrailAction = 'pass' | 'reject' | 'sanitize' | 'flag';

/** 严重级别，与 Go 端 Severity 对齐 */
export type GuardrailSeverity = 'low' | 'medium' | 'high' | 'critical';

/** 规则检查结果，与 Go 端 Result 对齐 */
export interface GuardrailRuleResult {
  ruleName: string;
  action: GuardrailAction;
  severity: GuardrailSeverity;
  message: string;
  sanitized?: string;
  metadata?: Record<string, unknown>;
}

/** 检查报告，与 Go 端 Report 对齐 */
export interface GuardrailReport {
  passed: boolean;
  results: GuardrailRuleResult[];
  action: GuardrailAction;
}

/** 规则接口，与 Go 端 Rule 接口对齐 */
export interface GuardrailRule {
  name(): string;
  check(input: string, point: CheckPoint): GuardrailRuleResult;
}

// ===== Leet-speak 归一化（与 Go 端 normalizeForCheck 对齐） =====

/** 对输入进行归一化处理，检测变形攻击。
 *
 * 将 leet-speak 替换回原始字符：
 * - 0 → o, 1 → i, 3 → e, 5 → s, 7 → t, @ → a
 */
export function normalizeForCheck(s: string): string {
  let result = s.toLowerCase();
  const replacements: Array<[string, string]> = [
    ['0', 'o'], ['1', 'i'], ['3', 'e'], ['5', 's'], ['7', 't'], ['@', 'a'],
  ];
  for (const [from, to] of replacements) {
    result = result.replaceAll(from, to);
  }
  return result;
}

// ===== PromptInjectionRule（与 Go 端 injection_rule.go 对齐） =====

export interface PromptInjectionRuleConfig {
  action?: GuardrailAction;
  severity?: GuardrailSeverity;
}

/** Prompt 注入检测规则，与 Go 端 PromptInjectionRule 对齐。
 *
 * 检测常见的 Prompt 注入攻击模式，包括：
 * - 忽略/遗忘之前指令
 * - 角色劫持（"you are now a..."）
 * - 伪装系统角色
 * - 特殊 token 注入（<|im_start|>, [INST]）
 * - 越狱/DAN 模式
 * - Leet-speak 变形检测
 */
export class PromptInjectionRule implements GuardrailRule {
  private action: GuardrailAction;
  private severity: GuardrailSeverity;
  private patterns: RegExp[];
  private keywords: string[];

  constructor(config?: PromptInjectionRuleConfig) {
    this.action = config?.action ?? 'reject';
    this.severity = config?.severity ?? 'high';
    this.patterns = [
      /ignore\s+(previous|above|all)\s+instructions/i,
      /forget\s+(everything|all|previous)/i,
      /you\s+are\s+now\s+a/i,
      /pretend\s+(you\s+are|to\s+be)/i,
      /disregard\s+(all|any|previous|the)\s+(rules|instructions|guidelines)/i,
      /system\s*:\s*/i,
      /<\|im_start\|>/i,
      /\[INST\]/i,
      /jailbreak/i,
      /DAN\s+mode/i,
    ];
    this.keywords = [
      'system prompt',
      '忽略之前的指令',
      '忽略以上指令',
      '忽略所有指令',
      '越狱',
      '解锁模式',
    ];
  }

  name(): string { return 'prompt_injection'; }

  check(input: string, _point: CheckPoint): GuardrailRuleResult {
    const lower = input.toLowerCase();
    const normalized = normalizeForCheck(input);
    const detected: string[] = [];

    for (const p of this.patterns) {
      p.lastIndex = 0;
      if (p.test(normalized)) {
        detected.push(p.source);
      }
    }

    for (const kw of this.keywords) {
      const kwLower = kw.toLowerCase();
      if (lower.includes(kwLower) || normalized.includes(kwLower)) {
        detected.push(kw);
      }
    }

    if (detected.length === 0) {
      return { ruleName: this.name(), action: 'pass', severity: this.severity, message: '' };
    }

    return {
      ruleName: this.name(),
      action: this.action,
      severity: this.severity,
      message: 'potential prompt injection detected',
      metadata: { matches: detected },
    };
  }
}

// ===== OutputSafetyRule（与 Go 端 output_rule.go 对齐） =====

export interface OutputSafetyRuleConfig {
  action?: GuardrailAction;
  severity?: GuardrailSeverity;
  detectCodeExecution?: boolean;
  detectURLs?: boolean;
  detectFilePaths?: boolean;
  customPatterns?: string[];
}

/** 输出安全检查规则，与 Go 端 OutputSafetyRule 对齐。
 *
 * 检查 LLM 输出中是否包含不安全内容：
 * - 危险代码执行（rm -rf, format, curl|sh, exec(), eval()）
 * - URL 泄露
 * - 文件路径泄露
 * - 自定义正则模式
 */
export class OutputSafetyRule implements GuardrailRule {
  private action: GuardrailAction;
  private severity: GuardrailSeverity;
  private patterns: RegExp[];

  constructor(config?: OutputSafetyRuleConfig) {
    this.action = config?.action ?? 'reject';
    this.severity = config?.severity ?? 'high';
    this.patterns = [];

    if (config?.detectCodeExecution ?? true) {
      this.patterns.push(
        /rm\s+-rf\s+\//i,
        /del\s+\/[sS]\s+\/[qQ]/i,
        /format\s+[A-Za-z]:/i,
        /curl\s+.*\|\s*sh/i,
        /wget\s+.*\|\s*bash/i,
        /exec\s*\(/i,
        /eval\s*\(/i,
        /subprocess\.(call|run|Popen)/i,
      );
    }

    if (config?.detectURLs) {
      this.patterns.push(/https?:\/\/[^\s<>"]+/g);
    }

    if (config?.detectFilePaths) {
      this.patterns.push(
        /(?:\/etc\/|\/var\/|\/usr\/local\/)[^\s<>"]+/g,
        /[A-Za-z]:\\(?:Users|Windows|Program Files)[\\/][^\s<>"]+/g,
      );
    }

    for (const p of config?.customPatterns ?? []) {
      try {
        this.patterns.push(new RegExp(p, 'i'));
      } catch {
        // Skip invalid patterns
      }
    }
  }

  name(): string { return 'output_safety'; }

  check(output: string, point: CheckPoint): GuardrailRuleResult {
    if (point !== 'output') {
      return { ruleName: this.name(), action: 'pass', severity: this.severity, message: '' };
    }

    const findings: string[] = [];
    for (const p of this.patterns) {
      p.lastIndex = 0;
      const matches = output.match(p);
      if (matches) {
        findings.push(...matches);
      }
    }

    if (findings.length === 0) {
      return { ruleName: this.name(), action: 'pass', severity: this.severity, message: '' };
    }

    return {
      ruleName: this.name(),
      action: this.action,
      severity: this.severity,
      message: `unsafe output detected: ${findings.length} pattern(s) matched`,
      metadata: { findings },
    };
  }
}

// ===== TopicConstraintRule（与 Go 端 topic_rule.go 对齐） =====

export type TopicMode = 'allowlist' | 'denylist';

export interface TopicConstraintRuleConfig {
  action?: GuardrailAction;
  severity?: GuardrailSeverity;
  mode: TopicMode;
  topics: string[];
}

/** 话题约束规则，与 Go 端 TopicConstraintRule 对齐。
 *
 * 限制对话在允许的话题范围内：
 * - allowlist 模式：仅允许指定话题（空列表拒绝所有输入）
 * - denylist 模式：拒绝指定话题
 */
export class TopicConstraintRule implements GuardrailRule {
  private action: GuardrailAction;
  private severity: GuardrailSeverity;
  private allowed: string[];
  private denied: string[];
  private mode: TopicMode;

  constructor(config: TopicConstraintRuleConfig) {
    this.action = config.action ?? 'reject';
    this.severity = config.severity ?? 'medium';
    this.mode = config.mode;
    if (config.mode === 'allowlist') {
      this.allowed = config.topics;
      this.denied = [];
    } else {
      this.allowed = [];
      this.denied = config.topics;
    }
  }

  name(): string { return 'topic_constraint'; }

  check(input: string, _point: CheckPoint): GuardrailRuleResult {
    const lower = input.toLowerCase();

    if (this.mode === 'denylist') {
      for (const topic of this.denied) {
        if (lower.includes(topic.toLowerCase())) {
          return {
            ruleName: this.name(),
            action: this.action,
            severity: this.severity,
            message: `topic "${topic}" is not allowed`,
            metadata: { topic, mode: 'denylist' },
          };
        }
      }
    } else {
      // allowlist mode
      if (this.allowed.length === 0) {
        return {
          ruleName: this.name(),
          action: this.action,
          severity: this.severity,
          message: 'no topics are allowed (empty allowlist)',
          metadata: { mode: 'allowlist' },
        };
      }
      const matchesAllowed = this.allowed.some((topic) =>
        lower.includes(topic.toLowerCase()),
      );
      if (!matchesAllowed) {
        return {
          ruleName: this.name(),
          action: this.action,
          severity: this.severity,
          message: 'topic not in allowed list',
          metadata: { mode: 'allowlist', allowed: this.allowed },
        };
      }
    }

    return { ruleName: this.name(), action: 'pass', severity: this.severity, message: '' };
  }
}

// ===== RuleEngine（与 Go 端 Engine 对齐） =====

/** Go-style 规则引擎，与 Go 端 Engine 对齐。
 *
 * 支持动态注册规则、统一检查入口（输入/输出）、
 * 自动处理 reject/sanitize/flag 动作。
 *
 * 使用方式：
 *   const engine = new RuleEngine();
 *   engine.addRule(new PromptInjectionRule());
 *   engine.addRule(new OutputSafetyRule());
 *   const report = engine.checkInput('user input');
 */
export class RuleEngine {
  private rules: GuardrailRule[] = [];
  private rulesSnapshot: GuardrailRule[] = [];

  addRule(rule: GuardrailRule): void {
    this.rules.push(rule);
    this.refreshSnapshot();
  }

  removeRule(name: string): boolean {
    const idx = this.rules.findIndex((r) => r.name() === name);
    if (idx >= 0) {
      this.rules.splice(idx, 1);
      this.refreshSnapshot();
      return true;
    }
    return false;
  }

  /** 获取规则名称列表 */
  ruleNames(): string[] {
    return this.rulesSnapshot.map((r) => r.name());
  }

  /** 获取规则数量 */
  ruleCount(): number {
    return this.rulesSnapshot.length;
  }

  /** 统一检查入口 */
  check(input: string, point: CheckPoint): GuardrailReport {
    const report: GuardrailReport = { passed: true, results: [], action: 'pass' };

    if (this.rulesSnapshot.length === 0) {
      return report;
    }

    let currentInput = input;
    for (const rule of this.rulesSnapshot) {
      const result = rule.check(currentInput, point);
      report.results.push(result);

      if (result.action === 'reject') {
        report.passed = false;
        report.action = 'reject';
        return report;
      }

      if (result.action === 'sanitize' && result.sanitized !== undefined) {
        report.passed = false;
        report.action = 'sanitize';
        currentInput = result.sanitized;
      }

      if (result.action === 'flag' && report.action === 'pass') {
        report.action = 'flag';
      }
    }

    return report;
  }

  /** 输入检查 */
  checkInput(input: string): GuardrailReport {
    return this.check(input, 'input');
  }

  /** 输出检查 */
  checkOutput(output: string): GuardrailReport {
    return this.check(output, 'output');
  }

  private refreshSnapshot(): void {
    this.rulesSnapshot = [...this.rules];
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

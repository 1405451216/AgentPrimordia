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

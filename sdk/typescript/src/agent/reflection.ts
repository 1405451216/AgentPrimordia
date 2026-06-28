import type { Provider } from '../llm/provider.js';

// ===== Reflection Types =====

export type Severity = 'low' | 'medium' | 'high' | 'critical';

export interface Reflection {
  strengths: string[];
  weaknesses: string[];
  suggestions: string[];
  confidence: number;
}

export interface Issue {
  description: string;
  location?: string;
  severity: Severity;
}

export interface Correction {
  original: string;
  corrected: string;
  reason: string;
}

export interface Critique {
  issues: Issue[];
  severity: Severity;
  corrections: Correction[];
}

export interface Reflector {
  reflect(input: string, output: string): Promise<Reflection>;
  critique(output: string): Promise<Critique>;
  improve(output: string, feedback: Critique): Promise<string>;
}

// ===== LLM Reflector =====

export class LLMReflector implements Reflector {
  private provider: Provider;
  private model?: string;

  constructor(provider: Provider, model?: string) {
    this.provider = provider;
    this.model = model;
  }

  async reflect(input: string, output: string): Promise<Reflection> {
    const prompt = `Analyze the following conversation and provide a reflection.

Input: ${input}
Output: ${output}

Return JSON with:
- strengths: array of strengths
- weaknesses: array of weaknesses
- suggestions: array of improvement suggestions
- confidence: float 0-1

Return ONLY valid JSON.`;

    const resp = await this.provider.complete({
      messages: [{ role: 'user', content: prompt }],
      model: this.model,
      temperature: 0,
    });

    return this.parseReflection(resp.content);
  }

  async critique(output: string): Promise<Critique> {
    const prompt = `Critique the following output. Identify issues and corrections.

Output: ${output}

Return JSON with:
- issues: array of {description, location, severity}
- severity: overall severity (low/medium/high/critical)
- corrections: array of {original, corrected, reason}

Return ONLY valid JSON.`;

    const resp = await this.provider.complete({
      messages: [{ role: 'user', content: prompt }],
      model: this.model,
      temperature: 0,
    });

    return this.parseCritique(resp.content);
  }

  async improve(output: string, feedback: Critique): Promise<string> {
    const corrections = feedback.corrections.map(c =>
      `Original: "${c.original}" → Corrected: "${c.corrected}" (Reason: ${c.reason})`
    ).join('\n');

    const prompt = `Improve the following output based on feedback.

Original output: ${output}

Corrections:
${corrections}

Return the improved output only.`;

    const resp = await this.provider.complete({
      messages: [{ role: 'user', content: prompt }],
      model: this.model,
      temperature: 0,
    });

    return resp.content;
  }

  private parseReflection(text: string): Reflection {
    const json = this.extractJSON(text);
    return {
      strengths: Array.isArray(json.strengths) ? (json.strengths as string[]) : [],
      weaknesses: Array.isArray(json.weaknesses) ? (json.weaknesses as string[]) : [],
      suggestions: Array.isArray(json.suggestions) ? (json.suggestions as string[]) : [],
      confidence: typeof json.confidence === 'number' ? json.confidence : 0.5,
    };
  }

  private parseCritique(text: string): Critique {
    const json = this.extractJSON(text);
    const issues = Array.isArray(json.issues) ? (json.issues as Record<string, unknown>[]) : [];
    const corrections = Array.isArray(json.corrections) ? (json.corrections as Record<string, unknown>[]) : [];
    return {
      issues: issues.map((i) => ({
        description: String(i.description ?? ''),
        location: i.location ? String(i.location) : undefined,
        severity: (i.severity as Severity) ?? 'medium',
      })),
      severity: (json.severity as Severity) ?? 'medium',
      corrections: corrections.map((c) => ({
        original: String(c.original ?? ''),
        corrected: String(c.corrected ?? ''),
        reason: String(c.reason ?? ''),
      })),
    };
  }

  private extractJSON(text: string): Record<string, unknown> {
    try { return JSON.parse(text); } catch {}
    const match = text.match(/\{[\s\S]*\}/);
    if (match) {
      try { return JSON.parse(match[0]); } catch {}
    }
    return {};
  }
}

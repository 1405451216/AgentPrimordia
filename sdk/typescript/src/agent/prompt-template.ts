/**
 * PromptTemplate supports {{.Variable}} format variable injection.
 * Allows dynamic system prompt construction with agent name, permissions, etc.
 */
export class PromptTemplate {
  private template: string;
  private vars: Map<string, string> = new Map();

  constructor(template: string) {
    this.template = template;
  }

  /** Set a template variable. */
  withVar(name: string, value: string): PromptTemplate {
    this.vars.set(name, value);
    return this;
  }

  /** Set multiple template variables. */
  withVars(vars: Record<string, string>): PromptTemplate {
    for (const [k, v] of Object.entries(vars)) {
      this.vars.set(k, v);
    }
    return this;
  }

  /** Set file scope rules in the template. */
  withScopeRules(scopes: string[]): PromptTemplate {
    this.vars.set('ScopeRules', scopes.map((s) => `- ${s}`).join('\n'));
    return this;
  }

  /** Render the template with all variables. */
  render(): string {
    let result = this.template;
    for (const [key, value] of this.vars) {
      result = result.replaceAll(`{{.${key}}}`, value);
    }
    return result;
  }

  /** Create a copy of this template. */
  clone(): PromptTemplate {
    const t = new PromptTemplate(this.template);
    for (const [k, v] of this.vars) {
      t.vars.set(k, v);
    }
    return t;
  }
}

/** Default system prompt template with agent name and optional scope rules. */
export function defaultSystemPrompt(): PromptTemplate {
  return new PromptTemplate(
    'You are an AI assistant named {{.AgentName}}.\n' +
    '{{.ScopeRules}}\n' +
    'Follow instructions carefully and use available tools when needed.'
  );
}

/** Code assistant template with file scope restrictions. */
export function codeAssistantTemplate(basePrompt: string, scopes: string[]): PromptTemplate {
  return new PromptTemplate(basePrompt + '\n\nFile access scope:\n{{.ScopeRules}}')
    .withScopeRules(scopes);
}

/** RAG context injection template. */
export function ragContextTemplate(): PromptTemplate {
  return new PromptTemplate(
    '=== Relevant Knowledge ===\n{{.Context}}\n=== End Knowledge ===\n'
  );
}

/** Format RAG documents for prompt injection. */
export interface RAGDoc {
  id: string;
  content: string;
  score: number;
  source?: string;
  role?: string;
}

export function formatRAGDocuments(docs: RAGDoc[]): string {
  if (docs.length === 0) return '';
  let result = '=== Relevant Knowledge ===\n';
  for (let i = 0; i < docs.length; i++) {
    const doc = docs[i];
    const role = doc.role ?? 'knowledge';
    result += `[${i + 1} | score: ${doc.score.toFixed(2)} | ${role}] ${doc.content}\n`;
  }
  result += '=== End Knowledge ===\n';
  return result;
}

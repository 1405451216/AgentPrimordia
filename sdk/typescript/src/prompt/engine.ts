import type { Message } from '../types.js';

// ===== Prompt Template =====

export interface PromptTemplate {
  name: string;
  template: string;
  variables: string[];
  system?: string;
}

export class PromptEngine {
  private templates: Map<string, PromptTemplate> = new Map();

  register(name: string, template: string, system?: string): void {
    const variables = this.extractVariables(template);
    this.templates.set(name, { name, template, variables, system });
  }

  get(name: string): PromptTemplate | undefined {
    return this.templates.get(name);
  }

  render(name: string, vars: Record<string, string>): Message[] {
    const tpl = this.templates.get(name);
    if (!tpl) throw new Error(`Template "${name}" not found`);

    const messages: Message[] = [];
    if (tpl.system) {
      messages.push({ role: 'system', content: this.interpolate(tpl.system, vars) });
    }
    messages.push({ role: 'user', content: this.interpolate(tpl.template, vars) });
    return messages;
  }

  list(): string[] {
    return Array.from(this.templates.keys());
  }

  has(name: string): boolean {
    return this.templates.has(name);
  }

  private extractVariables(template: string): string[] {
    const matches = template.matchAll(/\{\{(\w+)\}\}/g);
    const vars = new Set<string>();
    for (const match of matches) {
      vars.add(match[1]!);
    }
    return Array.from(vars);
  }

  private interpolate(template: string, vars: Record<string, string>): string {
    return template.replace(/\{\{(\w+)\}\}/g, (_, key: string) => {
      return vars[key] ?? `{{${key}}}`;
    });
  }
}

// ===== Few-Shot Prompting =====

export interface FewShotExample {
  input: string;
  output: string;
}

export class FewShotPrompt {
  private examples: FewShotExample[] = [];
  private format: 'qa' | 'chat' | 'json' = 'qa';

  constructor(format?: 'qa' | 'chat' | 'json') {
    if (format) this.format = format;
  }

  addExample(input: string, output: string): this {
    this.examples.push({ input, output });
    return this;
  }

  build(query: string, system?: string): Message[] {
    const messages: Message[] = [];
    if (system) messages.push({ role: 'system', content: system });

    switch (this.format) {
      case 'qa':
        for (const ex of this.examples) {
          messages.push({ role: 'user', content: ex.input });
          messages.push({ role: 'assistant', content: ex.output });
        }
        messages.push({ role: 'user', content: query });
        break;

      case 'chat':
        messages.push({
          role: 'system',
          content: `Here are some examples:\n${this.examples.map(e =>
            `User: ${e.input}\nAssistant: ${e.output}`
          ).join('\n\n')}\n\nNow respond to: ${query}`,
        });
        break;

      case 'json':
        messages.push({
          role: 'user',
          content: `Examples:\n${this.examples.map(e =>
            JSON.stringify({ input: e.input, output: e.output })
          ).join('\n')}\n\nQuery: ${query}`,
        });
        break;
    }

    return messages;
  }
}

// ===== Prompt Parser =====

export interface ParsedPrompt {
  role: 'system' | 'user' | 'assistant';
  content: string;
  variables?: string[];
}

export class PromptParser {
  /** Parse a multi-role prompt string into messages. */
  parse(text: string): ParsedPrompt[] {
    const prompts: ParsedPrompt[] = [];
    const lines = text.split('\n');

    let currentRole: 'system' | 'user' | 'assistant' | null = null;
    let currentContent: string[] = [];

    for (const line of lines) {
      const match = line.match(/^@(system|user|assistant):\s*(.*)$/);
      if (match) {
        if (currentRole) {
          prompts.push({
            role: currentRole,
            content: currentContent.join('\n'),
            variables: this.extractVars(currentContent.join('\n')),
          });
        }
        currentRole = match[1] as 'system' | 'user' | 'assistant';
        currentContent = match[2] ? [match[2]] : [];
      } else {
        currentContent.push(line);
      }
    }

    if (currentRole) {
      prompts.push({
        role: currentRole,
        content: currentContent.join('\n'),
        variables: this.extractVars(currentContent.join('\n')),
      });
    }

    return prompts;
  }

  /** Parse variables from a template string. */
  extractVars(text: string): string[] {
    const matches = text.matchAll(/\{\{(\w+)\}\}/g);
    const vars = new Set<string>();
    for (const match of matches) vars.add(match[1]!);
    return Array.from(vars);
  }

  /** Fill template variables. */
  fillTemplate(template: string, vars: Record<string, string>): string {
    return template.replace(/\{\{(\w+)\}\}/g, (_, key: string) => vars[key] ?? '');
  }
}

// ===== Prompt Registry =====

export class PromptRegistry {
  private templates: Map<string, { template: string; description?: string; tags: string[] }> = new Map();

  register(name: string, template: string, opts?: { description?: string; tags?: string[] }): void {
    this.templates.set(name, {
      template,
      description: opts?.description,
      tags: opts?.tags ?? [],
    });
  }

  get(name: string): string | undefined {
    return this.templates.get(name)?.template;
  }

  list(): Array<{ name: string; description?: string; tags: string[] }> {
    return Array.from(this.templates.entries()).map(([name, val]) => ({
      name,
      description: val.description,
      tags: val.tags,
    }));
  }

  search(query: string): Array<{ name: string; template: string }> {
    const results: Array<{ name: string; template: string }> = [];
    for (const [name, val] of this.templates) {
      if (
        name.includes(query) ||
        val.description?.includes(query) ||
        val.tags.some(t => t.includes(query))
      ) {
        results.push({ name, template: val.template });
      }
    }
    return results;
  }

  delete(name: string): boolean {
    return this.templates.delete(name);
  }

  toMessages(name: string, vars: Record<string, string>): Message[] {
    const template = this.get(name);
    if (!template) throw new Error(`Template "${name}" not found`);

    const parser = new PromptParser();
    const parsed = parser.parse(template);
    return parsed.map(p => ({
      role: p.role,
      content: parser.fillTemplate(p.content, vars),
    }));
  }
}

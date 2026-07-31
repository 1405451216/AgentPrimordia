import type { AgentTemplate } from './types.js';
import { validateTemplate } from './validator.js';

/**
 * TemplateRegistry — Agent template registry with search, rating, and ranking.
 * Logic aligned with Go's TemplateRegistry in internal/agent/marketplace/template.go.
 */
export class TemplateRegistry {
  private templates = new Map<string, AgentTemplate>();
  private ratings = new Map<string, number[]>();

  /** Register a new template. Validates before storing. */
  register(tmpl: AgentTemplate): void {
    const vr = validateTemplate(tmpl);
    if (!vr.valid) {
      throw new Error(`marketplace: template validation failed: ${vr.errors?.join(', ')}`);
    }
    if (this.templates.has(tmpl.id)) {
      throw new Error(`marketplace: template "${tmpl.id}" already exists`);
    }
    const now = new Date().toISOString();
    tmpl.created_at = tmpl.created_at || now;
    tmpl.updated_at = now;
    this.templates.set(tmpl.id, { ...tmpl });
  }

  /** Update an existing template. Validates before storing. */
  update(tmpl: AgentTemplate): void {
    const vr = validateTemplate(tmpl);
    if (!vr.valid) {
      throw new Error(`marketplace: template validation failed: ${vr.errors?.join(', ')}`);
    }
    if (!this.templates.has(tmpl.id)) {
      throw new Error(`marketplace: template "${tmpl.id}" not found`);
    }
    tmpl.updated_at = new Date().toISOString();
    this.templates.set(tmpl.id, { ...tmpl });
  }

  /** Unregister (remove) a template by ID. */
  unregister(id: string): void {
    if (!this.templates.has(id)) {
      throw new Error(`marketplace: template "${id}" not found`);
    }
    this.templates.delete(id);
    this.ratings.delete(id);
  }

  /** Get a template by ID. Returns a copy or undefined. */
  get(id: string): AgentTemplate | undefined {
    const tmpl = this.templates.get(id);
    return tmpl ? { ...tmpl } : undefined;
  }

  /**
   * Search templates by keyword query, category, and tags.
   * Aligned with Go's Search(query, category, tags) logic.
   */
  search(query: string, category?: string, tags?: string[]): AgentTemplate[] {
    const results: AgentTemplate[] = [];
    const cat = category ?? '';
    const searchTags = tags ?? [];

    for (const tmpl of this.templates.values()) {
      // Category filter
      if (cat && tmpl.category !== cat) continue;

      // Tag filter (case-insensitive, any-match)
      if (searchTags.length > 0) {
        const matched = searchTags.some(tag =>
          (tmpl.tags ?? []).some(tTag => tTag.toLowerCase() === tag.toLowerCase()),
        );
        if (!matched) continue;
      }

      // Keyword search across name, description, system_prompt
      if (query) {
        const q = query.toLowerCase();
        const inName = tmpl.name.toLowerCase().includes(q);
        const inDesc = tmpl.description.toLowerCase().includes(q);
        const inPrompt = tmpl.system_prompt.toLowerCase().includes(q);
        if (!inName && !inDesc && !inPrompt) continue;
      }

      results.push({ ...tmpl });
    }
    return results;
  }

  /** List all templates. */
  list(): AgentTemplate[] {
    return Array.from(this.templates.values()).map(t => ({ ...t }));
  }

  /** Rate a template (0-5). Recalculates average rating. */
  rateTemplate(id: string, rating: number): void {
    if (rating < 0 || rating > 5) {
      throw new Error(`marketplace: rating must be 0-5, got ${rating}`);
    }
    const tmpl = this.templates.get(id);
    if (!tmpl) {
      throw new Error(`marketplace: template "${id}" not found`);
    }
    const list = this.ratings.get(id) ?? [];
    list.push(rating);
    this.ratings.set(id, list);
    const sum = list.reduce((a, b) => a + b, 0);
    tmpl.rating = sum / list.length;
    tmpl.updated_at = new Date().toISOString();
  }

  /** Increment download count for a template. */
  incrementDownloads(id: string): void {
    const tmpl = this.templates.get(id);
    if (tmpl) {
      tmpl.downloads++;
    }
  }

  /** Get top N templates sorted by downloads (descending). */
  topByDownloads(n: number): AgentTemplate[] {
    const all = this.list();
    all.sort((a, b) => b.downloads - a.downloads);
    return all.slice(0, n);
  }

  /** Get top N templates sorted by rating (descending). */
  topByRating(n: number): AgentTemplate[] {
    const all = this.list();
    all.sort((a, b) => b.rating - a.rating);
    return all.slice(0, n);
  }

  /** Get the number of registered templates. */
  get size(): number {
    return this.templates.size;
  }
}

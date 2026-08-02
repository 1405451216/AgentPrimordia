// Skills module — Skill evolution and lifecycle
// Mirrors Go internal/agent/skills/

// ===== Skill Model =====

export type SkillStatus = 'draft' | 'verified' | 'active' | 'deprecated';

export interface StepDef {
  id: string;
  description: string;
  toolName: string;
  inputMapping?: Record<string, string>;
  outputKey?: string;
  dependsOn?: string[];
}

export interface IOSchema {
  fields: Record<string, string>;
  required?: string[];
}

export interface Version {
  major: number;
  minor: number;
  patch: number;
}

export function versionString(v: Version): string {
  return `${v.major}.${v.minor}.${v.patch}`;
}

export function isCompatible(a: Version, b: Version): boolean {
  return a.major === b.major;
}

export interface Skill {
  id: string;
  name: string;
  description: string;
  version: Version;
  status: SkillStatus;
  steps: StepDef[];
  input: IOSchema;
  output: IOSchema;
  dependencies?: string[];
  tags?: string[];
  metadata?: Record<string, string>;
  createdAt: Date;
  updatedAt: Date;
  successRate: number;
  usageCount: number;
}

let skillCounter = 0;

export function createSkill(name: string, description: string, steps: StepDef[]): Skill {
  skillCounter++;
  return {
    id: `skill-${Date.now()}-${skillCounter}`,
    name,
    description,
    version: { major: 1, minor: 0, patch: 0 },
    status: 'draft',
    steps,
    input: { fields: {} },
    output: { fields: {} },
    tags: [],
    metadata: {},
    createdAt: new Date(),
    updatedAt: new Date(),
    successRate: 0,
    usageCount: 0,
  };
}

export function activateSkill(skill: Skill): void {
  skill.status = 'active';
  skill.updatedAt = new Date();
}

export function deprecateSkill(skill: Skill): void {
  skill.status = 'deprecated';
  skill.updatedAt = new Date();
}

export function recordUsage(skill: Skill, success: boolean): void {
  skill.usageCount++;
  const prevTotal = skill.successRate * (skill.usageCount - 1);
  skill.successRate = (prevTotal + (success ? 1 : 0)) / skill.usageCount;
  skill.updatedAt = new Date();
}

// ===== Skill Store =====

export class SkillStore {
  private skills = new Map<string, Skill>();

  save(skill: Skill): void {
    this.skills.set(skill.id, skill);
  }

  get(id: string): Skill | undefined {
    return this.skills.get(id);
  }

  delete(id: string): void {
    this.skills.delete(id);
  }

  list(): Skill[] {
    return [...this.skills.values()];
  }

  listActive(): Skill[] {
    return this.list().filter(s => s.status === 'active');
  }

  findByName(name: string): Skill | undefined {
    return this.list().find(s => s.name === name);
  }

  get count(): number {
    return this.skills.size;
  }
}

// ===== Skill Matcher =====

export type ConfidenceLevel = 'high' | 'medium' | 'low';

export interface MatchResult {
  skill: Skill;
  score: number;
  confidence: ConfidenceLevel;
}

export interface MatcherConfig {
  highThreshold?: number;
  mediumThreshold?: number;
}

export class SkillMatcher {
  private store: SkillStore;
  private highThreshold: number;
  private mediumThreshold: number;

  constructor(store: SkillStore, cfg: MatcherConfig = {}) {
    this.store = store;
    this.highThreshold = cfg.highThreshold ?? 0.8;
    this.mediumThreshold = cfg.mediumThreshold ?? 0.5;
  }

  match(taskDescription: string): MatchResult | null {
    const active = this.store.listActive();
    if (active.length === 0) return null;

    let best: MatchResult | null = null;
    for (const skill of active) {
      const score = this.score(skill, taskDescription);
      if (!best || score > best.score) {
        best = { skill, score, confidence: this.classify(score) };
      }
    }

    if (best && best.confidence === 'low') return null;
    return best;
  }

  private score(skill: Skill, task: string): number {
    let score = 0;
    let factors = 0;

    factors++;
    if (task.includes(skill.name)) score += 1.0;

    factors++;
    if (skill.description && task.includes(skill.description.slice(0, 10))) score += 0.5;

    factors++;
    const tagHits = (skill.tags ?? []).filter(t => task.includes(t)).length;
    if ((skill.tags ?? []).length > 0) score += tagHits / (skill.tags ?? []).length;

    return factors > 0 ? score / factors : 0;
  }

  private classify(score: number): ConfidenceLevel {
    if (score >= this.highThreshold) return 'high';
    if (score >= this.mediumThreshold) return 'medium';
    return 'low';
  }
}

// ===== Validator =====

export function validateSkill(skill: Skill): string[] {
  const errors: string[] = [];
  if (!skill.name) errors.push('技能名称不能为空');
  if (skill.steps.length === 0) errors.push('至少需要一个步骤');

  const ids = new Set<string>();
  for (const step of skill.steps) {
    if (!step.id) { errors.push('存在空步骤 ID'); continue; }
    if (ids.has(step.id)) errors.push(`步骤 ID 重复: ${step.id}`);
    ids.add(step.id);
  }

  for (const step of skill.steps) {
    for (const dep of step.dependsOn ?? []) {
      if (!ids.has(dep)) errors.push(`步骤 ${step.id} 依赖不存在的步骤 ${dep}`);
    }
  }

  return errors;
}

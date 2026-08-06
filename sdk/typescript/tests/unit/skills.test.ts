import { describe, it, expect } from 'vitest';
import {
  createSkill, activateSkill, deprecateSkill, recordUsage,
  SkillStore, SkillMatcher, validateSkill,
  isCompatible, versionString,
} from '../../src/skills/index.js';

describe('skills lifecycle (v3.4)', () => {
  it('creates skill in draft with v1.0.0', () => {
    const s = createSkill('data-fix', 'fix data', [{ id: 's1', toolName: 'query', description: 'q' }]);
    expect(s.status).toBe('draft');
    expect(versionString(s.version)).toBe('1.0.0');
    expect(s.steps).toHaveLength(1);
  });

  it('activates and deprecates', () => {
    const s = createSkill('x', 'd', [{ id: 's1', toolName: 't', description: 'd' }]);
    activateSkill(s);
    expect(s.status).toBe('active');
    deprecateSkill(s);
    expect(s.status).toBe('deprecated');
  });

  it('tracks success rate via recordUsage', () => {
    const s = createSkill('x', 'd', [{ id: 's1', toolName: 't', description: 'd' }]);
    recordUsage(s, true);
    recordUsage(s, true);
    recordUsage(s, false);
    expect(s.usageCount).toBe(3);
    expect(s.successRate).toBeCloseTo(2 / 3, 5);
  });

  it('version compatibility by major', () => {
    expect(isCompatible({ major: 1, minor: 0, patch: 0 }, { major: 1, minor: 9, patch: 0 })).toBe(true);
    expect(isCompatible({ major: 1, minor: 0, patch: 0 }, { major: 2, minor: 0, patch: 0 })).toBe(false);
  });
});

describe('SkillStore', () => {
  it('save/get/list/listActive', () => {
    const store = new SkillStore();
    const s = createSkill('a', 'd', [{ id: 's1', toolName: 't', description: 'd' }]);
    store.save(s);
    expect(store.count).toBe(1);
    expect(store.get(s.id)?.name).toBe('a');
    expect(store.listActive()).toHaveLength(0);
    activateSkill(s);
    store.save(s);
    expect(store.listActive()).toHaveLength(1);
    store.delete(s.id);
    expect(store.count).toBe(0);
  });
});

describe('SkillMatcher', () => {
  it('matches active skill by name/tag', () => {
    const store = new SkillStore();
    const s = createSkill('数据修复', '修复异常数据', [{ id: 's1', toolName: 't', description: 'd' }]);
    s.tags = ['数据', '修复'];
    activateSkill(s);
    store.save(s);
    const m = new SkillMatcher(store);
    const r = m.match('数据修复任务');
    expect(r).not.toBeNull();
    expect(r!.skill.id).toBe(s.id);
  });

  it('returns null when no active skills', () => {
    const store = new SkillStore();
    const m = new SkillMatcher(store);
    expect(m.match('anything')).toBeNull();
  });
});

describe('validateSkill', () => {
  it('rejects empty name and empty steps', () => {
    const bad = createSkill('', 'd', []);
    const errs = validateSkill(bad);
    expect(errs.length).toBeGreaterThan(0);
  });

  it('passes valid skill', () => {
    const ok = createSkill('n', 'd', [{ id: 's1', toolName: 't', description: 'd' }]);
    expect(validateSkill(ok)).toHaveLength(0);
  });
});

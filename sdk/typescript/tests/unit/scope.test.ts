import { describe, it, expect } from 'vitest';
import { FileScopePolicy } from '../../src/tools/scope.js';

describe('FileScopePolicy', () => {
  it('should set and get scope', () => {
    const policy = new FileScopePolicy();
    policy.setScope('agent-1', ['/path/to/dir']);
    expect(policy.getScope('agent-1')).toEqual(['/path/to/dir']);
  });

  it('should return undefined for unset scope', () => {
    const policy = new FileScopePolicy();
    expect(policy.getScope('unknown')).toBeUndefined();
  });

  it('should remove scope', () => {
    const policy = new FileScopePolicy();
    policy.setScope('agent-1', ['/path']);
    policy.removeScope('agent-1');
    expect(policy.getScope('agent-1')).toBeUndefined();
  });

  it('should deny access when no scope set', () => {
    const policy = new FileScopePolicy();
    expect(policy.allow('agent-1', '/path/to/file')).toBe(false);
  });

  it('should allow all when scope is empty array (global)', () => {
    const policy = new FileScopePolicy();
    policy.setScope('agent-1', []);
    expect(policy.allow('agent-1', '/any/path')).toBe(true);
    expect(policy.allow('agent-1', '/other/path')).toBe(true);
  });

  it('should allow access to files within scope', () => {
    const policy = new FileScopePolicy();
    policy.setScope('agent-1', ['/home/user/project']);
    expect(policy.allow('agent-1', '/home/user/project/file.ts')).toBe(true);
    expect(policy.allow('agent-1', '/home/user/project/sub/dir/file.ts')).toBe(true);
  });

  it('should deny access outside scope', () => {
    const policy = new FileScopePolicy();
    policy.setScope('agent-1', ['/home/user/project']);
    expect(policy.allow('agent-1', '/home/user/other/file.ts')).toBe(false);
    expect(policy.allow('agent-1', '/etc/passwd')).toBe(false);
  });

  it('should handle backslash paths (Windows)', () => {
    const policy = new FileScopePolicy();
    policy.setScope('agent-1', ['C:\\Users\\project']);
    expect(policy.allow('agent-1', 'C:/Users/project/file.ts')).toBe(true);
    expect(policy.allow('agent-1', 'C:\\Users\\project\\file.ts')).toBe(true);
  });

  it('should handle multiple scope paths', () => {
    const policy = new FileScopePolicy();
    policy.setScope('agent-1', ['/dir1', '/dir2']);
    expect(policy.allow('agent-1', '/dir1/file.ts')).toBe(true);
    expect(policy.allow('agent-1', '/dir2/file.ts')).toBe(true);
    expect(policy.allow('agent-1', '/dir3/file.ts')).toBe(false);
  });

  it('should normalize paths with trailing slashes', () => {
    const policy = new FileScopePolicy();
    policy.setScope('agent-1', ['/home/project/']);
    expect(policy.allow('agent-1', '/home/project/file.ts')).toBe(true);
  });

  it('should normalize paths with double slashes', () => {
    const policy = new FileScopePolicy();
    policy.setScope('agent-1', ['/home//project']);
    expect(policy.allow('agent-1', '/home/project/file.ts')).toBe(true);
  });

  describe('validate', () => {
    it('should return null for valid non-overlapping scopes', () => {
      const policy = new FileScopePolicy();
      const scopes = new Map([
        ['agent-1', ['/dir1']],
        ['agent-2', ['/dir2']],
      ]);
      expect(policy.validate(scopes)).toBeNull();
    });

    it('should return null for single global scope', () => {
      const policy = new FileScopePolicy();
      const scopes = new Map([
        ['agent-1', []],
        ['agent-2', ['/dir1']],
      ]);
      expect(policy.validate(scopes)).toBeNull();
    });

    it('should return error for multiple global scopes', () => {
      const policy = new FileScopePolicy();
      const scopes = new Map([
        ['agent-1', []],
        ['agent-2', []],
      ]);
      const err = policy.validate(scopes);
      expect(err).not.toBeNull();
      expect(err!.message).toContain('全局写权限');
    });

    it('should return error for overlapping scopes', () => {
      const policy = new FileScopePolicy();
      const scopes = new Map([
        ['agent-1', ['/home/project']],
        ['agent-2', ['/home/project/sub']],
      ]);
      const err = policy.validate(scopes);
      expect(err).not.toBeNull();
      expect(err!.message).toContain('重叠');
    });

    it('should return error for identical scopes', () => {
      const policy = new FileScopePolicy();
      const scopes = new Map([
        ['agent-1', ['/same/path']],
        ['agent-2', ['/same/path']],
      ]);
      const err = policy.validate(scopes);
      expect(err).not.toBeNull();
    });

    it('should handle empty scopes map', () => {
      const policy = new FileScopePolicy();
      const scopes = new Map();
      expect(policy.validate(scopes)).toBeNull();
    });
  });
});

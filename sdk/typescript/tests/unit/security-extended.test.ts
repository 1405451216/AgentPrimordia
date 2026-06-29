import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import * as fs from 'node:fs';
import * as path from 'node:path';
import * as os from 'node:os';
import {
  containsShellMetacharacter,
  validatePathTraversal,
  resolvePathSafe,
  InputSanitizer,
  CommandGuard,
} from '../../src/security/extended.js';

// ===== containsShellMetacharacter tests =====
describe('containsShellMetacharacter', () => {
  it('should detect semicolon', () => {
    const result = containsShellMetacharacter('echo hello; rm -rf /');
    expect(result.found).toBe(true);
    expect(result.char).toBe(';');
  });

  it('should detect pipe', () => {
    const result = containsShellMetacharacter('echo hello | cat');
    expect(result.found).toBe(true);
    expect(result.char).toBe('|');
  });

  it('should detect ampersand', () => {
    const result = containsShellMetacharacter('echo hello & cat');
    expect(result.found).toBe(true);
  });

  it('should detect dollar sign', () => {
    const result = containsShellMetacharacter('echo $HOME');
    expect(result.found).toBe(true);
    expect(result.char).toBe('$');
  });

  it('should detect backtick', () => {
    const result = containsShellMetacharacter('echo `whoami`');
    expect(result.found).toBe(true);
    expect(result.char).toBe('`');
  });

  it('should detect redirect >', () => {
    const result = containsShellMetacharacter('echo > file.txt');
    expect(result.found).toBe(true);
  });

  it('should detect redirect <', () => {
    const result = containsShellMetacharacter('cat < file.txt');
    expect(result.found).toBe(true);
  });

  it('should detect newline', () => {
    const result = containsShellMetacharacter('echo hello\nrm -rf /');
    expect(result.found).toBe(true);
  });

  it('should detect carriage return', () => {
    const result = containsShellMetacharacter('echo hello\rrm -rf /');
    expect(result.found).toBe(true);
  });

  it('should detect parentheses', () => {
    const result = containsShellMetacharacter('echo (test)');
    expect(result.found).toBe(true);
  });

  it('should return false for safe command', () => {
    const result = containsShellMetacharacter('echo hello world');
    expect(result.found).toBe(false);
    expect(result.char).toBeUndefined();
  });
});

// ===== validatePathTraversal tests =====
describe('validatePathTraversal', () => {
  it('should detect .. in path', () => {
    const result = validatePathTraversal('../../etc/passwd');
    expect(result.safe).toBe(false);
    expect(result.reason).toContain('Path traversal');
  });

  it('should detect null bytes', () => {
    const result = validatePathTraversal('file\0.txt');
    expect(result.safe).toBe(false);
    expect(result.reason).toContain('Null byte');
  });

  it('should detect URL-encoded traversal', () => {
    const result = validatePathTraversal('%2e%2e%2fetc');
    expect(result.safe).toBe(false);
    expect(result.reason).toContain('URL-encoded');
  });

  it('should pass safe paths', () => {
    const result = validatePathTraversal('safe/path/file.txt');
    expect(result.safe).toBe(true);
    expect(result.reason).toBeUndefined();
  });

  it('should pass simple filename', () => {
    const result = validatePathTraversal('file.txt');
    expect(result.safe).toBe(true);
  });
});

// ===== resolvePathSafe tests =====
describe('resolvePathSafe', () => {
  let tmpDir: string;

  beforeEach(() => {
    tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ap-sec-'));
  });

  afterEach(() => {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  });

  it('should resolve safe path within root', () => {
    const result = resolvePathSafe(tmpDir, 'file.txt');
    expect(result.safe).toBe(true);
    expect(result.resolved).toContain('file.txt');
  });

  it('should detect path escaping root', () => {
    const result = resolvePathSafe(tmpDir, '../../etc/passwd');
    expect(result.safe).toBe(false);
    expect(result.reason).toContain('escapes root');
  });

  it('should allow root itself', () => {
    const result = resolvePathSafe(tmpDir, '.');
    expect(result.safe).toBe(true);
  });

  it('should detect symlink escape', () => {
    // Create a symlink to a directory outside root
    const outsideDir = path.join(os.tmpdir(), 'ap-outside');
    fs.mkdirSync(outsideDir, { recursive: true });
    try {
      const linkPath = path.join(tmpDir, 'escape-link');
      fs.symlinkSync(outsideDir, linkPath);
      const result = resolvePathSafe(tmpDir, 'escape-link');
      expect(result.safe).toBe(false);
      expect(result.reason).toContain('Symlink escape');
    } finally {
      fs.rmSync(outsideDir, { recursive: true, force: true });
    }
  });

  it('should allow symlink within root', () => {
    const targetDir = path.join(tmpDir, 'target');
    fs.mkdirSync(targetDir);
    const linkPath = path.join(tmpDir, 'safe-link');
    fs.symlinkSync(targetDir, linkPath);
    const result = resolvePathSafe(tmpDir, 'safe-link');
    expect(result.safe).toBe(true);
  });
});

// ===== InputSanitizer tests =====
describe('InputSanitizer', () => {
  it('should pass safe input', () => {
    const sanitizer = new InputSanitizer();
    const result = sanitizer.sanitize('Hello World');
    expect(result.safe).toBe(true);
    expect(result.sanitized).toBe('Hello World');
    expect(result.issues).toHaveLength(0);
  });

  it('should reject input exceeding max length', () => {
    const sanitizer = new InputSanitizer({ maxLength: 10 });
    const result = sanitizer.sanitize('a'.repeat(20));
    expect(result.safe).toBe(false);
    expect(result.issues[0]).toContain('maximum length');
  });

  it('should remove null bytes', () => {
    const sanitizer = new InputSanitizer();
    const result = sanitizer.sanitize('hello\0world');
    expect(result.safe).toBe(true);
    expect(result.sanitized).toBe('helloworld');
    expect(result.issues).toContain('Removed null bytes');
  });

  it('should normalize unicode', () => {
    const sanitizer = new InputSanitizer();
    const result = sanitizer.sanitize('café');
    expect(result.safe).toBe(true);
    expect(result.sanitized).toBe('café'.normalize('NFC'));
  });

  it('should block patterns', () => {
    const sanitizer = new InputSanitizer({
      blockedPatterns: [/eval\s*\(/],
    });
    const result = sanitizer.sanitize('eval(malicious)');
    expect(result.safe).toBe(false);
    expect(result.issues[0]).toContain('Blocked pattern');
  });

  it('should use default max length of 100000', () => {
    const sanitizer = new InputSanitizer();
    const result = sanitizer.sanitize('a'.repeat(100));
    expect(result.safe).toBe(true);
  });

  it('should handle empty input', () => {
    const sanitizer = new InputSanitizer();
    const result = sanitizer.sanitize('');
    expect(result.safe).toBe(true);
    expect(result.sanitized).toBe('');
  });

  it('should allow custom blocked patterns', () => {
    const sanitizer = new InputSanitizer({
      blockedPatterns: [/password/i],
    });
    expect(sanitizer.sanitize('enter password').safe).toBe(false);
    expect(sanitizer.sanitize('safe text').safe).toBe(true);
  });
});

// ===== CommandGuard tests =====
describe('CommandGuard', () => {
  it('should allow safe commands', () => {
    const guard = new CommandGuard();
    const result = guard.check('echo hello');
    expect(result.allowed).toBe(true);
  });

  it('should reject empty command', () => {
    const guard = new CommandGuard();
    const result = guard.check('');
    expect(result.allowed).toBe(false);
    expect(result.reason).toContain('Empty');
  });

  it('should reject whitespace-only command', () => {
    const guard = new CommandGuard();
    const result = guard.check('   ');
    expect(result.allowed).toBe(false);
  });

  it('should reject shell metacharacters', () => {
    const guard = new CommandGuard();
    const result = guard.check('echo hello; rm -rf /');
    expect(result.allowed).toBe(false);
    expect(result.reason).toContain('metacharacter');
  });

  it('should reject blacklisted commands', () => {
    const guard = new CommandGuard();
    expect(guard.check('rm file').allowed).toBe(false);
    expect(guard.check('sudo command').allowed).toBe(false);
    expect(guard.check('kill -9 1234').allowed).toBe(false);
  });

  it('should reject blacklisted arguments', () => {
    const guard = new CommandGuard();
    const result = guard.check('command -rf');
    expect(result.allowed).toBe(false);
    expect(result.reason).toContain('Argument');
  });

  it('should reject --force argument', () => {
    const guard = new CommandGuard();
    const result = guard.check('command --force');
    expect(result.allowed).toBe(false);
  });

  it('should enforce whitelist', () => {
    const guard = new CommandGuard({ whitelist: ['echo', 'ls'] });
    expect(guard.check('echo hello').allowed).toBe(true);
    expect(guard.check('cat file').allowed).toBe(false);
    expect(guard.check('cat file').reason).toContain('whitelist');
  });

  it('should use custom blacklist', () => {
    const guard = new CommandGuard({ blacklist: ['custombad'] });
    expect(guard.check('custombad').allowed).toBe(false);
    expect(guard.check('echo hello').allowed).toBe(true);
  });

  it('should use custom argBlacklist', () => {
    const guard = new CommandGuard({ argBlacklist: ['--danger'] });
    expect(guard.check('command --danger').allowed).toBe(false);
    expect(guard.check('command --safe').allowed).toBe(true);
  });

  it('should addToWhitelist', () => {
    const guard = new CommandGuard({ whitelist: ['echo'] });
    guard.addToWhitelist('ls');
    expect(guard.check('ls').allowed).toBe(true);
  });

  it('should addToBlacklist', () => {
    const guard = new CommandGuard();
    guard.addToBlacklist('customcmd');
    expect(guard.check('customcmd').allowed).toBe(false);
  });

  it('should use default blacklist when not specified', () => {
    const guard = new CommandGuard();
    expect(guard.check('rm').allowed).toBe(false);
    expect(guard.check('del').allowed).toBe(false);
    expect(guard.check('format').allowed).toBe(false);
    expect(guard.check('chmod').allowed).toBe(false);
  });

  it('should allow whitelisted command that is also blacklisted', () => {
    const guard = new CommandGuard({ whitelist: ['rm'] });
    // Whitelist check passes, but blacklist check should still block
    const result = guard.check('rm file');
    expect(result.allowed).toBe(false);
  });
});

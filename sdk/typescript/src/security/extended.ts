// ===== Extended Security: Shell Metacharacter Detection, Symlink Protection =====

import * as path from 'node:path';
import * as fs from 'node:fs';

const DANGEROUS_CHARS = [';', '|', '&', '$', '`', '>', '<', '\n', '\r', '(', ')'];

/** Check if a command string contains shell metacharacters. */
export function containsShellMetacharacter(cmd: string): { found: boolean; char?: string } {
  for (const ch of DANGEROUS_CHARS) {
    if (cmd.includes(ch)) {
      return { found: true, char: ch };
    }
  }
  return { found: false };
}

/** Validate a path for traversal attacks. */
export function validatePathTraversal(path: string): { safe: boolean; reason?: string } {
  // Check for path traversal patterns
  if (path.includes('..')) {
    return { safe: false, reason: 'Path traversal detected: ".." in path' };
  }

  // Check for null bytes
  if (path.includes('\0')) {
    return { safe: false, reason: 'Null byte in path' };
  }

  // Check for URL-encoded traversal
  const decoded = decodeURIComponent(path);
  if (decoded.includes('..')) {
    return { safe: false, reason: 'URL-encoded path traversal detected' };
  }

  return { safe: true };
}

/** Resolve a path and check it stays within the allowed root. */
export function resolvePathSafe(rootDir: string, filePath: string): { safe: boolean; resolved?: string; reason?: string } {
  const root = path.resolve(rootDir);
  const resolved = path.resolve(root, filePath);

  // Check the resolved path is within root
  if (!resolved.startsWith(root + path.sep) && resolved !== root) {
    return { safe: false, reason: `Path "${filePath}" escapes root directory` };
  }

  // Check for symlink escape
  try {
    // Check each component of the path for symlinks
    const parts = resolved.slice(root.length).split(path.sep).filter(Boolean);
    let currentPath = root;

    for (const part of parts) {
      currentPath = path.join(currentPath, part);

      try {
        const stat = fs.lstatSync(currentPath);
        if (stat.isSymbolicLink()) {
          const target = fs.readlinkSync(currentPath);
          const resolvedTarget = path.resolve(path.dirname(currentPath), target);

          // Check if symlink target is within root
          if (!resolvedTarget.startsWith(root + path.sep) && resolvedTarget !== root) {
            return { safe: false, reason: `Symlink escape detected: ${currentPath} -> ${resolvedTarget}` };
          }
        }
      } catch {
        // Path doesn't exist yet, that's OK
      }
    }
  } catch {
    // If we can't check symlinks, be cautious but don't block
  }

  return { safe: true, resolved };
}

// ===== Sanitizer =====

export class InputSanitizer {
  private maxLength: number;
  private blockedPatterns: RegExp[];

  constructor(opts?: { maxLength?: number; blockedPatterns?: RegExp[] }) {
    this.maxLength = opts?.maxLength ?? 100_000;
    this.blockedPatterns = opts?.blockedPatterns ?? [];
  }

  sanitize(input: string): { safe: boolean; sanitized: string; issues: string[] } {
    const issues: string[] = [];

    // Check length
    if (input.length > this.maxLength) {
      return {
        safe: false,
        sanitized: '',
        issues: [`Input exceeds maximum length of ${this.maxLength} characters`],
      };
    }

    let sanitized = input;

    // Remove null bytes
    if (sanitized.includes('\0')) {
      sanitized = sanitized.replace(/\0/g, '');
      issues.push('Removed null bytes');
    }

    // Normalize unicode (basic)
    sanitized = sanitized.normalize('NFC');

    // Check blocked patterns
    for (const pattern of this.blockedPatterns) {
      if (pattern.test(sanitized)) {
        issues.push(`Blocked pattern detected: ${pattern.source}`);
        return { safe: false, sanitized: '', issues };
      }
    }

    return { safe: true, sanitized, issues };
  }
}

// ===== Command Whitelist/Blacklist =====

export class CommandGuard {
  private whitelist: Set<string>;
  private blacklist: Set<string>;
  private argBlacklist: string[];

  constructor(opts?: { whitelist?: string[]; blacklist?: string[]; argBlacklist?: string[] }) {
    this.whitelist = new Set(opts?.whitelist ?? []);
    this.blacklist = new Set(opts?.blacklist ?? [
      'rm', 'rmdir', 'del', 'format', 'mkfs', 'dd', 'shutdown', 'reboot',
      'kill', 'killall', 'pkill', 'sudo', 'su', 'chmod', 'chown',
    ]);
    this.argBlacklist = opts?.argBlacklist ?? [
      '-rf', '--force', '-r', '-f', '/dev/', '/etc/', '/proc/', '/sys/',
    ];
  }

  check(command: string): { allowed: boolean; reason?: string } {
    const trimmed = command.trim();
    if (!trimmed) return { allowed: false, reason: 'Empty command' };

    // Check for metacharacters
    const metaCheck = containsShellMetacharacter(trimmed);
    if (metaCheck.found) {
      return { allowed: false, reason: `Shell metacharacter "${metaCheck.char}" not allowed` };
    }

    // Extract base command
    const parts = trimmed.split(/\s+/);
    const baseCmd = parts[0];

    // Check whitelist
    if (this.whitelist.size > 0 && !this.whitelist.has(baseCmd)) {
      return { allowed: false, reason: `Command "${baseCmd}" not in whitelist` };
    }

    // Check blacklist
    if (this.blacklist.has(baseCmd)) {
      return { allowed: false, reason: `Command "${baseCmd}" is blacklisted` };
    }

    // Check argument blacklist
    for (const arg of parts.slice(1)) {
      for (const blocked of this.argBlacklist) {
        if (arg === blocked || arg.startsWith(blocked)) {
          return { allowed: false, reason: `Argument "${arg}" is blacklisted` };
        }
      }
    }

    return { allowed: true };
  }

  addToWhitelist(cmd: string): void { this.whitelist.add(cmd); }
  addToBlacklist(cmd: string): void { this.blacklist.add(cmd); }
}

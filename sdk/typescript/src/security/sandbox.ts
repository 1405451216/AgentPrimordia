import { containsShellMetacharacter, validatePathTraversal } from './extended.js';

export type AccessLevel = 'none' | 'read' | 'write' | 'execute' | 'all';

const ACCESS_LEVELS: Record<AccessLevel, number> = {
  none: 0,
  read: 1,
  write: 2,
  execute: 4,
  all: 7,
};

/** 命令参数白名单模式 */
export interface ArgPattern {
  /** 编译后的正则表达式 */
  regex: RegExp;
  /** 不匹配时的提示信息 */
  message: string;
}

/** 创建一个参数模式 */
export function newArgPattern(regex: string, message: string): ArgPattern {
  return { regex: new RegExp(regex), message };
}

export class ACL {
  private rules: { agentID: string; resource: string; level: number }[] = [];
  private denyRules: { agentID: string; resource: string }[] = [];

  allow(agentID: string, resource: string, level: AccessLevel): void {
    this.rules.push({ agentID, resource, level: ACCESS_LEVELS[level] });
  }

  deny(agentID: string, resource: string): void {
    this.denyRules.push({ agentID, resource });
  }

  check(agentID: string, resource: string, required: AccessLevel): boolean {
    for (const rule of this.denyRules) {
      if (matchRule(rule.agentID, rule.resource, agentID, resource)) return false;
    }
    for (const rule of this.rules) {
      if (matchRule(rule.agentID, rule.resource, agentID, resource)) {
        return (rule.level & ACCESS_LEVELS[required]) === ACCESS_LEVELS[required];
      }
    }
    return false;
  }

  reset(): void {
    this.rules = [];
    this.denyRules = [];
  }
}

function matchRule(ruleAgent: string, ruleResource: string, agentID: string, resource: string): boolean {
  if (ruleAgent !== '*' && ruleAgent !== agentID) return false;
  const cleanResource = resource.replace(/\\/g, '/').replace(/\/+/g, '/');
  const cleanRule = ruleResource.replace(/\\/g, '/').replace(/\/+$/, '');
  if (cleanResource === cleanRule) return true;
  return cleanResource.startsWith(cleanRule + '/');
}

export class Sandbox {
  private acl: ACL;
  private allowedCmds: Set<string> = new Set();
  private blockedCmds: Set<string> = new Set();
  /** 命令参数白名单模式：命令名 → 参数模式列表 */
  private argPatterns: Map<string, ArgPattern[]> = new Map();

  constructor(acl: ACL) {
    this.acl = acl;
  }

  allowCommand(cmd: string): void {
    this.allowedCmds.add(cmd);
    this.blockedCmds.delete(cmd);
  }

  blockCommand(cmd: string): void {
    this.blockedCmds.add(cmd);
    this.allowedCmds.delete(cmd);
  }

  /** 允许命令并指定参数白名单模式 */
  allowCommandWithArgs(cmd: string, ...patterns: ArgPattern[]): void {
    this.allowedCmds.add(cmd);
    this.blockedCmds.delete(cmd);
    if (patterns.length > 0) {
      this.argPatterns.set(cmd, patterns);
    }
  }

  /** 为已允许的命令设置参数白名单模式 */
  setArgPatterns(cmd: string, ...patterns: ArgPattern[]): void {
    if (patterns.length > 0) {
      this.argPatterns.set(cmd, patterns);
    } else {
      this.argPatterns.delete(cmd);
    }
  }

  /**
   * 验证命令参数：
   * 1. 检查参数中是否包含路径遍历
   * 2. 检查参数中是否包含 shell 元字符
   * 3. 检查参数是否匹配白名单模式（若配置了）
   */
  private validateArgs(cmdName: string, args: string[]): Error | null {
    for (const arg of args) {
      // 跳过选项标志（如 -l, --verbose）
      if (arg.startsWith('-')) continue;

      // 检查路径遍历
      const traversal = validatePathTraversal(arg);
      if (!traversal.safe) {
        return new Error(`path traversal in argument "${arg}": ${traversal.reason}`);
      }

      // 检查 shell 元字符
      const meta = containsShellMetacharacter(arg);
      if (meta.found) {
        return new Error(`argument "${arg}" contains shell metacharacter '${meta.char}'`);
      }
    }

    // 检查参数白名单模式
    const patterns = this.argPatterns.get(cmdName);
    if (!patterns || patterns.length === 0) return null;

    // 将参数拼接为空格分隔的字符串进行模式匹配
    const argStr = args.join(' ');
    for (const p of patterns) {
      if (p.regex.test(argStr)) return null;
    }

    return new Error(`arguments "${argStr}" do not match allowed patterns for command "${cmdName}"`);
  }

  canExecute(agentID: string, cmd: string): Error | null {
    // 检查 shell 元字符，防止命令注入绕过
    const meta = containsShellMetacharacter(cmd);
    if (meta.found) {
      return new Error(`command contains shell metacharacter '${meta.char}'`);
    }

    // 提取命令名和参数
    const fields = cmd.trim().split(/\s+/);
    const cmdName = fields[0];
    if (!cmdName) return new Error('empty command');

    if (this.blockedCmds.has(cmdName)) {
      return new Error(`command "${cmdName}" is blocked`);
    }
    if (this.allowedCmds.size > 0 && !this.allowedCmds.has(cmdName)) {
      return new Error(`command "${cmdName}" is not in allowed list`);
    }

    // 验证命令参数
    const args = fields.slice(1);
    return this.validateArgs(cmdName, args);
  }

  canAccess(agentID: string, resource: string, level: AccessLevel): Error | null {
    if (!this.acl.check(agentID, resource, level)) {
      return new Error(`agent "${agentID}" denied ${level} access to "${resource}"`);
    }
    return null;
  }

  validatePath(agentID: string, path: string, level: AccessLevel): Error | null {
    let decoded = path;
    for (let i = 0; i < 10; i++) {
      try {
        const prev = decoded;
        decoded = decodeURIComponent(decoded);
        if (decoded === prev) break;
      } catch { break; }
    }
    if (decoded.includes('..') || decoded.includes('\0')) {
      return new Error(`invalid path: "${path}"`);
    }
    return this.canAccess(agentID, decoded, level);
  }
}

export type AccessLevel = 'none' | 'read' | 'write' | 'execute' | 'all';

const ACCESS_LEVELS: Record<AccessLevel, number> = {
  none: 0,
  read: 1,
  write: 2,
  execute: 4,
  all: 7,
};

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

  canExecute(agentID: string, cmd: string): Error | null {
    if (this.blockedCmds.has(cmd)) return new Error(`command "${cmd}" is blocked`);
    if (this.allowedCmds.size > 0 && !this.allowedCmds.has(cmd)) return new Error(`command "${cmd}" is not in allowed list`);
    return null;
  }

  canAccess(agentID: string, resource: string, level: AccessLevel): Error | null {
    if (!this.acl.check(agentID, resource, level)) {
      return new Error(`agent "${agentID}" denied ${level} access to "${resource}"`);
    }
    return null;
  }

  validatePath(agentID: string, path: string, level: AccessLevel): Error | null {
    if (path.includes('..')) return new Error(`path traversal detected: "${path}"`);
    return this.canAccess(agentID, path, level);
  }
}

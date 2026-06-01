export class FileScopePolicy {
  private agentScopes: Map<string, string[]> = new Map();

  setScope(agentID: string, paths: string[]): void {
    this.agentScopes.set(agentID, paths);
  }

  getScope(agentID: string): string[] | undefined {
    return this.agentScopes.get(agentID);
  }

  removeScope(agentID: string): void {
    this.agentScopes.delete(agentID);
  }

  allow(agentID: string, resource: string): boolean {
    const scope = this.agentScopes.get(agentID);
    if (!scope) return false;
    if (scope.length === 0) return true;

    const absPath = normalizePath(resource);
    for (const s of scope) {
      const scopeAbs = normalizePath(s);
      if (absPath === scopeAbs || absPath.startsWith(scopeAbs + '/')) {
        return true;
      }
    }
    return false;
  }

  validate(agentScopes: Map<string, string[]>): Error | null {
    let globalCount = 0;
    const scopes: string[][] = [];

    for (const [, scope] of agentScopes) {
      if (scope.length === 0) globalCount++;
      scopes.push(scope);
    }

    if (globalCount > 1) {
      return new Error(`同一批次中最多允许 1 个任务拥有全局写权限，当前有 ${globalCount} 个`);
    }

    for (let i = 0; i < scopes.length; i++) {
      if (scopes[i].length === 0) continue;
      for (let j = i + 1; j < scopes.length; j++) {
        if (scopes[j].length === 0) continue;
        const overlap = findScopeOverlap(scopes[i], scopes[j]);
        if (overlap) {
          return new Error(`任务 ${i + 1} 和任务 ${j + 1} 的 scope 存在重叠: ${overlap}`);
        }
      }
    }
    return null;
  }
}

function normalizePath(p: string): string {
  return p.replace(/\\/g, '/').replace(/\/+/g, '/').replace(/\/$/, '');
}

function findScopeOverlap(scopeA: string[], scopeB: string[]): string | null {
  for (const a of scopeA) {
    const cleanA = normalizePath(a);
    for (const b of scopeB) {
      const cleanB = normalizePath(b);
      if (cleanA === cleanB) return cleanA;
      if (cleanA.startsWith(cleanB + '/')) return `${cleanA} (属于 ${cleanB})`;
      if (cleanB.startsWith(cleanA + '/')) return `${cleanB} (属于 ${cleanA})`;
    }
  }
  return null;
}

import type { Tool } from '../types.js';

// ===== Tool Scope Policy =====

export interface ScopeRule {
  agentID: string;
  paths: string[];
  commands?: string[];
}

export class FileScopePolicy {
  private rules: Map<string, string[]> = new Map();
  private commandRules: Map<string, string[]> = new Map();

  allow(agentID: string, paths: string[]): void {
    this.rules.set(agentID, paths);
  }

  allowCommands(agentID: string, commands: string[]): void {
    this.commandRules.set(agentID, commands);
  }

  checkPath(agentID: string, path: string): boolean {
    const allowed = this.rules.get(agentID);
    if (!allowed || allowed.length === 0) return true; // No restrictions
    for (const allowedPath of allowed) {
      if (path.startsWith(allowedPath) || path === allowedPath) return true;
    }
    return false;
  }

  checkCommand(agentID: string, command: string): boolean {
    const allowed = this.commandRules.get(agentID);
    if (!allowed || allowed.length === 0) return true;
    const cmdBase = command.trim().split(/\s+/)[0];
    return allowed.includes(cmdBase);
  }

  getRules(agentID: string): { paths: string[]; commands: string[] } | undefined {
    return {
      paths: this.rules.get(agentID) ?? [],
      commands: this.commandRules.get(agentID) ?? [],
    };
  }
}

// ===== Tool Permission =====

export interface PermissionRequest {
  toolName: string;
  agentID: string;
  args: Record<string, unknown>;
}

export interface PermissionResult {
  allowed: boolean;
  reason?: string;
  modifiedArgs?: Record<string, unknown>;
}

export type PermissionHandler = (req: PermissionRequest) => Promise<PermissionResult>;

export class ToolPermission {
  private handler?: PermissionHandler;
  private requireConfirmation: Set<string> = new Set();

  requireConfirm(toolName: string): void {
    this.requireConfirmation.add(toolName);
  }

  setHandler(handler: PermissionHandler): void {
    this.handler = handler;
  }

  async check(req: PermissionRequest): Promise<PermissionResult> {
    if (!this.requireConfirmation.has(req.toolName)) {
      return { allowed: true };
    }
    if (!this.handler) {
      return { allowed: true };
    }
    return this.handler(req);
  }
}

// ===== Tool Executor with timeout and scope =====

export class ScopedExecutor {
  private scope: FileScopePolicy;
  private timeoutMs: number;

  constructor(scope: FileScopePolicy, timeoutMs: number = 30_000) {
    this.scope = scope;
    this.timeoutMs = timeoutMs;
  }

  async execute(
    tool: Tool,
    args: Record<string, unknown>,
    agentID: string,
    opts?: { timeoutMs?: number }
  ): Promise<{ content: string; isError: boolean }> {
    const timeout = opts?.timeoutMs ?? this.timeoutMs;

    // Check file scope
    if (args.path && typeof args.path === 'string') {
      if (!this.scope.checkPath(agentID, args.path)) {
        return { content: `Error: path "${args.path}" is outside allowed scope for agent "${agentID}"`, isError: true };
      }
    }

    // Check command scope
    if (args.command && typeof args.command === 'string') {
      if (!this.scope.checkCommand(agentID, args.command)) {
        return { content: `Error: command not allowed for agent "${agentID}"`, isError: true };
      }
    }

    // Execute with timeout
    try {
      const result = await Promise.race([
        tool.execute(args),
        new Promise<never>((_, reject) =>
          setTimeout(() => reject(new Error(`Tool "${tool.name}" timed out after ${timeout}ms`)), timeout)
        ),
      ]);
      return { content: result, isError: false };
    } catch (err) {
      return { content: `Error: ${(err as Error).message}`, isError: true };
    }
  }
}

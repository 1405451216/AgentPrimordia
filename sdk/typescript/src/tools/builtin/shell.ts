import { exec } from 'node:child_process';
import * as path from 'node:path';
import type { Tool } from '../../types.js';

export interface ShellConfig {
  commandWhitelist?: string[];
  commandBlacklist?: string[];
  timeoutMs?: number;
  maxOutputLength?: number;
  workingDir?: string;
}

/**
 * Shell tool — execute shell commands with safety controls.
 */
export class ShellTool implements Tool {
  name = 'shell';
  description = 'Execute shell commands with whitelist/blacklist safety controls';
  parameters = {
    type: 'object',
    properties: {
      command: { type: 'string', description: 'The shell command to execute' },
      cwd: { type: 'string', description: 'Working directory (optional)' },
      timeout: { type: 'number', description: 'Timeout in seconds (optional, default 30)' },
    },
    required: ['command'],
  };

  private config: Required<ShellConfig>;

  constructor(config?: ShellConfig) {
    this.config = {
      commandWhitelist: config?.commandWhitelist ?? [],
      commandBlacklist: config?.commandBlacklist ?? ['rm -rf /', 'mkfs', 'dd if=', 'shutdown', 'reboot'],
      timeoutMs: config?.timeoutMs ?? 30_000,
      maxOutputLength: config?.maxOutputLength ?? 10_000,
      workingDir: config?.workingDir ?? process.cwd(),
    };
  }

  async execute(args: Record<string, unknown>): Promise<string> {
    const command = args.command as string;
    if (!command?.trim()) return 'Error: command is required';

    // Check blacklist
    for (const blocked of this.config.commandBlacklist) {
      if (command.includes(blocked)) {
        return `Error: command contains blocked pattern: ${blocked}`;
      }
    }

    // Check whitelist (if configured)
    if (this.config.commandWhitelist.length > 0) {
      const cmdBase = command.trim().split(/\s+/)[0];
      if (!this.config.commandWhitelist.includes(cmdBase)) {
        return `Error: command "${cmdBase}" is not in the allowed list`;
      }
    }

    // Check for dangerous metacharacters
    const dangerousChars = [';', '|', '&', '$', '`', '>', '<', '\n', '\r'];
    for (const ch of dangerousChars) {
      if (command.includes(ch)) {
        return `Error: command contains shell metacharacter: "${ch}"`;
      }
    }

    const cwd = (args.cwd as string) || this.config.workingDir;
    const timeout = ((args.timeout as number) ?? 30) * 1000;

    return new Promise((resolve) => {
      exec(command, {
        cwd: path.resolve(cwd),
        timeout: Math.min(timeout, this.config.timeoutMs),
        maxBuffer: 1024 * 1024,
      }, (err, stdout, stderr) => {
        if (err) {
          resolve(`Error: ${err.message}\n${stderr || ''}`);
          return;
        }
        let output = stdout;
        if (stderr) output += `\nSTDERR: ${stderr}`;
        if (output.length > this.config.maxOutputLength) {
          output = output.slice(0, this.config.maxOutputLength) + '\n... (truncated)';
        }
        resolve(output || '(no output)');
      });
    });
  }
}

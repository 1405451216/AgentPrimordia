import type { Response } from '../types.js';
import { ReActAgent } from '../agent/react-loop.js';
import type { ReActConfig } from '../agent/react-loop.js';
import type { FileScopePolicy } from '../tools/scope.js';

export interface PoolTask {
  id: string;
  input: string;
  agentConfig: ReActConfig;
  scope?: string[];
  timeoutMs?: number;
}

export interface PoolResult {
  taskID: string;
  response: Response;
  error?: Error;
}

export class AgentPool {
  private maxConcurrent: number;
  private scopePolicy?: FileScopePolicy;
  private defaultTimeoutMs: number;

  constructor(opts: { maxConcurrent?: number; scopePolicy?: FileScopePolicy; defaultTimeoutMs?: number } = {}) {
    this.maxConcurrent = opts.maxConcurrent ?? 5;
    this.scopePolicy = opts.scopePolicy;
    this.defaultTimeoutMs = opts.defaultTimeoutMs ?? 120_000;
  }

  async dispatch(tasks: PoolTask[]): Promise<PoolResult[]> {
    const results: PoolResult[] = [];
    let nextIndex = 0;

    const worker = async (): Promise<void> => {
      while (nextIndex < tasks.length) {
        const currentIndex = nextIndex++;
        const task = tasks[currentIndex];
        if (!task) break;

        const timeoutMs = task.timeoutMs ?? this.defaultTimeoutMs;
        try {
          const agent = new ReActAgent(task.agentConfig);
          const response = await Promise.race([
            agent.run(task.input),
            new Promise<never>((_, reject) =>
              setTimeout(() => reject(new Error(`Task ${task.id} timed out after ${timeoutMs}ms`)), timeoutMs)
            ),
          ]);
          results.push({ taskID: task.id, response });
        } catch (err: any) {
          results.push({ taskID: task.id, response: { content: '', metrics: { totalTurns: 0, totalTools: 0, duration: 0, llmLatency: 0, toolLatency: 0 } }, error: err });
        }
      }
    };

    const workers = Array.from({ length: Math.min(this.maxConcurrent, tasks.length) }, () => worker());
    await Promise.all(workers);

    return results;
  }
}

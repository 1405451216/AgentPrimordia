import type { Response } from '../types.js';
import type { ReActAgent, ReActConfig } from '../agent/react-loop.js';
import type { FileScopePolicy } from '../tools/scope.js';

export interface PoolTask {
  id: string;
  input: string;
  agentConfig: ReActConfig;
  scope?: string[];
}

export interface PoolResult {
  taskID: string;
  response: Response;
  error?: Error;
}

export class AgentPool {
  private maxConcurrent: number;
  private scopePolicy?: FileScopePolicy;

  constructor(opts: { maxConcurrent?: number; scopePolicy?: FileScopePolicy } = {}) {
    this.maxConcurrent = opts.maxConcurrent ?? 5;
    this.scopePolicy = opts.scopePolicy;
  }

  async dispatch(tasks: PoolTask[]): Promise<PoolResult[]> {
    const results: PoolResult[] = [];
    const queue = [...tasks];

    const worker = async (): Promise<void> => {
      while (queue.length > 0) {
        const task = queue.shift();
        if (!task) break;

        try {
          const { ReActAgent } = await import('../agent/react-loop.js');
          const agent = new ReActAgent(task.agentConfig);
          const response = await agent.run(task.input);
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

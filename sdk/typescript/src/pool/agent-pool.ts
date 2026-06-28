import type { Response } from '../types.js';
import { ReActAgent } from '../agent/react-loop.js';
import type { ReActConfig } from '../agent/react-loop.js';
import type { FileScopePolicy } from '../tools/scope.js';

/** 任务定义，与 Go 端 PoolTask 对齐。
 *
 * 字段说明：
 * - id: 任务唯一标识
 * - input: 用户输入文本
 * - agentConfig: Agent 配置（每个任务可有独立配置）
 * - scope: 文件作用域限制（可选）
 * - timeoutMs: 超时时间（毫秒），默认 120000
 */
export interface PoolTask {
  id: string;
  input: string;
  agentConfig: ReActConfig;
  scope?: string[];
  timeoutMs?: number;
}

/** 任务执行结果，与 Go 端 PoolResult 对齐 */
export interface PoolResult {
  taskID: string;
  response: Response;
  error?: Error;
}

/** Agent 池，管理多个 Agent 实例的并发任务分发。
 *
 * 与 Go 端 AgentPool 对齐，核心特性：
 * - 并发控制：通过 maxConcurrent 限制同时执行的 Worker 数量
 * - 任务队列：使用 shift() 原子操作避免竞态条件
 * - 超时控制：每个任务支持独立超时，默认 120 秒
 * - 结果保留：按原始任务顺序返回结果
 *
 * 使用方式：
 *   const pool = new AgentPool({ maxConcurrent: 5 });
 *   const results = await pool.dispatch([task1, task2, task3]);
 */
export class AgentPool {
  private maxConcurrent: number;
  private scopePolicy?: FileScopePolicy;
  private defaultTimeoutMs: number;

  constructor(opts: { maxConcurrent?: number; scopePolicy?: FileScopePolicy; defaultTimeoutMs?: number } = {}) {
    this.maxConcurrent = opts.maxConcurrent ?? 5;
    this.scopePolicy = opts.scopePolicy;
    this.defaultTimeoutMs = opts.defaultTimeoutMs ?? 120_000;
  }

  /**
   * 分发任务到 worker 池并发执行。
   *
   * 使用任务队列（Array.shift）替代共享索引变量，避免 async worker 间
   * nextIndex++ 的竞态条件。shift() 在 JS 事件循环中是同步原子的，
   * 确保每个任务只被一个 worker 获取。
   */
  async dispatch(tasks: PoolTask[]): Promise<PoolResult[]> {
    // 任务队列：worker 通过 shift() 原子地获取下一个任务
    const queue: PoolTask[] = [...tasks];
    // 使用 Map 保留结果顺序（按原始 tasks 顺序返回）
    const resultMap = new Map<string, PoolResult>();

    const worker = async (): Promise<void> => {
      while (queue.length > 0) {
        // shift() 在 JS 单线程事件循环中是原子操作，无竞态
        const task = queue.shift();
        if (!task) break;

        const timeoutMs = task.timeoutMs ?? this.defaultTimeoutMs;
        try {
          const agent = new ReActAgent(task.agentConfig);
          const response = await Promise.race([
            agent.run(task.input),
            new Promise<never>((_, reject) =>
              setTimeout(() => reject(new Error(`Task ${task!.id} timed out after ${timeoutMs}ms`)), timeoutMs)
            ),
          ]);
          resultMap.set(task.id, { taskID: task.id, response });
        } catch (err: unknown) {
          const error = err instanceof Error ? err : new Error(String(err));
          resultMap.set(task.id, {
            taskID: task.id,
            response: { content: '', metrics: { totalTurns: 0, totalTools: 0, duration: 0, llmLatency: 0, toolLatency: 0 } },
            error,
          });
        }
      }
    };

    const workers = Array.from({ length: Math.min(this.maxConcurrent, tasks.length) }, () => worker());
    await Promise.all(workers);

    // 按原始任务顺序返回结果
    return tasks.map((t) => resultMap.get(t.id)!);
  }
}

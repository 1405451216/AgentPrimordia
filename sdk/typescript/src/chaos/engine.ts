/**
 * engine.ts — 混沌工程引擎
 *
 * 对齐 Go 端 internal/chaos/engine.go
 * Stability: Experimental
 */

import type { CleanupFunc, FaultResult } from './faults.js';
import type { SteadyState, SteadyStateResult } from './steady-state.js';

/** 实验状态 */
export type ExperimentStatus = 'pending' | 'running' | 'completed' | 'aborted' | 'failed';

/** 混沌实验定义（对齐 Go Experiment） */
export interface Experiment {
  /** 实验名称 */
  name: string;
  /** 实验描述 */
  description?: string;
  /** 假设（预期系统行为） */
  hypothesis: string;
  /** 要注入的故障列表 */
  faults: { type(): string; description(): string; inject(): Promise<CleanupFunc> }[];
  /** 稳态条件（实验前后必须满足） */
  steadyState?: SteadyState;
  /** 实验持续时间（ms），默认 30000 */
  durationMs?: number;
  /** 实验标签 */
  tags?: string[];
}

/** 实验执行结果（对齐 Go ExperimentResult） */
export interface ExperimentResult {
  experiment: Experiment;
  status: ExperimentStatus;
  startTime: Date;
  endTime: Date;
  durationMs: number;
  preSteadyState?: SteadyStateResult;
  postSteadyState?: SteadyStateResult;
  faultResults: FaultResult[];
  hypothesisValidated: boolean;
  error?: Error;
}

/** 引擎配置选项 */
export interface EngineOptions {
  /** 最大并发实验数（默认 1） */
  maxConcurrency?: number;
}

/** 混沌实验引擎（对齐 Go ChaosEngine） */
export class ChaosEngine {
  private active = new Map<string, () => void>();

  /** 运行一个混沌实验 */
  async run(exp: Experiment): Promise<ExperimentResult> {
    const startTime = new Date();
    const result: ExperimentResult = {
      experiment: exp,
      status: 'pending',
      startTime,
      endTime: startTime,
      durationMs: 0,
      faultResults: [],
      hypothesisValidated: false,
    };

    const durationMs = exp.durationMs ?? 30000;

    // 注册活跃实验
    let cancelFn!: () => void;
    const cancelPromise = new Promise<never>((_, reject) => {
      cancelFn = () => reject(new Error('experiment aborted'));
    });
    this.active.set(exp.name, cancelFn);

    result.status = 'running';

    // cleanups 需在 try 外声明以便 catch 中清理
    const cleanups: CleanupFunc[] = [];

    try {
      // 1. 实验前稳态检查
      if (exp.steadyState) {
        const pre = await exp.steadyState.check();
        result.preSteadyState = pre;
        if (!pre.met) {
          result.status = 'failed';
          result.error = new Error(`实验前稳态不满足: ${pre.message}`);
          result.endTime = new Date();
          result.durationMs = result.endTime.getTime() - startTime.getTime();
          this.active.delete(exp.name);
          return result;
        }
      }

      // 2. 注入所有故障
      for (const fault of exp.faults) {
        const fr: FaultResult = {
          faultType: fault.type(),
          description: fault.description(),
          injected: false,
          injectTime: new Date(),
        };

        try {
          const cleanup = await fault.inject();
          fr.injected = true;
          cleanups.push(cleanup);
        } catch (err) {
          fr.error = err instanceof Error ? err : new Error(String(err));
          // 清理已注入的故障
          for (const c of cleanups) {
            try { await c(); } catch { /* ignore */ }
          }
          result.faultResults.push(fr);
          result.status = 'failed';
          result.error = new Error(`故障 ${fault.type()} 注入失败: ${fr.error.message}`);
          result.endTime = new Date();
          result.durationMs = result.endTime.getTime() - startTime.getTime();
          this.active.delete(exp.name);
          return result;
        }

        result.faultResults.push(fr);
      }

      // 3. 等待实验持续时间
      await Promise.race([
        new Promise<void>(resolve => setTimeout(resolve, durationMs)),
        cancelPromise,
      ]);
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      if (msg === 'experiment aborted') {
        result.status = 'aborted';
      } else {
        result.status = 'failed';
        result.error = err instanceof Error ? err : new Error(msg);
      }

      // 清理已注入的故障
      for (let i = cleanups.length - 1; i >= 0; i--) {
        try { await cleanups[i](); } catch { /* ignore */ }
      }

      result.endTime = new Date();
      result.durationMs = result.endTime.getTime() - startTime.getTime();
      this.active.delete(exp.name);
      return result;
    }

    // 4. 清理所有故障（逆序清理）
    for (let i = cleanups.length - 1; i >= 0; i--) {
      result.faultResults[i].cleanupTime = new Date();
      try {
        await cleanups[i]();
      } catch (err) {
        result.faultResults[i].error = err instanceof Error ? err : new Error(String(err));
      }
    }

    // 5. 实验后稳态检查
    if (exp.steadyState) {
      try {
        const post = await exp.steadyState.check();
        result.postSteadyState = post;
      } catch (err) {
        result.postSteadyState = {
          met: false,
          message: `稳态检查错误: ${err instanceof Error ? err.message : String(err)}`,
        };
      }
    }

    // 6. 判定假设
    result.hypothesisValidated = result.postSteadyState?.met ?? true;
    result.status = 'completed';
    result.endTime = new Date();
    result.durationMs = result.endTime.getTime() - startTime.getTime();

    this.active.delete(exp.name);
    return result;
  }

  /** 中止一个活跃实验 */
  abort(experimentName: string): boolean {
    const cancel = this.active.get(experimentName);
    if (!cancel) return false;
    cancel();
    this.active.delete(experimentName);
    return true;
  }

  /** 列出活跃实验名称 */
  listActive(): string[] {
    return Array.from(this.active.keys());
  }

  /** 批量执行实验 */
  async runBatch(experiments: Experiment[]): Promise<ExperimentResult[]> {
    const results: ExperimentResult[] = [];
    for (const exp of experiments) {
      results.push(await this.run(exp));
    }
    return results;
  }
}

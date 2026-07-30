/**
 * engine.ts — 混沌工程引擎
 *
 * 对齐 Go 端 internal/chaos/engine.go
 * Stability: Experimental
 */

import type { FaultInjector, FaultResult } from './faults.js';
import type { SteadyState, SteadyStateResult } from './steady-state.js';

/** 实验状态 */
export type ExperimentStatus = 'pending' | 'running' | 'completed' | 'failed' | 'aborted';

/** 混沌实验定义 */
export interface Experiment {
  name: string;
  description?: string;
  hypothesis: string;
  faults: FaultInjector[];
  steadyState?: SteadyState;
  tags?: string[];
  timeoutMs?: number;
}

/** 实验执行结果 */
export interface ExperimentResult {
  experiment: Experiment;
  status: ExperimentStatus;
  startTime: Date;
  endTime: Date;
  durationMs: number;
  faultResults: FaultResult[];
  preSteadyState?: SteadyStateResult;
  postSteadyState?: SteadyStateResult;
  hypothesisValidated: boolean;
  error?: Error;
}

/** 引擎配置选项 */
export interface EngineOptions {
  /** 最大并发实验数（默认 1） */
  maxConcurrency?: number;
  /** 稳态检查失败时中止（默认 false） */
  abortOnSteadyStateFailure?: boolean;
  /** 故障清理超时（默认 5000ms） */
  cleanupTimeoutMs?: number;
}

/** 混沌工程引擎（对齐 Go ChaosEngine 接口） */
export class ChaosEngine {
  private readonly options: Required<EngineOptions>;

  constructor(options: EngineOptions = {}) {
    this.options = {
      maxConcurrency: options.maxConcurrency ?? 1,
      abortOnSteadyStateFailure: options.abortOnSteadyStateFailure ?? false,
      cleanupTimeoutMs: options.cleanupTimeoutMs ?? 5000,
    };
  }

  /** 执行单个实验 */
  async run(exp: Experiment): Promise<ExperimentResult> {
    const startTime = new Date();
    const result: ExperimentResult = {
      experiment: exp,
      status: 'running',
      startTime,
      endTime: startTime,
      durationMs: 0,
      faultResults: [],
      hypothesisValidated: false,
    };

    try {
      // 1. 实验前稳态检查
      if (exp.steadyState) {
        result.preSteadyState = await exp.steadyState.check();
        if (!result.preSteadyState.met && this.options.abortOnSteadyStateFailure) {
          result.status = 'aborted';
          result.error = new Error(`实验前稳态检查未通过: ${result.preSteadyState.message}`);
          result.endTime = new Date();
          result.durationMs = result.endTime.getTime() - startTime.getTime();
          return result;
        }
      }

      // 2. 注入故障
      for (const fault of exp.faults) {
        const faultResult: FaultResult = {
          faultType: fault.type(),
          description: fault.description(),
          injected: false,
          injectTime: new Date(),
        };

        try {
          await fault.inject();
          faultResult.injected = true;
        } catch (err) {
          faultResult.error = err instanceof Error ? err : new Error(String(err));
        }

        result.faultResults.push(faultResult);
      }

      // 3. 等待观察期（使用 timeout 或默认 100ms）
      const observeMs = exp.timeoutMs ?? 100;
      await new Promise(resolve => setTimeout(resolve, Math.min(observeMs, 1000)));

      // 4. 清理故障
      for (const fault of exp.faults) {
        try {
          await fault.cleanup();
        } catch {
          // 清理失败不中断实验
        }
      }

      // 5. 实验后稳态检查
      if (exp.steadyState) {
        result.postSteadyState = await exp.steadyState.check();
        result.hypothesisValidated = result.postSteadyState.met;
      } else {
        // 无稳态检查时，假设验证通过（所有故障成功注入即为通过）
        result.hypothesisValidated = result.faultResults.every(r => r.injected);
      }

      result.status = 'completed';
    } catch (err) {
      result.status = 'failed';
      result.error = err instanceof Error ? err : new Error(String(err));

      // 尝试清理已注入的故障
      for (const fault of exp.faults) {
        try { await fault.cleanup(); } catch { /* ignore */ }
      }
    }

    result.endTime = new Date();
    result.durationMs = result.endTime.getTime() - startTime.getTime();
    return result;
  }

  /** 批量执行实验 */
  async runBatch(experiments: Experiment[]): Promise<ExperimentResult[]> {
    const results: ExperimentResult[] = [];
    const concurrency = this.options.maxConcurrency;

    for (let i = 0; i < experiments.length; i += concurrency) {
      const batch = experiments.slice(i, i + concurrency);
      const batchResults = await Promise.all(batch.map(exp => this.run(exp)));
      results.push(...batchResults);
    }

    return results;
  }
}

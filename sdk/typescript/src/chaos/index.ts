/**
 * chaos/ — 混沌工程模块
 *
 * 对齐 Go 端 internal/chaos/ 包，提供：
 *   - ChaosEngine：实验编排器
 *   - FaultInjector：故障注入器接口与多种实现
 *   - SteadyState：稳态验证器
 *   - Report：自动生成实验报告
 *
 * Stability: Experimental
 */

// 引擎
export { ChaosEngine } from './engine.js';
export type { Experiment, ExperimentResult, ExperimentStatus, EngineOptions } from './engine.js';

// 故障注入器
export {
  LatencyFault, ErrorFault, ResourceFault, NoopFault,
  LLMTimeoutFault, LLMErrorFault, LLMRateLimitFault,
} from './faults.js';
export type { FaultInjector, FaultResult, LLMFault } from './faults.js';

// 稳态验证
export {
  SLOSteadyState, AvailabilitySteadyState, LatencySteadyState, CustomSteadyState,
} from './steady-state.js';
export type { SteadyState, SteadyStateResult } from './steady-state.js';

// 报告
export { summarize, formatReport, formatSummaryTable } from './report.js';
export type { ExperimentSummary } from './report.js';

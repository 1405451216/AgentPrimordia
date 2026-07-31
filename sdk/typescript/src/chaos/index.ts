/**
 * chaos/ — 混沌工程模块
 *
 * 对齐 Go 端 internal/chaos/ 包，提供：
 *   - ChaosEngine：实验编排器
 *   - Fault：故障定义接口与多种实现
 *   - SteadyState：稳态验证器
 *   - Report：自动生成实验报告
 *
 * Stability: Experimental
 */

// 引擎
export { ChaosEngine } from './engine.js';
export type { Experiment, ExperimentResult, ExperimentStatus, EngineOptions } from './engine.js';

// 故障注入器（基础）
export {
  NetworkDelayFault, PartitionFault, ConnectionRefusedFault,
  CPUStressFault, MemoryStressFault, ProcessKillFault,
  CompositeFault, NoopFault,
  // 向后兼容别名
  LatencyFault, ErrorFault, ResourceFault,
} from './faults.js';
export type { Fault, FaultInjector, CleanupFunc, FaultResult } from './faults.js';

// LLM 故障
export {
  LLMHTTPStatusFault, LLMTimeoutFault, LLMIntermittentFault, LLMSlowResponseFault,
  llmHTTP503Fault, llmHTTP429Fault, llmHTTP500Fault,
  llmFailoverScenario, llmChaosScenario,
  // 向后兼容别名
  LLMErrorFault, LLMRateLimitFault,
} from './llm-faults.js';
export type { LLMFaultScenario } from './llm-faults.js';

// 稳态验证
export {
  SLOSteadyState, AvailabilitySteadyState, LatencySteadyState,
  CompositeSteadyState, CustomSteadyState,
} from './steady-state.js';
export type { SteadyState, SteadyStateResult } from './steady-state.js';

// 报告
export { summarize, formatReport, formatSummaryTable } from './report.js';
export type { ExperimentSummary } from './report.js';

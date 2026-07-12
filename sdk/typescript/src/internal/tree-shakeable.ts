/**
 * Tree-shakeable 导出标记
 *
 * 本模块提供显式的具名导出，确保打包工具（esbuild/rollup/webpack）
 * 能够正确进行 tree-shaking 分析。
 *
 * 所有导出均为纯函数或纯类型，无副作用。
 * 配合 package.json 中的 "sideEffects: false" 使用。
 *
 * @module internal/tree-shakeable
 */

// ===== 核心类型导出 =====

export type { Tool, ToolDefinition, ToolResult, ToolCall } from '../types.js';
export type { Message, Chunk } from '../types.js';
export type { CompletionRequest, CompletionResponse } from '../types.js';

// ===== 错误类型导出 =====

export { CodeError, getErrorCode, withCode } from '../errors.js';
export type {
  ErrAgentStopped,
  ErrToolNotFound,
  ErrLLMCallFailed,
} from '../errors.js';

// ===== 工具函数导出 =====

/** 检查模块是否支持 tree-shaking */
export const __treeShakeable = true;

/** 模块版本标识 */
export const __bundlerMarker = '@agentprimordia/sdk' as const;
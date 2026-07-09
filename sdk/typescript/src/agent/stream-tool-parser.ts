// stream-tool-parser.ts — Phase 1 T1-3 流式 tool_calls 实时解析
// 在 LLM 流式输出中增量识别并提取 tool_call JSON 片段，
// 一旦 JSON 完整就立即返回（不等整个流结束）。
//
// 触发策略：
//   1. 启发式检测：扫描 chunk 中是否包含 tool_call 起始标记
//      （"{"name\"" / "\"function_call\"" / "<tool_call>" 等）
//   2. 找到起始标记后开始累积到 jsonBuffer
//   3. 每加一个 chunk 尝试解析 jsonBuffer
//   4. 解析成功：yield ToolCall 并重置状态
//   5. 流结束仍未完成：丢弃部分 buffer（graceful degradation）
//
// 与 doc 02-phase1-implementation.md T1-3 的差异：
//   - 用纯 JSON 解析而非"partial JSON"（避免 partial-json 依赖）
//   - 支持多种起始标记启发式（不同 LLM 格式）
//   - 单个流可产出多个 tool_call

import type { ToolCall } from '../types.js';

/** 流式 tool_call 解析器 */
export class StreamToolCallParser {
  private jsonBuffer = '';
  private inToolCall = false;
  private braceDepth = 0;
  private startMarkerUsed: string | null = null;

  // 多种 LLM 格式的起始标记（启发式匹配）
  private static readonly START_MARKERS = [
    '{"name"',
    '{"function"',
    '"tool_calls"',
    '"function_call"',
    '<tool_call>',
  ];

  // 对应的结束条件（括号匹配即可，无需特定标记）
  private static readonly END_MARKER = '</tool_call>';

  /**
   * 处理下一个 chunk，返回该 chunk 中提取到的 tool_calls（0 个或多个）。
   * 不修改 chunk 内容；当 chunk 不包含 tool_call 起始时返回空数组。
   */
  push(chunk: string): ToolCall[] {
    if (!chunk) return [];

    // 还未进入 tool_call 状态：扫描起始标记
    if (!this.inToolCall) {
      const markerIdx = this.findStartMarker(chunk);
      if (markerIdx < 0) {
        return []; // 当前 chunk 是普通文本
      }
      // 进入 tool_call 模式：从起始标记开始累积
      this.inToolCall = true;
      this.jsonBuffer = chunk.slice(markerIdx);
      this.startMarkerUsed = chunk.slice(markerIdx, markerIdx + 20);
      this.braceDepth = this.countBraces(this.jsonBuffer);
      return this.tryExtract();
    }

    // 已在 tool_call 状态：累积 + 解析
    this.jsonBuffer += chunk;
    this.braceDepth = this.countBraces(this.jsonBuffer);
    return this.tryExtract();
  }

  /**
   * 流结束时调用。返回剩余 buffer 中能解析出的 tool_calls（如果有）。
   * 解析失败则丢弃 buffer 并返回空。
   */
  end(): ToolCall[] {
    if (!this.inToolCall) return [];
    this.inToolCall = false;
    const result = this.tryExtract();
    this.reset();
    return result;
  }

  /** 重置状态（用于流异常中断） */
  reset(): void {
    this.jsonBuffer = '';
    this.inToolCall = false;
    this.braceDepth = 0;
    this.startMarkerUsed = null;
  }

  /** 当前是否处于 tool_call 累积状态（用于诊断/调试） */
  isAccumulating(): boolean {
    return this.inToolCall;
  }

  // ===== 内部 =====

  private findStartMarker(chunk: string): number {
    let earliest = -1;
    for (const marker of StreamToolCallParser.START_MARKERS) {
      const idx = chunk.indexOf(marker);
      if (idx >= 0 && (earliest < 0 || idx < earliest)) {
        earliest = idx;
      }
    }
    return earliest;
  }

  /**
   * 计算字符串中 { 和 } 的差值（未配对的 { 数量）。
   * 返回 0 表示大括号已平衡（可能已完整）。
   */
  private countBraces(s: string): number {
    let depth = 0;
    for (let i = 0; i < s.length; i++) {
      const c = s[i];
      if (c === '{') depth++;
      else if (c === '}') depth--;
      if (depth < 0) depth = 0; // 容错：多于的 } 视为已闭合
    }
    return depth;
  }

  /**
   * 尝试从 buffer 中解析 tool_calls。
   * - 平衡但无法解析：返回空（继续累积）
   * - 平衡且解析成功：返回 tool_calls 并重置 buffer（保留未消费部分）
   */
  private tryExtract(): ToolCall[] {
    if (this.braceDepth > 0) {
      // 还未平衡，继续累积
      return [];
    }
    if (!this.jsonBuffer) return [];

    // 尝试解析整个 buffer
    const extracted = this.extractToolCallsFromJson(this.jsonBuffer);
    if (extracted.length > 0) {
      // 成功提取：从 buffer 中移除已解析部分
      // 简化策略：解析完整 → 重置整个 buffer
      // 生产实现应跟踪 lastIndex 复用前缀
      this.jsonBuffer = '';
      this.inToolCall = false;
      this.braceDepth = 0;
      this.startMarkerUsed = null;
    }
    return extracted;
  }

  /**
   * 从 JSON 字符串中提取 ToolCall 列表。
   * 支持三种格式：
   *   1. 单个 ToolCall 对象：{"name": "...", "arguments": "...", "id": "..."}
   *   2. tool_calls 数组：[{"name": ..., "id": ...}, ...]
   *   3. 嵌套：{"tool_calls": [...]}
   */
  private extractToolCallsFromJson(text: string): ToolCall[] {
    // 提取所有 { ... } 顶层对象（启发式：找第一个 { 到匹配的 }）
    const startIdx = text.indexOf('{');
    if (startIdx < 0) return [];

    // 尝试 1：整个文本是一个 JSON 对象
    try {
      const obj = JSON.parse(text);
      return this.normalizeToolCalls(obj);
    } catch {
      // 不是合法 JSON，继续尝试其他方式
    }

    // 尝试 2：提取所有顶层 {...}
    const results: ToolCall[] = [];
    let depth = 0;
    let objStart = -1;
    for (let i = 0; i < text.length; i++) {
      const c = text[i];
      if (c === '{') {
        if (depth === 0) objStart = i;
        depth++;
      } else if (c === '}') {
        depth--;
        if (depth === 0 && objStart >= 0) {
          const candidate = text.slice(objStart, i + 1);
          try {
            const parsed = JSON.parse(candidate);
            const tc = this.normalizeToolCalls(parsed);
            results.push(...tc);
          } catch {
            // 跳过单个不可解析的对象
          }
          objStart = -1;
        }
      }
    }
    return results;
  }

  /**
   * 将任意解析结果规范化为 ToolCall[]。
   * 支持：单个 ToolCall / 数组 / 含 tool_calls 字段的对象。
   */
  private normalizeToolCalls(parsed: unknown): ToolCall[] {
    if (Array.isArray(parsed)) {
      return parsed.flatMap((p) => this.normalizeToolCalls(p));
    }
    if (parsed && typeof parsed === 'object') {
      const obj = parsed as Record<string, unknown>;
      if (Array.isArray(obj.tool_calls)) {
        return obj.tool_calls.flatMap((p) => this.normalizeToolCalls(p));
      }
      // 单个 ToolCall：必须有 name + arguments 或 function
      if (typeof obj.name === 'string' && typeof obj.arguments === 'string') {
        return [{
          id: typeof obj.id === 'string' ? obj.id : generateToolCallId(),
          name: obj.name,
          arguments: obj.arguments,
        }];
      }
      // OpenAI function_call 格式：{ function: { name, arguments }, id }
      if (obj.function && typeof obj.function === 'object') {
        const fn = obj.function as Record<string, unknown>;
        if (typeof fn.name === 'string') {
          return [{
            id: typeof obj.id === 'string' ? obj.id : generateToolCallId(),
            name: fn.name,
            arguments: typeof fn.arguments === 'string' ? fn.arguments : JSON.stringify(fn.arguments ?? {}),
          }];
        }
      }
    }
    return [];
  }
}

let toolCallCounter = 0;
function generateToolCallId(): string {
  return `tc_${Date.now()}_${++toolCallCounter}`;
}
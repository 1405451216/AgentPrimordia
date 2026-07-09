/**
 * Prompt Registry — Prompt 平台化的「版本管理」入口（T2-2 文档命名文件）。
 *
 * 复用已实现的 VersionedPromptRegistry（versioned-registry.ts）作为底层存储，
 * 在此之上提供：
 * - 与文档一致的 PromptRegistry 别名
 * - diffPrompts()：基于 LCS 的 Prompt 文本差异，用于 Review / 回放
 *
 * 这样既满足 evolution 计划「新增 prompt-registry.ts」的命名要求，
 * 又不重复实现版本管理核心逻辑（其单元测试见 prompt-platform-t2-2.test.ts）。
 */

export {
  VersionedPromptRegistry as PromptRegistry,
  type PromptVersion,
  type PromptEntry,
  type PromptRegistryOptions,
} from './versioned-registry.js';

/** 单行差异类型 */
export type DiffOp = 'equal' | 'insert' | 'delete';
export interface DiffLine {
  op: DiffOp;
  text: string;
  /** 在 a 中的行号（从 1 开始，insert 为 null） */
  aLine: number | null;
  /** 在 b 中的行号（从 1 开始，delete 为 null） */
  bLine: number | null;
}

/** 两 Prompt 文本的差异结果 */
export interface PromptDiff {
  lines: DiffLine[];
  added: number;
  removed: number;
  /** 相似度 [0,1]（基于保留行占比） */
  similarity: number;
}

/**
 * 计算两个 Prompt 文本的差异（基于行级 LCS）。
 * 用于 Prompt 变更评审、回放对比与热更新预览。
 */
export function diffPrompts(a: string, b: string): PromptDiff {
  const aLines = a.split('\n');
  const bLines = b.split('\n');
  const n = aLines.length;
  const m = bLines.length;

  // LCS DP 表
  const dp: number[][] = Array.from({ length: n + 1 }, () => new Array(m + 1).fill(0));
  for (let i = n - 1; i >= 0; i--) {
    for (let j = m - 1; j >= 0; j--) {
      dp[i][j] = aLines[i] === bLines[j]
        ? dp[i + 1][j + 1] + 1
        : Math.max(dp[i + 1][j], dp[i][j + 1]);
    }
  }

  const lines: DiffLine[] = [];
  let i = 0, j = 0;
  let kept = 0;
  const total = Math.max(n, m);
  while (i < n && j < m) {
    if (aLines[i] === bLines[j]) {
      lines.push({ op: 'equal', text: aLines[i]!, aLine: i + 1, bLine: j + 1 });
      kept++;
      i++; j++;
    } else if (dp[i + 1][j] >= dp[i][j + 1]) {
      lines.push({ op: 'delete', text: aLines[i]!, aLine: i + 1, bLine: null });
      i++;
    } else {
      lines.push({ op: 'insert', text: bLines[j]!, aLine: null, bLine: j + 1 });
      j++;
    }
  }
  while (i < n) lines.push({ op: 'delete', text: aLines[i]!, aLine: i + 1, bLine: null }), i++;
  while (j < m) lines.push({ op: 'insert', text: bLines[j]!, aLine: null, bLine: j + 1 }), j++;

  const added = lines.filter((l) => l.op === 'insert').length;
  const removed = lines.filter((l) => l.op === 'delete').length;
  const similarity = total > 0 ? kept / total : 1;

  return { lines, added, removed, similarity };
}

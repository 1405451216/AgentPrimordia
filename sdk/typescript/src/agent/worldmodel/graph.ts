/**
 * graph.ts — 世界模型状态图内核（TypeScript 端，矩阵 #1 对等）。
 *
 * 与 Go 端 internal/agent/worldmodel/graph.go 逐语义对齐：
 *   - 五类节点 / 四类边；预演态与观测态靠边分型；
 *   - 同一 (Kind, 规范化 Summary) 只产生一个节点：确定性 ID =
 *     "<kind>:<fnv1a64 十六进制>"（Kind 与规范化摘要以 NUL 分隔后哈希）；
 *   - 并发安全在 TS 单线程事件循环下天然满足；对外只暴露值快照。
 *
 * 跨语言对账：nodeId 输出与 Go NodeID 逐位一致（fixture 覆盖 ASCII/空白
 * 规范化/中文/Kind 区分），见 src/agent/__tests__/worldmodel.test.ts。
 */

// ===== 节点种类（对应路线图 §三 的五类世界事实）=====

export const KindTask = 'task' as const;
export const KindPlan = 'plan' as const;
export const KindToolCall = 'tool_call' as const;
export const KindObservation = 'observation' as const;
export const KindHypothesis = 'hypothesis' as const;

export type NodeKind = typeof KindTask | typeof KindPlan | typeof KindToolCall | typeof KindObservation | typeof KindHypothesis;

// ===== 边种类（预演态/观测态在状态图中强制分型的载体）=====

export const EdgeCause = 'cause' as const;
export const EdgePlan = 'plan' as const;
export const EdgeContext = 'context' as const;
export const EdgeHypothesis = 'hypothesis' as const;

export type EdgeKind = typeof EdgeCause | typeof EdgePlan | typeof EdgeContext | typeof EdgeHypothesis;

/**
 * 有向边（键名与 Go 序列化形态一致：Snapshot JSON 双线可直接互换）。
 */
export interface StateEdge {
  To: string;
  Kind: EdgeKind;
}

/** 状态图节点：一条结构化世界事实（键名与 Go 序列化形态一致）。 */
export interface StateNode {
  ID: string;
  Kind: NodeKind;
  Summary: string;
  CreatedAtTurn: number;
  Edges: StateEdge[] | null;
}

/** 摘要规范化：按 Unicode 空白切分后以单空格重组（与 Go strings.Fields 对齐）。 */
export function normalizeSummary(s: string): string {
  return s
    .trim()
    .split(/\s+/)
    .filter((x) => x.length > 0)
    .join(' ');
}

/** FNV-1a 64 位（BigInt 模拟 uint64 回绕；与 Go hash/fnv 逐位一致）。 */
function fnv1a64(bytes: Uint8Array): bigint {
  let h = 0xcbf29ce484222325n;
  const prime = 0x100000001b3n;
  const mask = (1n << 64n) - 1n;
  for (const b of bytes) {
    h ^= BigInt(b);
    h = (h * prime) & mask;
  }
  return h;
}

/** stateNodeID 由 (Kind, 规范化摘要) 派生确定性节点 ID（与 Go stateNodeID 一致）。 */
function stateNodeID(kind: NodeKind, normalized: string): string {
  const enc = new TextEncoder();
  const kindBytes = enc.encode(kind);
  const sumBytes = enc.encode(normalized);
  const buf = new Uint8Array(kindBytes.length + 1 + sumBytes.length);
  buf.set(kindBytes, 0);
  buf[kindBytes.length] = 0; // NUL 分隔防拼接歧义
  buf.set(sumBytes, kindBytes.length + 1);
  return kind + ':' + fnv1a64(buf).toString(16);
}

/** nodeId 由 (Kind, 摘要) 派生确定性节点 ID（先规范化再哈希，与 Go NodeID 一致）。 */
export function nodeId(kind: NodeKind, summary: string): string {
  return stateNodeID(kind, normalizeSummary(summary));
}

/** cloneNode 节点防御性拷贝（Edges 深拷贝，出参改动不回流内部状态）。 */
function cloneNode(n: StateNode): StateNode {
  return { ...n, Edges: n.Edges ? n.Edges.map((e) => ({ ...e })) : null };
}

/** 有向状态图（AddNode/AddEdge/Node/Nodes/PathTo 与 Go 同语义同确定性）。 */
export class StateGraph {
  private nodeMap = new Map<string, StateNode>();
  private rev = new Map<string, string[]>();

  /** 添加（或命中去重）节点，返回 [节点 ID, 是否新建]。 */
  addNode(kind: NodeKind, summary: string, createdAtTurn: number): [string, boolean] {
    const norm = normalizeSummary(summary);
    const id = stateNodeID(kind, norm);
    if (this.nodeMap.has(id)) {
      return [id, false];
    }
    this.nodeMap.set(id, { ID: id, Kind: kind, Summary: norm, CreatedAtTurn: createdAtTurn, Edges: null });
    return [id, true];
  }

  /**
   * 按权威 ID 直接插入节点（仅供快照恢复 graphFromNodes 使用，与 Go 的
   * 直接 map 插入语义一致——快照 ID 是权威，不强制等于派生 ID）。
   * 调用方必须已做去重校验。
   */
  insertNode(n: StateNode): void {
    this.nodeMap.set(n.ID, cloneNode(n));
  }

  /** 添加有向边，返回是否新建（端点不存在或同 (To,Kind) 边已存在时不新建）。 */
  addEdge(from: string, to: string, kind: EdgeKind): boolean {
    const fromNode = this.nodeMap.get(from);
    if (!fromNode || !this.nodeMap.has(to)) {
      return false;
    }
    if (fromNode.Edges) {
      for (const e of fromNode.Edges) {
        if (e.To === to && e.Kind === kind) {
          return false;
        }
      }
    }
    const edge: StateEdge = { To: to, Kind: kind };
    if (fromNode.Edges) {
      fromNode.Edges.push(edge);
    } else {
      fromNode.Edges = [edge];
    }
    const revList = this.rev.get(to);
    if (revList) {
      revList.push(from);
    } else {
      this.rev.set(to, [from]);
    }
    return true;
  }

  /** 返回节点值快照（防御性拷贝）；不存在时返回 undefined。 */
  node(id: string): StateNode | undefined {
    const n = this.nodeMap.get(id);
    return n ? cloneNode(n) : undefined;
  }

  /** 返回全部节点快照，按 ID 升序（确定性遍历序）。 */
  nodes(): StateNode[] {
    return [...this.nodeMap.values()].map(cloneNode).sort((a, b) => (a.ID < b.ID ? -1 : a.ID > b.ID ? 1 : 0));
  }

  /**
   * 从某个根节点（入度为零）到目标节点的 BFS 最短路径（含两端）。
   * 确定性约定：根按 ID 升序尝试，邻接按目标 ID 升序扩展。
   */
  pathTo(id: string): string[] | null {
    if (!this.nodeMap.has(id)) {
      return null;
    }
    const roots = [...this.nodeMap.keys()].filter((k) => !this.rev.has(k)).sort();
    for (const root of roots) {
      const path = this.bfsPath(root, id);
      if (path) {
        return path;
      }
    }
    return null;
  }

  private bfsPath(root: string, target: string): string[] | null {
    if (root === target) {
      return [root];
    }
    const visited = new Set<string>([root]);
    const parent = new Map<string, string>();
    const queue: string[] = [root];
    while (queue.length > 0) {
      const cur = queue.shift()!;
      const nexts = (this.nodeMap.get(cur)!.Edges ?? []).map((e) => e.To).sort();
      for (const next of nexts) {
        if (visited.has(next)) {
          continue;
        }
        visited.add(next);
        parent.set(next, cur);
        if (next === target) {
          const path: string[] = [target];
          let p = parent.get(target)!;
          while (p !== root) {
            path.push(p);
            p = parent.get(p)!;
          }
          path.push(root);
          path.reverse();
          return path;
        }
        queue.push(next);
      }
    }
    return null;
  }
}

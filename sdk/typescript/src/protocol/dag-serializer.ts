/**
 * DAG JSON 序列化/反序列化（TypeScript 端）。
 *
 * 与 Go 端 internal/agent/dag/serializer.go 互通：
 * - 字段命名使用 snake_case（对应 Go 的 json tag）
 * - 确保 Go 序列化的 JSON 可被 TS 端解析，反之亦然
 */
import { DAGBuilder } from '../orchestration/advanced.js';

/** DAG JSON 协议版本 */
export const DAG_JSON_VERSION = '1.0';

/** 节点类型常量 */
export const NodeTypeAgent = 'agent';
export const NodeTypeTool = 'tool';
export const NodeTypeCondition = 'condition';

export type NodeType =
  | typeof NodeTypeAgent
  | typeof NodeTypeTool
  | typeof NodeTypeCondition;

// ===== JSON 结构定义 =====

/** DAG 的 JSON 兼容表示 */
export interface DAGJSON {
  version: string;
  name: string;
  nodes: DAGNodeJSON[];
  edges: DAGEdgeJSON[];
  metadata?: Record<string, string>;
}

/** 节点的 JSON 表示 */
export interface DAGNodeJSON {
  id: string;
  type: NodeType;
  config: Record<string, unknown>;
  inputs: string[];
  outputs: string[];
  depends_on?: string[];
}

/** 边的 JSON 表示 */
export interface DAGEdgeJSON {
  from: string;
  to: string;
  label?: string;
  condition: boolean;
}

// ===== 序列化 =====

/**
 * 从 DAG 的节点/边数据构建 DAGJSON。
 *
 * 由于 DAGWorkflow 的内部结构（nodes Map, edges array）是 private 的，
 * 序列化需要调用方提供节点和边的信息。
 */
export function serializeDAG(params: {
  name: string;
  nodes: Array<{
    id: string;
    config?: Record<string, unknown>;
  }>;
  edges: Array<{
    from: string;
    to: string;
    condition?: boolean;
  }>;
  metadata?: Record<string, string>;
}): DAGJSON {
  const { name, nodes, edges, metadata } = params;

  // 构建邻接映射
  const outgoingMap: Record<string, string[]> = {};
  const incomingMap: Record<string, string[]> = {};
  for (const edge of edges) {
    if (!outgoingMap[edge.from]) outgoingMap[edge.from] = [];
    outgoingMap[edge.from].push(edge.to);
    if (!incomingMap[edge.to]) incomingMap[edge.to] = [];
    incomingMap[edge.to].push(edge.from);
  }

  // 按节点 ID 排序（确定性输出，与 Go 端一致）
  const sortedNodes = [...nodes].sort((a, b) => a.id.localeCompare(b.id));

  return {
    version: DAG_JSON_VERSION,
    name,
    nodes: sortedNodes.map((node) => {
      const outputs = outgoingMap[node.id] ?? [];
      const dependsOn = incomingMap[node.id] ?? [];
      return {
        id: node.id,
        type: NodeTypeAgent,
        config: node.config ?? {},
        inputs: [] as string[],
        outputs: [...outputs].sort(),
        depends_on: dependsOn.length > 0 ? [...dependsOn].sort() : undefined,
      };
    }),
    edges: edges.map((edge) => ({
      from: edge.from,
      to: edge.to,
      label: undefined,
      condition: edge.condition ?? false,
    })),
    metadata,
  };
}

/**
 * 将 DAGJSON 序列化为 JSON 字符串。
 */
export function serializeDAGToString(
  params: Parameters<typeof serializeDAG>[0],
): string {
  return JSON.stringify(serializeDAG(params));
}

// ===== 反序列化 =====

/**
 * 从 DAGJSON 重建 DAGWorkflow。
 *
 * handlers 参数用于注入每个节点的 handler。
 */
export function deserializeDAG(
  json: DAGJSON,
  handlers: Record<string, (input: string) => Promise<string>>,
): ReturnType<DAGBuilder['build']> {
  const builder = new DAGBuilder(json.name);

  for (const nodeJSON of json.nodes) {
    const handler = handlers[nodeJSON.id] ?? (async (input: string) => input);
    builder.nodeWithConfig(nodeJSON.id, handler as any, nodeJSON.config as any);
  }

  for (const edgeJSON of json.edges) {
    if (edgeJSON.condition) {
      builder.edge(edgeJSON.from, edgeJSON.to, () => true);
    } else {
      builder.edge(edgeJSON.from, edgeJSON.to);
    }
  }

  return builder.build();
}

/**
 * 从 JSON 字符串反序列化 DAGWorkflow。
 */
export function deserializeDAGFromString(
  json: string,
  handlers: Record<string, (input: string) => Promise<string>>,
): ReturnType<DAGBuilder['build']> {
  const dagJSON: DAGJSON = JSON.parse(json);
  return deserializeDAG(dagJSON, handlers);
}


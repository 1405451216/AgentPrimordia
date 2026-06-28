import type { Tool } from '../types.js';
import type { ReActAgent } from './react-loop.js';

// ===== AgentTool 配置 =====

/** AgentTool 配置，与 Go 端 AgentToolConfig 对齐。
 *
 * 字段说明：
 * - description: 工具描述，默认自动生成
 * - paramSchema: 自定义输入参数 JSON Schema
 * - maxSubTurns: 子 Agent 最大轮数，默认 10
 * - passContext: 是否将父 Agent 上下文传递给子 Agent
 */
export interface AgentToolConfig {
  description?: string;
  paramSchema?: Record<string, unknown>;
  maxSubTurns?: number;
  passContext?: boolean;
}

/** 默认参数 Schema，与 Go 端 defaultParamSchema 对齐 */
const DEFAULT_PARAM_SCHEMA: Record<string, unknown> = {
  type: 'object',
  properties: {
    input: {
      type: 'string',
      description: '传递给子 Agent 的输入文本',
    },
  },
  required: ['input'],
};

// ===== AgentTool =====

/** AgentTool 将 Agent 适配为 Tool 接口，与 Go 端 AgentTool 对齐。
 *
 * 使一个 Agent 可以在 ReAct Loop 中作为工具调用另一个 Agent，
 * 实现 Agent 组合与委托模式。
 *
 * 使用方式：
 *   const subAgent = new ReActAgent({ name: 'math', ... });
 *   const tool = new AgentTool(subAgent, { description: '数学计算助手' });
 *   registry.register(tool);
 *   // 现在主 Agent 可以通过 tool_call 调用子 Agent
 */
export class AgentTool implements Tool {
  readonly name: string;
  readonly description: string;
  readonly parameters: Record<string, unknown>;

  private agent: ReActAgent;
  private config: AgentToolConfig;

  /**
   * 创建 Agent-as-Tool 适配器。
   *
   * @param agent - 子 Agent 实例
   * @param opts - 可选配置
   */
  constructor(agent: ReActAgent, opts?: AgentToolConfig) {
    this.agent = agent;
    this.config = {
      maxSubTurns: 10,
      passContext: false,
      ...opts,
    };

    this.name = `agent_${agent.name}`;
    this.description = this.config.description ?? `委托子 Agent [${agent.name}] 执行任务`;
    this.parameters = this.config.paramSchema ?? DEFAULT_PARAM_SCHEMA;
  }

  /**
   * 执行工具调用 — 调用子 Agent 并返回结果。
   *
   * 与 Go 端 AgentTool.Execute 对齐：
   * 1. 解析参数中的 input 字段
   * 2. 调用子 Agent 的 run 方法
   * 3. 返回子 Agent 的响应内容
   *
   * @param args - 工具参数，必须包含 input 字段
   * @returns 子 Agent 的执行结果字符串
   */
  async execute(args: Record<string, unknown>): Promise<string> {
    const input = args.input;
    if (typeof input !== 'string' || input.trim() === '') {
      throw new Error("缺少必需参数 'input'");
    }

    try {
      const resp = await this.agent.run(input);
      return resp.content;
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      throw new Error(`子 Agent [${this.agent.name}] 执行失败: ${msg}`);
    }
  }
}
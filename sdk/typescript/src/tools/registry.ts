import type { Tool, ToolDefinition, ToolCall, ToolResult } from '../types.js';

/** 工具注册表，管理工具的注册、查找和执行。
 *
 * 与 Go 端 tools.Registry 对齐，核心功能：
 * - 注册/移除工具（register / remove）
 * - 按名称查找工具（get / has）
 * - 导出工具定义供 LLM 使用（definitions）
 * - 执行工具调用（execute），自动解析 JSON 参数
 *
 * 使用方式：
 *   const registry = new ToolRegistry();
 *   registry.register(myTool);
 *   const result = await registry.execute({ id: '1', name: 'myTool', arguments: '{}' });
 */
export class ToolRegistry {
  private tools: Map<string, Tool> = new Map();

  /** 注册工具，同名工具会被覆盖 */
  register(tool: Tool): void {
    this.tools.set(tool.name, tool);
  }

  /** 按名称获取工具，不存在返回 undefined */
  get(name: string): Tool | undefined {
    return this.tools.get(name);
  }

  /** 返回所有已注册工具的列表 */
  list(): Tool[] {
    return Array.from(this.tools.values());
  }

  /** 导出工具定义列表，格式兼容 OpenAI Function Calling */
  definitions(): ToolDefinition[] {
    return this.list().map((t) => ({
      type: 'function' as const,
      function: {
        name: t.name,
        description: t.description,
        parameters: t.parameters,
      },
    }));
  }

  /** 执行工具调用，自动解析 JSON 参数 */
  async execute(call: ToolCall): Promise<ToolResult> {
    const tool = this.tools.get(call.name);
    if (!tool) {
      return {
        toolCallId: call.id,
        content: `tool not found: ${call.name}`,
        isError: true,
      };
    }

    try {
      const args = JSON.parse(call.arguments);
      const content = await tool.execute(args);
      // Ensure content is always a string — tools may return objects/arrays
      const contentStr = typeof content === 'string' ? content : JSON.stringify(content, null, 2);
      return { toolCallId: call.id, content: contentStr, isError: false };
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      return { toolCallId: call.id, content: msg, isError: true };
    }
  }

  /** 检查工具是否已注册 */
  has(name: string): boolean {
    return this.tools.has(name);
  }

  /** 移除工具，返回是否成功 */
  remove(name: string): boolean {
    return this.tools.delete(name);
  }

  /** 返回已注册工具数量 */
  size(): number {
    return this.tools.size;
  }
}

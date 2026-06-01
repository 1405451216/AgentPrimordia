import type { Tool, ToolDefinition, ToolCall, ToolResult } from '../types.js';

export class ToolRegistry {
  private tools: Map<string, Tool> = new Map();

  register(tool: Tool): void {
    this.tools.set(tool.name, tool);
  }

  get(name: string): Tool | undefined {
    return this.tools.get(name);
  }

  list(): Tool[] {
    return Array.from(this.tools.values());
  }

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
      return { toolCallId: call.id, content, isError: false };
    } catch (err: any) {
      return { toolCallId: call.id, content: err.message, isError: true };
    }
  }

  has(name: string): boolean {
    return this.tools.has(name);
  }

  remove(name: string): boolean {
    return this.tools.delete(name);
  }

  size(): number {
    return this.tools.size;
  }
}

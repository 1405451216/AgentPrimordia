/**
 * ToolRegistry 核心单元测试
 *
 * 覆盖：
 * - register / get / has / remove / list / size
 * - definitions（OpenAI Function Calling 格式）
 * - execute（正常 / 工具不存在 / JSON 解析失败 / 工具抛异常）
 * - 同名工具覆盖语义
 */
import { describe, it, expect, vi } from 'vitest';
import { ToolRegistry } from '../../src/tools/registry.js';
import type { Tool } from '../../src/types.js';

// ===== Mock 工具 =====

class EchoTool implements Tool {
  name = 'echo';
  description = 'Echo back the input argument';
  parameters = {
    type: 'object' as const,
    properties: {
      message: { type: 'string', description: 'The message to echo' },
    },
    required: ['message'],
  };
  async execute(args: Record<string, unknown>): Promise<string> {
    return `echo: ${args.message}`;
  }
}

class AddTool implements Tool {
  name = 'add';
  description = 'Add two numbers';
  parameters = {
    type: 'object' as const,
    properties: {
      a: { type: 'number' },
      b: { type: 'number' },
    },
    required: ['a', 'b'],
  };
  async execute(args: Record<string, unknown>): Promise<string> {
    const result = Number(args.a) + Number(args.b);
    return String(result);
  }
}

class ErrorTool implements Tool {
  name = 'error_tool';
  description = 'A tool that always throws';
  parameters = { type: 'object' as const, properties: {} };
  async execute(): Promise<string> {
    throw new Error('tool execution failed');
  }
}

class ObjectReturnTool implements Tool {
  name = 'object_return';
  description = 'Returns an object (not string)';
  parameters = { type: 'object' as const, properties: {} };
  async execute(): Promise<string> {
    // Tools must return string, but we test the safety net
    return JSON.stringify({ key: 'value' });
  }
}

// ===== Tests =====

describe('ToolRegistry', () => {
  describe('register / get / has', () => {
    it('should register and retrieve a tool', () => {
      const reg = new ToolRegistry();
      const tool = new EchoTool();
      reg.register(tool);

      expect(reg.has('echo')).toBe(true);
      expect(reg.has('nonexistent')).toBe(false);

      const retrieved = reg.get('echo');
      expect(retrieved).toBe(tool);
    });

    it('should return undefined for unknown tool', () => {
      const reg = new ToolRegistry();
      expect(reg.get('unknown')).toBeUndefined();
    });

    it('should overwrite tool with same name', () => {
      const reg = new ToolRegistry();
      const tool1 = new EchoTool();
      tool1.description = 'First version';
      reg.register(tool1);

      const tool2 = new EchoTool();
      tool2.description = 'Second version';
      reg.register(tool2);

      expect(reg.size()).toBe(1);
      expect(reg.get('echo')!.description).toBe('Second version');
    });
  });

  describe('list / size', () => {
    it('should list all registered tools', () => {
      const reg = new ToolRegistry();
      reg.register(new EchoTool());
      reg.register(new AddTool());

      const list = reg.list();
      expect(list.length).toBe(2);
      expect(list.map((t) => t.name).sort()).toEqual(['add', 'echo']);
    });

    it('should return empty list when no tools registered', () => {
      const reg = new ToolRegistry();
      expect(reg.list()).toEqual([]);
      expect(reg.size()).toBe(0);
    });

    it('size should reflect unique tool count', () => {
      const reg = new ToolRegistry();
      reg.register(new EchoTool());
      reg.register(new EchoTool()); // overwrite
      reg.register(new AddTool());

      expect(reg.size()).toBe(2);
    });
  });

  describe('remove', () => {
    it('should remove a registered tool', () => {
      const reg = new ToolRegistry();
      reg.register(new EchoTool());

      expect(reg.remove('echo')).toBe(true);
      expect(reg.has('echo')).toBe(false);
      expect(reg.size()).toBe(0);
    });

    it('should return false when removing unknown tool', () => {
      const reg = new ToolRegistry();
      expect(reg.remove('nonexistent')).toBe(false);
    });
  });

  describe('definitions', () => {
    it('should export OpenAI-compatible function definitions', () => {
      const reg = new ToolRegistry();
      reg.register(new EchoTool());
      reg.register(new AddTool());

      const defs = reg.definitions();
      expect(defs.length).toBe(2);

      // Each definition should have type='function' and function.name/description/parameters
      for (const def of defs) {
        expect(def.type).toBe('function');
        expect(def.function.name).toBeTruthy();
        expect(def.function.description).toBeTruthy();
        expect(def.function.parameters).toBeDefined();
      }

      const names = defs.map((d) => d.function.name).sort();
      expect(names).toEqual(['add', 'echo']);
    });

    it('should return empty array when no tools registered', () => {
      const reg = new ToolRegistry();
      expect(reg.definitions()).toEqual([]);
    });
  });

  describe('execute', () => {
    it('should execute a tool with valid JSON arguments', async () => {
      const reg = new ToolRegistry();
      reg.register(new EchoTool());

      const result = await reg.execute({
        id: 'call-1',
        name: 'echo',
        arguments: JSON.stringify({ message: 'hello' }),
      });

      expect(result.toolCallId).toBe('call-1');
      expect(result.content).toBe('echo: hello');
      expect(result.isError).toBe(false);
    });

    it('should execute add tool with numeric arguments', async () => {
      const reg = new ToolRegistry();
      reg.register(new AddTool());

      const result = await reg.execute({
        id: 'call-2',
        name: 'add',
        arguments: JSON.stringify({ a: 3, b: 5 }),
      });

      expect(result.content).toBe('8');
      expect(result.isError).toBe(false);
    });

    it('should return error result for unknown tool', async () => {
      const reg = new ToolRegistry();

      const result = await reg.execute({
        id: 'call-3',
        name: 'nonexistent',
        arguments: '{}',
      });

      expect(result.isError).toBe(true);
      expect(result.content).toContain('tool not found');
      expect(result.toolCallId).toBe('call-3');
    });

    it('should return error result for invalid JSON arguments', async () => {
      const reg = new ToolRegistry();
      reg.register(new EchoTool());

      const result = await reg.execute({
        id: 'call-4',
        name: 'echo',
        arguments: 'not valid json',
      });

      expect(result.isError).toBe(true);
      expect(result.toolCallId).toBe('call-4');
    });

    it('should catch tool execution errors and return error result', async () => {
      const reg = new ToolRegistry();
      reg.register(new ErrorTool());

      const result = await reg.execute({
        id: 'call-5',
        name: 'error_tool',
        arguments: '{}',
      });

      expect(result.isError).toBe(true);
      expect(result.content).toBe('tool execution failed');
      expect(result.toolCallId).toBe('call-5');
    });

    it('should handle empty arguments string', async () => {
      const reg = new ToolRegistry();
      reg.register(new EchoTool());

      const result = await reg.execute({
        id: 'call-6',
        name: 'echo',
        arguments: '',
      });

      // Empty string is not valid JSON → should error
      expect(result.isError).toBe(true);
    });

    it('should preserve toolCallId in all result types', async () => {
      const reg = new ToolRegistry();
      reg.register(new EchoTool());

      // Success case
      const ok = await reg.execute({ id: 'id-ok', name: 'echo', arguments: '{"message":"hi"}' });
      expect(ok.toolCallId).toBe('id-ok');

      // Error case (unknown tool)
      const err = await reg.execute({ id: 'id-err', name: 'unknown', arguments: '{}' });
      expect(err.toolCallId).toBe('id-err');
    });
  });
});

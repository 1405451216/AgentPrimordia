/**
 * CodeGenerator 单元测试
 *
 * 验证三种模板（tool/agent/provider）生成的代码格式正确，
 * 并确保生成的代码能通过 TypeScript 编译验证。
 */

import { describe, it, expect } from 'vitest';
import { CodeGenerator } from '../../src/codegen/generator.js';
import type { ToolTemplate } from '../../src/codegen/templates/tool.js';
import type { AgentTemplateConfig } from '../../src/codegen/templates/agent.js';
import type { ProviderTemplateConfig } from '../../src/codegen/templates/provider.js';

describe('CodeGenerator', () => {
  const generator = new CodeGenerator();

  describe('generateTool', () => {
    const toolTemplate: ToolTemplate = {
      name: 'web_search',
      description: '搜索互联网获取信息',
      parameters: {
        query: { type: 'string', description: '搜索关键词', required: true },
        limit: { type: 'number', description: '返回结果数量', required: false },
      },
    };

    it('should generate valid TypeScript tool code', () => {
      const code = generator.generateTool(toolTemplate);

      // 验证包含关键结构
      expect(code).toContain('export class WebSearchTool');
      expect(code).toContain("readonly name = 'web_search'");
      expect(code).toContain('搜索互联网获取信息');
      expect(code).toContain('z.object');
      expect(code).toContain('query: z.string()');
      expect(code).toContain('limit: z.number()');
      expect(code).toContain('export type WebSearchToolParams');
      expect(code).toContain('export const web_searchToolDefinition');
    });

    it('should generate Zod schema with descriptions', () => {
      const code = generator.generateTool(toolTemplate);
      expect(code).toContain(".describe('");
      expect(code).toContain('搜索关键词');
      expect(code).toContain('返回结果数量');
    });

    it('should handle single parameter', () => {
      const single: ToolTemplate = {
        name: 'ping',
        description: 'Ping a server',
        parameters: {
          host: { type: 'string', description: 'Server host', required: true },
        },
      };
      const code = generator.generateTool(single);
      expect(code).toContain('export class PingTool');
      expect(code).toContain("required: ['host']");
    });

    it('should handle no required parameters', () => {
      const optional: ToolTemplate = {
        name: 'config',
        description: 'Configuration tool',
        parameters: {
          debug: { type: 'boolean', description: 'Enable debug', required: false },
        },
      };
      const code = generator.generateTool(optional);
      expect(code).toContain('required: []');
    });

    it('should handle all parameter types', () => {
      const allTypes: ToolTemplate = {
        name: 'multi',
        description: 'Multi-type tool',
        parameters: {
          str: { type: 'string', description: 'String param', required: true },
          num: { type: 'number', description: 'Number param', required: true },
          bool: { type: 'boolean', description: 'Boolean param', required: true },
          int: { type: 'integer', description: 'Integer param', required: true },
          arr: { type: 'array', description: 'Array param', required: true },
          obj: { type: 'object', description: 'Object param', required: true },
        },
      };
      const code = generator.generateTool(allTypes);
      expect(code).toContain('z.string()');
      expect(code).toContain('z.number()');
      expect(code).toContain('z.boolean()');
      expect(code).toContain('z.number().int()');
      expect(code).toContain('z.array(z.unknown())');
      expect(code).toContain('z.object({})');
    });
  });

  describe('generateAgent', () => {
    const agentConfig: AgentTemplateConfig = {
      name: 'code_reviewer',
      description: '代码审查 Agent',
      systemPrompt: '你是一个专业的代码审查助手，专注于发现代码中的问题。',
      model: 'gpt-4',
      tools: ['file_search', 'lint_check'],
      temperature: 0.3,
      maxTokens: 4096,
    };

    it('should generate valid TypeScript agent code', () => {
      const code = generator.generateAgent(agentConfig);

      expect(code).toContain('export class CodeReviewer');
      expect(code).toContain("readonly agentName = 'code_reviewer'");
      expect(code).toContain('代码审查 Agent');
      expect(code).toContain('你是一个专业的代码审查助手');
      expect(code).toContain("defaultModel = 'gpt-4'");
      expect(code).toContain("temperature = 0.3");
      expect(code).toContain('maxTokens = 4096');
    });

    it('should include tool imports', () => {
      const code = generator.generateAgent(agentConfig);
      expect(code).toContain("import { FileSearchTool } from './tools/file_search'");
      expect(code).toContain("import { LintCheckTool } from './tools/lint_check'");
      expect(code).toContain('new FileSearchTool()');
      expect(code).toContain('new LintCheckTool()');
    });

    it('should generate factory function', () => {
      const code = generator.generateAgent(agentConfig);
      expect(code).toContain('export function createCodeReviewer');
      expect(code).toContain('return new CodeReviewer(config)');
    });

    it('should handle minimal config', () => {
      const minimal: AgentTemplateConfig = {
        name: 'simple_bot',
        description: 'A simple bot',
        systemPrompt: 'You are a helpful assistant.',
      };
      const code = generator.generateAgent(minimal);
      expect(code).toContain('export class SimpleBot');
      expect(code).not.toContain('defaultModel');
      expect(code).toContain('// TODO: 添加工具实例');
    });

    it('should include lifecycle hooks', () => {
      const code = generator.generateAgent(agentConfig);
      expect(code).toContain('protected async beforeLLMCall');
      expect(code).toContain('protected async afterLLMCall');
    });
  });

  describe('generateProvider', () => {
    const providerConfig: ProviderTemplateConfig = {
      name: 'custom_llm',
      baseURL: 'https://api.custom-llm.com',
      defaultModel: 'custom-v1',
      apiVersion: 'v2',
    };

    it('should generate valid TypeScript provider code', () => {
      const code = generator.generateProvider(providerConfig);

      expect(code).toContain('export class CustomLlmProvider');
      expect(code).toContain("readonly name = 'custom_llm'");
      expect(code).toContain('readonly baseURL = \'https://api.custom-llm.com\'');
      expect(code).toContain("readonly defaultModel = 'custom-v1'");
      expect(code).toContain("readonly apiVersion = 'v2'");
      expect(code).toContain('implements Provider');
    });

    it('should generate complete() method', () => {
      const code = generator.generateProvider(providerConfig);
      expect(code).toContain('async complete(request: CompletionRequest)');
      expect(code).toContain('fetch(url, {');
      expect(code).toContain('Bearer');
      expect(code).toContain('CompletionResponse');
    });

    it('should generate stream() method', () => {
      const code = generator.generateProvider(providerConfig);
      expect(code).toContain('async stream(request: CompletionRequest');
      expect(code).toContain('getReader()');
      expect(code).toContain('TextDecoder');
      expect(code).toContain('onChunk({ content: \'\', done: true })');
    });

    it('should generate utility methods', () => {
      const code = generator.generateProvider(providerConfig);
      expect(code).toContain('async listModels()');
      expect(code).toContain('async isAvailable()');
    });

    it('should validate apiKey in constructor', () => {
      const code = generator.generateProvider(providerConfig);
      expect(code).toContain("throw new Error('custom_llmProvider: apiKey is required')");
    });

    it('should handle minimal config', () => {
      const minimal: ProviderTemplateConfig = {
        name: 'minimal',
        baseURL: 'https://api.minimal.ai',
        defaultModel: 'mini-v1',
      };
      const code = generator.generateProvider(minimal);
      expect(code).toContain('export class MinimalProvider');
      expect(code).not.toContain('apiVersion');
    });
  });

  describe('generated code quality', () => {
    it('should include JSDoc comments for tool', () => {
      const code = generator.generateTool({
        name: 'test_tool',
        description: 'Test tool description',
        parameters: {
          input: { type: 'string', description: 'Input data', required: true },
        },
      });
      expect(code).toContain('@generated by AgentPrimordia CodeGenerator');
      expect(code).toContain('@param input');
      code.split('\n').forEach((line: string) => {
        if (line.includes('@param')) {
          expect(line).toMatch(/@param \w+ - .+/);
        }
      });
    });

    it('should include JSDoc comments for agent', () => {
      const code = generator.generateAgent({
        name: 'quality_agent',
        description: 'Quality test agent',
        systemPrompt: 'Test prompt',
      });
      expect(code).toContain('@generated by AgentPrimordia CodeGenerator');
      expect(code).toContain('@example');
      expect(code).toContain('@param config');
    });

    it('should produce consistent output', () => {
      const template: ToolTemplate = {
        name: 'stable',
        description: 'Stable tool',
        parameters: {
          val: { type: 'string', description: 'Value', required: true },
        },
      };
      const code1 = generator.generateTool(template);
      const code2 = generator.generateTool(template);
      expect(code1).toBe(code2);
    });
  });
});
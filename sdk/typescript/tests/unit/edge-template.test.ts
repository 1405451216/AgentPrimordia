/**
 * Edge Agent 模板测试
 *
 * 测试 createCloudflareAgentHandler / generateWranglerConfig / generateEntryFile 等功能
 */

import { describe, it, expect } from 'vitest';
import {
  createCloudflareAgentHandler,
  createDenoAgentHandler,
  createBunAgentHandler,
  createEdgeAgentHandler,
  generateWranglerConfig,
  generateEntryFile,
  generatePackageJSON,
  generateTSConfig,
  type ScaffoldConfig,
} from '../../src/edge/template.js';

// 创建一个 mock provider 用于测试
function createMockProvider() {
  return {
    complete: async () => ({
      id: 'test-id',
      content: 'Mock response',
      role: 'assistant' as const,
      usage: { promptTokens: 10, completionTokens: 5, totalTokens: 15 },
    }),
    stream: async function* () {
      yield { content: 'Mock', done: false };
      yield { content: ' response', done: true };
    },
  };
}

describe('EdgeAgentTemplate', () => {
  describe('createCloudflareAgentHandler', () => {
    it('should create a handler function', () => {
      const handler = createCloudflareAgentHandler({
        name: 'test-agent',
        provider: createMockProvider() as any,
        systemPrompt: 'You are helpful',
      });
      expect(typeof handler).toBe('function');
    });

    it('should handle health check requests', async () => {
      const handler = createCloudflareAgentHandler({
        name: 'test-agent',
        provider: createMockProvider() as any,
        enableHealth: true,
      });

      const request = new Request('http://localhost/health', {
        method: 'GET',
      });
      const response = await handler(request);

      expect(response.status).toBe(200);
      const body = await response.json();
      expect(body.healthy).toBeDefined();
      expect(typeof body.totalRequests).toBe('number');
    });

    it('should return 404 for unknown routes', async () => {
      const handler = createCloudflareAgentHandler({
        name: 'test-agent',
        provider: createMockProvider() as any,
      });

      const request = new Request('http://localhost/unknown', {
        method: 'GET',
      });
      const response = await handler(request);

      expect(response.status).toBe(404);
    });
  });

  describe('createDenoAgentHandler', () => {
    it('should create a handler function', () => {
      const handler = createDenoAgentHandler({
        name: 'deno-agent',
        provider: createMockProvider() as any,
      });
      expect(typeof handler).toBe('function');
    });
  });

  describe('createBunAgentHandler', () => {
    it('should create a handler function', () => {
      const handler = createBunAgentHandler({
        name: 'bun-agent',
        provider: createMockProvider() as any,
      });
      expect(typeof handler).toBe('function');
    });
  });

  describe('createEdgeAgentHandler', () => {
    it('should create a runtime-agnostic handler', async () => {
      const handler = createEdgeAgentHandler({
        name: 'edge-agent',
        provider: createMockProvider() as any,
      });
      expect(typeof handler).toBe('function');

      // 验证健康检查端点正常工作
      const request = new Request('http://localhost/health', {
        method: 'GET',
      });
      const response = await handler(request);
      expect(response.status).toBe(200);
    });
  });

  describe('generateWranglerConfig', () => {
    it('should generate valid wrangler.toml', () => {
      const config = generateWranglerConfig({
        name: 'my-agent',
      });

      expect(config).toContain('name = "my-agent"');
      expect(config).toContain('main = "src/index.ts"');
      expect(config).toContain('compatibility_date');
    });

    it('should support custom compatibility date', () => {
      const config = generateWranglerConfig({
        name: 'my-agent',
        compatibilityDate: '2024-01-01',
      });

      expect(config).toContain('compatibility_date = "2024-01-01"');
    });

    it('should support observability toggle', () => {
      const config = generateWranglerConfig({
        name: 'my-agent',
        observability: false,
      });

      expect(config).toContain('enabled = false');
    });
  });

  describe('generateEntryFile', () => {
    it('should generate valid TypeScript entry file', () => {
      const config: ScaffoldConfig = {
        projectName: 'my-edge-agent',
        platform: 'cloudflare',
      };

      const entry = generateEntryFile(config);

      expect(entry).toContain('my-edge-agent');
      expect(entry).toContain('createEdgeAgentHandler');
      expect(entry).toContain('createProvider');
      expect(entry).toContain('export default');
    });

    it('should use custom agent name and system prompt', () => {
      const entry = generateEntryFile({
        projectName: 'test',
        platform: 'cloudflare',
        agentName: 'custom-agent',
        systemPrompt: 'Custom prompt',
      });

      expect(entry).toContain('custom-agent');
      expect(entry).toContain('Custom prompt');
    });

    it('should support different providers', () => {
      const entry = generateEntryFile({
        projectName: 'test',
        platform: 'cloudflare',
        provider: 'anthropic',
      });

      expect(entry).toContain('anthropic');
    });
  });

  describe('generatePackageJSON', () => {
    it('should generate valid package.json', () => {
      const pkg = generatePackageJSON({
        projectName: 'my-agent',
        platform: 'cloudflare',
      });

      const parsed = JSON.parse(pkg);
      expect(parsed.name).toBe('my-agent');
      expect(parsed.type).toBe('module');
      expect(parsed.scripts.dev).toBeDefined();
      expect(parsed.dependencies['@agentprimordia/sdk']).toBeDefined();
    });
  });

  describe('generateTSConfig', () => {
    it('should generate valid tsconfig.json', () => {
      const tsconfig = generateTSConfig();

      const parsed = JSON.parse(tsconfig);
      expect(parsed.compilerOptions.target).toBe('ES2022');
      expect(parsed.compilerOptions.strict).toBe(true);
      expect(parsed.include).toContain('src/**/*.ts');
    });
  });
});

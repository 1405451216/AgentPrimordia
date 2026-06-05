import { describe, it, expect, beforeAll, afterAll } from 'vitest';
import http from 'http';
import { OpenAIProvider, MockProvider, ResilientProvider, ToolRegistry, ReActAgent } from '../../src/index.js';

let sharedServer: http.Server;
const MOCK_PORT = 9876;

function startMockServer(port: number): Promise<http.Server> {
  return new Promise((resolve) => {
    const server = http.createServer((req, res) => {
      let body = '';
      req.on('data', (chunk: Buffer) => { body += chunk.toString(); });
      req.on('end', () => {
        res.writeHead(200, { 'Content-Type': 'application/json' });

        if (req.url?.includes('/chat/completions')) {
          const parsed = JSON.parse(body || '{}');
          const hasTools = parsed.tools && parsed.tools.length > 0;

          if (hasTools) {
            res.end(JSON.stringify({
              id: 'mock-id',
              object: 'chat.completion',
              choices: [{
                message: {
                  role: 'assistant',
                  content: null,
                  tool_calls: [{
                    id: 'call_1',
                    type: 'function',
                    function: { name: parsed.tools[0].function.name, arguments: '{}' },
                  }],
                },
                finish_reason: 'tool_calls',
              }],
              usage: { prompt_tokens: 10, completion_tokens: 20, total_tokens: 30 },
            }));
          } else {
            res.end(JSON.stringify({
              id: 'mock-id',
              object: 'chat.completion',
              choices: [{
                message: { role: 'assistant', content: 'Mock response from server' },
                finish_reason: 'stop',
              }],
              usage: { prompt_tokens: 5, completion_tokens: 10, total_tokens: 15 },
            }));
          }
        } else {
          res.end(JSON.stringify({ error: 'not found' }));
        }
      });
    });

    server.listen(port, () => resolve(server));
  });
}

beforeAll(async () => {
  sharedServer = await startMockServer(MOCK_PORT);
}, 10000);

afterAll(() => {
  sharedServer.close();
});

describe('Integration: OpenAI Provider with Mock HTTP Server', () => {
  it('completes a request against mock server', async () => {
    const provider = new OpenAIProvider({
      apiKey: 'test-key',
      baseURL: `http://localhost:${MOCK_PORT}/v1`,
      model: 'gpt-4o',
    });

    const result = await provider.complete({
      messages: [{ role: 'user', content: 'Hello' }],
      model: 'gpt-4o',
    });

    expect(result.content).toBe('Mock response from server');
    expect(result.role).toBe('assistant');
  });

  it('handles tool calls against mock server', async () => {
    const provider = new OpenAIProvider({
      apiKey: 'test-key',
      baseURL: `http://localhost:${MOCK_PORT}/v1`,
      model: 'gpt-4o',
    });

    const result = await provider.callTools({
      messages: [{ role: 'user', content: 'Use a tool' }],
      tools: [{
        type: 'function',
        function: { name: 'test_tool', description: 'A test tool', parameters: {} },
      }],
      model: 'gpt-4o',
    });

    expect(result.toolCalls).toBeDefined();
    expect(result.toolCalls!.length).toBeGreaterThan(0);
    expect(result.toolCalls![0].name).toBe('test_tool');
  });
});

describe('Integration: ReActAgent with MockProvider', () => {
  it('runs full ReAct loop', async () => {
    const registry = new ToolRegistry();
    const provider = new MockProvider({ response: 'Agent completed!' });

    const agent = new ReActAgent({
      name: 'integration-agent',
      model: provider,
      toolkit: registry,
      maxTurns: 1,
    });

    const result = await agent.run('Hello integration test');
    expect(result.content).toBe('Agent completed!');
    expect(result.metrics).toBeDefined();
    expect(result.metrics.totalTurns).toBeGreaterThanOrEqual(1);
  });

  it('runs agent with tools using mock provider', async () => {
    const provider = new MockProvider({
      response: 'Used the tool',
      toolCalls: [{
        id: 'call_1',
        name: 'echo',
        arguments: { msg: 'hello' },
      }],
    });

    const registry = new ToolRegistry();
    registry.register({
      name: 'echo',
      description: 'Echo tool',
      parameters: { type: 'object', properties: { msg: { type: 'string' } }, required: ['msg'] },
      execute: async (args: Record<string, unknown>) => `Echo: ${args.msg}`,
    });

    const agent = new ReActAgent({
      name: 'tool-agent',
      model: provider,
      toolkit: registry,
      maxTurns: 3,
    });

    const result = await agent.run('Use echo tool');
    expect(result.content).toBeDefined();
  });
});

describe('Integration: ResilientProvider fallback', () => {
  it('falls back when primary fails', async () => {
    const primary = new MockProvider({ error: new Error('Primary failed') });
    const fallback = new MockProvider({ response: 'Fallback worked!' });

    const resilient = new ResilientProvider(primary, { maxRetries: 0 });
    resilient.addFallback(fallback);

    const result = await resilient.complete({
      messages: [{ role: 'user', content: 'Test fallback' }],
      model: 'gpt-4o',
    });

    expect(result.content).toBe('Fallback worked!');
  });
});

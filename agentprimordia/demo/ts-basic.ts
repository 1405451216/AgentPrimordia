import {
  ReActAgent,
  MockProvider,
  ToolRegistry,
  HookManager,
  InMemoryStore,
  VectorStore,
  Bus,
  ACL,
  Sandbox,
  MetricsCollector,
  VERSION,
  ErrorCodes,
} from '@agentprimordia/sdk';

async function main() {
  console.log('=== AgentPrimordia TypeScript SDK: Docker Demo ===\n');
  console.log(`SDK Version: ${VERSION}\n`);

  const provider = new MockProvider({
    content: 'Hello from AgentPrimordia!',
    role: 'assistant',
  });

  const registry = new ToolRegistry();
  registry.register({
    name: 'echo',
    description: 'Echo back the input',
    parameters: {
      type: 'object',
      properties: {
        text: { type: 'string', description: 'Text to echo' },
      },
      required: ['text'],
    },
    execute: async (args: Record<string, unknown>) => {
      return { result: args.text };
    },
  });

  const hooks = new HookManager();
  hooks.register('before_run', (ctx) => {
    console.log(`[Hook] Agent starting: ${ctx.agentID}`);
  });
  hooks.register('on_complete', (ctx) => {
    console.log(`[Hook] Agent completed: ${ctx.agentID}`);
  });

  const agent = new ReActAgent({
    name: 'DockerAgent',
    systemPrompt: 'You are a helpful assistant running in Docker',
    model: provider,
    toolkit: registry,
    maxTurns: 3,
    hooks,
  });

  const response = await agent.run('Hello! Can you help me?');
  console.log(`Response: ${response.content}`);
  console.log(`Turns: ${response.metrics.totalTurns}`);

  const memory = new InMemoryStore();
  await memory.add({
    id: 'ep-1',
    sessionId: 'docker-session',
    role: 'user',
    content: 'Hello from Docker',
    summary: 'Docker greeting',
  });

  const results = await memory.search('Docker');
  console.log(`\nMemory search: ${results.length} result(s)`);

  const vectorStore = new VectorStore(3);
  await vectorStore.add('vec-1', [1, 0, 0], { label: 'x-axis' });
  const searchResults = await vectorStore.search([1, 0.1, 0], 1);
  console.log(`Vector search: score=${searchResults[0]?.score.toFixed(3)}`);

  const acl = new ACL();
  acl.allow('agent-1', '/data', 'write');
  console.log(`\nACL: agent-1 can write /data = ${acl.check('agent-1', '/data', 'write')}`);

  const sandbox = new Sandbox(acl);
  sandbox.blockCommand('rm');
  console.log(`Sandbox: rm blocked = ${sandbox.canExecute('agent-1', 'rm') !== null}`);

  console.log(`\nError code: AGENT_STOPPED = ${ErrorCodes.AGENT_STOPPED}`);
  console.log(`Error code: PATH_TRAVERSAL = ${ErrorCodes.PATH_TRAVERSAL}`);

  console.log('\n--- Docker demo complete ---');
}

main().catch(console.error);

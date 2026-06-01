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
} from '../src/index.js';

async function main() {
  console.log('=== AgentPrimordia TypeScript SDK: 基础示例 ===\n');
  console.log(`SDK Version: ${VERSION}\n`);

  const provider = new MockProvider({
    content: '你好！我是 AgentPrimordia 助手。',
    role: 'assistant',
  });

  const registry = new ToolRegistry();
  registry.register({
    name: 'calculator',
    description: '执行数学计算',
    parameters: {
      type: 'object',
      properties: {
        expression: { type: 'string', description: '数学表达式' },
      },
      required: ['expression'],
    },
    execute: async (args: Record<string, unknown>) => {
      return { result: `计算结果: ${args.expression}` };
    },
  });

  const hooks = new HookManager();
  hooks.register('before_run', (ctx) => {
    console.log(`[Hook] Agent 开始运行: ${ctx.agentID}`);
  });
  hooks.register('after_run', (ctx) => {
    console.log(`[Hook] Agent 运行完成: ${ctx.agentID}`);
  });

  const agent = new ReActAgent({
    name: 'DemoAgent',
    systemPrompt: '你是一个友好的助手',
    model: provider,
    toolkit: registry,
    maxTurns: 3,
    hooks,
  });

  const response = await agent.run('你好！请帮我计算 2+2');
  console.log(`回复: ${response.content}`);
  console.log(`轮数: ${response.metrics.totalTurns}`);

  const memory = new InMemoryStore();
  await memory.add({
    id: 'ep-1',
    sessionId: 'session-1',
    role: 'user',
    content: '你好',
    summary: '用户打招呼',
  });

  const results = await memory.search('你好');
  console.log(`\n记忆搜索结果: ${results.length} 条`);

  const vectorStore = new VectorStore(3);
  await vectorStore.add('vec-1', [1, 0, 0], { label: 'x-axis' });
  const searchResults = await vectorStore.search([1, 0.1, 0], 1);
  console.log(`向量搜索: 找到 ${searchResults.length} 条, 最高分 ${searchResults[0]?.score.toFixed(3)}`);

  const bus = new Bus(16);
  let receivedEvent: string | undefined;
  bus.subscribe('agent.start', (event) => {
    receivedEvent = event.type;
  });
  bus.publish({ id: 'evt-1', type: 'agent.start', source: 'demo', timestamp: new Date(), payload: {} });
  console.log(`\n事件总线: 收到事件类型 = ${receivedEvent}`);

  const acl = new ACL();
  acl.allow('agent-1', '/data', 'write');
  console.log(`ACL 检查: agent-1 访问 /data = ${acl.check('agent-1', '/data', 'write')}`);

  const sandbox = new Sandbox(acl);
  sandbox.allowCommand('ls');
  sandbox.allowCommand('cat');
  sandbox.blockCommand('rm');
  const cmdErr = sandbox.canExecute('agent-1', 'rm');
  console.log(`Sandbox: rm 命令 = ${cmdErr ? '被阻止' : '允许'}`);

  const metrics = new MetricsCollector();
  metrics.recordLLMCall(150, false);
  metrics.recordToolCall(50, false);
  const snapshot = metrics.snapshot();
  console.log(`\n指标: LLM调用=${snapshot.llm_total_calls}, 工具调用=${snapshot.tool_total_calls}`);

  console.log(`\n错误码示例: AGENT_STOPPED = ${ErrorCodes.AGENT_STOPPED}`);
  console.log(`错误码示例: PATH_TRAVERSAL = ${ErrorCodes.PATH_TRAVERSAL}`);

  console.log('\n--- TypeScript SDK 示例运行完成 ---');
}

main().catch(console.error);

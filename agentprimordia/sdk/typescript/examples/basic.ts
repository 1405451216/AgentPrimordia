import { MockProvider, ReActAgent, Pool, VERSION } from "../src/index.js";

async function basicExample(): Promise<void> {
  console.log("=== AgentPrimordia: TypeScript 基础示例 ===\n");

  const mockLLM = new MockProvider("你好！我是 AgentPrimordia 助手，有什么可以帮你的？");

  const agent = new ReActAgent({
    name: "BasicAgent",
    systemPrompt: "你是一个友好的助手",
    model: mockLLM,
    maxTurns: 3,
  });

  const result = await agent.run("你好！");
  console.log(`回复: ${result.content}`);
  console.log(`轮数: ${result.turns}`);
  console.log(`版本: ${VERSION}`);
}

async function multiAgentExample(): Promise<void> {
  console.log("\n=== AgentPrimordia: TypeScript 多 Agent 调度示例 ===\n");

  const pool = new Pool({
    maxConcurrency: 3,
    defaultAgent: {
      name: "DefaultAgent",
      systemPrompt: "你是一个任务处理助手",
      maxTurns: 3,
    },
  });

  pool.setModel(new MockProvider());

  const tasks = [
    { id: "task-1", title: "数据分析", prompt: "分析销售数据趋势" },
    { id: "task-2", title: "报告生成", prompt: "生成月度报告" },
    { id: "task-3", title: "邮件撰写", prompt: "撰写客户跟进邮件" },
  ];

  const results = await pool.dispatch(tasks);

  for (const r of results) {
    const status = r.error ? r.error.message : "成功";
    console.log(`任务 [${r.taskID}] ${r.task.title}: ${status} (耗时 ${r.duration}ms)`);
  }

  pool.close();
}

async function main(): Promise<void> {
  await basicExample();
  await multiAgentExample();
}

main().catch(console.error);

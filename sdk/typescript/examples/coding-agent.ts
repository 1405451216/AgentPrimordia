/**
 * coding-agent — 一体化 Coding Harness 示例（TS 线，与 Go 线 coding-agent 镜像）
 *
 * 单个 Agent 端到端打通 计划 → 编写 → 实施 → 测试 → 审查 → 发布 全流程：
 * - planner（LLMPlanner）：run 入口分解为 编写→实施→测试→审查→发布 的 DAG
 * - filesystem/shell 内置工具：编写、实施（运行代码验证）与测试
 * - reflector（LLMReflector）：每个子任务完成路径批评，severity 达阈值才改写
 * - 发布：shell 驱动 git add/commit/tag
 *
 * 运行（脚本化 MockProvider 演示，无需 API Key）:
 *   npx tsx examples/coding-agent.ts
 *
 * 真实使用时把 ScriptedProvider 替换为 OpenAIProvider 等真实 Provider，
 * harness 装配与流程完全不变。
 */
import * as fs from 'node:fs';
import * as os from 'node:os';
import * as path from 'node:path';
import { execFileSync } from 'node:child_process';
import { ReActAgent } from '../src/agent/react-loop.js';
import { MockProvider } from '../src/llm/provider.js';
import { ToolRegistry } from '../src/tools/registry.js';
import { FileSystemTool } from '../src/tools/builtin/filesystem.js';
import { ShellTool } from '../src/tools/builtin/shell.js';
import { LLMPlanner } from '../src/agent/planning.js';
import { LLMReflector } from '../src/agent/reflection.js';
import type { ToolCall, CompletionRequest, ToolCallRequest, CompletionResponse, ToolCallResponse } from '../src/types.js';

// ===== 脚本化 Provider：Complete 与 CallTools 双队列 =====

class ScriptedProvider extends MockProvider {
  private completeQueue: string[] = [];
  private toolCallQueue: { content: string; toolCalls: ToolCall[] }[] = [];

  withResponse(content: string): this {
    this.completeQueue.push(content);
    return this;
  }

  withToolResponse(toolCalls: ToolCall[], content = ''): this {
    this.toolCallQueue.push({ content, toolCalls });
    return this;
  }

  override async complete(_req: CompletionRequest): Promise<CompletionResponse> {
    const content = this.completeQueue.shift() ?? 'default complete';
    return {
      id: 'scripted', content, role: 'assistant',
      usage: { promptTokens: 1, completionTokens: 1, totalTokens: 2 },
    };
  }

  override async callTools(_req: ToolCallRequest): Promise<ToolCallResponse> {
    const next = this.toolCallQueue.shift() ?? { content: 'fallback', toolCalls: [] };
    return {
      content: next.content, toolCalls: next.toolCalls,
      usage: { promptTokens: 1, completionTokens: 1, totalTokens: 2 },
    };
  }
}

// 计划协议：JSON 数组 [{id, description, depends_on}]
const pipelinePlan = JSON.stringify([
  { id: '1', description: '编写：创建 hello.ts 文件', depends_on: [] },
  { id: '2', description: '实施：运行 node hello.ts 验证可执行', depends_on: ['1'] },
  { id: '3', description: '测试：检查工作区确认文件已生成', depends_on: ['2'] },
  { id: '4', description: '审查：评估代码质量并给出结论', depends_on: ['3'] },
  { id: '5', description: '发布：git 提交并打标签 v1.0.0', depends_on: ['4'] },
]);

// 批评协议：severity low 低于阈值 high，不触发改写
const lowCritique = JSON.stringify({ issues: [], severity: 'low', corrections: [] });

function git(workdir: string, ...args: string[]): string {
  return execFileSync('git', args, { cwd: workdir, encoding: 'utf-8' });
}

async function main() {
  console.log('=== AgentPrimordia TS SDK: Coding Harness Example ===\n');

  // 1. 准备临时工作区（真实场景替换为项目目录）
  const workdir = fs.mkdtempSync(path.join(os.tmpdir(), 'coding-agent-'));
  console.log(`工作区: ${workdir}`);
  git(workdir, 'init', '-b', 'main');
  git(workdir, 'config', 'user.email', 'coding-agent@example.com');
  git(workdir, 'config', 'user.name', 'coding-agent');

  try {
    // 2. 脚本化 LLM（真实场景替换为真实 Provider）
    const provider = scriptLLM(workdir);

    // 3. 工具注册：编写(filesystem) + 测试/发布(shell)
    const registry = new ToolRegistry();
    registry.register(new FileSystemTool({ rootDir: workdir }));
    registry.register(new ShellTool({ workingDir: workdir }));

    // 4. 装配一体化 harness：认知（Planner+Reflector）+ 工具
    const agent = new ReActAgent({
      name: 'coding-agent',
      model: provider,
      toolkit: registry,
      systemPrompt: '你是一个全自动编码 Agent，按计划完成编写、测试、审查与发布。',
      maxTurns: 8,
      planner: new LLMPlanner(provider),
      reflector: new LLMReflector(provider),
      reflectionSeverityThreshold: 'high',
    });

    const goal = '创建 hello.ts，运行验证，检查工作区，审查代码，然后提交并打标签 v1.0.0';
    console.log(`目标: ${goal}\n`);

    const resp = await agent.run(goal);

    // 5. 输出结果与产物校验
    console.log(`最终结论: ${resp.content}`);
    console.log(`消耗轮次: ${resp.metrics.totalTurns} | 工具调用: ${resp.metrics.totalTools}\n`);

    console.log('产物 hello.ts:');
    console.log(fs.readFileSync(path.join(workdir, 'hello.ts'), 'utf-8').trim());

    // 实施环节校验：程序真实可运行
    console.log(`\nnode 运行输出: ${execFileSync('node', ['hello.ts'], { cwd: workdir, encoding: 'utf-8' }).trim()}`);

    console.log(`\ngit log: ${git(workdir, 'log', '--oneline').trim()}`);
    console.log(`git tag: ${git(workdir, 'tag', '-l').trim()}`);
    console.log('\n--- Done ---');
  } finally {
    fs.rmSync(workdir, { recursive: true, force: true });
  }
}

// scriptLLM 脚本化模拟真实 LLM 的全流程决策。
// 队列时序（每个子任务）：
// - CallTools 队列：若干 tool 调用轮 + 末尾空 toolCalls 携带结论
// - Complete 队列：Planner 根计划 + 每个子任务的 Critique
function scriptLLM(workdir: string): ScriptedProvider {
  const p = new ScriptedProvider();
  p.withResponse(pipelinePlan); // 根任务 Planning：分解为 5 个子任务

  const fileContent =
    "console.log('Hello from AgentPrimordia TS coding harness!');\n" +
    "export const hello = (): string => 'Hello from AgentPrimordia TS coding harness!';\n";

  // 子任务1：编写（filesystem.write）
  p.withToolResponse([{
    id: 'call_write', name: 'filesystem',
    arguments: JSON.stringify({ action: 'write', path: 'hello.ts', content: fileContent }),
  }])
    .withToolResponse([], 'hello.ts 已创建')
    .withResponse(lowCritique);

  // 子任务2：实施（shell 实际运行程序）
  p.withToolResponse([{
    id: 'call_run', name: 'shell',
    arguments: JSON.stringify({ command: 'node hello.ts', cwd: workdir }),
  }])
    .withToolResponse([], '运行成功：Hello from AgentPrimordia TS coding harness!')
    .withResponse(lowCritique);

  // 子任务3：测试（shell 校验工作区）
  p.withToolResponse([{
    id: 'call_check', name: 'shell',
    arguments: JSON.stringify({ command: 'git status --short', cwd: workdir }),
  }])
    .withToolResponse([], '工作区检查通过：hello.ts 已生成')
    .withResponse(lowCritique);

  // 子任务4：审查（纯推理，无工具）
  p.withToolResponse([], '审查通过：代码结构清晰，无高严重度问题')
    .withResponse(lowCritique);

  // 子任务5：发布（git add+commit 同轮双调用，再 tag）
  p.withToolResponse([
    { id: 'call_add', name: 'shell', arguments: JSON.stringify({ command: 'git add .', cwd: workdir }) },
    {
      id: 'call_commit', name: 'shell',
      arguments: JSON.stringify({ command: 'git commit -m "feat: add hello.ts"', cwd: workdir }),
    },
  ])
    .withToolResponse([{
      id: 'call_tag', name: 'shell',
      arguments: JSON.stringify({ command: 'git tag v1.0.0', cwd: workdir }),
    }])
    .withToolResponse([], '发布完成：v1.0.0')
    .withResponse(lowCritique);

  return p;
}

main().catch(console.error);

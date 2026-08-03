/**
 * TS 线一体化 coding harness 端到端测试
 *
 * 与 Go 线 test/e2e/coding_pipeline_test.go 镜像：
 * 计划（LLMPlanner 分解 DAG）→ 编写（filesystem 写文件）→ 测试（shell 校验）
 * → 审查（LLMReflector 批评）→ 发布（shell 驱动 git add/commit/tag），
 * 全部由脚本化 Provider 驱动，不依赖真实 LLM 与网络。
 */
import { describe, it, expect } from 'vitest';
import * as fs from 'node:fs';
import * as os from 'node:os';
import * as path from 'node:path';
import { execFileSync } from 'node:child_process';
import { ReActAgent } from '../../src/agent/react-loop.js';
import { MockProvider } from '../../src/llm/provider.js';
import { ToolRegistry } from '../../src/tools/registry.js';
import { FileSystemTool } from '../../src/tools/builtin/filesystem.js';
import { ShellTool } from '../../src/tools/builtin/shell.js';
import { LLMPlanner } from '../../src/agent/planning.js';
import { LLMReflector } from '../../src/agent/reflection.js';
import type { ToolCall, CompletionRequest, ToolCallRequest, CompletionResponse, ToolCallResponse } from '../../src/types.js';

// ===== 脚本化 Provider：Complete 与 CallTools 双队列，时序确定 =====

class ScriptedProvider extends MockProvider {
  completeQueue: string[] = [];
  toolCallQueue: { content: string; toolCalls: ToolCall[] }[] = [];

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

// ===== 协议常量 =====

// 根计划：编写 → 测试 → 审查 → 发布 依赖链
const pipelinePlan = JSON.stringify([
  { id: '1', description: '编写：创建 hello.ts 文件', depends_on: [] },
  { id: '2', description: '测试：检查工作区确认文件已生成', depends_on: ['1'] },
  { id: '3', description: '审查：评估代码质量并给出结论', depends_on: ['2'] },
  { id: '4', description: '发布：git 提交并打标签 v1.0.0', depends_on: ['3'] },
]);

// 批评应答：severity low 低于阈值 high，不触发改写
const lowCritique = JSON.stringify({ issues: [], severity: 'low', corrections: [] });

// ===== 辅助 =====

function git(workdir: string, ...args: string[]): string {
  return execFileSync('git', args, { cwd: workdir, encoding: 'utf-8' });
}

function hasGit(): boolean {
  try {
    execFileSync('git', ['--version'], { encoding: 'utf-8' });
    return true;
  } catch {
    return false;
  }
}

describe('TS coding pipeline 端到端', () => {
  it('计划→编写→测试→审查→发布全流程打通', async () => {
    if (!hasGit()) return; // 无 git 环境跳过

    const workdir = fs.mkdtempSync(path.join(os.tmpdir(), 'ts-coding-pipeline-'));
    try {
      git(workdir, 'init', '-b', 'main');
      git(workdir, 'config', 'user.email', 'e2e@example.com');
      git(workdir, 'config', 'user.name', 'e2e-bot');

      const provider = new ScriptedProvider();
      provider.withResponse(pipelinePlan); // 根任务 Planning：分解为 4 个子任务

      const fileContent = 'export const hello = (): string => "Hello from AgentPrimordia TS harness!";\n';

      // 子任务1：编写（filesystem 写文件）
      provider
        .withToolResponse([{
          id: 'call_write', name: 'filesystem',
          arguments: JSON.stringify({ action: 'write', path: 'hello.ts', content: fileContent }),
        }])
        .withToolResponse([], '已创建 hello.ts')
        .withResponse(lowCritique);

      // 子任务2：测试（shell 校验工作区）
      provider
        .withToolResponse([{
          id: 'call_check', name: 'shell',
          arguments: JSON.stringify({ command: 'git status --short', cwd: workdir }),
        }])
        .withToolResponse([], '工作区检查通过')
        .withResponse(lowCritique);

      // 子任务3：审查（无工具，直接结论）
      provider
        .withToolResponse([], '审查通过：代码结构清晰')
        .withResponse(lowCritique);

      // 子任务4：发布（git add + commit 同轮双调用，再 tag）
      provider
        .withToolResponse([
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

      // ===== 装配一体化 harness =====
      const registry = new ToolRegistry();
      registry.register(new FileSystemTool({ rootDir: workdir }));
      registry.register(new ShellTool({ workingDir: workdir }));

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

      const resp = await agent.run('创建 hello.ts，验证工作区，审查代码，然后提交并打标签 v1.0.0');

      // ===== 断言 =====
      expect(resp.content).toBe('发布完成：v1.0.0');
      expect(fs.readFileSync(path.join(workdir, 'hello.ts'), 'utf-8')).toBe(fileContent);
      expect(git(workdir, 'log', '--oneline')).toContain('feat: add hello.ts');
      expect(git(workdir, 'tag', '-l')).toContain('v1.0.0');
      // write + status + add + commit + tag = 5 次工具调用
      expect(resp.metrics.totalTools).toBe(5);
      // 批评队列被 4 个子任务各消费一次
      expect(provider.completeQueue.length).toBe(0);
    } finally {
      fs.rmSync(workdir, { recursive: true, force: true });
    }
  }, 30_000);
});

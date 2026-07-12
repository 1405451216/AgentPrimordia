import type { ReActAgent } from '../agent/react-loop.js';
import type { Response } from '../types.js';

export interface StepResult {
  stepName: string;
  response: Response;
  skipped: boolean;
}

/** 流式管道事件（用于 streamRun） */
export type PipelineStreamEvent =
  | { type: 'step_start'; stepName: string; index: number }
  | { type: 'step_done'; stepName: string; index: number; response: Response }
  | { type: 'pipeline_done'; results: StepResult[] }
  | { type: 'error'; stepName?: string; index?: number; error: Error };

export interface PipelineStep {
  name: string;
  agent: ReActAgent;
  input: string;
  condition?: (prev: StepResult | null) => boolean;
}

export class Pipeline {
  constructor(private steps: PipelineStep[]) {}

  async run(initialInput?: string): Promise<StepResult[]> {
    const results: StepResult[] = [];
    for (let i = 0; i < this.steps.length; i++) {
      const step = this.steps[i];
      if (step.condition && !step.condition(results[i - 1] ?? null)) {
        results.push({ stepName: step.name, response: { content: '', metrics: { totalTurns: 0, totalTools: 0, duration: 0, llmLatency: 0, toolLatency: 0 } }, skipped: true });
        continue;
      }
      const input = i === 0 && initialInput ? initialInput : step.input;
      const response = await step.agent.run(input);
      results.push({ stepName: step.name, response, skipped: false });
    }
    return results;
  }

  /**
   * 流式执行管道。
   *
   * 与 run() 的区别：streamRun() 在每个 Agent 完成后即 yield 结果事件，
   * 而不是等待所有步骤完成后再返回。
   * 完全的 token 级跨步传递请使用 StreamingPipeline。
   */
  async *streamRun(initialInput?: string): AsyncGenerator<PipelineStreamEvent> {
    const results: StepResult[] = [];
    for (let i = 0; i < this.steps.length; i++) {
      const step = this.steps[i]!;
      yield { type: 'step_start', stepName: step.name, index: i };

      if (step.condition && !step.condition(results[i - 1] ?? null)) {
        const skipped: StepResult = {
          stepName: step.name,
          response: { content: '', metrics: { totalTurns: 0, totalTools: 0, duration: 0, llmLatency: 0, toolLatency: 0 } },
          skipped: true,
        };
        results.push(skipped);
        yield { type: 'step_done', stepName: step.name, index: i, response: skipped.response };
        continue;
      }

      const input = i === 0 && initialInput ? initialInput : step.input;

      try {
        const response = await step.agent.run(input);
        results.push({ stepName: step.name, response, skipped: false });
        yield { type: 'step_done', stepName: step.name, index: i, response };
      } catch (err) {
        const error = err instanceof Error ? err : new Error(String(err));
        yield { type: 'error', stepName: step.name, index: i, error };
        return;
      }
    }

    yield { type: 'pipeline_done', results };
  }
}

export class ParallelRun {
  constructor(private steps: PipelineStep[]) {}
  async run(): Promise<StepResult[]> {
    const promises = this.steps.map(async (step) => {
      const response = await step.agent.run(step.input);
      return { stepName: step.name, response, skipped: false } as StepResult;
    });
    return Promise.all(promises);
  }
}

export class Handoff {
  constructor(private agents: ReActAgent[], private maxRounds: number = 3) {}
  async run(input: string): Promise<StepResult[]> {
    const results: StepResult[] = [];
    let currentInput = input;
    for (let round = 0; round < this.maxRounds; round++) {
      for (const agent of this.agents) {
        const response = await agent.run(currentInput);
        results.push({ stepName: agent.name, response, skipped: false });
        currentInput = response.content;
      }
    }
    return results;
  }
}

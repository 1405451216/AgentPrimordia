import type { ReActAgent } from '../agent/react-loop.js';
import type { Response } from '../types.js';

export interface StepResult {
  stepName: string;
  response: Response;
  skipped: boolean;
}

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

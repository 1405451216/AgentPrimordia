/**
 * Streaming Pipeline - token-level streaming between steps.
 *
 * Difference from regular Pipeline:
 * - Regular Pipeline waits for each Agent to complete before passing to the next step
 * - StreamingPipeline passes output streamingly as tokens are produced
 */
export interface StreamingPipelineStep {
  name: string;
  streamInput?: boolean;
  process(input: string | AsyncIterable<string>): Promise<string | AsyncIterable<string>>;
}

/** @deprecated Use PipelineEvent instead */
export type StreamingPipelineEvent = PipelineEvent;

export type PipelineEvent =
  | { type: 'step_start'; step: string; index: number }
  | { type: 'token'; step: string; index: number; content: string }
  | { type: 'step_done'; step: string; index: number; output: string }
  | { type: 'pipeline_done'; results: string[] }
  | { type: 'error'; step?: string; index?: number; error: Error };

async function* stringToAsyncIterable(s: string): AsyncIterable<string> {
  const chunkSize = 64;
  for (let i = 0; i < s.length; i += chunkSize) {
    yield s.slice(i, Math.min(i + chunkSize, s.length));
  }
}

export class StreamingPipeline {
  private steps: StreamingPipelineStep[] = [];

  addStep(step: StreamingPipelineStep): this {
    this.steps.push(step);
    return this;
  }

  async *run(input: string): AsyncGenerator<PipelineEvent> {
    if (this.steps.length === 0) {
      yield { type: 'pipeline_done', results: [] };
      return;
    }
    const results: string[] = [];
    let currentInput: string = input;
    for (let i = 0; i < this.steps.length; i++) {
      const step = this.steps[i]!;
      yield { type: 'step_start', step: step.name, index: i };
      try {
        let output: string | AsyncIterable<string>;
        if (step.streamInput) {
          output = await step.process(stringToAsyncIterable(currentInput));
        } else {
          output = await step.process(currentInput);
        }
        if (typeof output !== 'string') {
          let fullText = '';
          for await (const chunk of output) {
            fullText += chunk;
            yield { type: 'token', step: step.name, index: i, content: chunk };
          }
          results.push(fullText);
          yield { type: 'step_done', step: step.name, index: i, output: fullText };
          currentInput = fullText;
        } else {
          results.push(output);
          yield { type: 'step_done', step: step.name, index: i, output };
          currentInput = output;
        }
      } catch (err) {
        const error = err instanceof Error ? err : new Error(String(err));
        yield { type: 'error', step: step.name, index: i, error };
        return;
      }
    }
    yield { type: 'pipeline_done', results };
  }

  async runSimple(input: string): Promise<string[]> {
    const results: string[] = [];
    for await (const event of this.run(input)) {
      if (event.type === 'step_done') results.push(event.output);
    }
    return results;
  }
}

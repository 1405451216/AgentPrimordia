/**
 * StreamingPipeline unit tests
 */
import { describe, it, expect } from 'vitest';
import { StreamingPipeline } from '../../src/orchestration/streaming-pipeline.js';
import type { PipelineEvent } from '../../src/orchestration/streaming-pipeline.js';

async function collectEvents(gen: AsyncGenerator<PipelineEvent>): Promise<PipelineEvent[]> {
  const events: PipelineEvent[] = [];
  for await (const e of gen) {
    events.push(e);
  }
  return events;
}

async function collectIterable(iter: AsyncIterable<string>): Promise<string> {
  let result = '';
  for await (const chunk of iter) {
    result += chunk;
  }
  return result;
}

describe('StreamingPipeline', () => {
  it('should execute non-streaming steps sequentially', async () => {
    const pipeline = new StreamingPipeline();
    pipeline
      .addStep({
        name: 'step1',
        process: async (input: string | AsyncIterable<string>) => {
          return `[${input} -> step1]`;
        },
      })
      .addStep({
        name: 'step2',
        process: async (input: string | AsyncIterable<string>) => {
          return `${input} -> step2`;
        },
      });

    const results = await pipeline.runSimple('hello');
    expect(results.length).toBe(2);
    expect(results[0]).toBe('[hello -> step1]');
    expect(results[1]).toBe('[hello -> step1] -> step2');
  });

  it('should emit step_start, step_done, pipeline_done events', async () => {
    const pipeline = new StreamingPipeline();
    pipeline.addStep({
      name: 'only',
      process: async (input) => `processed: ${input}`,
    });

    const events = await collectEvents(pipeline.run('test'));
    expect(events[0]).toMatchObject({ type: 'step_start', step: 'only', index: 0 });
    expect(events[1]).toMatchObject({ type: 'step_done', step: 'only', index: 0, output: 'processed: test' });
    expect(events[2]).toMatchObject({ type: 'pipeline_done', results: ['processed: test'] });
  });

  it('should handle empty pipeline', async () => {
    const pipeline = new StreamingPipeline();
    const events = await collectEvents(pipeline.run('input'));
    expect(events.length).toBe(1);
    expect(events[0]).toEqual({ type: 'pipeline_done', results: [] });
  });

  it('should auto-wrap string output for streaming downstream', async () => {
    const pipeline = new StreamingPipeline();
    pipeline
      .addStep({
        name: 'producer',
        process: async (input) => `output-of(${input})`,
      })
      .addStep({
        name: 'consumer',
        streamInput: true,
        process: async (input) => {
          const chunks: string[] = [];
          for await (const chunk of input as AsyncIterable<string>) {
            chunks.push(chunk);
          }
          return `consumed(${chunks.join(',')})`;
        },
      });

    const results = await pipeline.runSimple('data');
    expect(results.length).toBe(2);
    expect(results[0]).toBe('output-of(data)');
    expect(results[1]).toMatch(/^consumed\(/);
  });

  it('should emit token events from streaming steps', async () => {
    const pipeline = new StreamingPipeline();
    pipeline
      .addStep({
        name: 'uppercaser',
        process: async (input) => input.toUpperCase(),
      })
      .addStep({
        name: 'streamer',
        streamInput: true,
        process: async function* (input) {
          const text = await collectIterable(input as AsyncIterable<string>);
          for (const ch of text) {
            yield ch;
          }
        },
      });

    const events = await collectEvents(pipeline.run('abc'));
    const tokens = events.filter((e) => e.type === 'token');
    expect(tokens.length).toBe(3);
    expect(tokens.map((t) => (t as any).content).join('')).toBe('ABC');
  });

  it('should handle mixed streaming and non-streaming steps', async () => {
    const pipeline = new StreamingPipeline();
    pipeline
      .addStep({
        name: 'text-gen',
        process: async (input) => `text(${input})`,
      })
      .addStep({
        name: 'stream-transform',
        streamInput: true,
        process: async function* (input) {
          const text = await collectIterable(input as AsyncIterable<string>);
          for (const ch of text) {
            yield ch;
          }
        },
      })
      .addStep({
        name: 'final',
        process: async (input) => `final[${input}]`,
      });

    const results = await pipeline.runSimple('hi');
    expect(results.length).toBe(3);
    expect(results[0]).toBe('text(hi)');
    expect(results[2]).toBe('final[' + results[1] + ']');
  });

  it('should handle error in step', async () => {
    const pipeline = new StreamingPipeline();
    pipeline
      .addStep({
        name: 'ok',
        process: async (input) => `ok-${input}`,
      })
      .addStep({
        name: 'fail',
        process: async () => {
          throw new Error('step failure');
        },
      })
      .addStep({
        name: 'never',
        process: async (input) => `never-${input}`,
      });

    const events = await collectEvents(pipeline.run('test'));
    const errorEvent = events.find((e) => e.type === 'error');
    expect(errorEvent).toBeDefined();
    expect((errorEvent as any).error.message).toBe('step failure');
    expect((errorEvent as any).step).toBe('fail');
    const doneEvents = events.filter((e) => e.type === 'step_done');
    expect(doneEvents.length).toBe(1);
  });

  it('should pass AsyncIterable to streamInput step', async () => {
    let receivedType = '';
    const pipeline = new StreamingPipeline();
    pipeline
      .addStep({
        name: 'gen',
        process: async (input) => `gen(${input})`,
      })
      .addStep({
        name: 'receiver',
        streamInput: true,
        process: async (input) => {
          receivedType = typeof input;
          const text = await collectIterable(input as AsyncIterable<string>);
          return `got(${text})`;
        },
      });

    const results = await pipeline.runSimple('x');
    expect(receivedType).toBe('object');
    expect(results[1]).toBe('got(gen(x))');
  });

  it('should collect full output from streaming step for next non-streaming step', async () => {
    const pipeline = new StreamingPipeline();
    pipeline
      .addStep({
        name: 'streamer',
        streamInput: true,
        process: async function* (input) {
          const text = await collectIterable(input as AsyncIterable<string>);
          yield text.toUpperCase();
          yield '!';
        },
      })
      .addStep({
        name: 'collector',
        process: async (input) => `[${input}]`,
      });

    const results = await pipeline.runSimple('hello');
    expect(results.length).toBe(2);
    expect(results[1]).toBe('[HELLO!]');
  });
});

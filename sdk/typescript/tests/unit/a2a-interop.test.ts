import { describe, it, expect } from 'vitest';
import {
  newTextMessage, textContent, isTerminalState, OpenErr,
} from '../../src/a2a/interop.js';

describe('a2a open interop schema (v3.5)', () => {
  it('builds and reads text message', () => {
    const m = newTextMessage('user', 'hello');
    expect(m.role).toBe('user');
    expect(textContent(m)).toBe('hello');
  });

  it('textContent empty when no text part', () => {
    expect(textContent({ role: 'agent', parts: [{ type: 'data', data: { x: 1 } }] })).toBe('');
  });

  it('terminal states align with open spec', () => {
    expect(isTerminalState('completed')).toBe(true);
    expect(isTerminalState('failed')).toBe(true);
    expect(isTerminalState('canceled')).toBe(true);
    expect(isTerminalState('working')).toBe(false);
    expect(isTerminalState('submitted')).toBe(false);
  });

  it('standard error codes match open spec', () => {
    expect(OpenErr.ParseError).toBe(-32700);
    expect(OpenErr.InvalidRequest).toBe(-32600);
    expect(OpenErr.MethodNotFound).toBe(-32601);
    expect(OpenErr.InvalidParams).toBe(-32602);
    expect(OpenErr.Internal).toBe(-32603);
    expect(OpenErr.TaskNotFound).toBe(-32001);
  });
});

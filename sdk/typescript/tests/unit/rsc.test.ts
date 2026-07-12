/**
 * RSC (React Server Components) ��Ԫ����
 *
 * ���Բ��ԣ�
 * - StreamView����֤ ReadableStream �����߼��������� React DOM
 * - useAgent����֤ hook ״̬�����߼����� mock React hooks��
 */

import { describe, it, expect, vi } from 'vitest';
import { StreamView } from '../../src/react/StreamView.js';

describe('StreamView', () => {
  it('should consume a stream', async () => {
    const stream = new ReadableStream({
      start(controller) {
        controller.enqueue('hello');
        controller.enqueue(' ');
        controller.enqueue('world');
        controller.close();
      }
    });
    const tokens: string[] = [];
    const result = await StreamView({ stream, onToken: t => tokens.push(t) });
    expect(result).toBe('hello world');
    expect(tokens).toEqual(['hello', ' ', 'world']);
  });

  it('should handle empty stream', async () => {
    const stream = new ReadableStream({
      start(controller) {
        controller.close();
      }
    });
    const tokens: string[] = [];
    const result = await StreamView({ stream, onToken: t => tokens.push(t) });
    expect(result).toBe('');
    expect(tokens).toEqual([]);
  });

  it('should handle single chunk', async () => {
    const stream = new ReadableStream({
      start(controller) {
        controller.enqueue('single');
        controller.close();
      }
    });
    const result = await StreamView({ stream });
    expect(result).toBe('single');
  });

  it('should call onToken for each chunk', async () => {
    const stream = new ReadableStream({
      start(controller) {
        controller.enqueue('a');
        controller.enqueue('b');
        controller.enqueue('c');
        controller.close();
      }
    });
    const onToken = vi.fn();
    await StreamView({ stream, onToken });
    expect(onToken).toHaveBeenCalledTimes(3);
    expect(onToken).toHaveBeenNthCalledWith(1, 'a');
    expect(onToken).toHaveBeenNthCalledWith(2, 'b');
    expect(onToken).toHaveBeenNthCalledWith(3, 'c');
  });

  it('should work without onToken callback', async () => {
    const stream = new ReadableStream({
      start(controller) {
        controller.enqueue('no callback');
        controller.close();
      }
    });
    const result = await StreamView({ stream });
    expect(result).toBe('no callback');
  });

  it('should handle multi-byte characters', async () => {
    const stream = new ReadableStream({
      start(controller) {
        controller.enqueue('���');
        controller.enqueue('����');
        controller.close();
      }
    });
    const tokens: string[] = [];
    const result = await StreamView({ stream, onToken: t => tokens.push(t) });
    expect(result).toBe('�������');
    expect(tokens).toEqual(['���', '����']);
  });
});

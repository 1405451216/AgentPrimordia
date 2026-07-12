/**
 * StreamView �� ��ʽ��� React ���
 *
 * ���� ReadableStream<string>���� token �ص� onToken��
 * ���շ��������ı���
 *
 * ʹ�÷�ʽ��
 *   const text = await StreamView({ stream, onToken: t => setText(prev => prev + t) });
 *
 * ���Ҫ�㣺
 * - ���첽����������� hook�������� Server Component ��ʹ��
 * - ֧�� onToken �ص������� token ���� UI
 * - ��������ƴ�Ӻ���ַ���
 */

import type { ReactElement } from 'react';

/** StreamView ���� */
export interface StreamViewProps {
  /** �ı��� */
  stream: ReadableStream<string>;
  /** ÿ�յ�һ�� token ʱ�Ļص� */
  onToken?: (token: string) => void;
}

/**
 * StreamView �� �����ı��������������ı�
 *
 * @param props �� stream + onToken
 * @returns ����ƴ�Ӻ���ı�
 */
export async function StreamView({ stream, onToken }: StreamViewProps): Promise<string> {
  const reader = stream.getReader();
  let text = '';
  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    text += value;
    onToken?.(value);
  }
  return text;
}

export default StreamView;

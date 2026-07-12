/**
 * useAgent �� �ͻ��� Hook���� AgentPrimordia REST API ����
 *
 * ʹ�÷�ʽ��
 *   const { messages, loading, send, abort, clear } = useAgent('agent-id');
 *
 * ���Ҫ�㣺
 * - ͨ�� fetch �� /api/playground/agent/:id/chat ����
 * - ֧�� AbortController ȡ������
 * - ������������״̬������
 * - ʹ�� React 18+ hooks
 */

import { useState, useCallback, useRef } from 'react';

/** ��Ϣ�ṹ */
export interface Message {
  role: 'user' | 'assistant' | 'system' | 'tool';
  content: string;
}

/** useAgent ����ֵ */
export interface UseAgentResult {
  /** ��Ϣ��ʷ */
  messages: Message[];
  /** �Ƿ��������� */
  loading: boolean;
  /** ������Ϣ */
  send: (content: string) => Promise<string>;
  /** ȡ����ǰ���� */
  abort: () => void;
  /** �����Ϣ��ʷ */
  clear: () => void;
}

/** useAgent ѡ�� */
export interface UseAgentOptions {
  /** API ����·����Ĭ�� /api */
  apiBase?: string;
  /** ��ʼ��Ϣ�б� */
  initialMessages?: Message[];
}

/**
 * useAgent Hook �� �� Agent ���жԻ�
 *
 * @param agentId �� Agent ID
 * @param options �� ����ѡ��
 */
export function useAgent(agentId: string, options: UseAgentOptions = {}): UseAgentResult {
  const { apiBase = '/api', initialMessages = [] } = options;
  const [messages, setMessages] = useState<Message[]>(initialMessages);
  const [loading, setLoading] = useState(false);
  const abortRef = useRef<AbortController | null>(null);

  const send = useCallback(async (content: string): Promise<string> => {
    setLoading(true);
    const controller = new AbortController();
    abortRef.current = controller;

    const userMsg: Message = { role: 'user', content };
    setMessages(prev => [...prev, userMsg]);

    try {
      const resp = await fetch(`${apiBase}/playground/agent/${agentId}/chat`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ message: content }),
        signal: controller.signal,
      });

      if (!resp.ok) {
        throw new Error(`HTTP ${resp.status}: ${resp.statusText}`);
      }

      const data = await resp.json();
      const assistantMsg: Message = { role: 'assistant', content: data.response ?? '' };
      setMessages(prev => [...prev, assistantMsg]);
      return assistantMsg.content;
    } catch (e) {
      if ((e as Error).name === 'AbortError') {
        return '';
      }
      throw e;
    } finally {
      setLoading(false);
    }
  }, [agentId, apiBase]);

  const abort = useCallback(() => {
    abortRef.current?.abort();
  }, []);

  const clear = useCallback(() => {
    setMessages([]);
  }, []);

  return { messages, loading, send, abort, clear };
}

export default useAgent;

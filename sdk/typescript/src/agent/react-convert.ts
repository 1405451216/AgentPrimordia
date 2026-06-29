// react-convert.ts — 消息类型转换工具
// 包含 Message → LLM ChatMessage、OpenAI 格式、工具定义等转换
// Mirrors Go internal/agent/react_convert.go

import type { Message, ToolCall, ToolDefinition } from '../types.js';

// ===== OpenAI 兼容类型 =====

export interface OpenAIMessage {
  role: 'system' | 'user' | 'assistant' | 'tool';
  content: string | OpenAIContentItem[];
  tool_calls?: OpenAIToolCall[];
  tool_call_id?: string;
  name?: string;
}

export interface OpenAIContentItem {
  type: string;
  text?: string;
  image_url?: {
    url: string;
    detail?: string;
  };
  input_audio?: {
    data: string;
    format: string;
  };
}

export interface OpenAIToolCall {
  id: string;
  type: 'function';
  function: {
    name: string;
    arguments: string;
  };
}

export interface OpenAIToolDefinition {
  type: 'function';
  function: {
    name: string;
    description: string;
    parameters: Record<string, unknown>;
  };
}

// ===== 多模态内容类型 =====

export interface ContentPart {
  type: 'text' | 'image_url' | 'image_b64' | 'audio';
  text?: string;
  url?: string;
  detail?: string;
  data?: string;
  mime?: string;
}

// ===== Message 扩展类型 =====

export interface ExtendedMessage extends Message {
  contentParts?: ContentPart[];
  metadata?: {
    extra?: Record<string, string>;
  };
}

// ===== 转换函数 =====

/** 将内部 Message 转换为 OpenAI 兼容格式 */
export function toOpenAIMessages(messages: Message[]): OpenAIMessage[] {
  return messages.map((m) => {
    const content = buildMultimodalContent(m);
    const msg: OpenAIMessage = {
      role: m.role,
      content,
    };

    // 工具调用
    if (m.role === 'assistant' && m.toolCalls && m.toolCalls.length > 0) {
      msg.tool_calls = m.toolCalls.map((tc) => ({
        id: tc.id,
        type: 'function' as const,
        function: {
          name: tc.name,
          arguments: tc.arguments,
        },
      }));
    }

    // 工具调用 ID
    if (m.role === 'tool' && m.toolCallId) {
      msg.tool_call_id = m.toolCallId;
    }

    // 名称
    if (m.name) {
      msg.name = m.name;
    }

    return msg;
  });
}

/** 构建多模态内容（兼容 OpenAI Vision API） */
export function buildMultimodalContent(m: Message): string | OpenAIContentItem[] {
  const ext = m as ExtendedMessage;
  if (!ext.contentParts || ext.contentParts.length === 0) {
    return m.content;
  }

  const items: OpenAIContentItem[] = ext.contentParts.map((p) => {
    switch (p.type) {
      case 'text':
        return { type: 'text', text: p.text ?? '' };
      case 'image_url':
        return {
          type: 'image_url',
          image_url: { url: p.url ?? '', detail: p.detail },
        };
      case 'image_b64':
        return {
          type: 'image_url',
          image_url: {
            url: `data:${p.mime ?? 'image/png'};base64,${p.data}`,
            detail: p.detail,
          },
        };
      case 'audio':
        return {
          type: 'input_audio',
          input_audio: {
            data: p.data ?? '',
            format: p.mime ?? 'wav',
          },
        };
      default:
        return { type: 'text', text: p.text ?? '' };
    }
  });

  return items;
}

/** 将 ToolDefinition 转换为 OpenAI 兼容格式 */
export function toOpenAIToolDefinitions(tools: ToolDefinition[]): OpenAIToolDefinition[] {
  return tools.map((t) => ({
    type: 'function' as const,
    function: {
      name: t.function.name,
      description: t.function.description,
      parameters: t.function.parameters,
    },
  }));
}

/** 将 OpenAI 工具调用转换回内部 ToolCall */
export function fromOpenAIToolCalls(calls: OpenAIToolCall[]): ToolCall[] {
  return calls.map((c) => ({
    id: c.id,
    name: c.function.name,
    arguments: c.function.arguments,
  }));
}

/** 将内部 ToolCall 转换为 OpenAI 工具调用 */
export function toOpenAIToolCalls(calls: ToolCall[]): OpenAIToolCall[] {
  return calls.map((c) => ({
    id: c.id,
    type: 'function' as const,
    function: {
      name: c.name,
      arguments: c.arguments,
    },
  }));
}

/** 从 OpenAI 消息中提取内部 Message */
export function fromOpenAIMessage(msg: OpenAIMessage): Message {
  const content = typeof msg.content === 'string' ? msg.content : JSON.stringify(msg.content);

  const message: Message = {
    role: msg.role as Message['role'],
    content,
  };

  if (msg.tool_calls && msg.tool_calls.length > 0) {
    message.toolCalls = fromOpenAIToolCalls(msg.tool_calls);
  }

  if (msg.tool_call_id) {
    message.toolCallId = msg.tool_call_id;
  }

  if (msg.name) {
    message.name = msg.name;
  }

  return message;
}

/** 从 OpenAI 消息列表中提取内部 Message 列表 */
export function fromOpenAIMessages(messages: OpenAIMessage[]): Message[] {
  return messages.map(fromOpenAIMessage);
}

// ===== 内容提取辅助函数 =====

/** 提取消息的纯文本内容 */
export function extractTextContent(msg: Message): string {
  const ext = msg as ExtendedMessage;
  if (ext.contentParts && ext.contentParts.length > 0) {
    return ext.contentParts
      .filter((p) => p.type === 'text')
      .map((p) => p.text ?? '')
      .join('\n');
  }
  return msg.content;
}

/** 检查消息是否包含多模态内容 */
export function hasMultimodal(msg: Message): boolean {
  const ext = msg as ExtendedMessage;
  return !!(ext.contentParts && ext.contentParts.length > 0);
}

/** 构建消息历史摘要（用于调试和日志） */
export function summarizeHistory(messages: Message[], maxChars: number = 200): string {
  const parts: string[] = [];
  let total = 0;

  for (const m of messages) {
    const text = extractTextContent(m);
    const line = `[${m.role}] ${text.slice(0, 50)}`;
    if (total + line.length > maxChars) {
      parts.push(`... (${messages.length - parts.length} more messages)`);
      break;
    }
    parts.push(line);
    total += line.length;
  }

  return parts.join('\n');
}
/**
 * types.ts — Agent 层核心类型定义
 * 镜像 Go internal/agent/types.go + internal/agent/multimodal/multimodal.go + internal/agent/lifecycle/lifecycle.go
 */

// ===== Role 消息角色 =====

/** 消息角色类型，与 Go Role 一致 */
export type Role = "system" | "user" | "assistant" | "tool";

/** 系统消息角色常量 */
export const RoleSystem: Role = "system";

/** 用户消息角色常量 */
export const RoleUser: Role = "user";

/** 助手消息角色常量 */
export const RoleAssistant: Role = "assistant";

/** 工具消息角色常量 */
export const RoleTool: Role = "tool";

// ===== Metadata 消息元数据 =====

/** 消息元数据，与 Go Metadata 一致 */
export interface Metadata {
  /** 会话 ID */
  sessionId?: string;
  /** 时间戳 */
  timestamp: Date;
  /** 附加键值对 */
  extra?: Record<string, string>;
}

// ===== ContentPart 多模态内容片段 =====

/**
 * 多模态内容片段，镜像 Go multimodal.ContentPart
 * type 可为: text, image_url, image_b64, audio, video 等
 */
export interface ContentPart {
  /** 内容类型：text, image_url, image_b64, audio, video */
  type: string;
  /** 文本内容（type 为 text 时使用） */
  text?: string;
  /** 图片/视频 URL（type 为 image_url 时使用） */
  imageUrl?: string;
  /** Base64 编码数据（type 为 image_b64/audio/video 时使用） */
  data?: unknown;
}

// ===== Message 对话消息 =====

/** 对话消息，与 Go Message 一致 */
export interface Message {
  /** 消息角色 */
  role: Role;
  /** 文本内容 */
  content: string;
  /** 多模态内容片段列表 */
  contentParts?: ContentPart[];
  /** LLM 请求的工具调用列表 */
  toolCalls?: ToolCall[];
  /** 消息元数据 */
  metadata?: Metadata;
}

// ===== ToolCall LLM 请求的工具调用 =====

/** LLM 请求的工具调用，与 Go ToolCall 一致 */
export interface ToolCall {
  /** 调用唯一 ID */
  id: string;
  /** 工具名称 */
  name: string
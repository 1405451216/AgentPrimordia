/**
 * Agent 模板定义
 * 用于 CodeGenerator 生成 Agent 类代码时的模板配置
 */

/** Agent 模板配置 */
export interface AgentTemplateConfig {
  /** Agent 类名 */
  name: string;
  /** Agent 描述 */
  description: string;
  /** 系统提示词 */
  systemPrompt: string;
  /** 模型名称 */
  model?: string;
  /** 可用工具名称列表 */
  tools?: string[];
  /** 温度参数 */
  temperature?: number;
  /** 最大 token 数 */
  maxTokens?: number;
}
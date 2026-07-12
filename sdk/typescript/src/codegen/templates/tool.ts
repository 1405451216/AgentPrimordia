/**
 * Tool 模板定义
 * 用于 CodeGenerator 生成工具代码时的模板配置
 */

/** 工具参数定义 */
export interface ToolParameter {
  type: string;
  description: string;
  required: boolean;
}

/** 工具模板配置 */
export interface ToolTemplate {
  name: string;
  description: string;
  parameters: Record<string, ToolParameter>;
}
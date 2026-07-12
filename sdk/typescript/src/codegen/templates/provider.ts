/**
 * Provider 模板定义
 * 用于 CodeGenerator 生成 LLM Provider 骨架时的模板配置
 */

/** Provider 模板配置 */
export interface ProviderTemplateConfig {
  /** Provider 名称 */
  name: string;
  /** 基础 URL */
  baseURL: string;
  /** 默认模型 */
  defaultModel: string;
  /** API 版本 */
  apiVersion?: string;
}
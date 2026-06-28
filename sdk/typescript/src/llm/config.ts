// config.ts 实现 LLM 配置验证、环境变量加载和热重载
// 与 Go 端 config.go 对齐

import type { ProviderConfig } from '../types.js';

// ===== 配置验证 =====

/** 验证配置有效性，与 Go 端 Config.Validate 对齐 */
export function validateConfig(config: ProviderConfig): string[] {
  const errs: string[] = [];

  if (!config.model) {
    errs.push('model is required');
  }

  // 验证 Temperature 范围（放宽到 [0, 3]，部分本地模型支持更高温度）
  if (config.temperature !== undefined && (config.temperature < 0 || config.temperature > 3)) {
    errs.push(`temperature must be between 0 and 3, got ${config.temperature}`);
  }

  // 验证 MaxTokens 范围
  if (config.maxTokens !== undefined && config.maxTokens < 0) {
    errs.push(`max_tokens must be >= 0, got ${config.maxTokens}`);
  }

  // 验证 BaseURL 格式
  if (config.baseURL && !config.baseURL.startsWith('http://') && !config.baseURL.startsWith('https://')) {
    errs.push(`base_url must start with http:// or https://, got "${config.baseURL}"`);
  }

  return errs;
}

/** 验证配置并抛出错误（如果有问题），与 Go 端 Config.Validate 对齐 */
export function validateConfigOrThrow(config: ProviderConfig): void {
  const errs = validateConfig(config);
  if (errs.length > 0) {
    throw new Error(`config validation failed: ${errs.join('; ')}`);
  }
}

// ===== 环境变量加载 =====

/** 从环境变量加载配置，与 Go 端 ConfigFromEnv 对齐
 *
 * 使用方式：
 *   const config = configFromEnv('AP_LLM');
 *   // 读取 AP_LLM_API_KEY, AP_LLM_BASE_URL, AP_LLM_MODEL 等
 */
export function configFromEnv(prefix: string = 'AP_LLM'): ProviderConfig {
  const getEnv = (key: string): string | undefined => {
    // Node.js 环境
    if (typeof process !== 'undefined' && process.env) {
      return process.env[key] || undefined;
    }
    return undefined;
  };

  const cfg: ProviderConfig = {
    apiKey: getEnv(`${prefix}_API_KEY`) ?? '',
    baseURL: getEnv(`${prefix}_BASE_URL`),
    model: getEnv(`${prefix}_MODEL`),
    temperature: envFloat(getEnv(`${prefix}_TEMPERATURE`), 0),
    maxTokens: envInt(getEnv(`${prefix}_MAX_TOKENS`), 0),
  };

  return cfg;
}

/** 从环境变量加载配置，返回已验证的配置 */
export function configFromEnvValidated(prefix: string = 'AP_LLM'): ProviderConfig {
  const config = configFromEnv(prefix);
  validateConfigOrThrow(config);
  return config;
}

// ===== 环境变量辅助函数 =====

function envFloat(value: string | undefined, def: number): number | undefined {
  if (!value) return def === 0 ? undefined : def;
  const parsed = parseFloat(value);
  return isNaN(parsed) ? (def === 0 ? undefined : def) : parsed;
}

function envInt(value: string | undefined, def: number): number | undefined {
  if (!value) return def === 0 ? undefined : def;
  const parsed = parseInt(value, 10);
  return isNaN(parsed) ? (def === 0 ? undefined : def) : parsed;
}

// ===== 配置热重载 =====

/** 配置变更回调，与 Go 端 ConfigWatcher 对齐 */
export type ConfigChangeCallback = (config: ProviderConfig) => void;

/** LLM 配置热重载选项 */
export interface LLMConfigWatcherOptions {
  /** 检查间隔（毫秒，默认 30000） */
  interval?: number;
  /** 环境变量前缀（默认 AP_LLM） */
  prefix?: string;
}

/** LLM 配置热重载器
 *
 * 定期检查 LLM 配置变化并在变化时回调。
 * 与 Go 端 config 包中的热重载能力对齐。
 *
 * 使用方式：
 *   const watcher = new LLMConfigWatcher((config) => {
 *     console.log('LLM 配置已更新:', config);
 *   });
 *   watcher.start();
 *   // ... 稍后
 *   watcher.stop();
 */
export class LLMConfigWatcher {
  private callback: ConfigChangeCallback;
  private interval: number;
  private prefix: string;
  private currentConfig: ProviderConfig;
  private timer: ReturnType<typeof setInterval> | null = null;

  constructor(callback: ConfigChangeCallback, options?: LLMConfigWatcherOptions) {
    this.callback = callback;
    this.interval = options?.interval ?? 30000;
    this.prefix = options?.prefix ?? 'AP_LLM';
    this.currentConfig = configFromEnv(this.prefix);
  }

  /** 开始监听配置变化 */
  start(): void {
    if (this.timer) return;
    this.timer = setInterval(() => {
      const newConfig = configFromEnv(this.prefix);
      if (!configsEqual(this.currentConfig, newConfig)) {
        this.currentConfig = newConfig;
        this.callback(newConfig);
      }
    }, this.interval);
  }

  /** 停止监听 */
  stop(): void {
    if (this.timer) {
      clearInterval(this.timer);
      this.timer = null;
    }
  }

  /** 获取当前配置 */
  getConfig(): ProviderConfig {
    return { ...this.currentConfig };
  }
}

/** 比较两个配置是否相等 */
function configsEqual(a: ProviderConfig, b: ProviderConfig): boolean {
  return (
    a.apiKey === b.apiKey &&
    a.baseURL === b.baseURL &&
    a.model === b.model &&
    a.temperature === b.temperature &&
    a.maxTokens === b.maxTokens
  );
}
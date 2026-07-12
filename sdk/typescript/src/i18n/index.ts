/**
 * 国际化 (i18n) 模块
 *
 * 为 AgentPrimordia SDK 提供多语言支持，包括：
 * - 运行时 locale 切换
 * - 参数插值
 * - 错误消息多语言化
 *
 * 支持的语言：
 * - en (默认)
 * - zh-CN
 * - ja
 *
 * @example
 * import { setLocale, t } from '@agentprimordia/sdk/i18n';
 *
 * setLocale('zh-CN');
 * console.log(t('timeout.operation', { seconds: 30 }));
 * // => "操作在 30 秒后超时"
 */

import { en, type LocaleKey } from './locales/en.js';
import { zhCN } from './locales/zh-CN.js';
import { ja } from './locales/ja.js';

export type { LocaleKey } from './locales/en.js';

/** 支持的语言列表 */
export type Locale = 'en' | 'zh-CN' | 'ja';

/** 语言包映射表 */
const LOCALES: Record<Locale, Record<string, string>> = {
  en,
  'zh-CN': zhCN,
  ja,
};

/** 当前 locale (默认英文) */
let currentLocale: Locale = 'en';

/**
 * 设置当前 locale
 *
 * @param locale - 目标语言标识
 * @throws 当 locale 不在支持范围内时抛出错误
 *
 * @example
 * setLocale('zh-CN'); // 切换到中文
 * setLocale('ja');    // 切换到日文
 */
export function setLocale(locale: Locale): void {
  if (!(locale in LOCALES)) {
    console.warn(`[i18n] Unsupported locale: ${locale}, falling back to 'en'`);
    currentLocale = 'en';
    return;
  }
  currentLocale = locale;
}

/**
 * 获取当前 locale
 *
 * @returns 当前设置的语言标识
 */
export function getLocale(): Locale {
  return currentLocale;
}

/**
 * 获取指定 locale 的翻译映射表
 */
function getMessages(locale: Locale): Record<string, string> {
  return LOCALES[locale] ?? LOCALES.en;
}

/**
 * 参数插值
 *
 * 将模板字符串中的 {param} 占位符替换为实际值。
 *
 * @param template - 模板字符串，包含 {key} 占位符
 * @param params - 插值参数映射
 * @returns 插值后的字符串
 *
 * @example
 * interpolate('Hello, {name}!', { name: 'World' });
 * // => "Hello, World!"
 *
 * interpolate('Timeout after {seconds}s', { seconds: 30 });
 * // => "Timeout after 30s"
 */
export function interpolate(template: string, params?: Record<string, string | number>): string {
  if (!params) return template;

  return template.replace(/\{(\w+)\}/g, (match, key) => {
    const value = params[key];
    return value !== undefined ? String(value) : match;
  });
}

/**
 * 翻译函数 - 根据 key 获取本地化字符串
 *
 * 自动进行参数插值。如果 key 不存在，返回 key 本身。
 *
 * @param key - 翻译键 (LocaleKey)
 * @param params - 插值参数
 * @returns 本地化字符串
 *
 * @example
 * t('timeout.operation', { seconds: 30 });
 * // => "Operation timed out after 30 seconds"
 *
 * t('tool.notFound', { name: 'web_search' });
 * // => "Tool \"web_search\" not found"
 */
export function t(key: string, params?: Record<string, string | number>): string {
  const messages = getMessages(currentLocale);
  const template = messages[key];

  if (template === undefined) {
    // 回退到英文
    const enTemplate = LOCALES.en[key];
    if (enTemplate === undefined) {
      console.warn(`[i18n] Missing translation key: ${key}`);
      return key;
    }
    return interpolate(enTemplate, params);
  }

  return interpolate(template, params);
}

/**
 * 检查指定 key 是否存在
 *
 * @param key - 翻译键
 * @returns 是否存在
 */
export function hasKey(key: string): boolean {
  const messages = getMessages(currentLocale);
  return key in messages;
}

/**
 * 获取支持的语言列表
 */
export function getSupportedLocales(): Locale[] {
  return Object.keys(LOCALES) as Locale[];
}

/**
 * 批量注册自定义翻译
 *
 * @param locale - 目标语言
 * @param messages - 翻译映射
 *
 * @example
 * mergeMessages('en', { 'custom.key': 'Custom Value' });
 */
export function mergeMessages(locale: Locale, messages: Record<string, string>): void {
  if (LOCALES[locale]) {
    Object.assign(LOCALES[locale], messages);
  }
}

// 导出消息对象（用于工具或测试）
export const messages = LOCALES;